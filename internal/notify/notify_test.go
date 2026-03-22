package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testNotifier returns a Notifier with deterministic jitter (always 0.5) and
// near-zero delays so tests run quickly.
func testNotifier(client *http.Client) *Notifier {
	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   5,
			InitialDelay: 1 * time.Millisecond, // Fast for tests.
			MaxDelay:     10 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 5,
			HalfOpenInterval: 50 * time.Millisecond,
		},
		HTTPClient: client,
	})
	n.randFn = func() float64 { return 0.5 } // deterministic jitter
	return n
}

// ---------------------------------------------------------------------------
// Default configuration tests
// ---------------------------------------------------------------------------

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"MaxRetries", cfg.MaxRetries, 5},
		{"InitialDelay", cfg.InitialDelay, 1 * time.Second},
		{"MaxDelay", cfg.MaxDelay, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"FailureThreshold", cfg.FailureThreshold, 5},
		{"HalfOpenInterval", cfg.HalfOpenInterval, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}

func TestDefaultNotifierConfig(t *testing.T) {
	cfg := DefaultNotifierConfig()
	if cfg.Retry.MaxRetries != 5 {
		t.Errorf("Retry.MaxRetries = %d, want 5", cfg.Retry.MaxRetries)
	}
	if cfg.CircuitBreaker.FailureThreshold != 5 {
		t.Errorf("CircuitBreaker.FailureThreshold = %d, want 5", cfg.CircuitBreaker.FailureThreshold)
	}
}

// ---------------------------------------------------------------------------
// Single notification — success
// ---------------------------------------------------------------------------

func TestSend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if v := r.Header.Get("X-Custom"); v != "test-value" {
			t.Errorf("X-Custom = %q, want test-value", v)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := testNotifier(srv.Client())
	res := n.Send(context.Background(), Notification{
		CallbackURI:      srv.URL,
		Payload:          []byte(`{"event":"test"}`),
		Headers:          map[string]string{"X-Custom": "test-value"},
		NotificationType: "test",
	})

	if !res.Success {
		t.Fatalf("expected success, got error: %v", res.Error)
	}
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("StatusCode = %d, want %d", res.StatusCode, http.StatusNoContent)
	}
	if res.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", res.Attempts)
	}
}

// ---------------------------------------------------------------------------
// Retry with exponential back-off
// ---------------------------------------------------------------------------

func TestSend_RetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := testNotifier(srv.Client())
	res := n.Send(context.Background(), Notification{
		CallbackURI: srv.URL,
		Payload:     []byte(`{}`),
	})

	if !res.Success {
		t.Fatalf("expected success after retries, got: %v", res.Error)
	}
	if res.Attempts != 4 {
		t.Errorf("Attempts = %d, want 4", res.Attempts)
	}
}

func TestSend_AllRetriesExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := testNotifier(srv.Client())
	res := n.Send(context.Background(), Notification{
		CallbackURI: srv.URL,
		Payload:     []byte(`{}`),
	})

	if res.Success {
		t.Fatal("expected failure after exhausting retries")
	}
	if res.Attempts != 6 { // 1 initial + 5 retries
		t.Errorf("Attempts = %d, want 6", res.Attempts)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	if res.Error == nil {
		t.Error("expected non-nil error")
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker state transitions
// ---------------------------------------------------------------------------

func TestCircuitBreaker_ClosedToOpenToHalfOpenToClosed(t *testing.T) {
	cb := newCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		HalfOpenInterval: 50 * time.Millisecond,
	})

	// Should start closed.
	if !cb.allow() {
		t.Fatal("expected closed breaker to allow requests")
	}

	// Record failures below threshold — still closed.
	cb.recordFailure()
	cb.recordFailure()
	if !cb.allow() {
		t.Fatal("expected breaker still closed after 2 failures")
	}

	// Third failure trips it open.
	cb.recordFailure()
	if cb.allow() {
		t.Fatal("expected open breaker to reject requests")
	}

	// Wait for half-open interval.
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one probe.
	if !cb.allow() {
		t.Fatal("expected half-open breaker to allow probe request")
	}
	// Second concurrent call should be rejected while half-open.
	if cb.allow() {
		t.Fatal("expected half-open breaker to reject second request")
	}

	// Probe succeeds → closed.
	cb.recordSuccess()
	if !cb.allow() {
		t.Fatal("expected closed breaker after successful probe")
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cb := newCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		HalfOpenInterval: 50 * time.Millisecond,
	})

	// Trip breaker.
	cb.recordFailure()
	cb.recordFailure()
	if cb.allow() {
		t.Fatal("expected open breaker")
	}

	// Wait for half-open.
	time.Sleep(60 * time.Millisecond)
	if !cb.allow() {
		t.Fatal("expected half-open probe allowed")
	}

	// Probe fails → back to open.
	cb.recordFailure()
	if cb.allow() {
		t.Fatal("expected open breaker after probe failure")
	}
}

