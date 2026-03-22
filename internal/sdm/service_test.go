package sdm

// Service layer tests for the Nudm_SDM service.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
// Based on: docs/testing-strategy.md (unit testing patterns)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tulm4/dsub/internal/common/errors"
)

// ---------------------------------------------------------------------------
// Mock helpers for database layer
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

// mockDB implements the DB interface used by Service.
type mockDB struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
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

// mockRows implements pgx.Rows for unit tests.
type mockRows struct {
	scanFn func(dest ...any) error
	data   [][]any
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
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	row := r.data[r.idx-1]
	for i, d := range dest {
		if i >= len(row) {
			break
		}
		switch p := d.(type) {
		case *json.RawMessage:
			if v, ok := row[i].(json.RawMessage); ok {
				*p = v
			}
		case *[]string:
			if v, ok := row[i].([]string); ok {
				*p = v
			}
		}
	}
	return nil
}

func (r *mockRows) Close()                                       {}
func (r *mockRows) Err() error                                   { return r.errVal }
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }

// assertStatus is a test helper that checks a *ProblemDetails error for the expected HTTP status.
func assertStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", wantStatus)
	}
	pd, ok := err.(*errors.ProblemDetails)
	if !ok {
		t.Fatalf("expected *ProblemDetails, got %T: %v", err, err)
	}
	if pd.Status != wantStatus {
		t.Errorf("expected status %d, got %d (detail: %s)", wantStatus, pd.Status, pd.Detail)
	}
}

// ---------------------------------------------------------------------------
// Existing tests
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
	// The explicit nil assignment is a standard Go pattern for compile-time checks.
}

// TestValidateSUPIOrUeId tests the identifier validation helper.
func TestValidateSUPIOrUeId(t *testing.T) {
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
			err := validateSUPIOrUeId(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateSUPIOrUeId(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetAmData tests
// ---------------------------------------------------------------------------

func TestGetAmData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, args ...any) pgx.Row {
			if args[0] != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI arg: %v", args[0])
			}
			return &mockRow{scanFn: func(dest ...any) error {
				// Populate a few representative fields among the 22 scan targets.
				if p, ok := dest[0].(*[]string); ok {
					*p = []string{"msisdn-12025551234"}
				}
				if p, ok := dest[14].(*string); ok {
					*p = "ALL_PACKET_SERVICES"
				}
				if p, ok := dest[12].(*bool); ok {
					*p = true
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetAmData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Gpsis) != 1 || result.Gpsis[0] != "msisdn-12025551234" {
		t.Errorf("unexpected gpsis: %v", result.Gpsis)
	}
	if !result.MicoAllowed {
		t.Error("expected micoAllowed=true")
	}
}

func TestGetAmData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetAmData(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

func TestGetAmData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetAmData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// GetSmData tests
// ---------------------------------------------------------------------------

func TestGetSmData_Success(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{
						json.RawMessage(`{"sst":1}`),
						json.RawMessage(`{"internet":{}}`),
						[]string{"group1"},
						[]string{"shared1"},
					},
				},
			}, nil
		},
	}
	svc := NewService(db)
	results, err := svc.GetSmData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if string(results[0].SingleNssai) != `{"sst":1}` {
		t.Errorf("unexpected singleNssai: %s", results[0].SingleNssai)
	}
}

func TestGetSmData_NotFound(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{data: [][]any{}}, nil
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetSmData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetSmData(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

func TestGetSmData_QueryError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusInternalServerError)
}

func TestGetSmData_ScanError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data:   [][]any{{}},
				scanFn: func(_ ...any) error { return fmt.Errorf("scan failed") },
			}, nil
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusInternalServerError)
}

func TestGetSmData_RowsErr(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data:   [][]any{{}},
				errVal: fmt.Errorf("rows iteration error"),
			}, nil
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// GetSmfSelData tests
// ---------------------------------------------------------------------------

func TestGetSmfSelData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"1":{"dnnInfos":[]}}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetSmfSelData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscribedSnssaiInfos == nil {
		t.Error("expected non-nil subscribedSnssaiInfos")
	}
}

