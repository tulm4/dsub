# GitHub Copilot Instructions — 5G UDM Project

---

## 1. Project Context

This project implements a **5G Core Unified Data Management (UDM)** network function as defined by 3GPP specifications. The UDM is the authoritative source for subscriber data, authentication credentials, and registration state within a 5G Standalone (SA) core network.

**Key characteristics:**
- **10 independent Nudm microservices** written in Go, each mapping to one 3GPP Nudm service-based interface (TS 29.503)
- **No separate UDR** — the UDM consolidates application logic and data storage access into a single logical NF
- **YugabyteDB** (distributed SQL, PostgreSQL wire protocol) as the primary data store
- **Multi-region active-active** deployment across 3 geographic regions on Kubernetes
- **Carrier-grade** requirements: 99.999% availability, sub-10ms p50 latency, 100K+ TPS
- **3GPP-aligned** system — all interfaces, data models, procedures, and security mechanisms follow 3GPP Release 17+ specifications

**Microservices:**
| Service | API Root | Purpose |
|---------|----------|---------|
| udm-ueau | `/nudm-ueau/v1` | UE Authentication (5G-AKA, EAP-AKA') |
| udm-sdm | `/nudm-sdm/v2` | Subscriber Data Management |
| udm-uecm | `/nudm-uecm/v1` | UE Context Management |
| udm-ee | `/nudm-ee/v1` | Event Exposure |
| udm-pp | `/nudm-pp/v1` | Parameter Provisioning |
| udm-mt | `/nudm-mt/v1` | Mobile Terminated |
| udm-ssau | `/nudm-ssau/v1` | Service-Specific Authorization |
| udm-niddau | `/nudm-niddau/v1` | NIDD Authorization |
| udm-rsds | `/nudm-rsds/v1` | Report SMS Delivery Status |
| udm-ueid | `/nudm-ueid/v1` | UE Identification (SUCI de-concealment) |

---

## 2. Source of Truth

Copilot MUST prioritize these sources in the following order:

1. **`./docs/*`** — Primary system design documents (architecture, service decomposition, data model, SBI API design, security, observability, performance, deployment, testing strategy, sequence diagrams)
2. **`./docs/3gpp/*`** — 3GPP standard references (TS specifications and OpenAPI YAML files)
3. **`IMPLEMENTATION_ROADMAP.md`** — Implementation phases, status tracking, and traceability matrix

> **RULE: If a code suggestion conflicts with any design document in `./docs/` or `./docs/3gpp/`, FOLLOW THE DOCS.** The design documents are the authoritative source of truth for all architectural decisions, API contracts, data models, and procedures.

---

## 3. Coding Guidelines

### 3.1 Architecture Compliance

- Follow the service decomposition defined in `docs/service-decomposition.md`
- Each microservice owns **exactly one** Nudm API root — do not mix service responsibilities
- Shared logic goes in `internal/` packages (`common`, `db`, `cache`, `notify`, `nrf`) — not in service-specific packages
- All services are **stateless** — no in-memory session state, no local disk state
- All mutable state is persisted in **YugabyteDB**

### 3.2 Module Boundaries

- **`cmd/<service>/main.go`** — Service entry point only (init, wire dependencies, start HTTP server)
- **`internal/<service>/handler.go`** — HTTP handler layer (request parsing, response formatting)
- **`internal/<service>/service.go`** — Business logic layer (3GPP procedure implementation)
- **`internal/<service>/<domain>.go`** — Domain-specific logic (e.g., `milenage.go`, `amf_registration.go`)
- **`internal/common/`** — Cross-cutting utilities shared by all services
- **`internal/db/`** — Database access layer (pgx pool, queries, transactions)
- **`internal/cache/`** — Caching layer (in-memory L1, Redis L2)
- **`internal/notify/`** — Callback dispatch engine
- **`internal/nrf/`** — NRF client (registration, discovery, OAuth2)

### 3.3 API Conventions

- All Nudm API endpoints must follow the paths defined in `docs/sbi-api-design.md` §3
- Use HTTP/2 as the transport protocol
- JSON (`application/json`) for serialization per IETF RFC 8259
- Error responses must use 3GPP `ProblemDetails` format (RFC 7807) with cause codes from TS 29.503
- 3GPP custom HTTP headers (`3gpp-Sbi-Target-apiRoot`, `3gpp-Sbi-Callback`, `3gpp-Sbi-Oci`) must be supported as defined in `docs/sbi-api-design.md` §5
- Do **NOT** create undocumented API endpoints

### 3.4 Naming Conventions

- Use names directly from the design documents and 3GPP specifications:
  - `SUPI`, `GPSI`, `SUCI` (not `subscriberId`, `phoneNumber`, `concealedId`)
  - `Amf3GppAccessRegistration` (not `AmfRegistration`)
  - `AuthenticationInfoRequest`, `AuthenticationInfoResult` (not `AuthRequest`, `AuthResponse`)
  - `AccessAndMobilitySubscriptionData` (not `AmData`)
- Go naming conventions: exported types use PascalCase, private fields use camelCase
- Database columns use snake_case matching `docs/data-model.md`
- API paths use kebab-case matching 3GPP OpenAPI specs in `docs/3gpp/TS29503_Nudm_*.yaml`

### 3.5 Database Access

- Use `pgx` driver for YugabyteDB (PostgreSQL wire protocol)
- Use the query builder from `internal/db` — avoid raw SQL string concatenation
- Use parameterized queries to prevent SQL injection
- Wrap multi-statement operations in transactions via the transaction manager
- Handle serialization conflicts (`error code 40001`) with automatic retry
- SUPI is the root key for all subscriber data — all queries should be SUPI-anchored

### 3.6 Error Handling

- Return 3GPP-compliant `ProblemDetails` (RFC 7807) for all error responses
- Use appropriate HTTP status codes as defined in `docs/sbi-api-design.md` §7
- Include `cause` field with 3GPP-defined cause codes from TS 29.503
- Log errors with structured fields (SUPI, operation, cause, trace ID)

### 3.7 Testing

- Follow the testing strategy defined in `docs/testing-strategy.md`
- Write table-driven unit tests for all business logic
- Maintain ≥80% code coverage per service
- Use mocks/stubs for database and external service dependencies in unit tests
- Integration tests should run against a real YugabyteDB instance

---

## 4. 3GPP Compliance Rules

### 4.1 Protocol Fidelity

- Always align with procedures defined in `./docs/3gpp/*` and `docs/sequence-diagrams.md`
- Do **NOT** simplify protocol logic incorrectly — 3GPP procedures have specific ordering, state transitions, and error handling that must be preserved
- Authentication vector generation (Milenage/TUAK) must produce correct cryptographic outputs per TS 35.206/TS 35.231

### 4.2 State Machines

- Maintain correct state machines for UE registration states as shown in `docs/sequence-diagrams.md` §13
- AMF registration enforces mutual exclusion — only one AMF per access type per subscriber
- SQN (authentication sequence number) must be strictly serialized to prevent replay attacks

### 4.3 Message Flows

- Follow the exact sequence of operations shown in `docs/sequence-diagrams.md`
- Registration flow: Authentication → AMF Registration (UECM) → Data Retrieval (SDM)
- SUCI resolution must happen before credential lookup when the identifier is a SUCI
- Deregistration must trigger cleanup of all associated NF registrations and notify affected NFs

### 4.4 Data Model Alignment

- Database schemas must match the 3GPP TS 29.505 data model as mapped in `docs/data-model.md`
- JSONB columns must preserve the nested structure of 3GPP data objects (NSSAI, DNN configs, QoS)
- Do not flatten 3GPP structures that are defined as nested objects in the spec

### 4.5 Identity Handling

- SUPI format: `imsi-<MCC><MNC><MSIN>` (per TS 23.003)
- GPSI format: `msisdn-<CC><NDC><SN>` (per TS 23.003)
- SUCI structure: scheme ID, HN key ID, protection scheme, cipher text (per TS 23.003, TS 33.501)
- Always validate identifier formats before processing

---

## 5. Implementation Awareness

Copilot MUST be aware of the current implementation phase as tracked in `IMPLEMENTATION_ROADMAP.md`.

### 5.1 Phase-Aware Code Generation

- **ONLY** suggest code relevant to the current active phase
- Do **NOT** generate components from future phases unless explicitly requested
- Check the Phase Summary Dashboard in `IMPLEMENTATION_ROADMAP.md` §6 to determine which phases are `IN_PROGRESS` or `DONE`

### 5.2 Phase Mapping

| Phase | What to Generate | What NOT to Generate |
|-------|-----------------|---------------------|
| Phase 1 | Shared libraries (`internal/common`, `internal/db`, `internal/cache`) | Service-specific business logic |
| Phase 2 | SQL schema migrations, table definitions | Service endpoint handlers |
| Phase 3 | UEAU, SDM, UECM handlers and business logic, notify engine | EE, PP, MT, SSAU, NIDDAU, RSDS |
| Phase 4 | UEID (SUCI deconceal), NRF client | Security hardening, observability dashboards |
| Phase 5 | EE, PP, MT handlers and business logic | SSAU, NIDDAU, RSDS |
| Phase 6 | SSAU, NIDDAU, RSDS handlers and business logic | Infrastructure deployment manifests |
| Phase 7 | Security middleware, encryption, audit logging | Performance optimization |
| Phase 8 | Metrics, tracing, logging, alarms, dashboards | Deployment manifests |
| Phase 9 | K8s manifests, Istio config, YugabyteDB geo-config | Load test scripts |
| Phase 10 | Performance test scripts, optimization patches | E2E test harnesses |
| Phase 11 | E2E test suites, 3GPP conformance tests | New features |

---

## 6. Sync Enforcement Rule

> **CRITICAL: If new code introduces behavior not described in the design documents:**
> - **STOP**
> - **Suggest updating the relevant design document(s) in `./docs/` FIRST**
> - Then implement the code change
>
> This ensures the design documents remain the authoritative source of truth.

### 6.1 When to Flag a Doc Update

- Adding a new API endpoint not in `docs/sbi-api-design.md`
- Adding a new database table not in `docs/data-model.md`
- Changing a procedure flow that differs from `docs/sequence-diagrams.md`
- Modifying service responsibilities beyond what's defined in `docs/service-decomposition.md`
- Adding a new shared library not documented in `docs/service-decomposition.md` §3
- Changing security controls not described in `docs/security.md`

---

## 7. Code Reference Comments

When generating code, include reference comments pointing to the relevant design documentation and 3GPP specification:

```go
// Based on: docs/service-decomposition.md §2.1 (udm-ueau)
// 3GPP: TS 29.503 Nudm_UEAU — GenerateAuthData
// 3GPP: TS 33.501 §6.1.3 — 5G-AKA authentication procedure
func (s *UEAUService) GenerateAuthData(ctx context.Context, req *AuthenticationInfoRequest) (*AuthenticationInfoResult, error) {
    // ...
}
```

```go
// Based on: docs/data-model.md §3 (YugabyteDB Schema)
// 3GPP: TS 29.505 — Authentication Subscription Data
const createAuthDataTable = `
CREATE TABLE authentication_data (
    supi TEXT PRIMARY KEY,
    auth_method TEXT NOT NULL,
    ...
);`
```

```go
// Based on: docs/sequence-diagrams.md §2 (5G UE Registration Flow)
// 3GPP: TS 23.502 §4.2.2.2 — Registration Procedure
func (h *UECMHandler) RegisterAMF3GPPAccess(w http.ResponseWriter, r *http.Request) {
    // ...
}
```

---

## 8. Anti-Drift Rule

### 8.1 Prevent Divergence Between Code, Roadmap, and Design Docs

The following sources **MUST remain synchronized** at all times:

```
  Code (*.go, *.sql, *.yaml)
       ↕ must match
  IMPLEMENTATION_ROADMAP.md
       ↕ must match
  ./docs/* (design documents)
       ↕ must align with
  ./docs/3gpp/* (3GPP standards)
```

### 8.2 Drift Detection Checklist

Before completing any code change, verify:

- [ ] API endpoints match `docs/sbi-api-design.md` endpoint catalog
- [ ] Database schemas match `docs/data-model.md` table definitions
- [ ] Service responsibilities match `docs/service-decomposition.md` service catalog
- [ ] Procedure flows match `docs/sequence-diagrams.md` sequence diagrams
- [ ] Security controls match `docs/security.md` requirements
- [ ] Performance characteristics align with `docs/performance.md` targets
- [ ] The change is within scope of the current active phase in `IMPLEMENTATION_ROADMAP.md`

### 8.3 What to Do on Drift Detection

1. **Stop** the current implementation task
2. **Identify** which document is out of date (code vs. doc)
3. **Update** the design document if the change is intentional and approved
4. **Update** `IMPLEMENTATION_ROADMAP.md` if the change affects phase scope or deliverables
5. **Resume** implementation once documents are synchronized

---

## 9. Technology Constraints

| Constraint | Requirement | Reference |
|------------|-------------|-----------|
| Language | Go (Golang) | `docs/architecture.md` §10 |
| Database driver | pgx (PostgreSQL wire protocol) | `docs/service-decomposition.md` §3.3 |
| Database | YugabyteDB (YSQL) | `docs/data-model.md` |
| Cache | Redis Cluster + in-memory (Ristretto or sync.Map) | `docs/service-decomposition.md` §3.4 |
| HTTP | HTTP/2 mandatory, TLS 1.2+ in production | `docs/sbi-api-design.md` §1 |
| Serialization | JSON (application/json) | `docs/sbi-api-design.md` §9 |
| Telemetry | OpenTelemetry SDK | `docs/observability.md` §1 |
| Logging | Structured JSON (zerolog or slog) | `docs/service-decomposition.md` §3.2 |
| Container base | gcr.io/distroless/static-debian12 | `docs/architecture.md` §4.4 |
| Build | CGO_ENABLED=0, static linking | `docs/architecture.md` §4.4 |

---

## 10. Key Design Decisions to Respect

These decisions are documented in the design docs and must NOT be overridden:

1. **No separate UDR** — All services access YugabyteDB directly (`docs/architecture.md` §1.4)
2. **1:1 service-to-spec mapping** — Each microservice owns exactly one Nudm API root (`docs/service-decomposition.md` §1.1)
3. **SUPI as root key** — All subscriber data is anchored by SUPI for hash-based sharding (`docs/data-model.md` §1.1)
4. **JSONB for nested 3GPP structures** — Complex 3GPP objects stored as JSONB to avoid excessive normalization (`docs/data-model.md` §1.1)
5. **Stateless services** — All pods are freely replaceable; no local state (`docs/architecture.md` §4.2)
6. **Two-tier caching** — In-memory L1 + Redis L2, write-through invalidation (`docs/service-decomposition.md` §3.4)
7. **Strong consistency for registrations** — UECM writes use Raft leader writes (`docs/service-decomposition.md` §2.3)
8. **Exponential backoff with circuit breaker** for notification callbacks (`docs/service-decomposition.md` §3.5)