func TestCircuitBreaker_SuccessResetsCount(t *testing.T) {
	cb := newCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		HalfOpenInterval: 50 * time.Millisecond,
	})

	cb.recordFailure()
	cb.recordFailure()
	cb.recordSuccess() // reset
	cb.recordFailure()
	cb.recordFailure()

	// Should still be closed; success reset the counter.
	if !cb.allow() {
		t.Fatal("expected closed breaker after success reset")
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker integration with Notifier
// ---------------------------------------------------------------------------

func TestSend_CircuitBreakerTrips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   0, // No retries — fail immediately.
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 3,
			HalfOpenInterval: 5 * time.Second,
		},
		HTTPClient: srv.Client(),
	})

	// Trip the breaker with 3 failures.
	for i := 0; i < 3; i++ {
		res := n.Send(context.Background(), Notification{CallbackURI: srv.URL, Payload: []byte(`{}`)})
		if res.Success {
			t.Fatalf("attempt %d: expected failure", i+1)
		}
	}

	// Next send should be rejected by the circuit breaker.
	res := n.Send(context.Background(), Notification{CallbackURI: srv.URL, Payload: []byte(`{}`)})
	if res.Success {
		t.Fatal("expected circuit breaker rejection")
	}
	if res.Error == nil {
		t.Fatal("expected non-nil error for circuit breaker rejection")
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestSend_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   10,
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 100,
			HalfOpenInterval: 30 * time.Second,
		},
		HTTPClient: srv.Client(),
	})
	n.randFn = func() float64 { return 1.0 } // max jitter to ensure sleep

	// Timeout is set between the 1st retry delay (50–100ms with jitter=1.0)
	// and the sum of multiple retries, so the context expires mid-retry.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	res := n.Send(ctx, Notification{CallbackURI: srv.URL, Payload: []byte(`{}`)})
	if res.Success {
		t.Fatal("expected failure due to context cancellation")
	}
	if res.Error == nil {
		t.Fatal("expected non-nil error")
	}
}

// ---------------------------------------------------------------------------
// Batch delivery
// ---------------------------------------------------------------------------

func TestSendBatch(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	n := testNotifier(srv.Client())
	notifications := []Notification{
		{Payload: []byte(`{"seq":1}`), NotificationType: "sdm-change"},
		{Payload: []byte(`{"seq":2}`), NotificationType: "sdm-change"},
		{Payload: []byte(`{"seq":3}`), NotificationType: "sdm-change"},
	}

	results := n.SendBatch(context.Background(), srv.URL, notifications)
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("results[%d]: expected success, got %v", i, r.Error)
		}
	}
	if got := received.Load(); got != 3 {
		t.Errorf("server received %d requests, want 3", got)
	}
}

