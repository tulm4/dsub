//go:build integration

// Package pp integration tests validate the PP service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/pp/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
package pp

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

// TestIntegrationGetPPData_NotFound queries PP data for an unknown SUPI and
// verifies a 404 error is returned.
//
// Based on: docs/sbi-api-design.md §3.5 (GET /{ueId}/pp-data)
// 3GPP: TS 29.503 Nudm_PP — GetPPData
func TestIntegrationGetPPData_NotFound(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000301"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	_, err := svc.GetPPData(ctx, testSUPI)
	if err == nil {
		t.Fatal("expected error for no PP data, got nil")
	}

	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *errors.ProblemDetails, got %T", err)
	}
	if pd.Status != 404 {
		t.Errorf("ProblemDetails.Status = %d, want 404", pd.Status)
	}
}

// TestIntegrationUpdatePPData upserts PP data and verifies the returned values.
//
// Based on: docs/sbi-api-design.md §3.5 (PATCH /{ueId}/pp-data)
// 3GPP: TS 29.503 Nudm_PP — UpdatePPData
func TestIntegrationUpdatePPData(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000302"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	dlCount := 10
	patch := &PpData{
		CommunicationCharacteristics: json.RawMessage(`{"nonIpInd":true}`),
		SupportedFeatures:            "A1",
		PpDlPacketCount:              &dlCount,
	}

	result, err := svc.UpdatePPData(ctx, testSUPI, patch)
	if err != nil {
		t.Fatalf("UpdatePPData: %v", err)
	}
	if result.SupportedFeatures != "A1" {
		t.Errorf("SupportedFeatures = %q, want %q", result.SupportedFeatures, "A1")
	}
	if result.PpDlPacketCount == nil || *result.PpDlPacketCount != 10 {
		t.Errorf("PpDlPacketCount = %v, want 10", result.PpDlPacketCount)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM udm.pp_data WHERE supi = $1", testSUPI)
	})
}

// TestIntegrationUpdatePPData_Roundtrip updates then gets PP data to verify
// a full roundtrip.
//
// Based on: docs/sbi-api-design.md §3.5
// 3GPP: TS 29.503 Nudm_PP — GetPPData / UpdatePPData
func TestIntegrationUpdatePPData_Roundtrip(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000303"
	seedSubscriber(t, pool, ctx, testSUPI)

	svc := NewService(pool)

	maxResp := 30
	patch := &PpData{
		SupportedFeatures:     "B2",
		PpMaximumResponseTime: &maxResp,
	}

	_, err := svc.UpdatePPData(ctx, testSUPI, patch)
	if err != nil {
		t.Fatalf("UpdatePPData: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM udm.pp_data WHERE supi = $1", testSUPI)
	})

	got, err := svc.GetPPData(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetPPData: %v", err)
	}
	if got.SupportedFeatures != "B2" {
		t.Errorf("SupportedFeatures = %q, want %q", got.SupportedFeatures, "B2")
	}
	if got.PpMaximumResponseTime == nil || *got.PpMaximumResponseTime != 30 {
		t.Errorf("PpMaximumResponseTime = %v, want 30", got.PpMaximumResponseTime)
	}
}

// TestIntegrationCreate5GVnGroup creates a VN group and verifies creation.
//
// Based on: docs/sbi-api-design.md §3.5 (PUT /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Create5GVnGroup
func TestIntegrationCreate5GVnGroup(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	extGroupID := "group-inttest-001"
	cfg := &VnGroupConfiguration{
		Dnn:     "internet",
		Members: []string{"imsi-001010000000001"},
	}

	result, created, err := svc.Create5GVnGroup(ctx, extGroupID, cfg)
	if err != nil {
		t.Fatalf("Create5GVnGroup: %v", err)
	}
	if !created {
		t.Error("expected created=true for new VN group")
	}
	if result.Dnn != "internet" {
		t.Errorf("Dnn = %q, want %q", result.Dnn, "internet")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.vn_groups WHERE ext_group_id = $1", extGroupID,
		)
	})
}

