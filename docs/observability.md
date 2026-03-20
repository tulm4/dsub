# UDM Observability Architecture

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

---

## Table of Contents

1. [Observability Overview](#1-observability-overview)
2. [Metrics Architecture](#2-metrics-architecture)
3. [KPI Definitions](#3-kpi-definitions)
4. [Distributed Tracing](#4-distributed-tracing)
5. [Structured Logging](#5-structured-logging)
6. [Telecom Alarm System](#6-telecom-alarm-system)
7. [Alerting Strategy for NOC/SOC](#7-alerting-strategy-for-nocsoc)
8. [OSS/BSS Integration](#8-ossbss-integration)
9. [Dashboards](#9-dashboards)

---

## 1. Observability Overview

### 1.1 Purpose

This document defines the observability architecture for the 5G Core UDM system. In a
telecom-grade deployment, observability is not optional — it is a regulatory and
operational requirement. Every subscriber-facing operation must be auditable, every
latency spike must be traceable, and every failure must generate an alarm within seconds.

The UDM observability stack is built on **four pillars**:

| Pillar | Technology | Purpose |
|--------|-----------|---------|
| **Metrics** | Prometheus + OpenTelemetry | Quantitative health, KPIs, capacity planning |
| **Tracing** | OpenTelemetry SDK + Jaeger/Tempo | Request-level flow analysis across services |
| **Logging** | Structured JSON + Loki/ELK | Event recording, audit trail, debugging |
| **Alarms** | 3GPP-aligned Fault Management | NOC/SOC notification, SNMP traps, auto-remediation triggers |

### 1.2 Design Principles

- **Zero-instrumentation overhead in the hot path.** Metrics collection uses lock-free
  counters; tracing uses sampling to bound overhead.
- **Subscriber-correlated telemetry.** Every log, trace, and metric can be filtered by
  SUPI to debug a single subscriber's experience end-to-end.
- **3GPP compliance.** Alarm severity levels, PM counters, and fault management follow
  3GPP TS 28.532 (Management Services) and TS 28.552 (Performance Measurements).
- **Multi-region awareness.** All telemetry is tagged with `region` and `availability_zone`
  labels to support Active-Active topology debugging.
- **Separation of concerns.** Application code emits telemetry via OpenTelemetry SDK;
  collection, aggregation, and storage are handled by infrastructure components.

### 1.3 Observability Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        UDM Microservices                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │ udm-ueau │ │ udm-sdm  │ │ udm-uecm │ │ udm-ee   │  ...     │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘           │
│       │ OTel SDK    │            │            │                  │
└───────┼─────────────┼────────────┼────────────┼─────────────────┘
        │             │            │            │
        ▼             ▼            ▼            ▼
┌───────────────────────────────────────────────────────────────┐
│                  OpenTelemetry Collector                       │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐              │
│  │  Metrics   │  │   Traces   │  │    Logs    │              │
│  │  Pipeline  │  │  Pipeline  │  │  Pipeline  │              │
│  └─────┬──────┘  └─────┬──────┘  └─────┬──────┘              │
└────────┼───────────────┼───────────────┼──────────────────────┘
         │               │               │
         ▼               ▼               ▼
   ┌───────────┐  ┌────────────┐  ┌───────────┐
   │Prometheus │  │Jaeger/Tempo│  │ Loki/ELK  │
   └─────┬─────┘  └────────────┘  └─────┬─────┘
         │                               │
         ▼                               ▼
   ┌───────────┐                  ┌────────────┐
   │  Grafana  │◄─────────────────│Alertmanager│
   └───────────┘                  └─────┬──────┘
                                        │
                                        ▼
                                 ┌─────────────┐
                                 │ PagerDuty / │
                                 │  OpsGenie   │
                                 └─────────────┘
```

---

## 2. Metrics Architecture

### 2.1 Collection Stack

All UDM microservices expose a `/metrics` endpoint in Prometheus exposition format.
The OpenTelemetry SDK is used as the in-process instrumentation library, with a
Prometheus exporter bridge for backward compatibility.

| Component | Role | Version |
|-----------|------|---------|
| **OpenTelemetry Go SDK** | In-process metric instrumentation | v1.28+ |
| **Prometheus client_golang** | `/metrics` endpoint exposition | v1.20+ |
| **OpenTelemetry Collector** | Metric scraping, relabeling, remote-write | v0.100+ |
| **Prometheus / Thanos** | Long-term metric storage, HA query | v2.53+ / v0.35+ |

### 2.2 Metric Types

The UDM uses three Prometheus metric types:

| Type | Use Case | Example |
|------|----------|---------|
| **Counter** | Monotonically increasing totals (requests, errors, bytes) | `udm_http_requests_total` |
| **Gauge** | Point-in-time values (pool size, active connections, queue depth) | `udm_db_connection_pool_active` |
| **Histogram** | Latency distributions with configurable buckets | `udm_http_request_duration_seconds` |

Standard histogram buckets for latency metrics:

```yaml
# Telecom-grade buckets optimized for low-latency operations
buckets: [0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5]
```

### 2.3 Common Labels

Every metric emitted by a UDM service includes the following base labels:

| Label | Description | Example |
|-------|-------------|---------|
| `service` | Microservice name | `udm-ueau` |
| `instance` | Pod name / instance ID | `udm-ueau-7b4f9c-x2k9z` |
| `region` | Deployment region | `us-east-1` |
| `az` | Availability zone | `us-east-1a` |
| `version` | Application version | `1.4.2` |

### 2.4 Per-Service Metrics

#### 2.4.1 UDM-UEAU (Authentication)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_ueau_auth_requests_total` | Counter | `auth_type`, `result` | Total authentication requests. `auth_type` ∈ {`5g-aka`, `eap-aka-prime`}. `result` ∈ {`success`, `failure`, `sync_failure`, `unknown_subscriber`}. |
| `udm_ueau_auth_latency_seconds` | Histogram | `auth_type` | End-to-end latency of authentication vector generation, including SQN management and DB access. |
| `udm_ueau_auth_vector_generations_total` | Counter | `auth_type` | Total authentication vectors generated (subset of successful requests). |
| `udm_ueau_sqn_resync_total` | Counter | — | Total SQN resynchronization operations triggered by AUTS mismatch. |

```promql
# Example: UEAU authentication success rate over 5 minutes
sum(rate(udm_ueau_auth_requests_total{result="success"}[5m]))
/
sum(rate(udm_ueau_auth_requests_total[5m]))
```

#### 2.4.2 UDM-SDM (Subscriber Data Management)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_sdm_get_requests_total` | Counter | `data_type`, `result` | Total SDM retrieval requests. `data_type` ∈ {`am-data`, `smf-select`, `sm-data`, `nssai`, `ue-context-in-smf`, `ue-context-in-smsf`, `sdm-subscriptions`, `trace-data`, `lcs-mo-data`}. `result` ∈ {`success`, `not_found`, `error`}. |
| `udm_sdm_get_latency_seconds` | Histogram | `data_type` | Latency of subscriber data retrieval by data type. |
| `udm_sdm_subscriptions_active` | Gauge | — | Number of active SDM subscriptions (Nudm_SDM_Subscribe). |
| `udm_sdm_notifications_sent_total` | Counter | `data_type`, `result` | SDM change notifications dispatched to NF consumers. |

```promql
# Example: SDM p99 latency for access-and-mobility data
histogram_quantile(0.99, rate(udm_sdm_get_latency_seconds_bucket{data_type="am-data"}[5m]))
```

#### 2.4.3 UDM-UECM (UE Context Management)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_uecm_registrations_total` | Counter | `registration_type`, `result` | Total registration operations. `registration_type` ∈ {`amf-3gpp`, `amf-non3gpp`, `smf`, `smsf-3gpp`, `smsf-non3gpp`}. `result` ∈ {`success`, `failure`, `conflict`}. |
| `udm_uecm_registration_latency_seconds` | Histogram | `registration_type` | End-to-end registration latency including DB write and deregistration notification. |
| `udm_uecm_deregistrations_total` | Counter | `registration_type`, `reason` | Total deregistration events by reason (`explicit`, `implicit`, `purge`). |
| `udm_uecm_active_registrations` | Gauge | `registration_type` | Current number of active UE registrations by type. |

#### 2.4.4 Additional Service Metrics

Each of the remaining services (`udm-ee`, `udm-pp`, `udm-mt`, `udm-ssau`,
`udm-niddau`, `udm-rsds`, `udm-ueid`) follows the same pattern:

| Service | Key Counter | Key Histogram |
|---------|-------------|---------------|
| **udm-ee** | `udm_ee_subscriptions_total{event_type, result}` | `udm_ee_notification_latency_seconds` |
| **udm-pp** | `udm_pp_updates_total{data_type, result}` | `udm_pp_update_latency_seconds` |
| **udm-mt** | `udm_mt_requests_total{operation, result}` | `udm_mt_request_latency_seconds` |
| **udm-ssau** | `udm_ssau_auth_requests_total{result}` | `udm_ssau_auth_latency_seconds` |
| **udm-niddau** | `udm_niddau_auth_requests_total{result}` | `udm_niddau_auth_latency_seconds` |
| **udm-rsds** | `udm_rsds_sds_requests_total{result}` | `udm_rsds_sds_latency_seconds` |
| **udm-ueid** | `udm_ueid_requests_total{operation, result}` | `udm_ueid_request_latency_seconds` |

### 2.5 Cross-Cutting Metrics

#### 2.5.1 HTTP Server Metrics (all services)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_http_requests_total` | Counter | `service`, `method`, `path`, `status_code` | Total HTTP requests received by each service. |
| `udm_http_request_duration_seconds` | Histogram | `service`, `method`, `path` | End-to-end HTTP request duration. |
| `udm_http_request_size_bytes` | Histogram | `service`, `method` | Request payload size in bytes. |
| `udm_http_response_size_bytes` | Histogram | `service`, `method` | Response payload size in bytes. |
| `udm_http_active_requests` | Gauge | `service` | Number of in-flight HTTP requests. |

#### 2.5.2 Database Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_db_query_duration_seconds` | Histogram | `table`, `operation` | Query execution time. `operation` ∈ {`select`, `insert`, `update`, `delete`, `batch`}. |
| `udm_db_connection_pool_active` | Gauge | `service` | Number of active connections in the pgx connection pool. |
| `udm_db_connection_pool_idle` | Gauge | `service` | Number of idle connections in the pool. |
| `udm_db_connection_pool_max` | Gauge | `service` | Maximum pool size (configuration value). |
| `udm_db_query_errors_total` | Counter | `table`, `operation`, `error_type` | Database query errors. `error_type` ∈ {`timeout`, `connection_refused`, `constraint_violation`, `serialization_failure`}. |
| `udm_db_transaction_duration_seconds` | Histogram | `service`, `operation` | Duration of database transactions (begin to commit/rollback). |

#### 2.5.3 Cache Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `udm_cache_hit_ratio` | Gauge | `cache_name` | Cache hit ratio (0.0–1.0) over the last measurement window. `cache_name` ∈ {`auth_data`, `subscription_data`, `serving_nf`}. |
| `udm_cache_hits_total` | Counter | `cache_name` | Total cache hits. |
| `udm_cache_misses_total` | Counter | `cache_name` | Total cache misses. |
| `udm_cache_evictions_total` | Counter | `cache_name` | Total cache evictions. |
| `udm_cache_size_bytes` | Gauge | `cache_name` | Current cache memory usage. |

### 2.6 Infrastructure Metrics

Infrastructure metrics are collected by the Prometheus node exporter and
kube-state-metrics. Key metrics monitored:

| Category | Metric | Alert Threshold |
|----------|--------|-----------------|
| **CPU** | `container_cpu_usage_seconds_total` | >80% sustained for 5m |
| **Memory** | `container_memory_working_set_bytes` | >85% of limit |
| **Network** | `container_network_receive_bytes_total` | Anomaly detection |
| **Disk** | `kubelet_volume_stats_used_bytes` | >75% capacity |
| **Pod** | `kube_pod_status_ready` | Any pod not ready >30s |
| **HPA** | `kube_horizontalpodautoscaler_status_current_replicas` | At max replicas >5m |

### 2.7 YugabyteDB-Specific Metrics

YugabyteDB exposes metrics on its own `/prometheus-metrics` endpoint. The following
are critical for UDM operations:

| Metric | Description | Alert Condition |
|--------|-------------|-----------------|
| `tablet_split_count` | Number of tablet splits in progress | >0 sustained (monitor for impact) |
| `async_replication_lag_micros` | Cross-region xCluster replication lag | >500ms |
| `rocksdb_current_version_sst_files_size` | LSM tree SST file total size | Rapid growth |
| `rocksdb_bloom_filter_useful` | Bloom filter hit count | Low ratio indicates scan-heavy workload |
| `tserver_rpc_queue_length` | TServer RPC queue depth | >100 |
| `handler_latency_yb_tserver_TabletServerService_Read` | Tablet read latency | p99 >10ms |
| `handler_latency_yb_tserver_TabletServerService_Write` | Tablet write latency | p99 >20ms |
| `node_up` | YB node availability | 0 (node down) |

---

## 3. KPI Definitions

### 3.1 Telecom-Grade KPIs

The following KPIs are contractual obligations tied to SLA guarantees. They are
computed from raw metrics and reported to the OSS layer at 15-minute granularity
per 3GPP TS 28.552.

#### 3.1.1 Registration Success Rate

| Property | Value |
|----------|-------|
| **Formula** | `sum(udm_uecm_registrations_total{result="success"}) / sum(udm_uecm_registrations_total)` |
| **Target** | ≥ 99.99% |
| **Measurement Window** | 15 minutes (rolling) |
| **Alarm Trigger** | < 99.95% triggers Major alarm; < 99.9% triggers Critical alarm |
| **Scope** | Per region and aggregate |

#### 3.1.2 Authentication Latency

| Percentile | Target | Alarm Threshold |
|------------|--------|-----------------|
| **p50** | < 10 ms | > 15 ms (Warning) |
| **p95** | < 25 ms | > 40 ms (Minor) |
| **p99** | < 50 ms | > 75 ms (Major) |

```promql
# p95 authentication latency
histogram_quantile(0.95, sum(rate(udm_ueau_auth_latency_seconds_bucket[5m])) by (le))
```

#### 3.1.3 Database Query Latency

| Percentile | Target | Alarm Threshold |
|------------|--------|-----------------|
| **p50** | < 2 ms | > 5 ms (Warning) |
| **p95** | < 8 ms | > 15 ms (Minor) |
| **p99** | < 20 ms | > 40 ms (Major) |

#### 3.1.4 Error Rate per API

| Property | Value |
|----------|-------|
| **Formula** | `sum(rate(udm_http_requests_total{status_code=~"5.."}[5m])) / sum(rate(udm_http_requests_total[5m]))` |
| **Target** | < 0.01% (one in ten thousand) |
| **Alarm Trigger** | > 0.05% triggers Minor; > 0.1% triggers Major |
| **Scope** | Per service, per API path |

#### 3.1.5 Throughput (TPS) per Service

| Service | Expected Steady-State TPS | Peak TPS (2×) | Capacity Alarm |
|---------|--------------------------|---------------|----------------|
| **udm-ueau** | 50,000 | 100,000 | >80% peak capacity |
| **udm-sdm** | 80,000 | 160,000 | >80% peak capacity |
| **udm-uecm** | 40,000 | 80,000 | >80% peak capacity |
| **udm-ee** | 10,000 | 20,000 | >80% peak capacity |
| **udm-pp** | 5,000 | 10,000 | >80% peak capacity |
| **Others** | 2,000 | 4,000 | >80% peak capacity |

#### 3.1.6 Subscriber Data Retrieval Latency

| Property | Value |
|----------|-------|
| **Formula** | `histogram_quantile(0.99, rate(udm_sdm_get_latency_seconds_bucket[5m]))` |
| **Target (p99)** | < 30 ms |
| **Alarm Trigger** | > 50 ms (Major) |
| **Scope** | Per `data_type` label |

#### 3.1.7 Event Notification Delivery Latency

| Property | Value |
|----------|-------|
| **Formula** | `histogram_quantile(0.99, rate(udm_ee_notification_latency_seconds_bucket[5m]))` |
| **Target (p99)** | < 100 ms |
| **Alarm Trigger** | > 200 ms (Major) |
| **Note** | Includes HTTP callback delivery time to NF consumer |

#### 3.1.8 Cross-Region Replication Lag

| Property | Value |
|----------|-------|
| **Source Metric** | `async_replication_lag_micros` (YugabyteDB xCluster) |
| **Target** | < 100 ms (p99) |
| **Alarm Trigger** | > 250 ms (Minor); > 500 ms (Major); > 1000 ms (Critical) |
| **Impact** | Stale reads in the secondary region during failover |

---

## 4. Distributed Tracing

### 4.1 OpenTelemetry SDK Integration

Every UDM Go microservice initializes the OpenTelemetry SDK at startup:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func initTracer(serviceName, version string) (*sdktrace.TracerProvider, error) {
    exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("otel-collector:4317"),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String(serviceName),
            semconv.ServiceVersionKey.String(version),
        )),
        sdktrace.WithSampler(sdktrace.ParentBased(
            sdktrace.TraceIDRatioBased(0.01), // 1% head-based sampling
        )),
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

### 4.2 Trace Propagation

All inter-service HTTP calls propagate trace context using the
**W3C TraceContext** standard (`traceparent` / `tracestate` headers):

```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
tracestate: udm=region:us-east-1
```

Propagation is injected automatically via the `otelhttp` middleware:

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

// Wrap HTTP handler
handler := otelhttp.NewHandler(mux, "udm-uecm")

// Wrap HTTP client for outbound calls
client := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport),
}
```

### 4.3 DB-Level Tracing

Database queries are traced by wrapping the `pgx` driver with OpenTelemetry
instrumentation. Each SQL statement creates a child span:

```go
import "github.com/exaring/otelpgx"

cfg, _ := pgxpool.ParseConfig(connString)
cfg.ConnConfig.Tracer = otelpgx.NewTracer()

pool, _ := pgxpool.NewWithConfig(ctx, cfg)
```

Resulting span attributes:

| Attribute | Example |
|-----------|---------|
| `db.system` | `yugabytedb` |
| `db.statement` | `SELECT auth_data FROM authentication_data WHERE supi = $1` |
| `db.operation` | `SELECT` |
| `db.sql.table` | `authentication_data` |
| `db.yugabyte.tablet_id` | `a1b2c3d4...` |

### 4.4 Span Naming Conventions

Spans follow a hierarchical naming convention:

| Span Type | Format | Example |
|-----------|--------|---------|
| **HTTP Server** | `HTTP {METHOD} {path_template}` | `HTTP POST /nudm-ueau/v1/{supi}/security-information/generate-auth-data` |
| **HTTP Client** | `HTTP {METHOD} {target_service}` | `HTTP POST udm-ee` |
| **DB Query** | `DB {operation} {table}` | `DB SELECT authentication_data` |
| **DB Transaction** | `DB TX {description}` | `DB TX register-amf-context` |
| **Cache** | `CACHE {operation} {cache_name}` | `CACHE GET auth_data` |
| **Business Logic** | `{service}.{operation}` | `uecm.RegisterAMF` |

### 4.5 Sampling Strategy

A two-stage sampling strategy balances cost with debuggability:

| Stage | Strategy | Configuration |
|-------|----------|---------------|
| **Head-based** | `TraceIDRatioBased` at service entry point | 1% of all traces in steady state |
| **Tail-based** | OpenTelemetry Collector `tail_sampling` processor | 100% of error traces, 100% of slow traces (>p99), 100% of traces matching specific SUPIs |

```yaml
# OpenTelemetry Collector tail-sampling configuration
processors:
  tail_sampling:
    decision_wait: 10s
    policies:
      - name: errors
        type: status_code
        status_code:
          status_codes: [ERROR]
      - name: slow-requests
        type: latency
        latency:
          threshold_ms: 100
      - name: debug-subscriber
        type: string_attribute
        string_attribute:
          key: subscriber.supi
          values: ["imsi-001010000000001"]  # dynamically configured
```

### 4.6 Trace Backend

| Component | Role |
|-----------|------|
| **Jaeger** | Real-time trace visualization, dependency graph, comparison |
| **Grafana Tempo** | Long-term trace storage with object storage backend (S3/GCS) |

Traces are retained for **30 days** in Tempo and **7 days** in Jaeger for
real-time access.

### 4.7 Example Trace: AMF Registration Flow

The following shows the span hierarchy for a 3GPP Registration procedure
(AMF → UDM-UECM → DB):

```
Trace ID: 4bf92f3577b34da6a3ce929d0e0e4736

[  0ms ─────────────────────────────────── 18ms ]  HTTP POST /nudm-uecm/v1/{supi}/registrations/amf-3gpp-access
  │
  ├── [  1ms ─── 3ms ]  CACHE GET serving_nf                        (cache miss)
  │
  ├── [  3ms ──────── 8ms ]  uecm.RegisterAMF                       (business logic)
  │   │
  │   └── [  4ms ── 7ms ]  DB TX register-amf-context
  │       │
  │       ├── [  4ms ─ 5ms ]  DB SELECT amf_registrations            (check existing)
  │       │
  │       └── [  5ms ─ 7ms ]  DB INSERT amf_registrations            (write new)
  │
  ├── [  8ms ────── 14ms ]  HTTP POST udm-ee (notify registration)  (async notification)
  │
  └── [ 14ms ── 17ms ]  uecm.BuildResponse                         (serialize response)

Total duration: 18ms | Spans: 8 | Status: OK
```

---

## 5. Structured Logging

### 5.1 Log Format

All UDM services emit logs in **JSON format** to stdout. A sidecar or DaemonSet
log agent (Fluent Bit / Promtail) ships logs to the aggregation layer.

#### 5.1.1 Mandatory Fields

Every log entry MUST include:

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string (RFC 3339 Nano) | Log event time in UTC |
| `level` | string | Log severity: `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL` |
| `service` | string | Microservice name (e.g., `udm-ueau`) |
| `trace_id` | string | OpenTelemetry trace ID (32 hex chars) |
| `span_id` | string | OpenTelemetry span ID (16 hex chars) |
| `supi` | string | Subscriber permanent identifier for correlation (if applicable) |
| `request_id` | string | Unique request identifier (X-Request-ID header) |
| `method` | string | HTTP method |
| `path` | string | HTTP request path |
| `status_code` | int | HTTP response status code |
| `latency_ms` | float | Request processing time in milliseconds |
| `caller` | string | Source file and line number |
| `msg` | string | Human-readable log message |

### 5.2 Log Levels

| Level | Usage | Example |
|-------|-------|---------|
| **DEBUG** | Detailed diagnostic data; disabled in production by default | Query parameters, cache lookup details |
| **INFO** | Normal operations, request completion, state transitions | Registration completed, subscription created |
| **WARN** | Recoverable anomalies that may indicate degradation | High cache miss rate, retry triggered, slow query |
| **ERROR** | Operation failures that affect a single request | DB connection timeout, upstream NF unreachable |
| **FATAL** | Unrecoverable errors requiring process restart | Unable to bind port, configuration invalid |

### 5.3 Example Log Entries

#### Successful Authentication Request (INFO)

```json
{
  "timestamp": "2025-03-15T14:22:33.456789012Z",
  "level": "INFO",
  "service": "udm-ueau",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "supi": "imsi-001010000012345",
  "request_id": "req-8a4b2c1d-e5f6-7890-abcd-ef1234567890",
  "method": "POST",
  "path": "/nudm-ueau/v1/imsi-001010000012345/security-information/generate-auth-data",
  "status_code": 200,
  "latency_ms": 8.42,
  "caller": "handler/ueau.go:127",
  "msg": "Authentication vector generated",
  "auth_type": "5g-aka",
  "region": "us-east-1"
}
```

#### Database Connection Error (ERROR)

```json
{
  "timestamp": "2025-03-15T14:22:34.789012345Z",
  "level": "ERROR",
  "service": "udm-uecm",
  "trace_id": "9de91a7c8b2e4f01aabc123456789def",
  "span_id": "1a2b3c4d5e6f7890",
  "supi": "imsi-001010000067890",
  "request_id": "req-1f2e3d4c-b5a6-7890-cdef-ab1234567890",
  "method": "PUT",
  "path": "/nudm-uecm/v1/imsi-001010000067890/registrations/amf-3gpp-access",
  "status_code": 500,
  "latency_ms": 5002.31,
  "caller": "db/pool.go:89",
  "msg": "Database query timeout",
  "error": "context deadline exceeded",
  "db_operation": "INSERT",
  "db_table": "amf_registrations",
  "retry_count": 3,
  "region": "us-east-1"
}
```

#### Slow Query Warning (WARN)

```json
{
  "timestamp": "2025-03-15T14:22:35.123456789Z",
  "level": "WARN",
  "service": "udm-sdm",
  "trace_id": "abcdef1234567890abcdef1234567890",
  "span_id": "fedcba0987654321",
  "supi": "imsi-001010000099999",
  "request_id": "req-aa11bb22-cc33-dd44-ee55-ff6677889900",
  "method": "GET",
  "path": "/nudm-sdm/v2/imsi-001010000099999/am-data",
  "status_code": 200,
  "latency_ms": 42.87,
  "caller": "db/query.go:204",
  "msg": "Slow database query detected",
  "db_operation": "SELECT",
  "db_table": "access_and_mobility_subscription_data",
  "query_duration_ms": 38.12,
  "threshold_ms": 20.0,
  "region": "us-east-1"
}
```

### 5.4 Per-Subscriber Trace Capability

Operators can enable DEBUG-level logging for a specific subscriber by setting a
dynamic configuration flag:

```yaml
# Runtime configuration (ConfigMap or API call)
debug_subscribers:
  - supi: "imsi-001010000012345"
    log_level: DEBUG
    expiry: "2025-03-15T15:00:00Z"
```

When enabled, all operations for the specified SUPI are logged at DEBUG level
regardless of the global log level. The `expiry` field ensures debug logging is
automatically disabled to prevent log volume explosion.

### 5.5 Log Aggregation

| Component | Role |
|-----------|------|
| **Fluent Bit** (DaemonSet) | Lightweight log collector on each node; parses JSON, adds k8s metadata |
| **Grafana Loki** | Primary log aggregation and query engine (LogQL) |
| **Elasticsearch** (optional) | Full-text search for compliance and audit queries |

### 5.6 Log Retention Policy

| Environment | Retention | Storage Tier |
|-------------|-----------|-------------|
| Production | 90 days (hot) + 1 year (cold/S3) | SSD → Object Storage |
| Staging | 30 days | SSD |
| Development | 7 days | Local disk |

Logs containing SUPI or other PII are subject to GDPR retention rules and may
require earlier purging in EU deployments.

---

## 6. Telecom Alarm System

### 6.1 Overview

The UDM alarm system is aligned with **3GPP TS 28.532** (Management Services) and
follows the **ITU-T X.733** alarm model. Alarms represent conditions that require
operator attention, as distinct from log events or metric thresholds.

### 6.2 Alarm Severity Levels

| Severity | Color | Description | Response Time |
|----------|-------|-------------|---------------|
| **Critical** | 🔴 Red | Service-affecting condition requiring immediate action | < 5 minutes |
| **Major** | 🟠 Orange | Significant degradation; potential service impact | < 15 minutes |
| **Minor** | 🟡 Yellow | Non-service-affecting anomaly; trending toward degradation | < 1 hour |
| **Warning** | 🔵 Blue | Informational; preventive maintenance recommended | Next business day |
| **Clear** | 🟢 Green | Previously raised alarm condition has been resolved | Automatic |

### 6.3 Alarm Catalog

#### 6.3.1 Critical Alarms

| Alarm ID | Name | Condition | Probable Cause | Auto-Clear Rule |
|----------|------|-----------|---------------|-----------------|
| `UDM-C-001` | DB Connection Failure | All database connections in the pool are exhausted or YugabyteDB cluster is unreachable | `udm_db_connection_pool_active == udm_db_connection_pool_max` AND `udm_db_query_errors_total{error_type="connection_refused"}` increasing | Clear when pool has ≥1 idle connection for 60s |
| `UDM-C-002` | Service Complete Outage | All pods of a microservice are in CrashLoopBackOff or not ready | `kube_deployment_status_available_replicas == 0` | Clear when ≥1 replica is ready |
| `UDM-C-003` | Registration Success Rate Critical | Registration success rate drops below 99.9% | `rate(udm_uecm_registrations_total{result!="success"}[5m]) / rate(udm_uecm_registrations_total[5m]) > 0.001` | Clear when rate returns above 99.95% for 5m |

#### 6.3.2 Major Alarms

| Alarm ID | Name | Condition | Probable Cause | Auto-Clear Rule |
|----------|------|-----------|---------------|-----------------|
| `UDM-M-001` | Auth Failure Rate Threshold Exceeded | Authentication failure rate >0.1% over 5 minutes | `rate(udm_ueau_auth_requests_total{result="failure"}[5m]) / rate(udm_ueau_auth_requests_total[5m]) > 0.001` | Clear when failure rate <0.05% for 10m |
| `UDM-M-002` | Replication Lag Exceeded | Cross-region xCluster replication lag >500ms | `async_replication_lag_micros > 500000` | Clear when lag <250ms for 5m |
| `UDM-M-003` | Service Instance Unhealthy | One or more pods of a service are not ready but service is not fully down | `kube_deployment_status_available_replicas < kube_deployment_spec_replicas` | Clear when all replicas are ready |
| `UDM-M-004` | High Error Rate | HTTP 5xx error rate >0.1% on any service | `rate(udm_http_requests_total{status_code=~"5.."}[5m])` exceeds threshold | Clear when error rate <0.05% for 10m |

#### 6.3.3 Minor Alarms

| Alarm ID | Name | Condition | Probable Cause | Auto-Clear Rule |
|----------|------|-----------|---------------|-----------------|
| `UDM-m-001` | Cache Miss Rate High | Cache hit ratio drops below 80% | `udm_cache_hit_ratio{cache_name="subscription_data"} < 0.8` | Clear when hit ratio >85% for 15m |
| `UDM-m-002` | Latency Degradation | p95 latency exceeds 2× baseline | Histogram quantile exceeds dynamic threshold | Clear when p95 returns to within 1.5× baseline |
| `UDM-m-003` | Connection Pool Saturation | DB pool utilization >80% | `udm_db_connection_pool_active / udm_db_connection_pool_max > 0.8` | Clear when utilization <60% for 5m |

#### 6.3.4 Warning Alarms

| Alarm ID | Name | Condition | Probable Cause | Auto-Clear Rule |
|----------|------|-----------|---------------|-----------------|
| `UDM-W-001` | Certificate Expiry Approaching | TLS certificate expires within 30 days | Certificate monitor check | Clear when certificate is renewed |
| `UDM-W-002` | Disk Usage High | Persistent volume usage >75% | `kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes > 0.75` | Clear when usage <60% |
| `UDM-W-003` | HPA at Maximum Replicas | HorizontalPodAutoscaler is at max replicas | `kube_horizontalpodautoscaler_status_current_replicas == kube_horizontalpodautoscaler_spec_max_replicas` | Clear when replicas scale down below max |

### 6.4 SNMP Trap Integration

For legacy OSS/NMS systems, alarms are forwarded as SNMP v2c/v3 traps:

| Component | Description |
|-----------|-------------|
| **SNMP Exporter** | Converts Alertmanager webhook notifications to SNMP traps |
| **MIB** | Custom UDM MIB defining alarm OIDs under enterprise subtree |
| **Trap Destination** | Configurable list of NMS receivers |
| **Transport** | UDP/162 (v2c) or UDP/162 with USM (v3) |

Trap payload includes: alarm ID, severity, timestamp, affected resource,
probable cause, and proposed repair action.

### 6.5 Alarm Correlation and Deduplication

| Rule | Description |
|------|-------------|
| **Deduplication** | Identical alarms (same ID + same resource) within a 5-minute window are merged; the count is incremented. |
| **Correlation** | Child alarms are suppressed when a parent alarm is active (e.g., DB connection failure suppresses individual query timeout alarms). |
| **Flap Detection** | If an alarm clears and re-fires >3 times in 15 minutes, it is held in a "flapping" state and escalated for investigation. |
| **Grouping** | Alarms from the same service or region are grouped in a single notification to avoid alert fatigue. |

### 6.6 Auto-Clear Rules

Every alarm has an associated auto-clear rule. When the triggering condition is no
longer met for the specified duration (hysteresis), the alarm transitions to
**Clear** severity automatically. This prevents operator intervention for transient
conditions.

---

## 7. Alerting Strategy for NOC/SOC

### 7.1 Alertmanager Configuration

Prometheus Alertmanager routes alerts based on severity and service ownership:

```yaml
# Alertmanager routing configuration
route:
  group_by: ['alertname', 'service', 'region']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'default-noc'
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty-critical'
      repeat_interval: 15m
      continue: true
    - match:
        severity: major
      receiver: 'pagerduty-major'
      repeat_interval: 1h
    - match:
        severity: minor
      receiver: 'opsgenie-minor'
      repeat_interval: 4h
    - match:
        severity: warning
      receiver: 'slack-warning'
      repeat_interval: 24h
```

### 7.2 Alert Rule Examples

```yaml
groups:
  - name: udm-critical
    rules:
      - alert: UDMDatabaseUnreachable
        expr: |
          sum by (service) (udm_db_connection_pool_active)
          ==
          sum by (service) (udm_db_connection_pool_max)
          and
          rate(udm_db_query_errors_total{error_type="connection_refused"}[2m]) > 0
        for: 1m
        labels:
          severity: critical
          alarm_id: UDM-C-001
        annotations:
          summary: "Database unreachable for {{ $labels.service }}"
          description: "All DB connections exhausted and connection errors detected."
          runbook_url: "https://runbooks.internal/udm/UDM-C-001"

      - alert: UDMAuthFailureRateHigh
        expr: |
          (
            sum(rate(udm_ueau_auth_requests_total{result="failure"}[5m]))
            /
            sum(rate(udm_ueau_auth_requests_total[5m]))
          ) > 0.001
        for: 5m
        labels:
          severity: major
          alarm_id: UDM-M-001
        annotations:
          summary: "Authentication failure rate exceeds 0.1%"
          description: "Current failure rate: {{ $value | humanizePercentage }}"
          runbook_url: "https://runbooks.internal/udm/UDM-M-001"

      - alert: UDMReplicationLagHigh
        expr: async_replication_lag_micros > 500000
        for: 2m
        labels:
          severity: major
          alarm_id: UDM-M-002
        annotations:
          summary: "Cross-region replication lag exceeds 500ms"
          description: "Current lag: {{ $value | humanizeDuration }}"
          runbook_url: "https://runbooks.internal/udm/UDM-M-002"
```

### 7.3 PagerDuty / OpsGenie Integration

| Severity | Integration | On-Call Team | Response SLA |
|----------|-------------|-------------|-------------|
| **Critical** | PagerDuty (high-urgency) | UDM Platform On-Call | 5 minutes acknowledge, 15 minutes engage |
| **Major** | PagerDuty (low-urgency) | UDM Platform On-Call | 15 minutes acknowledge, 1 hour engage |
| **Minor** | OpsGenie (P3) | UDM SRE Team | 1 hour acknowledge, next business day resolve |
| **Warning** | Slack #udm-alerts | UDM SRE Team | Best effort |

### 7.4 Escalation Policies

```
Level 1 (0 min):     UDM On-Call Engineer
Level 2 (15 min):    UDM Tech Lead
Level 3 (30 min):    Platform Engineering Manager
Level 4 (60 min):    VP Engineering + Incident Commander
```

For Critical alarms, a **bridge call** is automatically initiated if Level 2
escalation is triggered.

### 7.5 Runbook Links

Every alert includes a `runbook_url` annotation pointing to a structured runbook:

| Section | Content |
|---------|---------|
| **Symptoms** | What the operator will observe |
| **Impact** | Which subscribers/services are affected |
| **Diagnosis** | Step-by-step investigation commands (kubectl, PromQL, LogQL) |
| **Remediation** | Immediate actions to restore service |
| **Root Cause Analysis** | Common root causes and permanent fixes |
| **Escalation** | When and how to escalate |

### 7.6 Dashboard Design (Grafana)

See [Section 9: Dashboards](#9-dashboards) for detailed Grafana dashboard specifications.

---

## 8. OSS/BSS Integration

### 8.1 Northbound Interface

The UDM exposes a northbound management interface for telco OSS systems, providing
standardized access to performance data, fault notifications, and configuration
management.

| Interface | Protocol | Standard | Purpose |
|-----------|----------|----------|---------|
| **Performance Management** | REST / File-based (XML/CSV) | 3GPP TS 28.532 | PM counter export at 15-min granularity |
| **Fault Management** | SNMP v3 / VES (ONAP) | ITU-T X.733 / ONAP VES 7.2 | Alarm notification to NMS |
| **Configuration Management** | NETCONF/YANG or REST | 3GPP TS 28.532 | NF configuration and provisioning |
| **Accounting** | CDR files / Kafka | 3GPP TS 32.298 | Billing event generation |

### 8.2 FCAPS Model Alignment

The UDM observability architecture maps to the ISO FCAPS model:

| FCAPS Domain | UDM Implementation |
|--------------|-------------------|
| **Fault** | Alarm system (§6), Alertmanager (§7), SNMP traps |
| **Configuration** | Kubernetes ConfigMaps, Helm values, GitOps (Argo CD) |
| **Accounting** | CDR generation for authentication and registration events |
| **Performance** | Prometheus metrics (§2), KPIs (§3), PM counters |
| **Security** | TLS certificate monitoring, auth failure tracking, audit logs |

### 8.3 CDR/Billing Event Generation

For operators requiring billing integration, the UDM can generate CDR (Call Detail
Record) events for key operations:

| Event Type | Trigger | CDR Fields |
|------------|---------|------------|
| **Authentication** | Successful 5G-AKA completion | SUPI, timestamp, auth_type, serving_network, AMF_ID |
| **Registration** | AMF/SMF registration | SUPI, timestamp, registration_type, NF_ID, PLMN |
| **Data Retrieval** | SDM data access | SUPI, timestamp, data_type, requester_NF |

CDR events are published to a Kafka topic (`udm.cdr.events`) for downstream
processing by BSS billing systems.

### 8.4 Performance Management Reporting

PM counters are aggregated at 15-minute intervals per 3GPP TS 28.552 and exported
in the following formats:

| Format | Transport | Consumer |
|--------|-----------|----------|
| 3GPP XML (measCollec) | SFTP push | Legacy OSS/PM systems |
| JSON | REST API pull | Modern OSS, ONAP DCAE |
| Prometheus remote-write | HTTP | Cloud-native monitoring stacks |

Key PM counters reported:

| Counter Group | Counters |
|---------------|----------|
| **Registration** | Attempted, Successful, Failed (by cause), Mean Duration |
| **Authentication** | Attempted, Successful, Failed (by cause), Mean Latency |
| **Data Management** | Get Requests, Subscribe Requests, Notify Sent, Errors |
| **System** | CPU Utilization, Memory Utilization, Active Connections |

---

## 9. Dashboards

### 9.1 Executive Dashboard — Overall Health

**Purpose:** Single-pane-of-glass for NOC operators and management. Shows aggregate
health across all regions and services.

| Panel | Visualization | Query |
|-------|--------------|-------|
| **Overall Health Score** | Stat (Green/Yellow/Red) | Composite of registration success rate, error rate, latency |
| **Active Subscribers** | Stat | `sum(udm_uecm_active_registrations)` |
| **Requests per Second** | Time series | `sum(rate(udm_http_requests_total[1m]))` |
| **Error Rate** | Gauge | `sum(rate(udm_http_requests_total{status_code=~"5.."}[5m])) / sum(rate(udm_http_requests_total[5m]))` |
| **p99 Latency** | Time series | `histogram_quantile(0.99, sum(rate(udm_http_request_duration_seconds_bucket[5m])) by (le))` |
| **Active Alarms** | Table | Alertmanager API — grouped by severity |
| **Regional Status** | Status map | Per-region health indicators |
| **Replication Lag** | Stat per region pair | `async_replication_lag_micros` |

### 9.2 Per-Service Operational Dashboard

**Purpose:** Deep-dive into a specific microservice. One dashboard template
parameterized by `$service` variable.

| Panel | Visualization | Description |
|-------|--------------|-------------|
| **Request Rate** | Time series | `rate(udm_http_requests_total{service="$service"}[1m])` by status code |
| **Latency Percentiles** | Time series (p50/p95/p99) | `histogram_quantile({0.5, 0.95, 0.99}, ...)` |
| **Error Rate** | Time series | 5xx / total ratio |
| **Active Requests** | Gauge | `udm_http_active_requests{service="$service"}` |
| **Pod Status** | Table | Pod name, status, restarts, age |
| **CPU / Memory** | Time series | `container_cpu_usage_seconds_total`, `container_memory_working_set_bytes` |
| **Service-Specific KPIs** | Stat panels | Auth success rate (ueau), registration rate (uecm), etc. |
| **Recent Errors** | Log panel (Loki) | `{service="$service"} |= "ERROR"` |

### 9.3 Database Performance Dashboard

**Purpose:** YugabyteDB health and query performance.

| Panel | Visualization | Description |
|-------|--------------|-------------|
| **Query Latency** | Heatmap | `udm_db_query_duration_seconds_bucket` by operation |
| **Queries per Second** | Time series | `rate(udm_db_query_duration_seconds_count[1m])` by table |
| **Connection Pool** | Gauge + time series | Active / idle / max connections per service |
| **Query Errors** | Time series | `rate(udm_db_query_errors_total[5m])` by error type |
| **Tablet Split Activity** | Time series | `tablet_split_count` |
| **Replication Lag** | Time series | `async_replication_lag_micros` per region pair |
| **LSM Compaction** | Time series | `rocksdb_current_version_sst_files_size` |
| **TServer RPC Queue** | Time series | `tserver_rpc_queue_length` |
| **Slow Queries** | Log panel | `{service=~"udm-.*"} | json | db_operation != "" | query_duration_ms > 20` |

### 9.4 Security and Auth Monitoring Dashboard

**Purpose:** Authentication monitoring, threat detection, and compliance.

| Panel | Visualization | Description |
|-------|--------------|-------------|
| **Auth Success Rate** | Stat (with thresholds) | `sum(rate(udm_ueau_auth_requests_total{result="success"}[5m])) / sum(rate(udm_ueau_auth_requests_total[5m]))` |
| **Auth Failures by Type** | Pie chart | Breakdown by `result` label |
| **Auth Latency** | Time series (p50/p95/p99) | `histogram_quantile` on `udm_ueau_auth_latency_seconds` |
| **SQN Resync Rate** | Time series | `rate(udm_ueau_sqn_resync_total[5m])` — high rate may indicate replay attack |
| **Failed Auth by SUPI** | Top-N table | Top subscribers by authentication failures (potential brute-force) |
| **TLS Certificate Expiry** | Table | Days until expiry per service certificate |
| **Error Response Codes** | Bar chart | Distribution of 4xx/5xx responses across services |
| **Geo-Anomaly** | Map visualization | Authentication requests by source PLMN — unusual patterns flagged |

---

## 10. References

| Reference | Description |
|-----------|-------------|
| 3GPP TS 28.532 | Management and orchestration; Management services |
| 3GPP TS 28.552 | Management and orchestration; 5G performance measurements |
| 3GPP TS 32.298 | Telecommunication management; Charging Data Record (CDR) parameter description |
| ITU-T X.733 | Information technology — Alarm reporting function |
| OpenTelemetry Specification | https://opentelemetry.io/docs/specs/otel/ |
| Prometheus Best Practices | https://prometheus.io/docs/practices/ |
| W3C Trace Context | https://www.w3.org/TR/trace-context/ |
| [architecture.md](architecture.md) | UDM High-Level Architecture |
| [service-decomposition.md](service-decomposition.md) | UDM Internal Service Decomposition |
| [data-model.md](data-model.md) | UDM Data Model and Schema Design |
