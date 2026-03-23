// Package rsds implements the Nudm_RSDS service for Report SMS Delivery
// Status in the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
// 3GPP: TS 29.503 Nudm_RSDS — Report SMS Delivery Status service operations
package rsds

import "encoding/json"

// SmDeliveryStatus represents the request body for the ReportSMDeliveryStatus
// operation.
//
// 3GPP: TS 29.503 — SmDeliveryStatus data type
type SmDeliveryStatus struct {
	Gpsi            string          `json:"gpsi"`
	SmStatusReport  json.RawMessage `json:"smStatusReport"`
}
