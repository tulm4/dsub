package nrf

// NRF client for UDM NF lifecycle management: registration, heartbeat,
// discovery, OAuth2 token acquisition, and deregistration.
//
// Based on: docs/service-decomposition.md §3.6 (udm-nrf)
// 3GPP: TS 29.510 — NRF NF Management and Discovery APIs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ClientConfig holds configuration for the NRF client.
type ClientConfig struct {
	// NRFURL is the base URL of the NRF (e.g., "https://nrf.5gc.mnc001.mcc001.3gppnetwork.org").
	NRFURL string
	// NFProfile is the UDM's NF profile to register with the NRF.
	NFProfile NFProfile
	// HeartbeatInterval is the interval between heartbeat PATCHes (default: 30s).
	HeartbeatInterval time.Duration
	// RequestTimeout is the HTTP request timeout for NRF API calls.
	RequestTimeout time.Duration
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		HeartbeatInterval: 30 * time.Second,
		RequestTimeout:    5 * time.Second,
	}
}

// Client implements the NRF client for NF lifecycle management.
//
// Based on: docs/service-decomposition.md §3.6
// 3GPP: TS 29.510 — NRF NF Management and Discovery
type Client struct {
	cfg    ClientConfig
	http   *http.Client
	cancel context.CancelFunc

	mu             sync.RWMutex
	running        bool                      // guards against multiple StartHeartbeat calls
	discoveryCache map[string]*DiscoveryEntry // keyed by targetNFType
	tokenCache     map[string]*cachedToken    // keyed by "targetNFType:scope"
}

// NewClient creates a new NRF client.
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		discoveryCache: make(map[string]*DiscoveryEntry),
		tokenCache:     make(map[string]*cachedToken),
	}
}

// Register registers the UDM NF instance with the NRF.
//
// 3GPP: TS 29.510 §5.2.2.2 — NF Register
// Method: PUT /nnrf-nfm/v1/nf-instances/{nfInstanceId}
func (c *Client) Register(ctx context.Context) error {
	url := fmt.Sprintf("%s/nnrf-nfm/v1/nf-instances/%s", c.cfg.NRFURL, c.cfg.NFProfile.NFInstanceID)

	body, err := json.Marshal(c.cfg.NFProfile)
	if err != nil {
		return fmt.Errorf("nrf: marshal NF profile: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nrf: create register request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nrf: register request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nrf: register failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Deregister removes the UDM NF instance from the NRF.
//
// 3GPP: TS 29.510 §5.2.2.4 — NF Deregister
// Method: DELETE /nnrf-nfm/v1/nf-instances/{nfInstanceId}
func (c *Client) Deregister(ctx context.Context) error {
	url := fmt.Sprintf("%s/nnrf-nfm/v1/nf-instances/%s", c.cfg.NRFURL, c.cfg.NFProfile.NFInstanceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("nrf: create deregister request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nrf: deregister request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nrf: deregister failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Heartbeat sends a periodic heartbeat PATCH to the NRF to maintain registration.
//
// 3GPP: TS 29.510 §5.2.2.3 — NF Heartbeat (Update)
// Method: PATCH /nnrf-nfm/v1/nf-instances/{nfInstanceId}
func (c *Client) Heartbeat(ctx context.Context) error {
	url := fmt.Sprintf("%s/nnrf-nfm/v1/nf-instances/%s", c.cfg.NRFURL, c.cfg.NFProfile.NFInstanceID)

	patch := []map[string]any{
		{
			"op":    "replace",
			"path":  "/nfStatus",
			"value": "REGISTERED",
		},
		{
			"op":    "replace",
			"path":  "/load",
			"value": c.cfg.NFProfile.Load,
		},
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("nrf: marshal heartbeat patch: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("nrf: create heartbeat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json-patch+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nrf: heartbeat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nrf: heartbeat failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// StartHeartbeat begins periodic heartbeat in a background goroutine.
// Call Stop() to cancel. The heartbeat goroutine runs until the context is cancelled.
// Calling StartHeartbeat multiple times is safe — subsequent calls are no-ops
// if a heartbeat loop is already running.
func (c *Client) StartHeartbeat(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	hbCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	go func() {
		ticker := time.NewTicker(c.cfg.HeartbeatInterval)
		defer ticker.Stop()
		defer func() {
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
		}()

		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				// Best-effort heartbeat; errors are silently discarded to avoid
				// stopping the loop. In production, add structured logging here.
				_ = c.Heartbeat(hbCtx)
			}
		}
	}()
}

// Stop cancels the heartbeat goroutine.
func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Discover queries the NRF for NF instances of the given type. Results are
// cached locally with TTL based on the NRF's validityPeriod.
//
// 3GPP: TS 29.510 §5.3.2.2 — NF Discovery
// Method: GET /nnrf-disc/v1/nf-instances?target-nf-type={type}
func (c *Client) Discover(ctx context.Context, targetNFType string) (*NFDiscoveryResult, error) {
	// Check cache first
	c.mu.RLock()
	entry, ok := c.discoveryCache[targetNFType]
	c.mu.RUnlock()

	if ok && time.Now().Before(entry.ExpiresAt) {
		return entry.Result, nil
	}

	// Cache miss or expired — query NRF
	url := fmt.Sprintf("%s/nnrf-disc/v1/nf-instances?target-nf-type=%s&requester-nf-type=UDM",
		c.cfg.NRFURL, targetNFType)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("nrf: create discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nrf: discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("nrf: discovery failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result NFDiscoveryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("nrf: decode discovery response: %w", err)
	}

	// Cache the result
	ttl := time.Duration(result.ValidityPeriod) * time.Second
	if ttl <= 0 {
		ttl = 60 * time.Second // default cache TTL
	}

	c.mu.Lock()
	c.discoveryCache[targetNFType] = &DiscoveryEntry{
		Result:    &result,
		ExpiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	return &result, nil
}
