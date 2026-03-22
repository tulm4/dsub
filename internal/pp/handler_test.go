package pp

// HTTP handler tests for the Nudm_PP service.
//
// Based on: docs/sbi-api-design.md §3.5 (PP Endpoints)
// Based on: docs/sbi-api-design.md §7 (Error Handling)

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// mockService implements ServiceInterface for handler tests.
type mockService struct {
	getPPDataFn                func(ctx context.Context, ueID string) (*PpData, error)
	updatePPDataFn             func(ctx context.Context, ueID string, patch *PpData) (*PpData, error)
	create5GVnGroupFn          func(ctx context.Context, extGroupID string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error)
	get5GVnGroupFn             func(ctx context.Context, extGroupID string) (*VnGroupConfiguration, error)
	modify5GVnGroupFn          func(ctx context.Context, extGroupID string, patch *VnGroupConfiguration) (*VnGroupConfiguration, error)
	delete5GVnGroupFn          func(ctx context.Context, extGroupID string) error
	createMbsGroupMembershipFn func(ctx context.Context, extGroupID string, memb *MbsGroupMemb) (*MbsGroupMemb, bool, error)
	getMbsGroupMembershipFn    func(ctx context.Context, extGroupID string) (*MbsGroupMemb, error)
	modifyMbsGroupMembershipFn func(ctx context.Context, extGroupID string, patch *MbsGroupMemb) (*MbsGroupMemb, error)
	deleteMbsGroupMembershipFn func(ctx context.Context, extGroupID string) error
}

