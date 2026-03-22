package pp

// Service layer tests for the Nudm_PP service.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
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

// scanIntPtr safely assigns an *int to a scan destination.
func scanIntPtr(dest any, val *int) {
	if p, ok := dest.(**int); ok {
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

// TestNewService verifies the Service constructor returns a properly
// initialized instance.
func TestNewService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// TestServiceImplementsInterface verifies that *Service satisfies
// ServiceInterface at compile time.
func TestServiceImplementsInterface(t *testing.T) {
	var _ ServiceInterface = (*Service)(nil)
}

// ---------------------------------------------------------------------------
// validateSUPI tests
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
		{name: "GPSI rejected", input: "msisdn-12025551234", wantErr: true},
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
// GetPPData tests
// ---------------------------------------------------------------------------

func TestGetPPData_Success(t *testing.T) {
	count := 10
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanJSON(dest[0], json.RawMessage(`{"rat":"NR"}`))
				scanString(dest[1], "abc")
				scanJSON(dest[2], json.RawMessage(`{"period":3600}`))
				// dest[3..7]: ec_restriction, acs_info, sor_info, five_mbs, steering — nil
				scanIntPtr(dest[8], &count)
				// dest[9]: pp_dl_packet_count_ext — nil
				// dest[10]: pp_maximum_response_time — nil
				// dest[11]: pp_maximum_latency — nil
				return nil
			}}
		},
	}
	svc := NewService(db)

	result, err := svc.GetPPData(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SupportedFeatures != "abc" {
		t.Errorf("expected supportedFeatures abc, got %s", result.SupportedFeatures)
	}
	if result.PpDlPacketCount == nil || *result.PpDlPacketCount != 10 {
		t.Errorf("expected ppDlPacketCount 10, got %v", result.PpDlPacketCount)
	}
	if string(result.CommunicationCharacteristics) != `{"rat":"NR"}` {
		t.Errorf("unexpected communicationCharacteristics: %s", string(result.CommunicationCharacteristics))
	}
}

func TestGetPPData_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.GetPPData(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestGetPPData_GPSIRejected(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.GetPPData(context.Background(), "msisdn-12025551234")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestGetPPData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetPPData(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestGetPPData_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetPPData(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// UpdatePPData tests
// ---------------------------------------------------------------------------

func TestUpdatePPData_Success(t *testing.T) {
	count := 5
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanJSON(dest[0], json.RawMessage(`{"rat":"NR"}`))
				scanString(dest[1], "def")
				// dest[2..7]: expected_ue, ec, acs, sor, five_mbs, steering — nil
				scanIntPtr(dest[8], &count)
				// dest[9..11]: ext, response_time, latency — nil
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &PpData{
		SupportedFeatures:            "def",
		CommunicationCharacteristics: json.RawMessage(`{"rat":"NR"}`),
		PpDlPacketCount:              &count,
	}

	result, err := svc.UpdatePPData(context.Background(), testSUPI, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SupportedFeatures != "def" {
		t.Errorf("expected supportedFeatures def, got %s", result.SupportedFeatures)
	}
	if result.PpDlPacketCount == nil || *result.PpDlPacketCount != 5 {
		t.Errorf("expected ppDlPacketCount 5, got %v", result.PpDlPacketCount)
	}
}

func TestUpdatePPData_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.UpdatePPData(context.Background(), "bad-id", &PpData{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdatePPData_NilPatch(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.UpdatePPData(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdatePPData_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.UpdatePPData(context.Background(), testSUPI, &PpData{SupportedFeatures: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}
