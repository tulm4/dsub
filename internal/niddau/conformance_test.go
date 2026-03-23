package niddau

// 3GPP API conformance tests for Nudm_NIDDAU (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_NIDDAU

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_AuthorizeNiddData_Returns200 verifies that POST on
// /{ueIdentity}/authorize returns HTTP 200 per TS 29.503.
func TestConformance_AuthorizeNiddData_Returns200(t *testing.T) {
	svc := &handlerMockService{
		authorizeNiddDataFn: func(_ context.Context, _ string, _ *AuthorizationInfo) (*AuthorizationData, error) {
			return &AuthorizationData{
				AuthorizationData: []NiddAuthorizationInfo{
					{
						Supi:         "imsi-001010000000001",
						ValidityTime: "2026-12-31T23:59:59Z",
					},
				},
				ValidityTime: "2026-12-31T23:59:59Z",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"iot","validityTime":"2026-12-31T23:59:59Z"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/imsi-001010000000001/authorize",
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

// TestConformance_AuthorizeNiddData_NotFound_Returns404 verifies that when
// no subscriber exists, a 404 with ProblemDetails is returned per TS 29.503.
func TestConformance_AuthorizeNiddData_NotFound_Returns404(t *testing.T) {
	svc := &handlerMockService{
		authorizeNiddDataFn: func(_ context.Context, _ string, _ *AuthorizationInfo) (*AuthorizationData, error) {
			return nil, errors.NewNotFound("subscriber not found", errors.CauseUserNotFound)
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"iot"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/imsi-999990000000001/authorize",
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
		t.Error("ProblemDetails.Cause is empty but expected USER_NOT_FOUND")
	}
}

// TestConformance_AuthorizeNiddData_BadRequest_Returns400 verifies that an
// invalid request body returns 400 with ProblemDetails per TS 29.503.
func TestConformance_AuthorizeNiddData_BadRequest_Returns400(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/msisdn-12025551234/authorize",
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
