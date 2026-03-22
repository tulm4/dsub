package uecm

// Service layer tests for the Nudm_UECM service.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
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

// mockRows implements pgx.Rows for unit tests.
type mockRows struct {
	data   []func(dest ...any) error
	idx    int
	errVal error
}

func (r *mockRows) Next() bool {
	if r.idx < len(r.data) {
		r.idx++
		return true
	}
	return false
}

func (r *mockRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx-1 >= len(r.data) {
		return fmt.Errorf("mockRows: Scan called without valid Next()")
	}
	return r.data[r.idx-1](dest...)
}
func (r *mockRows) Close()     {}
func (r *mockRows) Err() error { return r.errVal }
func (r *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("")
}
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }

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
// Existing tests (preserved as-is)
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
	// This will fail at compile time if Service does not implement ServiceInterface.
}

// TestValidateUeID tests the identifier validation helper.
func TestValidateUeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "valid GPSI msisdn", input: "msisdn-12025551234", wantErr: false},
		{name: "valid GPSI extid", input: "extid-user@example.com", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "invalid SUPI digits", input: "imsi-12345", wantErr: true},
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

// TestParsePduSessionID tests the PDU session ID parser.
func TestParsePduSessionID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "valid 0", input: "0", want: 0, wantErr: false},
		{name: "valid 5", input: "5", want: 5, wantErr: false},
		{name: "valid 255", input: "255", want: 255, wantErr: false},
		{name: "negative", input: "-1", want: 0, wantErr: true},
		{name: "too large", input: "256", want: 0, wantErr: true},
		{name: "non-numeric", input: "abc", want: 0, wantErr: true},
		{name: "empty", input: "", want: 0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePduSessionID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parsePduSessionID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("parsePduSessionID(%q): got %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Register3GppAccess tests
// ---------------------------------------------------------------------------

func TestRegister3GppAccess_Success_Created(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false // existed=false → created=true
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:    "amf-001",
		DeregCallbackURI: "https://amf/dereg",
	}

	result, created, err := svc.Register3GppAccess(context.Background(), testSUPI, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result != reg {
		t.Error("expected returned reg to be the same object")
	}
}

func TestRegister3GppAccess_Success_Updated(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = true // existed=true → created=false
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:    "amf-001",
		DeregCallbackURI: "https://amf/dereg",
	}

	_, created, err := svc.Register3GppAccess(context.Background(), testSUPI, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false for update")
	}
}

func TestRegister3GppAccess_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:    "amf-001",
		DeregCallbackURI: "https://amf/dereg",
	}

	_, _, err := svc.Register3GppAccess(context.Background(), "bad-id", reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRegister3GppAccess_NilRegistration(t *testing.T) {
	svc := NewService(&mockDB{})

	_, _, err := svc.Register3GppAccess(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRegister3GppAccess_MissingAmfInstanceID(t *testing.T) {
	svc := NewService(&mockDB{})
	reg := &Amf3GppAccessRegistration{
		DeregCallbackURI: "https://amf/dereg",
	}

	_, _, err := svc.Register3GppAccess(context.Background(), testSUPI, reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRegister3GppAccess_MissingDeregCallback(t *testing.T) {
	svc := NewService(&mockDB{})
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID: "amf-001",
	}

	_, _, err := svc.Register3GppAccess(context.Background(), testSUPI, reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestRegister3GppAccess_ScanError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("db failure")
			}}
		},
	}
	svc := NewService(db)
	reg := &Amf3GppAccessRegistration{
		AmfInstanceID:    "amf-001",
		DeregCallbackURI: "https://amf/dereg",
	}

	_, _, err := svc.Register3GppAccess(context.Background(), testSUPI, reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Get3GppRegistration tests
// ---------------------------------------------------------------------------

func TestGet3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "amf-001"
				*dest[1].(*string) = "https://amf/dereg"
				*dest[2].(*json.RawMessage) = json.RawMessage(`{}`)
				*dest[3].(*string) = "NR"
				*dest[4].(*bool) = true
				*dest[5].(*string) = "imei-123456789012345"
				*dest[6].(*string) = "2024-01-01T00:00:00Z"
				return nil
			}}
		},
	}
	svc := NewService(db)

	reg, err := svc.Get3GppRegistration(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.AmfInstanceID != "amf-001" {
		t.Errorf("expected AmfInstanceID=amf-001, got %s", reg.AmfInstanceID)
	}
	if reg.RatType != "NR" {
		t.Errorf("expected RatType=NR, got %s", reg.RatType)
	}
	if !reg.InitialRegistrationInd {
		t.Error("expected InitialRegistrationInd=true")
	}
}

