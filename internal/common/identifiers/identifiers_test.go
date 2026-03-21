package identifiers

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SUPI Tests
// ---------------------------------------------------------------------------

func TestValidateSUPI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid
		{name: "valid 15-digit IMSI (3+2+10)", input: "imsi-310260000000001", wantErr: false},
		{name: "valid 15-digit IMSI (3+3+9)", input: "imsi-234150123456789", wantErr: false},
		{name: "valid 14-digit IMSI (3+2+9)", input: "imsi-31026000000001", wantErr: false},
		{name: "valid 15-digit all nines", input: "imsi-999999999999999", wantErr: false},
		{name: "valid 14-digit all zeros", input: "imsi-00000000000000", wantErr: false},

		// Invalid
		{name: "empty string", input: "", wantErr: true},
		{name: "missing prefix", input: "310260000000001", wantErr: true},
		{name: "wrong prefix", input: "supi-310260000000001", wantErr: true},
		{name: "too short (13 digits)", input: "imsi-3102600000001", wantErr: true},
		{name: "too long (16 digits)", input: "imsi-3102600000000001", wantErr: true},
		{name: "non-digit characters", input: "imsi-31026000000abcd", wantErr: true},
		{name: "spaces in IMSI", input: "imsi-310 260 000000001", wantErr: true},
		{name: "prefix only", input: "imsi-", wantErr: true},
		{name: "trailing dash", input: "imsi-310260000000001-", wantErr: true},
		{name: "uppercase IMSI prefix", input: "IMSI-310260000000001", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSUPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSUPI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseSUPI(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantMCC         string
		wantMNC         string
		wantMSIN        string
		wantErr         bool
	}{
		{
			name:    "15-digit IMSI (greedy: 3-digit MNC, 9-digit MSIN)",
			input:   "imsi-310260000000001",
			wantMCC: "310", wantMNC: "260", wantMSIN: "000000001",
		},
		{
			name:    "15-digit IMSI with 2-digit MNC and 10-digit MSIN",
			input:   "imsi-310261234567890",
			wantMCC: "310", wantMNC: "261", wantMSIN: "234567890",
		},
		{
			name:    "14-digit IMSI (2-digit MNC, 9-digit MSIN)",
			input:   "imsi-31026000000001",
			wantMCC: "310", wantMNC: "26", wantMSIN: "000000001",
		},
		{
			name:    "all zeros 14-digit",
			input:   "imsi-00000000000000",
			wantMCC: "000", wantMNC: "00", wantMSIN: "000000000",
		},
		{
			name:    "invalid input",
			input:   "bad",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcc, mnc, msin, err := ParseSUPI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSUPI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if mcc != tt.wantMCC {
				t.Errorf("MCC = %q, want %q", mcc, tt.wantMCC)
			}
			if mnc != tt.wantMNC {
				t.Errorf("MNC = %q, want %q", mnc, tt.wantMNC)
			}
			if msin != tt.wantMSIN {
				t.Errorf("MSIN = %q, want %q", msin, tt.wantMSIN)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GPSI Tests
// ---------------------------------------------------------------------------

func TestValidateGPSI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid MSISDN
		{name: "valid msisdn 11 digits", input: "msisdn-12015551234", wantErr: false},
		{name: "valid msisdn min 5 digits", input: "msisdn-12345", wantErr: false},
		{name: "valid msisdn max 15 digits", input: "msisdn-123456789012345", wantErr: false},
		{name: "valid msisdn all zeros", input: "msisdn-00000", wantErr: false},

		// Valid External ID
		{name: "valid extid simple", input: "extid-user@example.com", wantErr: false},
		{name: "valid extid numeric", input: "extid-12345", wantErr: false},
		{name: "valid extid with special chars", input: "extid-a-b_c.d@e", wantErr: false},

		// Invalid MSISDN
		{name: "msisdn too short (4 digits)", input: "msisdn-1234", wantErr: true},
		{name: "msisdn too long (16 digits)", input: "msisdn-1234567890123456", wantErr: true},
		{name: "msisdn with letters", input: "msisdn-1234abcde", wantErr: true},
		{name: "msisdn with plus", input: "msisdn-+12015551234", wantErr: true},
		{name: "msisdn empty value", input: "msisdn-", wantErr: true},

		// Invalid External ID
		{name: "extid empty value", input: "extid-", wantErr: true},

		// Invalid format
		{name: "empty string", input: "", wantErr: true},
		{name: "wrong prefix", input: "gpsi-12015551234", wantErr: true},
		{name: "no prefix", input: "12015551234", wantErr: true},
		{name: "imsi prefix", input: "imsi-310260000000001", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGPSI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGPSI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseGPSI(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFormat string
		wantValue  string
		wantErr    bool
	}{
		{
			name:       "parse msisdn",
			input:      "msisdn-12015551234",
			wantFormat: "msisdn", wantValue: "12015551234",
		},
		{
			name:       "parse extid",
			input:      "extid-user@example.com",
			wantFormat: "extid", wantValue: "user@example.com",
		},
		{
			name:       "parse msisdn min length",
			input:      "msisdn-12345",
			wantFormat: "msisdn", wantValue: "12345",
		},
		{
			name:    "invalid gpsi",
			input:   "bad-input",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, value, err := ParseGPSI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseGPSI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if format != tt.wantFormat {
				t.Errorf("format = %q, want %q", format, tt.wantFormat)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SUCI Tests
// ---------------------------------------------------------------------------

func TestValidateSUCI(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid
		{name: "null scheme (0)", input: "suci-0-310-26-0000-0-0-abcdef1234", wantErr: false},
		{name: "profile A (1)", input: "suci-0-310-260-ff-1-27-abcdef1234567890", wantErr: false},
		{name: "profile B (2)", input: "suci-0-001-01-abcd-2-255-deadbeef", wantErr: false},
		{name: "empty routing ID", input: "suci-0-310-26--0-0-abcdef", wantErr: false},
		{name: "HN key ID 0", input: "suci-0-310-26-0000-1-0-aabb", wantErr: false},
		{name: "HN key ID 255", input: "suci-0-310-26-0000-1-255-aabb", wantErr: false},
		{name: "3-digit MNC", input: "suci-0-310-260-0000-0-0-aabbcc", wantErr: false},
		{name: "2-digit MNC", input: "suci-0-310-26-0000-0-0-aabbcc", wantErr: false},

		// Invalid
		{name: "empty string", input: "", wantErr: true},
		{name: "wrong prefix", input: "imsi-310-26-0000-0-0-abcdef", wantErr: true},
		{name: "missing suci-0 prefix", input: "suci-1-310-26-0000-0-0-abcdef", wantErr: true},
		{name: "scheme_id 3 invalid", input: "suci-0-310-26-0000-3-0-abcdef", wantErr: true},
		{name: "HN key ID 256 invalid", input: "suci-0-310-26-0000-0-256-abcdef", wantErr: true},
		{name: "MCC too short", input: "suci-0-31-26-0000-0-0-abcdef", wantErr: true},
		{name: "MCC too long", input: "suci-0-3100-26-0000-0-0-abcdef", wantErr: true},
		{name: "MNC too short", input: "suci-0-310-2-0000-0-0-abcdef", wantErr: true},
		{name: "MNC too long", input: "suci-0-310-2601-0000-0-0-abcdef", wantErr: true},
		{name: "missing encrypted MSIN", input: "suci-0-310-26-0000-0-0-", wantErr: true},
		{name: "non-hex encrypted MSIN", input: "suci-0-310-26-0000-0-0-xyz123", wantErr: true},
		{name: "non-hex routing ID", input: "suci-0-310-26-ghij-0-0-abcdef", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSUCI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSUCI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseSUCI(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantMCC           string
		wantMNC           string
		wantRoutingID     string
		wantSchemeID      int
		wantHNKeyID       int
		wantEncryptedMSIN string
		wantErr           bool
	}{
		{
			name:              "null scheme",
			input:             "suci-0-310-26-0000-0-0-abcdef1234",
			wantMCC:           "310",
			wantMNC:           "26",
			wantRoutingID:     "0000",
			wantSchemeID:      0,
			wantHNKeyID:       0,
			wantEncryptedMSIN: "abcdef1234",
		},
		{
			name:              "profile A with 3-digit MNC",
			input:             "suci-0-310-260-ff-1-27-deadbeef",
			wantMCC:           "310",
			wantMNC:           "260",
			wantRoutingID:     "ff",
			wantSchemeID:      1,
			wantHNKeyID:       27,
			wantEncryptedMSIN: "deadbeef",
		},
		{
			name:              "profile B max HN key ID",
			input:             "suci-0-001-01-abcd-2-255-cafebabe",
			wantMCC:           "001",
			wantMNC:           "01",
			wantRoutingID:     "abcd",
			wantSchemeID:      2,
			wantHNKeyID:       255,
			wantEncryptedMSIN: "cafebabe",
		},
		{
			name:              "empty routing ID",
			input:             "suci-0-310-26--0-0-aabb",
			wantMCC:           "310",
			wantMNC:           "26",
			wantRoutingID:     "",
			wantSchemeID:      0,
			wantHNKeyID:       0,
			wantEncryptedMSIN: "aabb",
		},
		{
			name:    "invalid SUCI",
			input:   "not-a-suci",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := ParseSUCI(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSUCI(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if c.MCC != tt.wantMCC {
				t.Errorf("MCC = %q, want %q", c.MCC, tt.wantMCC)
			}
			if c.MNC != tt.wantMNC {
				t.Errorf("MNC = %q, want %q", c.MNC, tt.wantMNC)
			}
			if c.RoutingID != tt.wantRoutingID {
				t.Errorf("RoutingID = %q, want %q", c.RoutingID, tt.wantRoutingID)
			}
			if c.SchemeID != tt.wantSchemeID {
				t.Errorf("SchemeID = %d, want %d", c.SchemeID, tt.wantSchemeID)
			}
			if c.HNKeyID != tt.wantHNKeyID {
				t.Errorf("HNKeyID = %d, want %d", c.HNKeyID, tt.wantHNKeyID)
			}
			if c.EncryptedMSIN != tt.wantEncryptedMSIN {
				t.Errorf("EncryptedMSIN = %q, want %q", c.EncryptedMSIN, tt.wantEncryptedMSIN)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper Tests
// ---------------------------------------------------------------------------

func TestIsSUPI(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"imsi-310260000000001", true},
		{"imsi-", true}, // prefix check only
		{"IMSI-310260000000001", false},
		{"msisdn-12345", false},
		{"suci-0-310-26-0000-0-0-ab", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsSUPI(tt.input); got != tt.want {
				t.Errorf("IsSUPI(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsGPSI(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"msisdn-12015551234", true},
		{"extid-user@example.com", true},
		{"msisdn-", true}, // prefix check only
		{"extid-", true},  // prefix check only
		{"imsi-310260000000001", false},
		{"suci-0-310-26-0000-0-0-ab", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsGPSI(tt.input); got != tt.want {
				t.Errorf("IsGPSI(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSUCI(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"suci-0-310-26-0000-0-0-abcdef", true},
		{"suci-", true}, // prefix check only
		{"imsi-310260000000001", false},
		{"msisdn-12345", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsSUCI(tt.input); got != tt.want {
				t.Errorf("IsSUCI(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRedactSUPI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard 15-digit IMSI",
			input: "imsi-310260000000001",
			want:  "imsi-***000001",
		},
		{
			name:  "14-digit IMSI",
			input: "imsi-31026000000001",
			want:  "imsi-***000001",
		},
		{
			name:  "short numeric (6 digits or fewer)",
			input: "imsi-123456",
			want:  "imsi-123456",
		},
		{
			name:  "exactly 6 digits",
			input: "imsi-123456",
			want:  "imsi-123456",
		},
		{
			name:  "7 digits",
			input: "imsi-1234567",
			want:  "imsi-***234567",
		},
		{
			name:  "not a SUPI",
			input: "msisdn-12345",
			want:  "***REDACTED***",
		},
		{
			name:  "empty string",
			input: "",
			want:  "***REDACTED***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactSUPI(tt.input)
			if got != tt.want {
				t.Errorf("RedactSUPI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRedactSUPI_NoLeakage verifies that a redacted SUPI does not contain the
// full original numeric portion.
func TestRedactSUPI_NoLeakage(t *testing.T) {
	supi := "imsi-310260000000001"
	redacted := RedactSUPI(supi)
	full := strings.TrimPrefix(supi, "imsi-")
	if strings.Contains(redacted, full) {
		t.Errorf("RedactSUPI leaked full IMSI: %q contains %q", redacted, full)
	}
}
