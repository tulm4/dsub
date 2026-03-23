package ssau

// 3GPP API conformance tests for Nudm_SSAU (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_SSAU

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_Authorize_Returns200 verifies that POST on
// /{ueIdentity}/{serviceType}/authorize returns HTTP 200 per TS 29.503.
func TestConformance_Authorize_Returns200(t *testing.T) {
	svc := &handlerMockService{
		authorizeFn: func(_ context.Context, _ string, _ string, _ *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error) {
			return &ServiceSpecificAuthorizationData{
				AuthID:            "auth-001",
				AuthorizationUeID: json.RawMessage(`{"gpsi":"msisdn-12025551234"}`),
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"ims","afId":"af-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/authorize",
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

// TestConformance_Authorize_BadRequest_Returns400 verifies that an invalid
// request body returns 400 with ProblemDetails per TS 29.503.
func TestConformance_Authorize_BadRequest_Returns400(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/authorize",
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

// TestConformance_Remove_Returns204 verifies that POST on
// /{ueIdentity}/{serviceType}/remove returns HTTP 204 per TS 29.503.
func TestConformance_Remove_Returns204(t *testing.T) {
	svc := &handlerMockService{
		removeFn: func(_ context.Context, _ string, _ string, _ *ServiceSpecificAuthorizationRemoveData) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	body := `{"authId":"auth-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/remove",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_Remove_NotFound_Returns404 verifies that removal of a
// non-existent authorization returns 404 with ProblemDetails.
func TestConformance_Remove_NotFound_Returns404(t *testing.T) {
	svc := &handlerMockService{
		removeFn: func(_ context.Context, _ string, _ string, _ *ServiceSpecificAuthorizationRemoveData) error {
			return errors.NewNotFound("authorization not found", errors.CauseContextNotFound)
		},
	}
	mux := newTestMux(svc)

	body := `{"authId":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/remove",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
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
