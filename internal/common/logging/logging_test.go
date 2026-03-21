package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

// newTestLogger creates a logger that writes JSON to the returned buffer,
// useful for capturing and inspecting structured log output in tests.
func newTestLogger(level slog.Level, attrs ...slog.Attr) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	if len(attrs) > 0 {
		args := make([]any, len(attrs))
		for i, a := range attrs {
			args[i] = a
		}
		logger = logger.With(args...)
	}
	return logger, &buf
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{name: "debug", input: "debug", want: slog.LevelDebug},
		{name: "info", input: "info", want: slog.LevelInfo},
		{name: "warn", input: "warn", want: slog.LevelWarn},
		{name: "error", input: "error", want: slog.LevelError},
		{name: "upper_case", input: "DEBUG", want: slog.LevelDebug},
		{name: "mixed_case", input: "Warn", want: slog.LevelWarn},
		{name: "with_spaces", input: "  info  ", want: slog.LevelInfo},
		{name: "unknown_defaults_to_info", input: "trace", want: slog.LevelInfo},
		{name: "empty_defaults_to_info", input: "", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewLogger_OutputsJSON(t *testing.T) {
	// Use the test helper to create a logger writing to a buffer so we can
	// verify structured output without depending on os.Stdout.
	logger, buf := newTestLogger(slog.LevelInfo,
		slog.String("service", "udm-ueau"),
		slog.String("region", "us-east-1"),
	)

	logger.Info("test message", slog.String("key", "value"))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	// Verify required fields.
	checks := map[string]string{
		"msg":     "test message",
		"service": "udm-ueau",
		"region":  "us-east-1",
		"key":     "value",
	}
	for field, want := range checks {
		got, ok := entry[field].(string)
		if !ok {
			t.Errorf("expected field %q in JSON output, not found; entry=%v", field, entry)
			continue
		}
		if got != want {
			t.Errorf("field %q = %q, want %q", field, got, want)
		}
	}

	// Verify the level field is present.
	if _, ok := entry["level"]; !ok {
		t.Error("expected 'level' field in JSON output")
	}
}

func TestNewLogger_RespectsLevel(t *testing.T) {
	logger, buf := newTestLogger(slog.LevelWarn)

	logger.Info("should be filtered")
	if buf.Len() != 0 {
		t.Errorf("expected Info message to be filtered at Warn level, got: %s", buf.String())
	}

	logger.Warn("should appear")
	if buf.Len() == 0 {
		t.Error("expected Warn message to appear at Warn level, but buffer is empty")
	}
}

func TestWithTraceID(t *testing.T) {
	logger, buf := newTestLogger(slog.LevelInfo)
	traced := WithTraceID(logger, "abc-123-def")

	traced.Info("traced message")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	got, ok := entry["trace_id"].(string)
	if !ok {
		t.Fatalf("expected 'trace_id' field in JSON output; entry=%v", entry)
	}
	if got != "abc-123-def" {
		t.Errorf("trace_id = %q, want %q", got, "abc-123-def")
	}
}

func TestWithSUPI(t *testing.T) {
	tests := []struct {
		name     string
		supi     string
		wantSUPI string
	}{
		{
			name:     "valid_supi_redacted",
			supi:     "imsi-310028900000001",
			wantSUPI: "imsi-***000001",
		},
		{
			name:     "short_supi_preserved",
			supi:     "imsi-123456",
			wantSUPI: "imsi-123456",
		},
		{
			name:     "invalid_supi_fully_redacted",
			supi:     "not-a-supi",
			wantSUPI: "***REDACTED***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := newTestLogger(slog.LevelInfo)
			enriched := WithSUPI(logger, tt.supi)

			enriched.Info("subscriber operation")

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
			}

			got, ok := entry["supi"].(string)
			if !ok {
				t.Fatalf("expected 'supi' field in JSON output; entry=%v", entry)
			}
			if got != tt.wantSUPI {
				t.Errorf("supi = %q, want %q", got, tt.wantSUPI)
			}
		})
	}
}
