// Package mt implements the Nudm_MT service for Mobile Terminated
// operations in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
// 3GPP: TS 29.503 Nudm_MT — Mobile Terminated service operations
package mt

// UeInfo represents the UE reachability and location information returned
// by the QueryUeInfo operation.
//
// 3GPP: TS 29.503 — UeInfo data type
type UeInfo struct {
	UserState         string `json:"userState,omitempty"`
	ServingAmfId      string `json:"servingAmfId,omitempty"`
	RatType           string `json:"ratType,omitempty"`
	AccessType        string `json:"accessType,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// LocationInfoRequest is the request body for ProvideLocationInfo.
//
// 3GPP: TS 29.503 — LocationInfoRequest data type
type LocationInfoRequest struct {
	Supi              string `json:"supi,omitempty"`
	Req5gsInd         bool   `json:"req5gsInd,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}

// LocationInfoResult is the response body for ProvideLocationInfo.
//
// 3GPP: TS 29.503 — LocationInfoResult data type
type LocationInfoResult struct {
	Supi              string `json:"supi,omitempty"`
	ServingAmfId      string `json:"servingAmfId,omitempty"`
	UserState         string `json:"userState,omitempty"`
	RatType           string `json:"ratType,omitempty"`
	SupportedFeatures string `json:"supportedFeatures,omitempty"`
}
