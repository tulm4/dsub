package uecm

// HTTP handler tests for the Nudm_UECM service.
//
// Based on: docs/sbi-api-design.md §3.3 (UECM Endpoints)
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
	register3GppAccessFn         func(ctx context.Context, ueID string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error)
	get3GppRegistrationFn        func(ctx context.Context, ueID string) (*Amf3GppAccessRegistration, error)
	update3GppRegistrationFn     func(ctx context.Context, ueID string, patch *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, error)
	deregAMFFn                   func(ctx context.Context, ueID string, data *DeregistrationData) error
	peiUpdateFn                  func(ctx context.Context, ueID string, info *PeiUpdateInfo) error
	updateRoamingInformationFn   func(ctx context.Context, ueID string, info *RoamingInfoUpdate) error
	registerNon3GppAccessFn      func(ctx context.Context, ueID string, reg *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, bool, error)
	getNon3GppRegistrationFn     func(ctx context.Context, ueID string) (*AmfNon3GppAccessRegistration, error)
	updateNon3GppRegistrationFn  func(ctx context.Context, ueID string, patch *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, error)
	registerSmfFn                func(ctx context.Context, ueID string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error)
	getSmfRegistrationFn         func(ctx context.Context, ueID string) ([]SmfRegistration, error)
	retrieveSmfRegistrationFn    func(ctx context.Context, ueID string, pduSessionID int) (*SmfRegistration, error)
	updateSmfRegistrationFn      func(ctx context.Context, ueID string, pduSessionID int, patch *SmfRegistration) (*SmfRegistration, error)
	deregisterSmfFn              func(ctx context.Context, ueID string, pduSessionID int) error
	registerSmsf3GppFn           func(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error)
	getSmsf3GppRegistrationFn    func(ctx context.Context, ueID string) (*SmsfRegistration, error)
	updateSmsf3GppRegistrationFn func(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error)
	deregisterSmsf3GppFn         func(ctx context.Context, ueID string) error
	registerSmsfNon3GppFn        func(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error)
	getSmsfNon3GppRegistrationFn func(ctx context.Context, ueID string) (*SmsfRegistration, error)
	updateSmsfNon3GppRegFn       func(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error)
	deregisterSmsfNon3GppFn      func(ctx context.Context, ueID string) error
	getRegistrationsFn           func(ctx context.Context, ueID string) (*RegistrationDataSets, error)
	sendRoutingInfoSmFn          func(ctx context.Context, ueID string, req *RoutingInfoSmRequest) (*RoutingInfoSmResponse, error)
}

