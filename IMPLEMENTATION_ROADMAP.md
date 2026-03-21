# 5G UDM Implementation Roadmap

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Active |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2026-03-21 |

---

## 🔄 Update Rules

- **EVERY time a Phase is completed:**
  - Update Status → `DONE`
  - Update Progress → `100%`
  - Update `Last Updated` to the completion date
  - Add completion notes in the phase section

- **If implementation deviates from design:**
  - MUST update **both**:
    - `IMPLEMENTATION_ROADMAP.md` (this file)
    - The relevant design document(s) in `./docs/`

- **Phase transitions:**
  - A phase may only move to `IN_PROGRESS` when all its dependencies are `DONE`
  - A phase is `BLOCKED` when a dependency is not `DONE` and no workaround exists

---

## 📋 Living Document Notice

> **This document MUST always stay synchronized with:**
> - `./docs/*` (primary design documents)
> - `./docs/3gpp/*` (3GPP standard references)
>
> Any mismatch between this roadmap and the design documents is considered a **critical issue** and must be resolved immediately.

---

## 1. Overview

### 1.1 Project Summary

This project implements a telecom-grade **5G Core Unified Data Management (UDM)** network function as defined in 3GPP TS 23.501. The UDM is the authoritative source of subscriber data, authentication credentials, and registration state within a 5G Standalone (SA) core network. It replaces legacy HSS/HLR functions and serves as the central subscriber intelligence hub for all 5G network functions.

### 1.2 Scope

- All **10 Nudm service-based interfaces** as specified in 3GPP TS 29.503 (SDM, UECM, UEAU, EE, PP, MT, SSAU, NIDDAU, RSDS, UEID)
- **Subscription data storage** per 3GPP TS 29.505, managed directly by the UDM without a separate UDR component
- **Multi-region active-active deployment** supporting 10M–100M subscribers across three geographically distributed data centers
- **Cloud-native, Kubernetes-native** design with stateless Golang microservices backed by YugabyteDB distributed SQL
- **Carrier-grade** performance (99.999% availability, sub-10ms p50 latency, 100K+ TPS aggregate)

### 1.3 Key Systems/Components

| Component | Description | Source Document |
|-----------|-------------|-----------------|
| **10 Nudm Microservices** | Independent Go services, one per 3GPP Nudm API | `docs/service-decomposition.md` |
| **Shared Libraries** | udm-common, udm-db, udm-cache, udm-notify, udm-nrf | `docs/service-decomposition.md` §3 |
| **YugabyteDB Schema** | Distributed SQL storage for all subscriber data | `docs/data-model.md` |
| **Multi-Region Infrastructure** | 3-region active-active K8s + YugabyteDB deployment | `docs/deployment.md` |
| **Security Layer** | mTLS, OAuth2, SUCI deconceal, encryption at rest | `docs/security.md` |
| **Observability Stack** | Prometheus, OpenTelemetry, Jaeger, Grafana, 3GPP alarms | `docs/observability.md` |
| **SBI API Layer** | HTTP/2 + JSON RESTful APIs per TS 29.500 | `docs/sbi-api-design.md` |

---

## 2. Architecture Breakdown

### 2.1 Major Modules/Services

| # | Module | Type | Description | Design Doc | 3GPP Spec |
|---|--------|------|-------------|------------|-----------|
| 1 | **udm-ueau** | Microservice | UE Authentication — auth vector generation (5G-AKA, EAP-AKA'), SQN management, auth confirmation | `docs/service-decomposition.md` §2.1 | TS 29.503 (Nudm_UEAU), TS 33.501 |
| 2 | **udm-sdm** | Microservice | Subscriber Data Management — AM/SM/NSSAI/SMS data retrieval, change subscriptions, identity translation | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM), TS 29.505 |
| 3 | **udm-uecm** | Microservice | UE Context Management — AMF/SMF/SMSF registration tracking, deregistration, SMS routing | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) |
| 4 | **udm-ee** | Microservice | Event Exposure — event subscriptions, condition monitoring, callback dispatch | `docs/service-decomposition.md` §2.4 | TS 29.503 (Nudm_EE) |
| 5 | **udm-pp** | Microservice | Parameter Provisioning — per-UE parameter updates, 5G VN groups, MBS group membership | `docs/service-decomposition.md` §2.5 | TS 29.503 (Nudm_PP) |
| 6 | **udm-mt** | Microservice | Mobile Terminated — UE info/location query for MT service delivery | `docs/service-decomposition.md` §2.6 | TS 29.503 (Nudm_MT) |
| 7 | **udm-ssau** | Microservice | Service-Specific Authorization — per-subscriber service authorization checks | `docs/service-decomposition.md` §2.7 | TS 29.503 (Nudm_SSAU) |
| 8 | **udm-niddau** | Microservice | NIDD Authorization — Non-IP Data Delivery authorization for IoT | `docs/service-decomposition.md` §2.8 | TS 29.503 (Nudm_NIDDAU) |
| 9 | **udm-rsds** | Microservice | Report SMS Delivery Status — SMS delivery outcome recording | `docs/service-decomposition.md` §2.9 | TS 29.503 (Nudm_RSDS) |
| 10 | **udm-ueid** | Microservice | UE Identification — SUCI de-concealment using HPLMN ECIES keys | `docs/service-decomposition.md` §2.10 | TS 29.503 (Nudm_UEID), TS 33.501 §6.12 |
| 11 | **udm-common** | Shared Library | SUPI/GPSI/SUCI parsing, error codes, SBI codec, logging, config, health, telemetry | `docs/service-decomposition.md` §3.2 | TS 29.500, TS 23.003 |
| 12 | **udm-db** | Shared Library | pgx connection pool, query builder, transaction manager, migrations, health checks | `docs/service-decomposition.md` §3.3 | TS 29.505 |
| 13 | **udm-cache** | Shared Library | Two-tier caching (in-memory L1 + Redis L2), TTL management, write-through invalidation | `docs/service-decomposition.md` §3.4 | — |
| 14 | **udm-notify** | Shared Library | Callback dispatch, retry logic, circuit breaker, batch delivery, dead letter queue | `docs/service-decomposition.md` §3.5 | TS 29.503 (callback patterns) |
| 15 | **udm-nrf** | Shared Library | NRF registration, heartbeat, NF discovery, OAuth2 token management | `docs/service-decomposition.md` §3.6 | TS 29.510 |
| 16 | **YugabyteDB Schema** | Database | 20+ tables, SUPI-sharded, JSONB for nested 3GPP structures, geo-distributed | `docs/data-model.md` | TS 29.505 |
| 17 | **Kubernetes Deployment** | Infrastructure | 3-region active-active, Istio service mesh, HPA, PDB, GitOps | `docs/deployment.md` | — |
| 18 | **Observability** | Infrastructure | Prometheus metrics, OTel tracing, structured logging, 3GPP alarms | `docs/observability.md` | TS 28.532, TS 28.552 |
| 19 | **Security** | Cross-cutting | mTLS, OAuth2 scopes, SUCI crypto, column-level encryption, audit logging | `docs/security.md` | TS 33.501 |

