package ueid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	svcerrors "github.com/tulm4/dsub/internal/common/errors"
)

// mockService implements ServiceInterface for handler testing.
type mockService struct {
	deconcealFn func(ctx context.Context, req *SuciDeconcealRequest) (*SuciDeconcealResponse, error)
}

func (m *mockService) Deconceal(ctx context.Context, req *SuciDeconcealRequest) (*SuciDeconcealResponse, error) {
	return m.deconcealFn(ctx, req)
}

// TestHandleDeconceal_Success tests a successful SUCI de-concealment HTTP flow.
func TestHandleDeconceal_Success(t *testing.T) {
	svc := &mockService{
		deconcealFn: func(_ context.Context, req *SuciDeconcealRequest) (*SuciDeconcealResponse, error) {
			if req.Suci != "suci-0-001-01-0000-0-0-0123456789" {
				t.Errorf("unexpected SUCI: %s", req.Suci)
			}
			return &SuciDeconcealResponse{Supi: "imsi-001010123456789"}, nil
		},
	}

	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"suci":"suci-0-001-01-0000-0-0-0123456789"}`
	req := httptest.NewRequest(http.MethodPost, "/nudm-ueid/v1/deconceal", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status code: got %d, want %d", rec.Code, http.StatusOK)
	}

	var resp SuciDeconcealResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Supi != "imsi-001010123456789" {
		t.Errorf("SUPI mismatch: got %q, want %q", resp.Supi, "imsi-001010123456789")
	}
}

// TestHandleDeconceal_BadRequestBody tests rejection of invalid JSON.
func TestHandleDeconceal_BadRequestBody(t *testing.T) {
	h := NewHandler(&mockService{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/nudm-ueid/v1/deconceal", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status code: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
	assertProblemDetailsContentType(t, rec)
}

// TestHandleDeconceal_ServiceError tests propagation of service-layer errors.
func TestHandleDeconceal_ServiceError(t *testing.T) {
	svc := &mockService{
		deconcealFn: func(_ context.Context, _ *SuciDeconcealRequest) (*SuciDeconcealResponse, error) {
			return nil, svcerrors.NewNotFound("SUCI profile not found for HN key ID: 99", svcerrors.CauseDataNotFound)
		},
	}

	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"suci":"suci-0-001-01-0000-1-99-aabbccdd"}`
	req := httptest.NewRequest(http.MethodPost, "/nudm-ueid/v1/deconceal", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d", rec.Code, http.StatusNotFound)
	}
	assertProblemDetailsContentType(t, rec)
}

// TestRouteNotFound tests that unknown paths return 404.
func TestRouteNotFound(t *testing.T) {
	h := NewHandler(&mockService{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/nudm-ueid/v1/unknown-endpoint", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status code: got %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestSplitPath verifies URL path splitting.
func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"", 0},
		{"deconceal", 1},
		{"a/b/c", 3},
		{"/a/b/", 2},
	}
	for _, tt := range tests {
		got := splitPath(tt.path)
		if len(got) != tt.want {
			t.Errorf("splitPath(%q): got %d segments, want %d", tt.path, len(got), tt.want)
		}
	}
}

// TestMatchPath verifies wildcard path matching.
func TestMatchPath(t *testing.T) {
	tests := []struct {
		segments []string
		pattern  string
		want     bool
	}{
		{[]string{"deconceal"}, "deconceal", true},
		{[]string{"deconceal"}, "other", false},
		{[]string{"a", "b"}, "*/b", true},
		{[]string{"a"}, "*/b", false},
	}
	for _, tt := range tests {
		got := matchPath(tt.segments, tt.pattern)
		if got != tt.want {
			t.Errorf("matchPath(%v, %q): got %v, want %v", tt.segments, tt.pattern, got, tt.want)
		}
	}
}

// TestProblemDetailsFormat verifies RFC 7807 compliance of error responses.
func TestProblemDetailsFormat(t *testing.T) {
	svc := &mockService{
		deconcealFn: func(_ context.Context, _ *SuciDeconcealRequest) (*SuciDeconcealResponse, error) {
			return nil, svcerrors.NewBadRequest("test error", svcerrors.CauseMandatoryIEMissing)
		},
	}

	h := NewHandler(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"suci":"suci-0-001-01-0000-0-0-0123456789"}`
	req := httptest.NewRequest(http.MethodPost, "/nudm-ueid/v1/deconceal", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	assertProblemDetailsContentType(t, rec)

	var pd svcerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusBadRequest {
		t.Errorf("ProblemDetails.Status = %d, want %d", pd.Status, http.StatusBadRequest)
	}
	if pd.Cause != svcerrors.CauseMandatoryIEMissing {
		t.Errorf("ProblemDetails.Cause = %q, want %q", pd.Cause, svcerrors.CauseMandatoryIEMissing)
	}
}

// assertProblemDetailsContentType checks that Content-Type is application/problem+json.
func assertProblemDetailsContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}
