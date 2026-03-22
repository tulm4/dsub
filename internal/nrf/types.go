// Package nrf provides the NRF client library for UDM NF lifecycle management.
//
// Based on: docs/service-decomposition.md §3.6 (udm-nrf)
// 3GPP: TS 29.510 — NRF NF Management and Discovery APIs
// 3GPP: TS 29.500 — OAuth2 client credentials flow for SBI
package nrf

import "time"

// NFProfile represents the UDM's NF profile registered with the NRF.
//
// 3GPP: TS 29.510 — NFProfile data type
type NFProfile struct {
	NFInstanceID  string   `json:"nfInstanceId"`
	NFType        string   `json:"nfType"`
	NFStatus      string   `json:"nfStatus"`
	PLMNList      []PLMNID `json:"plmnList,omitempty"`
	NFServices    []NFService `json:"nfServices,omitempty"`
	FQDN          string   `json:"fqdn,omitempty"`
	IPV4Addresses []string `json:"ipv4Addresses,omitempty"`
	Capacity      int      `json:"capacity,omitempty"`
	Load          int      `json:"load,omitempty"`
	HeartbeatTimer int     `json:"heartBeatTimer,omitempty"`
}

// PLMNID identifies a Public Land Mobile Network.
//
// 3GPP: TS 29.510 — PlmnId data type
type PLMNID struct {
	MCC string `json:"mcc"`
	MNC string `json:"mnc"`
}

// NFService describes a service offered by an NF instance.
//
// 3GPP: TS 29.510 — NFService data type
type NFService struct {
	ServiceInstanceID string   `json:"serviceInstanceId"`
	ServiceName       string   `json:"serviceName"`
	Versions          []NFServiceVersion `json:"versions"`
	Scheme            string   `json:"scheme"`
	NFServiceStatus   string   `json:"nfServiceStatus"`
}

// NFServiceVersion describes the API version of an NF service.
//
// 3GPP: TS 29.510 — NFServiceVersion data type
type NFServiceVersion struct {
	APIVersionInURI string `json:"apiVersionInUri"`
	APIFullVersion  string `json:"apiFullVersion"`
}

// NFDiscoveryResult contains the result of an NF discovery query.
//
// 3GPP: TS 29.510 — SearchResult data type
type NFDiscoveryResult struct {
	ValidityPeriod int         `json:"validityPeriod,omitempty"`
	NFInstances    []NFProfile `json:"nfInstances"`
}

// OAuth2TokenRequest represents an OAuth2 client credentials token request.
//
// 3GPP: TS 29.510 — AccessTokenReq data type
type OAuth2TokenRequest struct {
	GrantType          string `json:"grant_type"`
	NFInstanceID       string `json:"nfInstanceId"`
	NFType             string `json:"nfType"`
	TargetNFType       string `json:"targetNfType"`
	Scope              string `json:"scope"`
	TargetNFInstanceID string `json:"targetNfInstanceId,omitempty"`
}

// OAuth2TokenResponse represents an OAuth2 access token response.
//
// 3GPP: TS 29.510 — AccessTokenRsp data type
type OAuth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// cachedToken holds an OAuth2 token with its expiry time.
type cachedToken struct {
	Token     string
	ExpiresAt time.Time
}

// DiscoveryEntry holds a cached NF discovery result with its expiry time.
type DiscoveryEntry struct {
	Result    *NFDiscoveryResult
	ExpiresAt time.Time
}
