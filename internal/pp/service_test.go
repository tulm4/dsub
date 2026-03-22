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

// ---------------------------------------------------------------------------
// Scan helpers for VN Group / MBS tests
// ---------------------------------------------------------------------------

// scanBool safely assigns a bool to a scan destination.
func scanBool(dest any, val bool) {
	if p, ok := dest.(*bool); ok {
		*p = val
	}
}

// scanBytes safely assigns a []byte to a scan destination.
func scanBytes(dest any, val []byte) {
	if p, ok := dest.(*[]byte); ok {
		*p = val
	}
}

// ---------------------------------------------------------------------------
// validateExtGroupID tests
// ---------------------------------------------------------------------------

func TestValidateExtGroupID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid group id", input: "ext-group-1", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExtGroupID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateExtGroupID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create5GVnGroup tests
// ---------------------------------------------------------------------------

func TestCreate5GVnGroup_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanBool(dest[0], true)
				return nil
			}}
		},
	}
	svc := NewService(db)

	cfg := &VnGroupConfiguration{
		Dnn:     "internet",
		Members: []string{"msisdn-12025551234"},
	}
	result, created, err := svc.Create5GVnGroup(context.Background(), "ext-group-1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result.Dnn != "internet" {
		t.Errorf("expected dnn 'internet', got %s", result.Dnn)
	}
	if len(result.Members) != 1 || result.Members[0] != "msisdn-12025551234" {
		t.Errorf("unexpected members: %v", result.Members)
	}
}

func TestCreate5GVnGroup_EmptyExtGroupID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, _, err := svc.Create5GVnGroup(context.Background(), "", &VnGroupConfiguration{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreate5GVnGroup_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	_, _, err := svc.Create5GVnGroup(context.Background(), "ext-group-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreate5GVnGroup_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, _, err := svc.Create5GVnGroup(context.Background(), "ext-group-1", &VnGroupConfiguration{Dnn: "internet"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Get5GVnGroup tests
// ---------------------------------------------------------------------------

func TestGet5GVnGroup_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "internet")                              // dnn
				scanJSON(dest[1], json.RawMessage(`{"sst":1}`))              // s_nssai
				scanBytes(dest[2], []byte(`["IPV4"]`))                       // pdu_session_types
				// dest[3]: app_descriptors — nil
				scanBool(dest[4], true)                                      // secondary_auth
				// dest[5]: dn_aaa_address — nil
				scanString(dest[6], "aaa.example.com")                       // dn_aaa_fqdn
				scanBytes(dest[7], []byte(`["msisdn-12025551234"]`))         // members
				scanString(dest[8], "ref-1")                                 // reference_id
				scanString(dest[9], "af-1")                                  // af_instance_id
				scanString(dest[10], "int-group-1")                          // internal_group_identifier
				// dest[11]: mtc_provider_information — nil
				return nil
			}}
		},
	}
	svc := NewService(db)

	result, err := svc.Get5GVnGroup(context.Background(), "ext-group-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Dnn != "internet" {
		t.Errorf("expected dnn 'internet', got %s", result.Dnn)
	}
	if string(result.SNssai) != `{"sst":1}` {
		t.Errorf("unexpected sNssai: %s", string(result.SNssai))
	}
	if len(result.PduSessionTypes) != 1 || result.PduSessionTypes[0] != "IPV4" {
		t.Errorf("unexpected pduSessionTypes: %v", result.PduSessionTypes)
	}
	if !result.SecondaryAuth {
		t.Error("expected secondaryAuth=true")
	}
	if len(result.Members) != 1 || result.Members[0] != "msisdn-12025551234" {
		t.Errorf("unexpected members: %v", result.Members)
	}
	if result.ReferenceId != "ref-1" {
		t.Errorf("expected referenceId 'ref-1', got %s", result.ReferenceId)
	}
}

