// Package ssau implements the Nudm_SSAU service for Service-Specific
// Authorization in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
// 3GPP: TS 29.503 Nudm_SSAU — Service-Specific Authorization service operations
package ssau

import "encoding/json"

// ServiceSpecificAuthorizationInfo represents the request body for the
// ServiceSpecificAuthorization operation.
//
// 3GPP: TS 29.503 — ServiceSpecificAuthorizationInfo data type
type ServiceSpecificAuthorizationInfo struct {
	Snssai                json.RawMessage `json:"snssai,omitempty"`
	Dnn                   string          `json:"dnn,omitempty"`
	MtcProviderInfo       json.RawMessage `json:"mtcProviderInformation,omitempty"`
	AuthUpdateCallbackURI string          `json:"authUpdateCallbackUri,omitempty"`
	AfID                  string          `json:"afId,omitempty"`
	NefID                 string          `json:"nefId,omitempty"`
}

// ServiceSpecificAuthorizationData represents the response body for a
// successful ServiceSpecificAuthorization operation.
//
// 3GPP: TS 29.503 — ServiceSpecificAuthorizationData data type
type ServiceSpecificAuthorizationData struct {
	AuthorizationUeID json.RawMessage `json:"authorizationUeId,omitempty"`
	ExtGroupID        string          `json:"extGroupId,omitempty"`
	IntGroupID        string          `json:"intGroupId,omitempty"`
	AuthID            string          `json:"authId,omitempty"`
}

// ServiceSpecificAuthorizationRemoveData represents the request body for the
// ServiceSpecificAuthorizationRemoval operation.
//
// 3GPP: TS 29.503 — ServiceSpecificAuthorizationRemoveData data type
type ServiceSpecificAuthorizationRemoveData struct {
	AuthID string `json:"authId"`
}
