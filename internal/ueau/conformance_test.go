package ueau

// 3GPP API conformance tests for Nudm_UEAU (TS 29.503).
// These are unit tests that validate HTTP routing, content types,
// response codes, and ProblemDetails conformance using mock services.
// Based on: docs/sbi-api-design.md §3, docs/testing-strategy.md
// 3GPP: TS 29.503 Nudm_UEAU

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/errors"
)

// newConformanceMux creates a test mux wired to the given mock service.
func newConformanceMux(svc *mockService) *http.ServeMux {
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

// TestConformance_GenerateAuthData_ContentType verifies that a successful
// GenerateAuthData response uses the application/json Content-Type header
// as required by 3GPP TS 29.503.
func TestConformance_GenerateAuthData_ContentType(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			return &AuthenticationInfoResult{
				AuthType: "5G_AKA",
				AuthenticationVector: &AuthenticationVector{
					AvType: "5G_HE_AKA",
					Rand:   "aabbccdd",
					Autn:   "11223344",
				},
			}, nil
		},
	}
	mux := newConformanceMux(svc)

	body := `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"ausf-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}

// TestConformance_GenerateAuthData_SuccessReturns200 verifies that a
// successful GenerateAuthData returns HTTP 200 per TS 29.503 §6.1.3.
func TestConformance_GenerateAuthData_SuccessReturns200(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			return &AuthenticationInfoResult{
				AuthType: "5G_AKA",
				AuthenticationVector: &AuthenticationVector{
					AvType: "5G_HE_AKA",
					Rand:   "aabbccdd",
					Autn:   "11223344",
				},
			}, nil
		},
	}
	mux := newConformanceMux(svc)

	body := `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"ausf-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// TestConformance_GenerateAuthData_NotFound_Returns404 verifies that when
// the subscriber is not found, a 404 ProblemDetails response is returned
// with application/problem+json Content-Type per RFC 7807.
func TestConformance_GenerateAuthData_NotFound_Returns404(t *testing.T) {
	svc := &mockService{
		generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
			return nil, errors.NewNotFound("subscriber not found", "USER_NOT_FOUND")
		},
	}
	mux := newConformanceMux(svc)

	body := `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"ausf-001"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-999990000000001/security-information/generate-auth-data",
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
	if pd.Status != http.StatusNotFound {
		t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusNotFound)
	}
}

// TestConformance_ConfirmAuth_Created_Returns201 verifies that creating a
// new auth event via POST /auth-events returns HTTP 201 per TS 29.503.
func TestConformance_ConfirmAuth_Created_Returns201(t *testing.T) {
	svc := &mockService{
		confirmAuthFn: func(_ context.Context, _ string, event *AuthEvent) (*AuthEvent, error) {
			return event, nil
		},
	}
	mux := newConformanceMux(svc)

	body := `{"nfInstanceId":"ausf-001","success":true,"authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org"}`
	req := httptest.NewRequest(http.MethodPost,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events",
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

// TestConformance_DeleteAuth_Returns204 verifies that deleting an auth event
// via PUT /auth-events/{authEventId} returns HTTP 204 per TS 29.503.
func TestConformance_DeleteAuth_Returns204(t *testing.T) {
	svc := &mockService{
		deleteAuthEventFn: func(_ context.Context, _ string, _ string) error {
			return nil
		},
	}
	mux := newConformanceMux(svc)

	body := `{"nfInstanceId":"ausf-001","success":true,"authType":"5G_AKA","servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org"}`
	req := httptest.NewRequest(http.MethodPut,
		"/nudm-ueau/v1/imsi-001010000000001/auth-events/evt-001",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

// TestConformance_InvalidMethod_Returns404 verifies that a request to an
// unknown endpoint returns 404 ProblemDetails.
func TestConformance_InvalidMethod_Returns404(t *testing.T) {
	svc := &mockService{}
	mux := newConformanceMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-ueau/v1/imsi-001010000000001/nonexistent-path",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)
}

// TestConformance_ProblemDetails_RFC7807 verifies that all error responses
// contain required RFC 7807 fields: status, title, and detail.
func TestConformance_ProblemDetails_RFC7807(t *testing.T) {
	tests := []struct {
		name       string
		svc        *mockService
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name: "NotFound",
			svc: &mockService{
				generateAuthDataFn: func(_ context.Context, _ string, _ *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
					return nil, errors.NewNotFound("subscriber not found", "USER_NOT_FOUND")
				},
			},
			method:     http.MethodPost,
			path:       "/nudm-ueau/v1/imsi-999990000000001/security-information/generate-auth-data",
			body:       `{"servingNetworkName":"5G:mnc001.mcc001.3gppnetwork.org","ausfInstanceId":"ausf-001"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "UnknownEndpoint",
			svc:        &mockService{},
			method:     http.MethodGet,
			path:       "/nudm-ueau/v1/imsi-001010000000001/does-not-exist",
			body:       "",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := newConformanceMux(tc.svc)

			var bodyReader *bytes.Buffer
			if tc.body != "" {
				bodyReader = bytes.NewBufferString(tc.body)
			}

			var req *http.Request
			if bodyReader != nil {
				req = httptest.NewRequest(tc.method, tc.path, bodyReader)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, http.NoBody)
			}
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status code: got %d, want %d", w.Code, tc.wantStatus)
			}
			assertProblemDetailsContentType(t, w)

			var pd errors.ProblemDetails
			if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
				t.Fatalf("decode ProblemDetails: %v", err)
			}
			if pd.Status == 0 {
				t.Error("ProblemDetails.Status is missing (0)")
			}
			if pd.Title == "" {
				t.Error("ProblemDetails.Title is empty")
			}
			if pd.Detail == "" {
				t.Error("ProblemDetails.Detail is empty")
			}
		})
	}
}

// TestConformance_GetRgAuthData_Returns501 verifies that the unimplemented
// RG authentication data endpoint returns 501 per spec.
func TestConformance_GetRgAuthData_Returns501(t *testing.T) {
	svc := &mockService{}
	mux := newConformanceMux(svc)

	req := httptest.NewRequest(http.MethodGet,
		"/nudm-ueau/v1/imsi-001010000000001/security-information-rg",
		http.NoBody)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Errorf("status code: got %d, want %d; body: %s", w.Code, http.StatusNotImplemented, w.Body.String())
	}
	assertProblemDetailsContentType(t, w)

	var pd errors.ProblemDetails
	if err := json.NewDecoder(w.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusNotImplemented {
		t.Errorf("ProblemDetails.Status: got %d, want %d", pd.Status, http.StatusNotImplemented)
	}
	if pd.Title == "" {
		t.Error("ProblemDetails.Title is empty")
	}
}
