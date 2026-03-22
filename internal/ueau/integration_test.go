//go:build integration

// Package ueau integration tests validate the UEAU service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/ueau/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// 3GPP: TS 29.503 Nudm_UEAU — UE Authentication service operations
package ueau

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tulm4/dsub/internal/db"
	"github.com/tulm4/dsub/migrations"
)

const integrationDefaultDSN = "postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable"

// Test constants — standard Milenage test vectors.
const (
	testSUPI           = "imsi-001010000000100"
	testServingNetwork = "5G:mnc001.mcc001.3gppnetwork.org"
	testNfInstanceID   = "nf-integration-001"
	testKHex           = "000102030405060708090a0b0c0d0e0f"
	testOPcHex         = "cdc202d5123e20f62b6d676ac72cb318"
	testSQN            = "000000000001"
	testAMF            = "8000"
)

// integrationPool returns a connected pgxpool for integration tests, or skips
// the test if the database is unreachable.
func integrationPool(t *testing.T) *pgxpool.Pool {
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

// cleanIntegrationSchema drops the udm schema and migration tracking table to
// ensure a clean state for each test run.
func cleanIntegrationSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS udm CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

// applyIntegrationMigrations applies all parsed migrations, skipping version 26
// (tablespace) which requires a multi-node YugabyteDB cluster.
func applyIntegrationMigrations(t *testing.T, runner *db.MigrationRunner, migs []db.Migration, ctx context.Context) {
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

// setupSchema applies all migrations and prepares the database for UEAU tests.
func setupSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	migs, err := db.ParseMigrations(migrations.FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}

	applyIntegrationMigrations(t, runner, migs, ctx)
}

// seedTestSubscriber inserts a test subscriber and authentication_data row using
// standard Milenage test vectors.
func seedTestSubscriber(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	kBytes, err := hex.DecodeString(testKHex)
	if err != nil {
		t.Fatalf("decode K hex: %v", err)
	}
	opcBytes, err := hex.DecodeString(testOPcHex)
	if err != nil {
		t.Fatalf("decode OPc hex: %v", err)
	}

	_, err = pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, $2)",
		testSUPI, "imsi",
	)
	if err != nil {
		t.Fatalf("INSERT subscriber: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO udm.authentication_data (supi, auth_method, k_key, opc_key, sqn, amf_value)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		testSUPI, "5G_AKA", kBytes, opcBytes, testSQN, testAMF,
	)
	if err != nil {
		t.Fatalf("INSERT authentication_data: %v", err)
	}
}

// cleanTestData removes test data inserted by seedTestSubscriber.
func cleanTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM udm.authentication_status WHERE supi = $1", testSUPI)
	_, _ = pool.Exec(ctx, "DELETE FROM udm.authentication_data WHERE supi = $1", testSUPI)
	_, _ = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", testSUPI)
}

