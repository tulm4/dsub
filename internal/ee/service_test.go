package ee

// Service layer tests for the Nudm_EE service.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// Based on: docs/testing-strategy.md (unit testing patterns)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
)

// testSUPI is a valid SUPI for use across all service tests.
const testSUPI = "imsi-001010000000001"

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

// mockRow implements pgx.Row for unit tests.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

// mockDB implements the DB interface for unit tests.
type mockDB struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return nil, pgx.ErrNoRows
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag(""), nil
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// scanString safely assigns a string to a scan destination.
func scanString(dest any, val string) {
	if p, ok := dest.(*string); ok {
		*p = val
	}
}

// scanJSON safely assigns a json.RawMessage to a scan destination.
func scanJSON(dest any, val json.RawMessage) {
	if p, ok := dest.(*json.RawMessage); ok {
		*p = val
	}
}

// assertProblemStatus asserts that err is a *ProblemDetails with the given HTTP status.
func assertProblemStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != wantStatus {
		t.Errorf("expected status %d, got %d", wantStatus, pd.Status)
	}
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestServiceImplementsInterface(t *testing.T) {
	var _ ServiceInterface = (*Service)(nil)
}

// ---------------------------------------------------------------------------
// validateUeID tests
// ---------------------------------------------------------------------------

func TestValidateUeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "valid GPSI msisdn", input: "msisdn-12025551234", wantErr: false},
		{name: "valid GPSI extid", input: "extid-user@example.com", wantErr: false},
		{name: "valid group ID", input: "group-001", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "invalid SUPI digits", input: "imsi-12345", wantErr: true},
		{name: "empty group", input: "group-", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUeID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateUeID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// identityColumns tests
// ---------------------------------------------------------------------------

func TestIdentityColumns(t *testing.T) {
	tests := []struct {
		input   string
		wantCol string
		wantVal string
	}{
		{"imsi-001010000000001", "supi", "imsi-001010000000001"},
		{"msisdn-12025551234", "gpsi", "msisdn-12025551234"},
		{"group-001", "ue_group_id", "group-001"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			col, val := identityColumns(tc.input)
			if col != tc.wantCol {
				t.Errorf("identityColumns(%q) col: got %q, want %q", tc.input, col, tc.wantCol)
			}
			if val != tc.wantVal {
				t.Errorf("identityColumns(%q) val: got %q, want %q", tc.input, val, tc.wantVal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateSubscription tests
// ---------------------------------------------------------------------------

func TestCreateSubscription_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "generated-uuid-001")
				return nil
			}}
		},
	}
	svc := NewService(db)
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`),
	}

	result, err := svc.CreateSubscription(context.Background(), testSUPI, sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscriptionID != "generated-uuid-001" {
		t.Errorf("expected subscriptionId=generated-uuid-001, got %s", result.SubscriptionID)
	}
	if result.EeSubscription != sub {
		t.Error("expected returned subscription to be the same object")
	}
}

func TestCreateSubscription_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{}}`),
	}

	_, err := svc.CreateSubscription(context.Background(), "bad-id", sub)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateSubscription_NilSubscription(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.CreateSubscription(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateSubscription_MissingCallbackReference(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &EeSubscription{
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{}}`),
	}

	_, err := svc.CreateSubscription(context.Background(), testSUPI, sub)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateSubscription_MissingMonitoringConfigurations(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &EeSubscription{
		CallbackReference: "https://nef.example.com/callback",
	}

	_, err := svc.CreateSubscription(context.Background(), testSUPI, sub)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateSubscription_NullMonitoringConfigurations(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`null`),
	}

	_, err := svc.CreateSubscription(context.Background(), testSUPI, sub)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateSubscription_ScanError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("db failure")
			}}
		},
	}
	svc := NewService(db)
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{}}`),
	}

	_, err := svc.CreateSubscription(context.Background(), testSUPI, sub)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

func TestCreateSubscription_GroupID(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "generated-uuid-grp")
				return nil
			}}
		},
	}
	svc := NewService(db)
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{}}`),
	}

	result, err := svc.CreateSubscription(context.Background(), "group-001", sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscriptionID != "generated-uuid-grp" {
		t.Errorf("expected subscriptionId=generated-uuid-grp, got %s", result.SubscriptionID)
	}
}

func TestCreateSubscription_GPSI(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "generated-uuid-gpsi")
				return nil
			}}
		},
	}
	svc := NewService(db)
	sub := &EeSubscription{
		CallbackReference:        "https://nef.example.com/callback",
		MonitoringConfigurations: json.RawMessage(`{"cfg1":{}}`),
	}

	result, err := svc.CreateSubscription(context.Background(), "msisdn-12025551234", sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscriptionID != "generated-uuid-gpsi" {
		t.Errorf("expected subscriptionId=generated-uuid-gpsi, got %s", result.SubscriptionID)
	}
}

// ---------------------------------------------------------------------------
// UpdateSubscription tests
// ---------------------------------------------------------------------------

func TestUpdateSubscription_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "https://nef.example.com/callback-updated")
				scanJSON(dest[1], json.RawMessage(`{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}`))
				scanJSON(dest[2], json.RawMessage(`null`))
				scanString(dest[3], "featureA")
				scanString(dest[4], "scef-001")
				scanString(dest[5], "nf-001")
				scanString(dest[6], "2025-01-01T00:00:00Z")
				return nil
			}}
		},
	}
	svc := NewService(db)
	cbRef := "https://nef.example.com/callback-updated"
	patch := &PatchEeSubscription{CallbackReference: &cbRef}

	result, err := svc.UpdateSubscription(context.Background(), testSUPI, "sub-001", patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CallbackReference != "https://nef.example.com/callback-updated" {
		t.Errorf("expected updated callbackReference, got %s", result.CallbackReference)
	}
	if result.ScefID != "scef-001" {
		t.Errorf("expected scefId=scef-001, got %s", result.ScefID)
	}
}

func TestUpdateSubscription_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	patch := &PatchEeSubscription{}

	_, err := svc.UpdateSubscription(context.Background(), "bad-id", "sub-001", patch)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdateSubscription_NilPatch(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.UpdateSubscription(context.Background(), testSUPI, "sub-001", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdateSubscription_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)
	patch := &PatchEeSubscription{}

	_, err := svc.UpdateSubscription(context.Background(), testSUPI, "nonexistent", patch)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeleteSubscription tests
// ---------------------------------------------------------------------------

func TestDeleteSubscription_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeleteSubscription(context.Background(), testSUPI, "sub-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSubscription_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.DeleteSubscription(context.Background(), "bad-id", "sub-001")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestDeleteSubscription_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeleteSubscription(context.Background(), testSUPI, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestDeleteSubscription_ExecError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), fmt.Errorf("connection lost")
		},
	}
	svc := NewService(db)

	err := svc.DeleteSubscription(context.Background(), testSUPI, "sub-001")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

func TestDeleteSubscription_GroupID(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeleteSubscription(context.Background(), "group-001", "sub-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
