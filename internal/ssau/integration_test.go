//go:build integration

// Package ssau integration tests validate the SSAU service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/ssau/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
package ssau

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

// TestIntegrationAuthorize creates an authorization and verifies the result.
func TestIntegrationAuthorize(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	req := &ServiceSpecificAuthorizationInfo{
		Dnn:  "ims",
		AfID: "af-001",
	}
	result, err := svc.Authorize(ctx, "msisdn-12025551234", "AF_GUIDANCE_FOR_URSP", req)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if result.AuthID == "" {
		t.Error("expected non-empty authId")
	}
	if result.AuthorizationUeID == nil {
		t.Error("expected non-nil authorizationUeId")
	}
}

// TestIntegrationAuthorizeAndRemove creates an authorization, then removes it.
func TestIntegrationAuthorizeAndRemove(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	authReq := &ServiceSpecificAuthorizationInfo{
		Dnn:  "ims",
		AfID: "af-001",
	}
	result, err := svc.Authorize(ctx, "msisdn-12025551234", "AF_GUIDANCE_FOR_URSP", authReq)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	removeReq := &ServiceSpecificAuthorizationRemoveData{AuthID: result.AuthID}
	err = svc.Remove(ctx, "msisdn-12025551234", "AF_GUIDANCE_FOR_URSP", removeReq)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Removing again should return 404
	err = svc.Remove(ctx, "msisdn-12025551234", "AF_GUIDANCE_FOR_URSP", removeReq)
	if err == nil {
		t.Fatal("expected error on second removal")
	}
	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *errors.ProblemDetails, got %T", err)
	}
	if pd.Status != 404 {
		t.Errorf("ProblemDetails.Status = %d, want 404", pd.Status)
	}
}