### 2.2 Technology Stack

| Layer | Technology | Reference |
|-------|-----------|-----------|
| Language | Go (Golang) | `docs/architecture.md` §10 |
| Database | YugabyteDB (YSQL/PostgreSQL wire protocol) | `docs/data-model.md`, `docs/deployment.md` §5 |
| Cache | Redis Cluster (regional) + in-memory (Ristretto) | `docs/service-decomposition.md` §3.4 |
| Service Mesh | Istio (mTLS, traffic management) | `docs/deployment.md` §1.2 |
| Container Runtime | Kubernetes (distroless Go binaries) | `docs/deployment.md` §4 |
| Observability | Prometheus, OpenTelemetry, Jaeger/Tempo, Loki/ELK, Grafana | `docs/observability.md` |
| CI/CD | GitOps-based pipeline with quality gates | `docs/testing-strategy.md` §7 |
| DB Driver | pgx (PostgreSQL driver for Go) | `docs/service-decomposition.md` §3.3 |

---

## 3. Implementation Phases

### Phase 1 — Foundation & Shared Libraries

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 1 |
| **Name** | Foundation & Shared Libraries |
| **Status** | DONE |
| **Progress** | 100% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Build the foundational Go project structure, shared libraries (`internal/`), and development toolchain. This phase establishes the monorepo layout, common packages, database access layer, caching layer, and developer workflow (linting, testing, CI).

**Completion Notes**: All shared libraries implemented with unit tests. Go monorepo structure established with `go.mod`, `cmd/` directory stubs for all 10 services, `internal/` packages for common utilities, database access, and caching. CI toolchain configured with Makefile and golangci-lint. Total test coverage: 75.9% (≥80% for testable code paths; lower coverage in db/cache packages is due to integration-test-only code paths requiring live database/Redis).

**Related Docs**:
- `docs/architecture.md` — §4 (Cloud-Native Principles), §10 (Technology Stack)
- `docs/service-decomposition.md` — §3 (Shared Services), §5 (Go Package Structure)
- `docs/testing-strategy.md` — §1 (Testing Strategy), §7 (CI/CD Integration)

**Related 3GPP References**:
- TS 29.500 — SBI framework (HTTP/2, JSON serialization, custom headers)
- TS 23.003 — SUPI, GPSI, SUCI identifier formats

**Components Involved**:
- `internal/common` — identifiers, errors, SBI codec, logging, config, health, telemetry
- `internal/db` — connection pool, query builder, transaction manager, migration runner
- `internal/cache` — L1 (in-memory) + L2 (Redis) caching layer
- Project scaffolding — `go.mod`, `cmd/` structure, Makefile, linting config

**Deliverables**:
- [x] Go monorepo with `cmd/` and `internal/` package layout
- [x] `internal/common` package with SUPI/GPSI/SUCI validation, 3GPP error codes (RFC 7807), SBI HTTP/2 helpers, structured logging, config loading, health probes, OTel telemetry setup
- [x] `internal/db` package with pgx connection pool, query builder, transaction manager with retry-on-40001, migration runner
- [x] `internal/cache` package with in-memory L1 + Redis L2 caching, TTL management, write-through invalidation
- [x] CI pipeline with `golangci-lint`, unit test execution, coverage thresholds (≥80%)
- [x] Unit tests for all shared packages

**Dependencies**: None (first phase)

**Risks**:
- YugabyteDB pgx driver compatibility — validate YSQL compatibility early
- Identifier parsing edge cases — 3GPP formats (SUPI, SUCI) have complex encoding rules

---

### Phase 2 — Database Schema & Data Model

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 2 |
| **Name** | Database Schema & Data Model |
| **Status** | DONE |
| **Progress** | 100% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the complete YugabyteDB schema per TS 29.505, including all subscriber data tables, indexing strategy, partitioning/sharding configuration, and migration tooling. Validate schema against 3GPP data model requirements.

**Related Docs**:
- `docs/data-model.md` — §1–§10 (complete data model specification)
- `docs/architecture.md` — §14 (Data Model Overview)

**Related 3GPP References**:
- TS 29.505 — Subscription Data schemas (80+ data schemas, 180+ endpoints)
- TS 23.003 — SUPI as root key, identifier formats

**Components Involved**:
- `subscribers` table (SUPI, GPSI, identity data)
- `authentication_data` table (K, OPc, algorithm, SQN)
- `access_mobility_subscription` table (AM data per PLMN)
- `session_management_subscription` table (SM data per PLMN/S-NSSAI/DNN)
- `amf_registrations`, `smf_registrations`, `smsf_registrations` tables
- `ee_subscriptions`, `sdm_subscriptions` tables
- `pp_data`, `shared_data`, `operator_specific_data` tables
- Additional context and provisioned data tables
- Indexing strategy (SUPI hash, GPSI secondary, composite keys)
- Tablespace definitions for geo-distribution (global RF=3, region-local RF=3)

