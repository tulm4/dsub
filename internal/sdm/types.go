// Package sdm implements the Nudm_SDM service for Subscriber Data Management
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
// 3GPP: TS 29.503 Nudm_SDM — Subscriber Data Management service operations
// 3GPP: TS 29.505 — Usage of the Unified Data Repository services (data model)
package sdm

import "encoding/json"

// AccessAndMobilitySubscriptionData contains subscriber access and mobility data.
//
// 3GPP: TS 29.505 — AccessAndMobilitySubscriptionData data type
type AccessAndMobilitySubscriptionData struct {
	Gpsis                  []string        `json:"gpsis,omitempty"`
	InternalGroupIds       []string        `json:"internalGroupIds,omitempty"`
	SubscribedUeAmbr       json.RawMessage `json:"subscribedUeAmbr,omitempty"`
	Nssai                  json.RawMessage `json:"nssai,omitempty"`
	RatRestrictions        json.RawMessage `json:"ratRestrictions,omitempty"`
	ForbiddenAreas         json.RawMessage `json:"forbiddenAreas,omitempty"`
	ServiceAreaRestriction json.RawMessage `json:"serviceAreaRestriction,omitempty"`
	RfspIndex              *int            `json:"rfspIndex,omitempty"`
	SubsRegTimer           *int            `json:"subsRegTimer,omitempty"`
	ActiveTime             *int            `json:"activeTime,omitempty"`
	SorInfo                json.RawMessage `json:"sorInfo,omitempty"`
	UpuInfo                json.RawMessage `json:"upuInfo,omitempty"`
	MicoAllowed            bool            `json:"micoAllowed,omitempty"`
	SharedAmDataIds        []string        `json:"sharedAmDataIds,omitempty"`
	OdbPacketServices      string          `json:"odbPacketServices,omitempty"`
	SubscribedDnnList      []string        `json:"subscribedDnnList,omitempty"`
	ServiceGapTime         *int            `json:"serviceGapTime,omitempty"`
	TraceData              json.RawMessage `json:"traceData,omitempty"`
	CagData                json.RawMessage `json:"cagData,omitempty"`
	RoutingIndicator       string          `json:"routingIndicator,omitempty"`
	MpsPriority            bool            `json:"mpsPriority,omitempty"`
	McsPriority            bool            `json:"mcsPriority,omitempty"`
}

// SessionManagementSubscriptionData contains subscriber session management data.
//
// 3GPP: TS 29.505 — SessionManagementSubscriptionData data type
type SessionManagementSubscriptionData struct {
	SingleNssai       json.RawMessage `json:"singleNssai"`
	DnnConfigurations json.RawMessage `json:"dnnConfigurations,omitempty"`
	InternalGroupIds  []string        `json:"internalGroupIds,omitempty"`
	SharedDataIds     []string        `json:"sharedVnGroupDataIds,omitempty"`
}

// SmfSelectionSubscriptionData contains SMF selection subscription data.
//
// 3GPP: TS 29.505 — SmfSelectionSubscriptionData data type
type SmfSelectionSubscriptionData struct {
	SubscribedSnssaiInfos json.RawMessage `json:"subscribedSnssaiInfos,omitempty"`
	SharedSnssaiInfosIds  []string        `json:"sharedSnssaiInfosIds,omitempty"`
}

// Nssai contains the subscriber's subscribed NSSAI information.
//
// 3GPP: TS 29.505 — Nssai data type
type Nssai struct {
	DefaultSingleNssais json.RawMessage `json:"defaultSingleNssais,omitempty"`
	SingleNssais        json.RawMessage `json:"singleNssais,omitempty"`
}

// SmsSubscriptionData contains SMS subscription data.
//
// 3GPP: TS 29.505 — SmsSubscriptionData data type
type SmsSubscriptionData struct {
	SmsSubscribed bool `json:"smsSubscribed,omitempty"`
}

// SmsManagementSubscriptionData contains SMS management subscription data.
//
// 3GPP: TS 29.505 — SmsManagementSubscriptionData data type
type SmsManagementSubscriptionData struct {
	SupportedFeatures   string   `json:"supportedFeatures,omitempty"`
	MtSmsSubscribed     bool     `json:"mtSmsSubscribed,omitempty"`
	MtSmsBarringAll     bool     `json:"mtSmsBarringAll,omitempty"`
	MtSmsBarringRoaming bool     `json:"mtSmsBarringRoaming,omitempty"`
	MoSmsSubscribed     bool     `json:"moSmsSubscribed,omitempty"`
	MoSmsBarringAll     bool     `json:"moSmsBarringAll,omitempty"`
	MoSmsBarringRoaming bool     `json:"moSmsBarringRoaming,omitempty"`
	SharedSmsMngDataIds []string `json:"sharedSmsMngDataIds,omitempty"`
}

