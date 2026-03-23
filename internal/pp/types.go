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

// VnGroupConfiguration represents a 5G VN group configuration.
//
// 3GPP: TS 29.503 — 5GVnGroupConfiguration data type
type VnGroupConfiguration struct {
	Dnn                     string          `json:"dnn,omitempty"`
	SNssai                  json.RawMessage `json:"sNssai,omitempty"`
	PduSessionTypes         []string        `json:"pduSessionTypes,omitempty"`
	AppDescriptors          json.RawMessage `json:"appDescriptors,omitempty"`
	SecondaryAuth           *bool           `json:"secondaryAuth,omitempty"`
	DnAaaAddress            json.RawMessage `json:"dnAaaAddress,omitempty"`
	DnAaaFqdn               string          `json:"dnAaaFqdn,omitempty"`
	Members                 []string        `json:"members,omitempty"`
	ReferenceId             string          `json:"referenceId,omitempty"`
	AfInstanceId            string          `json:"afInstanceId,omitempty"`
	InternalGroupIdentifier string          `json:"internalGroupIdentifier,omitempty"`
	MtcProviderInformation  json.RawMessage `json:"mtcProviderInformation,omitempty"`
}

// MbsGroupMemb represents a multicast MBS group membership.
//
// 3GPP: TS 29.503 — MulticastMbsGroupMemb data type
type MbsGroupMemb struct {
	MulticastGroupMemb      json.RawMessage `json:"multicastGroupMemb,omitempty"`
	AfInstanceId            string          `json:"afInstanceId,omitempty"`
	InternalGroupIdentifier string          `json:"internalGroupIdentifier,omitempty"`
}

// SdmSubscriptionInfo represents an SDM subscription that should be notified
// of data changes. It is returned by GetSdmSubscriptionsForNotify so that the
// PP service can identify which SDM subscribers to notify after provisioned
// parameter updates.
//
// Based on: docs/sequence-diagrams.md §8 (Subscription Data Update Notification)
// 3GPP: TS 29.503 Nudm_SDM — SdmSubscription (callback reference and URIs)
type SdmSubscriptionInfo struct {
	SubscriptionID        string   `json:"subscriptionId"`
	CallbackReference     string   `json:"callbackReference"`
	MonitoredResourceURIs []string `json:"monitoredResourceUris"`
}