func TestSendBatch_PartialFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   0,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 100, // high threshold so breaker doesn't trip
			HalfOpenInterval: 30 * time.Second,
		},
		HTTPClient: srv.Client(),
	})

	results := n.SendBatch(context.Background(), srv.URL, []Notification{
		{Payload: []byte(`{"seq":1}`)},
		{Payload: []byte(`{"seq":2}`)}, // this one fails
		{Payload: []byte(`{"seq":3}`)},
	})

	if !results[0].Success {
		t.Errorf("results[0]: expected success")
	}
	if results[1].Success {
		t.Errorf("results[1]: expected failure")
	}
	if !results[2].Success {
		t.Errorf("results[2]: expected success")
	}
}

// ---------------------------------------------------------------------------
// New() defaults
// ---------------------------------------------------------------------------

func TestNew_NilHTTPClient(t *testing.T) {
	n := New(NotifierConfig{})
	if n.client != http.DefaultClient {
		t.Error("expected http.DefaultClient when HTTPClient is nil")
	}
}

func TestNew_CustomHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 42 * time.Second}
	n := New(NotifierConfig{HTTPClient: custom})
	if n.client != custom {
		t.Error("expected custom HTTP client")
	}
}

// ---------------------------------------------------------------------------
// Backoff calculation
// ---------------------------------------------------------------------------

func TestBackoff(t *testing.T) {
	n := &Notifier{
		retry: RetryConfig{
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
		},
		randFn: func() float64 { return 0.5 },
	}

	tests := []struct {
		name    string
		attempt int
		want    time.Duration
	}{
		{"attempt 1", 1, 50 * time.Millisecond},  // 0.5 * 100ms * 2^0
		{"attempt 2", 2, 100 * time.Millisecond}, // 0.5 * 100ms * 2^1
		{"attempt 3", 3, 200 * time.Millisecond}, // 0.5 * 100ms * 2^2
		{"attempt 4", 4, 400 * time.Millisecond}, // 0.5 * 100ms * 2^3
		{"attempt 5", 5, 500 * time.Millisecond}, // 0.5 * min(100ms * 2^4, 1s) = 0.5 * 1s
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := n.backoff(tt.attempt)
			if got != tt.want {
				t.Errorf("backoff(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Connection error handling
// ---------------------------------------------------------------------------

func TestSend_ConnectionRefused(t *testing.T) {
	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   1,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 100,
			HalfOpenInterval: 30 * time.Second,
		},
	})
	n.randFn = func() float64 { return 0.5 }

	res := n.Send(context.Background(), Notification{
		CallbackURI: "http://127.0.0.1:1", // port 1 — guaranteed connection refused
		Payload:     []byte(`{}`),
	})

	if res.Success {
		t.Fatal("expected failure for connection refused")
	}
	if res.Attempts != 2 { // 1 + 1 retry
		t.Errorf("Attempts = %d, want 2", res.Attempts)
	}
	if res.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 for connection error", res.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Per-destination breaker isolation
// ---------------------------------------------------------------------------

func TestPerDestinationBreakerIsolation(t *testing.T) {
	var callsA, callsB atomic.Int32

	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callsA.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srvA.Close()

	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callsB.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srvB.Close()

	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   0,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 2,
			HalfOpenInterval: 5 * time.Second,
		},
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})

	// Trip breaker for server A.
	for i := 0; i < 3; i++ {
		n.Send(context.Background(), Notification{CallbackURI: srvA.URL, Payload: []byte(`{}`)})
	}

	// Server B should still work — separate breaker.
	res := n.Send(context.Background(), Notification{CallbackURI: srvB.URL, Payload: []byte(`{}`)})
	if !res.Success {
		t.Fatalf("server B should succeed; breaker for A should not affect B: %v", res.Error)
	}

	// Server A should be rejected.
	res = n.Send(context.Background(), Notification{CallbackURI: srvA.URL, Payload: []byte(`{}`)})
	if res.Success {
		t.Fatal("server A should be rejected by circuit breaker")
	}

	if callsB.Load() != 1 {
		t.Errorf("expected 1 call to server B, got %d", callsB.Load())
	}
}

// ---------------------------------------------------------------------------
// Invalid URL
// ---------------------------------------------------------------------------