func TestGet3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.Get3GppRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Update3GppRegistration tests
// ---------------------------------------------------------------------------

func TestUpdate3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "amf-001"
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &Amf3GppAccessRegistration{Pei: "imei-111111111111111"}

	result, err := svc.Update3GppRegistration(context.Background(), testSUPI, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AmfInstanceID != "amf-001" {
		t.Errorf("expected AmfInstanceID=amf-001, got %s", result.AmfInstanceID)
	}
}

func TestUpdate3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)
	patch := &Amf3GppAccessRegistration{}

	_, err := svc.Update3GppRegistration(context.Background(), testSUPI, patch)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestUpdate3GppRegistration_NilPatch(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.Update3GppRegistration(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// DeregAMF tests
// ---------------------------------------------------------------------------

func TestDeregAMF_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregAMF(context.Background(), testSUPI, &DeregistrationData{DeregReason: "UE_INITIAL_REGISTRATION"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeregAMF_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.DeregAMF(context.Background(), "bad-id", &DeregistrationData{DeregReason: "UE_INITIAL_REGISTRATION"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestDeregAMF_NilData(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.DeregAMF(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestDeregAMF_MissingDeregReason(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.DeregAMF(context.Background(), testSUPI, &DeregistrationData{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestDeregAMF_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregAMF(context.Background(), testSUPI, &DeregistrationData{DeregReason: "UE_INITIAL_REGISTRATION"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestDeregAMF_ExecError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag(""), fmt.Errorf("connection lost")
		},
	}
	svc := NewService(db)

	err := svc.DeregAMF(context.Background(), testSUPI, &DeregistrationData{DeregReason: "UE_INITIAL_REGISTRATION"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// RegisterNon3GppAccess tests
// ---------------------------------------------------------------------------

func TestRegisterNon3GppAccess_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &AmfNon3GppAccessRegistration{
		AmfInstanceID:    "amf-002",
		DeregCallbackURI: "https://amf/dereg",
	}

	result, created, err := svc.RegisterNon3GppAccess(context.Background(), testSUPI, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result != reg {
		t.Error("expected returned reg to be the same object")
	}
}

func TestRegisterNon3GppAccess_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	reg := &AmfNon3GppAccessRegistration{
		AmfInstanceID:    "amf-002",
		DeregCallbackURI: "https://amf/dereg",
	}

	_, _, err := svc.RegisterNon3GppAccess(context.Background(), "bad-id", reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetNon3GppRegistration tests
// ---------------------------------------------------------------------------

func TestGetNon3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "amf-002"
				*dest[1].(*string) = "https://amf/dereg"
				*dest[2].(*json.RawMessage) = json.RawMessage(`{}`)
				*dest[3].(*string) = "NR"
				*dest[4].(*bool) = false
				*dest[5].(*string) = ""
				*dest[6].(*string) = "2024-01-01T00:00:00Z"
				return nil
			}}
		},
	}
	svc := NewService(db)

	reg, err := svc.GetNon3GppRegistration(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.AmfInstanceID != "amf-002" {
		t.Errorf("expected AmfInstanceID=amf-002, got %s", reg.AmfInstanceID)
	}
}

func TestGetNon3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetNon3GppRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// UpdateNon3GppRegistration tests
// ---------------------------------------------------------------------------

func TestUpdateNon3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "amf-002"
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &AmfNon3GppAccessRegistration{Pei: "imei-222222222222222"}

	result, err := svc.UpdateNon3GppRegistration(context.Background(), testSUPI, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AmfInstanceID != "amf-002" {
		t.Errorf("expected AmfInstanceID=amf-002, got %s", result.AmfInstanceID)
	}
}

func TestUpdateNon3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)
	patch := &AmfNon3GppAccessRegistration{}

	_, err := svc.UpdateNon3GppRegistration(context.Background(), testSUPI, patch)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// RegisterSmf tests
// ---------------------------------------------------------------------------

func TestRegisterSmf_Success_Created(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &SmfRegistration{
		SmfInstanceID: "smf-001",
		Dnn:           "internet",
	}

	result, created, err := svc.RegisterSmf(context.Background(), testSUPI, 5, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result.PduSessionID != 5 {
		t.Errorf("expected PduSessionID=5, got %d", result.PduSessionID)
	}
}

func TestRegisterSmf_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	reg := &SmfRegistration{SmfInstanceID: "smf-001"}

	_, _, err := svc.RegisterSmf(context.Background(), "bad-id", 5, reg)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetSmfRegistration tests
// ---------------------------------------------------------------------------

func TestGetSmfRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data: []func(dest ...any) error{
					func(dest ...any) error {
						*dest[0].(*string) = "smf-001"
						*dest[1].(*int) = 5
						*dest[2].(*string) = "internet"
						*dest[3].(*json.RawMessage) = json.RawMessage(`{"sst":1}`)
						*dest[4].(*json.RawMessage) = json.RawMessage(`{"mcc":"001","mnc":"01"}`)
						*dest[5].(*string) = "2024-01-01T00:00:00Z"
						return nil
					},
				},
			}, nil
		},
	}
	svc := NewService(db)

	regs, err := svc.GetSmfRegistration(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].SmfInstanceID != "smf-001" {
		t.Errorf("expected SmfInstanceID=smf-001, got %s", regs[0].SmfInstanceID)
	}
	if regs[0].PduSessionID != 5 {
		t.Errorf("expected PduSessionID=5, got %d", regs[0].PduSessionID)
	}
}

func TestGetSmfRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}
	svc := NewService(db)

	_, err := svc.GetSmfRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestGetSmfRegistration_QueryError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, fmt.Errorf("connection lost")
		},
	}
	svc := NewService(db)

	_, err := svc.GetSmfRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// RetrieveSmfRegistration tests
// ---------------------------------------------------------------------------

func TestRetrieveSmfRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smf-001"
				*dest[1].(*int) = 5
				*dest[2].(*string) = "internet"
				*dest[3].(*json.RawMessage) = json.RawMessage(`{"sst":1}`)
				*dest[4].(*json.RawMessage) = json.RawMessage(`{"mcc":"001","mnc":"01"}`)
				*dest[5].(*string) = "2024-01-01T00:00:00Z"
				return nil
			}}
		},
	}
	svc := NewService(db)

	reg, err := svc.RetrieveSmfRegistration(context.Background(), testSUPI, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.SmfInstanceID != "smf-001" {
		t.Errorf("expected SmfInstanceID=smf-001, got %s", reg.SmfInstanceID)
	}
}

func TestRetrieveSmfRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.RetrieveSmfRegistration(context.Background(), testSUPI, 5)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeregisterSmf tests
// ---------------------------------------------------------------------------

func TestDeregisterSmf_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregisterSmf(context.Background(), testSUPI, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeregisterSmf_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregisterSmf(context.Background(), testSUPI, 5)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// UpdateSmfRegistration tests
// ---------------------------------------------------------------------------

func TestUpdateSmfRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smf-001"
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &SmfRegistration{Dnn: "ims"}

	result, err := svc.UpdateSmfRegistration(context.Background(), testSUPI, 5, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SmfInstanceID != "smf-001" {
		t.Errorf("expected SmfInstanceID=smf-001, got %s", result.SmfInstanceID)
	}
	if result.PduSessionID != 5 {
		t.Errorf("expected PduSessionID=5, got %d", result.PduSessionID)
	}
}

func TestUpdateSmfRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)
	patch := &SmfRegistration{}

	_, err := svc.UpdateSmfRegistration(context.Background(), testSUPI, 5, patch)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// RegisterSmsf3Gpp tests
// ---------------------------------------------------------------------------

func TestRegisterSmsf3Gpp_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &SmsfRegistration{SmsfInstanceID: "smsf-001"}

	result, created, err := svc.RegisterSmsf3Gpp(context.Background(), testSUPI, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result != reg {
		t.Error("expected returned reg to be the same object")
	}
}

