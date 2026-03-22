package sdm

// HTTP handler tests for the Nudm_SDM service.
//
// Based on: docs/sbi-api-design.md §3.2 (SDM Endpoints)
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
	getAmDataFn          func(ctx context.Context, supi string) (*AccessAndMobilitySubscriptionData, error)
	getSmDataFn          func(ctx context.Context, supi string) ([]SessionManagementSubscriptionData, error)
	getSmfSelDataFn      func(ctx context.Context, supi string) (*SmfSelectionSubscriptionData, error)
	getNSSAIFn           func(ctx context.Context, supi string) (*Nssai, error)
	getSmsDataFn         func(ctx context.Context, supi string) (*SmsSubscriptionData, error)
	getSmsMngtDataFn     func(ctx context.Context, supi string) (*SmsManagementSubscriptionData, error)
	getUeCtxInAmfDataFn  func(ctx context.Context, supi string) (*UeContextInAmfData, error)
	getUeCtxInSmfDataFn  func(ctx context.Context, supi string) (*UeContextInSmfData, error)
	getUeCtxInSmsfDataFn func(ctx context.Context, supi string) (*UeContextInSmsfData, error)
	getTraceConfigDataFn func(ctx context.Context, supi string) (*TraceData, error)
	getDataSetsFn        func(ctx context.Context, supi string, datasetNames []string) (*SubscriptionDataSets, error)
	getIdTranslationFn   func(ctx context.Context, ueID string) (*IdTranslationResult, error)
	subscribeFn          func(ctx context.Context, ueID string, sub *SdmSubscription) (*SdmSubscription, error)
	modifySubscriptionFn func(ctx context.Context, ueID, subscriptionID string, patch *SdmSubscription) (*SdmSubscription, error)
	unsubscribeFn        func(ctx context.Context, ueID, subscriptionID string) error
}

