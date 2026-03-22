package ee

// HTTP handler tests for the Nudm_EE service.
//
// Based on: docs/sbi-api-design.md §3.4 (EE Endpoints)
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
	createSubscriptionFn func(ctx context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error)
	updateSubscriptionFn func(ctx context.Context, ueIdentity, subscriptionID string, patch *PatchEeSubscription) (*EeSubscription, error)
	deleteSubscriptionFn func(ctx context.Context, ueIdentity, subscriptionID string) error
}

func (m *mockService) CreateSubscription(ctx context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error) {
	if m.createSubscriptionFn != nil {
		return m.createSubscriptionFn(ctx, ueIdentity, sub)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) UpdateSubscription(ctx context.Context, ueIdentity, subscriptionID string, patch *PatchEeSubscription) (*EeSubscription, error) {
	if m.updateSubscriptionFn != nil {
		return m.updateSubscriptionFn(ctx, ueIdentity, subscriptionID, patch)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeleteSubscription(ctx context.Context, ueIdentity, subscriptionID string) error {
	if m.deleteSubscriptionFn != nil {
		return m.deleteSubscriptionFn(ctx, ueIdentity, subscriptionID)
	}
	return errors.NewNotImplemented("not implemented")
}

func (m *mockService) GetMatchingSubscriptions(_ context.Context, _ string) ([]EeEventReport, error) {
	return nil, errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *mockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
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

// --- CreateSubscription Tests ---

func TestHandleCreateSubscription_Success(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error) {
			if ueIdentity != "imsi-001010000000001" {
				t.Errorf("unexpected ueIdentity: %s", ueIdentity)
			}
			if sub.CallbackReference != "https://nef.example.com/callback" {
				t.Errorf("unexpected callbackReference: %s", sub.CallbackReference)
			}
			return &CreatedEeSubscription{
				EeSubscription: sub,
				SubscriptionID: "sub-001",
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
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var result CreatedEeSubscription
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SubscriptionID != "sub-001" {
		t.Errorf("expected subscriptionId sub-001, got %s", result.SubscriptionID)
	}
}

func TestHandleCreateSubscription_BadBody(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions",
		bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleCreateSubscription_ServiceError(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, _ string, _ *EeSubscription) (*CreatedEeSubscription, error) {
			return nil, errors.NewBadRequest("invalid", errors.CauseMandatoryIEMissing)
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback","monitoringConfigurations":{"cfg1":{}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSubscription_InvalidUeID(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, _ string, _ *EeSubscription) (*CreatedEeSubscription, error) {
			return nil, errors.NewBadRequest("invalid ueIdentity", errors.CauseMandatoryIEIncorrect)
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback","monitoringConfigurations":{"cfg1":{}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/bad-id/ee-subscriptions",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleCreateSubscription_InternalError(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, _ string, _ *EeSubscription) (*CreatedEeSubscription, error) {
			return nil, errors.NewInternalError("db error")
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback","monitoringConfigurations":{"cfg1":{}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSubscription_GroupID(t *testing.T) {
	svc := &mockService{
		createSubscriptionFn: func(_ context.Context, ueIdentity string, sub *EeSubscription) (*CreatedEeSubscription, error) {
			if ueIdentity != "group-001" {
				t.Errorf("unexpected ueIdentity: %s", ueIdentity)
			}
			return &CreatedEeSubscription{
				EeSubscription: sub,
				SubscriptionID: "sub-group-001",
			}, nil
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback","monitoringConfigurations":{"cfg1":{}}}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/group-001/ee-subscriptions",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}
}

// --- UpdateSubscription Tests ---

func TestHandleUpdateSubscription_Success(t *testing.T) {
	svc := &mockService{
		updateSubscriptionFn: func(_ context.Context, ueIdentity, subscriptionID string, patch *PatchEeSubscription) (*EeSubscription, error) {
			if ueIdentity != "imsi-001010000000001" {
				t.Errorf("unexpected ueIdentity: %s", ueIdentity)
			}
			if subscriptionID != "sub-001" {
				t.Errorf("unexpected subscriptionId: %s", subscriptionID)
			}
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
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result EeSubscription
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.CallbackReference != "https://nef.example.com/callback-updated" {
		t.Errorf("expected updated callbackReference, got %s", result.CallbackReference)
	}
}

func TestHandleUpdateSubscription_BadBody(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001",
		bytes.NewBufferString("{bad"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleUpdateSubscription_NotFound(t *testing.T) {
	svc := &mockService{
		updateSubscriptionFn: func(_ context.Context, _ string, _ string, _ *PatchEeSubscription) (*EeSubscription, error) {
			return nil, errors.NewNotFound("subscription not found", errors.CauseSubscriptionNotFound)
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/nonexistent",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
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
	if pd.Cause != errors.CauseSubscriptionNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseSubscriptionNotFound, pd.Cause)
	}
}

func TestHandleUpdateSubscription_ServiceError(t *testing.T) {
	svc := &mockService{
		updateSubscriptionFn: func(_ context.Context, _ string, _ string, _ *PatchEeSubscription) (*EeSubscription, error) {
			return nil, errors.NewBadRequest("invalid", errors.CauseMandatoryIEIncorrect)
		},
	}
	mux := newTestMux(svc)

	body := `{"callbackReference":"https://nef.example.com/callback"}`
	req := httptest.NewRequest(http.MethodPatch,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- DeleteSubscription Tests ---

func TestHandleDeleteSubscription_Success(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, ueIdentity, subscriptionID string) error {
			if ueIdentity != "imsi-001010000000001" {
				t.Errorf("unexpected ueIdentity: %s", ueIdentity)
			}
			if subscriptionID != "sub-001" {
				t.Errorf("unexpected subscriptionId: %s", subscriptionID)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSubscription_NotFound(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, _ string, _ string) error {
			return errors.NewNotFound("subscription not found", errors.CauseSubscriptionNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleDeleteSubscription_ServiceError(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, _ string, _ string) error {
			return errors.NewInternalError("db error")
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/imsi-001010000000001/ee-subscriptions/sub-001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSubscription_GPSI(t *testing.T) {
	svc := &mockService{
		deleteSubscriptionFn: func(_ context.Context, ueIdentity, subscriptionID string) error {
			if ueIdentity != "msisdn-12025551234" {
				t.Errorf("unexpected ueIdentity: %s", ueIdentity)
			}
			return nil
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-ee/v1/msisdn-12025551234/ee-subscriptions/sub-001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Route Tests ---

func TestRouteNotFound(t *testing.T) {
	mux := newTestMux(&mockService{})

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ee/v1/imsi-001010000000001/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
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
		{"imsi-001/ee-subscriptions", 2},
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
		{[]string{"imsi-001", "ee-subscriptions"}, "*/ee-subscriptions", true},
		{[]string{"imsi-001", "ee-subscriptions", "sub-001"}, "*/ee-subscriptions/*", true},
		{[]string{"imsi-001", "ee-subscriptions", "sub-001"}, "*/ee-subscriptions", false},
		{[]string{"imsi-001"}, "*/ee-subscriptions", false},
		{[]string{"imsi-001", "wrong"}, "*/ee-subscriptions", false},
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
