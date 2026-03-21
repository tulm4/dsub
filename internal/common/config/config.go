// Package config provides 12-factor environment-based configuration loading
// for 5G UDM microservices. All configuration values are read from environment
// variables with sensible defaults for development.
//
// Based on: docs/service-decomposition.md §3 (Shared Libraries)
// Based on: docs/architecture.md §10 (Technology Stack)
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the complete runtime configuration for a UDM microservice.
//
// Based on: docs/service-decomposition.md §3 (Shared Libraries)
// Based on: docs/architecture.md §4.2 (Stateless Services)
type Config struct {
	// Service identity
	ServiceName      string `json:"serviceName"`
	ServiceVersion   string `json:"serviceVersion"`
	Region           string `json:"region"`
	AvailabilityZone string `json:"availabilityZone"`

	// HTTP
	// Based on: docs/sbi-api-design.md §1 (HTTP/2 Transport)
	HTTPPort    int `json:"httpPort"`
	MetricsPort int `json:"metricsPort"`

	// Logging
	// Based on: docs/service-decomposition.md §3.2 (Structured Logging)
	LogLevel string `json:"logLevel"`

	// Database
	// Based on: docs/data-model.md §3 (YugabyteDB Schema)
	// Based on: docs/service-decomposition.md §3.3 (pgx Driver)
	DBDSN             string        `json:"dbDSN"`
	DBMaxOpenConns    int           `json:"dbMaxOpenConns"`
	DBMaxIdleConns    int           `json:"dbMaxIdleConns"`
	DBConnMaxLifetime time.Duration `json:"dbConnMaxLifetime"`

	// Cache
	// Based on: docs/service-decomposition.md §3.4 (Two-Tier Caching)
	CacheL1MaxSize int64         `json:"cacheL1MaxSize"`
	CacheL1TTL     time.Duration `json:"cacheL1TTL"`
	CacheL2TTL     time.Duration `json:"cacheL2TTL"`
	RedisAddrs     []string      `json:"redisAddrs"`
	RedisPassword  string        `json:"-"`

	// NRF
	// Based on: docs/service-decomposition.md §3.6 (NRF Client)
	NRFDiscoveryURL      string        `json:"nrfDiscoveryURL"`
	NRFHeartbeatInterval time.Duration `json:"nrfHeartbeatInterval"`

	// TLS
	// Based on: docs/security.md (mTLS Requirements)
	TLSCertFile   string `json:"tlsCertFile"`
	TLSKeyFile    string `json:"tlsKeyFile"`
	TLSCACertFile string `json:"tlsCACertFile"`

	// Telemetry
	// Based on: docs/observability.md §1 (OpenTelemetry SDK)
	OTelEndpoint   string  `json:"otelEndpoint"`
	OTelSampleRate float64 `json:"otelSampleRate"`

	// Shutdown
	ShutdownTimeout time.Duration `json:"shutdownTimeout"`
}

