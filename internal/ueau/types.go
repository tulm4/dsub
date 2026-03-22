// Package ueau implements the Nudm_UEAU service for UE Authentication
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau)
// 3GPP: TS 29.503 Nudm_UEAU — UE Authentication service operations
// 3GPP: TS 33.501 §6.1 — 5G security architecture and procedures
package ueau

// AuthenticationInfoRequest is the POST body for the GenerateAuthData operation.
//
// 3GPP: TS 29.503 — AuthenticationInfoRequest data type
type AuthenticationInfoRequest struct {
	ServingNetworkName    string                 `json:"servingNetworkName"`
	AusfInstanceID        string                 `json:"ausfInstanceId"`
	ResynchronizationInfo *ResynchronizationInfo `json:"resynchronizationInfo,omitempty"`
	SupportedFeatures     string                 `json:"supportedFeatures,omitempty"`
}

// ResynchronizationInfo carries RAND and AUTS for SQN resynchronization.
//
// 3GPP: TS 29.503 — ResynchronizationInfo data type
// 3GPP: TS 33.501 §6.1.3.4 — SQN resynchronization procedure
type ResynchronizationInfo struct {
	Rand string `json:"rand"`
	Auts string `json:"auts"`
}

// AuthenticationInfoResult is the response for the GenerateAuthData operation.
//
// 3GPP: TS 29.503 — AuthenticationInfoResult data type
type AuthenticationInfoResult struct {
	AuthType             string                `json:"authType"`
	AuthenticationVector *AuthenticationVector `json:"authenticationVector,omitempty"`
	Supi                 string                `json:"supi,omitempty"`
	SupportedFeatures    string                `json:"supportedFeatures,omitempty"`
}

// AuthenticationVector carries the 5G authentication vector (5G HE AV).
//
// 3GPP: TS 29.503 — Av5gAka data type
// 3GPP: TS 33.501 §6.1.3.2 — Authentication vector generation
type AuthenticationVector struct {
	AvType   string `json:"avType"`
	Rand     string `json:"rand"`
	Autn     string `json:"autn"`
	XresStar string `json:"xresStar"`
	Kausf    string `json:"kausf"`
}

// AuthEvent represents an authentication event confirmation or deletion.
//
// 3GPP: TS 29.503 — AuthEvent data type
type AuthEvent struct {
	NfInstanceID       string `json:"nfInstanceId"`
	Success            bool   `json:"success"`
	TimeStamp          string `json:"timeStamp"`
	AuthType           string `json:"authType"`
	ServingNetworkName string `json:"servingNetworkName"`
	AuthRemovalInd     bool   `json:"authRemovalInd,omitempty"`
}

// RgAuthCtx represents Residential Gateway authentication context data.
//
// 3GPP: TS 29.503 — RgAuthCtx data type
type RgAuthCtx struct {
	AuthInd          bool   `json:"authInd"`
	Supi             string `json:"supi,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// HssAuthenticationInfoRequest is the POST body for GenerateAv (HSS interworking).
//
// 3GPP: TS 29.503 — HssAuthenticationInfoRequest data type
type HssAuthenticationInfoRequest struct {
	NumOfRequestedVectors  int                    `json:"numOfRequestedVectors"`
	AusfInstanceID         string                 `json:"ausfInstanceId"`
	ResynchronizationInfo  *ResynchronizationInfo `json:"resynchronizationInfo,omitempty"`
	SupportedFeatures      string                 `json:"supportedFeatures,omitempty"`
}

// HssAuthenticationInfoResult is the response for GenerateAv (HSS interworking).
//
// 3GPP: TS 29.503 — HssAuthenticationInfoResult data type
type HssAuthenticationInfoResult struct {
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// MilenageOutput holds the output parameters from the Milenage algorithm.
//
// 3GPP: TS 35.206 — Milenage output parameters
type MilenageOutput struct {
	MAC  []byte // 8 bytes — f1 output (MAC-A)
	XRES []byte // 8 bytes — f2 output
	CK   []byte // 16 bytes — f3 output
	IK   []byte // 16 bytes — f4 output
	AK   []byte // 6 bytes — f5 output
	AUTN []byte // 16 bytes — (SQN ⊕ AK) || AMF || MAC-A
}
