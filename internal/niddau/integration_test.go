//go:build integration

// Package niddau integration tests validate the NIDDAU service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/niddau/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.8 (udm-niddau)
package niddau

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/db"
	"github.com/tulm4/dsub/migrations"
)

const integrationDefaultDSN = "postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable"

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("YUGABYTE_DSN")
	if dsn == "" {
		dsn = integrationDefaultDSN
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: cannot create pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: cannot ping database: %v", err)
	}

	t.Cleanup(func() { pool.Close() })
	return pool
}

func cleanSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS udm CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

func applyMigrations(t *testing.T, runner *db.MigrationRunner, migs []db.Migration, ctx context.Context) {
	t.Helper()
	for _, m := range migs {
		if m.Version == 26 {
			t.Logf("skipping migration %d (%s): requires multi-node cluster", m.Version, m.Description)
			continue
		}
		if err := runner.Apply(ctx, m); err != nil {
			t.Fatalf("Apply migration %d (%s) error: %v", m.Version, m.Description, err)
		}
	}
}

func setupSchema(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migs, err := db.ParseMigrations(migrations.FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}

	applyMigrations(t, runner, migs, ctx)
	return pool, ctx
}

func seedSubscriber(t *testing.T, pool *pgxpool.Pool, ctx context.Context, supi, gpsi string) {
	t.Helper()

	_, err := pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type, gpsi) VALUES ($1, 'imsi', $2)",
		supi, gpsi,
	)
	if err != nil {
		t.Fatalf("seed subscriber %s: %v", supi, err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", supi)
	})
}

// TestIntegrationAuthorizeNiddData seeds a subscriber, then authorizes NIDD.
func TestIntegrationAuthorizeNiddData(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000601"
	testGPSI := "msisdn-16505550601"
	seedSubscriber(t, pool, ctx, testSUPI, testGPSI)

	svc := NewService(pool)

	req := &AuthorizationInfo{
		Dnn:          "iot",
		ValidityTime: "2026-12-31T23:59:59Z",
	}
	result, err := svc.AuthorizeNiddData(ctx, testSUPI, req)
	if err != nil {
		t.Fatalf("AuthorizeNiddData: %v", err)
	}
	if len(result.AuthorizationData) != 1 {
		t.Fatalf("expected 1 auth data entry, got %d", len(result.AuthorizationData))
	}
	if result.AuthorizationData[0].Supi != testSUPI {
		t.Errorf("Supi = %q, want %q", result.AuthorizationData[0].Supi, testSUPI)
	}
	if result.AuthorizationData[0].Gpsi != testGPSI {
		t.Errorf("Gpsi = %q, want %q", result.AuthorizationData[0].Gpsi, testGPSI)
	}
}

// TestIntegrationAuthorizeNiddData_NotFound queries NIDD for a nonexistent subscriber.
func TestIntegrationAuthorizeNiddData_NotFound(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	req := &AuthorizationInfo{Dnn: "iot"}
	_, err := svc.AuthorizeNiddData(ctx, "imsi-001010000000602", req)
	if err == nil {
		t.Fatal("expected error for nonexistent subscriber, got nil")
	}

	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *errors.ProblemDetails, got %T", err)
	}
	if pd.Status != 404 {
		t.Errorf("ProblemDetails.Status = %d, want 404", pd.Status)
	}
}
