// Package logging provides structured JSON logging with PII redaction for
// the 5G UDM network function.
//
// Based on: docs/service-decomposition.md §3.2 (udm-common logging)
// Technology: Go log/slog with JSON handler, output to stdout for Fluentd/OTel collection
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/tulm4/dsub/internal/common/identifiers"
)

// NewLogger creates a new structured JSON logger with the specified level and
// service attributes. Output format is JSON to stdout for collection by
// Fluentd/OTel.
//
// Based on: docs/service-decomposition.md §3.2 (udm-common logging)
func NewLogger(level, serviceName, region string) *slog.Logger {
	lvl := ParseLevel(level)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})
	return slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("region", region),
	)
}

// ParseLevel converts a string log level to slog.Level.
// Supported: "debug", "info", "warn", "error". Returns slog.LevelInfo for unknown.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// WithTraceID adds a trace_id attribute to the logger.
//
// Based on: docs/service-decomposition.md §3.2 (udm-common logging)
func WithTraceID(logger *slog.Logger, traceID string) *slog.Logger {
	return logger.With(slog.String("trace_id", traceID))
}

// WithSUPI adds a redacted SUPI attribute to the logger.
// Uses partial redaction via identifiers.RedactSUPI: imsi-***<last6>.
//
// Based on: docs/service-decomposition.md §3.2 (udm-common logging)
// 3GPP: TS 33.501 §6.7.1 — Privacy of subscription permanent identifier
func WithSUPI(logger *slog.Logger, supi string) *slog.Logger {
	return logger.With(slog.String("supi", identifiers.RedactSUPI(supi)))
}
