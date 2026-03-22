package uecm

// 3GPP API conformance tests for Nudm_UECM (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_UECM

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_Register3GppAccess_Created_Returns201 verifies that a new
// AMF 3GPP access registration returns HTTP 201 per TS 29.503.
func TestConformance_Register3GppAccess_Created_Returns201(t *testing.T) {
	svc := &mockService{
		register3GppAccessFn: func(_ context.Context, _ string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
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
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_Register3GppAccess_Updated_Returns200 verifies that
// updating an existing AMF registration returns HTTP 200 per TS 29.503.
func TestConformance_Register3GppAccess_Updated_Returns200(t *testing.T) {
	svc := &mockService{
		register3GppAccessFn: func(_ context.Context, _ string, reg *Amf3GppAccessRegistration) (*Amf3GppAccessRegistration, bool, error) {
			return reg, false, nil
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

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestConformance_GetRegistration_Returns200 verifies that GET on
// amf-3gpp-access registration returns HTTP 200 per TS 29.503.
func TestConformance_GetRegistration_Returns200(t *testing.T) {
	svc := &mockService{
		get3GppRegistrationFn: func(_ context.Context, _ string) (*Amf3GppAccessRegistration, error) {
			return &Amf3GppAccessRegistration{
				AmfInstanceID: "amf-001",
				RatType:       "NR",
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_GetRegistration_NotFound_ProblemDetails verifies that
// when no registration exists, a 404 with application/problem+json and
// cause field is returned per RFC 7807.
func TestConformance_GetRegistration_NotFound_ProblemDetails(t *testing.T) {
	svc := &mockService{
		get3GppRegistrationFn: func(_ context.Context, _ string) (*Amf3GppAccessRegistration, error) {
			return nil, errors.NewNotFound("registration not found", "CONTEXT_NOT_FOUND")
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-999990000000001/registrations/amf-3gpp-access",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause == "" {
		t.Error("ProblemDetails.Cause is empty but expected CONTEXT_NOT_FOUND")
	}
}

// TestConformance_DeregAMF_Returns204 verifies that AMF deregistration
// returns HTTP 204 per TS 29.503.
func TestConformance_DeregAMF_Returns204(t *testing.T) {
	svc := &mockService{
		deregAMFFn: func(_ context.Context, _ string, _ *DeregistrationData) error {
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
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_RegisterSmf_Created_Returns201 verifies that a new SMF
// registration returns HTTP 201 per TS 29.503.
func TestConformance_RegisterSmf_Created_Returns201(t *testing.T) {
	svc := &mockService{
		registerSmfFn: func(_ context.Context, _ string, pduSessionID int, reg *SmfRegistration) (*SmfRegistration, bool, error) {
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
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

// TestConformance_DeregisterSmf_Returns204 verifies that SMF deregistration
// returns HTTP 204 per TS 29.503.
func TestConformance_DeregisterSmf_Returns204(t *testing.T) {
	svc := &mockService{
		deregisterSmfFn: func(_ context.Context, _ string, _ int) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations/5",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_BadRequestBody_Returns400 verifies that a malformed JSON
// request body returns 400 Bad Request with ProblemDetails per TS 29.503.
func TestConformance_BadRequestBody_Returns400(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusBadRequest {
		t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusBadRequest)
	}
	if pd.Title == "" {
		t.Error("ProblemDetails.Title is empty")
	}
}

// TestConformance_GetRegistrations_Returns200 verifies that the aggregated
// registrations endpoint returns HTTP 200 per TS 29.503.
func TestConformance_GetRegistrations_Returns200(t *testing.T) {
	svc := &mockService{
		getRegistrationsFn: func(_ context.Context, _ string) (*RegistrationDataSets, error) {
			return &RegistrationDataSets{
				Amf3GppAccess: &Amf3GppAccessRegistration{
					AmfInstanceID: "amf-001",
				},
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-uecm/v1/imsi-001010000000001/registrations",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_SendRoutingInfoSm_Returns200 verifies that the SMS
// routing info endpoint returns HTTP 200 per TS 29.503.
func TestConformance_SendRoutingInfoSm_Returns200(t *testing.T) {
	svc := &mockService{
		sendRoutingInfoSmFn: func(_ context.Context, _ string, _ *RoutingInfoSmRequest) (*RoutingInfoSmResponse, error) {
			return &RoutingInfoSmResponse{
				SmsfInstanceID: "smsf-001",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"supportedFeatures":"0"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-uecm/v1/imsi-001010000000001/registrations/send-routing-info-sm",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}
