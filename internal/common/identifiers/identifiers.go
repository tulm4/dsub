// Package identifiers provides parsing and validation for 3GPP subscriber
// identifiers (SUPI, GPSI, SUCI) as defined in TS 23.003.
//
// Based on: docs/service-decomposition.md §3.1 (common/identifiers)
// 3GPP: TS 23.003 — Numbering, Addressing and Identification
// 3GPP: TS 29.503 — Nudm Services (identifier formats in API definitions)
// 3GPP: TS 33.501 §6.12 — SUCI construction and SUPI concealment
package identifiers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Regex patterns for identifier validation.
//
// SUPI (IMSI-based): imsi-<MCC 3 digits><MNC 2-3 digits><MSIN 9-10 digits>
// Total numeric part is 14-15 digits per TS 23.003 §2.2.
var supiRegex = regexp.MustCompile(`^imsi-(\d{3})(\d{2,3})(\d{9,10})$`)

// GPSI (MSISDN-based): msisdn-<E.164 number, 5-15 digits>
var msisdnRegex = regexp.MustCompile(`^msisdn-(\d{5,15})$`)

// GPSI (External ID-based): extid-<non-empty string>
var extidRegex = regexp.MustCompile(`^extid-(.+)$`)

// SUCI: suci-0-<MCC>-<MNC>-<routing_id>-<scheme_id>-<HN_pub_key_id>-<encrypted_MSIN>
// 3GPP: TS 23.003 §2.2B — SUCI format for IMSI-based SUPI
var suciRegex = regexp.MustCompile(
	`^suci-0-(\d{3})-(\d{2,3})-([0-9a-fA-F]*)-([0-2])-(\d{1,3})-([0-9a-fA-F]+)$`,
)

// SUCIComponents holds the parsed fields of a SUCI identifier.
//
// 3GPP: TS 23.003 §2.2B
type SUCIComponents struct {
	MCC           string // Mobile Country Code (3 digits)
	MNC           string // Mobile Network Code (2-3 digits)
	RoutingID     string // Routing Indicator (hex, may be empty)
	SchemeID      int    // Protection scheme: 0=null, 1=Profile A, 2=Profile B
	HNKeyID       int    // Home Network Public Key Identifier (0-255)
	EncryptedMSIN string // Hex-encoded concealed MSIN
}

// ---------------------------------------------------------------------------
// SUPI — Subscription Permanent Identifier
// 3GPP: TS 23.003 §2.2
// ---------------------------------------------------------------------------

// ValidateSUPI validates that the given string is a well-formed SUPI in
// IMSI format: imsi-<MCC><MNC><MSIN> with 14-15 total digits.
func ValidateSUPI(supi string) error {
	if supi == "" {
		return fmt.Errorf("SUPI must not be empty")
	}
	m := supiRegex.FindStringSubmatch(supi)
	if m == nil {
		return fmt.Errorf("invalid SUPI format: must match imsi-<MCC 3><MNC 2-3><MSIN 9-10>: %s", supi)
	}
	totalDigits := len(m[1]) + len(m[2]) + len(m[3])
	if totalDigits < 14 || totalDigits > 15 {
		return fmt.Errorf("invalid SUPI: total IMSI digits must be 14-15, got %d: %s", totalDigits, supi)
	}
	return nil
}

// ParseSUPI parses a SUPI string and returns its MCC, MNC, and MSIN
// components. It returns an error if the SUPI format is invalid.
func ParseSUPI(supi string) (mcc, mnc, msin string, err error) {
	if err = ValidateSUPI(supi); err != nil {
		return "", "", "", err
	}
	m := supiRegex.FindStringSubmatch(supi)
	return m[1], m[2], m[3], nil
}

// ---------------------------------------------------------------------------
// GPSI — Generic Public Subscription Identifier
// 3GPP: TS 23.003 §2.2A
// ---------------------------------------------------------------------------