// TestIntegrationGenerateAuthData verifies that auth vector generation works
// against a real YugabyteDB instance with seeded Milenage credentials.
//
// 3GPP: TS 29.503 Nudm_UEAU — GenerateAuthData
func TestIntegrationGenerateAuthData(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	req := &AuthenticationInfoRequest{
		ServingNetworkName: testServingNetwork,
	}
	result, err := svc.GenerateAuthData(ctx, testSUPI, req)
	if err != nil {
		t.Fatalf("GenerateAuthData error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.AuthType != "5G_AKA" {
		t.Errorf("AuthType = %q, want %q", result.AuthType, "5G_AKA")
	}
	if result.Supi != testSUPI {
		t.Errorf("Supi = %q, want %q", result.Supi, testSUPI)
	}

	av := result.AuthenticationVector
	if av == nil {
		t.Fatal("AuthenticationVector is nil")
	}
	if av.AvType != "5G_HE_AKA" {
		t.Errorf("AvType = %q, want %q", av.AvType, "5G_HE_AKA")
	}
	// RAND = 16 bytes → 32 hex chars
	if len(av.Rand) != 32 {
		t.Errorf("Rand hex length = %d, want 32", len(av.Rand))
	}
	// AUTN = 16 bytes → 32 hex chars
	if len(av.Autn) != 32 {
		t.Errorf("Autn hex length = %d, want 32", len(av.Autn))
	}
	// XRES* = 16 bytes → 32 hex chars
	if len(av.XresStar) != 32 {
		t.Errorf("XresStar hex length = %d, want 32", len(av.XresStar))
	}
	// Kausf = 32 bytes → 64 hex chars
	if len(av.Kausf) != 64 {
		t.Errorf("Kausf hex length = %d, want 64", len(av.Kausf))
	}

	// Verify all fields are valid hex
	for _, f := range []struct{ name, val string }{
		{"Rand", av.Rand},
		{"Autn", av.Autn},
		{"XresStar", av.XresStar},
		{"Kausf", av.Kausf},
	} {
		if _, decErr := hex.DecodeString(f.val); decErr != nil {
			t.Errorf("%s is not valid hex: %v", f.name, decErr)
		}
	}

	// Verify SQN was incremented in the database
	var updatedSQN string
	err = pool.QueryRow(ctx,
		"SELECT sqn FROM udm.authentication_data WHERE supi = $1", testSUPI,
	).Scan(&updatedSQN)
	if err != nil {
		t.Fatalf("SELECT updated SQN: %v", err)
	}
	if updatedSQN != "000000000002" {
		t.Errorf("SQN after GenerateAuthData = %q, want %q", updatedSQN, "000000000002")
	}
}

// TestIntegrationConfirmAuth verifies that an authentication event can be stored
// against a real YugabyteDB instance.
//
// 3GPP: TS 29.503 Nudm_UEAU — ConfirmAuth
func TestIntegrationConfirmAuth(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	event := &AuthEvent{
		NfInstanceID:       testNfInstanceID,
		AuthType:           "5G_AKA",
		ServingNetworkName: testServingNetwork,
		Success:            true,
		TimeStamp:          "2024-01-15T12:00:00Z",
	}
	result, err := svc.ConfirmAuth(ctx, testSUPI, event)
	if err != nil {
		t.Fatalf("ConfirmAuth error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.NfInstanceID != testNfInstanceID {
		t.Errorf("NfInstanceID = %q, want %q", result.NfInstanceID, testNfInstanceID)
	}
	if result.AuthType != "5G_AKA" {
		t.Errorf("AuthType = %q, want %q", result.AuthType, "5G_AKA")
	}

	// Verify the event was persisted in the database
	var authType string
	var success bool
	var nfID string
	err = pool.QueryRow(ctx,
		"SELECT auth_type, success, nf_instance_id FROM udm.authentication_status WHERE supi = $1 AND serving_network_name = $2",
		testSUPI, testServingNetwork,
	).Scan(&authType, &success, &nfID)
	if err != nil {
		t.Fatalf("SELECT authentication_status: %v", err)
	}
	if authType != "5G_AKA" {
		t.Errorf("stored auth_type = %q, want %q", authType, "5G_AKA")
	}
	if !success {
		t.Error("stored success = false, want true")
	}
	if nfID != testNfInstanceID {
		t.Errorf("stored nf_instance_id = %q, want %q", nfID, testNfInstanceID)
	}
}

// TestIntegrationDeleteAuthEvent verifies that an authentication event can be
// deleted from a real YugabyteDB instance.
//
// 3GPP: TS 29.503 Nudm_UEAU — DeleteAuth
func TestIntegrationDeleteAuthEvent(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	ctx := context.Background()

	// Insert an auth event directly so we can delete it
	_, err := pool.Exec(ctx,
		`INSERT INTO udm.authentication_status (supi, serving_network_name, auth_type, success, time_stamp, auth_removal_ind, nf_instance_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		testSUPI, testServingNetwork, "5G_AKA", true, "2024-01-15T12:00:00Z", false, testNfInstanceID,
	)
	if err != nil {
		t.Fatalf("INSERT authentication_status: %v", err)
	}

	svc := NewService(pool)

	// Delete using serving_network_name as the authEventID (composite PK lookup)
	err = svc.DeleteAuthEvent(ctx, testSUPI, testServingNetwork)
	if err != nil {
		t.Fatalf("DeleteAuthEvent error: %v", err)
	}

	// Verify the event was removed
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.authentication_status WHERE supi = $1 AND serving_network_name = $2",
		testSUPI, testServingNetwork,
	).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("authentication_status still exists after delete, count = %d", count)
	}

	// Verify deleting a non-existent event returns not-found error
	err = svc.DeleteAuthEvent(ctx, testSUPI, "5G:mnc999.mcc999.3gppnetwork.org")
	if err == nil {
		t.Error("expected error for non-existent auth event, got nil")
	}
}

// TestIntegrationAuthFlow_EndToEnd exercises the complete authentication flow:
// GenerateAuthData → ConfirmAuth → DeleteAuthEvent against a real database.
//
// Based on: docs/sequence-diagrams.md §2 (5G UE Registration Flow)
// 3GPP: TS 29.503 Nudm_UEAU — Full authentication lifecycle
func TestIntegrationAuthFlow_EndToEnd(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	// Step 1: Generate authentication data
	req := &AuthenticationInfoRequest{
		ServingNetworkName: testServingNetwork,
	}
	genResult, err := svc.GenerateAuthData(ctx, testSUPI, req)
	if err != nil {
		t.Fatalf("Step 1 — GenerateAuthData error: %v", err)
	}
	if genResult.AuthType != "5G_AKA" {
		t.Errorf("Step 1 — AuthType = %q, want %q", genResult.AuthType, "5G_AKA")
	}
	if genResult.AuthenticationVector == nil {
		t.Fatal("Step 1 — AuthenticationVector is nil")
	}

	// Step 2: Confirm authentication (simulate AUSF confirming success)
	event := &AuthEvent{
		NfInstanceID:       testNfInstanceID,
		AuthType:           genResult.AuthType,
		ServingNetworkName: testServingNetwork,
		Success:            true,
		TimeStamp:          time.Now().UTC().Format(time.RFC3339),
	}
	confirmResult, err := svc.ConfirmAuth(ctx, testSUPI, event)
	if err != nil {
		t.Fatalf("Step 2 — ConfirmAuth error: %v", err)
	}
	if !confirmResult.Success {
		t.Error("Step 2 — ConfirmAuth result.Success = false, want true")
	}

	// Verify auth status is persisted
	var storedAuthType string
	err = pool.QueryRow(ctx,
		"SELECT auth_type FROM udm.authentication_status WHERE supi = $1 AND serving_network_name = $2",
		testSUPI, testServingNetwork,
	).Scan(&storedAuthType)
	if err != nil {
		t.Fatalf("Step 2 — SELECT authentication_status: %v", err)
	}
	if storedAuthType != "5G_AKA" {
		t.Errorf("Step 2 — stored auth_type = %q, want %q", storedAuthType, "5G_AKA")
	}

	// Step 3: Delete the authentication event
	err = svc.DeleteAuthEvent(ctx, testSUPI, testServingNetwork)
	if err != nil {
		t.Fatalf("Step 3 — DeleteAuthEvent error: %v", err)
	}

	// Verify auth status was removed
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.authentication_status WHERE supi = $1 AND serving_network_name = $2",
		testSUPI, testServingNetwork,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Step 3 — SELECT count: %v", err)
	}
	if count != 0 {
		t.Errorf("Step 3 — auth event still present after delete, count = %d", count)
	}

	// Verify SQN was incremented (from "000000000001" → "000000000002")
	var finalSQN string
	err = pool.QueryRow(ctx,
		"SELECT sqn FROM udm.authentication_data WHERE supi = $1", testSUPI,
	).Scan(&finalSQN)
	if err != nil {
		t.Fatalf("Step 3 — SELECT final SQN: %v", err)
	}
	if finalSQN != "000000000002" {
		t.Errorf("final SQN = %q, want %q", finalSQN, "000000000002")
	}
}
