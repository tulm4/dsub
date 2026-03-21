// Package telemetry provides OpenTelemetry SDK setup and Prometheus metrics stubs.
//
// Based on: docs/service-decomposition.md §3.2 (common/telemetry)
// Based on: docs/observability.md §2 (Metrics), §4 (Distributed Tracing)
//
// Phase 1: No-op / in-memory implementations for testing and compilation.
// Real OpenTelemetry SDK and Prometheus client integration will be added in Phase 8.
package telemetry

import (
	"context"
	"sync/atomic"
)

// Config holds telemetry configuration.
// Based on: docs/observability.md §4.1 (OpenTelemetry SDK Integration)
type Config struct {
	// ServiceName identifies the microservice (e.g., "udm-ueau").
	ServiceName string
	// ServiceVersion is the semantic version of the service binary.
	ServiceVersion string
	// OTelEndpoint is the OpenTelemetry Collector gRPC endpoint (e.g., "otel-collector:4317").
	OTelEndpoint string
	// SampleRate is the head-based trace sampling ratio (0.0–1.0).
	SampleRate float64
}

// Provider manages telemetry lifecycle (tracing, metrics).
// Based on: docs/service-decomposition.md §3.2 (common/telemetry)
// Real OpenTelemetry SDK integration will be added in Phase 8.
type Provider struct {
	ServiceName    string
	ServiceVersion string
	OTelEndpoint   string
	SampleRate     float64
	shutdown       func(context.Context) error
}

// NewProvider creates a new telemetry provider with the given configuration.
// In Phase 1, this creates a no-op provider. OTel SDK will be integrated in Phase 8.
// Based on: docs/observability.md §4.1 (OpenTelemetry SDK Integration)
func NewProvider(cfg Config) *Provider {
	return &Provider{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		OTelEndpoint:   cfg.OTelEndpoint,
		SampleRate:     cfg.SampleRate,
		shutdown: func(context.Context) error {
			return nil
		},
	}
}

// Shutdown gracefully shuts down the telemetry provider, flushing any pending
// traces and metrics. Returns an error if shutdown fails.
// Based on: docs/observability.md §4.1 (TracerProvider shutdown)
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.shutdown != nil {
		return p.shutdown(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Metric types — interfaces for future OTel / Prometheus integration.
// Based on: docs/observability.md §2.2 (Metric Types)
// ---------------------------------------------------------------------------

// Counter represents a monotonically increasing counter metric.
// 3GPP metric example: udm_http_requests_total
// Based on: docs/observability.md §2.2 (Counter)
type Counter struct {
	Name   string
	Labels map[string]string
	value  atomic.Int64
}

// NewCounter creates a named counter.
// Phase 1: in-memory counter backed by sync/atomic for thread safety.
func NewCounter(name string) *Counter {
	return &Counter{
		Name:   name,
		Labels: make(map[string]string),
	}
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	c.value.Add(1)
}

// Add adds the given value to the counter. Delta must be non-negative.
func (c *Counter) Add(delta int64) {
	c.value.Add(delta)
}

// Value returns the current counter value (for testing only).
func (c *Counter) Value() int64 {
	return c.value.Load()
}

// Histogram represents a value distribution metric.
// 3GPP metric example: udm_http_request_duration_seconds
// Based on: docs/observability.md §2.2 (Histogram)
type Histogram struct {
	Name    string
	Buckets []float64
}

// defaultHistogramBuckets are telecom-grade latency buckets (in seconds) aligned
// with the carrier-grade sub-10ms p50 latency target.
// Based on: docs/observability.md §2.2 — udm_http_request_duration_seconds buckets
var defaultHistogramBuckets = []float64{
	0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5,
}

// NewHistogram creates a named histogram with telecom-grade default buckets.
// Based on: docs/observability.md §2.2 (Histogram buckets)
func NewHistogram(name string) *Histogram {
	buckets := make([]float64, len(defaultHistogramBuckets))
	copy(buckets, defaultHistogramBuckets)
	return &Histogram{
		Name:    name,
		Buckets: buckets,
	}
}

// Observe records a value in the histogram.
// Phase 1: no-op. Real implementation backed by OTel SDK in Phase 8.
func (h *Histogram) Observe(value float64) {
	// No-op in Phase 1. Will record into OTel histogram instrument in Phase 8.
}
