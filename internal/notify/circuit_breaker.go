// Package notify provides the asynchronous notification and callback dispatch
// engine for the 5G UDM system.
//
// Based on: docs/service-decomposition.md §3.5 (udm-notify)
package notify

import (
	"sync"
	"time"
)

// circuitState represents the current state of a circuit breaker.
type circuitState int

const (
	stateClosed   circuitState = iota // Normal operation; requests flow through.
	stateOpen                         // Failures exceeded threshold; requests are rejected.
	stateHalfOpen                     // Probing; a single request is allowed to test recovery.
)

// circuitBreaker implements a per-destination circuit breaker that prevents
// cascading failures to unreachable NFs.
//
// State transitions:
//
//	closed  → open      when consecutive failures ≥ failureThreshold
//	open    → halfOpen  after halfOpenInterval elapses
//	halfOpen → closed   on success of the probe request
//	halfOpen → open     on failure of the probe request
type circuitBreaker struct {
	mu               sync.Mutex
	state            circuitState
	consecutiveFails int
	failureThreshold int
	lastFailure      time.Time
	halfOpenInterval time.Duration
	now              func() time.Time // Seam for testing.
}

// newCircuitBreaker creates a circuit breaker with the given configuration.
func newCircuitBreaker(cfg CircuitBreakerConfig) *circuitBreaker {
	return &circuitBreaker{
		state:            stateClosed,
		failureThreshold: cfg.FailureThreshold,
		halfOpenInterval: cfg.HalfOpenInterval,
		now:              time.Now,
	}
}

// allow reports whether the next request should be permitted.
//
// In the closed state all requests are allowed.  In the open state requests
// are rejected unless the half-open interval has elapsed, in which case the
// breaker transitions to half-open and allows a single probe request.
func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if cb.now().Sub(cb.lastFailure) >= cb.halfOpenInterval {
			cb.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		// Only one probe request is allowed while half-open; subsequent
		// callers are rejected until the probe completes.
		return false
	default:
		return false
	}
}

// recordSuccess resets the breaker to the closed state.
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0
	cb.state = stateClosed
}

// recordFailure increments the consecutive failure counter and trips the
// breaker to open when the threshold is reached.
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	cb.lastFailure = cb.now()

	if cb.consecutiveFails >= cb.failureThreshold {
		cb.state = stateOpen
	}
}
