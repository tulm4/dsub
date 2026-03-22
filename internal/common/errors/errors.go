// Package errors provides RFC 7807 ProblemDetails with 3GPP extensions
// for the 5G UDM network function.
//
// Based on: docs/sbi-api-design.md §7 (Error Handling)
// 3GPP: TS 29.500 §5.2.7 — Error Signaling
// 3GPP: TS 29.503 — Nudm cause codes
package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ProblemDetails represents an RFC 7807 problem details object with 3GPP
// extensions defined in TS 29.500 §5.2.7.2.
//
// 3GPP: TS 29.500 §5.2.7.2 — Protocol Error Handling
// 3GPP: TS 29.503 — Nudm-specific cause codes
type ProblemDetails struct {
	Type              string         `json:"type,omitempty"`
	Title             string         `json:"title,omitempty"`
	Status            int            `json:"status,omitempty"`
	Detail            string         `json:"detail,omitempty"`
	Instance          string         `json:"instance,omitempty"`
	Cause             string         `json:"cause,omitempty"`
	InvalidParams     []InvalidParam `json:"invalidParams,omitempty"`
	AccessTokenError  string         `json:"accessTokenError,omitempty"`
	SupportedFeatures string         `json:"supportedFeatures,omitempty"`
}

// InvalidParam describes a single invalid parameter in a request, per
// RFC 7807 §3.
type InvalidParam struct {
	Param  string `json:"param"`
	Reason string `json:"reason"`
}

// Error implements the error interface so ProblemDetails can be returned and
// handled as a standard Go error.
func (p *ProblemDetails) Error() string {
	if p.Cause != "" {
		return fmt.Sprintf("HTTP %d %s: %s (cause: %s)", p.Status, p.Title, p.Detail, p.Cause)
	}
	return fmt.Sprintf("HTTP %d %s: %s", p.Status, p.Title, p.Detail)
}

// 3GPP cause codes used across Nudm services.
// 3GPP: TS 29.503 §6.1.7 — Application Error Codes
const (
	CauseAuthenticationRejected      = "AUTHENTICATION_REJECTED"
	CauseServingNetworkNotAuthorized = "SERVING_NETWORK_NOT_AUTHORIZED"
	CauseUserNotFound                = "USER_NOT_FOUND"
	CauseDataNotFound                = "DATA_NOT_FOUND"
	CauseContextNotFound             = "CONTEXT_NOT_FOUND"
	CauseSubscriptionNotFound        = "SUBSCRIPTION_NOT_FOUND"
	CauseModificationNotAllowed      = "MODIFICATION_NOT_ALLOWED"
	CauseMandatoryIEIncorrect        = "MANDATORY_IE_INCORRECT"
	CauseMandatoryIEMissing          = "MANDATORY_IE_MISSING"
	CauseUnspecifiedNFFailure        = "UNSPECIFIED_NF_FAILURE"
	CauseNFCongestion                = "NF_CONGESTION"
	CauseInsufficientResources       = "INSUFFICIENT_RESOURCES"
)

// Constructor helpers — one per HTTP status code.
// Based on: docs/sbi-api-design.md §7 (Error Response Codes)

// NewBadRequest creates a 400 Bad Request ProblemDetails.
func NewBadRequest(detail, cause string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusBadRequest,
		Title:  "Bad Request",
		Detail: detail,
		Cause:  cause,
	}
}

// NewUnauthorized creates a 401 Unauthorized ProblemDetails.
func NewUnauthorized(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusUnauthorized,
		Title:  "Unauthorized",
		Detail: detail,
	}
}

// NewForbidden creates a 403 Forbidden ProblemDetails.
func NewForbidden(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusForbidden,
		Title:  "Forbidden",
		Detail: detail,
	}
}

// NewNotFound creates a 404 Not Found ProblemDetails.
func NewNotFound(detail, cause string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusNotFound,
		Title:  "Not Found",
		Detail: detail,
		Cause:  cause,
	}
}

// NewConflict creates a 409 Conflict ProblemDetails.
func NewConflict(detail, cause string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusConflict,
		Title:  "Conflict",
		Detail: detail,
		Cause:  cause,
	}
}

// NewTooManyRequests creates a 429 Too Many Requests ProblemDetails.
func NewTooManyRequests(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusTooManyRequests,
		Title:  "Too Many Requests",
		Detail: detail,
	}
}

// NewInternalError creates a 500 Internal Server Error ProblemDetails.
func NewInternalError(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusInternalServerError,
		Title:  "Internal Server Error",
		Detail: detail,
	}
}

// NewNotImplemented creates a 501 Not Implemented ProblemDetails.
func NewNotImplemented(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusNotImplemented,
		Title:  "Not Implemented",
		Detail: detail,
	}
}

// NewBadGateway creates a 502 Bad Gateway ProblemDetails.
func NewBadGateway(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusBadGateway,
		Title:  "Bad Gateway",
		Detail: detail,
	}
}

// NewServiceUnavailable creates a 503 Service Unavailable ProblemDetails.
func NewServiceUnavailable(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusServiceUnavailable,
		Title:  "Service Unavailable",
		Detail: detail,
	}
}

// NewGatewayTimeout creates a 504 Gateway Timeout ProblemDetails.
func NewGatewayTimeout(detail string) *ProblemDetails {
	return &ProblemDetails{
		Status: http.StatusGatewayTimeout,
		Title:  "Gateway Timeout",
		Detail: detail,
	}
}

// WriteProblemDetails writes a ProblemDetails as JSON to an http.ResponseWriter
// with the correct Content-Type header per RFC 7807 §3.
//
// Based on: docs/sbi-api-design.md §7 (Error Response Format)
// 3GPP: TS 29.500 §5.2.7.2 — application/problem+json media type
func WriteProblemDetails(w http.ResponseWriter, pd *ProblemDetails) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(pd.Status)
	_ = json.NewEncoder(w).Encode(pd)
}
