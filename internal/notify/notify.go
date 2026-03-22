// Package notify provides the asynchronous notification and callback dispatch
// engine for the 5G UDM system.
//
// Based on: docs/service-decomposition.md §3.5 (udm-notify)
//
// The Notifier sends HTTP/2 POST callbacks to NF consumer URIs with 3GPP
// compliant notification payloads.  It includes:
//   - Exponential back-off with jitter (initial 1 s, max 30 s, up to 5 retries)
//   - Per-destination circuit breaker (trip after 5 consecutive failures,
//     half-open probe every 30 s)
//   - Batch delivery for same-destination notifications
//
// 3GPP: TS 29.503 — Nudm notification callbacks
package notify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Configuration types
// ---------------------------------------------------------------------------

// RetryConfig controls exponential back-off retry behaviour.
type RetryConfig struct {
	MaxRetries   int           // Maximum number of retry attempts (default 5).
	InitialDelay time.Duration // Base delay before the first retry (default 1 s).
	MaxDelay     time.Duration // Upper bound on the back-off delay (default 30 s).
}

// CircuitBreakerConfig controls the per-destination circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // Consecutive failures before the breaker opens (default 5).
	HalfOpenInterval time.Duration // How long to wait before a half-open probe (default 30 s).
}

// NotifierConfig is the top-level configuration for a Notifier.
type NotifierConfig struct {
	Retry          RetryConfig
	CircuitBreaker CircuitBreakerConfig
	HTTPClient     *http.Client // Optional; uses http.DefaultClient when nil.
}

// DefaultRetryConfig returns the design-default retry configuration.
//
// Based on: docs/service-decomposition.md §3.5
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
	}
}

// DefaultCircuitBreakerConfig returns the design-default circuit breaker
// configuration.
//
// Based on: docs/service-decomposition.md §3.5
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		HalfOpenInterval: 30 * time.Second,
	}
}

// DefaultNotifierConfig returns a NotifierConfig with all design defaults.
func DefaultNotifierConfig() NotifierConfig {
	return NotifierConfig{
		Retry:          DefaultRetryConfig(),
		CircuitBreaker: DefaultCircuitBreakerConfig(),
	}
}

// ---------------------------------------------------------------------------
// Notification & Result
// ---------------------------------------------------------------------------

// Notification represents a single callback to be dispatched to an NF consumer.
type Notification struct {
	CallbackURI      string            // Target NF callback URI.
	Payload          []byte            // JSON body.
	Headers          map[string]string // Additional HTTP headers (e.g. 3GPP SBI headers).
	NotificationType string            // Logical type (e.g. "sdm-change", "dereg").
}

// Result captures the outcome of a notification dispatch attempt.
type Result struct {
	Success    bool  // true when the server returned 2xx.
	StatusCode int   // HTTP status code of the final attempt (0 if the request never reached the server).
	Error      error // Non-nil on failure.
	Attempts   int   // Total number of attempts (initial + retries).
}

// ---------------------------------------------------------------------------
// Notifier
// ---------------------------------------------------------------------------

// Notifier dispatches HTTP/2 POST callbacks to NF consumer callback URIs with
// retry and circuit-breaker protection.
type Notifier struct {
	client *http.Client
	retry  RetryConfig
	cbCfg  CircuitBreakerConfig
	randFn func() float64 // Jitter source; seam for deterministic tests.

	mu       sync.Mutex
	breakers map[string]*circuitBreaker // keyed by callback URI
}

// New creates a Notifier with the supplied configuration.
func New(cfg NotifierConfig) *Notifier {
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &Notifier{
		client:   client,
		retry:    cfg.Retry,
		cbCfg:    cfg.CircuitBreaker,
		breakers: make(map[string]*circuitBreaker),
		randFn:   rand.Float64,
	}
}

// getBreaker returns (or lazily creates) the circuit breaker for a given URI.
func (n *Notifier) getBreaker(uri string) *circuitBreaker {
	n.mu.Lock()
	defer n.mu.Unlock()

	cb, ok := n.breakers[uri]
	if !ok {
		cb = newCircuitBreaker(n.cbCfg)
		n.breakers[uri] = cb
	}
	return cb
}

// Send dispatches a single notification to the callback URI, applying retry
// with exponential back-off and circuit breaker logic.
//
// Based on: docs/service-decomposition.md §3.5
func (n *Notifier) Send(ctx context.Context, notif Notification) Result {
	cb := n.getBreaker(notif.CallbackURI)

	var (
		lastStatus int
		lastErr    error
	)

	maxAttempts := 1 + n.retry.MaxRetries
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Circuit breaker gate.
		if !cb.allow() {
			return Result{
				Success:    false,
				StatusCode: lastStatus,
				Error:      fmt.Errorf("notify: circuit breaker open for %s", notif.CallbackURI),
				Attempts:   attempt,
			}
		}

		status, err := n.doPost(ctx, notif)
		lastStatus = status
		lastErr = err

		if err == nil && status >= 200 && status < 300 {
			cb.recordSuccess()
			return Result{
				Success:    true,
				StatusCode: status,
				Attempts:   attempt,
			}
		}

		cb.recordFailure()

		// Don't sleep after the last attempt.
		if attempt < maxAttempts {
			delay := n.backoff(attempt)
			select {
			case <-ctx.Done():
				return Result{
					Success:    false,
					StatusCode: lastStatus,
					Error:      fmt.Errorf("notify: context cancelled during retry: %w", ctx.Err()),
					Attempts:   attempt,
				}
			case <-time.After(delay):
			}
		}
	}

	return Result{
		Success:    false,
		StatusCode: lastStatus,
		Error:      fmt.Errorf("notify: exhausted %d attempts for %s: %w", maxAttempts, notif.CallbackURI, lastErr),
		Attempts:   maxAttempts,
	}
}

// SendBatch dispatches multiple notifications destined for the same callback
// URI.  Each notification is sent individually through Send so that it benefits
// from the per-destination circuit breaker.
//
// Based on: docs/service-decomposition.md §3.5 (batch delivery)
func (n *Notifier) SendBatch(ctx context.Context, callbackURI string, notifications []Notification) []Result {
	results := make([]Result, len(notifications))
	for i, notif := range notifications {
		notif.CallbackURI = callbackURI
		results[i] = n.Send(ctx, notif)
	}
	return results
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// doPost performs a single HTTP POST to the notification's callback URI.
func (n *Notifier) doPost(ctx context.Context, notif Notification) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notif.CallbackURI, bytes.NewReader(notif.Payload))
	if err != nil {
		return 0, fmt.Errorf("notify: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range notif.Headers {
		req.Header.Set(k, v)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("notify: POST %s: %w", notif.CallbackURI, err)
	}
	defer func() {
		// Drain and close body to allow connection reuse.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, nil
	}

	return resp.StatusCode, fmt.Errorf("notify: POST %s returned %d", notif.CallbackURI, resp.StatusCode)
}

// backoff computes the delay before retry attempt n (1-indexed), using
// exponential back-off with full jitter.
//
//	delay = rand(0, min(maxDelay, initialDelay * 2^(attempt-1)))
func (n *Notifier) backoff(attempt int) time.Duration {
	base := float64(n.retry.InitialDelay) * math.Pow(2, float64(attempt-1))
	if base > float64(n.retry.MaxDelay) {
		base = float64(n.retry.MaxDelay)
	}
	return time.Duration(n.randFn() * base)
}