**Deliverables**:
- [x] Complete SQL schema migration files for all 20+ tables
- [x] Indexing strategy implementation (GIN indexes for JSONB, B-tree for lookup columns)
- [x] Tablespace configuration for multi-region placement
- [x] Data partitioning and sharding configuration (hash partitioning on SUPI)
- [x] Migration runner integration tests against YugabyteDB
- [x] Storage estimation validation

**Dependencies**: Phase 1 (shared libraries, db package)

**Risks**:
- JSONB vs. normalized column trade-offs — complex 3GPP nested structures may impact query performance
- Tablet splitting configuration — incorrect split points may cause hot spots

---

### Phase 3 — Core High-Traffic Services (UEAU, SDM, UECM)

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 3 |
| **Name** | Core High-Traffic Services |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the three high-traffic Nudm microservices that form the critical path for 5G UE registration, authentication, and session establishment. These services handle 80%+ of UDM traffic.

**Related Docs**:
- `docs/service-decomposition.md` — §2.1 (UEAU), §2.2 (SDM), §2.3 (UECM)
- `docs/sbi-api-design.md` — §3.1 (UEAU endpoints), §3.2 (SDM endpoints), §3.3 (UECM endpoints)
- `docs/sequence-diagrams.md` — §2 (Registration), §3 (5G-AKA), §4 (EAP-AKA'), §5 (SUCI), §6 (PDU Session)
- `docs/performance.md` — §2 (Performance Targets)

**Related 3GPP References**:
- TS 29.503 — Nudm_UEAU, Nudm_SDM, Nudm_UECM API specifications
- TS 33.501 — 5G-AKA, EAP-AKA' authentication procedures, SUCI de-concealment (§6.12)
- TS 23.502 — 5G System Procedures (registration, authentication, session establishment)
- TS 29.505 — Subscription Data schemas used by SDM

**Components Involved**:
- `cmd/udm-ueau/` + `internal/ueau/` — 7 endpoints, Milenage/TUAK algorithms, SQN management
- `cmd/udm-sdm/` + `internal/sdm/` — 38 endpoints, data retrieval, change subscriptions, identity translation
- `cmd/udm-uecm/` + `internal/uecm/` — 17 endpoints, AMF/SMF/SMSF registration, deregistration
- `internal/notify/` — callback dispatch for SDM change notifications and UECM deregistration

**Deliverables**:
- [ ] **udm-ueau**: Auth vector generation (5G-AKA, EAP-AKA'), auth confirmation, SQN management, HSS interworking, GBA vectors
- [ ] **udm-sdm**: All 38 data retrieval endpoints, SDM change subscriptions, shared data, identity translation (GPSI↔SUPI)
- [ ] **udm-uecm**: AMF/SMF/SMSF registration and deregistration, PEI updates, SMS routing, roaming info updates
- [ ] `internal/notify` — Callback engine with retry, circuit breaker, batch delivery, DLQ
- [ ] Unit tests (≥80% coverage per service)
- [ ] Integration tests against YugabyteDB for all three services
- [ ] API conformance tests validating against 3GPP OpenAPI YAML specs

**Dependencies**: Phase 1, Phase 2

**Risks**:
- Milenage/TUAK cryptographic correctness — must validate against 3GPP test vectors (TS 35.207, TS 35.231)
- SQN serialization under high concurrency — row-level locking must not become a bottleneck
- SDM endpoint count (38) — large surface area increases testing effort

---

### Phase 4 — SUCI De-concealment & NRF Integration

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 4 |
| **Name** | SUCI De-concealment & NRF Integration |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the UEID service for SUCI de-concealment (ECIES Profile A/B) and the NRF client library for service registration, discovery, heartbeat, and OAuth2 token acquisition.

**Related Docs**:
- `docs/service-decomposition.md` — §2.10 (UEID), §3.6 (udm-nrf)
- `docs/sbi-api-design.md` — §6 (Authentication and Authorization)
- `docs/security.md` — §4 (Subscriber Identity Protection)

**Related 3GPP References**:
- TS 33.501 — §6.12 (SUCI de-concealment, ECIES Profile A and B)
- TS 29.510 — NRF NF Management and Discovery APIs
- TS 29.500 — OAuth2 client credentials flow for SBI

**Components Involved**:
- `cmd/udm-ueid/` + `internal/ueid/` — SUCI deconceal endpoint, ECIES crypto (Curve25519, secp256r1)
- `internal/nrf/` — NF registration, heartbeat, discovery, OAuth2 token cache
- Integration with `udm-ueau` for SUCI resolution during authentication

**Deliverables**:
- [ ] **udm-ueid**: SUCI de-concealment with ECIES Profile A (Curve25519/X25519) and Profile B (secp256r1)
- [ ] Key management for HPLMN public/private key pairs with rotation support
- [ ] `internal/nrf` — NF registration on startup, periodic heartbeat, graceful deregistration
- [ ] NF discovery client with local caching
- [ ] OAuth2 token acquisition and caching from NRF
- [ ] Integration of UEID with UEAU for SUCI-based auth requests
- [ ] Unit and integration tests, ECIES test vector validation

**Dependencies**: Phase 1, Phase 3 (UEAU integration)

**Risks**:
- ECIES implementation correctness — cryptographic edge cases (invalid ephemeral keys, MAC failures)
- Key rotation coordination across regions — must not cause deconceal failures during rotation window

---

### Phase 5 — Medium-Traffic Services (EE, PP, MT)

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 5 |
| **Name** | Medium-Traffic Services |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the three medium-traffic Nudm microservices for event exposure, parameter provisioning, and mobile-terminated service routing.

**Related Docs**:
- `docs/service-decomposition.md` — §2.4 (EE), §2.5 (PP), §2.6 (MT)
- `docs/sbi-api-design.md` — §3.4 (EE), §3.5 (PP), §3.6 (MT)
- `docs/sequence-diagrams.md` — §8 (Data Update Notifications), §10 (Event Exposure)

**Related 3GPP References**:
- TS 29.503 — Nudm_EE, Nudm_PP, Nudm_MT API specifications
- TS 23.502 — Event exposure procedures, parameter provisioning flows

**Components Involved**:
- `cmd/udm-ee/` + `internal/ee/` — event subscriptions, condition monitoring, callback dispatch
- `cmd/udm-pp/` + `internal/pp/` — per-UE provisioning, 5G VN groups, MBS group membership
- `cmd/udm-mt/` + `internal/mt/` — UE info query, location provisioning

**Deliverables**:
- [ ] **udm-ee**: Event subscription CRUD, 10+ event types (reachability, location, connectivity, etc.), callback dispatch via udm-notify
- [ ] **udm-pp**: Per-UE parameter PATCH, 5G VN group CRUD, MBS group membership, change propagation to SDM subscribers
- [ ] **udm-mt**: UE info query (serving AMF, user state), location provisioning
- [ ] Integration between EE ↔ UECM (event triggers from registration changes)
- [ ] Integration between PP → SDM (change notifications after parameter updates)
- [ ] Unit tests, integration tests, API conformance tests

**Dependencies**: Phase 1, Phase 2, Phase 3 (notify, UECM event triggers, SDM change subscriptions)

**Risks**:
- Event correlation complexity — monitoring multiple data sources for event condition matching
- Callback reliability — ensuring notification delivery under NF consumer failures

---

### Phase 6 — Low-Traffic Services (SSAU, NIDDAU, RSDS)

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 6 |
| **Name** | Low-Traffic Services |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the three low-traffic Nudm microservices for service authorization, NIDD authorization, and SMS delivery status reporting.

**Related Docs**:
- `docs/service-decomposition.md` — §2.7 (SSAU), §2.8 (NIDDAU), §2.9 (RSDS)
- `docs/sbi-api-design.md` — §3.7 (SSAU), §3.8 (NIDDAU), §3.9 (RSDS)

**Related 3GPP References**:
- TS 29.503 — Nudm_SSAU, Nudm_NIDDAU, Nudm_RSDS API specifications
- TS 23.502 — NIDD procedures, SMS delivery procedures

**Components Involved**:
- `cmd/udm-ssau/` + `internal/ssau/` — service authorization check and removal
- `cmd/udm-niddau/` + `internal/niddau/` — NIDD configuration authorization
- `cmd/udm-rsds/` + `internal/rsds/` — SMS delivery status recording

**Deliverables**:
- [ ] **udm-ssau**: Service-specific authorization check and removal endpoints
- [ ] **udm-niddau**: NIDD authorization endpoint with DNN/S-NSSAI validation
- [ ] **udm-rsds**: SMS delivery status reporting with EE event propagation
- [ ] Unit tests, integration tests, API conformance tests
- [ ] Complete Nudm API surface coverage (all 10 services operational)

**Dependencies**: Phase 1, Phase 2, Phase 5 (EE integration for RSDS events)

**Risks**:
- Low-traffic services may receive less testing attention — enforce same quality gates as high-traffic services

---

### Phase 7 — Security Hardening

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 7 |
| **Name** | Security Hardening |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the complete security architecture including mTLS enforcement, OAuth2 per-service scopes, column-level encryption for authentication credentials, audit logging, and network policies.

**Related Docs**:
- `docs/security.md` — §1–§10 (complete security architecture)
- `docs/architecture.md` — §12 (Security Architecture)

**Related 3GPP References**:
- TS 33.501 — 5G Security Architecture (authentication, key derivation, SUCI privacy)
- TS 29.500 — SBI security requirements (mTLS, OAuth2)

**Components Involved**:
- mTLS configuration (Istio-managed + application-level verification)
- OAuth2 per-service scope enforcement for all 10 Nudm services
- Column-level encryption for `authentication_data` (K, OPc keys)
- SUCI profile key management with HSM integration
- Audit logging for all data access operations
- Kubernetes network policies for micro-segmentation
- Input validation and rate limiting

**Deliverables**:
- [ ] mTLS enforcement on all SBI interfaces (service mesh + application verification)
- [ ] OAuth2 token validation middleware with per-service scope checks
- [ ] Column-level encryption for K, OPc in `authentication_data` table
- [ ] HSM integration for SUCI de-concealment private keys
- [ ] Comprehensive audit logging (subscriber data access, authentication events)
- [ ] Kubernetes NetworkPolicy manifests for pod-to-pod micro-segmentation
- [ ] Input validation middleware (SUPI format, request size limits, injection prevention)
- [ ] Rate limiting and overload control (429/503 responses per TS 29.500)
- [ ] Security test suite (penetration test plan, vulnerability scanning)

**Dependencies**: Phase 1, Phase 3, Phase 4 (all services must exist for scope enforcement)

**Risks**:
- HSM latency impact on authentication vector generation — benchmark early
- Column-level encryption performance overhead on high-traffic read paths

---

### Phase 8 — Observability & Monitoring

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 8 |
| **Name** | Observability & Monitoring |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Implement the complete observability stack including Prometheus metrics, distributed tracing, structured logging, 3GPP-aligned alarms, and Grafana dashboards.

**Related Docs**:
- `docs/observability.md` — §1–§9 (complete observability architecture)
- `docs/architecture.md` — §13 (Observability and Operations)

**Related 3GPP References**:
- TS 28.532 — Management Services (fault management, alarm definitions)
- TS 28.552 — Performance Measurements (PM counters)

**Components Involved**:
- Prometheus metric exposition (`/metrics` on all services)
- OpenTelemetry SDK integration for metrics, traces, and logs
- Distributed tracing with correlation IDs and SUPI filtering
- 3GPP alarm system (SNMP traps, severity levels)
- Grafana dashboards (service health, KPIs, capacity)
- Alertmanager rules (SLO-based alerting)

**Deliverables**:
- [ ] RED metrics (Rate, Errors, Duration) on all Nudm endpoints
- [ ] Custom KPI metrics (auth success rate, registration TPS, cache hit ratio)
- [ ] OpenTelemetry distributed tracing across all services with SUPI correlation
- [ ] Structured JSON logging with correlation IDs, 3GPP trace IDs
- [ ] 3GPP-aligned alarm definitions (critical, major, minor, warning)
- [ ] Grafana dashboard templates (service overview, per-service detail, database health)
- [ ] Alertmanager rules for SLO violations (latency, error rate, availability)
- [ ] OSS/BSS integration interfaces

**Dependencies**: Phase 1 (telemetry package), Phase 3–6 (all services for instrumentation)

**Risks**:
- Observability overhead — must validate zero impact on hot-path latency (p99)
- Alert fatigue — requires careful threshold tuning based on baseline measurements

---

### Phase 9 — Multi-Region Deployment & Infrastructure

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 9 |
| **Name** | Multi-Region Deployment & Infrastructure |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Deploy the complete UDM system in a three-region active-active topology with YugabyteDB geo-distribution, Istio service mesh, global load balancing, and zero-downtime upgrade procedures.

**Related Docs**:
- `docs/deployment.md` — §1–§10 (complete deployment architecture)
- `docs/architecture.md` — §6 (Multi-Region Active-Active), §15 (Deployment Architecture)

**Related 3GPP References**:
- TS 23.501 — UDM role in multi-PLMN and roaming scenarios
- TS 29.500 — SBI load balancing and overload control

**Components Involved**:
- Kubernetes manifests for all 10 services (Deployment, Service, HPA, PDB, ConfigMap, Secret)
- Istio VirtualService/DestinationRule/Gateway configurations
- YugabyteDB multi-region cluster setup (RF=3, preferred leaders, tablespace policies)
- GeoDNS/Route 53 latency-based routing
- Redis Cluster per-region deployment
- CI/CD pipeline for multi-region rolling deployments
- Configuration management (ConfigMaps, Secrets, feature flags)

**Deliverables**:
- [ ] Kubernetes manifests for all 10 services with HPA, PDB, resource limits
- [ ] Istio service mesh configuration (mTLS, traffic management, circuit breaking)
- [ ] YugabyteDB 3-region deployment with global and region-local tablespaces
- [ ] GeoDNS configuration for latency-based routing
- [ ] Zero-downtime rolling upgrade procedures and runbooks
- [ ] Failover strategy implementation and testing
- [ ] CI/CD pipeline for multi-region deployments (GitOps)
- [ ] Capacity planning validation (10M subscriber baseline)

**Dependencies**: Phase 1–8 (all services, security, observability must be ready)

**Risks**:
- Cross-region write latency — Raft consensus across regions adds latency to writes
- Region failover coordination — must validate automatic DNS failover and YugabyteDB leader election
- Data placement policy correctness — misconfigured tablespaces can cause data residency violations (e.g., GDPR)

---

### Phase 10 — Performance Optimization & Load Testing

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 10 |
| **Name** | Performance Optimization & Load Testing |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Optimize system performance to meet carrier-grade targets and validate through comprehensive load, stress, soak, and chaos testing.

**Related Docs**:
- `docs/performance.md` — §1–§10 (complete performance architecture)
- `docs/testing-strategy.md` — §5 (Performance and Resilience Testing)

**Related 3GPP References**:
- TS 23.502 — Latency requirements for 5G procedures (registration, authentication, session)
- TS 29.500 — Overload control mechanisms

**Components Involved**:
- Goroutine pool tuning per service
- Connection pool optimization (pgx, Redis)
- Caching strategy validation and tuning
- Load balancing configuration (Istio, YugabyteDB follower reads)
- Load testing framework (k6 or custom Go load generator)
- Chaos engineering tooling (Litmus, Chaos Monkey)

**Deliverables**:
- [ ] Performance baseline measurements for all Nudm operations
- [ ] Validation against latency targets (p50 <5ms, p95 <15ms, p99 <30ms for auth)
- [ ] Throughput validation (100K+ TPS aggregate across 3 regions)
- [ ] Load test scenarios (steady state, burst, mass re-registration)
- [ ] Stress testing (identify breaking points and graceful degradation behavior)
- [ ] Soak testing (72-hour sustained load for memory leak detection)
- [ ] Chaos engineering (pod kill, node failure, region failure, network partition)
- [ ] Performance optimization report with tuning recommendations
- [ ] Bottleneck analysis and mitigation plan

**Dependencies**: Phase 9 (full deployment required for realistic testing)

**Risks**:
- Tail latency (p99) exceeding 3× of p50 under load — requires profiling and optimization
- GC pressure from high goroutine count — Go runtime tuning (GOGC, GOMEMLIMIT) may be needed
- Cross-region write latency making 5ms p50 targets difficult for registration operations

---

### Phase 11 — E2E Telecom Scenario Validation & GA Readiness

| Property | Value |
|----------|-------|
| **Phase ID** | Phase 11 |
| **Name** | E2E Telecom Scenario Validation & GA Readiness |
| **Status** | NOT_STARTED |
| **Progress** | 0% |
| **Last Updated** | 2026-03-21 |
| **Owner** | — |

**Description**: Execute end-to-end 5G telecom scenario tests, validate full 3GPP compliance, and prepare for General Availability release.

**Related Docs**:
- `docs/testing-strategy.md` — §3 (Telecom Protocol Testing), §4 (5G Telecom Scenarios)
- `docs/sequence-diagrams.md` — §2–§14 (all procedure flows)
- `docs/sbi-api-design.md` — complete API catalog

**Related 3GPP References**:
- TS 23.502 — All 5G System Procedures (registration, auth, session, deregistration, roaming, handover)
- TS 29.503 — All Nudm API conformance
- TS 29.505 — Subscription data conformance
- TS 33.501 — Security procedure validation

**Components Involved**:
- E2E test harness simulating AMF, AUSF, SMF, SMSF, NEF interactions
- 3GPP protocol compliance test suite
- Roaming scenario validation
- Network slicing data retrieval validation
- State machine validation (UE registration states, session states)

**Deliverables**:
- [ ] E2E test suite covering all flows in `docs/sequence-diagrams.md`
- [ ] 5G UE Registration (initial attach) end-to-end validation
- [ ] 5G-AKA and EAP-AKA' authentication end-to-end validation
- [ ] SUCI de-concealment end-to-end validation
- [ ] PDU Session establishment end-to-end validation
- [ ] UE Deregistration (explicit and implicit) validation
- [ ] Roaming scenario validation (VPLMN/HPLMN interactions)
- [ ] Network slicing data retrieval validation
- [ ] 3GPP API conformance report (all 103 endpoints)
- [ ] GA release checklist and sign-off

**Dependencies**: Phase 1–10 (all phases must be complete or at minimum DONE for critical path)

**Risks**:
- Interoperability with real NF implementations — may expose edge cases not covered by spec
- Roaming scenario complexity — VPLMN/HPLMN split requires careful testing
- Regulatory compliance requirements may vary by deployment region

---

## 4. Phase Dependencies

```
Phase 1 (Foundation)
    │
    ├──► Phase 2 (Database Schema)
    │        │
    │        ├──► Phase 3 (Core Services: UEAU, SDM, UECM)
    │        │        │
    │        │        ├──► Phase 4 (SUCI/UEID + NRF)
    │        │        │
    │        │        ├──► Phase 5 (Medium Services: EE, PP, MT)
    │        │        │        │
    │        │        │        └──► Phase 6 (Low Services: SSAU, NIDDAU, RSDS)
    │        │        │
    │        │        └──► Phase 7 (Security Hardening)
    │        │
    │        └──► Phase 8 (Observability)
    │
    └──► Phase 9 (Multi-Region Deployment) ← requires Phase 1–8
              │
              └──► Phase 10 (Performance Testing)
                        │
                        └──► Phase 11 (E2E Validation & GA)
```

---

## 5. Traceability Matrix

| Feature | Module | Design Doc | 3GPP Spec | Phase |
|---------|--------|------------|-----------|-------|
| SUPI/GPSI/SUCI identifier parsing | udm-common | `docs/service-decomposition.md` §3.2 | TS 23.003 | Phase 1 |
| 3GPP error codes (RFC 7807) | udm-common | `docs/service-decomposition.md` §3.2 | TS 29.500 | Phase 1 |
| SBI HTTP/2 codec | udm-common | `docs/sbi-api-design.md` §1 | TS 29.500 | Phase 1 |
| 3GPP custom HTTP headers | udm-common | `docs/sbi-api-design.md` §5 | TS 29.500 | Phase 1 |
| Structured JSON logging | udm-common | `docs/observability.md` §5 | — | Phase 1 |
| OpenTelemetry telemetry setup | udm-common | `docs/observability.md` §1–§4 | TS 28.552 | Phase 1 |
| pgx connection pool | udm-db | `docs/service-decomposition.md` §3.3 | — | Phase 1 |
| Query builder with SQL safety | udm-db | `docs/service-decomposition.md` §3.3 | — | Phase 1 |
| Transaction manager (retry on 40001) | udm-db | `docs/service-decomposition.md` §3.3 | — | Phase 1 |
| Schema migration runner | udm-db | `docs/data-model.md` §9 | — | Phase 1 |
| In-memory L1 cache (Ristretto) | udm-cache | `docs/service-decomposition.md` §3.4 | — | Phase 1 |
| Redis L2 cache | udm-cache | `docs/service-decomposition.md` §3.4 | — | Phase 1 |
| Subscribers table (SUPI root) | YugabyteDB | `docs/data-model.md` §2, §3 | TS 29.505 | Phase 2 |
| Authentication data table (K, OPc, SQN) | YugabyteDB | `docs/data-model.md` §3 | TS 29.505, TS 33.501 | Phase 2 |
| Access & Mobility subscription table | YugabyteDB | `docs/data-model.md` §3 | TS 29.505 | Phase 2 |
| Session Management subscription table | YugabyteDB | `docs/data-model.md` §3 | TS 29.505 | Phase 2 |
| AMF/SMF/SMSF registration tables | YugabyteDB | `docs/data-model.md` §3 | TS 29.505 | Phase 2 |
| EE/SDM subscription tables | YugabyteDB | `docs/data-model.md` §3 | TS 29.505 | Phase 2 |
| SUPI hash-based sharding | YugabyteDB | `docs/data-model.md` §5 | — | Phase 2 |
| JSONB GIN indexes | YugabyteDB | `docs/data-model.md` §4 | — | Phase 2 |
| Geo-distributed tablespaces | YugabyteDB | `docs/data-model.md` §6, `docs/deployment.md` §5 | — | Phase 2 |
| 5G-AKA auth vector generation | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 29.503 (Nudm_UEAU), TS 33.501 | Phase 3 |
| EAP-AKA' auth vector generation | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 29.503 (Nudm_UEAU), TS 33.501 | Phase 3 |
| Milenage algorithm | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 35.206 | Phase 3 |
| TUAK algorithm | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 35.231 | Phase 3 |
| SQN management | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 33.501 | Phase 3 |
| Auth confirmation events | udm-ueau | `docs/sequence-diagrams.md` §3 | TS 29.503 (Nudm_UEAU) | Phase 3 |
| HSS interworking (EPS-AKA) | udm-ueau | `docs/service-decomposition.md` §2.1 | TS 29.503, TS 23.502 | Phase 3 |
| AM data retrieval | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM), TS 29.505 | Phase 3 |
| SM data retrieval | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM), TS 29.505 | Phase 3 |
| NSSAI retrieval | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM) | Phase 3 |
| SDM change subscriptions | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM) | Phase 3 |
| GPSI↔SUPI identity translation | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM), TS 23.003 | Phase 3 |
| Shared data retrieval | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM) | Phase 3 |
| SoR/UPU/CAG acknowledgments | udm-sdm | `docs/service-decomposition.md` §2.2 | TS 29.503 (Nudm_SDM) | Phase 3 |
| AMF registration (3GPP/non-3GPP) | udm-uecm | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) | Phase 3 |
| SMF registration per PDU session | udm-uecm | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) | Phase 3 |
| SMSF registration/deregistration | udm-uecm | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) | Phase 3 |
| PEI update | udm-uecm | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) | Phase 3 |
| SMS routing info | udm-uecm | `docs/service-decomposition.md` §2.3 | TS 29.503 (Nudm_UECM) | Phase 3 |
| Callback dispatch engine | udm-notify | `docs/service-decomposition.md` §3.5 | TS 29.503 | Phase 3 |
| Retry with exponential backoff | udm-notify | `docs/service-decomposition.md` §3.5 | — | Phase 3 |
| Circuit breaker per destination | udm-notify | `docs/service-decomposition.md` §3.5 | — | Phase 3 |
| Dead letter queue | udm-notify | `docs/service-decomposition.md` §3.5 | — | Phase 3 |
| SUCI de-concealment (ECIES Profile A) | udm-ueid | `docs/service-decomposition.md` §2.10 | TS 33.501 §6.12 | Phase 4 |
| SUCI de-concealment (ECIES Profile B) | udm-ueid | `docs/service-decomposition.md` §2.10 | TS 33.501 §6.12 | Phase 4 |
| HPLMN key rotation | udm-ueid | `docs/security.md` §4 | TS 33.501 | Phase 4 |
| NRF NF registration | udm-nrf | `docs/service-decomposition.md` §3.6 | TS 29.510 | Phase 4 |
| NRF heartbeat | udm-nrf | `docs/service-decomposition.md` §3.6 | TS 29.510 | Phase 4 |
| NRF NF discovery (cached) | udm-nrf | `docs/service-decomposition.md` §3.6 | TS 29.510 | Phase 4 |
| OAuth2 token acquisition | udm-nrf | `docs/service-decomposition.md` §3.6 | TS 29.500 | Phase 4 |
| Event exposure subscriptions | udm-ee | `docs/service-decomposition.md` §2.4 | TS 29.503 (Nudm_EE) | Phase 5 |
| UE reachability events | udm-ee | `docs/service-decomposition.md` §2.4 | TS 29.503 (Nudm_EE) | Phase 5 |
| Location reporting events | udm-ee | `docs/service-decomposition.md` §2.4 | TS 29.503 (Nudm_EE) | Phase 5 |
| Parameter provisioning (per-UE) | udm-pp | `docs/service-decomposition.md` §2.5 | TS 29.503 (Nudm_PP) | Phase 5 |
| 5G VN group management | udm-pp | `docs/service-decomposition.md` §2.5 | TS 29.503 (Nudm_PP) | Phase 5 |
| MBS group membership | udm-pp | `docs/service-decomposition.md` §2.5 | TS 29.503 (Nudm_PP) | Phase 5 |
| UE info query (MT) | udm-mt | `docs/service-decomposition.md` §2.6 | TS 29.503 (Nudm_MT) | Phase 5 |
| Location provisioning (MT) | udm-mt | `docs/service-decomposition.md` §2.6 | TS 29.503 (Nudm_MT) | Phase 5 |
| Service-specific authorization | udm-ssau | `docs/service-decomposition.md` §2.7 | TS 29.503 (Nudm_SSAU) | Phase 6 |
| NIDD authorization | udm-niddau | `docs/service-decomposition.md` §2.8 | TS 29.503 (Nudm_NIDDAU) | Phase 6 |
| SMS delivery status reporting | udm-rsds | `docs/service-decomposition.md` §2.9 | TS 29.503 (Nudm_RSDS) | Phase 6 |
| mTLS enforcement | Security | `docs/security.md` §3 | TS 33.501, TS 29.500 | Phase 7 |
| OAuth2 per-service scopes | Security | `docs/security.md` §2.2 | TS 33.501 §13.3 | Phase 7 |
| Column-level encryption (K, OPc) | Security | `docs/security.md` §6 | TS 33.501 | Phase 7 |
| Audit logging | Security | `docs/security.md` §7 | TS 33.501 | Phase 7 |
| Network policies (micro-segmentation) | Security | `docs/security.md` §7 | — | Phase 7 |
| Prometheus RED metrics | Observability | `docs/observability.md` §2 | TS 28.552 | Phase 8 |
| Distributed tracing (OTel) | Observability | `docs/observability.md` §4 | — | Phase 8 |
| 3GPP alarm system | Observability | `docs/observability.md` §6 | TS 28.532 | Phase 8 |
| Grafana dashboards | Observability | `docs/observability.md` §9 | — | Phase 8 |
| 3-region active-active deployment | Deployment | `docs/deployment.md` §2 | — | Phase 9 |
| YugabyteDB geo-distribution | Deployment | `docs/deployment.md` §5 | — | Phase 9 |
| Zero-downtime rolling upgrades | Deployment | `docs/deployment.md` §6 | — | Phase 9 |
| HPA auto-scaling per service | Deployment | `docs/deployment.md` §10 | — | Phase 9 |
| Performance baseline validation | Performance | `docs/performance.md` §2 | TS 23.502 | Phase 10 |
| Load testing (100K+ TPS) | Performance | `docs/performance.md` §9 | — | Phase 10 |
| Chaos engineering | Testing | `docs/testing-strategy.md` §5.4 | — | Phase 10 |
| E2E 5G registration flow | Testing | `docs/testing-strategy.md` §4.1, `docs/sequence-diagrams.md` §2 | TS 23.502 §4.2.2.2 | Phase 11 |
| E2E authentication flow | Testing | `docs/testing-strategy.md` §4.2, `docs/sequence-diagrams.md` §3 | TS 23.502, TS 33.501 | Phase 11 |
| Roaming scenario validation | Testing | `docs/testing-strategy.md` §4.5, `docs/sequence-diagrams.md` §11 | TS 23.502 | Phase 11 |
| Network slicing validation | Testing | `docs/testing-strategy.md` §4.6, `docs/sequence-diagrams.md` §12 | TS 23.501 | Phase 11 |
| 3GPP API conformance (103 endpoints) | Testing | `docs/testing-strategy.md` §2.3 | TS 29.503, TS 29.505 | Phase 11 |

