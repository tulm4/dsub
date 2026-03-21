// Package sbi provides HTTP/2 helpers, 3GPP custom header constants, and
// JSON codec utilities for the 5G UDM Service-Based Interface (SBI).
//
// Based on: docs/sbi-api-design.md §1 (SBI Overview), §5 (3GPP Custom HTTP Headers), §9 (Content Negotiation)
// 3GPP: TS 29.500 §5.2.3 — HTTP/2 as transport for SBI
// 3GPP: TS 29.500 §6.10 — Custom HTTP headers
package sbi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// 3GPP custom headers per TS 29.500 §6.10.
//
// Based on: docs/sbi-api-design.md §5 (3GPP Custom HTTP Headers)
const (
	HeaderTargetAPIRoot   = "3gpp-Sbi-Target-apiRoot"
	HeaderCallback        = "3gpp-Sbi-Callback"
	HeaderCorrelationInfo = "3gpp-Sbi-Correlation-Info"
	HeaderOCI             = "3gpp-Sbi-Oci"
	HeaderLCI             = "3gpp-Sbi-Lci"
	HeaderMessagePriority = "3gpp-Sbi-Message-Priority"
	HeaderMaxRspTime      = "3gpp-Sbi-Max-Rsp-Time"
	HeaderRoutingBinding  = "3gpp-Sbi-Routing-Binding"
)

// Standard content-type values used across all Nudm service APIs.
//
// Based on: docs/sbi-api-design.md §9 (Content Negotiation)
const (
	ContentTypeJSON        = "application/json"
	ContentTypeProblemJSON = "application/problem+json"
)

// Client timeout and request body size constants.
//
// Based on: docs/sbi-api-design.md §1 (SBI Overview)
// Based on: docs/performance.md (latency targets)
const (
	DefaultConnectTimeout = 1 * time.Second
	DefaultReadTimeout    = 3 * time.Second
	DefaultRequestTimeout = 5 * time.Second
	MaxRequestBodySize    = 1 << 20 // 1 MB
)

// WriteJSON writes a JSON response with the given HTTP status code.
// It sets Content-Type to application/json before writing the status code so
// that the header is included in the response.
//
// Callers must ensure v is JSON-serializable. If encoding fails after the
// status code has been written, the error is returned but the HTTP status
// cannot be changed (headers are already flushed).
//
// Based on: docs/sbi-api-design.md §9 (Content Negotiation)
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// ReadJSON decodes the request body into v. It enforces a maximum body size of
// MaxRequestBodySize (1 MB) and rejects payloads with unknown JSON fields.
//
// Possible errors:
//   - empty request body
//   - body exceeds MaxRequestBodySize
//   - malformed JSON or unknown fields
//
// Based on: docs/sbi-api-design.md §9 (Content Negotiation)
func ReadJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}

	limited := io.LimitReader(r.Body, MaxRequestBodySize+1)
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty request body")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// After decoding the first JSON value, ensure there is no trailing data.
	// A well-formed single JSON document must cause the next Decode to return io.EOF.
	var extra struct{}
	if err := dec.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		// Non-EOF error here indicates trailing junk or a truncated/invalid body.
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Successfully decoded a second top-level JSON value: treat as oversized body
	// or multiple JSON documents, both of which are rejected.
	return errors.New("request body too large")
}

// GetCorrelationInfo returns the value of the 3gpp-Sbi-Correlation-Info header.
// An empty string is returned when the header is absent.
//
// Based on: docs/sbi-api-design.md §5 (3GPP Custom HTTP Headers)
// 3GPP: TS 29.500 §6.10.5 — Correlation information
func GetCorrelationInfo(r *http.Request) string {
	return r.Header.Get(HeaderCorrelationInfo)
}

// GetMessagePriority returns the value of the 3gpp-Sbi-Message-Priority header
// as an integer. It returns -1 when the header is absent or cannot be parsed.
//
// Based on: docs/sbi-api-design.md §5 (3GPP Custom HTTP Headers)
// 3GPP: TS 29.500 §6.10.8 — Message priority
func GetMessagePriority(r *http.Request) int {
	v := r.Header.Get(HeaderMessagePriority)
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}