// ---------------------------------------------------------------------------
// GetSmsf3GppRegistration tests
// ---------------------------------------------------------------------------

func TestGetSmsf3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smsf-001"
				*dest[1].(*json.RawMessage) = json.RawMessage(`{"mcc":"001","mnc":"01"}`)
				*dest[2].(*string) = "2024-01-01T00:00:00Z"
				return nil
			}}
		},
	}
	svc := NewService(db)

	reg, err := svc.GetSmsf3GppRegistration(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.SmsfInstanceID != "smsf-001" {
		t.Errorf("expected SmsfInstanceID=smsf-001, got %s", reg.SmsfInstanceID)
	}
}

func TestGetSmsf3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetSmsf3GppRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeregisterSmsf3Gpp tests
// ---------------------------------------------------------------------------

func TestDeregisterSmsf3Gpp_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregisterSmsf3Gpp(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeregisterSmsf3Gpp_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregisterSmsf3Gpp(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// UpdateSmsf3GppRegistration tests
// ---------------------------------------------------------------------------

func TestUpdateSmsf3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smsf-001"
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &SmsfRegistration{RegistrationTime: "2024-06-01T00:00:00Z"}

	result, err := svc.UpdateSmsf3GppRegistration(context.Background(), testSUPI, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SmsfInstanceID != "smsf-001" {
		t.Errorf("expected SmsfInstanceID=smsf-001, got %s", result.SmsfInstanceID)
	}
}

// ---------------------------------------------------------------------------
// RegisterSmsfNon3Gpp tests
// ---------------------------------------------------------------------------

func TestRegisterSmsfNon3Gpp_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*bool) = false
				return nil
			}}
		},
	}
	svc := NewService(db)
	reg := &SmsfRegistration{SmsfInstanceID: "smsf-002"}

	result, created, err := svc.RegisterSmsfNon3Gpp(context.Background(), testSUPI, reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result != reg {
		t.Error("expected returned reg to be the same object")
	}
}

// ---------------------------------------------------------------------------
// GetSmsfNon3GppRegistration tests
// ---------------------------------------------------------------------------

func TestGetSmsfNon3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smsf-002"
				*dest[1].(*json.RawMessage) = json.RawMessage(`{"mcc":"001","mnc":"01"}`)
				*dest[2].(*string) = "2024-01-01T00:00:00Z"
				return nil
			}}
		},
	}
	svc := NewService(db)

	reg, err := svc.GetSmsfNon3GppRegistration(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.SmsfInstanceID != "smsf-002" {
		t.Errorf("expected SmsfInstanceID=smsf-002, got %s", reg.SmsfInstanceID)
	}
}