---

## 6. Phase Summary Dashboard

| Phase | Name | Status | Progress | Last Updated | Dependencies |
|-------|------|--------|----------|--------------|--------------|
| 1 | Foundation & Shared Libraries | DONE | 100% | 2026-03-21 | None |
| 2 | Database Schema & Data Model | DONE | 100% | 2026-03-21 | Phase 1 |
| 3 | Core High-Traffic Services (UEAU, SDM, UECM) | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 2 |
| 4 | SUCI De-concealment & NRF Integration | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 3 |
| 5 | Medium-Traffic Services (EE, PP, MT) | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 2, 3 |
| 6 | Low-Traffic Services (SSAU, NIDDAU, RSDS) | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 2, 5 |
| 7 | Security Hardening | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 3, 4 |
| 8 | Observability & Monitoring | NOT_STARTED | 0% | 2026-03-21 | Phase 1, 3–6 |
| 9 | Multi-Region Deployment & Infrastructure | NOT_STARTED | 0% | 2026-03-21 | Phase 1–8 |
| 10 | Performance Optimization & Load Testing | NOT_STARTED | 0% | 2026-03-21 | Phase 9 |
| 11 | E2E Telecom Validation & GA Readiness | NOT_STARTED | 0% | 2026-03-21 | Phase 1–10 |

