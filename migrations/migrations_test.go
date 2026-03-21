package migrations

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tulm4/dsub/internal/db"
)

// TestEmbeddedMigrationsParseable verifies that all embedded .up.sql migration files
// are correctly parsed by the migration runner's ParseMigrations function.
func TestEmbeddedMigrationsParseable(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	// Phase 2 delivers 25 migration files (000001–000025)
	const expectedCount = 25
	if len(migrations) != expectedCount {
		t.Fatalf("expected %d migrations, got %d", expectedCount, len(migrations))
	}

	// Verify sequential ordering (versions 1..25)
	for i, m := range migrations {
		wantVersion := i + 1
		if m.Version != wantVersion {
			t.Errorf("migration[%d].Version = %d, want %d", i, m.Version, wantVersion)
		}
	}
}

// TestMigrationDescriptions verifies each migration has a non-empty description.
func TestMigrationDescriptions(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	for _, m := range migrations {
		if m.Description == "" {
			t.Errorf("migration %d has empty description", m.Version)
		}
	}
}

// TestMigrationSQLNotEmpty verifies each migration has non-empty SQL content.
func TestMigrationSQLNotEmpty(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	for _, m := range migrations {
		trimmed := strings.TrimSpace(m.SQL)
		if trimmed == "" {
			t.Errorf("migration %d (%s) has empty SQL", m.Version, m.Description)
		}
	}
}

// TestMigrationExpectedNames verifies that each migration has the expected description.
// Based on: docs/data-model.md §3.1–§3.23, §4 (Indexing Strategy)
func TestMigrationExpectedNames(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	expected := []struct {
		version     int
		description string
	}{
		{1, "create_schema"},
		{2, "create_subscribers"},
		{3, "create_authentication_data"},
		{4, "create_authentication_status"},
		{5, "create_access_mobility_subscription"},
		{6, "create_session_management_subscription"},
		{7, "create_smf_selection_data"},
		{8, "create_sms_subscription_data"},
		{9, "create_sms_management_data"},
		{10, "create_amf_registrations"},
		{11, "create_smf_registrations"},
		{12, "create_smsf_registrations"},
		{13, "create_ee_subscriptions"},
		{14, "create_sdm_subscriptions"},
		{15, "create_pp_data"},
		{16, "create_pp_profile_data"},
		{17, "create_network_slice_data"},
		{18, "create_operator_specific_data"},
		{19, "create_shared_data"},
		{20, "create_ue_update_confirmation"},
		{21, "create_trace_data"},
		{22, "create_ip_sm_gw_registrations"},
		{23, "create_message_waiting_data"},
		{24, "create_audit_log"},
		{25, "create_indexes"},
	}

	if len(migrations) != len(expected) {
		t.Fatalf("migration count = %d, want %d", len(migrations), len(expected))
	}

	for i, want := range expected {
		got := migrations[i]
		if got.Version != want.version {
			t.Errorf("migration[%d].Version = %d, want %d", i, got.Version, want.version)
		}
		if got.Description != want.description {
			t.Errorf("migration[%d].Description = %q, want %q", i, got.Description, want.description)
		}
	}
}

// TestSchemaSetupMigration validates the schema setup migration contains required SQL.
func TestSchemaSetupMigration(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	schema := migrations[0]
	if schema.Version != 1 {
		t.Fatalf("first migration version = %d, want 1", schema.Version)
	}

	requiredStatements := []string{
		"CREATE SCHEMA IF NOT EXISTS udm",
		"CREATE EXTENSION IF NOT EXISTS",
	}

	for _, stmt := range requiredStatements {
		if !strings.Contains(schema.SQL, stmt) {
			t.Errorf("schema migration missing required SQL: %q", stmt)
		}
	}
}

// TestTableMigrationsContainCreateTable validates that table migration files
// contain CREATE TABLE statements referencing the udm schema.
func TestTableMigrationsContainCreateTable(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	// Migrations 2–24 are table creation migrations
	for _, m := range migrations[1:24] {
		if !strings.Contains(m.SQL, "CREATE TABLE udm.") {
			t.Errorf("migration %d (%s) missing 'CREATE TABLE udm.' statement",
				m.Version, m.Description)
		}
	}
}

// TestIndexMigrationContainsAllIndexTypes validates the index migration
// contains secondary, covering, and GIN index types.
func TestIndexMigrationContainsAllIndexTypes(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	indexMigration := migrations[24]
	if indexMigration.Version != 25 {
		t.Fatalf("index migration version = %d, want 25", indexMigration.Version)
	}

	// Verify secondary indexes
	secondaryIndexes := []string{
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
	}
	for _, idx := range secondaryIndexes {
		if !strings.Contains(indexMigration.SQL, idx) {
			t.Errorf("index migration missing secondary index: %s", idx)
		}
	}

	// Verify covering indexes (INCLUDE keyword)
	coveringIndexes := []string{
		"idx_am_data_covering",
		"idx_auth_covering",
		"idx_amf_reg_covering",
		"idx_sm_data_covering",
	}
	for _, idx := range coveringIndexes {
		if !strings.Contains(indexMigration.SQL, idx) {
			t.Errorf("index migration missing covering index: %s", idx)
		}
	}
	if !strings.Contains(indexMigration.SQL, "INCLUDE") {
		t.Error("index migration missing INCLUDE keyword for covering indexes")
	}

	// Verify GIN indexes
	ginIndexes := []string{
		"idx_am_nssai_gin",
		"idx_ee_monitoring_gin",
		"idx_sm_dnn_configs_gin",
	}
	for _, idx := range ginIndexes {
		if !strings.Contains(indexMigration.SQL, idx) {
			t.Errorf("index migration missing GIN index: %s", idx)
		}
	}
	if !strings.Contains(indexMigration.SQL, "USING GIN") {
		t.Error("index migration missing 'USING GIN' keyword")
	}
}

