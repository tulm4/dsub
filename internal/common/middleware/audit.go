// Audit logging middleware for the 5G UDM network function.
//
// Based on: docs/security.md §6.7 (Audit Logging Format)
// Based on: docs/security.md §9.1 (Security Event Logging)
// 3GPP: TS 33.501 §6.7 — Privacy and security audit requirements

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tulm4/dsub/internal/common/identifiers"
)

// AuditEventType classifies the audit event.
//
// Based on: docs/security.md §6.7 (Audit Event Types)
type AuditEventType string

const (
	AuditDataAccess           AuditEventType = "DATA_ACCESS"
	AuditDataModification     AuditEventType = "DATA_MODIFICATION"
	AuditAuthenticationEvent  AuditEventType = "AUTHENTICATION_EVENT"
	AuditAuthorizationFailure AuditEventType = "AUTHORIZATION_FAILURE"
	AuditRateLimitBreach      AuditEventType = "RATE_LIMIT_BREACH"
	AuditSecurityEvent        AuditEventType = "SECURITY_EVENT"
)

// AuditEvent represents a single audit log entry.
//
// Based on: docs/security.md §6.7 (Audit Logging Format)
type AuditEvent struct {
	Timestamp      time.Time      `json:"timestamp"`
	EventType      AuditEventType `json:"event_type"`
	Service        string         `json:"service"`
	Operation      string         `json:"operation"`
	SUPIRedacted   string         `json:"supi_redacted,omitempty"`
	SourceNFType   string         `json:"source_nf_type,omitempty"`
	SourceNFID     string         `json:"source_nf_instance,omitempty"`
	OAuth2Scope    string         `json:"oauth2_scope,omitempty"`
	Result         string         `json:"result"`
	FieldsAccessed []string       `json:"fields_accessed,omitempty"`
	TraceID        string         `json:"trace_id,omitempty"`
	HTTPMethod     string         `json:"http_method,omitempty"`
	HTTPPath       string         `json:"http_path,omitempty"`
	HTTPStatus     int            `json:"http_status,omitempty"`
	DurationMs     float64        `json:"duration_ms,omitempty"`
}

// AuditLogger defines the interface for writing audit events.
// Implementations may write to structured logs, Kafka, or a database.
//
// Based on: docs/security.md §6.7 (Audit Log Destinations)
type AuditLogger interface {
	// Log writes an audit event. It must be safe for concurrent use.
	Log(ctx context.Context, event AuditEvent)
}

// SlogAuditLogger writes audit events using slog (structured JSON).
// This is the default implementation; Kafka-backed implementations
// will be added when the message queue infrastructure is available.
//
// Based on: docs/security.md §6.7 (Audit Logging Format)
type SlogAuditLogger struct {
	logger *slog.Logger
}

// NewSlogAuditLogger creates an audit logger that writes to slog.
func NewSlogAuditLogger(logger *slog.Logger) *SlogAuditLogger {
	return &SlogAuditLogger{logger: logger}
}

// Log writes the audit event as a structured JSON log entry.
func (l *SlogAuditLogger) Log(_ context.Context, event AuditEvent) {
	l.logger.Info("audit",
		slog.String("event_type", string(event.EventType)),
		slog.String("service", event.Service),
		slog.String("operation", event.Operation),
		slog.String("supi_redacted", event.SUPIRedacted),
		slog.String("source_nf_type", event.SourceNFType),
		slog.String("source_nf_instance", event.SourceNFID),
		slog.String("oauth2_scope", event.OAuth2Scope),
		slog.String("result", event.Result),
		slog.Any("fields_accessed", event.FieldsAccessed),
		slog.String("trace_id", event.TraceID),
		slog.String("http_method", event.HTTPMethod),
		slog.String("http_path", event.HTTPPath),
		slog.Int("http_status", event.HTTPStatus),
		slog.Float64("duration_ms", event.DurationMs),
	)
}

