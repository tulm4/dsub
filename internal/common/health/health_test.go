package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartupProbe(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]Checker
		wantCode   int
		wantStatus string
	}{
		{
			name: "all checks pass",
			checks: map[string]Checker{
				"db":    func(ctx context.Context) error { return nil },
				"cache": func(ctx context.Context) error { return nil },
			},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:       "no checks registered",
			checks:     map[string]Checker{},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name: "one check fails",
			checks: map[string]Checker{
				"db":    func(ctx context.Context) error { return errors.New("connection refused") },
				"cache": func(ctx context.Context) error { return nil },
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unavailable",
		},
		{
			name: "all checks fail",
			checks: map[string]Checker{
				"db":    func(ctx context.Context) error { return errors.New("connection refused") },
				"cache": func(ctx context.Context) error { return errors.New("timeout") },
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			for name, check := range tt.checks {
				h.AddStartupCheck(name, check)
			}

			req := httptest.NewRequest(http.MethodGet, "/healthz/startup", http.NoBody)
			rec := httptest.NewRecorder()

			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantCode)
			}

			var status Status
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if status.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status.Status, tt.wantStatus)
			}

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}
		})
	}
}

func TestReadinessProbe(t *testing.T) {
	tests := []struct {
		name       string
		ready      bool
		checks     map[string]Checker
		wantCode   int
		wantStatus string
	}{
		{
			name:  "ready with all checks passing",
			ready: true,
			checks: map[string]Checker{
				"db": func(ctx context.Context) error { return nil },
			},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:       "ready with no checks",
			ready:      true,
			checks:     map[string]Checker{},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:  "not ready",
			ready: false,
			checks: map[string]Checker{
				"db": func(ctx context.Context) error { return nil },
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not_ready",
		},
		{
			name:  "ready but check fails",
			ready: true,
			checks: map[string]Checker{
				"db": func(ctx context.Context) error { return errors.New("pool exhausted") },
			},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "not_ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			h.SetReady(tt.ready)
			for name, check := range tt.checks {
				h.AddReadinessCheck(name, check)
			}

			req := httptest.NewRequest(http.MethodGet, "/healthz/ready", http.NoBody)
			rec := httptest.NewRecorder()

			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantCode)
			}

			var status Status
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if status.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status.Status, tt.wantStatus)
			}
		})
	}
}

func TestLivenessProbe(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]Checker
		wantCode   int
		wantStatus string
		wantUptime bool
	}{
		{
			name:       "healthy with no checks",
			checks:     map[string]Checker{},
			wantCode:   http.StatusOK,
			wantStatus: "alive",
			wantUptime: true,
		},
		{
			name: "healthy with passing checks",
			checks: map[string]Checker{
				"goroutine_count": func(ctx context.Context) error { return nil },
			},
			wantCode:   http.StatusOK,
			wantStatus: "alive",
			wantUptime: true,
		},
		{
			name: "unhealthy with failing check",
			checks: map[string]Checker{
				"deadlock": func(ctx context.Context) error { return errors.New("deadlock detected") },
			},
			wantCode:   http.StatusInternalServerError,
			wantStatus: "unhealthy",
			wantUptime: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler()
			for name, check := range tt.checks {
				h.AddLivenessCheck(name, check)
			}

			req := httptest.NewRequest(http.MethodGet, "/healthz/live", http.NoBody)
			rec := httptest.NewRecorder()

			mux := http.NewServeMux()
			h.RegisterRoutes(mux)
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", rec.Code, tt.wantCode)
			}

			var status Status
			if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if status.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status.Status, tt.wantStatus)
			}
			if tt.wantUptime && status.Uptime <= 0 {
				t.Errorf("uptime_seconds = %f, want > 0", status.Uptime)
			}
		})
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := NewHandler()
	h.SetReady(true)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	endpoints := []struct {
		path     string
		wantCode int
	}{
		{"/healthz/startup", http.StatusOK},
		{"/healthz/ready", http.StatusOK},
		{"/healthz/live", http.StatusOK},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep.path, http.NoBody)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != ep.wantCode {
				t.Errorf("%s: status code = %d, want %d", ep.path, rec.Code, ep.wantCode)
			}

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("%s: Content-Type = %q, want %q", ep.path, ct, "application/json")
			}
		})
	}
}

func TestStartupCheckResults(t *testing.T) {
	h := NewHandler()
	h.AddStartupCheck("db", func(ctx context.Context) error { return nil })
	h.AddStartupCheck("cache", func(ctx context.Context) error { return errors.New("timeout") })

	req := httptest.NewRequest(http.MethodGet, "/healthz/startup", http.NoBody)
	rec := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rec, req)

	var status Status
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if status.Checks["db"] != "ok" {
		t.Errorf("checks[db] = %q, want %q", status.Checks["db"], "ok")
	}
	if status.Checks["cache"] != "timeout" {
		t.Errorf("checks[cache] = %q, want %q", status.Checks["cache"], "timeout")
	}
}
