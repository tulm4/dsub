package nrf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewClient verifies client construction.
func TestNewClient(t *testing.T) {
	cfg := DefaultClientConfig()
	cfg.NRFURL = "http://nrf.example.com"
	cfg.NFProfile = NFProfile{
		NFInstanceID: "test-instance",
		NFType:       "UDM",
		NFStatus:     "REGISTERED",
	}

	c := NewClient(cfg)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("default HeartbeatInterval: got %v, want 30s", c.cfg.HeartbeatInterval)
	}
}

// TestRegister tests NF registration with the NRF.
func TestRegister(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{
		NFInstanceID: "test-nf-001",
		NFType:       "UDM",
		NFStatus:     "REGISTERED",
	}

	c := NewClient(cfg)
	if err := c.Register(context.Background()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("method: got %s, want PUT", gotMethod)
	}
	if gotPath != "/nnrf-nfm/v1/nf-instances/test-nf-001" {
		t.Errorf("path: got %s, want /nnrf-nfm/v1/nf-instances/test-nf-001", gotPath)
	}
}

// TestRegister_ServerError tests registration failure handling.
func TestRegister_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001"}

	c := NewClient(cfg)
	err := c.Register(context.Background())
	if err == nil {
		t.Fatal("expected error from Register, got nil")
	}
}

// TestDeregister tests NF deregistration from the NRF.
func TestDeregister(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001"}

	c := NewClient(cfg)
	if err := c.Deregister(context.Background()); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method: got %s, want DELETE", gotMethod)
	}
}

// TestHeartbeat tests NF heartbeat PATCH.
func TestHeartbeat(t *testing.T) {
	var gotMethod, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001", Load: 50}

	c := NewClient(cfg)
	if err := c.Heartbeat(context.Background()); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method: got %s, want PATCH", gotMethod)
	}
	if gotContentType != "application/json-patch+json" {
		t.Errorf("content-type: got %s, want application/json-patch+json", gotContentType)
	}
}

// TestStartHeartbeat_Stop tests background heartbeat start/stop lifecycle.
func TestStartHeartbeat_Stop(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.HeartbeatInterval = 50 * time.Millisecond
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001"}

	c := NewClient(cfg)
	c.StartHeartbeat(context.Background())

	// Wait for at least a couple heartbeats
	time.Sleep(200 * time.Millisecond)
	c.Stop()

	count := callCount.Load()
	if count < 1 {
		t.Errorf("expected at least 1 heartbeat call, got %d", count)
	}
}

// TestDiscover tests NF discovery with caching.
func TestDiscover(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		result := NFDiscoveryResult{
			ValidityPeriod: 60,
			NFInstances: []NFProfile{
				{NFInstanceID: "ausf-001", NFType: "AUSF", NFStatus: "REGISTERED"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001"}

	c := NewClient(cfg)

	// First call should hit the server
	result, err := c.Discover(context.Background(), "AUSF")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.NFInstances) != 1 {
		t.Fatalf("expected 1 NF instance, got %d", len(result.NFInstances))
	}
	if result.NFInstances[0].NFInstanceID != "ausf-001" {
		t.Errorf("NF instance ID: got %q, want %q", result.NFInstances[0].NFInstanceID, "ausf-001")
	}

	// Second call should use cache (no additional server request)
	_, err = c.Discover(context.Background(), "AUSF")
	if err != nil {
		t.Fatalf("Discover (cached): %v", err)
	}

	if requestCount.Load() != 1 {
		t.Errorf("expected 1 server request (cache hit), got %d", requestCount.Load())
	}
}

// TestGetAccessToken tests OAuth2 token acquisition with caching.
func TestGetAccessToken(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		resp := OAuth2TokenResponse{
			AccessToken: "test-token-abc123",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
			Scope:       "nudm-sdm",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001", NFType: "UDM"}

	c := NewClient(cfg)

	// First call should hit the server
	token, err := c.GetAccessToken(context.Background(), "nudm-sdm")
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if token != "test-token-abc123" {
		t.Errorf("token: got %q, want %q", token, "test-token-abc123")
	}

	// Second call should use cache
	token2, err := c.GetAccessToken(context.Background(), "nudm-sdm")
	if err != nil {
		t.Fatalf("GetAccessToken (cached): %v", err)
	}
	if token2 != "test-token-abc123" {
		t.Errorf("cached token: got %q, want %q", token2, "test-token-abc123")
	}

	if requestCount.Load() != 1 {
		t.Errorf("expected 1 server request (cache hit), got %d", requestCount.Load())
	}
}

// TestGetAccessToken_ServerError tests token acquisition failure handling.
func TestGetAccessToken_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL
	cfg.NFProfile = NFProfile{NFInstanceID: "test-nf-001", NFType: "UDM"}

	c := NewClient(cfg)
	_, err := c.GetAccessToken(context.Background(), "nudm-sdm")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDiscover_ServerError tests discovery failure handling.
func TestDiscover_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service unavailable"))
	}))
	defer server.Close()

	cfg := DefaultClientConfig()
	cfg.NRFURL = server.URL

	c := NewClient(cfg)
	_, err := c.Discover(context.Background(), "AUSF")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
