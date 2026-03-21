package telemetry

import (
	"context"
	"sync"
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "creates provider with all fields",
			cfg: Config{
				ServiceName:    "udm-ueau",
				ServiceVersion: "1.0.0",
				OTelEndpoint:   "otel-collector:4317",
				SampleRate:     0.01,
			},
		},
		{
			name: "creates provider with zero-value config",
			cfg:  Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(tt.cfg)
			if p == nil {
				t.Fatal("NewProvider returned nil")
			}
			if p.ServiceName != tt.cfg.ServiceName {
				t.Errorf("ServiceName = %q, want %q", p.ServiceName, tt.cfg.ServiceName)
			}
			if p.ServiceVersion != tt.cfg.ServiceVersion {
				t.Errorf("ServiceVersion = %q, want %q", p.ServiceVersion, tt.cfg.ServiceVersion)
			}
			if p.OTelEndpoint != tt.cfg.OTelEndpoint {
				t.Errorf("OTelEndpoint = %q, want %q", p.OTelEndpoint, tt.cfg.OTelEndpoint)
			}
			if p.SampleRate != tt.cfg.SampleRate {
				t.Errorf("SampleRate = %f, want %f", p.SampleRate, tt.cfg.SampleRate)
			}
		})
	}
}

func TestProviderShutdown(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "no-op shutdown returns nil",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(Config{ServiceName: "udm-sdm"})
			err := p.Shutdown(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Shutdown() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCounter(t *testing.T) {
	tests := []struct {
		name        string
		counterName string
	}{
		{
			name:        "creates counter with name",
			counterName: "udm_http_requests_total",
		},
		{
			name:        "creates counter with empty name",
			counterName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCounter(tt.counterName)
			if c == nil {
				t.Fatal("NewCounter returned nil")
			}
			if c.Name != tt.counterName {
				t.Errorf("Name = %q, want %q", c.Name, tt.counterName)
			}
			if c.Labels == nil {
				t.Error("Labels map is nil, want initialized map")
			}
			if c.Value() != 0 {
				t.Errorf("initial Value() = %d, want 0", c.Value())
			}
		})
	}
}

func TestCounterInc(t *testing.T) {
	c := NewCounter("udm_auth_attempts_total")

	tests := []struct {
		name string
		want int64
	}{
		{name: "first increment", want: 1},
		{name: "second increment", want: 2},
		{name: "third increment", want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Inc()
			if got := c.Value(); got != tt.want {
				t.Errorf("Value() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCounterAdd(t *testing.T) {
	tests := []struct {
		name  string
		delta int64
		want  int64
	}{
		{name: "add 5", delta: 5, want: 5},
		{name: "add 10 more", delta: 10, want: 15},
		{name: "add 0", delta: 0, want: 15},
		{name: "add 1", delta: 1, want: 16},
	}

	c := NewCounter("udm_http_bytes_total")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Add(tt.delta)
			if got := c.Value(); got != tt.want {
				t.Errorf("Value() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCounterAddNegativeDelta(t *testing.T) {
	c := NewCounter("udm_test_counter")
	c.Add(10)
	c.Add(-5) // should be silently ignored
	if got := c.Value(); got != 10 {
		t.Errorf("Value() = %d after negative Add, want 10 (negative delta should be ignored)", got)
	}
}

func TestCounterConcurrency(t *testing.T) {
	c := NewCounter("udm_concurrent_requests_total")

	const goroutines = 100
	const incPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range incPerGoroutine {
				c.Inc()
			}
		}()
	}

	wg.Wait()

	want := int64(goroutines * incPerGoroutine)
	if got := c.Value(); got != want {
		t.Errorf("Value() after concurrent Inc = %d, want %d", got, want)
	}
}

func TestNewHistogram(t *testing.T) {
	tests := []struct {
		name          string
		histogramName string
		wantBuckets   []float64
	}{
		{
			name:          "creates histogram with default buckets",
			histogramName: "udm_http_request_duration_seconds",
			wantBuckets:   []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHistogram(tt.histogramName)
			if h == nil {
				t.Fatal("NewHistogram returned nil")
			}
			if h.Name != tt.histogramName {
				t.Errorf("Name = %q, want %q", h.Name, tt.histogramName)
			}
			if len(h.Buckets) != len(tt.wantBuckets) {
				t.Fatalf("len(Buckets) = %d, want %d", len(h.Buckets), len(tt.wantBuckets))
			}
			for i, b := range h.Buckets {
				if b != tt.wantBuckets[i] {
					t.Errorf("Buckets[%d] = %f, want %f", i, b, tt.wantBuckets[i])
				}
			}
		})
	}
}

func TestHistogramObserveNoop(t *testing.T) {
	tests := []struct {
		name  string
		value float64
	}{
		{name: "observe zero", value: 0.0},
		{name: "observe sub-millisecond", value: 0.0005},
		{name: "observe typical latency", value: 0.008},
		{name: "observe high latency", value: 2.5},
		{name: "observe negative value", value: -1.0},
	}

	h := NewHistogram("udm_http_request_duration_seconds")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Phase 1: Observe is no-op; verify it does not panic.
			h.Observe(tt.value)
		})
	}
}

func TestHistogramBucketsAreDefensiveCopy(t *testing.T) {
	h := NewHistogram("udm_test_histogram")
	// Mutating the returned buckets should not affect the default.
	h.Buckets[0] = 999.0

	h2 := NewHistogram("udm_test_histogram_2")
	if h2.Buckets[0] == 999.0 {
		t.Error("NewHistogram buckets are shared; expected defensive copy")
	}
}
