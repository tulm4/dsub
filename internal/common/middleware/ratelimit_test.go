package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
	"github.com/tulm4/dsub/internal/common/logging"
)

// ---------------------------------------------------------------------------
// TokenBucket tests
// ---------------------------------------------------------------------------

func TestTokenBucket_AllowBurst(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// Should allow up to burst (5) requests immediately.
	for i := range 5 {
		ok, _ := tb.Allow()
		if !ok {
			t.Errorf("request %d should be allowed within burst", i)
		}
	}

	// 6th request should be denied.
	ok, retryAfter := tb.Allow()
	if ok {
		t.Error("request beyond burst should be denied")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive when denied")
	}
}

func TestTokenBucket_AllowAt_RefillsTokens(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// Use a future time so elapsed from constructor's lastTime is always positive.
	now := time.Now().Add(1 * time.Second)

	// Consume all tokens at now.
	for range 5 {
		tb.AllowAt(now)
	}

	// No tokens left.
	ok, _ := tb.AllowAt(now)
	if ok {
		t.Error("should be denied when no tokens")
	}

	// Advance 500ms — should refill 5 tokens (10 tokens/sec * 0.5s).
	later := now.Add(500 * time.Millisecond)
	ok, _ = tb.AllowAt(later)
	if !ok {
		t.Error("should be allowed after token refill")
	}

	ok, _ = tb.AllowAt(later)
	if !ok {
		t.Error("second request should also be allowed after refill")
	}
}

func TestTokenBucket_AllowAt_DoesNotExceedBurst(t *testing.T) {
	now := time.Now()
	tb := NewTokenBucket(100, 10)

	// Consume all tokens.
	for range 10 {
		tb.AllowAt(now)
	}

	// Advance 10 seconds — enough to refill 1000 tokens, but capped at burst=10.
	later := now.Add(10 * time.Second)
	allowed := 0
	for range 20 {
		ok, _ := tb.AllowAt(later)
		if ok {
			allowed++
		}
	}
	if allowed != 10 {
		t.Errorf("allowed %d requests, expected burst cap of 10", allowed)
	}
}

func TestTokenBucket_RetryAfterDuration(t *testing.T) {
	tb := NewTokenBucket(2, 1) // 2 req/sec, burst=1

	// Consume the one burst token.
	ok, _ := tb.Allow()
	if !ok {
		t.Fatal("first request should be allowed")
	}

	// Next should be denied with ~500ms retry.
	ok, retryAfter := tb.Allow()
	if ok {
		t.Fatal("second request should be denied")
	}
	if retryAfter <= 0 || retryAfter > 1*time.Second {
		t.Errorf("retryAfter = %v, expected ~500ms", retryAfter)
	}
}

// ---------------------------------------------------------------------------
// RateLimitMiddleware tests
// ---------------------------------------------------------------------------

func TestRateLimitMiddleware_AllowsWithinLimit(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	cfg := RateLimitConfig{Rate: 100, Burst: 10}
	mw := NewRateLimitMiddleware(cfg, logger)

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

func TestRateLimitMiddleware_Returns429WhenExceeded(t *testing.T) {
	logger := logging.NewLogger("error", "test", "us-east")
	cfg := RateLimitConfig{Rate: 1, Burst: 1}
	mw := NewRateLimitMiddleware(cfg, logger)

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: allowed.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rec.Code)
	}

	// Second request: should be rate limited.
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", rec.Code)
	}

	// Check Retry-After header.
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("missing Retry-After header")
	}

	// Check ProblemDetails body.
	var pd udmerrors.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
		t.Fatalf("decode ProblemDetails: %v", err)
	}
	if pd.Status != http.StatusTooManyRequests {
		t.Errorf("ProblemDetails status = %d, want %d", pd.Status, http.StatusTooManyRequests)
	}
	if pd.Cause != udmerrors.CauseNFCongestion {
		t.Errorf("cause = %q, want %q", pd.Cause, udmerrors.CauseNFCongestion)
	}
}

// ---------------------------------------------------------------------------
// DefaultRateLimits tests
// ---------------------------------------------------------------------------

func TestDefaultRateLimits_AllServicesConfigured(t *testing.T) {
	requiredServices := []string{
		ScopeNudmUEAU, ScopeNudmSDM, ScopeNudmUECM, ScopeNudmEE,
		ScopeNudmSDEC, ScopeNudmPP, ScopeNudmMT, ScopeNudmNIDD,
	}

	for _, svc := range requiredServices {
		cfg, ok := DefaultRateLimits[svc]
		if !ok {
			t.Errorf("missing rate limit config for %q", svc)
			continue
		}
		if cfg.Rate <= 0 {
			t.Errorf("rate for %q must be positive, got %f", svc, cfg.Rate)
		}
		if cfg.Burst <= 0 {
			t.Errorf("burst for %q must be positive, got %f", svc, cfg.Burst)
		}
	}
}
