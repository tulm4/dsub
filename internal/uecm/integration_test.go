//go:build integration

// Package uecm integration tests validate the UECM service against a real
// YugabyteDB instance. These tests require a running YugabyteDB and are excluded
// from fast unit test runs.
//
// Run with: go test ./internal/uecm/... -tags=integration -count=1 -timeout 5m
//
// Environment variables:
//
//	YUGABYTE_DSN — PostgreSQL-compatible connection string
//	  Default: postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
//
// Based on: docs/testing-strategy.md §7.1 (Integration Tests)
// 3GPP: TS 29.503 Nudm_UECM — UE Context Management service operations
package uecm

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

// Test constants for UECM integration tests.
const (
	integrationSUPI           = "imsi-001010000000200"
	integrationAmfInstanceID  = "amf-integration-001"
	integrationSmfInstanceID  = "smf-integration-001"
	integrationDeregCallback  = "https://amf.example.com/namf-callback/v1/dereg"
	integrationPduSessionID   = 5
	integrationDNN            = "internet"
	integrationRatType        = "NR"
	integrationRegistrationTS = "2024-01-15T12:00:00Z"
)

var (
	integrationGuami       = json.RawMessage(`{"plmnId":{"mcc":"001","mnc":"01"},"amfId":"cafe00"}`)
	integrationPlmnID      = json.RawMessage(`{"mcc":"001","mnc":"01"}`)
	integrationSingleNSSAI = json.RawMessage(`{"sst":1,"sd":"000001"}`)
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

// setupSchema applies all migrations and prepares the database for UECM tests.
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

// seedTestSubscriber inserts a test subscriber for UECM integration tests.
func seedTestSubscriber(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		"INSERT INTO udm.subscribers (supi, supi_type) VALUES ($1, $2)",
		integrationSUPI, "imsi",
	)
	if err != nil {
		t.Fatalf("INSERT subscriber: %v", err)
	}
}

// cleanTestData removes all test data inserted during UECM integration tests.
func cleanTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM udm.smf_registrations WHERE supi = $1", integrationSUPI)
	_, _ = pool.Exec(ctx, "DELETE FROM udm.smsf_registrations WHERE supi = $1", integrationSUPI)
	_, _ = pool.Exec(ctx, "DELETE FROM udm.amf_registrations WHERE supi = $1", integrationSUPI)
	_, _ = pool.Exec(ctx, "DELETE FROM udm.subscribers WHERE supi = $1", integrationSUPI)
}

// TestIntegrationRegister3GppAccess verifies that an AMF 3GPP access registration
// (UPSERT) creates a new row and returns created=true.
//
// 3GPP: TS 29.503 Nudm_UECM — 3GppRegistration
func TestIntegrationRegister3GppAccess(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:          integrationAmfInstanceID,
		DeregCallbackURI:       integrationDeregCallback,
		Guami:                  integrationGuami,
		RatType:                integrationRatType,
		InitialRegistrationInd: true,
		Pei:                    "imei-123456789012345",
		RegistrationTime:       integrationRegistrationTS,
	}

	result, created, err := svc.Register3GppAccess(ctx, integrationSUPI, reg)
	if err != nil {
		t.Fatalf("Register3GppAccess error: %v", err)
	}
	if !created {
		t.Error("expected created=true for new registration, got false")
	}
	if result.AmfInstanceID != integrationAmfInstanceID {
		t.Errorf("AmfInstanceID = %q, want %q", result.AmfInstanceID, integrationAmfInstanceID)
	}

	// Verify the row exists in the database
	var amfID string
	err = pool.QueryRow(ctx,
		"SELECT amf_instance_id FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2",
		integrationSUPI, accessType3GPP,
	).Scan(&amfID)
	if err != nil {
		t.Fatalf("SELECT amf_registrations: %v", err)
	}
	if amfID != integrationAmfInstanceID {
		t.Errorf("stored amf_instance_id = %q, want %q", amfID, integrationAmfInstanceID)
	}
}

