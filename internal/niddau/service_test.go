package niddau

// Service layer tests for the Nudm_NIDDAU service.
//
// Based on: docs/service-decomposition.md §2.8 (udm-niddau)
// Based on: docs/testing-strategy.md (unit testing patterns)

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
)

// testGPSI is a valid GPSI for use across all service tests.
const testGPSI = "msisdn-12025551234"

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
// ValidateUeID tests
// ---------------------------------------------------------------------------

func TestValidateUeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid GPSI msisdn", input: "msisdn-12025551234", wantErr: false},
		{name: "valid GPSI extid", input: "extid-user@example.com", wantErr: false},
		{name: "valid group", input: "group-group1", wantErr: false},
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
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
// AuthorizeNiddData tests
// ---------------------------------------------------------------------------

func TestAuthorizeNiddData_SuccessBySUPI(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "imsi-001010000000001")
				scanString(dest[1], "msisdn-12025551234")
				return nil
			}}
		},
	}
	svc := NewService(db)

	req := &AuthorizationInfo{
		Dnn:          "iot",
		ValidityTime: "2026-12-31T23:59:59Z",
	}
	result, err := svc.AuthorizeNiddData(context.Background(), testSUPI, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AuthorizationData) != 1 {
		t.Fatalf("expected 1 authorization data entry, got %d", len(result.AuthorizationData))
	}
	if result.AuthorizationData[0].Supi != testSUPI {
		t.Errorf("expected supi %s, got %s", testSUPI, result.AuthorizationData[0].Supi)
	}
	if result.ValidityTime != "2026-12-31T23:59:59Z" {
		t.Errorf("expected validityTime 2026-12-31T23:59:59Z, got %s", result.ValidityTime)
	}
}

func TestAuthorizeNiddData_SuccessByGPSI(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "imsi-001010000000001")
				scanString(dest[1], "msisdn-12025551234")
				return nil
			}}
		},
	}
	svc := NewService(db)

	req := &AuthorizationInfo{Dnn: "iot"}
	result, err := svc.AuthorizeNiddData(context.Background(), testGPSI, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AuthorizationData) != 1 {
		t.Fatalf("expected 1 authorization data entry, got %d", len(result.AuthorizationData))
	}
	if result.AuthorizationData[0].Gpsi != "msisdn-12025551234" {
		t.Errorf("expected gpsi msisdn-12025551234, got %s", result.AuthorizationData[0].Gpsi)
	}
}

func TestAuthorizeNiddData_GroupID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &AuthorizationInfo{
		Dnn:          "iot",
		ValidityTime: "2026-12-31T23:59:59Z",
	}
	result, err := svc.AuthorizeNiddData(context.Background(), "group-iot-group", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ValidityTime != "2026-12-31T23:59:59Z" {
		t.Errorf("expected validityTime 2026-12-31T23:59:59Z, got %s", result.ValidityTime)
	}
}

func TestAuthorizeNiddData_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &AuthorizationInfo{Dnn: "iot"}
	_, err := svc.AuthorizeNiddData(context.Background(), "bad-id", req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestAuthorizeNiddData_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.AuthorizeNiddData(context.Background(), testGPSI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestAuthorizeNiddData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	req := &AuthorizationInfo{Dnn: "iot"}
	_, err := svc.AuthorizeNiddData(context.Background(), testSUPI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)

	pd := err.(*errors.ProblemDetails)
	if pd.Cause != errors.CauseUserNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseUserNotFound, pd.Cause)
	}
}

func TestAuthorizeNiddData_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection reset")
			}}
		},
	}
	svc := NewService(db)

	req := &AuthorizationInfo{Dnn: "iot"}
	_, err := svc.AuthorizeNiddData(context.Background(), testSUPI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}
