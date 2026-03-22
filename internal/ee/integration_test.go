//go:build integration

// Package ee integration tests validate the EE service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/ee/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
package ee

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

// TestIntegrationCreateSubscription seeds a subscriber, creates an EE
// subscription, and verifies the subscription ID is returned.
//
// Based on: docs/sbi-api-design.md §3.4 (POST /{ueIdentity}/ee-subscriptions)
// 3GPP: TS 29.503 Nudm_EE — CreateEeSubscription
func TestIntegrationCreateSubscription(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000201"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/ee-callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`),
		NfInstanceID:             "nef-001",
	}

	result, err := svc.CreateSubscription(ctx, testSUPI, sub)
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if result.SubscriptionID == "" {
		t.Fatal("CreateSubscription returned empty SubscriptionID")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.ee_subscriptions WHERE subscription_id = $1",
			result.SubscriptionID,
		)
	})
}

// TestIntegrationUpdateSubscription creates a subscription then updates it,
// verifying the updated fields are returned.
//
// Based on: docs/sbi-api-design.md §3.4 (PATCH /{ueIdentity}/ee-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_EE — UpdateEeSubscription
func TestIntegrationUpdateSubscription(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000202"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/ee-callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`),
		NfInstanceID:             "nef-001",
	}

	created, err := svc.CreateSubscription(ctx, testSUPI, sub)
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	subID := created.SubscriptionID

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.ee_subscriptions WHERE subscription_id = $1", subID,
		)
	})

	updatedCallback := "https://nef.example.com/ee-callback-updated"
	patch := &PatchEeSubscription{
		CallbackReference: &updatedCallback,
	}

	result, err := svc.UpdateSubscription(ctx, testSUPI, subID, patch)
	if err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}
	if result.CallbackReference != updatedCallback {
		t.Errorf("CallbackReference = %q, want %q", result.CallbackReference, updatedCallback)
	}
}

// TestIntegrationDeleteSubscription creates a subscription then deletes it,
// verifying the subscription no longer exists.
//
// Based on: docs/sbi-api-design.md §3.4 (DELETE /{ueIdentity}/ee-subscriptions/{subscriptionId})
// 3GPP: TS 29.503 Nudm_EE — DeleteEeSubscription
func TestIntegrationDeleteSubscription(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000203"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/ee-callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`),
	}

	created, err := svc.CreateSubscription(ctx, testSUPI, sub)
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	subID := created.SubscriptionID

	if err := svc.DeleteSubscription(ctx, testSUPI, subID); err != nil {
		t.Fatalf("DeleteSubscription: %v", err)
	}

	// Verify deletion
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.ee_subscriptions WHERE subscription_id = $1",
		subID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("subscription still exists after DeleteSubscription, count = %d", count)
	}
}

// TestIntegrationGetMatchingSubscriptions creates two subscriptions for the
// same SUPI and verifies GetMatchingSubscriptions returns both.
//
// Based on: docs/sequence-diagrams.md §10 (Event Exposure)
// 3GPP: TS 29.503 Nudm_EE — Event notification
func TestIntegrationGetMatchingSubscriptions(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000204"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	sub1 := &EeSubscription{
		CallbackReference:        "https://nef.example.com/ee-callback-1",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`),
	}
	sub2 := &EeSubscription{
		CallbackReference:        "https://nef.example.com/ee-callback-2",
		MonitoringConfigurations: json.RawMessage(`{"cfg2":{"eventType":"UE_REACHABILITY_FOR_DATA"}}`),
	}

	created1, err := svc.CreateSubscription(ctx, testSUPI, sub1)
	if err != nil {
		t.Fatalf("CreateSubscription 1: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.ee_subscriptions WHERE subscription_id = $1",
			created1.SubscriptionID,
		)
	})

	created2, err := svc.CreateSubscription(ctx, testSUPI, sub2)
	if err != nil {
		t.Fatalf("CreateSubscription 2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.ee_subscriptions WHERE subscription_id = $1",
			created2.SubscriptionID,
		)
	})

	reports, err := svc.GetMatchingSubscriptions(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetMatchingSubscriptions: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("GetMatchingSubscriptions returned %d reports, want 2", len(reports))
	}
}

// TestIntegrationDeleteSubscription_NotFound verifies that deleting a
// non-existent subscription returns a 404 error.
//
// Based on: docs/sbi-api-design.md §7 (Error Handling)
// 3GPP: TS 29.503 — SUBSCRIPTION_NOT_FOUND cause code
func TestIntegrationDeleteSubscription_NotFound(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000205"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	err := svc.DeleteSubscription(ctx, testSUPI, "nonexistent-sub-id")
	if err == nil {
		t.Fatal("expected error for non-existent subscription, got nil")
	}

	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *errors.ProblemDetails, got %T", err)
	}
	if pd.Status != 404 {
		t.Errorf("ProblemDetails.Status = %d, want 404", pd.Status)
	}
}