// TestIntegrationRegister3GppAccess_Update verifies that re-registering an AMF
// for 3GPP access performs an UPSERT and returns created=false.
//
// 3GPP: TS 29.503 Nudm_UECM — 3GppRegistration (update case)
func TestIntegrationRegister3GppAccess_Update(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:          integrationAmfInstanceID,
		DeregCallbackURI:       integrationDeregCallback,
		Guami:                  integrationGuami,
		RatType:                integrationRatType,
		InitialRegistrationInd: true,
		RegistrationTime:       integrationRegistrationTS,
	}

	// First registration — should create
	_, created, err := svc.Register3GppAccess(ctx, integrationSUPI, reg)
	if err != nil {
		t.Fatalf("first Register3GppAccess error: %v", err)
	}
	if !created {
		t.Error("expected created=true for first registration")
	}

	// Second registration — should update
	updatedAMF := "amf-integration-002"
	reg.AmfInstanceID = updatedAMF
	result, created, err := svc.Register3GppAccess(ctx, integrationSUPI, reg)
	if err != nil {
		t.Fatalf("second Register3GppAccess error: %v", err)
	}
	if created {
		t.Error("expected created=false for update, got true")
	}
	if result.AmfInstanceID != updatedAMF {
		t.Errorf("AmfInstanceID = %q, want %q", result.AmfInstanceID, updatedAMF)
	}

	// Verify the updated AMF ID in the database
	var storedAMF string
	err = pool.QueryRow(ctx,
		"SELECT amf_instance_id FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2",
		integrationSUPI, accessType3GPP,
	).Scan(&storedAMF)
	if err != nil {
		t.Fatalf("SELECT amf_registrations: %v", err)
	}
	if storedAMF != updatedAMF {
		t.Errorf("stored amf_instance_id = %q, want %q", storedAMF, updatedAMF)
	}
}

// TestIntegrationGet3GppRegistration verifies retrieval of an AMF 3GPP access
// registration after it has been registered.
//
// 3GPP: TS 29.503 Nudm_UECM — Get3GppRegistration
func TestIntegrationGet3GppRegistration(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	// Register first
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:          integrationAmfInstanceID,
		DeregCallbackURI:       integrationDeregCallback,
		Guami:                  integrationGuami,
		RatType:                integrationRatType,
		InitialRegistrationInd: true,
		Pei:                    "imei-123456789012345",
		RegistrationTime:       integrationRegistrationTS,
	}
	_, _, err := svc.Register3GppAccess(ctx, integrationSUPI, reg)
	if err != nil {
		t.Fatalf("Register3GppAccess error: %v", err)
	}

	// Retrieve
	got, err := svc.Get3GppRegistration(ctx, integrationSUPI)
	if err != nil {
		t.Fatalf("Get3GppRegistration error: %v", err)
	}
	if got.AmfInstanceID != integrationAmfInstanceID {
		t.Errorf("AmfInstanceID = %q, want %q", got.AmfInstanceID, integrationAmfInstanceID)
	}
	if got.DeregCallbackURI != integrationDeregCallback {
		t.Errorf("DeregCallbackURI = %q, want %q", got.DeregCallbackURI, integrationDeregCallback)
	}
	if got.RatType != integrationRatType {
		t.Errorf("RatType = %q, want %q", got.RatType, integrationRatType)
	}
	if !got.InitialRegistrationInd {
		t.Error("InitialRegistrationInd = false, want true")
	}
	if got.Pei != "imei-123456789012345" {
		t.Errorf("Pei = %q, want %q", got.Pei, "imei-123456789012345")
	}

	// Verify GUAMI was stored as JSONB and retrieved correctly
	var guamiMap map[string]interface{}
	if err := json.Unmarshal(got.Guami, &guamiMap); err != nil {
		t.Errorf("Guami is not valid JSON: %v", err)
	}
}

