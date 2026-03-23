package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/tulm4/dsub/internal/common/logging"
)

// ---------------------------------------------------------------------------
// SecurityEvent tests
// ---------------------------------------------------------------------------

func TestSecuritySeverity_Constants(t *testing.T) {
	severities := []SecuritySeverity{SeverityWarning, SeverityHigh, SeverityCritical}
	seen := make(map[SecuritySeverity]bool)
	for _, s := range severities {
		if s == "" {
			t.Error("severity constant must not be empty")
		}
		if seen[s] {
			t.Errorf("duplicate severity: %q", s)
		}
		seen[s] = true
	}
}

func TestSecurityEventCategory_Constants(t *testing.T) {
	categories := []SecurityEventCategory{
		CategoryAuthenticationFailure,
		CategoryAuthorizationViolation,
		CategoryCredentialAnomaly,
		CategoryIdentityAttack,
		CategoryInfrastructure,
		CategoryRateLimitBreach,
	}
	seen := make(map[SecurityEventCategory]bool)
	for _, c := range categories {
		if c == "" {
			t.Error("category constant must not be empty")
		}
		if seen[c] {
			t.Errorf("duplicate category: %q", c)
		}
		seen[c] = true
	}
}

// ---------------------------------------------------------------------------
// InMemorySecurityEventLogger tests
// ---------------------------------------------------------------------------

func TestInMemorySecurityEventLogger_LogAndRetrieve(t *testing.T) {
	logger := NewInMemorySecurityEventLogger()

	event := SecurityEvent{
		Timestamp:   time.Now(),
		Category:    CategoryAuthenticationFailure,
		Severity:    SeverityWarning,
		Service:     "udm-ueau",
		Description: "invalid access token from AUSF",
		SourceNFID:  "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org",
		SourceNFType: "AUSF",
	}
	logger.LogEvent(context.Background(), event)

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Category != CategoryAuthenticationFailure {
		t.Errorf("Category = %q, want %q", events[0].Category, CategoryAuthenticationFailure)
	}
	if events[0].Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", events[0].Severity, SeverityWarning)
	}
	if events[0].Service != "udm-ueau" {
		t.Errorf("Service = %q, want %q", events[0].Service, "udm-ueau")
	}
}

func TestInMemorySecurityEventLogger_MultipleEvents(t *testing.T) {
	logger := NewInMemorySecurityEventLogger()

	events := []SecurityEvent{
		{Category: CategoryAuthenticationFailure, Severity: SeverityWarning},
		{Category: CategoryCredentialAnomaly, Severity: SeverityCritical},
		{Category: CategoryIdentityAttack, Severity: SeverityHigh},
	}
	for _, e := range events {
		logger.LogEvent(context.Background(), e)
	}

	got := logger.Events()
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
}

func TestInMemorySecurityEventLogger_Reset(t *testing.T) {
	logger := NewInMemorySecurityEventLogger()
	logger.LogEvent(context.Background(), SecurityEvent{Category: CategoryInfrastructure})
	logger.LogEvent(context.Background(), SecurityEvent{Category: CategoryRateLimitBreach})

	if len(logger.Events()) != 2 {
		t.Fatalf("expected 2 events before reset")
	}

	logger.Reset()
	if len(logger.Events()) != 0 {
		t.Errorf("expected 0 events after reset, got %d", len(logger.Events()))
	}
}

func TestInMemorySecurityEventLogger_EventsCopyIsolation(t *testing.T) {
	logger := NewInMemorySecurityEventLogger()
	logger.LogEvent(context.Background(), SecurityEvent{Category: CategoryInfrastructure})

	events := logger.Events()
	events[0].Category = "MODIFIED" // modify the copy

	// Original should be unaffected.
	original := logger.Events()
	if original[0].Category != CategoryInfrastructure {
		t.Errorf("original event was mutated: %q", original[0].Category)
	}
}

// ---------------------------------------------------------------------------
// SlogSecurityEventLogger tests
// ---------------------------------------------------------------------------

func TestSlogSecurityEventLogger_DoesNotPanic(t *testing.T) {
	slogLogger := logging.NewLogger("error", "test", "us-east")
	secLogger := NewSlogSecurityEventLogger(slogLogger)

	// Should not panic for any severity level.
	severities := []SecuritySeverity{SeverityWarning, SeverityHigh, SeverityCritical}
	for _, sev := range severities {
		secLogger.LogEvent(context.Background(), SecurityEvent{
			Timestamp:   time.Now(),
			Category:    CategoryAuthenticationFailure,
			Severity:    sev,
			Service:     "udm-ueau",
			Description: "test event",
		})
	}
}

// ---------------------------------------------------------------------------
// DefaultThreatThresholds tests
// ---------------------------------------------------------------------------

func TestDefaultThreatThresholds(t *testing.T) {
	thresholds := DefaultThreatThresholds()

	if thresholds.BruteForceSUCIMaxFails != 100 {
		t.Errorf("BruteForceSUCIMaxFails = %d, want 100", thresholds.BruteForceSUCIMaxFails)
	}
	if thresholds.BruteForceSUCIWindow != 60*time.Second {
		t.Errorf("BruteForceSUCIWindow = %v, want 60s", thresholds.BruteForceSUCIWindow)
	}
	if thresholds.SQNResyncMaxRequests != 50 {
		t.Errorf("SQNResyncMaxRequests = %d, want 50", thresholds.SQNResyncMaxRequests)
	}
	if thresholds.SQNResyncWindow != 24*time.Hour {
		t.Errorf("SQNResyncWindow = %v, want 24h", thresholds.SQNResyncWindow)
	}
	if thresholds.CredentialScanMaxSUPIs != 1000 {
		t.Errorf("CredentialScanMaxSUPIs = %d, want 1000", thresholds.CredentialScanMaxSUPIs)
	}
	if thresholds.CredentialScanWindow != 10*time.Minute {
		t.Errorf("CredentialScanWindow = %v, want 10m", thresholds.CredentialScanWindow)
	}
}

// ---------------------------------------------------------------------------
// SecurityEvent full-field test
// ---------------------------------------------------------------------------

func TestSecurityEvent_AllFields(t *testing.T) {
	now := time.Now()
	event := SecurityEvent{
		Timestamp:    now,
		Category:     CategoryCredentialAnomaly,
		Severity:     SeverityCritical,
		Service:      "udm-ueau",
		Description:  "bulk K/OPc read detected",
		SourceNFID:   "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org",
		SourceNFType: "AUSF",
		TargetSUPI:   "imsi-***000001",
		TraceID:      "trace-abc-123",
		Details: map[string]string{
			"supi_count":   "1500",
			"time_window":  "10m",
			"threshold":    "1000",
		},
	}

	logger := NewInMemorySecurityEventLogger()
	logger.LogEvent(context.Background(), event)

	got := logger.Events()[0]
	if got.Category != CategoryCredentialAnomaly {
		t.Errorf("Category = %q, want %q", got.Category, CategoryCredentialAnomaly)
	}
	if got.Severity != SeverityCritical {
		t.Errorf("Severity = %q, want %q", got.Severity, SeverityCritical)
	}
	if got.TargetSUPI != "imsi-***000001" {
		t.Errorf("TargetSUPI = %q, want %q", got.TargetSUPI, "imsi-***000001")
	}
	if got.Details["supi_count"] != "1500" {
		t.Errorf("Details[supi_count] = %q, want %q", got.Details["supi_count"], "1500")
	}
}