// TestIntegrationGet5GVnGroup creates then gets a VN group.
//
// Based on: docs/sbi-api-design.md §3.5 (GET /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Get5GVnGroup
func TestIntegrationGet5GVnGroup(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	extGroupID := "group-inttest-002"
	cfg := &VnGroupConfiguration{
		Dnn:       "ims",
		DnAaaFqdn: "aaa.example.com",
		Members:   []string{"imsi-001010000000010", "imsi-001010000000011"},
	}

	_, _, err := svc.Create5GVnGroup(ctx, extGroupID, cfg)
	if err != nil {
		t.Fatalf("Create5GVnGroup: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.vn_groups WHERE ext_group_id = $1", extGroupID,
		)
	})

	got, err := svc.Get5GVnGroup(ctx, extGroupID)
	if err != nil {
		t.Fatalf("Get5GVnGroup: %v", err)
	}
	if got.Dnn != "ims" {
		t.Errorf("Dnn = %q, want %q", got.Dnn, "ims")
	}
	if got.DnAaaFqdn != "aaa.example.com" {
		t.Errorf("DnAaaFqdn = %q, want %q", got.DnAaaFqdn, "aaa.example.com")
	}
	if len(got.Members) != 2 {
		t.Errorf("Members length = %d, want 2", len(got.Members))
	}
}

// TestIntegrationDelete5GVnGroup creates then deletes a VN group.
//
// Based on: docs/sbi-api-design.md §3.5 (DELETE /5g-vn-groups/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — Delete5GVnGroup
func TestIntegrationDelete5GVnGroup(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	extGroupID := "group-inttest-003"
	cfg := &VnGroupConfiguration{
		Dnn: "internet",
	}

	_, _, err := svc.Create5GVnGroup(ctx, extGroupID, cfg)
	if err != nil {
		t.Fatalf("Create5GVnGroup: %v", err)
	}

	if err := svc.Delete5GVnGroup(ctx, extGroupID); err != nil {
		t.Fatalf("Delete5GVnGroup: %v", err)
	}

	// Verify deletion
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.vn_groups WHERE ext_group_id = $1",
		extGroupID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("VN group still exists after delete, count = %d", count)
	}
}

// TestIntegrationCreateMbsGroupMembership creates an MBS group membership
// and verifies creation.
//
// Based on: docs/sbi-api-design.md §3.5 (PUT /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — CreateMbsGroupMembership
func TestIntegrationCreateMbsGroupMembership(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	extGroupID := "mbs-inttest-001"
	memb := &MbsGroupMemb{
		MulticastGroupMemb: json.RawMessage(`{"mbsSessionId":"session-001"}`),
		AfInstanceId:       "af-001",
	}

	result, created, err := svc.CreateMbsGroupMembership(ctx, extGroupID, memb)
	if err != nil {
		t.Fatalf("CreateMbsGroupMembership: %v", err)
	}
	if !created {
		t.Error("expected created=true for new MBS membership")
	}
	if result.AfInstanceId != "af-001" {
		t.Errorf("AfInstanceId = %q, want %q", result.AfInstanceId, "af-001")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.mbs_group_membership WHERE ext_group_id = $1", extGroupID,
		)
	})
}

// TestIntegrationDeleteMbsGroupMembership creates then deletes an MBS
// group membership.
//
// Based on: docs/sbi-api-design.md §3.5 (DELETE /mbs-group-membership/{extGroupId})
// 3GPP: TS 29.503 Nudm_PP — DeleteMbsGroupMembership
func TestIntegrationDeleteMbsGroupMembership(t *testing.T) {
	pool, ctx := setupSchema(t)

	svc := NewService(pool)

	extGroupID := "mbs-inttest-002"
	memb := &MbsGroupMemb{
		MulticastGroupMemb: json.RawMessage(`{"mbsSessionId":"session-002"}`),
	}

	_, _, err := svc.CreateMbsGroupMembership(ctx, extGroupID, memb)
	if err != nil {
		t.Fatalf("CreateMbsGroupMembership: %v", err)
	}

	if err := svc.DeleteMbsGroupMembership(ctx, extGroupID); err != nil {
		t.Fatalf("DeleteMbsGroupMembership: %v", err)
	}

	// Verify deletion
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.mbs_group_membership WHERE ext_group_id = $1",
		extGroupID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("MBS membership still exists after delete, count = %d", count)
	}
}