// TestIntegrationDeregAMF verifies that an AMF 3GPP access deregistration
// removes the registration row.
//
// 3GPP: TS 29.503 Nudm_UECM — DeregAMF
// 3GPP: TS 23.502 §4.2.2.3.2 — UE-initiated de-registration
func TestIntegrationDeregAMF(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	// Register first
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:    integrationAmfInstanceID,
		DeregCallbackURI: integrationDeregCallback,
		Guami:            integrationGuami,
		RatType:          integrationRatType,
		RegistrationTime: integrationRegistrationTS,
	}
	_, _, err := svc.Register3GppAccess(ctx, integrationSUPI, reg)
	if err != nil {
		t.Fatalf("Register3GppAccess error: %v", err)
	}

	// Deregister
	deregData := &DeregistrationData{
		DeregReason: "UE_INITIAL_REGISTRATION",
	}
	err = svc.DeregAMF(ctx, integrationSUPI, deregData)
	if err != nil {
		t.Fatalf("DeregAMF error: %v", err)
	}

	// Verify the registration was removed
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.amf_registrations WHERE supi = $1 AND access_type = $2",
		integrationSUPI, accessType3GPP,
	).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT count after deregistration: %v", err)
	}
	if count != 0 {
		t.Errorf("amf_registrations still exists after DeregAMF, count = %d", count)
	}

	// Verify Get3GppRegistration returns an error after deregistration
	_, err = svc.Get3GppRegistration(ctx, integrationSUPI)
	if err == nil {
		t.Error("expected error for Get3GppRegistration after deregistration, got nil")
	}
}

// TestIntegrationRegisterSmf verifies that a SMF registration for a PDU session
// can be created.
//
// 3GPP: TS 29.503 Nudm_UECM — Registration (SMF)
func TestIntegrationRegisterSmf(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	reg := &SmfRegistration{
		SmfInstanceID:    integrationSmfInstanceID,
		Dnn:              integrationDNN,
		SingleNssai:      integrationSingleNSSAI,
		PlmnID:           integrationPlmnID,
		RegistrationTime: integrationRegistrationTS,
	}

	result, created, err := svc.RegisterSmf(ctx, integrationSUPI, integrationPduSessionID, reg)
	if err != nil {
		t.Fatalf("RegisterSmf error: %v", err)
	}
	if !created {
		t.Error("expected created=true for new SMF registration, got false")
	}
	if result.SmfInstanceID != integrationSmfInstanceID {
		t.Errorf("SmfInstanceID = %q, want %q", result.SmfInstanceID, integrationSmfInstanceID)
	}
	if result.PduSessionID != integrationPduSessionID {
		t.Errorf("PduSessionID = %d, want %d", result.PduSessionID, integrationPduSessionID)
	}

	// Verify the row exists in the database
	var smfID string
	var pduID int
	err = pool.QueryRow(ctx,
		"SELECT smf_instance_id, pdu_session_id FROM udm.smf_registrations WHERE supi = $1 AND pdu_session_id = $2",
		integrationSUPI, integrationPduSessionID,
	).Scan(&smfID, &pduID)
	if err != nil {
		t.Fatalf("SELECT smf_registrations: %v", err)
	}
	if smfID != integrationSmfInstanceID {
		t.Errorf("stored smf_instance_id = %q, want %q", smfID, integrationSmfInstanceID)
	}
	if pduID != integrationPduSessionID {
		t.Errorf("stored pdu_session_id = %d, want %d", pduID, integrationPduSessionID)
	}
}

// TestIntegrationDeregisterSmf verifies that a SMF registration can be removed
// for a specific PDU session.
//
// 3GPP: TS 29.503 Nudm_UECM — SmfDeregistration
func TestIntegrationDeregisterSmf(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	// Register first
	reg := &SmfRegistration{
		SmfInstanceID:    integrationSmfInstanceID,
		Dnn:              integrationDNN,
		SingleNssai:      integrationSingleNSSAI,
		PlmnID:           integrationPlmnID,
		RegistrationTime: integrationRegistrationTS,
	}
	_, _, err := svc.RegisterSmf(ctx, integrationSUPI, integrationPduSessionID, reg)
	if err != nil {
		t.Fatalf("RegisterSmf error: %v", err)
	}

	// Deregister
	err = svc.DeregisterSmf(ctx, integrationSUPI, integrationPduSessionID)
	if err != nil {
		t.Fatalf("DeregisterSmf error: %v", err)
	}

	// Verify the registration was removed
	var count int
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM udm.smf_registrations WHERE supi = $1 AND pdu_session_id = $2",
		integrationSUPI, integrationPduSessionID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("SELECT count after deregistration: %v", err)
	}
	if count != 0 {
		t.Errorf("smf_registrations still exists after DeregisterSmf, count = %d", count)
	}

	// Verify deregistering again returns not-found
	err = svc.DeregisterSmf(ctx, integrationSUPI, integrationPduSessionID)
	if err == nil {
		t.Error("expected error for DeregisterSmf of non-existent registration, got nil")
	}
}

