package nrf

// OAuth2 token acquisition and caching for SBI authentication.
//
// Based on: docs/sbi-api-design.md §6 (Authentication and Authorization)
// 3GPP: TS 29.510 — AccessTokenReq / AccessTokenRsp
// 3GPP: TS 29.500 — OAuth2 client credentials flow for SBI

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GetAccessToken obtains an OAuth2 access token from the NRF for the given
// scope. Tokens are cached with TTL based on the expires_in value.
//
// Based on: docs/sbi-api-design.md §6
// 3GPP: TS 29.510 §5.4.2.2 — Access Token Request
// Method: POST /oauth2/token (NRF token endpoint)
func (c *Client) GetAccessToken(ctx context.Context, scope string) (string, error) {
	// Check token cache first
	c.mu.RLock()
	cached, ok := c.tokenCache[scope]
	c.mu.RUnlock()

	if ok && time.Now().Before(cached.ExpiresAt) {
		return cached.Token, nil
	}

	// Cache miss or expired — request new token from NRF
	tokenURL := fmt.Sprintf("%s/oauth2/token", c.cfg.NRFURL)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"nfInstanceId":  {c.cfg.NFProfile.NFInstanceID},
		"nfType":        {c.cfg.NFProfile.NFType},
		"targetNfType":  {"UDM"},
		"scope":         {scope},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("nrf: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("nrf: token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nrf: token request returned status %d", resp.StatusCode)
	}

	var tokenResp OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("nrf: decode token response: %w", err)
	}

	// Cache the token with a safety margin (subtract 30s from expiry to avoid edge-case expiry)
	expiry := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiry > 30*time.Second {
		expiry -= 30 * time.Second
	}

	c.mu.Lock()
	c.tokenCache[scope] = &cachedToken{
		Token:     tokenResp.AccessToken,
		ExpiresAt: time.Now().Add(expiry),
	}
	c.mu.Unlock()

	return tokenResp.AccessToken, nil
}