func (m *mockService) GetPPData(ctx context.Context, ueID string) (*PpData, error) {
	if m.getPPDataFn != nil {
		return m.getPPDataFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdatePPData(ctx context.Context, ueID string, patch *PpData) (*PpData, error) {
	if m.updatePPDataFn != nil {
		return m.updatePPDataFn(ctx, ueID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Create5GVnGroup(ctx context.Context, extGroupID string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error) {
	if m.create5GVnGroupFn != nil {
		return m.create5GVnGroupFn(ctx, extGroupID, cfg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Get5GVnGroup(ctx context.Context, extGroupID string) (*VnGroupConfiguration, error) {
	if m.get5GVnGroupFn != nil {
		return m.get5GVnGroupFn(ctx, extGroupID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Modify5GVnGroup(ctx context.Context, extGroupID string, patch *VnGroupConfiguration) (*VnGroupConfiguration, error) {
	if m.modify5GVnGroupFn != nil {
		return m.modify5GVnGroupFn(ctx, extGroupID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Delete5GVnGroup(ctx context.Context, extGroupID string) error {
	if m.delete5GVnGroupFn != nil {
		return m.delete5GVnGroupFn(ctx, extGroupID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) CreateMbsGroupMembership(ctx context.Context, extGroupID string, memb *MbsGroupMemb) (*MbsGroupMemb, bool, error) {
	if m.createMbsGroupMembershipFn != nil {
		return m.createMbsGroupMembershipFn(ctx, extGroupID, memb)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetMbsGroupMembership(ctx context.Context, extGroupID string) (*MbsGroupMemb, error) {
	if m.getMbsGroupMembershipFn != nil {
		return m.getMbsGroupMembershipFn(ctx, extGroupID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) ModifyMbsGroupMembership(ctx context.Context, extGroupID string, patch *MbsGroupMemb) (*MbsGroupMemb, error) {
	if m.modifyMbsGroupMembershipFn != nil {
		return m.modifyMbsGroupMembershipFn(ctx, extGroupID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeleteMbsGroupMembership(ctx context.Context, extGroupID string) error {
	if m.deleteMbsGroupMembershipFn != nil {
		return m.deleteMbsGroupMembershipFn(ctx, extGroupID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSdmSubscriptionsForNotify(_ context.Context, _ string) ([]SdmSubscriptionInfo, error) {
	return nil, errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *mockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// --- GetPPData Tests ---

func TestHandleGetPPData_Success(t *testing.T) {
	count := 10
	svc := &mockService{
		getPPDataFn: func(_ context.Context, ueID string) (*PpData, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			return &PpData{
				SupportedFeatures: "abc",
				PpDlPacketCount:   &count,
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-001010000000001/pp-data", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result PpData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SupportedFeatures != "abc" {
		t.Errorf("expected supportedFeatures abc, got %s", result.SupportedFeatures)
	}
	if result.PpDlPacketCount == nil || *result.PpDlPacketCount != 10 {
		t.Errorf("expected ppDlPacketCount 10, got %v", result.PpDlPacketCount)
	}
}

func TestHandleGetPPData_NotFound(t *testing.T) {
	svc := &mockService{
		getPPDataFn: func(_ context.Context, _ string) (*PpData, error) {
			return nil, errors.NewNotFound("pp data not found", errors.CauseDataNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-001010000000001/pp-data", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != errors.CauseDataNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseDataNotFound, pd.Cause)
	}
}

func TestHandleGetPPData_ServiceError(t *testing.T) {
	svc := &mockService{
		getPPDataFn: func(_ context.Context, _ string) (*PpData, error) {
			return nil, errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-001010000000001/pp-data", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- UpdatePPData Tests ---

func TestHandleUpdatePPData_Success(t *testing.T) {
	count := 5
	svc := &mockService{
		updatePPDataFn: func(_ context.Context, ueID string, patch *PpData) (*PpData, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if patch.SupportedFeatures != "def" {
				t.Errorf("unexpected supportedFeatures: %s", patch.SupportedFeatures)
			}
			return &PpData{
				SupportedFeatures: patch.SupportedFeatures,
				PpDlPacketCount:   &count,
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"supportedFeatures":"def"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/imsi-001010000000001/pp-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result PpData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SupportedFeatures != "def" {
		t.Errorf("expected supportedFeatures def, got %s", result.SupportedFeatures)
	}
}

func TestHandleUpdatePPData_InvalidBody(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/imsi-001010000000001/pp-data",
		bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleUpdatePPData_EmptyBody(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/imsi-001010000000001/pp-data", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleUpdatePPData_ServiceError(t *testing.T) {
	svc := &mockService{
		updatePPDataFn: func(_ context.Context, _ string, _ *PpData) (*PpData, error) {
			return nil, errors.NewBadRequest("invalid ueId", errors.CauseMandatoryIEIncorrect)
		},
	}
	mux := newTestMux(svc)

	body := `{"supportedFeatures":"abc"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/imsi-001010000000001/pp-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- 5G VN Group Handler Tests ---

func TestHandleCreate5GVnGroup_Success(t *testing.T) {
	svc := &mockService{
		create5GVnGroupFn: func(_ context.Context, extGroupID string, cfg *VnGroupConfiguration) (*VnGroupConfiguration, bool, error) {
			if extGroupID != "group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return cfg, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"internet","members":["msisdn-12025551234"]}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-pp/v1/5g-vn-groups/group-1",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result VnGroupConfiguration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Dnn != "internet" {
		t.Errorf("expected dnn 'internet', got %s", result.Dnn)
	}
}

func TestHandleGet5GVnGroup_Success(t *testing.T) {
	svc := &mockService{
		get5GVnGroupFn: func(_ context.Context, extGroupID string) (*VnGroupConfiguration, error) {
			if extGroupID != "group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return &VnGroupConfiguration{
				Dnn:     "internet",
				Members: []string{"msisdn-12025551234"},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/5g-vn-groups/group-1", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result VnGroupConfiguration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Dnn != "internet" {
		t.Errorf("expected dnn 'internet', got %s", result.Dnn)
	}
}

func TestHandleGet5GVnGroup_NotFound(t *testing.T) {
	svc := &mockService{
		get5GVnGroupFn: func(_ context.Context, _ string) (*VnGroupConfiguration, error) {
			return nil, errors.NewNotFound("not found", errors.CauseDataNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/5g-vn-groups/group-1", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleModify5GVnGroup_Success(t *testing.T) {
	svc := &mockService{
		modify5GVnGroupFn: func(_ context.Context, extGroupID string, patch *VnGroupConfiguration) (*VnGroupConfiguration, error) {
			if extGroupID != "group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return &VnGroupConfiguration{
				Dnn:     patch.Dnn,
				Members: []string{"msisdn-12025551234"},
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"new-dnn"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-pp/v1/5g-vn-groups/group-1",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result VnGroupConfiguration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Dnn != "new-dnn" {
		t.Errorf("expected dnn 'new-dnn', got %s", result.Dnn)
	}
}

func TestHandleDelete5GVnGroup_Success(t *testing.T) {
	svc := &mockService{
		delete5GVnGroupFn: func(_ context.Context, extGroupID string) error {
			if extGroupID != "group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-pp/v1/5g-vn-groups/group-1", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- MBS Group Membership Handler Tests ---

func TestHandleCreateMbsGroupMembership_Success(t *testing.T) {
	svc := &mockService{
		createMbsGroupMembershipFn: func(_ context.Context, extGroupID string, memb *MbsGroupMemb) (*MbsGroupMemb, bool, error) {
			if extGroupID != "mbs-group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return memb, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"afInstanceId":"af-1"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-pp/v1/mbs-group-membership/mbs-group-1",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result MbsGroupMemb
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AfInstanceId != "af-1" {
		t.Errorf("expected afInstanceId 'af-1', got %s", result.AfInstanceId)
	}
}

func TestHandleGetMbsGroupMembership_Success(t *testing.T) {
	svc := &mockService{
		getMbsGroupMembershipFn: func(_ context.Context, extGroupID string) (*MbsGroupMemb, error) {
			if extGroupID != "mbs-group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return &MbsGroupMemb{
				AfInstanceId: "af-1",
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/mbs-group-membership/mbs-group-1", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result MbsGroupMemb
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AfInstanceId != "af-1" {
		t.Errorf("expected afInstanceId 'af-1', got %s", result.AfInstanceId)
	}
}

func TestHandleDeleteMbsGroupMembership_Success(t *testing.T) {
	svc := &mockService{
		deleteMbsGroupMembershipFn: func(_ context.Context, extGroupID string) error {
			if extGroupID != "mbs-group-1" {
				t.Errorf("unexpected extGroupID: %s", extGroupID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-pp/v1/mbs-group-membership/mbs-group-1", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Endpoint Not Found ---

func TestHandleEndpointNotFound(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-pp/v1/imsi-001010000000001/unknown", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- Utility Tests ---

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"/", 0},
		{"a/b/c", 3},
		{"/a/b/c/", 3},
		{"imsi-001/pp-data", 2},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := splitPath(tc.input)
			if len(result) != tc.expected {
				t.Errorf("splitPath(%q): expected %d segments, got %d (%v)", tc.input, tc.expected, len(result), result)
			}
		})
	}
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		segments []string
		pattern  string
		expected bool
	}{
		{[]string{"imsi-001", "pp-data"}, "*/pp-data", true},
		{[]string{"imsi-001", "pp-data", "extra"}, "*/pp-data", false},
		{[]string{"imsi-001"}, "*/pp-data", false},
		{[]string{"imsi-001", "wrong"}, "*/pp-data", false},
	}

	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			result := matchPath(tc.segments, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchPath(%v, %q): got %v, want %v", tc.segments, tc.pattern, result, tc.expected)
			}
		})
	}
}

// assertProblemDetailsContentType verifies the Content-Type is application/problem+json.
func assertProblemDetailsContentType(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	ct := w.Header().Get("Content-Type")
	expected := "application/problem+json"
	if ct != expected {
		t.Errorf("Content-Type: got %q, want %q", ct, expected)
	}
}
