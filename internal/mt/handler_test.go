package mt

// HTTP handler tests for the Nudm_MT service.
//
// Based on: docs/sbi-api-design.md §3.6 (MT Endpoints)
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
	queryUeInfoFn         func(ctx context.Context, supi string) (*UeInfo, error)
	provideLocationInfoFn func(ctx context.Context, supi string, req *LocationInfoRequest) (*LocationInfoResult, error)
}

func (m *mockService) QueryUeInfo(ctx context.Context, supi string) (*UeInfo, error) {
	if m.queryUeInfoFn != nil {
		return m.queryUeInfoFn(ctx, supi)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) ProvideLocationInfo(ctx context.Context, supi string, req *LocationInfoRequest) (*LocationInfoResult, error) {
	if m.provideLocationInfoFn != nil {
		return m.provideLocationInfoFn(ctx, supi, req)
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

// assertProblemDetailsContentType verifies the Content-Type is application/problem+json.
func assertProblemDetailsContentType(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	ct := w.Header().Get("Content-Type")
	expected := "application/problem+json"
	if ct != expected {
		t.Errorf("Content-Type: got %q, want %q", ct, expected)
	}
}

// --- QueryUeInfo Tests ---

func TestHandleQueryUeInfo_Success(t *testing.T) {
	svc := &mockService{
		queryUeInfoFn: func(_ context.Context, supi string) (*UeInfo, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected supi: %s", supi)
			}
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
		"/nudm-mt/v1/imsi-001010000000001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result UeInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ServingAmfId != "amf-001" {
		t.Errorf("expected servingAmfId amf-001, got %s", result.ServingAmfId)
	}
	if result.UserState != "REGISTERED" {
		t.Errorf("expected userState REGISTERED, got %s", result.UserState)
	}
}

func TestHandleQueryUeInfo_NotFound(t *testing.T) {
	svc := &mockService{
		queryUeInfoFn: func(_ context.Context, _ string) (*UeInfo, error) {
			return nil, errors.NewNotFound("registration not found", errors.CauseContextNotFound)
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-mt/v1/imsi-001010000000001", http.NoBody)
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

func TestHandleQueryUeInfo_ServiceError(t *testing.T) {
	svc := &mockService{
		queryUeInfoFn: func(_ context.Context, _ string) (*UeInfo, error) {
			return nil, errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-mt/v1/imsi-001010000000001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- ProvideLocationInfo Tests ---

func TestHandleProvideLocationInfo_Success(t *testing.T) {
	svc := &mockService{
		provideLocationInfoFn: func(_ context.Context, supi string, req *LocationInfoRequest) (*LocationInfoResult, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected supi: %s", supi)
			}
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
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result LocationInfoResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ServingAmfId != "amf-002" {
		t.Errorf("expected servingAmfId amf-002, got %s", result.ServingAmfId)
	}
}

func TestHandleProvideLocationInfo_NotFound(t *testing.T) {
	svc := &mockService{
		provideLocationInfoFn: func(_ context.Context, _ string, _ *LocationInfoRequest) (*LocationInfoResult, error) {
			return nil, errors.NewNotFound("registration not found", errors.CauseContextNotFound)
		},
	}
	mux := newTestMux(svc)

	body := `{"req5gsInd":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-mt/v1/imsi-001010000000001/loc-info/provide-loc-info",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleProvideLocationInfo_InvalidBody(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-mt/v1/imsi-001010000000001/loc-info/provide-loc-info",
		bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

func TestHandleProvideLocationInfo_ServiceError(t *testing.T) {
	svc := &mockService{
		provideLocationInfoFn: func(_ context.Context, _ string, _ *LocationInfoRequest) (*LocationInfoResult, error) {
			return nil, errors.NewInternalError("database failure")
		},
	}
	mux := newTestMux(svc)

	body := `{"req5gsInd":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-mt/v1/imsi-001010000000001/loc-info/provide-loc-info",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", w.Code, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// --- Route dispatch tests ---

func TestRoute_UnknownEndpoint(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-mt/v1/imsi-001010000000001/nonexistent", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRoute_UnsupportedMethod(t *testing.T) {
	svc := &mockService{}
	mux := newTestMux(svc)

	req := httptest.NewRequest(http.MethodDelete,
		"/nudm-mt/v1/imsi-001010000000001", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// DELETE is not registered on the mux, so Go's default mux returns 405 or 404
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 or 405, got %d: %s", w.Code, w.Body.String())
	}
}
