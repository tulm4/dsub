package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/logging"
)

// ---------------------------------------------------------------------------
// TokenClaims tests
// ---------------------------------------------------------------------------

func TestTokenClaims_IsExpired(t *testing.T) {
	tests := []struct {
		name   string
		expiry time.Time
		want   bool
	}{
		{name: "not expired", expiry: time.Now().Add(1 * time.Hour), want: false},
		{name: "expired", expiry: time.Now().Add(-1 * time.Hour), want: true},
		{name: "just expired", expiry: time.Now().Add(-1 * time.Millisecond), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &TokenClaims{Expiry: tt.expiry}
			if got := claims.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenClaims_HasScope(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		check    string
		want     bool
	}{
		{name: "single scope match", scope: "nudm-ueau", check: "nudm-ueau", want: true},
		{name: "multi scope match", scope: "nudm-ueau nudm-sdm", check: "nudm-sdm", want: true},
		{name: "no match", scope: "nudm-ueau", check: "nudm-sdm", want: false},
		{name: "empty scope", scope: "", check: "nudm-ueau", want: false},
		{name: "empty check", scope: "nudm-ueau", check: "", want: false},
		{name: "partial match not allowed", scope: "nudm-ueau", check: "nudm-uea", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &TokenClaims{Scope: tt.scope}
			if got := claims.HasScope(tt.check); got != tt.want {
				t.Errorf("HasScope(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// OAuth2Middleware tests
// ---------------------------------------------------------------------------

func TestOAuth2Middleware_MissingAuthHeader(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(nil)
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	var pd udmerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusUnauthorized {
		t.Errorf("ProblemDetails status = %d, want %d", pd.Status, http.StatusUnauthorized)
	}
}

func TestOAuth2Middleware_InvalidScheme(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(nil)
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestOAuth2Middleware_EmptyBearerToken(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(nil)
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestOAuth2Middleware_InvalidToken(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"valid-token": {
			Scope:  ScopeNudmUEAU,
			Expiry: time.Now().Add(1 * time.Hour),
		},
	})
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	var pd udmerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.AccessTokenError != "invalid_token" {
		t.Errorf("accessTokenError = %q, want %q", pd.AccessTokenError, "invalid_token")
	}
}

func TestOAuth2Middleware_ExpiredToken(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"expired-token": {
			Subject: "ausf-01",
			Scope:   ScopeNudmUEAU,
			Expiry:  time.Now().Add(-1 * time.Hour),
		},
	})
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestOAuth2Middleware_InsufficientScope(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"sdm-token": {
			Subject: "amf-01",
			Scope:   ScopeNudmSDM,
			Expiry:  time.Now().Add(1 * time.Hour),
		},
	})
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Bearer sdm-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}

	var pd udmerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.AccessTokenError != "insufficient_scope" {
		t.Errorf("accessTokenError = %q, want %q", pd.AccessTokenError, "insufficient_scope")
	}
}

func TestOAuth2Middleware_ValidToken(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"valid-token": {
			Issuer:   "nrf-01",
			Subject:  "ausf-01",
			Audience: "UDM",
			Scope:    ScopeNudmUEAU,
			Expiry:   time.Now().Add(1 * time.Hour),
			NFType:   "AUSF",
		},
	})
	mw := NewOAuth2Middleware(validator, ScopeNudmUEAU, logger)

	var receivedClaims *TokenClaims
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-ueau/v1/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if receivedClaims == nil {
		t.Fatal("expected claims in context, got nil")
	}
	if receivedClaims.Subject != "ausf-01" {
		t.Errorf("Subject = %q, want %q", receivedClaims.Subject, "ausf-01")
	}
	if receivedClaims.NFType != "AUSF" {
		t.Errorf("NFType = %q, want %q", receivedClaims.NFType, "AUSF")
	}
}

func TestOAuth2Middleware_EmptyScope_AllowsAll(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"any-token": {
			Subject: "nef-01",
			Scope:   "nudm-pp",
			Expiry:  time.Now().Add(1 * time.Hour),
		},
	})
	// Empty required scope means no scope check.
	mw := NewOAuth2Middleware(validator, "", logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer any-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestClaimsFromContext_NoClaims(t *testing.T) {
	ctx := context.Background()
	claims := ClaimsFromContext(ctx)
	if claims != nil {
		t.Errorf("expected nil claims, got %+v", claims)
	}
}

// ---------------------------------------------------------------------------
// StaticTokenValidator tests
// ---------------------------------------------------------------------------

func TestStaticTokenValidator_ValidToken(t *testing.T) {
	claims := &TokenClaims{Subject: "ausf-01", Scope: ScopeNudmUEAU}
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"token-abc": claims,
	})

	got, err := validator.ValidateToken(context.Background(), "token-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Subject != "ausf-01" {
		t.Errorf("Subject = %q, want %q", got.Subject, "ausf-01")
	}
}

func TestStaticTokenValidator_InvalidToken(t *testing.T) {
	validator := NewStaticTokenValidator(map[string]*TokenClaims{
		"token-abc": {Subject: "ausf-01"},
	})

	_, err := validator.ValidateToken(context.Background(), "token-xyz")
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
}

func TestStaticTokenValidator_EmptyMap(t *testing.T) {
	validator := NewStaticTokenValidator(nil)
	_, err := validator.ValidateToken(context.Background(), "any-token")
	if err == nil {
		t.Fatal("expected error for empty validator")
	}
}

// ---------------------------------------------------------------------------
// Scope constants tests
// ---------------------------------------------------------------------------

func TestScopeConstants(t *testing.T) {
	scopes := []string{
		ScopeNudmSDM, ScopeNudmUEAU, ScopeNudmUECM, ScopeNudmEE,
		ScopeNudmPP, ScopeNudmNIDD, ScopeNudmMT, ScopeNudmSDMSub,
		ScopeNudmReport, ScopeNudmSDEC,
	}
	seen := make(map[string]bool)
	for _, s := range scopes {
		if s == "" {
			t.Error("scope constant must not be empty")
		}
		if !strings.HasPrefix(s, "nudm-") {
			t.Errorf("scope %q must start with nudm-", s)
		}
		if seen[s] {
			t.Errorf("duplicate scope: %q", s)
		}
		seen[s] = true
	}
}
