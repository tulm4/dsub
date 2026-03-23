package rsds

// Service layer tests for the Nudm_RSDS service.
//
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
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
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "group ID rejected", input: "group-group1", wantErr: true},
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
// ReportSMDeliveryStatus tests
// ---------------------------------------------------------------------------

func TestReportSMDeliveryStatus_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	svc := NewService(db)

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportSMDeliveryStatus_WithSUPI(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			// Verify that SUPI is passed as first argument
			if supi, ok := args[0].(*string); ok && supi != nil {
				if *supi != testSUPI {
					return pgconn.NewCommandTag(""), fmt.Errorf("expected supi %s, got %s", testSUPI, *supi)
				}
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
	}
	svc := NewService(db)

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testSUPI, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportSMDeliveryStatus_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), "bad-id", req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_MissingGpsi(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           "",
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_MissingReport(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: nil,
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_NullReport(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: json.RawMessage(`null`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_InvalidGpsi(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           "bad-gpsi-format",
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testSUPI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_GpsiMismatch(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &SmDeliveryStatus{
		Gpsi:           "msisdn-19995559999",
		SmStatusReport: json.RawMessage(`{"status":"delivered"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err == nil {
		t.Fatal("expected error for GPSI mismatch")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestReportSMDeliveryStatus_DBError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), fmt.Errorf("db failure")
		},
	}
	svc := NewService(db)

	req := &SmDeliveryStatus{
		Gpsi:           testGPSI,
		SmStatusReport: json.RawMessage(`{"status":"failed"}`),
	}
	err := svc.ReportSMDeliveryStatus(context.Background(), testGPSI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}
