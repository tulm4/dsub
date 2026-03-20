# UDM Performance Architecture

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

---

## Table of Contents

1. [Performance Requirements Overview](#1-performance-requirements-overview)
2. [Performance Targets](#2-performance-targets)
3. [Golang Performance Optimization](#3-golang-performance-optimization)
4. [Database Performance Optimization](#4-database-performance-optimization)
5. [Caching Strategy](#5-caching-strategy)
6. [Connection Pool Optimization](#6-connection-pool-optimization)
7. [Load Balancing & Traffic Management](#7-load-balancing--traffic-management)
8. [Capacity Planning Model](#8-capacity-planning-model)
9. [Performance Testing Strategy](#9-performance-testing-strategy)
10. [Bottleneck Analysis & Mitigation](#10-bottleneck-analysis--mitigation)

---

## 1. Performance Requirements Overview

### 1.1 Purpose

This document defines the performance targets, optimization strategies, and capacity planning model for the 5G UDM system. As a telecom-grade network function, the UDM must meet strict latency, throughput, and availability requirements defined by 3GPP specifications and operator SLAs. Every design decision — from Golang runtime tuning to YugabyteDB tablet splitting — is evaluated against these targets.

### 1.2 Telecom-Grade Performance Principles

| Principle | Requirement | Rationale |
|-----------|-------------|-----------|
| **Ultra-low latency** | Sub-10ms p50 for all critical paths | UDM sits in the real-time signaling path; latency directly impacts call setup time and UE attach duration |
| **High throughput** | 100,000+ TPS aggregate across regions | Peak-hour signaling storms (e.g., mass re-registration after outage) require burst absorption |
| **Five-nines availability** | 99.999% uptime (≤5.26 min/year downtime) | Carrier-grade SLA; subscriber authentication must never be a single point of failure |
| **Predictable tail latency** | p99 within 3× of p50 | Tail latency spikes cause cascading timeouts in the 5G signaling chain (AMF → AUSF → UDM) |
| **Horizontal scalability** | Linear throughput scaling to 100M subscribers | Operator growth must not require architectural changes |
| **Graceful degradation** | Shed load before failing | Under overload, the UDM must reject excess traffic with 429/503 rather than collapse |

### 1.3 3GPP Performance Context

The UDM is invoked during latency-sensitive 5G procedures:

- **UE Registration** — AMF calls Nudm-UECM and Nudm-SDM during initial attach. Target: complete UDM interaction within 20ms.
- **Authentication** — AUSF calls Nudm-UEAU to generate authentication vectors (5G-AKA / EAP-AKA'). Target: complete within 15ms.
- **Session Establishment** — SMF calls Nudm-SDM for session management subscription data. Target: complete within 10ms.
- **Handover** — Target AMF re-registers via Nudm-UECM. Target: complete within 15ms.

---

## 2. Performance Targets

### 2.1 Latency Targets by Operation

| Operation | Target (p50) | Target (p95) | Target (p99) | Notes |
|-----------|-------------|-------------|-------------|-------|
| Auth vector generation (5G-AKA) | <5ms | <15ms | <30ms | Includes SQN management and MILENAGE/TUAK computation |
| SUCI deconcealment | <3ms | <10ms | <20ms | ECIES decryption (Profile A: Curve25519, Profile B: secp256r1) |
| SDM data retrieval | <3ms | <8ms | <15ms | Single-subscriber query with JSONB projection |
| AMF registration (UECM) | <5ms | <12ms | <25ms | Includes previous-AMF deregistration notification |
| SMF registration (UECM) | <5ms | <12ms | <25ms | Includes session context storage |
| EE subscription create | <8ms | <20ms | <40ms | Write-heavy; involves subscription persistence and callback setup |
| Parameter provisioning (PP) | <5ms | <12ms | <25ms | Subscription data modification with notification fan-out |
| MT location request | <5ms | <15ms | <30ms | Serving AMF lookup |
| DB single-row read | <1ms | <3ms | <5ms | YugabyteDB local-region read (follower reads or local leader) |
| DB single-row write | <2ms | <5ms | <10ms | YugabyteDB Raft-committed write (single-region quorum) |

### 2.2 Throughput Targets

| Traffic Class | Target TPS (per region) | Target TPS (aggregate, 3 regions) | Burst Factor |
|---------------|------------------------|-----------------------------------|--------------|
| Registration (UECM) | 10,000+ | 30,000+ | 5× (mass re-attach) |
| Authentication (UEAU) | 20,000+ | 60,000+ | 3× (re-auth storm) |
| SDM queries | 50,000+ | 150,000+ | 2× |
| Event exposure (EE) | 5,000+ | 15,000+ | 3× |
| Parameter provisioning | 2,000+ | 6,000+ | 2× |
| **Total system** | **~87,000** | **~100,000+** | — |

### 2.3 Availability & Reliability Targets

| Metric | Target | Measurement Window |
|--------|--------|--------------------|
| Service availability | 99.999% (five-nines) | Rolling 365-day |
| Planned maintenance downtime | 0 (zero-downtime upgrades) | Per event |
| Recovery Time Objective (RTO) | <30 seconds | Per-region failover |
| Recovery Point Objective (RPO) | 0 (synchronous replication within region) | Per transaction |
| Error rate (5xx responses) | <0.01% of total requests | Rolling 1-hour |
| Request timeout rate | <0.001% | Rolling 1-hour |

---

## 3. Golang Performance Optimization

### 3.1 Goroutine Pool Management

Each Nudm microservice handles thousands of concurrent requests. Unbounded goroutine creation during traffic spikes leads to memory exhaustion and GC pressure. A worker pool pattern bounds concurrency:

```go
package worker

import "context"

// Pool manages a fixed set of goroutines for bounded concurrency.
type Pool struct {
    sem chan struct{}
}

func NewPool(maxWorkers int) *Pool {
    return &Pool{sem: make(chan struct{}, maxWorkers)}
}

func (p *Pool) Submit(ctx context.Context, task func()) error {
    select {
    case p.sem <- struct{}{}:
        go func() {
            defer func() { <-p.sem }()
            task()
        }()
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**Per-service goroutine budget:**

| Service | Max Concurrent Goroutines | Rationale |
|---------|--------------------------|-----------|
| udm-ueau | 10,000 | High TPS, CPU-bound crypto |
| udm-sdm | 15,000 | Highest TPS, I/O-bound DB reads |
| udm-uecm | 10,000 | High TPS, mixed read/write |
| udm-ee | 5,000 | Medium TPS, long-lived subscriptions |
| Other services | 2,000 | Low-traffic services |

### 3.2 Memory Allocation Optimization

Frequent small allocations increase GC pressure. Use `sync.Pool` for request-scoped buffers and pre-allocate slices where sizes are predictable:

```go
import "sync"

// Reusable byte buffer pool for JSON serialization.
var bufferPool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 0, 4096)
        return &buf
    },
}

func GetBuffer() *[]byte {
    return bufferPool.Get().(*[]byte)
}

func PutBuffer(buf *[]byte) {
    *buf = (*buf)[:0]
    bufferPool.Put(buf)
}
```

**Pre-allocation guidelines:**

| Data Structure | Strategy | Example |
|---------------|----------|---------|
| Response slices | Pre-allocate to expected size | `make([]AuthVector, 0, 5)` |
| Map for subscriber data | Pre-allocate bucket count | `make(map[string]interface{}, 32)` |
| Byte buffers | Pool with `sync.Pool` | See example above |
| Protobuf messages | Reuse via `Reset()` | `msg.Reset()` before pool return |

### 3.3 Zero-Copy Techniques

Avoid unnecessary copies on the hot path:

- **String ↔ []byte conversion** — Use `unsafe.Slice` / `unsafe.String` (Go 1.20+) for read-only conversions in serialization paths where the source is guaranteed not to be modified.
- **io.Reader piping** — Stream DB results directly to HTTP response via `io.Pipe()` instead of buffering entire response in memory.
- **Slice reuse** — Return slices to pools rather than allocating new ones per request.

### 3.4 JSON Serialization Optimization

The standard `encoding/json` package uses reflection and is a significant CPU consumer on the SBI hot path. Use code-generated serializers:

```go
// Install: go install github.com/mailru/easyjson/...@latest
// Generate: easyjson -all model_types.go

//easyjson:json
type AuthenticationInfoResult struct {
    AuthType          string             `json:"authType"`
    AuthenticationVector *AuthVector     `json:"authenticationVector,omitempty"`
    Supi              string             `json:"supi,omitempty"`
}
```

**Serialization benchmark comparison:**

| Library | Marshal (ns/op) | Unmarshal (ns/op) | Allocs/op |
|---------|----------------|-------------------|-----------|
| `encoding/json` | ~2,500 | ~3,800 | ~40 |
| `easyjson` | ~600 | ~900 | ~5 |
| `sonic` | ~400 | ~700 | ~3 |
| `json-iterator` | ~1,200 | ~1,600 | ~20 |

Recommendation: Use **easyjson** for SBI request/response types (stable, widely adopted). Evaluate **sonic** for internal RPC where SIMD is available.

### 3.5 HTTP/2 Connection Management

All Nudm SBI interfaces use HTTP/2. Tune the transport for high-concurrency signaling:

```go
import (
    "crypto/tls"
    "net/http"
    "golang.org/x/net/http2"
    "time"
)

func NewSBITransport() *http.Transport {
    return &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS13,
        },
        MaxIdleConns:        500,
        MaxIdleConnsPerHost: 100,
        MaxConnsPerHost:     200,
        IdleConnTimeout:     90 * time.Second,
        ForceAttemptHTTP2:   true,
    }
}
```

**Key tuning parameters:**

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `MaxIdleConnsPerHost` | 100 | One UDM service fans out to limited NF peers; keep connections warm |
| `MaxConnsPerHost` | 200 | Upper bound prevents connection storms to a single peer |
| `IdleConnTimeout` | 90s | Match Kubernetes service mesh idle timeout |
| HTTP/2 `MaxConcurrentStreams` | 250 | Server-side limit per connection; prevents head-of-line blocking |

### 3.6 Context Cancellation and Timeout Handling

Every request must carry a context with a deadline to prevent resource leaks from stalled downstream calls:

```go
func (s *UEAUService) GenerateAuthData(ctx context.Context, supiOrSuci string) (*AuthResult, error) {
    ctx, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
    defer cancel()

    subData, err := s.repo.GetAuthSubscription(ctx, supiOrSuci)
    if err != nil {
        return nil, fmt.Errorf("auth subscription lookup: %w", err)
    }

    // Crypto operations also respect context cancellation.
    vector, err := s.crypto.GenerateVector(ctx, subData)
    if err != nil {
        return nil, fmt.Errorf("vector generation: %w", err)
    }

    return &AuthResult{Vector: vector}, nil
}
```

**Timeout budget allocation:**

| Layer | Budget | Cumulative |
|-------|--------|------------|
| HTTP handler | 30ms (p99 target) | 30ms |
| Business logic | 5ms | 5ms |
| DB query | 10ms | 15ms |
| Crypto operation | 10ms | 25ms |
| Serialization + I/O | 5ms | 30ms |

### 3.7 GC Tuning

Garbage collection pauses add tail latency. Tune the Go runtime for low-latency workloads:

```bash
# Reduce GC frequency — trade memory for fewer GC cycles.
export GOGC=200

# Set a hard memory limit to prevent OOM kills.
# Value = container memory limit minus 20% headroom.
export GOMEMLIMIT=1600MiB  # For a 2GiB container

# Enable GC trace logging for diagnostics (disable in production).
export GODEBUG=gctrace=1
```

**Recommended GC settings per service tier:**

| Service Tier | `GOGC` | `GOMEMLIMIT` | Container Memory | Rationale |
|-------------|--------|-------------|-----------------|-----------|
| High-traffic (UEAU, SDM, UECM) | 200 | 1600MiB | 2GiB | Fewer GC cycles; large working set |
| Medium-traffic (EE, PP, MT) | 150 | 800MiB | 1GiB | Moderate working set |
| Low-traffic (SSAU, NIDDAU, RSDS, UEID) | 100 (default) | 400MiB | 512MiB | Default is sufficient |

### 3.8 CPU Profiling and Optimization Strategy

Use `pprof` to identify hot paths and optimize iteratively:

```go
import (
    "net/http"
    _ "net/http/pprof"
)

func init() {
    // Expose pprof on a separate port, not on the SBI interface.
    go func() {
        _ = http.ListenAndServe("localhost:6060", nil)
    }()
}
```

**Profiling workflow:**

1. **Baseline** — Capture a 30-second CPU profile under steady-state load: `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30`
2. **Identify hot functions** — Focus on functions consuming >5% of total CPU.
3. **Optimize** — Apply targeted fixes (allocation reduction, algorithm improvement, caching).
4. **Validate** — Re-profile and compare flame graphs to confirm improvement.
5. **Regress-protect** — Add Go benchmarks for optimized functions to CI.

---

## 4. Database Performance Optimization

### 4.1 Connection Pooling (pgxpool)

YugabyteDB is accessed via the PostgreSQL wire protocol. Use `pgxpool` for high-performance connection management:

```go
import (
    "context"
    "github.com/jackc/pgx/v5/pgxpool"
)

func NewDBPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
    config, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, err
    }

    config.MaxConns = 40
    config.MinConns = 10
    config.MaxConnLifetime = 30 * time.Minute
    config.MaxConnIdleTime = 5 * time.Minute
    config.HealthCheckPeriod = 15 * time.Second

    return pgxpool.NewWithConfig(ctx, config)
}
```

### 4.2 Prepared Statements

Prepared statements eliminate repeated parse/plan overhead for high-frequency queries:

```go
// Prepared at connection init; executed thousands of times per second.
const getAuthSubscriptionSQL = `
    SELECT auth_method, permanent_key, opc, amf, sqn
    FROM auth_subscription
    WHERE supi = $1
`

func (r *AuthRepo) GetAuthSubscription(ctx context.Context, supi string) (*AuthSub, error) {
    var sub AuthSub
    err := r.pool.QueryRow(ctx, getAuthSubscriptionSQL, supi).Scan(
        &sub.AuthMethod, &sub.PermanentKey, &sub.OPc, &sub.AMF, &sub.SQN,
    )
    return &sub, err
}
```

### 4.3 Batch Operations

Batch inserts and updates for bulk provisioning and event exposure subscriptions:

```go
func (r *SubRepo) BatchUpsertSubscribers(ctx context.Context, subs []Subscriber) error {
    batch := &pgx.Batch{}
    for _, s := range subs {
        batch.Queue(
            `INSERT INTO subscriber_data (supi, subscription_data)
             VALUES ($1, $2)
             ON CONFLICT (supi) DO UPDATE SET subscription_data = $2`,
            s.SUPI, s.Data,
        )
    }

    br := r.pool.SendBatch(ctx, batch)
    defer br.Close()

    for range subs {
        if _, err := br.Exec(); err != nil {
            return fmt.Errorf("batch upsert: %w", err)
        }
    }
    return nil
}
```

### 4.4 Read Replicas and Follower Reads

SDM queries are read-heavy and latency-sensitive. Use YugabyteDB follower reads to serve from the nearest replica:

```sql
-- Serve reads from the closest follower (may be slightly stale).
SET yb_read_from_followers = true;
SET session_characteristics AS TRANSACTION READ ONLY;

-- Staleness bound: acceptable for subscription data that changes infrequently.
SET yb_follower_read_staleness_ms = 2000;
```

**Read strategy per service:**

| Service | Read Pattern | Follower Reads | Staleness Tolerance |
|---------|-------------|---------------|---------------------|
| udm-sdm | Subscription data lookups | Yes | 2 seconds |
| udm-uecm | AMF/SMF registration reads | No (strong consistency) | 0 |
| udm-ueau | Auth credential reads | No (SQN requires strong read) | 0 |
| udm-ee | Subscription listing | Yes | 5 seconds |
| udm-mt | Serving NF lookup | Yes | 1 second |

### 4.5 Query Optimization

Use `EXPLAIN ANALYZE` to validate query plans hit indexes:

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT supi, subscription_data->'accessAndMobilitySubscriptionData'
FROM subscriber_data
WHERE supi = 'imsi-001010000000001';

-- Expected output:
-- Index Scan using subscriber_data_pkey on subscriber_data
--   Index Cond: (supi = 'imsi-001010000000001'::text)
--   Execution Time: 0.5ms
```

**Query optimization checklist:**

| Check | Action |
|-------|--------|
| Full table scans | Add appropriate index or rewrite query |
| Sequential scan on large table | Verify index exists and is being used |
| High buffer reads | Consider caching or denormalization |
| Nested loop joins | Evaluate join order; consider colocation |
| JSONB full-document fetch | Use JSONB path projection (`->`, `->>`) |

### 4.6 Index Tuning

```sql
-- Primary key indexes (created automatically).
-- subscriber_data(supi) — point lookups by SUPI.

-- Secondary indexes for access patterns.
CREATE INDEX idx_amf_registration_supi ON amf_registration (supi) INCLUDE (amf_instance_id);
CREATE INDEX idx_smf_registration_supi_pdu ON smf_registration (supi, pdu_session_id);
CREATE INDEX idx_ee_subscription_supi ON ee_subscription (supi, subscription_id);

-- JSONB GIN index for flexible queries.
CREATE INDEX idx_sub_data_gin ON subscriber_data USING GIN (subscription_data jsonb_path_ops);
```

### 4.7 Tablet Splitting for Hot Partitions

High-traffic SUPIs (e.g., IoT gateways) can create hot tablets. Pre-split tables and use hash sharding:

```sql
-- Hash-sharded table distributes rows evenly across tablets.
CREATE TABLE auth_subscription (
    supi TEXT NOT NULL,
    auth_method TEXT NOT NULL,
    permanent_key BYTEA NOT NULL,
    opc BYTEA NOT NULL,
    amf BYTEA NOT NULL,
    sqn BYTEA NOT NULL,
    PRIMARY KEY (supi ASC)
) SPLIT AT VALUES (
    ('imsi-200000000000000'),
    ('imsi-400000000000000'),
    ('imsi-600000000000000'),
    ('imsi-800000000000000')
);
```

**Tablet sizing guidelines:**

| Subscriber Count | Tablets per Table | Tablet Size Target |
|-----------------|------------------|--------------------|
| 10M | 30 | ~4GB per tablet |
| 50M | 80 | ~5GB per tablet |
| 100M | 150 | ~5GB per tablet |

### 4.8 Colocation for Small Tables

Small, frequently-joined tables should be colocated to avoid cross-tablet network hops:

```sql
-- Create a colocated database for reference data.
CREATE DATABASE udm_config WITH COLOCATION = true;

-- Small tables (e.g., PLMN config, operator policies) are colocated by default.
CREATE TABLE operator_policy (
    policy_id TEXT PRIMARY KEY,
    policy_data JSONB NOT NULL
);

CREATE TABLE plmn_config (
    plmn_id TEXT PRIMARY KEY,
    config JSONB NOT NULL
);
```

### 4.9 JSONB Query Optimization

Subscription data is stored as JSONB. Optimize queries to avoid full-document deserialization:

```sql
-- Retrieve only the access-and-mobility slice of subscription data.
SELECT subscription_data->'accessAndMobilitySubscriptionData' AS am_data
FROM subscriber_data
WHERE supi = $1;

-- Filter on nested JSONB field with index support.
SELECT supi
FROM subscriber_data
WHERE subscription_data @> '{"nssai": {"defaultSingleNssais": [{"sst": 1}]}}';
```

---

## 5. Caching Strategy

### 5.1 Multi-Tier Cache Architecture

The UDM employs a two-tier caching strategy to minimize database load for read-heavy subscriber data:

| Tier | Technology | Scope | Latency | Capacity |
|------|-----------|-------|---------|----------|
| **L1 — In-Process** | Go `sync.Map` / LRU cache | Per-pod | <0.1ms | 100MB per pod |
| **L2 — Distributed** | Redis Cluster | Per-region | <1ms | 10–100GB per region |
| **L3 — Database** | YugabyteDB | Global | 1–5ms | Unbounded |

### 5.2 Cache-Aside Pattern for Subscription Data

Subscription data (SDM) is read-heavy and changes infrequently. Use cache-aside:

```go
func (s *SDMService) GetSubscriptionData(ctx context.Context, supi string) (*SubData, error) {
    // L1: Check in-process cache.
    if data, ok := s.l1Cache.Get(supi); ok {
        return data.(*SubData), nil
    }

    // L2: Check Redis.
    data, err := s.redisClient.Get(ctx, "sdm:"+supi).Result()
    if err == nil {
        var sub SubData
        if err := json.Unmarshal([]byte(data), &sub); err == nil {
            s.l1Cache.Set(supi, &sub)
            return &sub, nil
        }
    }

    // L3: Query database.
    sub, err := s.repo.GetSubscriptionData(ctx, supi)
    if err != nil {
        return nil, err
    }

    // Populate caches.
    serialized, _ := json.Marshal(sub)
    s.redisClient.Set(ctx, "sdm:"+supi, serialized, 5*time.Minute)
    s.l1Cache.Set(supi, sub)

    return sub, nil
}
```

### 5.3 Write-Through for Registration Data

Registration state (AMF/SMF context) must be immediately consistent. Use write-through caching:

```go
func (s *UECMService) RegisterAMF(ctx context.Context, supi string, reg *AMFRegistration) error {
    // Write to database first.
    if err := s.repo.UpsertAMFRegistration(ctx, supi, reg); err != nil {
        return err
    }

    // Update L2 cache synchronously.
    serialized, _ := json.Marshal(reg)
    return s.redisClient.Set(ctx, "uecm:amf:"+supi, serialized, 10*time.Minute).Err()
}
```

### 5.4 Cache Invalidation

| Strategy | Use Case | Implementation |
|----------|----------|----------------|
| **TTL-based** | Subscription data (SDM) | 5-minute TTL on L2; 30-second TTL on L1 |
| **Event-driven** | Registration state (UECM) | Publish invalidation event on write; subscribers evict L1 entries |
| **Version-based** | Provisioning updates (PP) | Store version counter; compare on read |

Event-driven invalidation uses Redis Pub/Sub to coordinate L1 cache eviction across pods:

```go
func (s *CacheInvalidator) Subscribe(ctx context.Context) {
    pubsub := s.redisClient.Subscribe(ctx, "cache:invalidate")
    ch := pubsub.Channel()

    for msg := range ch {
        s.l1Cache.Delete(msg.Payload)
    }
}
```

### 5.5 Cache Warming on Startup

Cold caches cause latency spikes after pod restarts. Warm L1 caches during startup:

```go
func (s *SDMService) WarmCache(ctx context.Context) error {
    // Load the most recently accessed subscribers into L1.
    rows, err := s.repo.GetRecentlyAccessedSubscribers(ctx, 10000)
    if err != nil {
        return err
    }
    for _, row := range rows {
        s.l1Cache.Set(row.SUPI, row.Data)
    }
    return nil
}
```

### 5.6 Cache Sizing by Subscriber Scale

| Subscriber Count | L1 Cache (per pod) | L2 Cache (Redis, per region) | Redis Topology |
|-----------------|--------------------|-----------------------------|----------------|
| 10M | 100MB (~100K hot entries) | ~10GB | 3-node Redis Sentinel |
| 50M | 200MB (~200K hot entries) | ~50GB | 6-node Redis Cluster |
| 100M | 200MB (~200K hot entries) | ~100GB | 12-node Redis Cluster |

### 5.7 Cache Hit Rate Targets

| Data Type | Target Hit Rate | Measurement |
|-----------|----------------|-------------|
| Subscription data (SDM) | >90% | L1 + L2 combined |
| Registration state (UECM) | >80% | L2 only (write-through) |
| Auth credentials (UEAU) | >70% | L2 only (security-sensitive) |
| Overall system | >85% | Weighted average across services |

---

## 6. Connection Pool Optimization

### 6.1 Pool Sizing Formula

The optimal connection pool size follows the established formula:

```
connections = (num_cpu_cores * 2) + effective_disk_spindles
```

For NVMe-backed YugabyteDB (no rotational disks), `effective_disk_spindles` ≈ 1, simplifying to:

```
connections = (num_cpu_cores * 2) + 1
```

### 6.2 Per-Service Pool Configuration

| Service | CPU Cores | Max Pool Size | Min Pool Size | Max Overflow |
|---------|-----------|---------------|---------------|--------------|
| udm-ueau | 4 | 9 | 4 | 18 |
| udm-sdm | 4 | 9 | 4 | 18 |
| udm-uecm | 4 | 9 | 4 | 18 |
| udm-ee | 2 | 5 | 2 | 10 |
| udm-pp | 2 | 5 | 2 | 10 |
| udm-mt | 2 | 5 | 2 | 10 |
| udm-ssau | 1 | 3 | 1 | 6 |
| udm-niddau | 1 | 3 | 1 | 6 |
| udm-rsds | 1 | 3 | 1 | 6 |
| udm-ueid | 1 | 3 | 1 | 6 |

### 6.3 Connection Lifecycle Management

```go
config.MaxConnLifetime = 30 * time.Minute   // Recycle connections to rebalance after node changes.
config.MaxConnIdleTime = 5 * time.Minute     // Release idle connections to free server resources.
config.HealthCheckPeriod = 15 * time.Second  // Detect and remove dead connections proactively.
```

### 6.4 Idle Connection Cleanup

Kubernetes pod autoscaling causes connection count fluctuation. Aggressive idle cleanup prevents connection exhaustion on the database:

- **Scale-up**: New pods create `MinConns` connections immediately.
- **Steady state**: Pool grows to `MaxConns` under load, then idle connections are reaped after `MaxConnIdleTime`.
- **Scale-down**: Terminating pods close all connections in the `Shutdown()` hook.

### 6.5 PgBouncer as Connection Multiplexer

For very high pod counts (>100 pods per service), interpose PgBouncer between application pods and YugabyteDB to multiplex connections:

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| `pool_mode` | `transaction` | Release connection after each transaction; maximizes reuse |
| `max_client_conn` | 5,000 | Accept many application connections |
| `default_pool_size` | 50 per YugabyteDB node | Limit actual DB connections |
| `reserve_pool_size` | 10 | Extra connections for burst absorption |
| `server_idle_timeout` | 300s | Keep warm connections to DB |

---

## 7. Load Balancing & Traffic Management

### 7.1 L7 Load Balancing for HTTP/2

The UDM SBI interfaces use HTTP/2, which multiplexes requests over a single TCP connection. L4 load balancers distribute poorly because a single connection carries all traffic. Use L7 (Istio/Envoy) load balancing:

| Feature | Implementation |
|---------|----------------|
| **Protocol-aware balancing** | Envoy distributes individual HTTP/2 streams across backend pods |
| **Health-aware routing** | Outlier detection removes unhealthy pods within 5 seconds |
| **Circuit breaking** | Limit concurrent requests per pod to prevent overload |
| **Retry budgets** | Max 20% of requests may be retried to prevent retry storms |

### 7.2 Client-Side Load Balancing

For internal service-to-service calls (e.g., notification callbacks), use client-side balancing for lower latency:

```go
// Round-robin with health checking over discovered endpoints.
type ClientLB struct {
    endpoints []string
    index     atomic.Uint64
}

func (lb *ClientLB) Next() string {
    idx := lb.index.Add(1)
    return lb.endpoints[idx%uint64(len(lb.endpoints))]
}
```

### 7.3 Locality-Aware Routing

In a multi-region deployment, route requests to the nearest UDM instance to minimize latency:

| Routing Priority | Description |
|-----------------|-------------|
| 1. Same zone | Pod in the same availability zone |
| 2. Same region | Pod in the same region, different zone |
| 3. Cross-region | Pod in a different region (only if local region is unhealthy) |

Istio `DestinationRule` configuration:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: udm-sdm-locality
spec:
  host: udm-sdm.udm.svc.cluster.local
  trafficPolicy:
    loadBalancer:
      localityLbSetting:
        enabled: true
        failover:
          - from: us-east-1
            to: us-west-2
          - from: us-west-2
            to: eu-west-1
    outlierDetection:
      consecutive5xxErrors: 3
      interval: 10s
      baseEjectionTime: 30s
```

### 7.4 Weighted Routing for Canary Deployments

Roll out new UDM versions safely with traffic shifting:

| Phase | Canary Weight | Duration | Gate Criteria |
|-------|--------------|----------|---------------|
| 1 | 1% | 15 minutes | p99 latency within 10% of baseline |
| 2 | 10% | 30 minutes | Error rate <0.01% |
| 3 | 50% | 1 hour | All SLOs met |
| 4 | 100% | — | Full rollout |

### 7.5 Connection Affinity for Stateful Flows

Certain UDM flows (e.g., EE notification subscriptions) benefit from connection affinity to reduce cache misses:

| Flow | Affinity Type | Key |
|------|--------------|-----|
| EE subscribe/notify | Consistent hashing | SUPI |
| UECM registration | None (stateless) | — |
| SDM get/subscribe | Consistent hashing | SUPI |

---

## 8. Capacity Planning Model

### 8.1 Per-Subscriber Resource Model

| Resource | Per Active Subscriber | Derivation |
|----------|----------------------|------------|
| **CPU** | ~0.001 cores | Based on 100K TPS across 100 cores |
| **Memory (cache)** | ~1KB | Average subscriber record in L1 cache |
| **Storage (DB)** | ~12KB | Auth (2KB) + SDM (6KB) + UECM (2KB) + Indexes (2KB) |
| **Network (ingress)** | ~0.5 Kbps | Average 5 signaling events/hour at 1KB each |

### 8.2 Scale-Out Planning Tables

**Compute Resources (per region):**

| Subscriber Count | 10M | 50M | 100M |
|-----------------|-----|-----|------|
| **udm-ueau pods** | 6 | 18 | 36 |
| **udm-sdm pods** | 8 | 24 | 48 |
| **udm-uecm pods** | 6 | 18 | 36 |
| **udm-ee pods** | 3 | 8 | 16 |
| **udm-pp pods** | 3 | 6 | 12 |
| **udm-mt pods** | 3 | 6 | 12 |
| **udm-ssau pods** | 2 | 4 | 6 |
| **udm-niddau pods** | 2 | 4 | 6 |
| **udm-rsds pods** | 2 | 4 | 6 |
| **udm-ueid pods** | 2 | 4 | 6 |
| **Total UDM pods** | **37** | **96** | **184** |
| **Total vCPUs** | 112 | 320 | 640 |
| **Total Memory** | 140GB | 400GB | 800GB |

**YugabyteDB Sizing (per region):**

| Subscriber Count | 10M | 50M | 100M |
|-----------------|-----|-----|------|
| TServer nodes | 3 | 9 | 18 |
| Master nodes | 3 | 3 | 5 |
| vCPUs per TServer | 8 | 16 | 16 |
| RAM per TServer | 32GB | 64GB | 64GB |
| Storage per TServer | 200GB NVMe | 500GB NVMe | 1TB NVMe |
| Total DB storage | 600GB | 4.5TB | 18TB |
| Replication factor | 3 | 3 | 3 |

**Network Bandwidth Estimation (per region):**

| Subscriber Count | Ingress (SBI) | DB Replication | Cache Traffic | Total |
|-----------------|---------------|----------------|---------------|-------|
| 10M | 2 Gbps | 1 Gbps | 0.5 Gbps | ~4 Gbps |
| 50M | 8 Gbps | 4 Gbps | 2 Gbps | ~14 Gbps |
| 100M | 15 Gbps | 8 Gbps | 4 Gbps | ~27 Gbps |

---

## 9. Performance Testing Strategy

### 9.1 Go Benchmark Suite

Every performance-critical function must have a corresponding Go benchmark:

```go
func BenchmarkGenerateAuthVector(b *testing.B) {
    svc := NewAuthService(testConfig)
    ctx := context.Background()
    sub := &AuthSubscription{
        AuthMethod:   "5G_AKA",
        PermanentKey: testKey,
        OPc:          testOPc,
    }

    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, err := svc.GenerateVector(ctx, sub)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}

func BenchmarkJSONMarshalAuthResult(b *testing.B) {
    result := &AuthenticationInfoResult{
        AuthType: "5G_AKA",
        Supi:     "imsi-001010000000001",
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = result.MarshalJSON() // easyjson generated
    }
}
```

**Benchmark CI gate:** Any PR that degrades a tracked benchmark by >5% is flagged for review.

### 9.2 Load Test Profiles

| Profile | Description | Duration | Target TPS | Success Criteria |
|---------|-------------|----------|-----------|------------------|
| **Steady state** | Normal production traffic mix | 1 hour | 50K TPS | p99 < targets; 0% errors |
| **Peak hour** | 2× steady-state traffic | 30 min | 100K TPS | p99 < 2× targets; <0.01% errors |
| **Burst (re-attach storm)** | 5× registration traffic for 60s | 5 min | 50K reg TPS | Graceful degradation; no crash |
| **Ramp-up** | Linear increase from 0 to 120K TPS | 30 min | 0→120K | Find breaking point; no cascading failure |
| **Soak test** | Steady state for extended period | 24 hours | 50K TPS | No memory leaks; stable p99 |
| **Region failover** | Kill one region during load | 15 min | 50K TPS | <30s recovery; no data loss |

### 9.3 Performance Regression Detection

Automated regression detection runs on every merge to `main`:

1. **Micro-benchmarks** — `go test -bench=. -benchmem` compared against baseline via `benchstat`.
2. **Integration benchmarks** — k6 load test against a staging environment; results stored in time-series DB.
3. **Alerting** — If p99 latency increases >10% or throughput drops >5%, the pipeline fails and notifies the team.

### 9.4 Continuous Performance Monitoring

Production performance is tracked via:

| Metric | Tool | Alert Threshold |
|--------|------|----------------|
| Request latency (p50/p95/p99) | Prometheus + Grafana | p99 > 2× target for 5 min |
| Request throughput (TPS) | Prometheus | Drop >20% from baseline |
| Error rate (5xx) | Prometheus | >0.01% over 5 min |
| Goroutine count | Go runtime metrics | >2× expected maximum |
| Heap allocation rate | Go runtime metrics | >500MB/s sustained |
| DB connection pool utilization | pgxpool metrics | >80% for 5 min |
| Cache hit rate | Redis metrics | <80% for 15 min |
| GC pause duration | Go runtime metrics | p99 >5ms |

---

## 10. Bottleneck Analysis & Mitigation

### 10.1 Common Telecom Bottlenecks

| Bottleneck | Symptom | Root Cause | Mitigation |
|-----------|---------|-----------|------------|
| **SQN synchronization** | Auth failures during handover | SQN window exhaustion under concurrent auth | Use SQN windowing (TS 33.102 §6.3.7); batch SQN increments |
| **SUCI deconcealment CPU** | High p99 on UEAU | ECIES decryption is CPU-intensive | Dedicate CPU cores to UEAU pods; use hardware acceleration if available |
| **Subscription data fan-out** | High SDM latency during bulk provisioning | DB write contention on subscriber_data table | Separate read/write paths; use follower reads for SDM |
| **Notification storms** | EE service overwhelmed | Single subscriber event triggers thousands of NF notifications | Rate-limit notifications; batch delivery with configurable delay |
| **Connection pool exhaustion** | Requests queuing; latency spike | Pool too small for traffic burst | Monitor pool utilization; auto-scale pool size with pod CPU |
| **GC stop-the-world pauses** | Periodic latency spikes (every 2–5s) | Large heap with default GOGC | Tune GOGC=200; set GOMEMLIMIT; reduce allocations on hot path |
| **Cross-region DB writes** | Write latency >50ms | Raft consensus across regions | Use geo-partitioned tables; prefer local leader for writes |
| **Hot tablet** | Single DB node at high CPU | Skewed data distribution (popular SUPI range) | Pre-split tables; hash-shard primary keys |
| **TLS handshake overhead** | Connection setup latency | Frequent new connections without reuse | Keep-alive connections; connection pooling; TLS session resumption |
| **JSON serialization** | High CPU on SBI path | `encoding/json` reflection overhead | Switch to easyjson/sonic; use code-generated marshalers |

### 10.2 Diagnostic Playbook

When a performance SLO is breached, follow this triage sequence:

1. **Check error rates** — Is the system returning 5xx errors? If yes, prioritize error resolution.
2. **Check latency breakdown** — Use distributed tracing (Jaeger) to identify which layer (HTTP, business logic, DB, cache) is slow.
3. **Check resource utilization** — CPU, memory, network, DB connections. If any resource is >80%, investigate saturation.
4. **Check cache hit rates** — A drop in cache hit rate causes a proportional increase in DB load.
5. **Check DB query plans** — Run `EXPLAIN ANALYZE` on slow queries; look for sequential scans or missing indexes.
6. **Check GC metrics** — High GC frequency or long pauses indicate memory pressure.
7. **Profile the hot path** — Use `pprof` to capture a CPU profile during the incident.

### 10.3 Performance Budget Enforcement

Each UDM service has a per-request performance budget that must not be exceeded:

| Component | Budget (ms) | Enforcement |
|-----------|-------------|-------------|
| HTTP middleware (auth, logging, metrics) | 1 | Benchmark in CI |
| Request deserialization | 0.5 | Use code-generated deserializers |
| Business logic | 3–5 | Code review + profiling |
| Database query (single row) | 3–5 | Query plan review; slow-query alerting |
| Cache lookup | 0.5 | Monitor cache latency |
| Response serialization | 0.5 | Use code-generated serializers |
| **Total** | **~10–15** | End-to-end latency monitoring |

---

## References

| Reference | Description |
|-----------|-------------|
| [architecture.md](architecture.md) | UDM high-level architecture |
| [deployment.md](deployment.md) | Deployment architecture and scaling |
| [data-model.md](data-model.md) | Database schema and data model |
| [observability.md](observability.md) | Monitoring, logging, and tracing |
| 3GPP TS 29.503 | Nudm service specification |
| 3GPP TS 33.501 | 5G security architecture |
| [YugabyteDB Performance Tuning](https://docs.yugabyte.com/preview/develop/best-practices-ysql/) | YugabyteDB YSQL best practices |
| [Go Performance Wiki](https://go.dev/wiki/Performance) | Go runtime performance guidance |
