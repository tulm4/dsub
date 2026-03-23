// Rate limiting middleware for the 5G UDM network function.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting)
// 3GPP: TS 29.500 §6.10.4 — Overload Control Information (OCI)

package middleware

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	udmerrors "github.com/tulm4/dsub/internal/common/errors"
)

// Default rate limits per Nudm service.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting Table)
var DefaultRateLimits = map[string]RateLimitConfig{
	ScopeNudmUEAU: {Rate: 10000, Burst: 15000},
	ScopeNudmSDM:  {Rate: 20000, Burst: 30000},
	ScopeNudmUECM: {Rate: 15000, Burst: 22500},
	ScopeNudmEE:   {Rate: 5000, Burst: 7500},
	ScopeNudmSDEC: {Rate: 50000, Burst: 50000},
	ScopeNudmPP:   {Rate: 5000, Burst: 7500},
	ScopeNudmMT:   {Rate: 5000, Burst: 7500},
	ScopeNudmNIDD: {Rate: 5000, Burst: 7500},
}

// RateLimitConfig defines the rate limit parameters for a service.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting)
type RateLimitConfig struct {
	// Rate is the sustained request rate in requests per second.
	Rate float64
	// Burst is the maximum burst size (token bucket capacity).
	Burst float64
}

// TokenBucket implements the token bucket algorithm for rate limiting.
// It is safe for concurrent use.
//
// Based on: docs/security.md §7.6 (Token Bucket Algorithm)
type TokenBucket struct {
	mu       sync.Mutex
	rate     float64   // tokens added per second
	burst    float64   // maximum tokens
	tokens   float64   // current tokens
	lastTime time.Time // last refill time
}

// NewTokenBucket creates a new token bucket with the given rate and burst.
func NewTokenBucket(rate, burst float64) *TokenBucket {
	return &TokenBucket{
		rate:     rate,
		burst:    burst,
		tokens:   burst, // start full
		lastTime: time.Now(),
	}
}

// Allow reports whether one request is allowed. If allowed, it consumes one
// token and returns true. Otherwise it returns false and retryAfter indicates
// how long the caller should wait before retrying.
func (tb *TokenBucket) Allow() (ok bool, retryAfter time.Duration) {
	return tb.AllowAt(time.Now())
}

// AllowAt is like Allow but uses the supplied time for testing determinism.
func (tb *TokenBucket) AllowAt(now time.Time) (ok bool, retryAfter time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(tb.lastTime).Seconds()
	if elapsed > 0 {
		tb.tokens = math.Min(tb.burst, tb.tokens+elapsed*tb.rate)
		tb.lastTime = now
	}

	if tb.tokens >= 1 {
		tb.tokens--
		return true, 0
	}

	// Calculate when the next token will be available.
	deficit := 1.0 - tb.tokens
	wait := time.Duration(deficit / tb.rate * float64(time.Second))
	return false, wait
}

// RateLimitMiddleware applies per-service rate limiting using a token bucket
// algorithm. When the rate limit is exceeded, it returns 429 Too Many Requests
// with a Retry-After header as required by TS 29.500.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting)
// 3GPP: TS 29.500 §6.10.4 — Overload Control Information
type RateLimitMiddleware struct {
	bucket *TokenBucket
	logger *slog.Logger
}

// NewRateLimitMiddleware creates rate limiting middleware for a specific service.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting Table)
func NewRateLimitMiddleware(cfg RateLimitConfig, logger *slog.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		bucket: NewTokenBucket(cfg.Rate, cfg.Burst),
		logger: logger,
	}
}

// Handler wraps the given handler with rate limiting. Requests that exceed
// the configured rate return 429 Too Many Requests with a Retry-After header.
//
// Based on: docs/security.md §7.6 (Per-NF Rate Limiting)
// 3GPP: TS 29.500 §6.10.4 — 429 response with Retry-After
func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retryAfter := m.bucket.Allow()
		if !ok {
			retrySeconds := int(math.Ceil(retryAfter.Seconds()))
			if retrySeconds < 1 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))

			m.logger.Warn("rate limit exceeded",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("retry_after_seconds", retrySeconds),
			)

			pd := udmerrors.NewTooManyRequests("rate limit exceeded, retry after " + strconv.Itoa(retrySeconds) + "s")
			pd.Cause = udmerrors.CauseNFCongestion
			udmerrors.WriteProblemDetails(w, pd)
			return
		}

		next.ServeHTTP(w, r)
	})
}
