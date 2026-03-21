//go:build integration

// Package migrations integration tests validate schema migrations against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./migrations/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/data-model.md §9 (Data Migration and Versioning)
package migrations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tulm4/dsub/internal/db"
)

const defaultDSN = "postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable"

// testPool returns a connected pgxpool for integration tests, or skips the test
// if the database is unreachable.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("YUGABYTE_DSN")
	if dsn == "" {
		dsn = defaultDSN
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

	// Drop in reverse dependency order
	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS udm CASCADE")
	_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations")
}

// applyMigrations applies all parsed migrations. Tablespace migration (version 26)
// is skipped on single-node YugabyteDB because CREATE TABLESPACE requires a
// multi-node cluster with placement info configured on each tserver.
func applyMigrations(t *testing.T, runner *db.MigrationRunner, migrations []db.Migration, ctx context.Context) {
	t.Helper()
	for _, m := range migrations {
		// Skip tablespace migration — requires multi-node cluster
		if m.Version == 26 {
			t.Logf("skipping migration %d (%s): requires multi-node cluster", m.Version, m.Description)
			continue
		}
		if err := runner.Apply(ctx, m); err != nil {
			t.Fatalf("Apply migration %d (%s) error: %v", m.Version, m.Description, err)
		}
	}
}

// TestIntegrationMigrationsApplyAll verifies that all migrations can be applied
// sequentially against a real YugabyteDB instance without errors.
func TestIntegrationMigrationsApplyAll(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}

	applyMigrations(t, runner, migrations, ctx)

	// Verify all non-skipped migrations recorded (26 total - 1 skipped = 25)
	applied, err := runner.GetAppliedVersions(ctx)
	if err != nil {
		t.Fatalf("GetAppliedVersions error: %v", err)
	}
	expectedApplied := len(migrations) - 1 // minus tablespace migration
	if len(applied) != expectedApplied {
		t.Errorf("applied count = %d, want %d", len(applied), expectedApplied)
	}
}

// TestIntegrationSchemaTablesExist verifies that after applying all migrations,
// the expected tables exist in the udm schema.
func TestIntegrationSchemaTablesExist(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}

	applyMigrations(t, runner, migrations, ctx)
	expectedTables := []string{
		"subscribers",
		"authentication_data",
		"authentication_status",
		"access_mobility_subscription",
		"session_management_subscription",
		"smf_selection_data",
		"sms_subscription_data",
		"sms_management_data",
		"amf_registrations",
		"smf_registrations",
		"smsf_registrations",
		"ee_subscriptions",
		"sdm_subscriptions",
		"pp_data",
		"pp_profile_data",
		"network_slice_data",
		"operator_specific_data",
		"shared_data",
		"ue_update_confirmation",
		"trace_data",
		"ip_sm_gw_registrations",
		"message_waiting_data",
		"audit_log",
	}

	for _, table := range expectedTables {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'udm' AND table_name = $1)",
			table,
		).Scan(&exists)
		if err != nil {
			t.Errorf("query table existence for %s: %v", table, err)
		} else if !exists {
			t.Errorf("table udm.%s does not exist after applying all migrations", table)
		}
	}
}

// TestIntegrationSubscriberCRUD verifies basic INSERT/SELECT/UPDATE/DELETE operations
// on the subscribers table after applying all migrations.
func TestIntegrationSubscriberCRUD(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}
	applyMigrations(t, runner, migrations, ctx)

	testSUPI := "imsi-001010000000001"

	// INSERT
	_, err = pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, $2)",
		testSUPI, "imsi",
	)
	if err != nil {
		t.Fatalf("INSERT subscriber: %v", err)
	}

	// SELECT
	var supi, supiType string
	err = pool.QueryRow(ctx,
		"SELECT supi, supi_type FROM udm.subscribers WHERE supi = $1", testSUPI,
	).Scan(&supi, &supiType)
	if err != nil {
		t.Fatalf("SELECT subscriber: %v", err)
	}
	if supi != testSUPI || supiType != "imsi" {
		t.Errorf("got supi=%q type=%q, want %q %q", supi, supiType, testSUPI, "imsi")
	}

	// UPDATE
	_, err = pool.Exec(ctx,
		"UPDATE udm.subscribers SET gpsi = $1, updated_at = NOW() WHERE supi = $2",
		"msisdn-14155550001", testSUPI,
	)
	if err != nil {
		t.Fatalf("UPDATE subscriber: %v", err)
	}

	var gpsi *string
	err = pool.QueryRow(ctx,
		"SELECT gpsi FROM udm.subscribers WHERE supi = $1", testSUPI,
	).Scan(&gpsi)
	if err != nil {
		t.Fatalf("SELECT updated subscriber: %v", err)
	}
	if gpsi == nil || *gpsi != "msisdn-14155550001" {
		t.Errorf("updated gpsi = %v, want msisdn-14155550001", gpsi)
	}

	// DELETE
	_, err = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", testSUPI)
	if err != nil {
		t.Fatalf("DELETE subscriber: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.subscribers WHERE supi = $1", testSUPI,
	).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("subscriber still exists after DELETE, count = %d", count)
	}
}

