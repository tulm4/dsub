//go:build integration

// Package rsds integration tests validate the RSDS service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/rsds/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
package rsds

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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

// TestIntegrationReportSMDeliveryStatus inserts a delivery status and verifies it.
func TestIntegrationReportSMDeliveryStatus(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	req := &SmDeliveryStatus{
		Gpsi:           "msisdn-16505550701",
		SmStatusReport: json.RawMessage(`{"status":"delivered","timestamp":"2026-03-23T12:00:00Z"}`),
	}
	err := svc.ReportSMDeliveryStatus(ctx, "msisdn-16505550701", req)
	if err != nil {
		t.Fatalf("ReportSMDeliveryStatus: %v", err)
	}

	// Verify the record was inserted
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.sms_delivery_status WHERE gpsi = $1",
		"msisdn-16505550701",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 delivery status record, got %d", count)
	}
}

// TestIntegrationReportSMDeliveryStatus_WithSUPI inserts a delivery status
// with SUPI identity and verifies the SUPI is recorded.
func TestIntegrationReportSMDeliveryStatus_WithSUPI(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	req := &SmDeliveryStatus{
		Gpsi:           "msisdn-16505550702",
		SmStatusReport: json.RawMessage(`{"status":"failed"}`),
	}
	err := svc.ReportSMDeliveryStatus(ctx, "imsi-001010000000702", req)
	if err != nil {
		t.Fatalf("ReportSMDeliveryStatus: %v", err)
	}

	var supi *string
	err = pool.QueryRow(ctx,
		"SELECT supi FROM udm.sms_delivery_status WHERE gpsi = $1",
		"msisdn-16505550702",
	).Scan(&supi)
	if err != nil {
		t.Fatalf("query supi: %v", err)
	}
	if supi == nil || *supi != "imsi-001010000000702" {
		t.Errorf("expected supi imsi-001010000000702, got %v", supi)
	}
}
