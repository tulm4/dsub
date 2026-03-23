// Security event logging for the 5G UDM network function.
//
// Based on: docs/security.md §9.1 (Security Event Logging)
// Based on: docs/security.md §9.2 (SIEM Integration)
// 3GPP: TS 33.501 §6.7 — Security audit and event monitoring

package middleware

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SecuritySeverity classifies the severity level of a security event.
//
// Based on: docs/security.md §9.1 (Security Event Severity Levels)
type SecuritySeverity string

const (
	SeverityWarning  SecuritySeverity = "WARNING"
	SeverityHigh     SecuritySeverity = "HIGH"
	SeverityCritical SecuritySeverity = "CRITICAL"
)

// SecurityEventCategory classifies the category of a security event.
//
// Based on: docs/security.md §9.1 (Security Event Categories)
type SecurityEventCategory string

const (
	// CategoryAuthenticationFailure covers invalid token, expired token, scope mismatch.
	CategoryAuthenticationFailure SecurityEventCategory = "AUTHENTICATION_FAILURE"
	// CategoryAuthorizationViolation covers NF accessing unauthorized SUPI, scope escalation.
	CategoryAuthorizationViolation SecurityEventCategory = "AUTHORIZATION_VIOLATION"
	// CategoryCredentialAnomaly covers bulk K/OPc reads, unusual SQN resync rate.
	CategoryCredentialAnomaly SecurityEventCategory = "CREDENTIAL_ACCESS_ANOMALY"
	// CategoryIdentityAttack covers SUCI deconceal failure, unknown HN_pub_key_id.
	CategoryIdentityAttack SecurityEventCategory = "IDENTITY_ATTACK"
	// CategoryInfrastructure covers HSM unreachable, cert expiry imminent, TLS failure.
	CategoryInfrastructure SecurityEventCategory = "INFRASTRUCTURE"
	// CategoryRateLimitBreach covers sustained rate limit violations by single NF.
	CategoryRateLimitBreach SecurityEventCategory = "RATE_LIMIT_BREACH"
)

// SecurityEvent represents a structured security event for SIEM integration.
//
// Based on: docs/security.md §9.1 (Security Event Logging)
// Based on: docs/security.md §9.2 (SIEM Integration — Fluentd → Kafka → SIEM)
type SecurityEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Category    SecurityEventCategory  `json:"category"`
	Severity    SecuritySeverity       `json:"severity"`
	Service     string                 `json:"service"`
	Description string                 `json:"description"`
	SourceNFID  string                 `json:"source_nf_instance,omitempty"`
	SourceNFType string               `json:"source_nf_type,omitempty"`
	TargetSUPI  string                 `json:"target_supi_redacted,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	Details     map[string]string      `json:"details,omitempty"`
}

// SecurityEventLogger defines the interface for publishing security events.
// Implementations may write to slog, Kafka (udm.security.events topic),
// or an external SIEM system.
//
// Based on: docs/security.md §9.2 (SIEM Integration)
type SecurityEventLogger interface {
	// LogEvent publishes a security event. Must be safe for concurrent use.
	LogEvent(ctx context.Context, event SecurityEvent)
}

// SlogSecurityEventLogger writes security events using slog (structured JSON).
// Events are written to stdout for collection by Fluentd sidecar, which
// forwards to the Kafka topic udm.security.events.
//
// Based on: docs/security.md §9.2 (SIEM Integration Pipeline)
type SlogSecurityEventLogger struct {
	logger *slog.Logger
}

// NewSlogSecurityEventLogger creates a security event logger that writes to slog.
func NewSlogSecurityEventLogger(logger *slog.Logger) *SlogSecurityEventLogger {
	return &SlogSecurityEventLogger{logger: logger}
}

// LogEvent writes the security event as a structured JSON log entry.
func (l *SlogSecurityEventLogger) LogEvent(_ context.Context, event SecurityEvent) {
	level := slog.LevelWarn
	switch event.Severity {
	case SeverityCritical:
		level = slog.LevelError
	case SeverityHigh:
		level = slog.LevelError
	case SeverityWarning:
		level = slog.LevelWarn
	}

	l.logger.Log(context.Background(), level, "security_event",
		slog.String("category", string(event.Category)),
		slog.String("severity", string(event.Severity)),
		slog.String("service", event.Service),
		slog.String("description", event.Description),
		slog.String("source_nf_instance", event.SourceNFID),
		slog.String("source_nf_type", event.SourceNFType),
		slog.String("target_supi_redacted", event.TargetSUPI),
		slog.String("trace_id", event.TraceID),
		slog.Any("details", event.Details),
	)
}

// InMemorySecurityEventLogger captures security events in memory for testing.
type InMemorySecurityEventLogger struct {
	mu     sync.Mutex
	events []SecurityEvent
}

// NewInMemorySecurityEventLogger creates a security event logger that stores
// events in memory.
func NewInMemorySecurityEventLogger() *InMemorySecurityEventLogger {
	return &InMemorySecurityEventLogger{}
}

// LogEvent stores the security event in memory.
func (l *InMemorySecurityEventLogger) LogEvent(_ context.Context, event SecurityEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

// Events returns a copy of all recorded security events.
func (l *InMemorySecurityEventLogger) Events() []SecurityEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]SecurityEvent, len(l.events))
	copy(result, l.events)
	return result
}

// Reset clears all recorded events.
func (l *InMemorySecurityEventLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = nil
}

// ---------------------------------------------------------------------------
// Threat detection rule thresholds
// Based on: docs/security.md §9.2 (Threat Detection Rules)
// ---------------------------------------------------------------------------

// ThreatThresholds defines configurable thresholds for threat detection rules.
//
// Based on: docs/security.md §9.2 (Threat Detection Rules)
type ThreatThresholds struct {
	// BruteForceSUCIMaxFails is the maximum number of failed SUCI deconceal
	// attempts from a single NF in BruteForceSUCIWindow before raising an alert.
	// Default: 100
	BruteForceSUCIMaxFails int
	// BruteForceSUCIWindow is the time window for brute-force SUCI detection.
	// Default: 60 seconds
	BruteForceSUCIWindow time.Duration

	// SQNResyncMaxRequests is the maximum number of SQN resync requests for a
	// single SUPI in SQNResyncWindow before raising an alert.
	// Default: 50
	SQNResyncMaxRequests int
	// SQNResyncWindow is the time window for SQN resync storm detection.
	// Default: 24 hours
	SQNResyncWindow time.Duration

	// CredentialScanMaxSUPIs is the maximum distinct SUPIs queried by a single
	// NF in CredentialScanWindow before raising an alert.
	// Default: 1000
	CredentialScanMaxSUPIs int
	// CredentialScanWindow is the time window for credential scan detection.
	// Default: 10 minutes
	CredentialScanWindow time.Duration
}

// DefaultThreatThresholds returns the default thresholds for threat detection.
//
// Based on: docs/security.md §9.2 (Threat Detection Rules)
func DefaultThreatThresholds() ThreatThresholds {
	return ThreatThresholds{
		BruteForceSUCIMaxFails: 100,
		BruteForceSUCIWindow:   60 * time.Second,
		SQNResyncMaxRequests:   50,
		SQNResyncWindow:        24 * time.Hour,
		CredentialScanMaxSUPIs: 1000,
		CredentialScanWindow:   10 * time.Minute,
	}
}
