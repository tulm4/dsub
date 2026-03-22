//go:build integration

// Package sdm integration tests validate the SDM service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/sdm/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
package sdm

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
func seedSubscriber(t *testing.T, pool *pgxpool.Pool, ctx context.Context, supi, gpsi string) {
	t.Helper()

	if gpsi == "" {
		_, err := pool.Exec(ctx,
			"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, 'imsi')",
			supi,
		)
		if err != nil {
			t.Fatalf("seed subscriber %s: %v", supi, err)
		}
	} else {
		_, err := pool.Exec(ctx,
			"INSERT INTO udm.subscribers (supi, supi_type, gpsi, gpsi_type) VALUES ($1, 'imsi', $2, 'msisdn')",
			supi, gpsi,
		)
		if err != nil {
			t.Fatalf("seed subscriber %s: %v", supi, err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", supi)
	})
}

// TestIntegrationGetAmData seeds Access and Mobility subscription data and
// verifies that the SDM service correctly retrieves it.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/am-data)
// 3GPP: TS 29.503 Nudm_SDM — GetAmData
func TestIntegrationGetAmData(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000101"
	seedSubscriber(t, pool, ctx, testSUPI, "")

	nssaiJSON := json.RawMessage(`{"defaultSingleNssais":[{"sst":1,"sd":"000001"}]}`)
	ambrJSON := json.RawMessage(`{"uplink":"1 Gbps","downlink":"2 Gbps"}`)

	_, err := pool.Exec(ctx,
		`INSERT INTO udm.access_mobility_subscription
			(supi, serving_plmn_id, nssai, subscribed_ue_ambr, rfsp_index,
			 mico_allowed, mps_priority, mcs_priority, routing_indicator,
			 gpsis, subscribed_dnn_list)
		 VALUES ($1, '00101', $2, $3, 10, true, true, false, '0123',
		         ARRAY['msisdn-14155550100'], ARRAY['internet','ims'])`,
		testSUPI, nssaiJSON, ambrJSON,
	)
	if err != nil {
		t.Fatalf("seed AM data: %v", err)
	}

	svc := NewService(pool)
	amData, err := svc.GetAmData(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetAmData: %v", err)
	}

	if amData.RfspIndex == nil || *amData.RfspIndex != 10 {
		t.Errorf("RfspIndex = %v, want 10", amData.RfspIndex)
	}
	if !amData.MicoAllowed {
		t.Error("MicoAllowed = false, want true")
	}
	if !amData.MpsPriority {
		t.Error("MpsPriority = false, want true")
	}
	if amData.McsPriority {
		t.Error("McsPriority = true, want false")
	}
	if amData.RoutingIndicator != "0123" {
		t.Errorf("RoutingIndicator = %q, want %q", amData.RoutingIndicator, "0123")
	}
	if len(amData.Gpsis) != 1 || amData.Gpsis[0] != "msisdn-14155550100" {
		t.Errorf("Gpsis = %v, want [msisdn-14155550100]", amData.Gpsis)
	}
	if len(amData.SubscribedDnnList) != 2 {
		t.Errorf("SubscribedDnnList length = %d, want 2", len(amData.SubscribedDnnList))
	}
	if len(amData.Nssai) == 0 {
		t.Error("Nssai is empty, want non-empty JSONB")
	}
	if len(amData.SubscribedUeAmbr) == 0 {
		t.Error("SubscribedUeAmbr is empty, want non-empty JSONB")
	}
}

// TestIntegrationGetSmData seeds Session Management subscription data and
// verifies that the SDM service correctly retrieves it.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/sm-data)
// 3GPP: TS 29.503 Nudm_SDM — GetSmData
func TestIntegrationGetSmData(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000102"
	seedSubscriber(t, pool, ctx, testSUPI, "")

	singleNssai1 := json.RawMessage(`{"sst":1,"sd":"000001"}`)
	dnnConfigs1 := json.RawMessage(`{"internet":{"pduSessionTypes":{"defaultSessionType":"IPV4"}}}`)
	singleNssai2 := json.RawMessage(`{"sst":1,"sd":"000002"}`)
	dnnConfigs2 := json.RawMessage(`{"ims":{"pduSessionTypes":{"defaultSessionType":"IPV4V6"}}}`)

	_, err := pool.Exec(ctx,
		`INSERT INTO udm.session_management_subscription
			(supi, serving_plmn_id, nssai_sst, nssai_sd, single_nssai, dnn_configurations)
		 VALUES ($1, '00101', 1, '000001', $2, $3)`,
		testSUPI, singleNssai1, dnnConfigs1,
	)
	if err != nil {
		t.Fatalf("seed SM data row 1: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO udm.session_management_subscription
			(supi, serving_plmn_id, nssai_sst, nssai_sd, single_nssai, dnn_configurations)
		 VALUES ($1, '00101', 1, '000002', $2, $3)`,
		testSUPI, singleNssai2, dnnConfigs2,
	)
	if err != nil {
		t.Fatalf("seed SM data row 2: %v", err)
	}

	svc := NewService(pool)
	smData, err := svc.GetSmData(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetSmData: %v", err)
	}

	if len(smData) != 2 {
		t.Fatalf("GetSmData returned %d rows, want 2", len(smData))
	}

	for i, d := range smData {
		if len(d.SingleNssai) == 0 {
			t.Errorf("row %d: SingleNssai is empty", i)
		}
		if len(d.DnnConfigurations) == 0 {
			t.Errorf("row %d: DnnConfigurations is empty", i)
		}
	}
}

// TestIntegrationGetNSSAI seeds NSSAI data in the access_mobility_subscription
// table and verifies that the SDM service correctly retrieves and unmarshals it.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{supi}/nssai)
// 3GPP: TS 29.503 Nudm_SDM — GetNSSAI
func TestIntegrationGetNSSAI(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000103"
	seedSubscriber(t, pool, ctx, testSUPI, "")

	nssaiJSON := json.RawMessage(`{
		"defaultSingleNssais": [{"sst":1,"sd":"000001"}],
		"singleNssais": [{"sst":1,"sd":"000001"},{"sst":2,"sd":"000002"}]
	}`)

	_, err := pool.Exec(ctx,
		`INSERT INTO udm.access_mobility_subscription (supi, serving_plmn_id, nssai)
		 VALUES ($1, '00101', $2)`,
		testSUPI, nssaiJSON,
	)
	if err != nil {
		t.Fatalf("seed NSSAI data: %v", err)
	}

	svc := NewService(pool)
	nssai, err := svc.GetNSSAI(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetNSSAI: %v", err)
	}

	if len(nssai.DefaultSingleNssais) == 0 {
		t.Error("DefaultSingleNssais is empty, want non-empty")
	}
	if len(nssai.SingleNssais) == 0 {
		t.Error("SingleNssais is empty, want non-empty")
	}
}

// TestIntegrationGetIdTranslation seeds a subscriber with a GPSI and verifies
// that the SDM service can translate SUPI→GPSI and GPSI→SUPI.
//
// Based on: docs/sbi-api-design.md §3.2 (GET /{ueId}/id-translation-result)
// 3GPP: TS 29.503 Nudm_SDM — GetSupiOrGpsi
func TestIntegrationGetIdTranslation(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000104"
	testGPSI := "msisdn-14155550104"
	seedSubscriber(t, pool, ctx, testSUPI, testGPSI)

	svc := NewService(pool)

	// SUPI → GPSI lookup
	result, err := svc.GetIdTranslation(ctx, testSUPI)
	if err != nil {
		t.Fatalf("GetIdTranslation(SUPI): %v", err)
	}
	if result.Supi != testSUPI {
		t.Errorf("SUPI lookup: Supi = %q, want %q", result.Supi, testSUPI)
	}
	if result.Gpsi != testGPSI {
		t.Errorf("SUPI lookup: Gpsi = %q, want %q", result.Gpsi, testGPSI)
	}

	// GPSI → SUPI lookup
	result, err = svc.GetIdTranslation(ctx, testGPSI)
	if err != nil {
		t.Fatalf("GetIdTranslation(GPSI): %v", err)
	}
	if result.Supi != testSUPI {
		t.Errorf("GPSI lookup: Supi = %q, want %q", result.Supi, testSUPI)
	}
	if result.Gpsi != testGPSI {
		t.Errorf("GPSI lookup: Gpsi = %q, want %q", result.Gpsi, testGPSI)
	}
}

// TestIntegrationSubscribeCRUD tests the full lifecycle of an SDM subscription:
// Subscribe → ModifySubscription → Unsubscribe.
//
// Based on: docs/sbi-api-design.md §3.2 (SDM subscription endpoints)
// 3GPP: TS 29.503 Nudm_SDM — Subscribe, Modify, Unsubscribe
func TestIntegrationSubscribeCRUD(t *testing.T) {
	pool, ctx := setupSchema(t)

	testSUPI := "imsi-001010000000105"
	seedSubscriber(t, pool, ctx, testSUPI, "")

	svc := NewService(pool)

	// Subscribe — create a new subscription
	sub := &SdmSubscription{
		NfInstanceID:          "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		CallbackReference:     "https://amf.example.com/nudm-sdm-notify/v2/callback",
		MonitoredResourceUris: []string{"/nudm-sdm/v2/" + testSUPI + "/am-data"},
	}

	created, err := svc.Subscribe(ctx, testSUPI, sub)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if created.SubscriptionID == "" {
		t.Fatal("Subscribe returned empty SubscriptionID")
	}
	subID := created.SubscriptionID

	t.Cleanup(func() {
		// Best-effort cleanup in case Unsubscribe test fails.
		_, _ = pool.Exec(ctx,
			"DELETE FROM udm.sdm_subscriptions WHERE subscription_id = $1", subID,
		)
	})

	// ModifySubscription — update callback reference
	patch := &SdmSubscription{
		CallbackReference: "https://amf.example.com/nudm-sdm-notify/v2/updated-callback",
	}

	modified, err := svc.ModifySubscription(ctx, testSUPI, subID, patch)
	if err != nil {
		t.Fatalf("ModifySubscription: %v", err)
	}
	if modified.SubscriptionID != subID {
		t.Errorf("ModifySubscription returned ID = %q, want %q", modified.SubscriptionID, subID)
	}

	// Verify the callback was updated in the database
	var callbackRef string
	err = pool.QueryRow(ctx,
		"SELECT callback_reference FROM udm.sdm_subscriptions WHERE subscription_id = $1",
		subID,
	).Scan(&callbackRef)
	if err != nil {
		t.Fatalf("verify modified callback: %v", err)
	}
	if callbackRef != "https://amf.example.com/nudm-sdm-notify/v2/updated-callback" {
		t.Errorf("callback_reference = %q, want updated URL", callbackRef)
	}

	// Unsubscribe — delete the subscription
	if err := svc.Unsubscribe(ctx, testSUPI, subID); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	// Verify subscription was deleted
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.sdm_subscriptions WHERE subscription_id = $1",
		subID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count after unsubscribe: %v", err)
	}
	if count != 0 {
		t.Errorf("subscription still exists after Unsubscribe, count = %d", count)
	}
}
