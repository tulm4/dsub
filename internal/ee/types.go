// Package ee implements the Nudm_EE service for Event Exposure
// in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// 3GPP: TS 29.503 Nudm_EE — Event Exposure service operations
// 3GPP: TS 23.502 §4.15.3 — Event Exposure procedure
package ee

import "encoding/json"

// EeSubscription represents an event exposure subscription request.
//
// 3GPP: TS 29.503 — EeSubscription data type
type EeSubscription struct {
	CallbackReference          string          `json:"callbackReference"`
	MonitoringConfigurations   json.RawMessage `json:"monitoringConfigurations"`
	ReportingOptions           json.RawMessage `json:"reportingOptions,omitempty"`
	SupportedFeatures          string          `json:"supportedFeatures,omitempty"`
	ScefID                     string          `json:"scefId,omitempty"`
	NfInstanceID               string          `json:"nfInstanceId,omitempty"`
	DataRestorationCallbackURI string          `json:"dataRestorationCallbackUri,omitempty"`
	ExcludedUnsubscribedUes    bool            `json:"excludedUnsubscribedUes,omitempty"`
	ExpiryTime                 string          `json:"expiryTime,omitempty"`
	ImmediateReportData        json.RawMessage `json:"immediateReportData,omitempty"`
}

// CreatedEeSubscription represents the response to a successful subscription creation.
//
// 3GPP: TS 29.503 — CreatedEeSubscription data type
type CreatedEeSubscription struct {
	EeSubscription  *EeSubscription `json:"eeSubscription"`
	SubscriptionID  string          `json:"subscriptionId"`
	MonitoringReport json.RawMessage `json:"monitoringReport,omitempty"`
}

// PatchEeSubscription represents a partial update to an existing subscription.
// All fields are optional (pointers) for JSON merge-patch semantics.
//
// 3GPP: TS 29.503 — PatchEeSubscription data type
type PatchEeSubscription struct {
	CallbackReference          *string          `json:"callbackReference,omitempty"`
	MonitoringConfigurations   *json.RawMessage `json:"monitoringConfigurations,omitempty"`
	ReportingOptions           *json.RawMessage `json:"reportingOptions,omitempty"`
	SupportedFeatures          *string          `json:"supportedFeatures,omitempty"`
	ScefID                     *string          `json:"scefId,omitempty"`
	NfInstanceID               *string          `json:"nfInstanceId,omitempty"`
	DataRestorationCallbackURI *string          `json:"dataRestorationCallbackUri,omitempty"`
	ExcludedUnsubscribedUes    *bool            `json:"excludedUnsubscribedUes,omitempty"`
	ExpiryTime                 *string          `json:"expiryTime,omitempty"`
	ImmediateReportData        *json.RawMessage `json:"immediateReportData,omitempty"`
}

// EeEventReport represents a report to be sent to a matching EE subscriber.
// It is returned by GetMatchingSubscriptions so that other services (e.g. UECM)
// can dispatch event notifications to the appropriate callback URIs.
//
// 3GPP: TS 29.503 Nudm_EE — MonitoringReport
type EeEventReport struct {
	SubscriptionID    string          `json:"subscriptionId"`
	CallbackReference string          `json:"callbackReference"`
	MonitoringReport  json.RawMessage `json:"monitoringReport,omitempty"`
}