// TestIntegrationForeignKeyCascade verifies that deleting a subscriber cascades
// to dependent tables (authentication_data, amf_registrations, etc.).
func TestIntegrationForeignKeyCascade(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}
	applyMigrations(t, runner, migrations, ctx)

	testSUPI := "imsi-001010000000099"

	// Insert subscriber
	_, err = pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, $2)",
		testSUPI, "imsi",
	)
	if err != nil {
		t.Fatalf("INSERT subscriber: %v", err)
	}

	// Insert auth data referencing subscriber
	_, err = pool.Exec(ctx,
		"INSERT INTO udm.authentication_data (supi, auth_method) VALUES ($1, $2)",
		testSUPI, "5G_AKA",
	)
	if err != nil {
		t.Fatalf("INSERT authentication_data: %v", err)
	}

	// Delete subscriber — should cascade to auth data
	_, err = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", testSUPI)
	if err != nil {
		t.Fatalf("DELETE subscriber (cascade): %v", err)
	}

	// Verify auth data was cascade-deleted
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.authentication_data WHERE supi = $1", testSUPI,
	).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT auth count after cascade: %v", err)
	}
	if count != 0 {
		t.Errorf("authentication_data not cascade-deleted, count = %d", count)
	}
}

// TestIntegrationIndexesExist verifies that all expected indexes exist after
// applying all migrations.
func TestIntegrationIndexesExist(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}
	applyMigrations(t, runner, migrations, ctx)

	expectedIndexes := []string{
		"idx_subscribers_gpsi",
		"idx_ee_subs_group",
		"idx_ee_subs_gpsi",
		"idx_ee_subs_supi",
		"idx_sdm_subs_supi",
		"idx_amf_reg_instance",
		"idx_smf_reg_instance",
		"idx_smf_reg_dnn_nssai",
		"idx_opdata_supi",
		"idx_audit_supi_time",
		"idx_audit_time",
		"idx_am_data_covering",
		"idx_auth_covering",
		"idx_amf_reg_covering",
		"idx_sm_data_covering",
		"idx_am_nssai_gin",
		"idx_ee_monitoring_gin",
		"idx_sm_dnn_configs_gin",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'udm' AND indexname = $1)",
			idx,
		).Scan(&exists)
		if err != nil {
			t.Errorf("query index existence for %s: %v", idx, err)
		} else if !exists {
			t.Errorf("index %s does not exist after applying all migrations", idx)
		}
	}
}

// TestIntegrationMigrationVersionTracking verifies that the migration runner
// correctly records each applied migration version in the schema_migrations table.
func TestIntegrationMigrationVersionTracking(t *testing.T) {
	pool := testPool(t)
	cleanSchema(t, pool)
	t.Cleanup(func() { cleanSchema(t, pool) })

	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations error: %v", err)
	}

	runner := db.NewMigrationRunner(pool)
	ctx := context.Background()

	if err := runner.EnsureMigrationTable(ctx); err != nil {
		t.Fatalf("EnsureMigrationTable error: %v", err)
	}

	// Apply all migrations (skipping tablespace)
	applyMigrations(t, runner, migrations, ctx)

	// Verify version tracking count (26 total - 1 skipped = 25)
	applied, err := runner.GetAppliedVersions(ctx)
	if err != nil {
		t.Fatalf("GetAppliedVersions error: %v", err)
	}
	expectedApplied := len(migrations) - 1 // minus tablespace migration
	if len(applied) != expectedApplied {
		t.Errorf("applied count = %d, want %d", len(applied), expectedApplied)
	}

	// Verify versions are sequential (1..25, skipped 26)
	for i, v := range applied {
		wantVersion := i + 1
		if v != wantVersion {
			t.Errorf("applied[%d] = %d, want %d", i, v, wantVersion)
		}
	}
}