// Load reads configuration from environment variables with defaults.
//
// Environment variables:
//
//	UDM_SERVICE_NAME, UDM_SERVICE_VERSION, UDM_REGION, UDM_AVAILABILITY_ZONE,
//	UDM_HTTP_PORT (default: 8080), UDM_METRICS_PORT (default: 9090),
//	UDM_LOG_LEVEL (default: "info"),
//	UDM_DB_DSN, UDM_DB_MAX_OPEN_CONNS (default: 50), UDM_DB_MAX_IDLE_CONNS (default: 25),
//	UDM_DB_CONN_MAX_LIFETIME (default: "300s"),
//	UDM_CACHE_L1_MAX_SIZE (default: 500000000), UDM_CACHE_L1_TTL (default: "10s"),
//	UDM_CACHE_L2_TTL (default: "60s"), REDIS_ADDRS (comma-separated), REDIS_PASSWORD,
//	UDM_NRF_DISCOVERY_URL, UDM_NRF_HEARTBEAT_INTERVAL (default: "30s"),
//	TLS_CERT_FILE, TLS_KEY_FILE, TLS_CA_CERT_FILE,
//	OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_SAMPLER_ARG (default: "0.01"),
//	UDM_SHUTDOWN_TIMEOUT (default: "30s")
func Load() (*Config, error) {
	httpPort, err := envInt("UDM_HTTP_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_HTTP_PORT: %w", err)
	}

	metricsPort, err := envInt("UDM_METRICS_PORT", 9090)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_METRICS_PORT: %w", err)
	}

	dbMaxOpenConns, err := envInt("UDM_DB_MAX_OPEN_CONNS", 50)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_DB_MAX_OPEN_CONNS: %w", err)
	}

	dbMaxIdleConns, err := envInt("UDM_DB_MAX_IDLE_CONNS", 25)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_DB_MAX_IDLE_CONNS: %w", err)
	}

	dbConnMaxLifetime, err := envDuration("UDM_DB_CONN_MAX_LIFETIME", 300*time.Second)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_DB_CONN_MAX_LIFETIME: %w", err)
	}

	cacheL1MaxSize, err := envInt64("UDM_CACHE_L1_MAX_SIZE", 500_000_000)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_CACHE_L1_MAX_SIZE: %w", err)
	}

	cacheL1TTL, err := envDuration("UDM_CACHE_L1_TTL", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_CACHE_L1_TTL: %w", err)
	}

	cacheL2TTL, err := envDuration("UDM_CACHE_L2_TTL", 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_CACHE_L2_TTL: %w", err)
	}

	nrfHeartbeatInterval, err := envDuration("UDM_NRF_HEARTBEAT_INTERVAL", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_NRF_HEARTBEAT_INTERVAL: %w", err)
	}

	otelSampleRate, err := envFloat64("OTEL_SAMPLER_ARG", 0.01)
	if err != nil {
		return nil, fmt.Errorf("config: invalid OTEL_SAMPLER_ARG: %w", err)
	}

	shutdownTimeout, err := envDuration("UDM_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("config: invalid UDM_SHUTDOWN_TIMEOUT: %w", err)
	}

	// Parse comma-separated Redis addresses.
	var redisAddrs []string
	if raw := os.Getenv("REDIS_ADDRS"); raw != "" {
		for _, addr := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(addr); trimmed != "" {
				redisAddrs = append(redisAddrs, trimmed)
			}
		}
	}

	cfg := &Config{
		ServiceName:      envStr("UDM_SERVICE_NAME", ""),
		ServiceVersion:   envStr("UDM_SERVICE_VERSION", ""),
		Region:           envStr("UDM_REGION", ""),
		AvailabilityZone: envStr("UDM_AVAILABILITY_ZONE", ""),

		HTTPPort:    httpPort,
		MetricsPort: metricsPort,

		LogLevel: envStr("UDM_LOG_LEVEL", "info"),

		DBDSN:             envStr("UDM_DB_DSN", ""),
		DBMaxOpenConns:    dbMaxOpenConns,
		DBMaxIdleConns:    dbMaxIdleConns,
		DBConnMaxLifetime: dbConnMaxLifetime,

		CacheL1MaxSize: cacheL1MaxSize,
		CacheL1TTL:     cacheL1TTL,
		CacheL2TTL:     cacheL2TTL,
		RedisAddrs:     redisAddrs,
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),

		NRFDiscoveryURL:      envStr("UDM_NRF_DISCOVERY_URL", ""),
		NRFHeartbeatInterval: nrfHeartbeatInterval,

		TLSCertFile:   envStr("TLS_CERT_FILE", ""),
		TLSKeyFile:    envStr("TLS_KEY_FILE", ""),
		TLSCACertFile: envStr("TLS_CA_CERT_FILE", ""),

		OTelEndpoint:   envStr("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSampleRate: otelSampleRate,

		ShutdownTimeout: shutdownTimeout,
	}

	return cfg, nil
}

// Validate checks that required configuration values are set.
// Currently only ServiceName is required; other fields are optional and
// validated at the point of use (e.g., DB connection, TLS setup).
func (c *Config) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("config: ServiceName is required (set UDM_SERVICE_NAME)")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Unexported helpers for reading environment variables with defaults
// ---------------------------------------------------------------------------

func envStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parsing %q as int: %w", v, err)
	}
	return n, nil
}

func envInt64(key string, defaultVal int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %q as int64: %w", v, err)
	}
	return n, nil
}

func envDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("parsing %q as duration: %w", v, err)
	}
	return d, nil
}

func envFloat64(key string, defaultVal float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %q as float64: %w", v, err)
	}
	return f, nil
}
