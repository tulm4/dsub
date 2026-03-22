package db

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxOptions configures transaction behavior.
type TxOptions struct {
	// ReadOnly sets the transaction access mode.
	ReadOnly bool
	// MaxRetries is the maximum number of retries on serialization conflict (40001).
	// Default: 3
	MaxRetries int
	// MaxRetryDelay is the maximum delay between retries.
	// Default: 1s
	MaxRetryDelay time.Duration
}

// DefaultTxOptions returns default transaction options.
func DefaultTxOptions() TxOptions {
	return TxOptions{
		MaxRetries:    3,
		MaxRetryDelay: 1 * time.Second,
	}
}

// serialization conflict error code in PostgreSQL/YugabyteDB.
const serializationConflictCode = "40001"

// WithTx executes the given function within a database transaction.
// On serialization conflict (error code 40001), the transaction is retried
// with exponential backoff per docs/service-decomposition.md §3.3.
func WithTx(ctx context.Context, pool *pgxpool.Pool, opts TxOptions, fn func(tx pgx.Tx) error) error {
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.MaxRetryDelay <= 0 {
		opts.MaxRetryDelay = 1 * time.Second
	}

	txOpts := pgx.TxOptions{
		AccessMode: pgx.ReadWrite,
	}
	if opts.ReadOnly {
		txOpts.AccessMode = pgx.ReadOnly
	}

	var lastErr error
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			delay := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			if delay > opts.MaxRetryDelay {
				delay = opts.MaxRetryDelay
			}
			// Add jitter (0-20% of delay). Guard upper bound so rand.Int64N never receives 0.
			maxJitter := int64(delay) / 5
			if maxJitter > 0 {
				jitter := time.Duration(rand.Int64N(maxJitter))
				delay += jitter
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("db: tx retry canceled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		lastErr = executeTx(ctx, pool, txOpts, fn)
		if lastErr == nil {
			return nil
		}

		// Check if this is a serialization conflict that should be retried
		if !isSerializationConflict(lastErr) {
			return lastErr
		}
	}

	return fmt.Errorf("db: tx failed after %d retries: %w", opts.MaxRetries, lastErr)
}

// executeTx runs a single transaction attempt.
func executeTx(ctx context.Context, pool *pgxpool.Pool, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		// Attempt rollback, but return original error
		_ = tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}

	return nil
}

// isSerializationConflict checks if the error is a PostgreSQL serialization conflict.
func isSerializationConflict(err error) bool {
	if err == nil {
		return false
	}
	// pgx wraps PostgreSQL errors - check for the SQLSTATE code
	// The pgx library uses pgconn.PgError with Code field
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == serializationConflictCode
	}
	return false
}
