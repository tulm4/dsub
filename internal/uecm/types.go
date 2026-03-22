// Package uecm implements the Nudm_UECM service for UE Context Management
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
// 3GPP: TS 29.503 Nudm_UECM — UE Context Management service operations
// 3GPP: TS 23.502 §4.2.2.2 — Registration procedure
package uecm

import "encoding/json"

// Amf3GppAccessRegistration represents an AMF registration for 3GPP access.
//
// 3GPP: TS 29.503 — Amf3GppAccessRegistration data type
type Amf3GppAccessRegistration struct {
	AmfInstanceID               string          `json:"amfInstanceId"`
	DeregCallbackURI            string          `json:"deregCallbackUri"`
	AmfServiceNameDereg         string          `json:"amfServiceNameDereg,omitempty"`
	Guami                       json.RawMessage `json:"guami"`
	RatType                     string          `json:"ratType"`
	InitialRegistrationInd      bool            `json:"initialRegistrationInd,omitempty"`
	EmergencyRegistrationInd    bool            `json:"emergencyRegistrationInd,omitempty"`
	UeReachable                 bool            `json:"ueReachable,omitempty"`
	ImsVoPs                     string          `json:"imsVoPs,omitempty"`
	PlmnID                      json.RawMessage `json:"plmnId,omitempty"`
	BackupAmfInfo               json.RawMessage `json:"backupAmfInfo,omitempty"`
	SupportedFeatures           string          `json:"supportedFeatures,omitempty"`
	PurgeFlag                   bool            `json:"purgeFlag,omitempty"`
	Pei                         string          `json:"pei,omitempty"`
	PcscfRestorationCallbackURI string          `json:"pcscfRestorationCallbackUri,omitempty"`
	NoEeSubscriptionInd         bool            `json:"noEeSubscriptionInd,omitempty"`
	RegistrationTime            string          `json:"registrationTime,omitempty"`
	ContextInfo                 json.RawMessage `json:"contextInfo,omitempty"`
}

// AmfNon3GppAccessRegistration represents an AMF registration for non-3GPP access.
//
// 3GPP: TS 29.503 — AmfNon3GppAccessRegistration data type
type AmfNon3GppAccessRegistration struct {
	AmfInstanceID               string          `json:"amfInstanceId"`
	DeregCallbackURI            string          `json:"deregCallbackUri"`
	AmfServiceNameDereg         string          `json:"amfServiceNameDereg,omitempty"`
	Guami                       json.RawMessage `json:"guami"`
	RatType                     string          `json:"ratType"`
	InitialRegistrationInd      bool            `json:"initialRegistrationInd,omitempty"`
	EmergencyRegistrationInd    bool            `json:"emergencyRegistrationInd,omitempty"`
	UeReachable                 bool            `json:"ueReachable,omitempty"`
	ImsVoPs                     string          `json:"imsVoPs,omitempty"`
	PlmnID                      json.RawMessage `json:"plmnId,omitempty"`
	BackupAmfInfo               json.RawMessage `json:"backupAmfInfo,omitempty"`
	SupportedFeatures           string          `json:"supportedFeatures,omitempty"`
	PurgeFlag                   bool            `json:"purgeFlag,omitempty"`
	Pei                         string          `json:"pei,omitempty"`
	PcscfRestorationCallbackURI string          `json:"pcscfRestorationCallbackUri,omitempty"`
	NoEeSubscriptionInd         bool            `json:"noEeSubscriptionInd,omitempty"`
	RegistrationTime            string          `json:"registrationTime,omitempty"`
	ContextInfo                 json.RawMessage `json:"contextInfo,omitempty"`
}

// SmfRegistration represents a SMF registration for a PDU session.
//
// 3GPP: TS 29.503 — SmfRegistration data type
type SmfRegistration struct {
	SmfInstanceID               string          `json:"smfInstanceId"`
	SmfSetID                    string          `json:"smfSetId,omitempty"`
	Dnn                         string          `json:"dnn"`
	SingleNssai                 json.RawMessage `json:"singleNssai"`
	PlmnID                      json.RawMessage `json:"plmnId"`
	PduSessionID                int             `json:"pduSessionId"`
	EmergencyServices           bool            `json:"emergencyServices,omitempty"`
	PduSessionType              string          `json:"pduSessionType,omitempty"`
	RegistrationTime            string          `json:"registrationTime,omitempty"`
	RegistrationReason          string          `json:"registrationReason,omitempty"`
	ContextInfo                 json.RawMessage `json:"contextInfo,omitempty"`
	PcscfRestorationCallbackURI string          `json:"pcscfRestorationCallbackUri,omitempty"`
	SupportedFeatures           string          `json:"supportedFeatures,omitempty"`
}

