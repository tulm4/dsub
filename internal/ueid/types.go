// Package ueid implements the Nudm_UEID service for SUCI de-concealment
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.10 (udm-ueid)
// 3GPP: TS 29.503 Nudm_UEID — UE Identification service operations
// 3GPP: TS 33.501 §6.12 — SUCI de-concealment (ECIES Profile A/B)
package ueid

import "fmt"

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

// SUCIProfile holds home network key metadata for SUCI de-concealment.
// Per docs/security.md §4.3/§4.4, private key material never leaves the HSM
// boundary. Only the HSM key reference is stored in the database.
//
// Based on: docs/security.md §4.3 (Home Network Key Management)
// 3GPP: TS 33.501 §6.12 — ECIES Profile A (X25519) and Profile B (secp256r1)
type SUCIProfile struct {
	HNKeyID     int    // Home Network Public Key Identifier (0-255)
	ProfileType string // "A" (X25519) or "B" (secp256r1)
	PublicKey   []byte // Raw public key bytes (for profile type verification)
	HSMKeyRef   string // HSM key reference/handle (private key never in app memory)
	IsActive    bool   // Whether this key is currently active
}

// HSMDecrypter abstracts the HSM-based ECIES de-concealment operation.
// In production, the implementation calls an HSM to perform ECDH key agreement
// and ECIES decryption without exposing the private key to application memory.
// For development/testing, a software-based implementation may be used.
//
// Based on: docs/security.md §4.3/§4.4 (private key must not leave HSM boundary)
// 3GPP: TS 33.501 Annex C — ECIES Profile A/B
type HSMDecrypter interface {
	// DeconcealMSIN performs ECIES de-concealment using the HSM-managed private key.
	// The hsmKeyRef identifies the key within the HSM. The profileType is "A" or "B".
	// ephemeralPubKey is the UE's ephemeral public key and cipherText contains the
	// encrypted MSIN concatenated with the 8-byte MAC tag.
	DeconcealMSIN(hsmKeyRef, profileType string, ephemeralPubKey, cipherText []byte) ([]byte, error)
}

// SoftwareHSM is a development/test implementation of HSMDecrypter that performs
// ECIES de-concealment in software. NOT suitable for production use — private keys
// are loaded into application memory.
type SoftwareHSM struct {
	// Keys maps HSM key references to raw private key bytes.
	// In tests, populate this with the test key material.
	Keys map[string][]byte
}

// DeconcealMSIN implements HSMDecrypter using software-based ECIES.
func (h *SoftwareHSM) DeconcealMSIN(hsmKeyRef, profileType string, ephemeralPubKey, cipherText []byte) ([]byte, error) {
	privKey, ok := h.Keys[hsmKeyRef]
	if !ok {
		return nil, fmt.Errorf("software-hsm: key not found for ref %q", hsmKeyRef)
	}
	return DeconcealMSIN(profileType, privKey, ephemeralPubKey, cipherText)
}