// ValidateGPSI validates that the given string is a well-formed GPSI in
// either MSISDN format (msisdn-<E.164>) or External ID format (extid-<id>).
func ValidateGPSI(gpsi string) error {
	if gpsi == "" {
		return fmt.Errorf("GPSI must not be empty")
	}
	if strings.HasPrefix(gpsi, "msisdn-") {
		if !msisdnRegex.MatchString(gpsi) {
			return fmt.Errorf("invalid GPSI MSISDN format: must be msisdn-<5-15 digits>: %s", gpsi)
		}
		return nil
	}
	if strings.HasPrefix(gpsi, "extid-") {
		if !extidRegex.MatchString(gpsi) {
			return fmt.Errorf("invalid GPSI External ID format: must be extid-<non-empty>: %s", gpsi)
		}
		return nil
	}
	return fmt.Errorf("invalid GPSI format: must start with msisdn- or extid-: %s", gpsi)
}

// ParseGPSI parses a GPSI string and returns the format type ("msisdn" or
// "extid") and the identifier value. It returns an error if the format is
// invalid.
func ParseGPSI(gpsi string) (format, value string, err error) {
	if err = ValidateGPSI(gpsi); err != nil {
		return "", "", err
	}
	if strings.HasPrefix(gpsi, "msisdn-") {
		return "msisdn", strings.TrimPrefix(gpsi, "msisdn-"), nil
	}
	return "extid", strings.TrimPrefix(gpsi, "extid-"), nil
}

// ---------------------------------------------------------------------------
// SUCI — Subscription Concealed Identifier
// 3GPP: TS 23.003 §2.2B, TS 33.501 §6.12
// ---------------------------------------------------------------------------

// ValidateSUCI validates that the given string is a well-formed SUCI.
func ValidateSUCI(suci string) error {
	if suci == "" {
		return fmt.Errorf("SUCI must not be empty")
	}
	m := suciRegex.FindStringSubmatch(suci)
	if m == nil {
		return fmt.Errorf(
			"invalid SUCI format: must match suci-0-<MCC>-<MNC>-<routing_id>-<scheme_id>-<HN_key_id>-<encrypted_MSIN>: %s",
			suci,
		)
	}

	schemeID, err := strconv.Atoi(m[4])
	if err != nil || schemeID < 0 || schemeID > 2 {
		return fmt.Errorf("invalid SUCI scheme_id: must be 0, 1, or 2: %s", suci)
	}

	hnKeyID, err := strconv.Atoi(m[5])
	if err != nil || hnKeyID < 0 || hnKeyID > 255 {
		return fmt.Errorf("invalid SUCI HN_pub_key_id: must be 0-255: %s", suci)
	}

	return nil
}

// ParseSUCI parses a SUCI string into its component fields. It returns an
// error if the SUCI format is invalid.
func ParseSUCI(suci string) (*SUCIComponents, error) {
	if err := ValidateSUCI(suci); err != nil {
		return nil, err
	}
	m := suciRegex.FindStringSubmatch(suci)

	schemeID, _ := strconv.Atoi(m[4])
	hnKeyID, _ := strconv.Atoi(m[5])

	return &SUCIComponents{
		MCC:           m[1],
		MNC:           m[2],
		RoutingID:     m[3],
		SchemeID:      schemeID,
		HNKeyID:       hnKeyID,
		EncryptedMSIN: m[6],
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// IsSUPI returns true if the string looks like a SUPI (starts with "imsi-").
func IsSUPI(id string) bool {
	return strings.HasPrefix(id, "imsi-")
}

// IsGPSI returns true if the string looks like a GPSI (starts with "msisdn-"
// or "extid-").
func IsGPSI(id string) bool {
	return strings.HasPrefix(id, "msisdn-") || strings.HasPrefix(id, "extid-")
}

// IsSUCI returns true if the string looks like a SUCI (starts with "suci-").
func IsSUCI(id string) bool {
	return strings.HasPrefix(id, "suci-")
}

// RedactSUPI redacts a SUPI for safe logging, preserving only the last 6
// digits: imsi-***<last6>. Non-SUPI strings are returned as "***REDACTED***".
func RedactSUPI(supi string) string {
	if !IsSUPI(supi) {
		return "***REDACTED***"
	}
	numeric := strings.TrimPrefix(supi, "imsi-")
	if len(numeric) <= 6 {
		return "imsi-" + numeric
	}
	return "imsi-***" + numeric[len(numeric)-6:]
}
