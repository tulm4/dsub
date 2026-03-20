# UDM Testing Strategy

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

---

## Table of Contents

1. [Testing Strategy Overview](#1-testing-strategy-overview)
   1. [Purpose](#11-purpose)
   2. [Testing Pyramid](#12-testing-pyramid)
   3. [Quality Gates](#13-quality-gates)
   4. [Shift-Left Approach](#14-shift-left-approach)
2. [Functional Testing](#2-functional-testing)
   1. [Unit Testing](#21-unit-testing)
   2. [Integration Testing](#22-integration-testing)
   3. [API Testing](#23-api-testing)
   4. [Database Testing](#24-database-testing)
3. [Telecom Protocol Testing](#3-telecom-protocol-testing)
   1. [HTTP/2 SBI Validation](#31-http2-sbi-validation)
   2. [5G Signaling Correctness](#32-5g-signaling-correctness)
   3. [Custom 3GPP Header Handling](#33-custom-3gpp-header-handling)
   4. [Content-Type Negotiation](#34-content-type-negotiation)
   5. [Response Code Compliance](#35-response-code-compliance)
4. [5G Telecom Scenario Testing](#4-5g-telecom-scenario-testing)
   1. [UE Registration (5G Attach)](#41-ue-registration-5g-attach)
   2. [Subscriber Authentication (5G AKA)](#42-subscriber-authentication-5g-aka)
   3. [SUPI/SUCI Handling](#43-supisuci-handling)
   4. [Subscription Data Retrieval and Update](#44-subscription-data-retrieval-and-update)
   5. [Roaming Scenarios](#45-roaming-scenarios)
   6. [Network Slicing](#46-network-slicing)
5. [Performance and Resilience Testing](#5-performance-and-resilience-testing)
   1. [Load Testing](#51-load-testing)
   2. [Stress Testing](#52-stress-testing)
   3. [Soak Testing](#53-soak-testing)
   4. [Chaos Engineering](#54-chaos-engineering)
6. [Test Environments](#6-test-environments)
7. [CI/CD Integration](#7-cicd-integration)
8. [Test Data Management](#8-test-data-management)

---

## 1. Testing Strategy Overview

### 1.1 Purpose

This document defines the comprehensive testing strategy for the 5G Unified Data Management (UDM)
system. It covers all testing layers — from unit tests of individual Go functions through full
end-to-end 5G telecom scenario validation — ensuring the UDM meets 3GPP specifications, operator
performance requirements, and production reliability standards.

The UDM is a critical 5G Core network function responsible for subscriber authentication,
subscription data management, and identity handling across 10 Nudm microservices backed by
YugabyteDB. Any defect in the UDM can impact millions of subscribers, making a rigorous,
multi-layered testing approach essential.

### 1.2 Testing Pyramid

The testing strategy follows a **testing pyramid** model with fast, isolated tests at the base and
progressively broader — but slower — tests toward the top.

```
            ╱ ╲
           ╱   ╲              E2E Telecom Scenarios
          ╱     ╲             (5G attach, auth, roaming)
         ╱───────╲
        ╱         ╲           Performance & Resilience
       ╱           ╲          (load, soak, chaos)
      ╱─────────────╲
     ╱               ╲        API & Protocol Tests
    ╱                 ╲        (HTTP/2 SBI, 3GPP compliance)
   ╱───────────────────╲
  ╱                     ╲     Integration Tests
 ╱                       ╲    (service + YugabyteDB)
╱─────────────────────────╲
╲                         ╱   Unit Tests
 ╲───────────────────────╱    (Go table-driven, mocks)
```

| Layer | Proportion | Execution Time | Frequency |
|-------|-----------|----------------|-----------|
| Unit Tests | ~60% of all tests | Seconds | Every commit |
| Integration Tests | ~20% | Minutes | Every merge to `main` |
| API & Protocol Tests | ~10% | Minutes | Every merge to `main` |
| Performance & Resilience | ~5% | Hours | Weekly / Monthly |
| E2E Telecom Scenarios | ~5% | Minutes–Hours | Nightly / On-demand |

### 1.3 Quality Gates

Every change must pass through a series of quality gates before reaching production.

| Gate | Trigger | Criteria | Blocking |
|------|---------|----------|----------|
| **G1 — Lint & Static Analysis** | PR opened | `golangci-lint` passes, no new findings | Yes |
| **G2 — Unit Tests** | PR opened | All unit tests pass, coverage ≥ 80% | Yes |
| **G3 — Integration Tests** | Merge to `main` | All integration tests pass against YugabyteDB | Yes |
| **G4 — API Conformance** | Merge to `main` | All 103 endpoints pass schema validation | Yes |
| **G5 — Performance Baseline** | Weekly schedule | No regression beyond 5% of baseline TPS | Advisory |
| **G6 — Chaos Resilience** | Monthly schedule | System recovers within SLA after fault injection | Advisory |
| **G7 — Staging Validation** | Pre-release | Full telecom scenario suite passes in staging | Yes |

### 1.4 Shift-Left Approach

Testing is embedded as early as possible in the development lifecycle:

- **Design phase** — API contract tests are defined alongside OpenAPI spec authoring. Database
  schema changes include migration test plans.
- **Development phase** — Developers write unit tests alongside production code (TDD encouraged).
  Integration test scaffolding is provided via shared `testutil` packages.
- **Code review** — CI enforces coverage thresholds and runs the full unit + lint suite before
  review begins. Reviewers verify test quality, not just code quality.
- **Pre-merge** — Integration and API conformance tests run automatically on the merge queue.
- **Post-merge** — Nightly telecom scenario tests catch cross-service regressions. Weekly
  performance tests detect latency and throughput drift.

---

## 2. Functional Testing

### 2.1 Unit Testing

Unit tests use the **Go standard `testing` package** with table-driven test patterns. Every
exported function and every handler in the 10 Nudm services must have corresponding unit tests.

#### 2.1.1 Standards and Targets

| Metric | Target |
|--------|--------|
| Line coverage | ≥ 80% per package |
| Branch coverage | ≥ 75% per package |
| Critical-path coverage (auth, deconceal) | ≥ 95% |
| Test naming convention | `Test<Function>_<Scenario>` |
| Execution time per package | < 30 seconds |

#### 2.1.2 Table-Driven Tests

All handler tests use the table-driven pattern to maximize scenario coverage with minimal
boilerplate:

```go
func TestGenerateAuthVector_5GAKA(t *testing.T) {
    tests := []struct {
        name           string
        supi           string
        servingNetwork string
        authType       string
        wantAlgo       string
        wantErr        bool
    }{
        {
            name:           "valid 5G-AKA with IMSI",
            supi:           "imsi-001010000000001",
            servingNetwork: "5G:mnc001.mcc001.3gppnetwork.org",
            authType:       "5G_AKA",
            wantAlgo:       "5G_AKA",
            wantErr:        false,
        },
        {
            name:           "valid EAP-AKA-prime",
            supi:           "imsi-001010000000002",
            servingNetwork: "5G:mnc001.mcc001.3gppnetwork.org",
            authType:       "EAP_AKA_PRIME",
            wantAlgo:       "EAP_AKA_PRIME",
            wantErr:        false,
        },
        {
            name:           "unknown subscriber",
            supi:           "imsi-999990000000001",
            servingNetwork: "5G:mnc001.mcc001.3gppnetwork.org",
            authType:       "5G_AKA",
            wantAlgo:       "",
            wantErr:        true,
        },
        {
            name:           "invalid SUPI format",
            supi:           "invalid-format",
            servingNetwork: "5G:mnc001.mcc001.3gppnetwork.org",
            authType:       "5G_AKA",
            wantAlgo:       "",
            wantErr:        true,
        },
        {
            name:           "empty serving network",
            supi:           "imsi-001010000000001",
            servingNetwork: "",
            authType:       "5G_AKA",
            wantAlgo:       "",
            wantErr:        true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := &mockAuthRepo{
                subscribers: map[string]*AuthSubscription{
                    "imsi-001010000000001": testAuthSub("5G_AKA"),
                    "imsi-001010000000002": testAuthSub("EAP_AKA_PRIME"),
                },
            }
            svc := NewUEAuthService(repo)

            result, err := svc.GenerateAuthVector(context.Background(), tt.supi, tt.servingNetwork, tt.authType)

            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.wantAlgo, result.AuthType)
            assert.NotEmpty(t, result.RAND)
            assert.NotEmpty(t, result.AUTN)
            assert.NotEmpty(t, result.XRES)
            assert.NotEmpty(t, result.KAUSF)
        })
    }
}
```

#### 2.1.3 SUCI Deconceal Test

```go
func TestDeconcealSUCI(t *testing.T) {
    tests := []struct {
        name     string
        suci     string
        wantSUPI string
        wantErr  bool
        errCode  string
    }{
        {
            name:     "null scheme — SUPI passed through",
            suci:     "suci-0-001-01-0000-0-0-0000000001",
            wantSUPI: "imsi-001010000000001",
            wantErr:  false,
        },
        {
            name:     "Profile A ECIES deconceal",
            suci:     "suci-0-001-01-0000-1-1-<encrypted_msin_hex>",
            wantSUPI: "imsi-001010000000001",
            wantErr:  false,
        },
        {
            name:     "Profile B ECIES deconceal",
            suci:     "suci-0-001-01-0000-2-1-<encrypted_msin_hex>",
            wantSUPI: "imsi-001010000000001",
            wantErr:  false,
        },
        {
            name:     "invalid SUCI format",
            suci:     "not-a-valid-suci",
            wantSUPI: "",
            wantErr:  true,
            errCode:  "INVALID_QUERY_PARAM",
        },
        {
            name:     "unknown protection scheme",
            suci:     "suci-0-001-01-0000-9-1-<data>",
            wantSUPI: "",
            wantErr:  true,
            errCode:  "UNSUPPORTED_PROTECTION_SCHEME",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := &mockKeyRepo{
                keys: testHomeNetworkKeys(),
            }
            svc := NewSUCIDeconcealService(repo)

            supi, err := svc.Deconceal(context.Background(), tt.suci)

            if tt.wantErr {
                assert.Error(t, err)
                var probErr *ProblemDetailsError
                assert.ErrorAs(t, err, &probErr)
                assert.Equal(t, tt.errCode, probErr.Cause)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.wantSUPI, supi)
        })
    }
}
```

#### 2.1.4 Mock Interfaces

All external dependencies are abstracted behind Go interfaces so that unit tests never touch a
real database or external network function:

```go
// Repository interfaces (mocked in unit tests)
type AuthSubscriptionRepo interface {
    GetBySupi(ctx context.Context, supi string) (*AuthSubscription, error)
    UpdateSQN(ctx context.Context, supi string, sqn []byte) error
}

type SubscriptionDataRepo interface {
    GetAccessAndMobilityData(ctx context.Context, supi string) (*AccessAndMobilitySubscriptionData, error)
    GetSessionManagementData(ctx context.Context, supi string, sNssai *Snssai, dnn string) (*SessionManagementSubscriptionData, error)
}

// External NF client interfaces (mocked in unit tests)
type NRFClient interface {
    RegisterNFInstance(ctx context.Context, profile *NFProfile) error
    Discover(ctx context.Context, targetType, requesterType string) ([]NFProfile, error)
}
```

Mock implementations use in-memory maps for fast, deterministic tests. The project avoids
code-generated mocks where hand-written mocks provide clearer test intent.

### 2.2 Integration Testing

Integration tests verify that services interact correctly with a real YugabyteDB instance. They
use **Testcontainers for Go** to spin up ephemeral database containers per test suite.

#### 2.2.1 Testcontainers Setup

```go
func setupYugabyteDB(t *testing.T) (*YBContainer, *pgxpool.Pool) {
    t.Helper()
    ctx := context.Background()

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "yugabytedb/yugabyte:2.20-latest",
            ExposedPorts: []string{"5433/tcp"},
            Cmd:          []string{"bin/yugabyted", "start", "--daemon=false"},
            WaitingFor:   wait.ForLog("YugabyteDB Started").WithStartupTimeout(120 * time.Second),
        },
        Started: true,
    })
    require.NoError(t, err)
    t.Cleanup(func() { _ = container.Terminate(ctx) })

    port, err := container.MappedPort(ctx, "5433")
    require.NoError(t, err)

    dsn := fmt.Sprintf("postgres://yugabyte:yugabyte@localhost:%s/udm_db?sslmode=disable", port.Port())
    pool, err := pgxpool.New(ctx, dsn)
    require.NoError(t, err)

    runMigrations(t, dsn)
    return container, pool
}
```

#### 2.2.2 Schema Migration Validation

Every schema migration is tested in both forward and backward directions:

| Test Case | Description |
|-----------|-------------|
| `TestMigration_Forward` | Apply all migrations sequentially, verify final schema |
| `TestMigration_Backward` | Roll back each migration, verify no data loss for reversible changes |
| `TestMigration_Idempotent` | Apply same migration twice, verify no error |
| `TestMigration_DataPreservation` | Seed data, apply migration, verify data integrity |

#### 2.2.3 CRUD Operations for All Tables

Integration tests cover create, read, update, and delete operations for all 23 database tables:

| Table Group | Tables | Key Test Scenarios |
|-------------|--------|--------------------|
| **Auth** | `auth_subscription`, `auth_event`, `auth_status` | Key rotation, SQN updates, event logging |
| **Subscriber Identity** | `subscriber_identity`, `gpsi_mapping` | SUPI/GPSI resolution, multi-GPSI per SUPI |
| **Access & Mobility** | `am_subscription`, `am_policy`, `operator_specific` | Full AM data lifecycle, operator-specific data |
| **Session Management** | `sm_subscription`, `sm_policy`, `dnn_config` | Per-slice SM data, DNN configuration |
| **NSSAI** | `nssai_subscription`, `nssai_mapping` | Slice subscription management |
| **SMS** | `sms_subscription`, `sms_management` | SMS service configuration |
| **UE Context** | `ue_context_in_amf`, `ue_context_in_smf`, `sdm_subscription` | Context registration, SDM subscriptions |
| **Event Exposure** | `ee_subscription`, `ee_profile`, `monitoring_config` | Event subscription lifecycle |
| **Shared Data** | `shared_data`, `shared_data_mapping` | Shared data resolution, mapping consistency |
| **Keys** | `home_network_key`, `sidf_key` | SIDF key retrieval, key rotation |

#### 2.2.4 Transaction Isolation Testing

```go
func TestTransactionIsolation_ConcurrentSQNUpdate(t *testing.T) {
    pool := setupTestDB(t)

    // Seed subscriber with initial SQN
    seedSubscriber(t, pool, "imsi-001010000000001", initialSQN)

    // Attempt concurrent SQN updates — only one should succeed
    var wg sync.WaitGroup
    results := make([]error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            results[idx] = updateSQNWithRetry(pool, "imsi-001010000000001", nextSQN(idx))
        }(i)
    }
    wg.Wait()

    // Verify exactly one final SQN value and no lost updates
    finalSQN := getSQN(t, pool, "imsi-001010000000001")
    assert.NotEqual(t, initialSQN, finalSQN)
}
```

#### 2.2.5 Connection Pool Behavior Under Load

| Test | Description | Success Criteria |
|------|-------------|-----------------|
| `TestPool_MaxConnections` | Open max connections, verify next request queues | No connection errors; request completes after release |
| `TestPool_IdleTimeout` | Hold idle connections, verify reclamation | Pool size returns to minimum after idle timeout |
| `TestPool_Exhaustion` | Exceed max pool size under load | Requests queue and complete within timeout; no panics |
| `TestPool_HealthCheck` | Kill DB, verify health check detects failure | Pool marks connections as stale; reconnects on recovery |

### 2.3 API Testing

API tests validate all **103 Nudm endpoints** across the 10 microservices against the
3GPP-defined OpenAPI specifications.

#### 2.3.1 HTTP/2 Endpoint Testing

All SBI endpoints are tested over **HTTP/2** (mandatory per 3GPP TS 29.500):

```go
func TestNudmSDM_GetAMData_HTTP2(t *testing.T) {
    client := &http.Client{
        Transport: &http2.Transport{
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        },
    }

    resp, err := client.Get("https://localhost:8443/nudm-sdm/v2/imsi-001010000000001/am-data")
    require.NoError(t, err)
    defer resp.Body.Close()

    assert.Equal(t, "HTTP/2.0", resp.Proto)
    assert.Equal(t, http.StatusOK, resp.StatusCode)

    var amData AccessAndMobilitySubscriptionData
    err = json.NewDecoder(resp.Body).Decode(&amData)
    assert.NoError(t, err)
    assert.NotEmpty(t, amData.Nssai)
}
```

#### 2.3.2 Request/Response Schema Validation

Every API response is validated against the corresponding 3GPP OpenAPI YAML specification:

| Service | Spec | Endpoints | Validation Approach |
|---------|------|-----------|---------------------|
| Nudm_UEAU | TS 29.503 | 8 | JSON Schema from `TS29503_Nudm_UEAU.yaml` |
| Nudm_SDM | TS 29.503 | 32 | JSON Schema from `TS29503_Nudm_SDM.yaml` |
| Nudm_UECM | TS 29.503 | 18 | JSON Schema from `TS29503_Nudm_UECM.yaml` |
| Nudm_EE | TS 29.503 | 12 | JSON Schema from `TS29503_Nudm_EE.yaml` |
| Nudm_PP | TS 29.503 | 6 | JSON Schema from `TS29503_Nudm_PP.yaml` |
| Nudm_NIDDAU | TS 29.503 | 4 | JSON Schema from `TS29503_Nudm_NIDDAU.yaml` |
| Nudm_MT | TS 29.503 | 5 | JSON Schema from `TS29503_Nudm_MT.yaml` |
| Nudm_SDM (shared) | TS 29.503 | 8 | JSON Schema from shared data specs |
| Nudm_RSDS | TS 29.503 | 5 | JSON Schema from `TS29503_Nudm_RSDS.yaml` |
| Nudm_SSAU | TS 29.503 | 5 | JSON Schema from `TS29503_Nudm_SSAU.yaml` |

The test framework loads OpenAPI specs at test initialization and uses `openapi3filter` to validate
every request and response body, headers, and status codes.

#### 2.3.3 Error Code Correctness (ProblemDetails)

All error responses must conform to **RFC 7807** `ProblemDetails` format as mandated by
3GPP TS 29.500:

```go
func TestErrorResponse_ProblemDetails(t *testing.T) {
    tests := []struct {
        name       string
        path       string
        wantStatus int
        wantCause  string
    }{
        {
            name:       "subscriber not found",
            path:       "/nudm-sdm/v2/imsi-999990000000001/am-data",
            wantStatus: http.StatusNotFound,
            wantCause:  "USER_NOT_FOUND",
        },
        {
            name:       "invalid query parameter",
            path:       "/nudm-sdm/v2/imsi-001010000000001/am-data?plmn-id=invalid",
            wantStatus: http.StatusBadRequest,
            wantCause:  "INVALID_QUERY_PARAM",
        },
        {
            name:       "unsupported media type",
            path:       "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
            wantStatus: http.StatusUnsupportedMediaType,
            wantCause:  "UNSUPPORTED_MEDIA_TYPE",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            resp := doRequest(t, tt.path)
            assert.Equal(t, tt.wantStatus, resp.StatusCode)

            var problem ProblemDetails
            err := json.NewDecoder(resp.Body).Decode(&problem)
            assert.NoError(t, err)
            assert.Equal(t, tt.wantCause, problem.Cause)
            assert.NotEmpty(t, problem.Title)
            assert.Equal(t, tt.wantStatus, problem.Status)
        })
    }
}
```

#### 2.3.4 OAuth2 Token Validation

SBI endpoints enforce **OAuth2 bearer token** validation as required by 3GPP TS 33.501:

| Test Case | Description |
|-----------|-------------|
| `TestOAuth2_ValidToken` | Request with valid NF token succeeds |
| `TestOAuth2_ExpiredToken` | Request with expired token returns 401 |
| `TestOAuth2_InvalidScope` | Token without required scope returns 403 |
| `TestOAuth2_MissingToken` | Request without Authorization header returns 401 |
| `TestOAuth2_MalformedToken` | Malformed bearer token returns 401 |
| `TestOAuth2_WrongAudience` | Token with wrong NF audience returns 403 |

### 2.4 Database Testing

#### 2.4.1 Replication Correctness

Tests verify that writes to one YugabyteDB node are consistently visible across all replicas:

| Test | Description | Success Criteria |
|------|-------------|-----------------|
| `TestReplication_WriteReadConsistency` | Write to leader, read from follower | Data matches within replication lag SLA |
| `TestReplication_StrongConsistency` | Write with strong consistency, read immediately | Immediate visibility on all nodes |
| `TestReplication_ConflictResolution` | Concurrent writes to same key from different nodes | Consistent conflict resolution, no data corruption |

#### 2.4.2 Consistency Verification Across Regions

Multi-region consistency is validated with a dedicated test harness that deploys a 3-region
YugabyteDB cluster:

- **Geo-partitioned reads** — Verify subscriber data is served from the nearest region.
- **Cross-region writes** — Verify write propagation latency stays within configured bounds.
- **Quorum verification** — Verify reads return committed data after leader failover.

#### 2.4.3 Failover Behavior Testing

| Scenario | Procedure | Expected Behavior |
|----------|-----------|-------------------|
| Single node failure | Kill one YugabyteDB tserver | Raft re-elects leader within 3 seconds; no query failures |
| Master failure | Kill YugabyteDB master leader | New master elected; DDL operations resume within 10 seconds |
| Region failure | Network-partition an entire region | Remaining regions continue serving; RPO = 0 for sync replication |
| Recovery after failure | Restart failed node | Node rejoins cluster; catches up via Raft log replay |

#### 2.4.4 Schema Migration Rollback

Every migration script is tested for safe rollback:

```
migration_001_create_auth_tables.up.sql   → Apply
migration_001_create_auth_tables.down.sql → Rollback
Verify: no residual objects, no data loss in unrelated tables
```

Rollback tests are mandatory for any migration that modifies existing columns or drops objects.
Additive-only migrations (new tables, new columns with defaults) may have no-op down migrations
but must still be tested for idempotency.

---

## 3. Telecom Protocol Testing

### 3.1 HTTP/2 SBI Validation

All Service-Based Interface (SBI) communication uses HTTP/2 as mandated by 3GPP TS 29.500 §5.2.
Tests validate protocol-level correctness:

| Test | Description |
|------|-------------|
| `TestHTTP2_Framing` | Verify proper DATA, HEADERS, and SETTINGS frames |
| `TestHTTP2_Multiplexing` | Multiple concurrent streams on a single connection |
| `TestHTTP2_HeaderCompression` | HPACK compression of 3GPP custom headers |
| `TestHTTP2_FlowControl` | Server-side flow control under back-pressure |
| `TestHTTP2_GOAWAY` | Graceful connection draining on server shutdown |
| `TestHTTP2_ServerPush` | Verify server push is disabled (not used in SBI) |
| `TestHTTP2_MaxConcurrentStreams` | Validate SETTINGS_MAX_CONCURRENT_STREAMS enforcement |

### 3.2 5G Signaling Correctness

Signaling tests verify that UDM message flows conform to 3GPP specifications:

- **TS 29.503** — Nudm service operations and information flows
- **TS 29.505** — Subscription data schema and structure
- **TS 33.501** — Security architecture and authentication procedures
- **TS 23.502** — Procedures for the 5G System (reference flows)

Each signaling test validates the complete request-response exchange including mandatory headers,
body schema, and correct HTTP method and status code per the specification.

### 3.3 Custom 3GPP Header Handling

3GPP defines custom HTTP headers (specified in `TS29500_CustomHeaders.abnf`) that must be
correctly handled:

| Header | Purpose | Test Coverage |
|--------|---------|---------------|
| `3gpp-Sbi-Target-apiRoot` | API root for indirect communication | Parse, validate, forward |
| `3gpp-Sbi-Callback` | Callback URI for notifications | URI validation, storage |
| `3gpp-Sbi-Binding` | Service binding indication | Binding context preservation |
| `3gpp-Sbi-Discovery-*` | NRF discovery delegation headers | Parameter extraction, forwarding |
| `3gpp-Sbi-Oci` | Overload control information | OCI parsing, load shedding response |
| `3gpp-Sbi-Message-Priority` | Message priority for congestion control | Priority extraction, queue ordering |

### 3.4 Content-Type Negotiation

Tests verify correct content-type handling for all supported media types:

| Content-Type | Usage | Test |
|-------------|-------|------|
| `application/json` | Default for all request/response bodies | Standard JSON serialization |
| `application/3gppHal+json` | Responses with hypermedia links | HAL link structure validation |
| `application/problem+json` | Error responses (RFC 7807) | ProblemDetails schema compliance |
| `multipart/related` | Bundled responses | Part boundary parsing, content-ID resolution |

Content negotiation tests also validate the `Accept` header handling — the server must return
`406 Not Acceptable` when the client requests an unsupported media type.

### 3.5 Response Code Compliance

Every endpoint is tested against the exact set of response codes defined in the corresponding
OpenAPI specification:

- **Success codes**: `200 OK`, `201 Created`, `204 No Content` — used where specified
- **Redirect codes**: `307 Temporary Redirect`, `308 Permanent Redirect` — for NF redirection
- **Client error codes**: `400`, `401`, `403`, `404`, `409`, `411`, `413`, `415`, `429`
- **Server error codes**: `500`, `503`

Tests ensure that no endpoint returns a status code not documented in its OpenAPI definition.

---

## 4. 5G Telecom Scenario Testing

End-to-end telecom scenario tests validate complete 5G procedures that involve the UDM. These
tests simulate real NF-to-NF interactions (AMF→AUSF→UDM) and verify the UDM's behavior as part
of larger 5G Core flows.

### 4.1 UE Registration (5G Attach)

#### 4.1.1 Full Registration Flow

The registration test simulates the complete AMF→AUSF→UDM call chain:

1. **AMF** sends `Nausf_UEAuthentication_Authenticate` to AUSF
2. **AUSF** calls `Nudm_UEAuthentication_GenerateAuthData` on UDM
3. **UDM** generates auth vectors, returns to AUSF
4. **AUSF** completes authentication, AMF calls `Nudm_UECM_Registration`
5. **AMF** retrieves subscription data via `Nudm_SDM_Get`
6. **AMF** subscribes to data changes via `Nudm_SDM_Subscribe`

| Scenario | Description | Key Assertions |
|----------|-------------|----------------|
| Initial Registration | First-time UE attach to network | Full auth + data retrieval flow completes |
| Mobility Registration | UE moves between AMFs | Old AMF context deregistered, new AMF registered |
| Periodic Registration | Periodic registration timer expiry | Context refreshed without full re-authentication |
| Emergency Registration | Emergency services attach | Reduced data retrieval, no authentication required |

#### 4.1.2 Registration Edge Cases

- UE registration with no subscription data → `404 USER_NOT_FOUND`
- Concurrent registrations from same UE → Serialized processing, latest wins
- Registration during database failover → Retry with backoff, eventual success

### 4.2 Subscriber Authentication (5G AKA)

#### 4.2.1 5G-HE-AKA Flow Validation

Tests validate the complete **5G Home Environment Authentication and Key Agreement** flow:

1. AUSF requests auth vectors → UDM computes `RAND`, `AUTN`, `XRES*`, `KAUSF`
2. UDM increments sequence number (SQN) in the database
3. AUSF sends auth confirmation → UDM stores auth result

| Test | Description |
|------|-------------|
| `TestAKA_SuccessfulAuth` | Full successful 5G-HE-AKA round trip |
| `TestAKA_AuthFailure_MacMismatch` | UE sends incorrect MAC → auth rejected |
| `TestAKA_Resync` | UE sends AUTS for SQN resynchronization → UDM recalculates |
| `TestAKA_ConcurrentAuthSameUE` | Parallel auth requests for same SUPI → serialized correctly |

#### 4.2.2 EAP-AKA' Flow Validation

| Test | Description |
|------|-------------|
| `TestEAPAKA_FullFlow` | Complete EAP-AKA' exchange |
| `TestEAPAKA_ReauthId` | Re-authentication identity handling |
| `TestEAPAKA_FastReauth` | Fast re-authentication procedure |

#### 4.2.3 Auth Vector Generation Correctness

Cryptographic correctness tests use **known-answer test vectors** from 3GPP TS 33.501 Annex A:

- Verify RAND generation entropy (NIST SP 800-22 randomness)
- Verify AUTN computation from SQN, AK, AMF, MAC
- Verify XRES* derivation using HMAC-SHA-256
- Verify KAUSF derivation using the correct KDF (TS 33.501 Annex A)

#### 4.2.4 Resynchronization Handling

When a UE detects a sequence number mismatch, it sends an AUTS parameter. The test verifies:

1. UDM extracts SQN from AUTS using the subscriber's key
2. UDM validates the received SQN against the stored SQN
3. UDM updates the SQN in the database
4. UDM generates a fresh auth vector with the corrected SQN

#### 4.2.5 Auth Confirmation Flow

After successful authentication, the AUSF confirms the result with the UDM:

- `Nudm_UEAuthentication_ResultConfirmation` with `authResult=SUCCESS`
- UDM stores the authentication timestamp and serving network
- UDM makes the auth event available for event exposure subscribers

### 4.3 SUPI/SUCI Handling

#### 4.3.1 SUCI Deconceal Correctness

| Protection Scheme | Test Description |
|-------------------|------------------|
| Null scheme (0) | SUCI contains cleartext MSIN — direct extraction |
| Profile A (1) | ECIES with Curve25519 — decrypt and extract MSIN |
| Profile B (2) | ECIES with secp256r1 — decrypt and extract MSIN |
| Operator-defined | Operator-specific scheme — plugin-based deconceal |

#### 4.3.2 SUPI Format Validation

- `imsi-<MCC><MNC><MSIN>` — Standard IMSI-based SUPI
- `nai-<user>@<realm>` — NAI-based SUPI for non-3GPP access
- Invalid formats → `400 Bad Request` with `INVALID_QUERY_PARAM` cause

#### 4.3.3 GPSI Resolution

- Map SUPI to GPSI (`msisdn-<number>`, `extid-<external_id>`)
- Map GPSI to SUPI (reverse lookup)
- Multiple GPSIs per SUPI — return all or filtered by type

### 4.4 Subscription Data Retrieval and Update

#### 4.4.1 All Data Types

| Data Type | Endpoint | Test Coverage |
|-----------|----------|---------------|
| Access & Mobility (AM) | `GET /nudm-sdm/v2/{supi}/am-data` | Full AM data, per-PLMN filtering |
| Session Management (SM) | `GET /nudm-sdm/v2/{supi}/sm-data` | Per-S-NSSAI, per-DNN filtering |
| NSSAI | `GET /nudm-sdm/v2/{supi}/nssai` | Configured NSSAI, PLMN-specific |
| SMS | `GET /nudm-sdm/v2/{supi}/sms-data` | SMS subscription configuration |
| SMS Management | `GET /nudm-sdm/v2/{supi}/sms-mng-data` | SMS management subscription |
| UE Context in SMF | `GET /nudm-sdm/v2/{supi}/ue-context-in-smf-data` | Active PDU sessions |
| Trace Data | `GET /nudm-sdm/v2/{supi}/trace-data` | Trace activation configuration |
| Operator-Specific | `GET /nudm-sdm/v2/{supi}/operator-specific-data` | Operator-defined data blobs |

#### 4.4.2 Shared Data Resolution

Shared data allows multiple subscribers to reference common subscription profiles:

- Resolve `sharedDataId` references to actual shared data objects
- Merge shared data with individual subscriber overrides
- Verify correct precedence: individual data overrides shared data

#### 4.4.3 Partial Updates (PATCH)

`PATCH` operations use **JSON Merge Patch (RFC 7396)** or **JSON Patch (RFC 6902)**:

| Test | Description |
|------|-------------|
| `TestPatch_MergePatch_SingleField` | Update one field, others unchanged |
| `TestPatch_MergePatch_NullRemoval` | Set field to null to remove it |
| `TestPatch_JSONPatch_Add` | Add new field via JSON Patch |
| `TestPatch_JSONPatch_Replace` | Replace existing field via JSON Patch |
| `TestPatch_ConcurrentPatches` | Concurrent patches to same subscriber |

#### 4.4.4 Change Notification Delivery

When subscription data changes, the UDM must notify subscribed NFs:

1. NF subscribes via `Nudm_SDM_Subscribe` → UDM stores subscription
2. Data changes via provisioning or PATCH → UDM detects change
3. UDM sends `Nudm_SDM_Notification` to callback URI → NF receives update
4. NF acknowledges → UDM marks notification as delivered

Tests validate notification reliability including retry on failure, subscription expiry, and
notification filtering (only changed data types trigger notifications).

### 4.5 Roaming Scenarios

#### 4.5.1 Home-Routed Roaming

All traffic routes through the home PLMN:

- Visited AMF contacts home UDM via SEPP/N32 interface
- UDM returns full subscription data including home-routed indicators
- Verify correct handling of `vplmnId` parameter in requests

#### 4.5.2 Local Breakout Roaming

Data traffic breaks out in the visited PLMN:

- UDM returns subscription data with local breakout authorization
- Per-S-NSSAI roaming allowed/not-allowed indicators
- Session continuity mode indicators for handover

#### 4.5.3 Visited PLMN Context Handling

| Test | Description |
|------|-------------|
| `TestRoaming_VisitedPLMNId` | Correct PLMN-specific data returned for visited network |
| `TestRoaming_RoamingNotAllowed` | Subscriber without roaming entitlement → `403 ROAMING_NOT_ALLOWED` |
| `TestRoaming_PLMNRestriction` | Subscriber restricted to specific visited PLMNs |
| `TestRoaming_CoreNetworkTypeRestriction` | 5GC vs EPC roaming restrictions |

#### 4.5.4 SoR (Steering of Roaming) Procedures

- UDM generates SoR information with preferred PLMN list
- SoR transparent container is correctly signed
- SoR acknowledgement from UE is processed
- SoR counter management and anti-replay protection

### 4.6 Network Slicing

#### 4.6.1 Per-Slice Subscription Data Retrieval

- Retrieve subscription data filtered by `singleNssai` (SST + SD)
- Verify correct data isolation between slices
- Default slice assignment when UE does not indicate preferred slice

#### 4.6.2 S-NSSAI Validation

| Test | Description |
|------|-------------|
| `TestSlice_ValidSNSSAI` | Valid SST/SD combination → data returned |
| `TestSlice_InvalidSST` | SST outside allowed range → `400 Bad Request` |
| `TestSlice_UnsubscribedSlice` | Subscriber not entitled to requested slice → `404` |
| `TestSlice_MappedSNSSAI` | Visited-to-home S-NSSAI mapping applied correctly |

#### 4.6.3 Slice-Specific AM Data

- Access & Mobility data varies per slice — verify correct AM data returned for each S-NSSAI
- DNN authorization per slice
- QoS profile per slice
- Slice-specific authentication and authorization (NSSAA) indications

---

## 5. Performance and Resilience Testing

### 5.1 Load Testing

#### 5.1.1 Performance Targets

| Metric | Target | Measurement Point |
|--------|--------|-------------------|
| Sustained throughput | 50,000 TPS | Aggregate across all Nudm endpoints |
| Registration burst | 10,000 registrations/sec | `Nudm_UECM_Registration` endpoint |
| Auth burst | 20,000 auth requests/sec | `Nudm_UEAU_GenerateAuthData` endpoint |
| P50 latency | < 5 ms | End-to-end request processing |
| P99 latency | < 25 ms | End-to-end request processing |
| P99.9 latency | < 100 ms | End-to-end request processing |
| Error rate | < 0.01% | Under sustained load at target TPS |

#### 5.1.2 Load Testing Tools

| Tool | Purpose | Usage |
|------|---------|-------|
| **k6** | Scripted load scenarios with ramp-up/down | Primary tool for API load tests |
| **Vegeta** | Constant-rate HTTP load generation | Burst testing and rate-limited scenarios |
| **Custom Go load generator** | 5G-specific traffic patterns | Realistic subscriber behavior simulation |

#### 5.1.3 Traffic Profiles

| Profile | TPS | Duration | Subscriber Mix |
|---------|-----|----------|----------------|
| **Busy hour** | 50,000 | 1 hour | 40% auth, 30% SDM, 20% UECM, 10% other |
| **Normal** | 20,000 | 8 hours | 20% auth, 40% SDM, 30% UECM, 10% other |
| **Low traffic** | 2,000 | 8 hours | 10% auth, 50% SDM, 30% UECM, 10% other |
| **Registration storm** | 10,000 | 10 min | 80% registration, 20% auth |
| **Morning ramp** | 0→50,000 | 30 min | Gradual increase simulating morning peak |

### 5.2 Stress Testing

Stress tests push the system beyond its designed capacity to identify breaking points and
verify graceful degradation:

| Test | Procedure | Expected Behavior |
|------|-----------|-------------------|
| **TPS overload** | Ramp to 2× target TPS (100,000) | Graceful degradation: latency increases, no crashes |
| **Connection exhaustion** | Open 10,000 concurrent connections | Connection queue fills; new connections wait or are rejected with 503 |
| **Memory pressure** | Trigger large response payloads repeatedly | GC handles pressure; OOM kill does not occur |
| **CPU saturation** | Compute-intensive auth requests at max rate | Throughput plateaus; no request corruption |
| **DB connection pool exhaustion** | Exceed max DB pool size | Requests queue; timeout produces 503, not 500 |

Graceful degradation is verified by checking that:

- No request receives a corrupted or incorrect response
- The system returns `503 Service Unavailable` with proper `Retry-After` headers
- The system recovers to normal operation within 60 seconds after load subsides
- No data is lost or corrupted in the database

### 5.3 Soak Testing

Soak tests run continuous load for extended periods to detect slow-developing issues:

| Parameter | Value |
|-----------|-------|
| **Duration** | 72 hours |
| **Load level** | 70% of target TPS (35,000 TPS) |
| **Subscriber pool** | 1,000,000 synthetic subscribers |
| **Monitoring interval** | Metrics sampled every 10 seconds |

#### 5.3.1 Monitored Metrics

| Metric | Alert Threshold | Description |
|--------|-----------------|-------------|
| Heap memory | > 20% growth over 24h | Indicates potential memory leak |
| Goroutine count | > 10% growth over 24h | Indicates goroutine leak |
| DB connection count | Exceeds pool max | Connection leak |
| P99 latency | > 50% increase from baseline | Performance degradation |
| Error rate | > 0.01% | Reliability degradation |
| GC pause time | > 10 ms P99 | Excessive GC pressure |
| Open file descriptors | > 80% of ulimit | File descriptor leak |

#### 5.3.2 Memory Leak Detection

- **Go pprof** profiles captured every hour for heap, goroutine, and mutex analysis
- Baseline comparison: hour-1 vs hour-72 heap profiles must show no monotonic growth
- Object allocation tracking for critical types (auth vectors, subscription data)

#### 5.3.3 Connection Pool Stability

- DB pool size must remain bounded within configured min/max
- Idle connections are reaped according to the configured idle timeout
- No connection leaks after 72 hours (verified via YugabyteDB `pg_stat_activity`)

#### 5.3.4 GC Pressure Monitoring

- GC pause time P99 must stay below 10 ms
- GC frequency must remain stable (not increasing over time)
- `GOGC` tuning is validated under sustained load

### 5.4 Chaos Engineering

Chaos tests inject faults into the running system to validate resilience and recovery.

#### 5.4.1 Fault Injection Scenarios

| Fault | Tool | Duration | Expected Behavior |
|-------|------|----------|-------------------|
| **Kill random pods** | Chaos Mesh (PodChaos) | Random intervals | Kubernetes restarts pod; requests fail over to healthy pods |
| **DB network partition** | Chaos Mesh (NetworkChaos) | 30 seconds | Raft leader re-election; brief query failures then recovery |
| **Region outage** | Litmus Chaos | 5 minutes | Traffic shifts to surviving regions; RPO = 0 |
| **Clock skew** | Chaos Mesh (TimeChaos) | 5 minutes | HLC (Hybrid Logical Clock) handles skew; no data corruption |
| **DNS failure** | Chaos Mesh (DNSChaos) | 60 seconds | Cached DNS entries used; service discovery retries |
| **CPU stress** | Chaos Mesh (StressChaos) | 2 minutes | Throughput decreases; no request corruption |
| **IO latency** | Chaos Mesh (IOChaos) | 2 minutes | DB query latency increases; request timeouts handled |

#### 5.4.2 Recovery Validation

After each fault injection, the system must:

1. **Detect** the failure within the configured health check interval
2. **Mitigate** by routing traffic away from the affected component
3. **Recover** to full capacity within the SLA (typically < 30 seconds for pod failures,
   < 60 seconds for node failures, < 5 minutes for region failures)
4. **Verify** data integrity post-recovery — no lost writes, no stale reads

#### 5.4.3 Chaos Testing Tools

| Tool | Scope | Integration |
|------|-------|-------------|
| **Chaos Mesh** | Kubernetes-native fault injection | Deployed as CRDs; tests defined as YAML manifests |
| **Litmus Chaos** | Complex multi-step chaos workflows | Used for region-level failure scenarios |
| **Custom Go fault injector** | Application-level fault injection | Inject errors into specific code paths via feature flags |

---

## 6. Test Environments

| Environment | Purpose | YugabyteDB Topology | Kubernetes | Data Scale |
|-------------|---------|---------------------|------------|------------|
| **Dev** | Developer local testing | Single-node (Docker) | Local (kind/minikube) | 1K subscribers |
| **Integration** | Automated integration tests | 3-node RF=3 | Shared cluster | 10K subscribers |
| **Staging** | Pre-production validation | Multi-region (3 regions, 9 nodes) | Production-like | 1M subscribers |
| **Performance Lab** | Load and stress testing | Multi-region (dedicated hardware) | Dedicated cluster | 10M subscribers |

### 6.1 Environment Specifications

#### Dev Environment

- **Purpose**: Fast feedback loop for developers
- **Provisioning**: `docker-compose up` or `make dev-env`
- **YugabyteDB**: Single `yugabyted` node in Docker
- **Kubernetes**: `kind` cluster with all 10 Nudm services deployed
- **Data**: Seeded with 1K synthetic subscribers via `make seed-dev`
- **Tests run**: Unit tests, single-service integration tests

#### Integration Environment

- **Purpose**: Cross-service integration and API conformance
- **Provisioning**: Terraform + Helm (automated via CI)
- **YugabyteDB**: 3-node cluster with replication factor 3
- **Kubernetes**: Shared namespace in team cluster
- **Data**: 10K subscribers, all data types populated
- **Tests run**: Integration tests, API conformance, schema migration

#### Staging Environment

- **Purpose**: Production-realistic validation before release
- **Provisioning**: Same IaC as production with reduced scale
- **YugabyteDB**: 3 regions × 3 nodes, geo-partitioned
- **Kubernetes**: Multi-cluster with cross-region service mesh
- **Data**: 1M subscribers with realistic distribution across regions
- **Tests run**: Full telecom scenario suite, roaming scenarios, multi-region failover

#### Performance Lab

- **Purpose**: Performance benchmarking, stress testing, soak testing
- **Provisioning**: Dedicated bare-metal or reserved cloud instances
- **YugabyteDB**: Production-equivalent topology with dedicated IOPS
- **Kubernetes**: Dedicated cluster with resource guarantees (no noisy neighbors)
- **Data**: 10M subscribers for realistic scale
- **Tests run**: Load tests, stress tests, 72-hour soak tests, chaos engineering

---

## 7. CI/CD Integration

### 7.1 Pipeline Stages

```
PR Opened          Merge to main       Nightly             Weekly              Monthly
─────────────      ─────────────       ───────             ──────              ───────
│                  │                   │                   │                   │
├─ Lint            ├─ Integration      ├─ Full Telecom     ├─ Load Tests       ├─ Chaos
│  (golangci-lint) │  Tests            │  Scenario Suite   │  (all profiles)   │  Tests
│                  │  (Testcontainers) │                   │                   │
├─ Unit Tests      │                   ├─ Multi-region     ├─ Stress Tests     ├─ Region
│  (go test)       ├─ API Conformance  │  DB Tests         │                   │  Failover
│                  │  Tests            │                   ├─ Soak Test        │
├─ Coverage        │                   │                   │  (72h)            │
│  Check (≥80%)    ├─ DB Migration     │                   │                   │
│                  │  Tests            │                   │                   │
├─ Build           │                   │                   │                   │
│  (go build)      ├─ Container Build  │                   │                   │
│                  │  + Push           │                   │                   │
└─ Security Scan   └─ Deploy to        └─ Report           └─ Report           └─ Report
   (govulncheck)      Staging             Generation          Generation          Generation
```

### 7.2 Test Execution Rules

| Test Category | Trigger | Blocking | Timeout | Retry |
|---------------|---------|----------|---------|-------|
| Lint + Static Analysis | PR opened/updated | Yes | 5 min | No |
| Unit Tests | PR opened/updated | Yes | 10 min | No |
| Integration Tests | Merge to `main` | Yes | 30 min | 1 retry |
| API Conformance | Merge to `main` | Yes | 20 min | 1 retry |
| DB Migration Tests | Merge to `main` | Yes | 15 min | No |
| Telecom Scenarios | Nightly (02:00 UTC) | Advisory | 2 hours | No |
| Load Tests | Weekly (Sunday 00:00 UTC) | Advisory | 4 hours | No |
| Stress Tests | Weekly (Sunday 06:00 UTC) | Advisory | 2 hours | No |
| Soak Tests | Weekly (Saturday 00:00 UTC) | Advisory | 76 hours | No |
| Chaos Tests | Monthly (1st Sunday) | Advisory | 6 hours | No |

### 7.3 Failure Handling

- **Blocking failures**: PR cannot merge; developer must fix and re-run.
- **Advisory failures**: Results posted to Slack `#udm-test-results` channel. On-call engineer
  triages within 24 hours. Performance regressions > 10% trigger a blocking investigation.
- **Flaky test policy**: Tests that fail intermittently are quarantined (tagged `@flaky`),
  investigated within one sprint, and either fixed or removed.

---

## 8. Test Data Management

### 8.1 Synthetic Subscriber Generation

Test data is generated using data factories that produce realistic subscriber profiles at
various scales:

| Scale | Subscribers | Use Case | Generation Time |
|-------|-------------|----------|-----------------|
| **10K** | 10,000 | Dev/Integration testing | < 1 minute |
| **100K** | 100,000 | API conformance, staging smoke | < 5 minutes |
| **1M** | 1,000,000 | Staging full validation | < 30 minutes |
| **10M** | 10,000,000 | Performance lab benchmarking | < 2 hours |

### 8.2 Data Factories

Data factories produce subscriber profiles for all data types and subscriber categories:

| Factory | Output | Configuration |
|---------|--------|---------------|
| `AuthSubscriptionFactory` | Auth credentials (K, OPc, SQN, AMF) | Key algorithm, auth method |
| `AMSubscriptionFactory` | Access & Mobility data | NSSAI, RAT restrictions, area restrictions |
| `SMSubscriptionFactory` | Session Management data | DNN list, QoS profiles, SSC modes |
| `IdentityFactory` | SUPI, GPSI, SUCI mappings | IMSI ranges, MSISDN ranges, protection schemes |
| `RoamingFactory` | Roaming subscriber profiles | Home PLMN, allowed visited PLMNs, SoR config |
| `SliceFactory` | Slice-specific subscription data | S-NSSAI list, per-slice policies |
| `EventExposureFactory` | EE subscription data | Monitoring types, reporting periods |

### 8.3 Subscriber Type Distribution

Generated data follows a realistic distribution to ensure meaningful test results:

| Subscriber Type | Proportion | Characteristics |
|-----------------|------------|-----------------|
| Consumer (postpaid) | 60% | Standard AM/SM data, 1-2 slices |
| Consumer (prepaid) | 20% | Limited data, single slice |
| IoT device | 10% | NAI-based SUPI, limited services |
| Enterprise | 7% | Multiple slices, enhanced QoS |
| Roaming | 3% | Visited PLMN contexts, SoR data |

### 8.4 Production Data Anonymization

When production data is used for debugging or performance baseline validation, it must be
anonymized before use in non-production environments:

| Field | Anonymization Method |
|-------|---------------------|
| IMSI (SUPI) | Replace MSIN with sequential numbers preserving MCC/MNC |
| MSISDN (GPSI) | Replace with synthetic numbers in non-routable ranges |
| Authentication keys (K, OPc) | Replace with randomly generated keys |
| IP addresses | Replace with RFC 5737 documentation ranges (192.0.2.0/24) |
| Subscriber names | Replace with synthetic names from a dictionary |
| Location data | Generalize to region level (no cell-level precision) |

Anonymization is enforced via a pipeline step that processes SQL dumps through a dedicated
anonymization tool before loading into non-production YugabyteDB instances. The anonymization
process is itself tested to verify no PII leakage.

### 8.5 Test Data Lifecycle

- **Creation**: Generated fresh for each CI run (integration, API tests) to avoid test
  interdependencies.
- **Seeding**: Performance and staging environments are seeded once per release cycle and
  refreshed on demand.
- **Cleanup**: Dev and integration environments are ephemeral — data is destroyed with the
  test container. Staging data is retained for debugging but rotated monthly.
- **Versioning**: Data factory configurations are version-controlled alongside test code to
  ensure reproducibility.

---

*For architecture context, see [architecture.md](architecture.md). For API specifications, see
[sbi-api-design.md](sbi-api-design.md). For database schema details, see
[data-model.md](data-model.md). For sequence diagrams of the procedures tested here, see
[sequence-diagrams.md](sequence-diagrams.md).*
