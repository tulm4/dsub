package sdm

// Service layer tests for the Nudm_SDM service.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
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
	// The explicit nil assignment is a standard Go pattern for compile-time checks.
}

// TestValidateSUPIOrUeId tests the identifier validation helper.
func TestValidateSUPIOrUeId(t *testing.T) {
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
			err := validateSUPIOrUeId(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateSUPIOrUeId(%q): got err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}
