package uecm

// Service layer tests for the Nudm_UECM service.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
// Based on: docs/testing-strategy.md (unit testing patterns)

import (
	"testing"
)

// TestNewService verifies the Service constructor returns a properly
// initialized instance.
func TestNewService(t *testing.T) {
	svc := NewService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// TestServiceImplementsInterface verifies that *Service satisfies
// ServiceInterface at compile time.
func TestServiceImplementsInterface(t *testing.T) {
	var _ ServiceInterface = (*Service)(nil)
	// This will fail at compile time if Service does not implement ServiceInterface.
}

// TestValidateUeID tests the identifier validation helper.
func TestValidateUeID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid SUPI", input: "imsi-001010000000001", wantErr: false},
		{name: "valid GPSI msisdn", input: "msisdn-12025551234", wantErr: false},
		{name: "valid GPSI extid", input: "extid-user@example.com", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "bad prefix", input: "unknown-001", wantErr: true},
		{name: "invalid SUPI digits", input: "imsi-12345", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUeID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateUeID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// TestParsePduSessionID tests the PDU session ID parser.
func TestParsePduSessionID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "valid 0", input: "0", want: 0, wantErr: false},
		{name: "valid 5", input: "5", want: 5, wantErr: false},
		{name: "valid 255", input: "255", want: 255, wantErr: false},
		{name: "negative", input: "-1", want: 0, wantErr: true},
		{name: "too large", input: "256", want: 0, wantErr: true},
		{name: "non-numeric", input: "abc", want: 0, wantErr: true},
		{name: "empty", input: "", want: 0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePduSessionID(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parsePduSessionID(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("parsePduSessionID(%q): got %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
