package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ServiceName", cfg.ServiceName, ""},
		{"ServiceVersion", cfg.ServiceVersion, ""},
		{"Region", cfg.Region, ""},
		{"AvailabilityZone", cfg.AvailabilityZone, ""},
		{"HTTPPort", cfg.HTTPPort, 8080},
		{"MetricsPort", cfg.MetricsPort, 9090},
		{"LogLevel", cfg.LogLevel, "info"},
		{"DBDSN", cfg.DBDSN, ""},
		{"DBMaxOpenConns", cfg.DBMaxOpenConns, 50},
		{"DBMaxIdleConns", cfg.DBMaxIdleConns, 25},
		{"DBConnMaxLifetime", cfg.DBConnMaxLifetime, 300 * time.Second},
		{"CacheL1MaxSize", cfg.CacheL1MaxSize, int64(500_000_000)},
		{"CacheL1TTL", cfg.CacheL1TTL, 10 * time.Second},
		{"CacheL2TTL", cfg.CacheL2TTL, 60 * time.Second},
		{"RedisAddrs length", len(cfg.RedisAddrs), 0},
		{"RedisPassword", cfg.RedisPassword, ""},
		{"NRFDiscoveryURL", cfg.NRFDiscoveryURL, ""},
		{"NRFHeartbeatInterval", cfg.NRFHeartbeatInterval, 30 * time.Second},
		{"TLSCertFile", cfg.TLSCertFile, ""},
		{"TLSKeyFile", cfg.TLSKeyFile, ""},
		{"TLSCACertFile", cfg.TLSCACertFile, ""},
		{"OTelEndpoint", cfg.OTelEndpoint, ""},
		{"OTelSampleRate", cfg.OTelSampleRate, 0.01},
		{"ShutdownTimeout", cfg.ShutdownTimeout, 30 * time.Second},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

func TestLoad_AllEnvVars(t *testing.T) {
	envs := map[string]string{
		"UDM_SERVICE_NAME":          "udm-ueau",
		"UDM_SERVICE_VERSION":       "v1.2.3",
		"UDM_REGION":                "us-east-1",
		"UDM_AVAILABILITY_ZONE":     "us-east-1a",
		"UDM_HTTP_PORT":             "9080",
		"UDM_METRICS_PORT":          "9190",
		"UDM_LOG_LEVEL":             "debug",
		"UDM_DB_DSN":                "postgres://user:pass@host:5433/udm",
		"UDM_DB_MAX_OPEN_CONNS":     "100",
		"UDM_DB_MAX_IDLE_CONNS":     "50",
		"UDM_DB_CONN_MAX_LIFETIME":  "600s",
		"UDM_CACHE_L1_MAX_SIZE":     "1000000000",
		"UDM_CACHE_L1_TTL":          "20s",
		"UDM_CACHE_L2_TTL":          "120s",
		"REDIS_ADDRS":               "redis-0:6379,redis-1:6379,redis-2:6379",
		"REDIS_PASSWORD":            "s3cret",
		"UDM_NRF_DISCOVERY_URL":     "https://nrf.5gc.local/nnrf-disc/v1",
		"UDM_NRF_HEARTBEAT_INTERVAL": "60s",
		"TLS_CERT_FILE":             "/certs/tls.crt",
		"TLS_KEY_FILE":              "/certs/tls.key",
		"TLS_CA_CERT_FILE":          "/certs/ca.crt",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "otel-collector:4317",
		"OTEL_SAMPLER_ARG":          "0.1",
		"UDM_SHUTDOWN_TIMEOUT":      "45s",
	}
	for k, v := range envs {
		t.Setenv(k, v)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ServiceName", cfg.ServiceName, "udm-ueau"},
		{"ServiceVersion", cfg.ServiceVersion, "v1.2.3"},
		{"Region", cfg.Region, "us-east-1"},
		{"AvailabilityZone", cfg.AvailabilityZone, "us-east-1a"},
		{"HTTPPort", cfg.HTTPPort, 9080},
		{"MetricsPort", cfg.MetricsPort, 9190},
		{"LogLevel", cfg.LogLevel, "debug"},
		{"DBDSN", cfg.DBDSN, "postgres://user:pass@host:5433/udm"},
		{"DBMaxOpenConns", cfg.DBMaxOpenConns, 100},
		{"DBMaxIdleConns", cfg.DBMaxIdleConns, 50},
		{"DBConnMaxLifetime", cfg.DBConnMaxLifetime, 600 * time.Second},
		{"CacheL1MaxSize", cfg.CacheL1MaxSize, int64(1_000_000_000)},
		{"CacheL1TTL", cfg.CacheL1TTL, 20 * time.Second},
		{"CacheL2TTL", cfg.CacheL2TTL, 120 * time.Second},
		{"RedisAddrs length", len(cfg.RedisAddrs), 3},
		{"RedisPassword", cfg.RedisPassword, "s3cret"},
		{"NRFDiscoveryURL", cfg.NRFDiscoveryURL, "https://nrf.5gc.local/nnrf-disc/v1"},
		{"NRFHeartbeatInterval", cfg.NRFHeartbeatInterval, 60 * time.Second},
		{"TLSCertFile", cfg.TLSCertFile, "/certs/tls.crt"},
		{"TLSKeyFile", cfg.TLSKeyFile, "/certs/tls.key"},
		{"TLSCACertFile", cfg.TLSCACertFile, "/certs/ca.crt"},
		{"OTelEndpoint", cfg.OTelEndpoint, "otel-collector:4317"},
		{"OTelSampleRate", cfg.OTelSampleRate, 0.1},
		{"ShutdownTimeout", cfg.ShutdownTimeout, 45 * time.Second},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}

	// Verify individual Redis addresses.
	wantAddrs := []string{"redis-0:6379", "redis-1:6379", "redis-2:6379"}
	for i, want := range wantAddrs {
		if i >= len(cfg.RedisAddrs) {
			t.Errorf("RedisAddrs[%d]: missing, want %q", i, want)
		} else if cfg.RedisAddrs[i] != want {
			t.Errorf("RedisAddrs[%d] = %q, want %q", i, cfg.RedisAddrs[i], want)
		}
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		envVal string
		errMsg string
	}{
		{"InvalidHTTPPort", "UDM_HTTP_PORT", "not-a-number", "UDM_HTTP_PORT"},
		{"InvalidMetricsPort", "UDM_METRICS_PORT", "abc", "UDM_METRICS_PORT"},
		{"InvalidDBMaxOpenConns", "UDM_DB_MAX_OPEN_CONNS", "xyz", "UDM_DB_MAX_OPEN_CONNS"},
		{"InvalidDBMaxIdleConns", "UDM_DB_MAX_IDLE_CONNS", "!!", "UDM_DB_MAX_IDLE_CONNS"},
		{"InvalidCacheL1MaxSize", "UDM_CACHE_L1_MAX_SIZE", "big", "UDM_CACHE_L1_MAX_SIZE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.envVal)
			_, err := Load()
			if err == nil {
				t.Fatal("Load() should have returned an error")
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tc.errMsg)
			}
		})
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		envVal string
		errMsg string
	}{
		{"InvalidDBConnMaxLifetime", "UDM_DB_CONN_MAX_LIFETIME", "not-a-duration", "UDM_DB_CONN_MAX_LIFETIME"},
		{"InvalidCacheL1TTL", "UDM_CACHE_L1_TTL", "bad", "UDM_CACHE_L1_TTL"},
		{"InvalidCacheL2TTL", "UDM_CACHE_L2_TTL", "nope", "UDM_CACHE_L2_TTL"},
		{"InvalidNRFHeartbeat", "UDM_NRF_HEARTBEAT_INTERVAL", "??", "UDM_NRF_HEARTBEAT_INTERVAL"},
		{"InvalidShutdownTimeout", "UDM_SHUTDOWN_TIMEOUT", "never", "UDM_SHUTDOWN_TIMEOUT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.envVal)
			_, err := Load()
			if err == nil {
				t.Fatal("Load() should have returned an error")
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tc.errMsg)
			}
		})
	}
}

