// Package niddau implements the Nudm_NIDDAU service for NIDD (Non-IP Data
// Delivery) Authorization in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.8 (udm-niddau)
// 3GPP: TS 29.503 Nudm_NIDDAU — NIDD Authorization service operations
package niddau

import "encoding/json"

// AuthorizationInfo represents the request body for the AuthorizeNiddData
// operation.
//
// 3GPP: TS 29.503 — AuthorizationInfo data type
type AuthorizationInfo struct {
	Snssai                json.RawMessage `json:"snssai,omitempty"`
	Dnn                   string          `json:"dnn,omitempty"`
	MtcProviderInfo       json.RawMessage `json:"mtcProviderInformation,omitempty"`
	AuthUpdateCallbackURI string          `json:"authUpdateCallbackUri,omitempty"`
	AfID                  string          `json:"afId,omitempty"`
	NefID                 string          `json:"nefId,omitempty"`
	ValidityTime          string          `json:"validityTime,omitempty"`
	ContextInfo           json.RawMessage `json:"contextInfo,omitempty"`
}

// AuthorizationData represents the response body for a successful
// AuthorizeNiddData operation.
//
// 3GPP: TS 29.503 — AuthorizationData data type
type AuthorizationData struct {
	AuthorizationData []NiddAuthorizationInfo `json:"authorizationData,omitempty"`
	ValidityTime      string                  `json:"validityTime,omitempty"`
}

// NiddAuthorizationInfo represents a single authorized UE in the response.
//
// 3GPP: TS 29.503 — NiddAuthorizationInfo data type
type NiddAuthorizationInfo struct {
	Supi         string `json:"supi,omitempty"`
	Gpsi         string `json:"gpsi,omitempty"`
	ValidityTime string `json:"validityTime,omitempty"`
}
