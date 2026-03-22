// Package pp implements the Nudm_PP service for Parameter Provisioning
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
// 3GPP: TS 29.503 Nudm_PP — Parameter Provisioning service operations
package pp

import "encoding/json"

// PpData represents provisioned parameter data for a subscriber.
//
// 3GPP: TS 29.503 — PpData data type
// 3GPP: TS 29.505 — Provisioned Parameter data definition
type PpData struct {
	CommunicationCharacteristics json.RawMessage `json:"communicationCharacteristics,omitempty"`
	SupportedFeatures            string          `json:"supportedFeatures,omitempty"`
	ExpectedUeBehaviour          json.RawMessage `json:"expectedUeBehaviour,omitempty"`
	EcRestriction                json.RawMessage `json:"ecRestriction,omitempty"`
	AcsInfo                      json.RawMessage `json:"acsInfo,omitempty"`
	SorInfo                      json.RawMessage `json:"sorInfo,omitempty"`
	FiveMbsAuthorizationInfo     json.RawMessage `json:"5mbsAuthorizationInfo,omitempty"`
	SteeringContainer            json.RawMessage `json:"steeringContainer,omitempty"`
	PpDlPacketCount              *int            `json:"ppDlPacketCount,omitempty"`
	PpDlPacketCountExt           json.RawMessage `json:"ppDlPacketCountExt,omitempty"`
	PpMaximumResponseTime        *int            `json:"ppMaximumResponseTime,omitempty"`
	PpMaximumLatency             *int            `json:"ppMaximumLatency,omitempty"`
}