func TestGetSmsfNon3GppRegistration_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetSmsfNon3GppRegistration(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeregisterSmsfNon3Gpp tests
// ---------------------------------------------------------------------------

func TestDeregisterSmsfNon3Gpp_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeregisterSmsfNon3Gpp(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateSmsfNon3GppRegistration tests
// ---------------------------------------------------------------------------

func TestUpdateSmsfNon3GppRegistration_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smsf-002"
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &SmsfRegistration{RegistrationTime: "2024-06-01T00:00:00Z"}

	result, err := svc.UpdateSmsfNon3GppRegistration(context.Background(), testSUPI, patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SmsfInstanceID != "smsf-002" {
		t.Errorf("expected SmsfInstanceID=smsf-002, got %s", result.SmsfInstanceID)
	}
}

// ---------------------------------------------------------------------------
// GetRegistrations tests
// ---------------------------------------------------------------------------

func TestGetRegistrations_Success(t *testing.T) {
	// GetRegistrations calls Get3Gpp, GetNon3Gpp, GetSmf, GetSmsf3Gpp, GetSmsfNon3Gpp.
	// All sub-calls return errors (not found), so the result is empty but non-nil.
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}
	svc := NewService(db)

	result, err := svc.GetRegistrations(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Amf3GppAccess != nil {
		t.Error("expected nil Amf3GppAccess")
	}
	if result.AmfNon3GppAccess != nil {
		t.Error("expected nil AmfNon3GppAccess")
	}
	if result.SmfRegistrations != nil {
		t.Error("expected nil SmfRegistrations")
	}
}

func TestGetRegistrations_WithData(t *testing.T) {
	// queryRowFn is called 4 times. We use a counter to populate
	// the correct fields for each sub-call.
	callNum := 0
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			callNum++
			switch callNum {
			case 1: // Get3GppRegistration (7 fields)
				return &mockRow{scanFn: func(dest ...any) error {
					*dest[0].(*string) = "amf-001"
					*dest[1].(*string) = "https://amf/dereg"
					*dest[2].(*json.RawMessage) = json.RawMessage(`{}`)
					*dest[3].(*string) = "NR"
					*dest[4].(*bool) = true
					*dest[5].(*string) = ""
					*dest[6].(*string) = "2024-01-01T00:00:00Z"
					return nil
				}}
			case 2: // GetNon3GppRegistration (7 fields)
				return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			case 3: // GetSmsf3GppRegistration (3 fields)
				return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			case 4: // GetSmsfNon3GppRegistration (3 fields)
				return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			default:
				return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{data: nil}, nil
		},
	}
	svc := NewService(db)

	result, err := svc.GetRegistrations(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Amf3GppAccess == nil {
		t.Fatal("expected non-nil Amf3GppAccess")
	}
	if result.Amf3GppAccess.AmfInstanceID != "amf-001" {
		t.Errorf("expected AmfInstanceID=amf-001, got %s", result.Amf3GppAccess.AmfInstanceID)
	}
}

func TestGetRegistrations_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.GetRegistrations(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// PeiUpdate tests
// ---------------------------------------------------------------------------

func TestPeiUpdate_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.PeiUpdate(context.Background(), testSUPI, &PeiUpdateInfo{Pei: "imei-123456789012345"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPeiUpdate_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.PeiUpdate(context.Background(), "bad-id", &PeiUpdateInfo{Pei: "imei-123456789012345"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestPeiUpdate_NilInfo(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.PeiUpdate(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestPeiUpdate_EmptyPei(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.PeiUpdate(context.Background(), testSUPI, &PeiUpdateInfo{Pei: ""})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestPeiUpdate_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.PeiUpdate(context.Background(), testSUPI, &PeiUpdateInfo{Pei: "imei-123456789012345"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// UpdateRoamingInformation tests
// ---------------------------------------------------------------------------

func TestUpdateRoamingInformation_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.UpdateRoamingInformation(context.Background(), testSUPI, &RoamingInfoUpdate{Roaming: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateRoamingInformation_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.UpdateRoamingInformation(context.Background(), "bad-id", &RoamingInfoUpdate{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdateRoamingInformation_NilInfo(t *testing.T) {
	svc := NewService(&mockDB{})

	err := svc.UpdateRoamingInformation(context.Background(), testSUPI, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestUpdateRoamingInformation_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.UpdateRoamingInformation(context.Background(), testSUPI, &RoamingInfoUpdate{Roaming: true})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// SendRoutingInfoSm tests
// ---------------------------------------------------------------------------

func TestSendRoutingInfoSm_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				*dest[0].(*string) = "smsf-001"
				return nil
			}}
		},
	}
	svc := NewService(db)

	resp, err := svc.SendRoutingInfoSm(context.Background(), testSUPI, &RoutingInfoSmRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SmsfInstanceID != "smsf-001" {
		t.Errorf("expected SmsfInstanceID=smsf-001, got %s", resp.SmsfInstanceID)
	}
}

func TestSendRoutingInfoSm_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.SendRoutingInfoSm(context.Background(), "bad-id", &RoutingInfoSmRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestSendRoutingInfoSm_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.SendRoutingInfoSm(context.Background(), testSUPI, &RoutingInfoSmRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}
