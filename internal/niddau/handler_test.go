package niddau

// HTTP handler tests for the Nudm_NIDDAU service.
//
// Based on: docs/sbi-api-design.md §3.8 (NIDDAU Endpoints)
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
	authorizeNiddDataFn func(ctx context.Context, ueIdentity string, req *AuthorizationInfo) (*AuthorizationData, error)
}

func (m *handlerMockService) AuthorizeNiddData(ctx context.Context, ueIdentity string, req *AuthorizationInfo) (*AuthorizationData, error) {
	if m.authorizeNiddDataFn != nil {
		return m.authorizeNiddDataFn(ctx, ueIdentity, req)
	}
	return nil, errors.NewNotImplemented("not implemented")
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

// --- AuthorizeNiddData Tests ---

func TestHandleAuthorizeNiddData_Success(t *testing.T) {
	svc := &handlerMockService{
		authorizeNiddDataFn: func(_ context.Context, _ string, _ *AuthorizationInfo) (*AuthorizationData, error) {
			return &AuthorizationData{
				AuthorizationData: []NiddAuthorizationInfo{
					{
						Supi:         "imsi-001010000000001",
						Gpsi:         "msisdn-12025551234",
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
		"/nudm-niddau/v1/msisdn-12025551234/authorize",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result AuthorizationData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.AuthorizationData) != 1 {
		t.Fatalf("expected 1 auth data entry, got %d", len(result.AuthorizationData))
	}
}

func TestHandleAuthorizeNiddData_InvalidBody(t *testing.T) {
	svc := &handlerMockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/msisdn-12025551234/authorize",
		bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleAuthorizeNiddData_ServiceError(t *testing.T) {
	svc := &handlerMockService{
		authorizeNiddDataFn: func(_ context.Context, _ string, _ *AuthorizationInfo) (*AuthorizationData, error) {
			return nil, errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"iot"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/msisdn-12025551234/authorize",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleAuthorizeNiddData_NotFound(t *testing.T) {
	svc := &handlerMockService{
		authorizeNiddDataFn: func(_ context.Context, _ string, _ *AuthorizationInfo) (*AuthorizationData, error) {
			return nil, errors.NewNotFound("subscriber not found", errors.CauseUserNotFound)
		},
	}
	mux := newTestMux(svc)

	body := `{"dnn":"iot"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-niddau/v1/imsi-001010000000001/authorize",
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
		"/nudm-niddau/v1/msisdn-12025551234/nonexistent",
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
		"/nudm-niddau/v1/msisdn-12025551234/authorize",
		http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// GET is not registered on the mux, so Go's default mux returns 405 or 404
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 or 405, got %d: %s", w.Code, w.Body.String())
	}
}