func TestGet5GVnGroup_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.Get5GVnGroup(context.Background(), "ext-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestGet5GVnGroup_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.Get5GVnGroup(context.Background(), "ext-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// Modify5GVnGroup tests
// ---------------------------------------------------------------------------

func TestModify5GVnGroup_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanString(dest[0], "new-dnn")
				scanJSON(dest[1], json.RawMessage(`{"sst":1}`))
				scanBytes(dest[2], []byte(`["IPV4"]`))
				// dest[3]: app_descriptors — nil
				scanBool(dest[4], false)
				// dest[5]: dn_aaa_address — nil
				scanString(dest[6], "aaa.example.com")
				scanBytes(dest[7], []byte(`["msisdn-12025551234"]`))
				scanString(dest[8], "ref-1")
				scanString(dest[9], "af-1")
				scanString(dest[10], "int-group-1")
				// dest[11]: mtc_provider_information — nil
				return nil
			}}
		},
	}
	svc := NewService(db)

	patch := &VnGroupConfiguration{Dnn: "new-dnn"}
	result, err := svc.Modify5GVnGroup(context.Background(), "ext-group-1", patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Dnn != "new-dnn" {
		t.Errorf("expected dnn 'new-dnn', got %s", result.Dnn)
	}
}

func TestModify5GVnGroup_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.Modify5GVnGroup(context.Background(), "ext-group-1", &VnGroupConfiguration{Dnn: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// Delete5GVnGroup tests
// ---------------------------------------------------------------------------

func TestDelete5GVnGroup_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.Delete5GVnGroup(context.Background(), "ext-group-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete5GVnGroup_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.Delete5GVnGroup(context.Background(), "ext-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// CreateMbsGroupMembership tests
// ---------------------------------------------------------------------------

func TestCreateMbsGroupMembership_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanBool(dest[0], true)
				return nil
			}}
		},
	}
	svc := NewService(db)

	memb := &MbsGroupMemb{
		AfInstanceId:            "af-1",
		InternalGroupIdentifier: "int-group-1",
	}
	result, created, err := svc.CreateMbsGroupMembership(context.Background(), "mbs-group-1", memb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}
	if result.AfInstanceId != "af-1" {
		t.Errorf("expected afInstanceId 'af-1', got %s", result.AfInstanceId)
	}
}

func TestCreateMbsGroupMembership_EmptyExtGroupID(t *testing.T) {
	svc := NewService(&mockDB{})

	_, _, err := svc.CreateMbsGroupMembership(context.Background(), "", &MbsGroupMemb{})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateMbsGroupMembership_NilBody(t *testing.T) {
	svc := NewService(&mockDB{})

	_, _, err := svc.CreateMbsGroupMembership(context.Background(), "mbs-group-1", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestCreateMbsGroupMembership_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, _, err := svc.CreateMbsGroupMembership(context.Background(), "mbs-group-1", &MbsGroupMemb{AfInstanceId: "af-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// GetMbsGroupMembership tests
// ---------------------------------------------------------------------------

func TestGetMbsGroupMembership_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanJSON(dest[0], json.RawMessage(`{"tmgi":"010203"}`)) // multicast_group_memb
				scanString(dest[1], "af-1")                             // af_instance_id
				scanString(dest[2], "int-group-1")                      // internal_group_identifier
				return nil
			}}
		},
	}
	svc := NewService(db)

	result, err := svc.GetMbsGroupMembership(context.Background(), "mbs-group-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AfInstanceId != "af-1" {
		t.Errorf("expected afInstanceId 'af-1', got %s", result.AfInstanceId)
	}
	if string(result.MulticastGroupMemb) != `{"tmgi":"010203"}` {
		t.Errorf("unexpected multicastGroupMemb: %s", string(result.MulticastGroupMemb))
	}
}