func TestGetSmfSelData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmfSelData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetSmfSelData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetSmfSelData(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetNSSAI tests
// ---------------------------------------------------------------------------

func TestGetNSSAI_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"defaultSingleNssais":[{"sst":1}],"singleNssais":[{"sst":1},{"sst":2}]}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetNSSAI(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.DefaultSingleNssais) == 0 {
		t.Error("expected non-empty defaultSingleNssais")
	}
}

func TestGetNSSAI_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetNSSAI(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetNSSAI_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetNSSAI(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

func TestGetNSSAI_EmptyRaw(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				// Scan succeeds but raw is empty (zero-length JSONB)
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage{}
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetNSSAI(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result even for empty JSONB")
	}
}

func TestGetNSSAI_BadJSON(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{bad json`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	_, err := svc.GetNSSAI(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// GetDataSets tests
// ---------------------------------------------------------------------------

func TestGetDataSets_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			// GetAmData query
			if containsSQL(sql, "access_mobility_subscription") {
				return &mockRow{scanFn: func(dest ...any) error {
					if p, ok := dest[0].(*[]string); ok {
						*p = []string{"msisdn-12025551234"}
					}
					return nil
				}}
			}
			// GetSmfSelData query
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"1":{}}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetDataSets(context.Background(), "imsi-001010000000001", []string{"AM", "SMF_SEL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AmData == nil {
		t.Error("expected amData in response")
	}
	if result.SmfSelData == nil {
		t.Error("expected smfSelData in response")
	}
}

func TestGetDataSets_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetDataSets(context.Background(), "bad-supi", []string{"AM"})
	assertStatus(t, err, http.StatusBadRequest)
}

func TestGetDataSets_SMDataset(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{json.RawMessage(`{"sst":1}`), json.RawMessage(`{}`), []string{}, []string{}},
				},
			}, nil
		},
	}
	svc := NewService(db)
	result, err := svc.GetDataSets(context.Background(), "imsi-001010000000001", []string{"SM"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.SmData) != 1 {
		t.Errorf("expected 1 SM data entry, got %d", len(result.SmData))
	}
}

func TestGetDataSets_SMSSubDataset(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"smsSubscribed":true}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetDataSets(context.Background(), "imsi-001010000000001", []string{"SMS_SUB"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SmsSubsData == nil {
		t.Error("expected smsSubsData in response")
	}
}

func TestGetDataSets_PartialFailure(t *testing.T) {
	// If one dataset fails (not found), the result still includes other datasets
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	result, err := svc.GetDataSets(context.Background(), "imsi-001010000000001", []string{"AM", "SMF_SEL"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both fail, so both are nil, but no error returned
	if result.AmData != nil || result.SmfSelData != nil {
		t.Error("expected nil data for not-found datasets")
	}
}

// ---------------------------------------------------------------------------
// GetSmsData tests (uses getJSONBData)
// ---------------------------------------------------------------------------

func TestGetSmsData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"smsSubscribed":true}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetSmsData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.SmsSubscribed {
		t.Error("expected smsSubscribed=true")
	}
}

func TestGetSmsData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmsData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetSmsData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetSmsData(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetSmsMngtData tests
// ---------------------------------------------------------------------------

func TestGetSmsMngtData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"mtSmsSubscribed":true}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetSmsMngtData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.MtSmsSubscribed {
		t.Error("expected mtSmsSubscribed=true")
	}
}

func TestGetSmsMngtData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetSmsMngtData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// GetUeCtxInAmfData tests
// ---------------------------------------------------------------------------

func TestGetUeCtxInAmfData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"accessDetails":{"3gpp":{}}}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetUeCtxInAmfData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGetUeCtxInAmfData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetUeCtxInAmfData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// GetUeCtxInSmfData tests
// ---------------------------------------------------------------------------

func TestGetUeCtxInSmfData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"pduSessions":{}}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetUeCtxInSmfData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGetUeCtxInSmfData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetUeCtxInSmfData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// GetUeCtxInSmsfData tests
// ---------------------------------------------------------------------------

func TestGetUeCtxInSmsfData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"smsfInfo3GppAccess":{}}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetUeCtxInSmsfData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGetUeCtxInSmsfData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetUeCtxInSmsfData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// GetTraceConfigData tests
// ---------------------------------------------------------------------------

func TestGetTraceConfigData_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{"traceRef":"001001-00001","traceDepth":"MINIMUM"}`)
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetTraceConfigData(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TraceRef != "001001-00001" {
		t.Errorf("unexpected traceRef: %s", result.TraceRef)
	}
}

func TestGetTraceConfigData_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetTraceConfigData(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetTraceConfigData_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetTraceConfigData(context.Background(), "bad-supi")
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// GetIdTranslation tests
// ---------------------------------------------------------------------------

func TestGetIdTranslation_SUPItoGPSI(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			if !containsSQL(sql, "supi = $1") {
				t.Error("expected SUPI lookup query")
			}
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "imsi-001010000000001"
				}
				if p, ok := dest[1].(*string); ok {
					*p = "msisdn-12025551234"
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetIdTranslation(context.Background(), "imsi-001010000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Supi != "imsi-001010000000001" {
		t.Errorf("unexpected supi: %s", result.Supi)
	}
	if result.Gpsi != "msisdn-12025551234" {
		t.Errorf("unexpected gpsi: %s", result.Gpsi)
	}
}

func TestGetIdTranslation_GPSItoSUPI(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if !containsSQL(sql, "gpsi = $1") {
				t.Error("expected GPSI lookup query")
			}
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "imsi-001010000000001"
				}
				if p, ok := dest[1].(*string); ok {
					*p = "msisdn-12025551234"
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	result, err := svc.GetIdTranslation(context.Background(), "msisdn-12025551234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Supi != "imsi-001010000000001" {
		t.Errorf("unexpected supi: %s", result.Supi)
	}
}

func TestGetIdTranslation_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetIdTranslation(context.Background(), "imsi-001010000000001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestGetIdTranslation_Invalid(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.GetIdTranslation(context.Background(), "bad-id")
	assertStatus(t, err, http.StatusBadRequest)
}

func TestGetIdTranslation_GPSINotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	_, err := svc.GetIdTranslation(context.Background(), "msisdn-12025551234")
	assertStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Subscribe tests
// ---------------------------------------------------------------------------

func TestSubscribe_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "sub-001"
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	sub := &SdmSubscription{
		NfInstanceID:          "amf-001",
		CallbackReference:     "https://amf.example.com/callback",
		MonitoredResourceUris: []string{"/nudm-sdm/v2/imsi-001010000000001/am-data"},
	}
	result, err := svc.Subscribe(context.Background(), "imsi-001010000000001", sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscriptionID != "sub-001" {
		t.Errorf("expected subscriptionId sub-001, got %s", result.SubscriptionID)
	}
}

func TestSubscribe_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.Subscribe(context.Background(), "imsi-001010000000001", nil)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestSubscribe_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &SdmSubscription{
		NfInstanceID:          "amf-001",
		CallbackReference:     "https://amf.example.com/callback",
		MonitoredResourceUris: []string{"/am-data"},
	}
	_, err := svc.Subscribe(context.Background(), "bad-id", sub)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestSubscribe_MissingNfInstanceID(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &SdmSubscription{
		CallbackReference:     "https://amf.example.com/callback",
		MonitoredResourceUris: []string{"/am-data"},
	}
	_, err := svc.Subscribe(context.Background(), "imsi-001010000000001", sub)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestSubscribe_MissingCallbackReference(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &SdmSubscription{
		NfInstanceID:          "amf-001",
		MonitoredResourceUris: []string{"/am-data"},
	}
	_, err := svc.Subscribe(context.Background(), "imsi-001010000000001", sub)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestSubscribe_MissingMonitoredResourceUris(t *testing.T) {
	svc := NewService(&mockDB{})
	sub := &SdmSubscription{
		NfInstanceID:      "amf-001",
		CallbackReference: "https://amf.example.com/callback",
	}
	_, err := svc.Subscribe(context.Background(), "imsi-001010000000001", sub)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestSubscribe_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return fmt.Errorf("insert failed") }}
		},
	}
	svc := NewService(db)
	sub := &SdmSubscription{
		NfInstanceID:          "amf-001",
		CallbackReference:     "https://amf.example.com/callback",
		MonitoredResourceUris: []string{"/am-data"},
	}
	_, err := svc.Subscribe(context.Background(), "imsi-001010000000001", sub)
	assertStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// ModifySubscription tests
// ---------------------------------------------------------------------------

func TestModifySubscription_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "sub-001"
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	patch := &SdmSubscription{CallbackReference: "https://amf.example.com/new-callback"}
	result, err := svc.ModifySubscription(context.Background(), "imsi-001010000000001", "sub-001", patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SubscriptionID != "sub-001" {
		t.Errorf("expected subscriptionId sub-001, got %s", result.SubscriptionID)
	}
}

func TestModifySubscription_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	patch := &SdmSubscription{CallbackReference: "https://amf.example.com/new-callback"}
	_, err := svc.ModifySubscription(context.Background(), "imsi-001010000000001", "sub-001", patch)
	assertStatus(t, err, http.StatusNotFound)
}

func TestModifySubscription_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})
	_, err := svc.ModifySubscription(context.Background(), "imsi-001010000000001", "sub-001", nil)
	assertStatus(t, err, http.StatusBadRequest)
}

