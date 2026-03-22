package ueau

// HTTP handler tests for the Nudm_UEAU service.
//
// Based on: docs/sbi-api-design.md §3.1 (UEAU Endpoints)
// Based on: docs/sbi-api-design.md §7 (Error Handling)

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// mockService implements ServiceInterface for handler tests.
type mockService struct {
	generateAuthDataFn func(ctx context.Context, supiOrSuci string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error)
	confirmAuthFn      func(ctx context.Context, supi string, event *AuthEvent) (*AuthEvent, error)
	deleteAuthEventFn  func(ctx context.Context, supi, authEventID string) error
}

func (m *mockService) GenerateAuthData(ctx context.Context, supiOrSuci string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
	if m.generateAuthDataFn != nil {
		return m.generateAuthDataFn(ctx, supiOrSuci, req)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) ConfirmAuth(ctx context.Context, supi string, event *AuthEvent) (*AuthEvent, error) {
	if m.confirmAuthFn != nil {
		return m.confirmAuthFn(ctx, supi, event)
	}
	return nil, errors.NewNotImplemented("not implemented")
}

func (m *mockService) DeleteAuthEvent(ctx context.Context, supi, authEventID string) error {
	if m.deleteAuthEventFn != nil {
		return m.deleteAuthEventFn(ctx, supi, authEventID)
	}
	return errors.NewNotImplemented("not implemented")
}

func TestHandleGenerateAuthData_Success(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, supiOrSuci string, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			if supiOrSuci != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supiOrSuci)
			}
			if req.ServingNetworkName != "5G:mnc001.mcc001.3gppnetwork.org" {
				t.Errorf("unexpected serving network: %s", req.ServingNetworkName)
			}
			return &AuthenticationInfoResult{
				AuthType: "5G_AKA",
				AuthenticationVector: &AuthenticationVector{
					AvType:   "5G_HE_AKA",
					Rand:     "aabbccdd",
					Autn:     "11223344",
					XresStar: "deadbeef",
					Kausf:    "cafebabe",
				},
				Supi: supiOrSuci,
			}, nil
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"test-ausf-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result AuthenticationInfoResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.AuthType != "5G_AKA" {
		t.Errorf("expected authType 5G_AKA, got %s", result.AuthType)
	}
	if result.AuthenticationVector == nil {
		t.Fatal("expected authentication vector, got nil")
	}
	if result.AuthenticationVector.AvType != "5G_HE_AKA" {
		t.Errorf("expected avType 5G_HE_AKA, got %s", result.AuthenticationVector.AvType)
	}
}

func TestHandleGenerateAuthData_BadRequestBody(t *testing.T) {
	handler := NewHandler(&mockService{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	assertProblemDetailsContentType(t, w)
}

func TestHandleGenerateAuthData_ServiceError(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			return nil, errors.NewNotFound("user not found", errors.CauseUserNotFound)
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"test"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != errors.CauseUserNotFound {
		t.Errorf("expected cause %s, got %s", errors.CauseUserNotFound, pd.Cause)
	}

	assertProblemDetailsContentType(t, w)
}

func TestHandleConfirmAuth_Success(t *testing.T) {
	svc := &mockService{
		confirmAuthFn: func(_ context.Context, supi string, event *AuthEvent) (*AuthEvent, error) {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supi)
			}
			return event, nil
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"nfInstanceId":"ausf-001","success":true,"timeStamp":"2024-01-01T00:00:00Z","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleConfirmAuth_BadRequestBody(t *testing.T) {
	handler := NewHandler(&mockService{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events",
		bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	assertProblemDetailsContentType(t, w)
}

func TestHandleDeleteAuth_Success(t *testing.T) {
	svc := &mockService{
		deleteAuthEventFn: func(_ context.Context, supi, authEventID string) error {
			if supi != "imsi-001010000000001" {
				t.Errorf("unexpected SUPI: %s", supi)
			}
			if authEventID != "5G:mnc001.mcc001.3gppnetwork.org" {
				t.Errorf("unexpected authEventID: %s", authEventID)
			}
			return nil
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"nfInstanceId":"ausf-001","success":true,"timeStamp":"2024-01-01T00:00:00Z","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","authRemovalInd":true}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events/5G:mnc001.mcc001.3gppnetwork.org",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteAuth_NotFound(t *testing.T) {
	svc := &mockService{
		deleteAuthEventFn: func(_ context.Context, _, _ string) error {
			return errors.NewNotFound("auth event not found", errors.CauseDataNotFound)
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"nfInstanceId":"ausf-001","success":true,"timeStamp":"2024-01-01T00:00:00Z","authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","authRemovalInd":true}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events/nonexistent",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}

	assertProblemDetailsContentType(t, w)
}

func TestHandleGetRgAuthData_NotImplemented(t *testing.T) {
	handler := NewHandler(&mockService{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-ueau/v1/imsi-001010000000001/security-information-rg", http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected status 501, got %d: %s", w.Code, w.Body.String())
	}

	assertProblemDetailsContentType(t, w)
}

func TestRouteNotFound(t *testing.T) {
	handler := NewHandler(&mockService{})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-ueau/v1/imsi-001010000000001/nonexistent", http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProblemDetailsFormat(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			return nil, errors.NewBadRequest("test error detail", errors.CauseMandatoryIEIncorrect)
		},
	}

	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body := `{"servingNetworkName":"test","ausfInstanceId":"test"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}

	// Verify RFC 7807 required fields
	if pd.Status != http.StatusBadRequest {
		t.Errorf("expected status 400 in body, got %d", pd.Status)
	}
	if pd.Title == "" {
		t.Error("expected non-empty title")
	}
	if pd.Detail == "" {
		t.Error("expected non-empty detail")
	}
	// Verify 3GPP cause code extension
	if pd.Cause != errors.CauseMandatoryIEIncorrect {
		t.Errorf("expected cause %s, got %s", errors.CauseMandatoryIEIncorrect, pd.Cause)
	}
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

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"/", 0},
		{"a/b/c", 3},
		{"/a/b/c/", 3},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
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
		{[]string{"imsi-001", "security-information", "generate-auth-data"}, "*/security-information/generate-auth-data", true},
		{[]string{"imsi-001", "auth-events"}, "*/auth-events", true},
		{[]string{"imsi-001", "auth-events", "evt1"}, "*/auth-events/*", true},
		{[]string{"imsi-001", "wrong-path"}, "*/security-information/generate-auth-data", false},
		{[]string{"imsi-001"}, "*/auth-events", false},
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