// InMemoryAuditLogger captures audit events in memory for testing.
type InMemoryAuditLogger struct {
	mu     sync.Mutex
	events []AuditEvent
}

// NewInMemoryAuditLogger creates an audit logger that stores events in memory.
func NewInMemoryAuditLogger() *InMemoryAuditLogger {
	return &InMemoryAuditLogger{}
}

// Log stores the audit event in memory.
func (l *InMemoryAuditLogger) Log(_ context.Context, event AuditEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

// Events returns a copy of all recorded audit events.
func (l *InMemoryAuditLogger) Events() []AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]AuditEvent, len(l.events))
	copy(result, l.events)
	return result
}

// Reset clears all recorded events.
func (l *InMemoryAuditLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = nil
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

// AuditMiddleware logs audit events for all incoming SBI requests.
//
// Based on: docs/security.md §6.7 (Audit Logging)
// Based on: docs/security.md §9.1 (Security Event Logging)
type AuditMiddleware struct {
	auditLogger AuditLogger
	serviceName string
}

// NewAuditMiddleware creates audit logging middleware for the given service.
//
// Based on: docs/security.md §6.7 (Audit Logging)
func NewAuditMiddleware(auditLogger AuditLogger, serviceName string) *AuditMiddleware {
	return &AuditMiddleware{
		auditLogger: auditLogger,
		serviceName: serviceName,
	}
}

// Handler wraps the given handler with audit logging. It records the request
// method, path, duration, status code, and OAuth2 claims (if available).
//
// Based on: docs/security.md §6.7 (Audit Logging Format)
func (m *AuditMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code.
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		// Execute the handler.
		next.ServeHTTP(recorder, r)

		// Build audit event.
		duration := time.Since(start)
		event := AuditEvent{
			Timestamp:  start.UTC(),
			EventType:  classifyAuditEvent(r.Method),
			Service:    m.serviceName,
			Operation:  r.Method + " " + r.URL.Path,
			HTTPMethod: r.Method,
			HTTPPath:   r.URL.Path,
			HTTPStatus: recorder.statusCode,
			DurationMs: float64(duration.Microseconds()) / 1000.0,
			TraceID:    r.Header.Get("3gpp-Sbi-Correlation-Info"),
		}

		// Extract SUPI from URL path if present.
		if supi := extractSUPIFromPath(r.URL.Path); supi != "" {
			event.SUPIRedacted = identifiers.RedactSUPI(supi)
		}

		// Populate OAuth2 claims if available.
		if claims := ClaimsFromContext(r.Context()); claims != nil {
			event.SourceNFType = claims.NFType
			event.SourceNFID = claims.Subject
			event.OAuth2Scope = claims.Scope
		}

		// Determine result.
		if recorder.statusCode >= 200 && recorder.statusCode < 300 {
			event.Result = "SUCCESS"
		} else {
			event.Result = "FAILURE"
		}

		m.auditLogger.Log(r.Context(), event)
	})
}

// classifyAuditEvent determines the audit event type from the HTTP method.
func classifyAuditEvent(method string) AuditEventType {
	switch method {
	case http.MethodGet, http.MethodHead:
		return AuditDataAccess
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return AuditDataModification
	default:
		return AuditDataAccess
	}
}

// extractSUPIFromPath extracts a SUPI (imsi-...) from a URL path.
// SBI paths use patterns like /nudm-sdm/v2/{supi}/... where supi starts
// with "imsi-".
func extractSUPIFromPath(path string) string {
	parts := splitPathSegments(path)
	for _, part := range parts {
		if identifiers.IsSUPI(part) {
			return part
		}
	}
	return ""
}

// splitPathSegments splits a URL path into non-empty segments.
func splitPathSegments(path string) []string {
	var segments []string
	for _, s := range splitBySlash(path) {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

// splitBySlash is a simple path splitter (avoids importing strings for a
// single use).
func splitBySlash(s string) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == '/' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
