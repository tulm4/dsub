package rsds

// 3GPP API conformance tests for Nudm_RSDS (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_RSDS

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_ReportSMDeliveryStatus_Returns204 verifies that POST on
// /{ueIdentity}/sm-delivery-status returns HTTP 204 per TS 29.503.
func TestConformance_ReportSMDeliveryStatus_Returns204(t *testing.T) {
	svc := &handlerMockService{
		reportSMDeliveryStatusFn: func(_ context.Context, _ string, _ *SmDeliveryStatus) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	body := `{"gpsi":"msisdn-12025551234","smStatusReport":{"status":"delivered"}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_ReportSMDeliveryStatus_BadRequest_Returns400 verifies that
// an invalid request body returns 400 with ProblemDetails per TS 29.503.
func TestConformance_ReportSMDeliveryStatus_BadRequest_Returns400(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
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

// TestConformance_ReportSMDeliveryStatus_InternalError_Returns500 verifies
// that a server error returns 500 with ProblemDetails per TS 29.503.
func TestConformance_ReportSMDeliveryStatus_InternalError_Returns500(t *testing.T) {
	svc := &handlerMockService{
		reportSMDeliveryStatusFn: func(_ context.Context, _ string, _ *SmDeliveryStatus) error {
			return errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	body := `{"gpsi":"msisdn-12025551234","smStatusReport":{"status":"failed"}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-rsds/v1/msisdn-12025551234/sm-delivery-status",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}