func (m *mockService) Register3GppAccess(ctx context.Context, ueID string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
	if m.register3GppAccessFn != nil {
		return m.register3GppAccessFn(ctx, ueID, reg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Get3GppRegistration(ctx context.Context, ueID string) (*Amf3GppAccessRegistration, error) {
	if m.get3GppRegistrationFn != nil {
		return m.get3GppRegistrationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) Update3GppRegistration(ctx context.Context, ueID string, patch *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, error) {
	if m.update3GppRegistrationFn != nil {
		return m.update3GppRegistrationFn(ctx, ueID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeregAMF(ctx context.Context, ueID string, data *DeregistrationData) error {
	if m.deregAMFFn != nil {
		return m.deregAMFFn(ctx, ueID, data)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) PeiUpdate(ctx context.Context, ueID string, info *PeiUpdateInfo) error {
	if m.peiUpdateFn != nil {
		return m.peiUpdateFn(ctx, ueID, info)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateRoamingInformation(ctx context.Context, ueID string, info *RoamingInfoUpdate) error {
	if m.updateRoamingInformationFn != nil {
		return m.updateRoamingInformationFn(ctx, ueID, info)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) RegisterNon3GppAccess(ctx context.Context, ueID string, reg *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, bool, error) {
	if m.registerNon3GppAccessFn != nil {
		return m.registerNon3GppAccessFn(ctx, ueID, reg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetNon3GppRegistration(ctx context.Context, ueID string) (*AmfNon3GppAccessRegistration, error) {
	if m.getNon3GppRegistrationFn != nil {
		return m.getNon3GppRegistrationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateNon3GppRegistration(ctx context.Context, ueID string, patch *AmfNon3GppAccessRegistration) (*AmfNon3GppAccessRegistration, error) {
	if m.updateNon3GppRegistrationFn != nil {
		return m.updateNon3GppRegistrationFn(ctx, ueID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) RegisterSmf(ctx context.Context, ueID string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error) {
	if m.registerSmfFn != nil {
		return m.registerSmfFn(ctx, ueID, pduSessionID, reg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmfRegistration(ctx context.Context, ueID string) ([]SmfRegistration, error) {
	if m.getSmfRegistrationFn != nil {
		return m.getSmfRegistrationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) RetrieveSmfRegistration(ctx context.Context, ueID string, pduSessionID int) (*SmfRegistration, error) {
	if m.retrieveSmfRegistrationFn != nil {
		return m.retrieveSmfRegistrationFn(ctx, ueID, pduSessionID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateSmfRegistration(ctx context.Context, ueID string, pduSessionID int, patch *SmfRegistration) (*SmfRegistration, error) {
	if m.updateSmfRegistrationFn != nil {
		return m.updateSmfRegistrationFn(ctx, ueID, pduSessionID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeregisterSmf(ctx context.Context, ueID string, pduSessionID int) error {
	if m.deregisterSmfFn != nil {
		return m.deregisterSmfFn(ctx, ueID, pduSessionID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) RegisterSmsf3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
	if m.registerSmsf3GppFn != nil {
		return m.registerSmsf3GppFn(ctx, ueID, reg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmsf3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error) {
	if m.getSmsf3GppRegistrationFn != nil {
		return m.getSmsf3GppRegistrationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateSmsf3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error) {
	if m.updateSmsf3GppRegistrationFn != nil {
		return m.updateSmsf3GppRegistrationFn(ctx, ueID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeregisterSmsf3Gpp(ctx context.Context, ueID string) error {
	if m.deregisterSmsf3GppFn != nil {
		return m.deregisterSmsf3GppFn(ctx, ueID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) RegisterSmsfNon3Gpp(ctx context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
	if m.registerSmsfNon3GppFn != nil {
		return m.registerSmsfNon3GppFn(ctx, ueID, reg)
	}
	return nil, false, errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetSmsfNon3GppRegistration(ctx context.Context, ueID string) (*SmsfRegistration, error) {
	if m.getSmsfNon3GppRegistrationFn != nil {
		return m.getSmsfNon3GppRegistrationFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateSmsfNon3GppRegistration(ctx context.Context, ueID string, patch *SmsfRegistration) (*SmsfRegistration, error) {
	if m.updateSmsfNon3GppRegFn != nil {
		return m.updateSmsfNon3GppRegFn(ctx, ueID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeregisterSmsfNon3Gpp(ctx context.Context, ueID string) error {
	if m.deregisterSmsfNon3GppFn != nil {
		return m.deregisterSmsfNon3GppFn(ctx, ueID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetRegistrations(ctx context.Context, ueID string) (*RegistrationDataSets, error) {
	if m.getRegistrationsFn != nil {
		return m.getRegistrationsFn(ctx, ueID)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) SendRoutingInfoSm(ctx context.Context, ueID string, req *RoutingInfoSmRequest) (*RoutingInfoSmResponse, error) {
	if m.sendRoutingInfoSmFn != nil {
		return m.sendRoutingInfoSmFn(ctx, ueID, req)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *mockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// --- AMF 3GPP Access Tests ---

func TestHandleRegister3GppAccess_Created(t *testing.T) {
	svc := &mockService{
		register3GppAccessFn: func(_ context.Context, ueID string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if reg.AmfInstanceID != "amf-001" {
				t.Errorf("unexpected amfInstanceId: %s", reg.AmfInstanceID)
			}
			return reg, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"amfInstanceId":"amf-001","deregCallbackUri":"https://amf.example.com/dereg","guami":{"plmnId":{"mcc":"001","mnc":"01"}},"ratType":"NR"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result Amf3GppAccessRegistration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AmfInstanceID != "amf-001" {
		t.Errorf("expected amfInstanceId amf-001, got %s", result.AmfInstanceID)
	}
}

func TestHandleRegister3GppAccess_Updated(t *testing.T) {
	svc := &mockService{
		register3GppAccessFn: func(_ context.Context, _ string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
			return reg, false, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"amfInstanceId":"amf-002","deregCallbackUri":"https://amf2.example.com/dereg","guami":{},"ratType":"NR"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGet3GppRegistration_Success(t *testing.T) {
	svc := &mockService{
		get3GppRegistrationFn: func(_ context.Context, ueID string) (*Amf3GppAccessRegistration, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			return &Amf3GppAccessRegistration{
				AmfInstanceID:    "amf-001",
				DeregCallbackURI: "https://amf.example.com/dereg",
				RatType:          "NR",
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result Amf3GppAccessRegistration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AmfInstanceID != "amf-001" {
		t.Errorf("expected amfInstanceId amf-001, got %s", result.AmfInstanceID)
	}
}

func TestHandleGet3GppRegistration_NotFound(t *testing.T) {
	svc := &mockService{
		get3GppRegistrationFn: func(_ context.Context, _ string) (*Amf3GppAccessRegistration, error) {
			return nil, errors.NewNotFound("registration not found", errors.CauseContextNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access", nil)
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
	if pd.Cause != errors.CauseContextNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseContextNotFound, pd.Cause)
	}
}

func TestHandleRegister3GppAccess_InvalidUeID(t *testing.T) {
	svc := &mockService{
		register3GppAccessFn: func(_ context.Context, _ string, _ *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
			return nil, false, errors.NewBadRequest("invalid ueId", errors.CauseMandatoryIEIncorrect)
		},
	}
	mux := newTestMux(svc)

	body := `{"amfInstanceId":"amf-001","deregCallbackUri":"https://amf.example.com/dereg","guami":{},"ratType":"NR"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/bad-id/registrations/amf-3gpp-access",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- DeregAMF Test ---

func TestHandleDeregAMF_Success(t *testing.T) {
	svc := &mockService{
		deregAMFFn: func(_ context.Context, ueID string, data *DeregistrationData) error {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if data.DeregReason != "UE_INITIAL_REGISTRATION" {
				t.Errorf("unexpected deregReason: %s", data.DeregReason)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	body := `{"deregReason":"UE_INITIAL_REGISTRATION"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access/dereg-amf",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- SMF Registration Tests ---

func TestHandleRegisterSmf_Created(t *testing.T) {
	svc := &mockService{
		registerSmfFn: func(_ context.Context, ueID string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if pduSessionID != 5 {
				t.Errorf("unexpected pduSessionId: %d", pduSessionID)
			}
			if reg.SmfInstanceID != "smf-001" {
				t.Errorf("unexpected smfInstanceId: %s", reg.SmfInstanceID)
			}
			reg.PduSessionID = pduSessionID
			return reg, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"smfInstanceId":"smf-001","dnn":"internet","singleNssai":{"sst":1},"plmnId":{"mcc":"001","mnc":"01"},"pduSessionId":5}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations/5",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result SmfRegistration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SmfInstanceID != "smf-001" {
		t.Errorf("expected smfInstanceId smf-001, got %s", result.SmfInstanceID)
	}
}

func TestHandleRegisterSmf_Updated(t *testing.T) {
	svc := &mockService{
		registerSmfFn: func(_ context.Context, _ string, _ int, reg *SmfRegistration) (*SmfRegistration, bool, error) {
			return reg, false, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"smfInstanceId":"smf-001","dnn":"internet","singleNssai":{"sst":1},"plmnId":{"mcc":"001","mnc":"01"}}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations/5",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeregisterSmf_Success(t *testing.T) {
	svc := &mockService{
		deregisterSmfFn: func(_ context.Context, ueID string, pduSessionID int) error {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if pduSessionID != 5 {
				t.Errorf("unexpected pduSessionId: %d", pduSessionID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations/5", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeregisterSmf_InvalidPduSessionID(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- SMSF 3GPP Registration Tests ---

func TestHandleRegisterSmsf3Gpp_Created(t *testing.T) {
	svc := &mockService{
		registerSmsf3GppFn: func(_ context.Context, ueID string, reg *SmsfRegistration) (*SmsfRegistration, bool, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			if reg.SmsfInstanceID != "smsf-001" {
				t.Errorf("unexpected smsfInstanceId: %s", reg.SmsfInstanceID)
			}
			return reg, true, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"smsfInstanceId":"smsf-001","plmnId":{"mcc":"001","mnc":"01"}}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smsf-3gpp-access",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result SmsfRegistration
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SmsfInstanceID != "smsf-001" {
		t.Errorf("expected smsfInstanceId smsf-001, got %s", result.SmsfInstanceID)
	}
}

func TestHandleDeregisterSmsf3Gpp_Success(t *testing.T) {
	svc := &mockService{
		deregisterSmsf3GppFn: func(_ context.Context, ueID string) error {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smsf-3gpp-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- GetRegistrations Test ---

func TestHandleGetRegistrations_Success(t *testing.T) {
	svc := &mockService{
		getRegistrationsFn: func(_ context.Context, ueID string) (*RegistrationDataSets, error) {
			if ueID != "imsi-001010000000001" {
				t.Errorf("unexpected ueID: %s", ueID)
			}
			return &RegistrationDataSets{
				Amf3GppAccess: &Amf3GppAccessRegistration{
					AmfInstanceID: "amf-001",
					RatType:       "NR",
				},
				SmfRegistrations: []SmfRegistration{
					{SmfInstanceID: "smf-001", PduSessionID: 5, Dnn: "internet"},
				},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result RegistrationDataSets
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Amf3GppAccess == nil {
		t.Fatal("expected amf3GppAccess in response, got nil")
	}
	if result.Amf3GppAccess.AmfInstanceID != "amf-001" {
		t.Errorf("expected amfInstanceId amf-001, got %s", result.Amf3GppAccess.AmfInstanceID)
	}
	if len(result.SmfRegistrations) != 1 {
		t.Fatalf("expected 1 SMF registration, got %d", len(result.SmfRegistrations))
	}
}

// --- Not Implemented / Route Not Found ---

func TestHandleNotImplemented_IpSmGw(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/ip-sm-gw", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestRouteNotFound(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRegister3GppAccess_BadRequestBody(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access",
		bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
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
		{"imsi-001/registrations/amf-3gpp-access", 3},
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
		{[]string{"imsi-001", "registrations"}, "*/registrations", true},
		{[]string{"imsi-001", "registrations", "amf-3gpp-access"}, "*/registrations/amf-3gpp-access", true},
		{[]string{"imsi-001", "registrations", "smf-registrations", "5"}, "*/registrations/smf-registrations/*", true},
		{[]string{"imsi-001", "registrations", "amf-3gpp-access", "dereg-amf"}, "*/registrations/amf-3gpp-access/dereg-amf", true},
		{[]string{"imsi-001", "registrations", "wrong"}, "*/registrations/amf-3gpp-access", false},
		{[]string{"imsi-001"}, "*/registrations", false},
		{[]string{"restore-pcscf"}, "*/registrations", false},
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