// TestIntegrationGetRegistrations verifies the aggregated GetRegistrations
// response after registering an AMF and SMF.
//
// 3GPP: TS 29.503 Nudm_UECM — GetRegistrations
func TestIntegrationGetRegistrations(t *testing.T) {
	pool := integrationPool(t)
	cleanIntegrationSchema(t, pool)
	t.Cleanup(func() { cleanIntegrationSchema(t, pool) })

	setupSchema(t, pool)
	seedTestSubscriber(t, pool)
	t.Cleanup(func() { cleanTestData(t, pool) })

	svc := NewService(pool)
	ctx := context.Background()

	// Register AMF 3GPP access
	amfReg := &Amf3GppAccessRegistration{
		AmfInstanceID:          integrationAmfInstanceID,
		DeregCallbackURI:       integrationDeregCallback,
		Guami:                  integrationGuami,
		RatType:                integrationRatType,
		InitialRegistrationInd: true,
		RegistrationTime:       integrationRegistrationTS,
	}
	_, _, err := svc.Register3GppAccess(ctx, integrationSUPI, amfReg)
	if err != nil {
		t.Fatalf("Register3GppAccess error: %v", err)
	}

	// Register SMF
	smfReg := &SmfRegistration{
		SmfInstanceID:    integrationSmfInstanceID,
		Dnn:              integrationDNN,
		SingleNssai:      integrationSingleNSSAI,
		PlmnID:           integrationPlmnID,
		RegistrationTime: integrationRegistrationTS,
	}
	_, _, err = svc.RegisterSmf(ctx, integrationSUPI, integrationPduSessionID, smfReg)
	if err != nil {
		t.Fatalf("RegisterSmf error: %v", err)
	}

	// Get all registrations
	result, err := svc.GetRegistrations(ctx, integrationSUPI)
	if err != nil {
		t.Fatalf("GetRegistrations error: %v", err)
	}
	if result == nil {
		t.Fatal("GetRegistrations result is nil")
	}

	// Verify AMF 3GPP access registration
	if result.Amf3GppAccess == nil {
		t.Fatal("Amf3GppAccess is nil")
	}
	if result.Amf3GppAccess.AmfInstanceID != integrationAmfInstanceID {
		t.Errorf("Amf3GppAccess.AmfInstanceID = %q, want %q", result.Amf3GppAccess.AmfInstanceID, integrationAmfInstanceID)
	}

	// Verify no AMF non-3GPP access registration (not registered)
	if result.AmfNon3GppAccess != nil {
		t.Error("AmfNon3GppAccess should be nil (not registered)")
	}

	// Verify SMF registrations
	if len(result.SmfRegistrations) != 1 {
		t.Fatalf("SmfRegistrations count = %d, want 1", len(result.SmfRegistrations))
	}
	if result.SmfRegistrations[0].SmfInstanceID != integrationSmfInstanceID {
		t.Errorf("SmfRegistrations[0].SmfInstanceID = %q, want %q",
			result.SmfRegistrations[0].SmfInstanceID, integrationSmfInstanceID)
	}
	if result.SmfRegistrations[0].Dnn != integrationDNN {
		t.Errorf("SmfRegistrations[0].Dnn = %q, want %q",
			result.SmfRegistrations[0].Dnn, integrationDNN)
	}
	if result.SmfRegistrations[0].PduSessionID != integrationPduSessionID {
		t.Errorf("SmfRegistrations[0].PduSessionID = %d, want %d",
			result.SmfRegistrations[0].PduSessionID, integrationPduSessionID)
	}

	// Verify SMSF registrations are nil (not registered)
	if result.Smsf3GppAccess != nil {
		t.Error("Smsf3GppAccess should be nil (not registered)")
	}
	if result.SmsfNon3GppAccess != nil {
		t.Error("SmsfNon3GppAccess should be nil (not registered)")
	}
}
