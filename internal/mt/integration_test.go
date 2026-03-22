//go:build integration

// Package mt integration tests validate the MT service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/mt/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
package mt

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/db"
	"github.com/tulm4/dsub/migrations"
)

const integrationDefaultDSN = "postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable"

// testPool returns a connected pgxpool for integration tests, or skips the test
// if the database is unreachable.
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

// cleanSchema drops the udm schema and migration tracking table to ensure
// a clean state for each test run.
func cleanSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS udm CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

// applyMigrations applies all parsed migrations. Tablespace migration (version 26)
// is skipped on single-node YugabyteDB because CREATE TABLESPACE requires a
// multi-node cluster with placement info configured on each tserver.
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

// setupSchema creates the pool, cleans the schema, and applies all migrations.
// Returns the pool and a background context for test queries.
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

// seedSubscriber inserts a test subscriber into udm.subscribers.
func seedSubscriber(t *testing.T, pool *pgxpool.Pool, ctx context.Context, supi string) {
	t.Helper()

	_, err := pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, 'imsi')",
		supi,
	)
	if err != nil {
		t.Fatalf("seed subscriber %s: %v", supi, err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", supi)
	})
}

// seedAmfRegistration inserts a 3GPP AMF registration for the given SUPI.
func seedAmfRegistration(t *testing.T, pool *pgxpool.Pool, ctx context.Context, supi string) {
	t.Helper()

	guami := json.RawMessage(`{"plmnId":{"mcc":"001","mnc":"01"},"amfId":"010001"}`)

	_, err := pool.Exec(ctx,
		`INSERT INTO udm.amf_registrations
			(supi, access_type, amf_instance_id, dereg_callback_uri, guami, rat_type)
		 VALUES ($1, '3GPP_ACCESS', 'amf-inttest-001', 'https://amf.example.com/dereg', $2, 'NR')`,
		supi, guami,
	)
	if err != nil {
		t.Fatalf("seed AMF registration for %s: %v", supi, err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.amf_registrations WHERE supi = $1 AND access_type = '3GPP_ACCESS'",
			supi,
		)
	})
}

// TestIntegrationQueryUeInfo seeds a subscriber and AMF registration, then
// queries UE info and verifies the serving AMF information.
//
// Based on: docs/sbi-api-design.md §3.6 (GET /{supi})
// 3GPP: TS 29.503 Nudm_MT — QueryUeInfo
func TestIntegrationQueryUeInfo(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000401"
	seedSubscriber(t, pool, ctx, testSUPI)
	seedAmfRegistration(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	ueInfo, err := svc.QueryUeInfo(ctx, testSUPI)
	if err != nil {
		t.Fatalf("QueryUeInfo: %v", err)
	}
	if ueInfo.ServingAmfId != "amf-inttest-001" {
		t.Errorf("ServingAmfId = %q, want %q", ueInfo.ServingAmfId, "amf-inttest-001")
	}
	if ueInfo.UserState != "REGISTERED" {
		t.Errorf("UserState = %q, want %q", ueInfo.UserState, "REGISTERED")
	}
	if ueInfo.RatType != "NR" {
		t.Errorf("RatType = %q, want %q", ueInfo.RatType, "NR")
	}
	if ueInfo.AccessType != "3GPP_ACCESS" {
		t.Errorf("AccessType = %q, want %q", ueInfo.AccessType, "3GPP_ACCESS")
	}
}

// TestIntegrationQueryUeInfo_NotFound queries UE info for a subscriber with
// no AMF registration and verifies a 404 error.
//
// Based on: docs/sbi-api-design.md §7 (Error Handling)
// 3GPP: TS 29.503 — CONTEXT_NOT_FOUND cause code
func TestIntegrationQueryUeInfo_NotFound(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000402"
	seedSubscriber(t, pool, ctx, testSUPI)
	// No AMF registration seeded

	svc := NewService(pool)

	_, err := svc.QueryUeInfo(ctx, testSUPI)
	if err == nil {
		t.Fatal("expected error for unregistered SUPI, got nil")
	}

	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *errors.ProblemDetails, got %T", err)
	}
	if pd.Status != 404 {
		t.Errorf("ProblemDetails.Status = %d, want 404", pd.Status)
	}
}

// TestIntegrationProvideLocationInfo seeds a subscriber and AMF registration,
// calls ProvideLocationInfo, and verifies the result.
//
// Based on: docs/sbi-api-design.md §3.6 (POST /{supi}/loc-info/provide-loc-info)
// 3GPP: TS 29.503 Nudm_MT — ProvideLocationInfo
func TestIntegrationProvideLocationInfo(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000403"
	seedSubscriber(t, pool, ctx, testSUPI)
	seedAmfRegistration(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	locReq := &LocationInfoRequest{
		Req5gsInd: true,
	}
	result, err := svc.ProvideLocationInfo(ctx, testSUPI, locReq)
	if err != nil {
		t.Fatalf("ProvideLocationInfo: %v", err)
	}
	if result.Supi != testSUPI {
		t.Errorf("Supi = %q, want %q", result.Supi, testSUPI)
	}
	if result.ServingAmfId != "amf-inttest-001" {
		t.Errorf("ServingAmfId = %q, want %q", result.ServingAmfId, "amf-inttest-001")
	}
	if result.UserState != "REGISTERED" {
		t.Errorf("UserState = %q, want %q", result.UserState, "REGISTERED")
	}
	if result.RatType != "NR" {
		t.Errorf("RatType = %q, want %q", result.RatType, "NR")
	}
}