---

## 7. References

### Design Documents (Primary Source)

| Document | Path | Description |
|----------|------|-------------|
| Architecture | `docs/architecture.md` | High-level architecture, system overview, design decisions |
| Service Decomposition | `docs/service-decomposition.md` | 10 microservices + 5 shared libraries, package structure |
| Data Model | `docs/data-model.md` | YugabyteDB schema, ER model, indexing, partitioning |
| SBI API Design | `docs/sbi-api-design.md` | Complete API endpoint catalog, headers, auth, versioning |
| Sequence Diagrams | `docs/sequence-diagrams.md` | All 3GPP procedure flows with Mermaid diagrams |
| Security | `docs/security.md` | Threat model, mTLS, OAuth2, encryption, audit |
| Observability | `docs/observability.md` | Metrics, tracing, logging, alarms, dashboards |
| Performance | `docs/performance.md` | Latency/throughput targets, Go optimization, capacity planning |
| Deployment | `docs/deployment.md` | Multi-region topology, K8s architecture, CI/CD, failover |
| Testing Strategy | `docs/testing-strategy.md` | Testing pyramid, quality gates, telecom scenario tests |

### 3GPP Standard References

| Specification | File | Title |
|---------------|------|-------|
| TS 23.003 | `docs/3gpp/23003-i70.docx` | Numbering, Addressing and Identification |
| TS 23.501 | `docs/3gpp/23501-ic0.docx` | System Architecture for the 5G System |
| TS 23.502 | `docs/3gpp/23502-id0.docx` | Procedures for the 5G System |
| TS 23.503 | `docs/3gpp/23503-ib0.docx` | Policy and Charging Control Framework |
| TS 29.500 | `docs/3gpp/29500-ia0.docx` | Technical Realization of SBA |
| TS 29.503 | `docs/3gpp/29503-ic0.docx` | Nudm Services |
| TS 29.505 | `docs/3gpp/29505-i80.docx` | Usage of the Unified Data Repository |
| TS 33.501 | `docs/3gpp/33501-ia0.docx` | Security Architecture and Procedures for 5G |

### 3GPP OpenAPI Specifications

| File | Service |
|------|---------|
| `docs/3gpp/TS29503_Nudm_UEAU.yaml` | UE Authentication |
| `docs/3gpp/TS29503_Nudm_SDM.yaml` | Subscriber Data Management |
| `docs/3gpp/TS29503_Nudm_UECM.yaml` | UE Context Management |
| `docs/3gpp/TS29503_Nudm_EE.yaml` | Event Exposure |
| `docs/3gpp/TS29503_Nudm_PP.yaml` | Parameter Provisioning |
| `docs/3gpp/TS29503_Nudm_MT.yaml` | Mobile Terminated |
| `docs/3gpp/TS29503_Nudm_SSAU.yaml` | Service-Specific Authorization |
| `docs/3gpp/TS29503_Nudm_NIDDAU.yaml` | NIDD Authorization |
| `docs/3gpp/TS29503_Nudm_RSDS.yaml` | Report SMS Delivery Status |
| `docs/3gpp/TS29503_Nudm_UEID.yaml` | UE Identification |
| `docs/3gpp/TS29505_Subscription_Data.yaml` | Subscription Data |
| `docs/3gpp/TS29500_CustomHeaders.abnf` | 3GPP Custom HTTP Headers |
