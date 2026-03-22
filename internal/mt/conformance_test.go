package mt

// 3GPP API conformance tests for Nudm_MT (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_MT

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_QueryUeInfo_Returns200 verifies that GET on /{supi}
// returns HTTP 200 per TS 29.503.
func TestConformance_QueryUeInfo_Returns200(t *testing.T) {
	svc := &mockService{
		queryUeInfoFn: func(_ context.Context, _ string) (*UeInfo, error) {
			return &UeInfo{
				UserState:    "REGISTERED",
				ServingAmfId: "amf-001",
				RatType:      "NR",
				AccessType:   "3GPP_ACCESS",
			}, nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-mt/v1/imsi-001010000000001",
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

// TestConformance_QueryUeInfo_NotFound_Returns404 verifies that when no
// AMF registration exists, a 404 with application/problem+json and cause
// field is returned per RFC 7807.
func TestConformance_QueryUeInfo_NotFound_Returns404(t *testing.T) {
	svc := &mockService{
		queryUeInfoFn: func(_ context.Context, _ string) (*UeInfo, error) {
			return nil, errors.NewNotFound("registration not found", errors.CauseContextNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-mt/v1/imsi-999990000000001",
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

// TestConformance_ProvideLocationInfo_Returns200 verifies that POST on
// /{supi}/loc-info/provide-loc-info returns HTTP 200 per TS 29.503.
func TestConformance_ProvideLocationInfo_Returns200(t *testing.T) {
	svc := &mockService{
		provideLocationInfoFn: func(_ context.Context, supi string, _ *LocationInfoRequest) (*LocationInfoResult, error) {
			return &LocationInfoResult{
				Supi:         supi,
				ServingAmfId: "amf-002",
				UserState:    "REGISTERED",
				RatType:      "NR",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"supi":"imsi-001010000000001","req5gsInd":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-mt/v1/imsi-001010000000001/loc-info/provide-loc-info",
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

// TestConformance_ProvideLocationInfo_MissingBody_Returns400 verifies that
// a request with invalid JSON body returns 400 with ProblemDetails per TS 29.503.
func TestConformance_ProvideLocationInfo_MissingBody_Returns400(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-mt/v1/imsi-001010000000001/loc-info/provide-loc-info",
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
