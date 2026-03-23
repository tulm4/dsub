package ssau

// HTTP handler tests for the Nudm_SSAU service.
//
// Based on: docs/sbi-api-design.md §3.7 (SSAU Endpoints)
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

// handlerMockService implements ServiceInterface for handler tests.
type handlerMockService struct {
	authorizeFn func(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error)
	removeFn    func(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationRemoveData) error
}

func (m *handlerMockService) Authorize(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error) {
	if m.authorizeFn != nil {
		return m.authorizeFn(ctx, ueIdentity, serviceType, req)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *handlerMockService) Remove(ctx context.Context, ueIdentity, serviceType string, req *ServiceSpecificAuthorizationRemoveData) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, ueIdentity, serviceType, req)
	}
	return errors.NewNotImplemented("not implemented")
}

// newTestMux creates an http.ServeMux wired to the given mock service.
func newTestMux(svc *handlerMockService) *http.ServeMux {
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

// --- Authorize Tests ---

func TestHandleAuthorize_Success(t *testing.T) {
	svc := &handlerMockService{
		authorizeFn: func(_ context.Context, ueID, svcType string, _ *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error) {
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
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result ServiceSpecificAuthorizationData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.AuthID != "auth-001" {
		t.Errorf("expected authId auth-001, got %s", result.AuthID)
	}
}

func TestHandleAuthorize_InvalidBody(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/authorize",
		bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleAuthorize_ServiceError(t *testing.T) {
	svc := &handlerMockService{
		authorizeFn: func(_ context.Context, _ string, _ string, _ *ServiceSpecificAuthorizationInfo) (*ServiceSpecificAuthorizationData, error) {
			return nil, errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"ims"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/authorize",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- Remove Tests ---

func TestHandleRemove_Success(t *testing.T) {
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
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRemove_NotFound(t *testing.T) {
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
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- Route dispatch tests ---

func TestRoute_UnknownEndpoint(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/nonexistent",
		http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRoute_UnsupportedMethod(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-ssau/v1/msisdn-12025551234/AF_GUIDANCE_FOR_URSP/authorize",
		http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// GET is not registered on the mux, so Go's default mux returns 405 or 404
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 or 405, got %d: %s", w.Code, w.Body.String())
	}
}
