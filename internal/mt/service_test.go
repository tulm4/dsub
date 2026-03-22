package mt

// Service layer tests for the Nudm_MT service.
//
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
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
// ValidateSUPI tests
// ---------------------------------------------------------------------------

func TestValidateSUPI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "GPSI not accepted", input: "msisdn-12025551234", wantErr: true},
		{name: "invalid SUPI digits", input: "imsi-12345", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSUPI(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateSUPI(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// QueryUeInfo tests
// ---------------------------------------------------------------------------

func TestQueryUeInfo_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "amf-001")
				scanString(dest[1], "NR")
				scanString(dest[2], "3GPP_ACCESS")
				return nil
			}}
		},
	}
	svc := NewService(db)

	result, err := svc.QueryUeInfo(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ServingAmfId != "amf-001" {
		t.Errorf("expected servingAmfId amf-001, got %s", result.ServingAmfId)
	}
	if result.RatType != "NR" {
		t.Errorf("expected ratType NR, got %s", result.RatType)
	}
	if result.UserState != "REGISTERED" {
		t.Errorf("expected userState REGISTERED, got %s", result.UserState)
	}
	if result.AccessType != "3GPP_ACCESS" {
		t.Errorf("expected accessType 3GPP_ACCESS, got %s", result.AccessType)
	}
}

func TestQueryUeInfo_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.QueryUeInfo(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestQueryUeInfo_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("no rows")
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.QueryUeInfo(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)

	pd := err.(*errors.ProblemDetails)
	if pd.Cause != errors.CauseContextNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseContextNotFound, pd.Cause)
	}
}

func TestQueryUeInfo_GPSIRejected(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.QueryUeInfo(context.Background(), "msisdn-12025551234")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// ProvideLocationInfo tests
// ---------------------------------------------------------------------------

func TestProvideLocationInfo_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "amf-002")
				scanString(dest[1], "NR")
				return nil
			}}
		},
	}
	svc := NewService(db)

	req := &LocationInfoRequest{Req5gsInd: true}
	result, err := svc.ProvideLocationInfo(context.Background(), testSUPI, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ServingAmfId != "amf-002" {
		t.Errorf("expected servingAmfId amf-002, got %s", result.ServingAmfId)
	}
	if result.Supi != testSUPI {
		t.Errorf("expected supi %s, got %s", testSUPI, result.Supi)
	}
	if result.UserState != "REGISTERED" {
		t.Errorf("expected userState REGISTERED, got %s", result.UserState)
	}
}

func TestProvideLocationInfo_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})

	req := &LocationInfoRequest{Req5gsInd: true}
	_, err := svc.ProvideLocationInfo(context.Background(), "bad-id", req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestProvideLocationInfo_NilRequest(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.ProvideLocationInfo(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestProvideLocationInfo_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("no rows")
			}}
		},
	}
	svc := NewService(db)

	req := &LocationInfoRequest{Req5gsInd: true}
	_, err := svc.ProvideLocationInfo(context.Background(), testSUPI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)

	pd := err.(*errors.ProblemDetails)
	if pd.Cause != errors.CauseContextNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseContextNotFound, pd.Cause)
	}
}

func TestProvideLocationInfo_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("db failure")
			}}
		},
	}
	svc := NewService(db)

	req := &LocationInfoRequest{Req5gsInd: true}
	_, err := svc.ProvideLocationInfo(context.Background(), testSUPI, req)
	if err == nil {
		t.Fatal("expected error")
	}
	// DB errors on scan are mapped to 404 (not found) since we can't distinguish
	// between "no rows" and "db error" with pgx.Row.Scan pattern
	assertProblemStatus(t, err, http.StatusNotFound)
}