func TestGetMbsGroupMembership_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetMbsGroupMembership(context.Background(), "mbs-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

func TestGetMbsGroupMembership_DBError(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return fmt.Errorf("connection refused")
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.GetMbsGroupMembership(context.Background(), "mbs-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

// ---------------------------------------------------------------------------
// ModifyMbsGroupMembership tests
// ---------------------------------------------------------------------------

func TestModifyMbsGroupMembership_Success(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(dest ...any) error {
				scanJSON(dest[0], json.RawMessage(`{"tmgi":"999999"}`))
				scanString(dest[1], "af-2")
				scanString(dest[2], "int-group-1")
				return nil
			}}
		},
	}
	svc := NewService(db)

	patch := &MbsGroupMemb{AfInstanceId: "af-2"}
	result, err := svc.ModifyMbsGroupMembership(context.Background(), "mbs-group-1", patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AfInstanceId != "af-2" {
		t.Errorf("expected afInstanceId 'af-2', got %s", result.AfInstanceId)
	}
}

func TestModifyMbsGroupMembership_NotFound(t *testing.T) {
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	svc := NewService(db)

	_, err := svc.ModifyMbsGroupMembership(context.Background(), "mbs-group-1", &MbsGroupMemb{AfInstanceId: "af-2"})
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// DeleteMbsGroupMembership tests
// ---------------------------------------------------------------------------

func TestDeleteMbsGroupMembership_Success(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 1"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeleteMbsGroupMembership(context.Background(), "mbs-group-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteMbsGroupMembership_NotFound(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}
	svc := NewService(db)

	err := svc.DeleteMbsGroupMembership(context.Background(), "mbs-group-1")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// mockRows for multi-row queries
// ---------------------------------------------------------------------------

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
		case *string:
			if v, ok := row[i].(string); ok {
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

// ---------------------------------------------------------------------------
// GetSdmSubscriptionsForNotify tests
// ---------------------------------------------------------------------------

func TestGetSdmSubscriptionsForNotify_Success(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data: [][]any{
					{"sdm-sub-001", "https://amf.example.com/sdm-cb", []string{"/nudm-sdm/v2/imsi-001010000000001/am-data"}},
					{"sdm-sub-002", "https://smf.example.com/sdm-cb", []string{"/nudm-sdm/v2/imsi-001010000000001/sm-data"}},
				},
			}, nil
		},
	}
	svc := NewService(db)

	subs, err := svc.GetSdmSubscriptionsForNotify(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}
	if subs[0].SubscriptionID != "sdm-sub-001" {
		t.Errorf("expected sdm-sub-001, got %s", subs[0].SubscriptionID)
	}
	if subs[0].CallbackReference != "https://amf.example.com/sdm-cb" {
		t.Errorf("expected amf callback, got %s", subs[0].CallbackReference)
	}
	if len(subs[0].MonitoredResourceURIs) != 1 {
		t.Errorf("expected 1 monitored URI, got %d", len(subs[0].MonitoredResourceURIs))
	}
	if subs[1].SubscriptionID != "sdm-sub-002" {
		t.Errorf("expected sdm-sub-002, got %s", subs[1].SubscriptionID)
	}
}

func TestGetSdmSubscriptionsForNotify_EmptyResult(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{data: [][]any{}}, nil
		},
	}
	svc := NewService(db)

	subs, err := svc.GetSdmSubscriptionsForNotify(context.Background(), testSUPI)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions, got %d", len(subs))
	}
}

func TestGetSdmSubscriptionsForNotify_InvalidSUPI(t *testing.T) {
	svc := NewService(&mockDB{})

	_, err := svc.GetSdmSubscriptionsForNotify(context.Background(), "bad-supi")
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusBadRequest)
}

func TestGetSdmSubscriptionsForNotify_QueryError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	svc := NewService(db)

	_, err := svc.GetSdmSubscriptionsForNotify(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

func TestGetSdmSubscriptionsForNotify_ScanError(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data:   [][]any{{"sdm-sub-001", "https://amf.example.com/cb", []string{"/am-data"}}},
				scanFn: func(_ ...any) error { return fmt.Errorf("scan failure") },
			}, nil
		},
	}
	svc := NewService(db)

	_, err := svc.GetSdmSubscriptionsForNotify(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}

func TestGetSdmSubscriptionsForNotify_RowsErr(t *testing.T) {
	db := &mockDB{
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{
				data:   [][]any{},
				errVal: fmt.Errorf("stream error"),
			}, nil
		},
	}
	svc := NewService(db)

	_, err := svc.GetSdmSubscriptionsForNotify(context.Background(), testSUPI)
	if err == nil {
		t.Fatal("expected error")
	}
	assertProblemStatus(t, err, http.StatusInternalServerError)
}