// TestSubscribersTableSchema validates critical columns in the subscribers table migration.
func TestSubscribersTableSchema(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	sub := migrations[1] // 000002_create_subscribers
	requiredColumns := []string{
		"supi", "gpsi", "supi_type", "gpsi_type",
		"group_ids", "identity_data", "odb_data",
		"roaming_allowed", "provisioning_status",
		"shared_data_ids", "version", "created_at", "updated_at",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(sub.SQL, col) {
			t.Errorf("subscribers migration missing column: %s", col)
		}
	}

	// Verify SUPI is primary key
	if !strings.Contains(sub.SQL, "PRIMARY KEY (supi)") {
		t.Error("subscribers migration missing PRIMARY KEY (supi)")
	}

	// Verify SPLIT INTO for sharding
	if !strings.Contains(sub.SQL, "SPLIT INTO 128 TABLETS") {
		t.Error("subscribers migration missing SPLIT INTO 128 TABLETS")
	}
}

// TestAuthenticationDataTableSchema validates critical columns in the auth data migration.
func TestAuthenticationDataTableSchema(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	auth := migrations[2] // 000003_create_authentication_data
	requiredColumns := []string{
		"supi", "auth_method", "k_key", "opc_key", "topc_key",
		"sqn", "sqn_scheme", "sqn_last_indexes", "sqn_ind_length",
		"amf_value", "algorithm_id", "version",
	}

	for _, col := range requiredColumns {
		if !strings.Contains(auth.SQL, col) {
			t.Errorf("authentication_data migration missing column: %s", col)
		}
	}

	// Verify FK constraint
	if !strings.Contains(auth.SQL, "REFERENCES udm.subscribers(supi) ON DELETE CASCADE") {
		t.Error("authentication_data migration missing FK to subscribers")
	}
}

// TestAllTablesMigrationHaveVersionAndTimestamps verifies that mutable data table
// migrations include version, created_at, and updated_at column definitions for
// optimistic locking.
// Based on: docs/data-model.md §9.2 (Data Versioning)
func TestAllTablesMigrationHaveVersionAndTimestamps(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	// Match actual column definitions (indented column name followed by type),
	// not just any occurrence of the word in comments or elsewhere.
	colDefPatterns := map[string]*regexp.Regexp{
		"version":    regexp.MustCompile(`(?m)^\s+version\s+BIGINT\b`),
		"created_at": regexp.MustCompile(`(?m)^\s+created_at\s+TIMESTAMPTZ\b`),
		"updated_at": regexp.MustCompile(`(?m)^\s+updated_at\s+TIMESTAMPTZ\b`),
	}

	// Exceptions: tables that intentionally omit version/updated_at
	// - authentication_status (4): event log with time_stamp, not mutable
	// - audit_log (24): append-only log, no version/updated_at
	skipVersionCheck := map[int]bool{4: true, 24: true}

	for _, m := range migrations[1:24] {
		if skipVersionCheck[m.Version] {
			continue
		}
		for colName, pattern := range colDefPatterns {
			if !pattern.MatchString(m.SQL) {
				t.Errorf("migration %d (%s) missing column definition for %q",
					m.Version, m.Description, colName)
			}
		}
	}
}

// TestForeignKeyConstraints validates that all subscriber-dependent tables
// have FK constraints referencing udm.subscribers(supi).
func TestForeignKeyConstraints(t *testing.T) {
	migrations, err := db.ParseMigrations(FS)
	if err != nil {
		t.Fatalf("ParseMigrations(FS) error = %v", err)
	}

	// Tables that reference subscribers via FK
	// Exceptions:
	// - subscribers (2): root table, no self-referencing FK
	// - shared_data (19): standalone reference table, no FK to subscribers
	// - audit_log (24): has supi column but no FK constraint per design
	fkRequired := map[int]bool{
		3: true, 4: true, 5: true, 6: true,
		7: true, 8: true, 9: true, 10: true, 11: true,
		12: true, 13: true, 14: true, 15: true, 16: true,
		17: true, 18: true, 20: true, 21: true, 22: true, 23: true,
	}

	for _, m := range migrations[1:24] {
		expectFK := fkRequired[m.Version]
		hasFK := strings.Contains(m.SQL, "REFERENCES udm.subscribers(supi)")

		if expectFK && !hasFK {
			t.Errorf("migration %d (%s) missing FK to udm.subscribers(supi)",
				m.Version, m.Description)
		}
		if !expectFK && hasFK {
			t.Errorf("migration %d (%s) has unexpected FK to udm.subscribers(supi)",
				m.Version, m.Description)
		}
	}
}
