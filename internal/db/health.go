package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthChecker provides database health check functionality.
// Returns nil if the database is reachable and responds within timeout.
// Based on: docs/service-decomposition.md §3.3
func HealthChecker(pool *pgxpool.Pool, timeout time.Duration) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		if err := pool.Ping(checkCtx); err != nil {
			return fmt.Errorf("db health check failed: %w", err)
		}
		return nil
	}
}
