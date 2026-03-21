// Package health provides Kubernetes health probe HTTP handlers for UDM services.
//
// Based on: docs/service-decomposition.md §3.2 (common/health)
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Checker represents a health check function.
// Returns nil if healthy, error with details if unhealthy.
type Checker func(ctx context.Context) error

// Status represents the result of a health check endpoint.
type Status struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
	Uptime float64           `json:"uptime_seconds,omitempty"`
}

// Handler provides HTTP handlers for Kubernetes health probes.
// Based on: docs/service-decomposition.md §3.2 (common/health)
type Handler struct {
	startTime       time.Time
	mu              sync.RWMutex
	startupChecks   map[string]Checker
	readinessChecks map[string]Checker
	livenessChecks  map[string]Checker
	ready           atomic.Bool
}

// NewHandler creates a new health check handler.
func NewHandler() *Handler {
	return &Handler{
		startTime:       time.Now(),
		startupChecks:   make(map[string]Checker),
		readinessChecks: make(map[string]Checker),
		livenessChecks:  make(map[string]Checker),
	}
}

// AddStartupCheck registers a named check for the startup probe.
func (h *Handler) AddStartupCheck(name string, check Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.startupChecks[name] = check
}

// AddReadinessCheck registers a named check for the readiness probe.
func (h *Handler) AddReadinessCheck(name string, check Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readinessChecks[name] = check
}

// AddLivenessCheck registers a named check for the liveness probe.
func (h *Handler) AddLivenessCheck(name string, check Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.livenessChecks[name] = check
}

// SetReady marks the service as ready (or not ready) after startup completes.
func (h *Handler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// RegisterRoutes registers health check endpoints on the provided mux.
//
// Endpoints:
//
//	/healthz/startup - Startup probe
//	/healthz/ready   - Readiness probe
//	/healthz/live    - Liveness probe
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz/startup", h.handleStartup)
	mux.HandleFunc("/healthz/ready", h.handleReadiness)
	mux.HandleFunc("/healthz/live", h.handleLiveness)
}

// handleStartup runs all startup checks and returns 200 if all pass, 503 otherwise.
func (h *Handler) handleStartup(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := make(map[string]Checker, len(h.startupChecks))
	for k, v := range h.startupChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	results, ok := runChecks(r.Context(), checks)

	status := Status{
		Status: "ready",
		Checks: results,
	}
	if !ok {
		status.Status = "unavailable"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleReadiness checks the ready flag and runs all readiness checks.
// Returns 200 if ready and all checks pass, 503 otherwise.
func (h *Handler) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if !h.ready.Load() {
		writeJSON(w, http.StatusServiceUnavailable, Status{
			Status: "not_ready",
		})
		return
	}

	h.mu.RLock()
	checks := make(map[string]Checker, len(h.readinessChecks))
	for k, v := range h.readinessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	results, ok := runChecks(r.Context(), checks)

	status := Status{
		Status: "ready",
		Checks: results,
	}
	if !ok {
		status.Status = "not_ready"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleLiveness runs all liveness checks and returns 200 if all pass, 500 otherwise.
func (h *Handler) handleLiveness(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := make(map[string]Checker, len(h.livenessChecks))
	for k, v := range h.livenessChecks {
		checks[k] = v
	}
	h.mu.RUnlock()

	results, ok := runChecks(r.Context(), checks)

	status := Status{
		Status: "alive",
		Checks: results,
		Uptime: time.Since(h.startTime).Seconds(),
	}
	if !ok {
		status.Status = "unhealthy"
		writeJSON(w, http.StatusInternalServerError, status)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// runChecks executes all checks and returns a map of results and an overall pass/fail.
func runChecks(ctx context.Context, checks map[string]Checker) (map[string]string, bool) {
	results := make(map[string]string, len(checks))
	ok := true
	for name, check := range checks {
		if err := check(ctx); err != nil {
			results[name] = err.Error()
			ok = false
		} else {
			results[name] = "ok"
		}
	}
	return results, ok
}

// writeJSON serializes status as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, code int, status Status) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(status) //nolint:errcheck
}
