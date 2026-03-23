package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/logging"
)

func TestInputValidation_ValidJSONRequest(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestInputValidation_MissingContentType(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	// No Content-Type header.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var pd udmerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Cause != udmerrors.CauseMandatoryIEMissing {
		t.Errorf("cause = %q, want %q", pd.Cause, udmerrors.CauseMandatoryIEMissing)
	}
}

func TestInputValidation_WrongContentType(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`<xml>data</xml>`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/xml")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", rec.Code)
	}
}

func TestInputValidation_ProblemJSONAccepted(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`{"type": "about:blank"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/problem+json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestInputValidation_ContentLengthTooLarge(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(100, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 200 // larger than maxBodySize
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rec.Code)
	}
}

func TestInputValidation_GetRequestSkipsContentTypeCheck(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestInputValidation_DeleteRequestSkipsContentTypeCheck(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestInputValidation_JSONCharset(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	mw := NewInputValidationMiddleware(0, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`{"key": "value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequiresBody(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{http.MethodPost, true},
		{http.MethodPut, true},
		{http.MethodPatch, true},
		{http.MethodGet, false},
		{http.MethodDelete, false},
		{http.MethodHead, false},
		{http.MethodOptions, false},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := requiresBody(tt.method); got != tt.want {
				t.Errorf("requiresBody(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestIsJSONMediaType(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{"application/json", true},
		{"application/problem+json", true},
		{"application/merge-patch+json", true},
		{"application/xml", false},
		{"text/html", false},
		{"text/plain", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			if got := isJSONMediaType(tt.mediaType); got != tt.want {
				t.Errorf("isJSONMediaType(%q) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}
