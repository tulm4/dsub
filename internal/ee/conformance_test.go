package ee

// 3GPP API conformance tests for Nudm_EE (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_EE

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// TestConformance_CreateSubscription_Returns201 verifies that a new EE
// subscription returns HTTP 201 per TS 29.503.
func TestConformance_CreateSubscription_Returns201(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, _ string, sub *EeSubscription) (*CreatedEeSubscription, error) {
			return &CreatedEeSubscription{
				EeSubscription: sub,
				SubscriptionID: "sub-conformance-001",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback","monitoringConfigurations":{"cfg1":{"eventType":"LOSS_OF_CONNECTIVITY"}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions",
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

// TestConformance_CreateSubscription_MissingBody_Returns400 verifies that a
// request with invalid JSON body returns 400 with ProblemDetails per TS 29.503.
func TestConformance_CreateSubscription_MissingBody_Returns400(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	body := `{this is not valid json`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions",
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

// TestConformance_UpdateSubscription_Returns200 verifies that updating an
// existing EE subscription returns HTTP 200 per TS 29.503.
func TestConformance_UpdateSubscription_Returns200(t *testing.T) {
	svc := &mockService{
		updateSubscriptionFn: func(_ context.Context, _, _ string, _ *PatchEeSubscription) (*EeSubscription, error) {
			return &EeSubscription{
				CallbackReference: "https://nef.example.com/callback-updated",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback-updated"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001",
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

// TestConformance_DeleteSubscription_Returns204 verifies that deleting an
// EE subscription returns HTTP 204 per TS 29.503.
func TestConformance_DeleteSubscription_Returns204(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_DeleteSubscription_NotFound_Returns404 verifies that
// deleting a non-existent subscription returns 404 with ProblemDetails
// per TS 29.503.
func TestConformance_DeleteSubscription_NotFound_Returns404(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, _, _ string) error {
			return errors.NewNotFound("subscription not found", errors.CauseSubscriptionNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/nonexistent",
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
		t.Error("ProblemDetails.Cause is empty but expected SUBSCRIPTION_NOT_FOUND")
	}
}
