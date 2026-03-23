package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tulm4/dsub/internal/common/logging"
)

// ---------------------------------------------------------------------------
// InMemoryAuditLogger tests
// ---------------------------------------------------------------------------

func TestInMemoryAuditLogger_Log(t *testing.T) {
	logger := NewInMemoryAuditLogger()

	event := AuditEvent{
		EventType: AuditDataAccess,
		Service:   "udm-ueau",
		Operation: "GET /nudm-ueau/v1/imsi-310260000000001/security-information",
		Result:    "SUCCESS",
	}
	logger.Log(nil, event)

	events := logger.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Service != "udm-ueau" {
		t.Errorf("Service = %q, want %q", events[0].Service, "udm-ueau")
	}
}

func TestInMemoryAuditLogger_Reset(t *testing.T) {
	logger := NewInMemoryAuditLogger()
	logger.Log(nil, AuditEvent{EventType: AuditDataAccess})
	logger.Log(nil, AuditEvent{EventType: AuditDataModification})

	if len(logger.Events()) != 2 {
		t.Fatalf("expected 2 events before reset")
	}

	logger.Reset()
	if len(logger.Events()) != 0 {
		t.Errorf("expected 0 events after reset, got %d", len(logger.Events()))
	}
}

// ---------------------------------------------------------------------------
// AuditMiddleware tests
// ---------------------------------------------------------------------------

func TestAuditMiddleware_LogsGETAsDataAccess(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-sdm")

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-sdm/v2/imsi-310260000000001/am-data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != AuditDataAccess {
		t.Errorf("EventType = %q, want %q", event.EventType, AuditDataAccess)
	}
	if event.Service != "udm-sdm" {
		t.Errorf("Service = %q, want %q", event.Service, "udm-sdm")
	}
	if event.SUPIRedacted != "imsi-***000001" {
		t.Errorf("SUPIRedacted = %q, want %q", event.SUPIRedacted, "imsi-***000001")
	}
	if event.Result != "SUCCESS" {
		t.Errorf("Result = %q, want %q", event.Result, "SUCCESS")
	}
	if event.HTTPMethod != "GET" {
		t.Errorf("HTTPMethod = %q, want %q", event.HTTPMethod, "GET")
	}
	if event.HTTPStatus != http.StatusOK {
		t.Errorf("HTTPStatus = %d, want %d", event.HTTPStatus, http.StatusOK)
	}
}

func TestAuditMiddleware_LogsPOSTAsDataModification(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-uecm")

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/nudm-uecm/v1/imsi-310260000000001/registrations/amf-3gpp-access", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != AuditDataModification {
		t.Errorf("EventType = %q, want %q", event.EventType, AuditDataModification)
	}
	if event.HTTPStatus != http.StatusCreated {
		t.Errorf("HTTPStatus = %d, want %d", event.HTTPStatus, http.StatusCreated)
	}
}

func TestAuditMiddleware_FailureResult(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-sdm")

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nudm-sdm/v2/imsi-310260000000001/am-data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Result != "FAILURE" {
		t.Errorf("Result = %q, want %q", events[0].Result, "FAILURE")
	}
}

func TestAuditMiddleware_WithOAuth2Claims(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-ueau")

	// Simulate handler that already has claims in context (from OAuth2 middleware).
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.Handler(innerHandler)

	req := httptest.NewRequest(http.MethodPost, "/nudm-ueau/v1/imsi-310260000000001/security-information/generate-auth-data", nil)
	// Set trace ID header.
	req.Header.Set("3gpp-Sbi-Correlation-Info", "trace-abc-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].TraceID != "trace-abc-123" {
		t.Errorf("TraceID = %q, want %q", events[0].TraceID, "trace-abc-123")
	}
}

func TestAuditMiddleware_NoSUPIInPath(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-sdm")

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].SUPIRedacted != "" {
		t.Errorf("SUPIRedacted should be empty for non-SUPI path, got %q", events[0].SUPIRedacted)
	}
}

func TestAuditMiddleware_DurationTracked(t *testing.T) {
	auditLog := NewInMemoryAuditLogger()
	mw := NewAuditMiddleware(auditLog, "udm-sdm")

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	events := auditLog.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].DurationMs < 0 {
		t.Errorf("DurationMs should be non-negative, got %f", events[0].DurationMs)
	}
}

// ---------------------------------------------------------------------------
// SlogAuditLogger tests
// ---------------------------------------------------------------------------

func TestSlogAuditLogger_DoesNotPanic(t *testing.T) {
	slogLogger := logging.NewLogger("error", "test", "us-east")
	auditLogger := NewSlogAuditLogger(slogLogger)

	// Should not panic.
	auditLogger.Log(nil, AuditEvent{
		EventType: AuditDataAccess,
		Service:   "udm-sdm",
		Operation: "GET /test",
		Result:    "SUCCESS",
	})
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestClassifyAuditEvent(t *testing.T) {
	tests := []struct {
		method string
		want   AuditEventType
	}{
		{http.MethodGet, AuditDataAccess},
		{http.MethodHead, AuditDataAccess},
		{http.MethodPost, AuditDataModification},
		{http.MethodPut, AuditDataModification},
		{http.MethodPatch, AuditDataModification},
		{http.MethodDelete, AuditDataModification},
		{http.MethodOptions, AuditDataAccess},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := classifyAuditEvent(tt.method); got != tt.want {
				t.Errorf("classifyAuditEvent(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestExtractSUPIFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "SUPI in SDM path",
			path: "/nudm-sdm/v2/imsi-310260000000001/am-data",
			want: "imsi-310260000000001",
		},
		{
			name: "SUPI in UEAU path",
			path: "/nudm-ueau/v1/imsi-310260000000001/security-information",
			want: "imsi-310260000000001",
		},
		{
			name: "no SUPI",
			path: "/healthz/ready",
			want: "",
		},
		{
			name: "SUCI not SUPI",
			path: "/nudm-ueau/v1/suci-0-310-260-1234-1-0-abcdef/security-information",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSUPIFromPath(tt.path); got != tt.want {
				t.Errorf("extractSUPIFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