func TestLoad_InvalidFloat(t *testing.T) {
	t.Setenv("OTEL_SAMPLER_ARG", "not-a-float")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() should have returned an error")
	}
	if !strings.Contains(err.Error(), "OTEL_SAMPLER_ARG") {
		t.Errorf("error %q should contain OTEL_SAMPLER_ARG", err.Error())
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "ValidConfig",
			cfg:     Config{ServiceName: "udm-ueau"},
			wantErr: false,
		},
		{
			name:    "EmptyServiceName",
			cfg:     Config{},
			wantErr: true,
			errMsg:  "ServiceName",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("Validate() should have returned an error")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() returned unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRedisAddrs_Parsing(t *testing.T) {
	tests := []struct {
		name      string
		envVal    string
		wantAddrs []string
	}{
		{
			name:      "Empty",
			envVal:    "",
			wantAddrs: nil,
		},
		{
			name:      "SingleAddress",
			envVal:    "redis:6379",
			wantAddrs: []string{"redis:6379"},
		},
		{
			name:      "MultipleAddresses",
			envVal:    "redis-0:6379,redis-1:6379,redis-2:6379",
			wantAddrs: []string{"redis-0:6379", "redis-1:6379", "redis-2:6379"},
		},
		{
			name:      "WithWhitespace",
			envVal:    " redis-0:6379 , redis-1:6379 , redis-2:6379 ",
			wantAddrs: []string{"redis-0:6379", "redis-1:6379", "redis-2:6379"},
		},
		{
			name:      "TrailingComma",
			envVal:    "redis-0:6379,redis-1:6379,",
			wantAddrs: []string{"redis-0:6379", "redis-1:6379"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envVal != "" {
				t.Setenv("REDIS_ADDRS", tc.envVal)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}

			if len(cfg.RedisAddrs) != len(tc.wantAddrs) {
				t.Fatalf("RedisAddrs length = %d, want %d", len(cfg.RedisAddrs), len(tc.wantAddrs))
			}
			for i, want := range tc.wantAddrs {
				if cfg.RedisAddrs[i] != want {
					t.Errorf("RedisAddrs[%d] = %q, want %q", i, cfg.RedisAddrs[i], want)
				}
			}
		})
	}
}