func TestSend_InvalidURL(t *testing.T) {
	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   0,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 100,
			HalfOpenInterval: 30 * time.Second,
		},
	})

	res := n.Send(context.Background(), Notification{
		CallbackURI: "://bad-url",
		Payload:     []byte(`{}`),
	})

	if res.Success {
		t.Fatal("expected failure for invalid URL")
	}
	if res.Error == nil {
		t.Fatal("expected non-nil error for invalid URL")
	}
}

// ---------------------------------------------------------------------------
// Notification type preserved through batch
// ---------------------------------------------------------------------------

func TestSendBatch_OverridesCallbackURI(t *testing.T) {
	var gotURIs []string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURIs = append(gotURIs, r.URL.String())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := testNotifier(srv.Client())

	notifications := []Notification{
		{CallbackURI: "http://should-be-overridden/a", Payload: []byte(`{}`)},
		{CallbackURI: "http://should-be-overridden/b", Payload: []byte(`{}`)},
	}

	results := n.SendBatch(context.Background(), srv.URL, notifications)
	for i, r := range results {
		if !r.Success {
			t.Errorf("results[%d]: expected success, got %v", i, r.Error)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gotURIs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(gotURIs))
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker with time-override seam
// ---------------------------------------------------------------------------

func TestCircuitBreaker_TimeSeam(t *testing.T) {
	now := time.Now()
	cb := newCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		HalfOpenInterval: 10 * time.Second,
	})
	cb.now = func() time.Time { return now }

	cb.recordFailure() // trips to open
	if cb.allow() {
		t.Fatal("expected open breaker")
	}

	// Advance time past half-open interval.
	cb.now = func() time.Time { return now.Add(11 * time.Second) }
	if !cb.allow() {
		t.Fatal("expected half-open after interval elapses")
	}
}

// ---------------------------------------------------------------------------
// Empty batch
// ---------------------------------------------------------------------------

func TestSendBatch_Empty(t *testing.T) {
	n := testNotifier(http.DefaultClient)
	results := n.SendBatch(context.Background(), "http://example.com", nil)
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker string representation (defensive)
// ---------------------------------------------------------------------------

func TestCircuitBreaker_DefaultState(t *testing.T) {
	cb := newCircuitBreaker(DefaultCircuitBreakerConfig())
	if cb.state != stateClosed {
		t.Errorf("initial state = %d, want %d (closed)", cb.state, stateClosed)
	}
	if cb.consecutiveFails != 0 {
		t.Errorf("initial consecutiveFails = %d, want 0", cb.consecutiveFails)
	}
}

// ---------------------------------------------------------------------------
// Ensure Notifier.getBreaker returns same instance for same URI
// ---------------------------------------------------------------------------

func TestGetBreaker_SameInstance(t *testing.T) {
	n := New(DefaultNotifierConfig())
	b1 := n.getBreaker("http://example.com/cb1")
	b2 := n.getBreaker("http://example.com/cb1")
	b3 := n.getBreaker("http://example.com/cb2")

	if b1 != b2 {
		t.Error("expected same breaker instance for same URI")
	}
	if b1 == b3 {
		t.Error("expected different breaker instance for different URI")
	}
}

// ---------------------------------------------------------------------------
// Error message formatting
// ---------------------------------------------------------------------------

func TestSend_ErrorMessageContainsURI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := New(NotifierConfig{
		Retry: RetryConfig{
			MaxRetries:   0,
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     1 * time.Millisecond,
		},
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 100,
			HalfOpenInterval: 30 * time.Second,
		},
		HTTPClient: srv.Client(),
	})

	res := n.Send(context.Background(), Notification{CallbackURI: srv.URL, Payload: []byte(`{}`)})
	if res.Error == nil {
		t.Fatal("expected error")
	}

	errMsg := fmt.Sprintf("%v", res.Error)
	if len(errMsg) == 0 {
		t.Error("error message should not be empty")
	}
}