func (m *mockService) GetAmData(ctx context.Context, supi string) (*AccessAndMobilitySubscriptionData, error) {
	if m.getAmDataFn != nil {
		return m.getAmDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmData(ctx context.Context, supi string) ([]SessionManagementSubscriptionData, error) {
	if m.getSmDataFn != nil {
		return m.getSmDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmfSelData(ctx context.Context, supi string) (*SmfSelectionSubscriptionData, error) {
	if m.getSmfSelDataFn != nil {
		return m.getSmfSelDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetNSSAI(ctx context.Context, supi string) (*Nssai, error) {
	if m.getNSSAIFn != nil {
		return m.getNSSAIFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmsData(ctx context.Context, supi string) (*SmsSubscriptionData, error) {
	if m.getSmsDataFn != nil {
		return m.getSmsDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmsMngtData(ctx context.Context, supi string) (*SmsManagementSubscriptionData, error) {
	if m.getSmsMngtDataFn != nil {
		return m.getSmsMngtDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetUeCtxInAmfData(ctx context.Context, supi string) (*UeContextInAmfData, error) {
	if m.getUeCtxInAmfDataFn != nil {
		return m.getUeCtxInAmfDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetUeCtxInSmfData(ctx context.Context, supi string) (*UeContextInSmfData, error) {
	if m.getUeCtxInSmfDataFn != nil {
		return m.getUeCtxInSmfDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetUeCtxInSmsfData(ctx context.Context, supi string) (*UeContextInSmsfData, error) {
	if m.getUeCtxInSmsfDataFn != nil {
		return m.getUeCtxInSmsfDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetTraceConfigData(ctx context.Context, supi string) (*TraceData, error) {
	if m.getTraceConfigDataFn != nil {
		return m.getTraceConfigDataFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetDataSets(ctx context.Context, supi string, datasetNames []string) (*SubscriptionDataSets, error) {
	if m.getDataSetsFn != nil {
		return m.getDataSetsFn(ctx, supi, datasetNames)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetIdTranslation(ctx context.Context, ueID string) (*IdTranslationResult, error) {
	if m.getIdTranslationFn != nil {
		return m.getIdTranslationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Subscribe(ctx context.Context, ueID string, sub *SdmSubscription) (*SdmSubscription, error) {
	if m.subscribeFn != nil {
		return m.subscribeFn(ctx, ueID, sub)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) ModifySubscription(ctx context.Context, ueID, subscriptionID string, patch *SdmSubscription) (*SdmSubscription, error) {
	if m.modifySubscriptionFn != nil {
		return m.modifySubscriptionFn(ctx, ueID, subscriptionID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Unsubscribe(ctx context.Context, ueID, subscriptionID string) error {
	if m.unsubscribeFn != nil {
		return m.unsubscribeFn(ctx, ueID, subscriptionID)
	}
	return errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *mockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func TestHandleGetAmData_Success(t *testing.T) {
	rfsp := 10
	svc := &mockService{
		getAmDataFn: func(_ context.Context, supi string) (*AccessAndMobilitySubscriptionData, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supi)
			}
			return &AccessAndMobilitySubscriptionData{
				Gpsis:             []string{"msisdn-12025551234"},
				SubscribedDnnList: []string{"internet", "ims"},
				RfspIndex:         &rfsp,
				MicoAllowed:       true,
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/am-data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result AccessAndMobilitySubscriptionData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Gpsis) != 1 || result.Gpsis[0] != "msisdn-12025551234" {
		t.Errorf("unexpected gpsis: %v", result.Gpsis)
	}
	if result.RfspIndex == nil || *result.RfspIndex != 10 {
		t.Errorf("unexpected rfspIndex: %v", result.RfspIndex)
	}
}

func TestHandleGetAmData_NotFound(t *testing.T) {
	svc := &mockService{
		getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
			return nil, errors.NewNotFound("AM data not found", errors.CauseDataNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/am-data", nil)
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

func TestHandleGetAmData_InvalidSUPI(t *testing.T) {
	svc := &mockService{
		getAmDataFn: func(_ context.Context, _ string) (*AccessAndMobilitySubscriptionData, error) {
			return nil, errors.NewBadRequest("invalid SUPI", errors.CauseMandatoryIEIncorrect)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/bad-supi/am-data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleGetSmData_Success(t *testing.T) {
	svc := &mockService{
		getSmDataFn: func(_ context.Context, supi string) ([]SessionManagementSubscriptionData, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supi)
			}
			return []SessionManagementSubscriptionData{
				{
					SingleNssai:       json.RawMessage(`{"sst":1,"sd":"000001"}`),
					DnnConfigurations: json.RawMessage(`{"internet":{"pduSessionTypes":{"defaultSessionType":"IPV4V6"}}}`),
				},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/sm-data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []SessionManagementSubscriptionData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 SM data entry, got %d", len(result))
	}
}

func TestHandleGetNSSAI_Success(t *testing.T) {
	svc := &mockService{
		getNSSAIFn: func(_ context.Context, supi string) (*Nssai, error) {
			return &Nssai{
				DefaultSingleNssais: json.RawMessage(`[{"sst":1}]`),
				SingleNssais:        json.RawMessage(`[{"sst":1},{"sst":2}]`),
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/nssai", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result Nssai
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.DefaultSingleNssais) == 0 {
		t.Error("expected non-empty defaultSingleNssais")
	}
}

func TestHandleSubscribe_Success(t *testing.T) {
	svc := &mockService{
		subscribeFn: func(_ context.Context, ueID string, sub *SdmSubscription) (*SdmSubscription, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			sub.SubscriptionID = "sub-001"
			return sub, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"nfInstanceId":"amf-001","callbackReference":"https://amf.example.com/callback","monitoredResourceUris":["/nudm-sdm/v2/imsi-001010000000001/am-data"]}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result SdmSubscription
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SubscriptionID != "sub-001" {
		t.Errorf("expected subscriptionId sub-001, got %s", result.SubscriptionID)
	}
	if result.NfInstanceID != "amf-001" {
		t.Errorf("expected nfInstanceId amf-001, got %s", result.NfInstanceID)
	}
}

func TestHandleSubscribe_BadRequestBody(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions",
		bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleUnsubscribe_Success(t *testing.T) {
	svc := &mockService{
		unsubscribeFn: func(_ context.Context, ueID, subscriptionID string) error {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if subscriptionID != "sub-001" {
				t.Errorf("unexpected subscriptionId: %s", subscriptionID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions/sub-001", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUnsubscribe_NotFound(t *testing.T) {
	svc := &mockService{
		unsubscribeFn: func(_ context.Context, _, _ string) error {
			return errors.NewNotFound("subscription not found", errors.CauseSubscriptionNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleGetIdTranslation_Success(t *testing.T) {
	svc := &mockService{
		getIdTranslationFn: func(_ context.Context, ueID string) (*IdTranslationResult, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			return &IdTranslationResult{
				Supi: "imsi-001010000000001",
				Gpsi: "msisdn-12025551234",
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/id-translation-result", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result IdTranslationResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Supi != "imsi-001010000000001" {
		t.Errorf("expected supi imsi-001010000000001, got %s", result.Supi)
	}
	if result.Gpsi != "msisdn-12025551234" {
		t.Errorf("expected gpsi msisdn-12025551234, got %s", result.Gpsi)
	}
}

func TestHandleGetDataSets_Success(t *testing.T) {
	rfsp := 10
	svc := &mockService{
		getDataSetsFn: func(_ context.Context, supi string, names []string) (*SubscriptionDataSets, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supi)
			}
			if len(names) != 2 {
				t.Errorf("expected 2 dataset names, got %d", len(names))
			}
			return &SubscriptionDataSets{
				AmData: &AccessAndMobilitySubscriptionData{
					RfspIndex: &rfsp,
				},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001?dataset-names=AM,SMF_SEL", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result SubscriptionDataSets
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AmData == nil {
		t.Fatal("expected amData in response, got nil")
	}
}

func TestHandleGetDataSets_MissingQueryParam(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleAcknowledge_Success(t *testing.T) {
	mux := newTestMux(&mockService{})

	body := `{"sorMacIue":"aabbccdd"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-sdm/v2/imsi-001010000000001/am-data/sor-ack",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAcknowledge_BadBody(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPut,
		"/nudm-sdm/v2/imsi-001010000000001/am-data/sor-ack",
		bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	assertProblemDetailsContentType(t, w)
}

func TestRouteNotFound(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/nonexistent-path", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotImplementedEndpoint(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-sdm/v2/imsi-001010000000001/v2x-data", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"/", 0},
		{"a/b/c", 3},
		{"/a/b/c/", 3},
		{"imsi-001/am-data", 2},
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
		{[]string{"imsi-001", "am-data"}, "*/am-data", true},
		{[]string{"imsi-001", "sm-data"}, "*/sm-data", true},
		{[]string{"imsi-001", "sdm-subscriptions"}, "*/sdm-subscriptions", true},
		{[]string{"imsi-001", "sdm-subscriptions", "sub-1"}, "*/sdm-subscriptions/*", true},
		{[]string{"imsi-001", "id-translation-result"}, "*/id-translation-result", true},
		{[]string{"shared-data", "sd-1"}, "shared-data/*", true},
		{[]string{"imsi-001", "wrong"}, "*/am-data", false},
		{[]string{"imsi-001"}, "*/am-data", false},
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