func TestModifySubscription_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	patch := &SdmSubscription{CallbackReference: "https://amf.example.com/new-callback"}
	_, err := svc.ModifySubscription(context.Background(), "bad-id", "sub-001", patch)
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Unsubscribe tests
// ---------------------------------------------------------------------------

func TestUnsubscribe_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*string); ok {
					*p = "sub-001"
				}
				return nil
			}}
		},
	}
	svc := NewService(db)
	err := svc.Unsubscribe(context.Background(), "imsi-001010000000001", "sub-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsubscribe_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	svc := NewService(db)
	err := svc.Unsubscribe(context.Background(), "imsi-001010000000001", "sub-001")
	assertStatus(t, err, http.StatusNotFound)
}

func TestUnsubscribe_InvalidUeID(t *testing.T) {
	svc := NewService(&mockDB{})
	err := svc.Unsubscribe(context.Background(), "bad-id", "sub-001")
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// getJSONBData edge cases
// ---------------------------------------------------------------------------

func TestGetJSONBData_DisallowedTable(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{}`)
				}
				return nil
			}}
		},
	}
	// Call getJSONBData directly with a disallowed table/column pair
	_, err := getJSONBData[SmsSubscriptionData](
		context.Background(), db, "imsi-001010000000001",
		"evil_table", "evil_column", "test",
	)
	assertStatus(t, err, http.StatusInternalServerError)
}

func TestGetJSONBData_EmptyRaw(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage{}
				}
				return nil
			}}
		},
	}
	result, err := getJSONBData[SmsSubscriptionData](
		context.Background(), db, "imsi-001010000000001",
		"sms_subscription_data", "sms_data", "SMS",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for empty JSONB")
	}
}

func TestGetJSONBData_BadJSON(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				if p, ok := dest[0].(*json.RawMessage); ok {
					*p = json.RawMessage(`{bad json`)
				}
				return nil
			}}
		},
	}
	_, err := getJSONBData[SmsSubscriptionData](
		context.Background(), db, "imsi-001010000000001",
		"sms_subscription_data", "sms_data", "SMS",
	)
	assertStatus(t, err, http.StatusInternalServerError)
}

func TestGetJSONBData_InvalidSUPI(t *testing.T) {
	_, err := getJSONBData[SmsSubscriptionData](
		context.Background(), &mockDB{}, "bad-supi",
		"sms_subscription_data", "sms_data", "SMS",
	)
	assertStatus(t, err, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// containsSQL checks if sql contains the given substring (for query routing).
func containsSQL(sql, substr string) bool {
	return strings.Contains(sql, substr)
}