// UeContextInAmfData contains UE context information in the AMF.
//
// 3GPP: TS 29.505 — UeContextInAmfData data type
type UeContextInAmfData struct {
	AccessDetails json.RawMessage `json:"accessDetails,omitempty"`
}

// UeContextInSmfData contains UE context information in the SMF.
//
// 3GPP: TS 29.505 — UeContextInSmfData data type
type UeContextInSmfData struct {
	PduSessions json.RawMessage `json:"pduSessions,omitempty"`
}

// UeContextInSmsfData contains UE context information in the SMSF.
//
// 3GPP: TS 29.505 — UeContextInSmsfData data type
type UeContextInSmsfData struct {
	SmsfInfo3GppAccess    json.RawMessage `json:"smsfInfo3GppAccess,omitempty"`
	SmsfInfoNon3GppAccess json.RawMessage `json:"smsfInfoNon3GppAccess,omitempty"`
}

// TraceData contains subscriber trace configuration data.
//
// 3GPP: TS 29.505 — TraceData data type
type TraceData struct {
	TraceRef                 string `json:"traceRef,omitempty"`
	TraceDepth               string `json:"traceDepth,omitempty"`
	NeTypeList               string `json:"neTypeList,omitempty"`
	EventList                string `json:"eventList,omitempty"`
	CollectionEntityIpv4Addr string `json:"collectionEntityIpv4Addr,omitempty"`
	InterfaceList            string `json:"interfaceList,omitempty"`
	SharedTraceDataId        string `json:"sharedTraceDataId,omitempty"`
}

// SubscriptionDataSets is the aggregated response for GetDataSets containing
// multiple subscription data sets in a single response.
//
// 3GPP: TS 29.505 — SubscriptionDataSets data type
type SubscriptionDataSets struct {
	AmData      *AccessAndMobilitySubscriptionData  `json:"amData,omitempty"`
	SmfSelData  *SmfSelectionSubscriptionData       `json:"smfSelData,omitempty"`
	SmsSubsData *SmsSubscriptionData                `json:"smsSubsData,omitempty"`
	SmData      []SessionManagementSubscriptionData `json:"smData,omitempty"`
	TraceData   *TraceData                          `json:"traceData,omitempty"`
}

// IdTranslationResult carries the result of an identity translation (SUPI/GPSI lookup).
//
// 3GPP: TS 29.503 — IdTranslationResult data type
type IdTranslationResult struct {
	Supi              string `json:"supi,omitempty"`
	Gpsi              string `json:"gpsi,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// SdmSubscription represents a subscription to subscriber data change notifications.
//
// 3GPP: TS 29.503 Nudm_SDM — SdmSubscription data type
type SdmSubscription struct {
	NfInstanceID          string   `json:"nfInstanceId"`
	CallbackReference     string   `json:"callbackReference"`
	MonitoredResourceUris []string `json:"monitoredResourceUris"`
	SubscriptionID        string   `json:"subscriptionId,omitempty"`
	ExpiryTime            string   `json:"expires,omitempty"`
	ImplicitUnsubscribe   bool     `json:"implicitUnsubscribe,omitempty"`
	SupportedFeatures     string   `json:"supportedFeatures,omitempty"`
}

// SharedData represents a shared data entry.
//
// 3GPP: TS 29.505 — SharedData data type
type SharedData struct {
	SharedDataID string          `json:"sharedDataId"`
	SharedAmData json.RawMessage `json:"sharedAmData,omitempty"`
	SharedSmData json.RawMessage `json:"sharedSmData,omitempty"`
}

// GroupIdentifiers carries group identifier information.
//
// 3GPP: TS 29.503 Nudm_SDM — GroupIdentifiers data type
type GroupIdentifiers struct {
	ExtGroupID string `json:"extGroupId,omitempty"`
	IntGroupID string `json:"intGroupId,omitempty"`
}

// SorAckInfo carries the SoR acknowledgement information.
//
// 3GPP: TS 29.503 Nudm_SDM — SorAckInfo
type SorAckInfo struct {
	SorMacIue string `json:"sorMacIue"`
}

// AcknowledgeInfo carries generic acknowledgement data for UPU, CAG, and SNSSAI.
//
// 3GPP: TS 29.503 Nudm_SDM — AcknowledgeInfo data type
type AcknowledgeInfo struct {
	SorMacIue string `json:"sorMacIue,omitempty"`
	UpuMacIue string `json:"upuMacIue,omitempty"`
}

// SorUpdateInfo carries the SoR update request data.
//
// 3GPP: TS 29.503 Nudm_SDM — SorUpdateInfo
type SorUpdateInfo struct {
	VplmnID json.RawMessage `json:"vplmnId"`
}
