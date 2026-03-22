// Package ueid implements the Nudm_UEID service for SUCI de-concealment
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.10 (udm-ueid)
// 3GPP: TS 29.503 Nudm_UEID — UE Identification service operations
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment (ECIES Profile A/B)
package ueid

// SuciDeconcealRequest is the POST body for the SUCI de-concealment operation.
//
// 3GPP: TS 29.503 — SuciInfo data type
type SuciDeconcealRequest struct {
	Suci              string `json:"suci"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// SuciDeconcealResponse is the response for the SUCI de-concealment operation.
//
// 3GPP: TS 29.503 — SupiInfo data type (de-concealment result)
type SuciDeconcealResponse struct {
	Supi              string `json:"supi"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// SUCIProfile holds a home network key pair for SUCI de-concealment.
//
// Based on: docs/security.md §4.3 (Home Network Key Management)
// 3GPP: TS 33.501 §6.12 — ECIES Profile A (X25519) and Profile B (secp256r1)
type SUCIProfile struct {
	HNKeyID     int    // Home Network Public Key Identifier (0-255)
	ProfileType string // "A" (X25519) or "B" (secp256r1)
	PublicKey   []byte // Raw public key bytes
	PrivateKey  []byte // Raw private key bytes (HSM reference in production)
	IsActive    bool   // Whether this key is currently active
}