// SmsfRegistration represents an SMSF registration.
//
// 3GPP: TS 29.503 — SmsfRegistration data type
type SmsfRegistration struct {
	SmsfInstanceID    string          `json:"smsfInstanceId"`
	SmsfSetID         string          `json:"smsfSetId,omitempty"`
	PlmnID            json.RawMessage `json:"plmnId"`
	UeReachable       bool            `json:"ueReachable,omitempty"`
	SupportedFeatures string          `json:"supportedFeatures,omitempty"`
	RegistrationTime  string          `json:"registrationTime,omitempty"`
	ContextInfo       json.RawMessage `json:"contextInfo,omitempty"`
}

// IpSmGwRegistration represents an IP-SM-GW registration.
//
// 3GPP: TS 29.503 — IpSmGwRegistration data type
type IpSmGwRegistration struct {
	IpSmGwMapAddress  string `json:"ipSmGwMapAddress,omitempty"`
	UnriIndicator     bool   `json:"unriIndicator,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// NwdafRegistration represents an NWDAF registration.
//
// 3GPP: TS 29.503 — NwdafRegistration data type
type NwdafRegistration struct {
	NwdafInstanceID   string          `json:"nwdafInstanceId"`
	AnalyticsIDs      json.RawMessage `json:"analyticsIds,omitempty"`
	NwdafSetID        string          `json:"nwdafSetId,omitempty"`
	RegistrationTime  string          `json:"registrationTime,omitempty"`
	ContextInfo       json.RawMessage `json:"contextInfo,omitempty"`
	SupportedFeatures string          `json:"supportedFeatures,omitempty"`
}

// RegistrationDataSets is the aggregated response for GetRegistrations containing
// all current NF registrations for a subscriber.
//
// 3GPP: TS 29.503 — RegistrationDataSets data type
type RegistrationDataSets struct {
	Amf3GppAccess      *Amf3GppAccessRegistration    `json:"amf3GppAccess,omitempty"`
	AmfNon3GppAccess   *AmfNon3GppAccessRegistration `json:"amfNon3GppAccess,omitempty"`
	SmfRegistrations   []SmfRegistration             `json:"smfRegistrations,omitempty"`
	Smsf3GppAccess     *SmsfRegistration             `json:"smsf3GppAccess,omitempty"`
	SmsfNon3GppAccess  *SmsfRegistration             `json:"smsfNon3GppAccess,omitempty"`
	IpSmGw             *IpSmGwRegistration           `json:"ipSmGw,omitempty"`
	NwdafRegistrations []NwdafRegistration           `json:"nwdafRegistrations,omitempty"`
}

// DeregistrationData carries deregistration request information.
//
// 3GPP: TS 29.503 — DeregistrationData data type
type DeregistrationData struct {
	DeregReason string `json:"deregReason"`
	AccessType  string `json:"accessType,omitempty"`
}

// PeiUpdateInfo carries PEI update request information.
//
// 3GPP: TS 29.503 — PeiUpdateInfo data type
type PeiUpdateInfo struct {
	Pei string `json:"pei"`
}

// RoamingInfoUpdate carries roaming information update request data.
//
// 3GPP: TS 29.503 — RoamingInfoUpdate data type
type RoamingInfoUpdate struct {
	Roaming bool `json:"roaming"`
}

// RoutingInfoSmRequest is the request body for SendRoutingInfoSm.
//
// 3GPP: TS 29.503 — RoutingInfoSmRequest data type
type RoutingInfoSmRequest struct {
	IP4Domain         string `json:"ip4Domain,omitempty"`
	IP6Prefix         string `json:"ip6Prefix,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// RoutingInfoSmResponse is the response body for SendRoutingInfoSm.
//
// 3GPP: TS 29.503 — RoutingInfoSmResponse data type
type RoutingInfoSmResponse struct {
	SmsfInstanceID    string `json:"smsfInstanceId,omitempty"`
	IP4Domain         string `json:"ip4Domain,omitempty"`
	IP6Prefix         string `json:"ip6Prefix,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// TriggerRequest carries authentication trigger request data.
//
// 3GPP: TS 29.503 — TriggerRequest data type
type TriggerRequest struct {
	Supi string `json:"supi,omitempty"`
}

// PcscfRestorationRequestData is the request body for RestorePcscf.
//
// 3GPP: TS 29.503 — PcscfRestorationRequestData data type
type PcscfRestorationRequestData struct {
	Dnn       string          `json:"dnn,omitempty"`
	SliceInfo json.RawMessage `json:"sliceInfo,omitempty"`
	Supi      string          `json:"supi,omitempty"`
	Ip4       string          `json:"ip4,omitempty"`
	Ip6       string          `json:"ip6,omitempty"`
}
