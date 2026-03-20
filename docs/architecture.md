# 5G Core Unified Data Management (UDM) — High-Level Architecture

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [System Overview](#2-system-overview)
3. [High-Level Architecture](#3-high-level-architecture)
4. [Cloud-Native Design Principles](#4-cloud-native-design-principles)
5. [UDM Service Layer](#5-udm-service-layer)
6. [Multi-Region Active-Active Architecture](#6-multi-region-active-active-architecture)
7. [Key Design Decisions](#7-key-design-decisions)
8. [Scalability Architecture](#8-scalability-architecture)
9. [High Availability and Fault Tolerance](#9-high-availability-and-fault-tolerance)
10. [Technology Stack](#10-technology-stack)
11. [Capacity Planning](#11-capacity-planning)
12. [Security Architecture](#12-security-architecture)
13. [Observability and Operations](#13-observability-and-operations)
14. [Data Model Overview](#14-data-model-overview)
15. [Deployment Architecture](#15-deployment-architecture)
16. [References](#16-references)

---

## 1. Executive Summary

### 1.1 Purpose

This document defines the high-level architecture for a telecom-grade **5G Core Unified
Data Management (UDM)** network function. The UDM is the authoritative source of
subscriber data, authentication credentials, and registration state within a 5G
Standalone (SA) core network. It replaces the legacy HSS/HLR functions from 4G/3G
and serves as the central subscriber intelligence hub for all 5G network functions.

### 1.2 Scope

The architecture covers:

- All **11 Nudm service-based interfaces** as specified in 3GPP TS 29.503, including
  SDM, UECM, PP, EE, UEAU, SSAU, NIDDAU, MT, RSDS, and UEID.
- **Subscription data storage** per 3GPP TS 29.505, managed directly by the UDM
  without a separate UDR component.
- **Multi-region active-active deployment** supporting 10 million to 100 million
  subscribers across geographically distributed data centers.
- **Cloud-native, Kubernetes-native** design with stateless Golang microservices
  backed by YugabyteDB distributed SQL.

### 1.3 Compliance Targets

| 3GPP Specification | Title | Relevance |
|--------------------|-------|-----------|
| **TS 23.501** | System Architecture for the 5G System | UDM role within SBA, NF interactions |
| **TS 23.502** | Procedures for the 5G System | Call flows involving UDM |
| **TS 23.503** | Policy and Charging Control Framework | PCF-UDM subscriber policy data |
| **TS 29.500** | Technical Realization of Service Based Architecture | SBI protocol requirements |
| **TS 29.503** | Nudm Services (UDM SBI) | All 11 Nudm API specifications |
| **TS 29.505** | Usage of the Unified Data Repository | Subscription data schema |
| **TS 33.501** | Security Architecture and Procedures | Authentication vectors, SUPI/SUCI, key derivation |
| **TS 23.003** | Numbering, Addressing and Identification | SUPI, GPSI, SUCI formats |

### 1.4 Design Philosophy

This UDM implementation follows the principle of **operational simplicity at scale**.
Rather than decomposing into UDM + UDR as optionally permitted by 3GPP TS 23.501
§6.2.11, we consolidate the UDM application logic and data storage access into a
single logical network function. This eliminates inter-NF latency between UDM and
UDR, reduces failure domains, and simplifies deployment topology — while remaining
fully compliant with all external SBI contracts defined in TS 29.503.

---

## 2. System Overview

### 2.1 UDM Role in the 5G Core

The UDM is a mandatory network function in the 5G System Architecture (TS 23.501).
It is responsible for:

- **Subscriber Data Management** — serving access and mobility data, session
  management data, SMS subscription data, and slice selection information to
  consuming NFs.
- **Authentication Credential Processing** — generating 5G-AKA and EAP-AKA'
  authentication vectors from long-term subscriber keys (K, OPc).
- **UE Context Management** — maintaining registrations of serving AMF, SMF, and
  SMSF instances for each subscriber.
- **Event Exposure** — notifying subscribed NFs of changes in subscriber state,
  reachability, location, and connectivity.
- **SUCI Deconcalment** — resolving the Subscription Concealed Identifier (SUCI)
  back to the Subscription Permanent Identifier (SUPI) using the home network
  private key.
- **Service-Specific Authorization** — authorizing NIDD configurations and other
  service-specific requests.

### 2.2 NF Interaction Map

The UDM interacts with the following 5G Core network functions over the SBI:

| Consumer NF | Nudm Services Used | Primary Interactions |
|-------------|-------------------|----------------------|
| **AMF** | SDM, UECM, MT | Subscriber data retrieval, AMF registration, UE reachability |
| **AUSF** | UEAU, UEID | Authentication vector generation, SUCI de-concealment |
| **SMF** | SDM, UECM | Session management data, SMF registration |
| **SMSF** | SDM, UECM, MT, RSDS | SMS subscription data, SMSF registration, SM delivery status |
| **NEF** | SDM, EE, PP, SSAU, NIDDAU | Event subscriptions, parameter provisioning, NIDD authorization |
| **PCF** | SDM | Policy-related subscription data |
| **NRF** | — (producer) | UDM NF profile registration and discovery |
| **NWDAF** | EE | Analytics event subscriptions |
| **AF** (via NEF) | PP, EE, SSAU | Application-level provisioning and monitoring |
| **HSS/IWF** | UECM, UEAU | 4G/5G interworking via N26 interface |

### 2.3 Service-Based Interface Endpoints

The UDM exposes the following Nudm service endpoints per TS 29.503:

| Service | API Root | Version | Description |
|---------|----------|---------|-------------|
| **Nudm_SDM** | `/nudm-sdm/v2` | 2.3.6 | Subscriber Data Management |
| **Nudm_UECM** | `/nudm-uecm/v1` | 1.3.3 | UE Context Management |
| **Nudm_PP** | `/nudm-pp/v1` | 1.3.3 | Parameter Provision |
| **Nudm_EE** | `/nudm-ee/v1` | 1.3.1 | Event Exposure |
| **Nudm_UEAU** | `/nudm-ueau/v1` | 1.3.2 | UE Authentication |
| **Nudm_SSAU** | `/nudm-ssau/v1` | 1.1.1 | Service Specific Authorization |
| **Nudm_NIDDAU** | `/nudm-niddau/v1` | 1.2.0 | NIDD Authorization |
| **Nudm_MT** | `/nudm-mt/v1` | 1.2.0 | Mobile Terminated services |
| **Nudm_RSDS** | `/nudm-rsds/v1` | 1.2.0 | Report SM Delivery Status |
| **Nudm_UEID** | `/nudm-ueid/v1` | 1.0.0 | UE Identifier (SUCI de-concealment) |

---

## 3. High-Level Architecture

### 3.1 System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          5G Core Network Functions                          │
│                                                                             │
│   ┌───────┐  ┌───────┐  ┌───────┐  ┌───────┐  ┌───────┐  ┌───────┐       │
│   │  AMF  │  │ AUSF  │  │  SMF  │  │ SMSF  │  │  NEF  │  │  PCF  │       │
│   └───┬───┘  └───┬───┘  └───┬───┘  └───┬───┘  └───┬───┘  └───┬───┘       │
│       │          │          │          │          │          │              │
│       └──────────┴──────────┴──────────┴──────────┴──────────┘              │
│                                    │                                        │
│                             SBI (HTTP/2 + JSON)                             │
│                             OAuth2 Secured                                  │
└────────────────────────────────────┼────────────────────────────────────────┘
                                     │
                    ┌────────────────────────────────┐
                    │     Global Load Balancer /      │
                    │     Service Mesh (Istio)        │
                    │     + NRF-based Discovery       │
                    └───────────────┬────────────────┘
                                    │
          ┌─────────────────────────┼─────────────────────────┐
          │                         │                         │
          ▼                         ▼                         ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│   Region A (US)  │  │  Region B (EU)   │  │  Region C (APAC) │
│                  │  │                  │  │                  │
│ ┌──────────────┐ │  │ ┌──────────────┐ │  │ ┌──────────────┐ │
│ │  K8s Cluster │ │  │ │  K8s Cluster │ │  │ │  K8s Cluster │ │
│ │              │ │  │ │              │ │  │ │              │ │
│ │  ┌────────┐  │ │  │ │  ┌────────┐  │ │  │ │  ┌────────┐  │ │
│ │  │ SDM    │  │ │  │ │  │ SDM    │  │ │  │ │  │ SDM    │  │ │
│ │  │ UECM   │  │ │  │ │  │ UECM   │  │ │  │ │  │ UECM   │  │ │
│ │  │ UEAU   │  │ │  │ │  │ UEAU   │  │ │  │ │  │ UEAU   │  │ │
│ │  │ EE     │  │ │  │ │  │ EE     │  │ │  │ │  │ EE     │  │ │
│ │  │ PP     │  │ │  │ │  │ PP     │  │ │  │ │  │ PP     │  │ │
│ │  │ MT     │  │ │  │ │  │ MT     │  │ │  │ │  │ MT     │  │ │
│ │  │ UEID   │  │ │  │ │  │ UEID   │  │ │  │ │  │ UEID   │  │ │
│ │  │ SSAU   │  │ │  │ │  │ SSAU   │  │ │  │ │  │ SSAU   │  │ │
│ │  │ NIDDAU │  │ │  │ │  │ NIDDAU │  │ │  │ │  │ NIDDAU │  │ │
│ │  │ RSDS   │  │ │  │ │  │ RSDS   │  │ │  │ │  │ RSDS   │  │ │
│ │  └────────┘  │ │  │ │  └────────┘  │ │  │ │  └────────┘  │ │
│ │  Stateless   │ │  │ │  Stateless   │ │  │ │  Stateless   │ │
│ │  Go Services │ │  │ │  Go Services │ │  │ │  Go Services │ │
│ └──────┬───────┘ │  │ └──────┬───────┘ │  │ └──────┬───────┘ │
│        │         │  │        │         │  │        │         │
│ ┌──────▼───────┐ │  │ ┌──────▼───────┐ │  │ ┌──────▼───────┐ │
│ │  YugabyteDB  │ │  │ │  YugabyteDB  │ │  │ │  YugabyteDB  │ │
│ │  Tablet Srvr │◄├──┼─┤► Tablet Srvr │◄├──┼─┤► Tablet Srvr │ │
│ │  (RF=3)      │ │  │ │  (RF=3)      │ │  │ │  (RF=3)      │ │
│ └──────────────┘ │  │ └──────────────┘ │  │ └──────────────┘ │
│                  │  │                  │  │                  │
│ ┌──────────────┐ │  │ ┌──────────────┐ │  │ ┌──────────────┐ │
│ │ Redis Cache  │ │  │ │ Redis Cache  │ │  │ │ Redis Cache  │ │
│ │ (Regional)   │ │  │ │ (Regional)   │ │  │ │ (Regional)   │ │
│ └──────────────┘ │  │ └──────────────┘ │  │ └──────────────┘ │
└──────────────────┘  └──────────────────┘  └──────────────────┘

         ◄──────── Synchronous Raft Replication ────────►
```

### 3.2 Component Summary

| Layer | Components | Responsibility |
|-------|-----------|----------------|
| **NF Consumer** | AMF, AUSF, SMF, SMSF, NEF, PCF | Issue Nudm service requests over SBI |
| **Ingress** | Global LB, Istio Gateway, NRF | Traffic routing, TLS termination, NF discovery |
| **Service** | 10 stateless Go microservices | Nudm API logic, auth vector generation, SUCI handling |
| **Cache** | Regional Redis clusters | Hot subscriber data, session affinity hints |
| **Storage** | YugabyteDB (geo-distributed) | Subscriber records, auth credentials, registrations |

---

## 4. Cloud-Native Design Principles

### 4.1 Kubernetes-Native Deployment

The UDM is designed as a first-class Kubernetes workload:

- **Deployments** — each Nudm service runs as an independent `Deployment` with
  configurable replica counts. Services with higher traffic (SDM, UECM, UEAU) can
  be scaled independently of lower-traffic services (NIDDAU, RSDS).
- **Horizontal Pod Autoscaler (HPA)** — CPU and custom metric-based autoscaling
  ensures each service scales with demand.
- **Pod Disruption Budgets (PDB)** — guarantee minimum availability during node
  drains and cluster upgrades.
- **Readiness / Liveness / Startup Probes** — health checks on each service endpoint
  ensure traffic is only routed to healthy pods.
- **Service Accounts and RBAC** — least-privilege access for each service component.

### 4.2 Stateless Service Layer

All UDM service pods are **fully stateless**:

- No local disk state or in-memory session data that cannot be reconstructed.
- Any pod can serve any subscriber request — no session affinity required at the
  application level.
- All mutable state is persisted in YugabyteDB.
- Subscriber caches (Redis) are warm caches only; cache misses fall through to the
  database transparently.

This enables:
- Instant horizontal scale-out by adding pods.
- Zero-downtime rolling deployments.
- Seamless pod rescheduling across nodes.

### 4.3 Twelve-Factor App Compliance

| Factor | Implementation |
|--------|---------------|
| **I. Codebase** | Single monorepo, one codebase per service, tracked in Git |
| **II. Dependencies** | Go modules (`go.mod`) with vendored dependencies |
| **III. Config** | Environment variables and Kubernetes ConfigMaps/Secrets |
| **IV. Backing Services** | YugabyteDB and Redis accessed via connection strings |
| **V. Build, Release, Run** | CI/CD pipeline produces immutable container images |
| **VI. Processes** | Stateless processes; shared-nothing architecture |
| **VII. Port Binding** | Each service self-hosts HTTP/2 on a configured port |
| **VIII. Concurrency** | Scale out via process replication (K8s replicas) |
| **IX. Disposability** | Fast startup (<2s), graceful shutdown with connection draining |
| **X. Dev/Prod Parity** | Identical container images across all environments |
| **XI. Logs** | Structured JSON logs written to stdout, collected by Fluentd/OTel |
| **XII. Admin Processes** | Database migrations and one-off tasks run as K8s Jobs |

### 4.4 Container-Based Packaging

- **Base image**: `gcr.io/distroless/static-debian12` — minimal attack surface,
  no shell, no package manager.
- **Binary**: Statically linked Go binary compiled with `CGO_ENABLED=0`.
- **Image size**: Target < 30 MB per service image.
- **Vulnerability scanning**: Trivy scan integrated into CI pipeline; zero critical
  or high CVEs policy.
- **Image signing**: Cosign signatures for supply chain integrity.

---

## 5. UDM Service Layer

### 5.1 Service Decomposition

Each Nudm service is implemented as an independent Go microservice. Services share
common libraries but are built, deployed, and scaled independently.

```
┌────────────────────────────────────────────────────────────────┐
│                     UDM Service Layer                          │
│                                                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │  nudm-   │  │  nudm-   │  │  nudm-   │  │  nudm-   │      │
│  │  sdm     │  │  uecm    │  │  ueau    │  │  ee      │      │
│  │          │  │          │  │          │  │          │      │
│  │ AM Data  │  │ AMF Reg  │  │ 5G-AKA   │  │ Monitor  │      │
│  │ SM Data  │  │ SMF Reg  │  │ EAP-AKA' │  │ Notify   │      │
│  │ NSSAI    │  │ SMSF Reg │  │ SQN Mgmt │  │ Callback │      │
│  │ SMS Data │  │ Routing  │  │ GBA      │  │ Mgmt     │      │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘      │
│                                                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │  nudm-   │  │  nudm-   │  │  nudm-   │  │  nudm-   │      │
│  │  pp      │  │  mt      │  │  ueid    │  │  ssau    │      │
│  │          │  │          │  │          │  │          │      │
│  │ Param    │  │ UE Info  │  │ SUCI     │  │ Service  │      │
│  │ Provision│  │ Location │  │ De-      │  │ Authz    │      │
│  │ VN Group │  │ Query    │  │ conceal  │  │          │      │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘      │
│                                                                │
│  ┌──────────┐  ┌──────────┐                                   │
│  │  nudm-   │  │  nudm-   │  ┌──────────────────────────────┐│
│  │  niddau  │  │  rsds    │  │   Shared Libraries           ││
│  │          │  │          │  │                               ││
│  │ NIDD     │  │ SM Delv  │  │  • DB access (pgx driver)    ││
│  │ Authz    │  │ Status   │  │  • Auth (Milenage, Tuak)     ││
│  └──────────┘  └──────────┘  │  • SBI codec (HTTP/2+JSON)   ││
│                               │  • Telemetry (OTel SDK)      ││
│                               │  • Config / health / logging ││
│                               └──────────────────────────────┘│
└────────────────────────────────────────────────────────────────┘
```

### 5.2 Service Traffic Profile

Based on typical 5G network traffic patterns, services are categorized by expected
request volume:

| Tier | Services | Typical RPS (per 10M subs) | Scaling Priority |
|------|----------|---------------------------|-----------------|
| **High** | SDM, UECM, UEAU | 50,000 – 200,000 | Aggressive HPA, priority scheduling |
| **Medium** | EE, PP, MT | 5,000 – 30,000 | Standard HPA |
| **Low** | SSAU, NIDDAU, RSDS, UEID | 500 – 5,000 | Minimum replicas with burst capacity |

### 5.3 Internal Service Architecture

Each Go microservice follows a consistent layered architecture:

```
┌─────────────────────────────────────────┐
│          HTTP/2 Transport Layer         │
│    (net/http Server, TLS 1.3, H2C)     │
├─────────────────────────────────────────┤
│         API Handler / Router            │
│    (OpenAPI-generated from 3GPP YAML)   │
├─────────────────────────────────────────┤
│          Business Logic Layer           │
│   (3GPP procedure implementation)       │
├─────────────────────────────────────────┤
│         Data Access Layer (DAL)         │
│   (SQL queries, connection pooling)     │
├─────────────────────────────────────────┤
│      PostgreSQL Wire Protocol (pgx)     │
│         ──► YugabyteDB YSQL            │
└─────────────────────────────────────────┘
```

---

## 6. Multi-Region Active-Active Architecture

### 6.1 Topology Overview

The UDM is deployed across **three or more geographic regions** in an active-active
configuration. Every region can independently serve all Nudm API requests for any
subscriber, providing true geo-redundancy.

```
                    ┌───────────────────────┐
                    │   Global DNS / GSLB   │
                    │  (Geo-aware routing)  │
                    └───────────┬───────────┘
                                │
            ┌───────────────────┼───────────────────┐
            │                   │                   │
            ▼                   ▼                   ▼
     ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
     │  Region A   │    │  Region B   │    │  Region C   │
     │  (Primary)  │    │  (Primary)  │    │  (Primary)  │
     │             │    │             │    │             │
     │ UDM Pods    │    │ UDM Pods    │    │ UDM Pods    │
     │ Redis Cache │    │ Redis Cache │    │ Redis Cache │
     │ YB Tablets  │    │ YB Tablets  │    │ YB Tablets  │
     └──────┬──────┘    └──────┬──────┘    └──────┬──────┘
            │                  │                  │
            └──────────────────┼──────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │    YugabyteDB       │
                    │  Raft Consensus     │
                    │  (Cross-Region)     │
                    └─────────────────────┘
```

### 6.2 Geo-Redundant YugabyteDB Deployment

YugabyteDB is deployed as a **single logical cluster spanning three or more regions**
with the following configuration:

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| **Replication Factor** | 3 (one replica per region) | Survive single-region failure |
| **Consistency** | Raft consensus (strong) | ACID guarantees for subscriber data |
| **Tablet Count** | Auto-split based on data size | ~4 GB per tablet target |
| **Placement Policy** | `cloud.region.zone` | Data distributed across all regions |
| **Preferred Leaders** | Geo-affinity per SUPI range | Minimize write latency for local subs |
| **Read Replicas** | Optional per region | Offload read-heavy SDM queries |

**Leader Placement Strategy**: YugabyteDB tablet leaders are placed in the region
closest to the subscriber's home PLMN. For a subscriber with SUPI `imsi-31026...`
(US MCC 310), the tablet leader is preferentially placed in the US region. This
minimizes write latency for the most common operations (registration updates) while
reads from any region are served with strong consistency via Raft.

### 6.3 Global Load Balancing Strategy

Traffic routing follows a three-tier model:

1. **Global DNS (GSLB)** — GeoDNS or Anycast routes NF consumers to the nearest
   regional UDM cluster. Failover to alternate regions on health check failure.

2. **Regional Ingress (Istio Gateway)** — TLS termination, HTTP/2 multiplexing,
   and request routing to the appropriate Nudm service within the region.

3. **NRF-based Discovery** — consuming NFs discover UDM instances through the NRF
   (per TS 29.510). Multiple UDM NF profiles are registered, one per region,
   enabling NF-level failover.

### 6.4 Cross-Region Data Replication

| Data Category | Replication Mode | Consistency | Latency Impact |
|---------------|-----------------|-------------|----------------|
| **Authentication credentials** (K, OPc, SQN) | Synchronous Raft | Strong | Write: +10-40ms cross-region |
| **Subscriber profile** (AM, SM, NSSAI) | Synchronous Raft | Strong | Write: +10-40ms cross-region |
| **Registration state** (AMF, SMF, SMSF) | Synchronous Raft | Strong | Write: +10-40ms cross-region |
| **Event subscriptions** (EE callbacks) | Synchronous Raft | Strong | Write: +10-40ms cross-region |
| **Cache (Redis)** | Regional-only (no replication) | Eventual | No cross-region cost |

### 6.5 Consistency Model

The UDM requires **strong consistency** for all subscriber data operations:

- **Authentication (UEAU)** — SQN (sequence number) increments MUST be strictly
  serialized to prevent replay attacks. YugabyteDB's Raft-based replication
  guarantees that concurrent auth requests from different regions see a consistent
  SQN, even during region failover.

- **Registration (UECM)** — AMF/SMF registration updates are mutually exclusive per
  access type. Strong consistency prevents split-brain where two regions believe
  different AMFs are serving the same subscriber.

- **Profile Reads (SDM)** — read-after-write consistency ensures that a profile
  update via PP is immediately visible to subsequent SDM reads from any region.

**Trade-off**: Cross-region Raft consensus adds 10-40ms to write latency depending on
inter-region network distance. This is acceptable because:
- UDM write operations (registration, auth) are infrequent per subscriber (~1-10/hour).
- UDM read operations (SDM queries) can leverage follower reads or Redis cache for
  sub-millisecond latency when strict freshness is not required.

---

## 7. Key Design Decisions

### 7.1 No Separate UDR Component

**Decision**: The UDM accesses YugabyteDB directly via SQL without an intermediate
UDR network function.

**Rationale**:

| Factor | UDM + UDR (standard) | UDM Direct-to-DB (this design) |
|--------|---------------------|-------------------------------|
| **Latency** | +2-5ms per UDR hop | Direct SQL access |
| **Failure domains** | UDM and UDR can fail independently | Single service to manage |
| **Operational complexity** | Two NFs to deploy, scale, monitor | One NF with clear ownership |
| **3GPP compliance** | Normative architecture | Permitted; external SBI behavior identical |
| **Data sharing** | UDR can serve multiple NFs | UDM owns its data exclusively |

The 3GPP architecture (TS 23.501 §6.2.11) describes UDR as a reference point for
data access, not a mandatory deployment boundary. As long as the Nudm SBI contracts
are correctly implemented, the internal data access mechanism is an implementation
choice. This design consolidates UDM + UDR into a single deployable unit.

**Trade-off acknowledged**: If other NFs (e.g., PCF, NEF) need direct access to
subscription data, they must go through the UDM's Nudm APIs rather than a shared UDR.
This is the preferred security model, as it centralizes access control.

### 7.2 YugabyteDB as Primary Data Store

**Decision**: YugabyteDB (YSQL — PostgreSQL-compatible distributed SQL) as the sole
persistent data store for all subscriber data.

**Rationale**:

- **Distributed SQL** — ACID transactions across a geo-distributed cluster without
  application-level sharding logic.
- **PostgreSQL compatibility** — mature ecosystem, pgx driver for Go, standard SQL,
  well-understood operational patterns.
- **Automatic sharding** — hash-based and range-based table partitioning with
  transparent tablet splitting.
- **Raft consensus** — strong consistency with configurable replication factor.
- **Online schema changes** — non-blocking DDL for zero-downtime migrations.
- **Proven at scale** — production deployments handling billions of rows and hundreds
  of thousands of TPS.

### 7.3 Golang for Service Layer

**Decision**: Go 1.22+ for all UDM service implementations.

**Rationale**:

- **Performance** — compiled language with low latency and efficient memory usage;
  garbage collector optimized for low-pause workloads.
- **Concurrency** — goroutines and channels provide lightweight concurrency for
  handling thousands of concurrent SBI requests per pod.
- **Small binaries** — statically compiled binaries (~15-25 MB) with fast cold start
  (<1s), essential for Kubernetes pod scheduling.
- **Crypto libraries** — native support for TLS 1.3, ECDHE, and AES-GCM required by
  TS 33.501; plus community Milenage/Tuak libraries for 3GPP auth.
- **Cloud-native ecosystem** — first-class support for gRPC, Prometheus client,
  OpenTelemetry SDK, and Kubernetes client libraries.

### 7.4 HTTP/2 + JSON for SBI Interfaces

Per TS 29.500 §5.2, the 5G SBI uses HTTP/2 as the transport protocol with JSON
(RFC 8259) as the serialization format. The UDM implements:

- **HTTP/2 with TLS 1.3** — mandatory for SBI per TS 33.501.
- **HTTP/2 cleartext (h2c)** — supported for service-mesh-internal traffic where
  mTLS is handled at the sidecar (Istio Envoy) level.
- **JSON encoding** — `encoding/json` with optional `json-iterator` for performance-
  critical paths.
- **Content-Type**: `application/json` for request/response bodies as specified by
  the Nudm OpenAPI definitions.
- **3GPP custom headers** — `3gpp-Sbi-Target-apiRoot`, `3gpp-Sbi-Callback`,
  `3gpp-Sbi-Oci` as defined in TS 29.500.

### 7.5 OAuth2 for Service Authentication

Per TS 33.501 §13.3 and TS 29.510, NF-to-NF authentication uses OAuth 2.0 Client
Credentials grant:

- Consuming NFs obtain an access token from the NRF.
- The UDM validates the token's `scope` claim against the requested Nudm service
  (e.g., `nudm-sdm`, `nudm-ueau`).
- Token validation uses NRF's public key (JWKS) for offline verification — no
  per-request call to NRF.
- mTLS between NFs provides transport-level authentication as a defense-in-depth
  measure.

---

## 8. Scalability Architecture

### 8.1 Horizontal Scaling Model

The UDM is designed to scale linearly from **10 million to 100 million subscribers**
by independently scaling the service layer and data layer:

```
                Subscribers:   10M         50M         100M
                              ────►       ────►       ────►

  Service Pods (per region):   30          150          300
  YugabyteDB Nodes (total):    9           27           54
  Redis Nodes (per region):    3            6           12
  Total TPS Capacity:        100K         500K        1,000K
```

### 8.2 Data Partitioning Strategy

**SUPI-Based Hash Sharding**

All subscriber data tables use the SUPI (Subscription Permanent Identifier) as the
primary partition key. YugabyteDB automatically hash-partitions data across tablets
based on the SUPI:

```sql
CREATE TABLE subscription_data (
    supi        TEXT        NOT NULL,
    data_type   TEXT        NOT NULL,
    plmn_id     TEXT,
    data        JSONB       NOT NULL,
    version     BIGINT      NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (supi, data_type)
) SPLIT INTO 128 TABLETS;
```

**Sharding Characteristics**:

| Aspect | Strategy |
|--------|----------|
| **Partition key** | SUPI (hash-distributed) |
| **Sort key** | `data_type` (AM, SM, NSSAI, etc.) |
| **Initial tablets** | 128 per table (auto-split at 4 GB) |
| **Data locality** | All data for one subscriber co-located in same tablet |
| **Cross-subscriber queries** | Scatter-gather (rare in UDM workloads) |

This ensures that all data for a single subscriber (profile, registrations, auth
state) is co-located, enabling efficient single-row and single-partition transactions
for the dominant access patterns.

### 8.3 Connection Pooling

Each Go service instance manages a connection pool to YugabyteDB:

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| **Max open connections** | 50 per pod | Balance throughput vs. DB connection cost |
| **Max idle connections** | 25 per pod | Keep warm connections for burst traffic |
| **Connection max lifetime** | 30 minutes | Prevent stale connections after DB failover |
| **Connection max idle time** | 5 minutes | Release unused connections during low traffic |
| **Health check interval** | 30 seconds | Detect broken connections proactively |

At scale (300 pods per region × 50 connections), the DB cluster handles ~15,000
connections per region. YugabyteDB supports this through its connection manager and
Golang's `pgxpool` efficiently multiplexes queries over pooled connections.

### 8.4 Caching Strategy

A two-tier caching approach minimizes database load for read-heavy operations:

**Tier 1: In-Process Cache (Go sync.Map / Ristretto)**

- **What**: Subscriber profile data, PLMN configuration, NSSAI mappings.
- **TTL**: 30-60 seconds.
- **Size**: 100-500 MB per pod (configurable).
- **Invalidation**: TTL-based expiry; SDM subscription notifications trigger
  proactive eviction.
- **Hit rate target**: >60% for SDM read operations.

**Tier 2: Regional Redis Cluster**

- **What**: Hot subscriber data, authentication SQN cache, registration state.
- **TTL**: 5-15 minutes (data-type dependent).
- **Topology**: Redis Cluster with 3-12 nodes per region.
- **Invalidation**: Write-through on UDM mutations; TTL expiry as safety net.
- **Hit rate target**: >85% combined with Tier 1.

```
  Request ──► In-Process Cache ──(miss)──► Redis ──(miss)──► YugabyteDB
                  (< 1μs)                (< 1ms)              (1-10ms)
```

**Cache-aside pattern**: The UDM service first checks the in-process cache, then
Redis, then the database. Writes always go to the database first, then update Redis,
then invalidate the in-process cache across pods via a pub/sub notification channel.

---

## 9. High Availability and Fault Tolerance

### 9.1 Availability Target

| Metric | Target |
|--------|--------|
| **Service availability** | 99.999% (five nines) |
| **Planned downtime** | Zero (rolling upgrades) |
| **Unplanned downtime** | < 5.26 minutes/year |
| **RPO (Recovery Point Objective)** | 0 (no data loss — synchronous replication) |
| **RTO (Recovery Time Objective)** | < 10 seconds (pod-level), < 30 seconds (region-level) |

### 9.2 Node-Level Failover

| Failure Scenario | Detection | Recovery | Impact |
|-----------------|-----------|----------|--------|
| **UDM pod crash** | K8s liveness probe (5s) | ReplicaSet restarts pod (< 10s) | Other pods serve traffic; no subscriber impact |
| **K8s node failure** | Node heartbeat timeout (40s) | Pods rescheduled to healthy nodes | Brief capacity reduction; HPA compensates |
| **YugabyteDB TServer crash** | Raft heartbeat (1.5s) | Leader election on surviving replicas (< 5s) | Transparent to UDM; pgx retries in-flight queries |
| **Redis node failure** | Sentinel/Cluster detection (5s) | Redis Cluster promotes replica (< 10s) | Cache misses served from DB during failover |

### 9.3 Region-Level Failover

When an entire region becomes unavailable:

1. **GSLB detects failure** — health checks fail for the region's UDM endpoints.
   DNS TTL is set to 30 seconds for rapid failover.
2. **Traffic reroutes** — NF consumers are directed to the next-nearest region.
3. **YugabyteDB elects new leaders** — tablet leaders in the failed region are
   re-elected to surviving regions within 5-10 seconds (Raft leader lease timeout).
4. **No data loss** — synchronous Raft replication ensures all committed writes are
   available in surviving regions.
5. **Capacity scales up** — HPA in surviving regions detects increased load and
   scales UDM pods accordingly.

### 9.4 Database Replication Factor

| Configuration | RF | Regions | Fault Tolerance |
|--------------|-----|---------|-----------------|
| **Minimum** | 3 | 3 | Survive 1 region failure |
| **Enhanced** | 5 | 3 (with 2 AZs per region) | Survive 1 region + 1 AZ failure |
| **Maximum** | 5 | 5 | Survive 2 region failures |

The recommended production configuration is **RF=3 across 3 regions**, which provides
an optimal balance of fault tolerance, write latency, and storage overhead.

### 9.5 Zero-Downtime Rolling Upgrades

The UDM supports rolling upgrades without service interruption:

```
  Step 1: Deploy new version alongside old (canary)
  Step 2: Shift 5% traffic to new version
  Step 3: Monitor error rates and latency (15 min)
  Step 4: Gradually increase to 25% → 50% → 100%
  Step 5: Drain and terminate old version pods
```

**Upgrade compatibility requirements**:
- API versioning ensures backward compatibility (N-1 version support).
- Database schema changes are always additive (new columns, new tables).
- Destructive migrations (column drops, type changes) are deferred to N+2 releases.

### 9.6 Circuit Breaker Patterns

The UDM implements circuit breakers at multiple levels to prevent cascading failures:

| Level | Tool | Configuration |
|-------|------|---------------|
| **HTTP client** (outbound callbacks) | Go `sony/gobreaker` | Open after 5 consecutive failures; half-open after 30s |
| **Database connections** | pgx pool + custom wrapper | Open after 3 connection timeouts; half-open after 10s |
| **Redis connections** | go-redis with circuit breaker | Open after 5 failures; bypass to DB on open |
| **Service mesh** | Istio `DestinationRule` | Outlier detection: 5xx errors > 5% → eject host for 30s |

**Fallback behavior**:
- If Redis circuit is open → serve from YugabyteDB directly.
- If YugabyteDB circuit is open → return `503 Service Unavailable` with `Retry-After`
  header per TS 29.500 §5.2.7.
- If outbound callback circuit is open → queue notification for retry with
  exponential backoff.

---

## 10. Technology Stack

### 10.1 Core Technology Stack

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| **Language** | Go | 1.22+ | Service implementation |
| **Database** | YugabyteDB | 2.20+ (YSQL) | Distributed SQL data store |
| **Cache** | Redis | 7.x (Cluster mode) | Regional hot data cache |
| **In-Process Cache** | Ristretto | 0.1.x | Per-pod memory cache |
| **Protocol** | HTTP/2 + JSON | RFC 7540, RFC 8259 | 3GPP SBI transport |
| **Container Runtime** | Docker / containerd | OCI compliant | Container packaging |
| **Orchestration** | Kubernetes | 1.28+ | Workload orchestration |
| **Service Mesh** | Istio (optional) | 1.20+ | mTLS, traffic management, observability |
| **Monitoring** | Prometheus | 2.50+ | Metrics collection |
| **Tracing** | OpenTelemetry | 1.x SDK | Distributed tracing |
| **Logging** | Structured JSON (slog) | Go stdlib | Centralized log aggregation |
| **CI/CD** | GitHub Actions / Argo CD | — | Build, test, deploy |
| **DB Driver** | pgx | 5.x | PostgreSQL wire protocol for YugabyteDB |
| **API Generation** | oapi-codegen | 2.x | Go server stubs from 3GPP OpenAPI YAMLs |
| **Crypto** | Go crypto + free5gc/milenage | — | TLS 1.3, Milenage, Tuak auth algorithms |

### 10.2 Development and Testing

| Tool | Purpose |
|------|---------|
| `go test` + `testify` | Unit and integration testing |
| `golangci-lint` | Static analysis and linting |
| `mockgen` | Interface mocking for unit tests |
| `k6` / `vegeta` | Load testing and benchmarking |
| `Testcontainers` | Integration tests with real YugabyteDB |
| `buf` / `openapi-diff` | API contract validation against 3GPP specs |
| `trivy` | Container image vulnerability scanning |
| `cosign` | Image signing and verification |

---

## 11. Capacity Planning

### 11.1 Target Performance Numbers

| Metric | 10M Subscribers | 50M Subscribers | 100M Subscribers |
|--------|----------------|-----------------|------------------|
| **Peak TPS (all services)** | 100,000 | 500,000 | 1,000,000 |
| **SDM read TPS** | 60,000 | 300,000 | 600,000 |
| **UECM write TPS** | 20,000 | 100,000 | 200,000 |
| **UEAU auth TPS** | 15,000 | 75,000 | 150,000 |
| **P50 latency (SDM read)** | < 2ms | < 3ms | < 5ms |
| **P99 latency (SDM read)** | < 10ms | < 15ms | < 20ms |
| **P50 latency (UEAU auth)** | < 5ms | < 8ms | < 10ms |
| **P99 latency (UEAU auth)** | < 20ms | < 30ms | < 40ms |
| **P50 latency (UECM write)** | < 5ms | < 8ms | < 10ms |
| **P99 latency (UECM write)** | < 25ms | < 35ms | < 50ms |

### 11.2 Storage Estimates

| Data Category | Per Subscriber | 10M Total | 50M Total | 100M Total |
|---------------|---------------|-----------|-----------|------------|
| **Authentication (K, OPc, SQN)** | ~500 B | 5 GB | 25 GB | 50 GB |
| **Subscriber profile (AM, SM, NSSAI)** | ~2 KB | 20 GB | 100 GB | 200 GB |
| **Registration state** | ~500 B | 5 GB | 25 GB | 50 GB |
| **Event subscriptions** | ~300 B | 3 GB | 15 GB | 30 GB |
| **Indexes and overhead (2x)** | — | 66 GB | 330 GB | 660 GB |
| **Total (per replica)** | ~3.3 KB | **~99 GB** | **~495 GB** | **~990 GB** |
| **Total (RF=3)** | — | **~297 GB** | **~1.5 TB** | **~3 TB** |

### 11.3 Compute Estimates (Per Region)

| Component | 10M Subscribers | 50M Subscribers | 100M Subscribers |
|-----------|----------------|-----------------|------------------|
| **UDM service pods** | 30 pods (4 vCPU, 8 GB each) | 150 pods | 300 pods |
| **YugabyteDB nodes** | 3 nodes (16 vCPU, 64 GB, 500 GB SSD) | 9 nodes | 18 nodes |
| **Redis nodes** | 3 nodes (8 vCPU, 32 GB) | 6 nodes | 12 nodes |
| **Istio control plane** | 3 pods | 3 pods | 6 pods |
| **Monitoring stack** | 3 pods | 6 pods | 9 pods |

### 11.4 Network Bandwidth

| Traffic Type | 10M Subs | 100M Subs |
|-------------|----------|-----------|
| **Inbound SBI (NF → UDM)** | 2 Gbps | 20 Gbps |
| **DB traffic (UDM → YugabyteDB)** | 1 Gbps | 10 Gbps |
| **Cross-region replication** | 500 Mbps | 5 Gbps |
| **Cache traffic (UDM → Redis)** | 500 Mbps | 5 Gbps |

---

## 12. Security Architecture

### 12.1 Security Layers

```
┌────────────────────────────────────────────────────┐
│  Layer 1: Network Security                         │
│  • Kubernetes NetworkPolicies (namespace isolation) │
│  • Istio PeerAuthentication (mTLS enforcement)     │
│  • Pod Security Standards (restricted profile)     │
├────────────────────────────────────────────────────┤
│  Layer 2: Transport Security                       │
│  • TLS 1.3 for all SBI traffic (TS 33.501)       │
│  • mTLS between all services (Istio SPIFFE)       │
│  • Certificate rotation (cert-manager)            │
├────────────────────────────────────────────────────┤
│  Layer 3: Application Security                     │
│  • OAuth2 token validation (NRF-issued JWT)       │
│  • Scope-based access control per Nudm service    │
│  • Rate limiting per consumer NF                  │
├────────────────────────────────────────────────────┤
│  Layer 4: Data Security                            │
│  • Encryption at rest (YugabyteDB TDE)            │
│  • SUPI/GPSI classified as PII                    │
│  • Authentication keys (K, OPc) encrypted with KMS│
│  • Audit logging for all data access              │
└────────────────────────────────────────────────────┘
```

### 12.2 Authentication Key Protection

Long-term subscriber authentication keys (K and OPc) require special protection per
TS 33.501:

- Keys are stored encrypted in YugabyteDB using AES-256-GCM.
- Encryption keys are managed by an external KMS (e.g., HashiCorp Vault, AWS KMS).
- Key material is decrypted in-memory only during authentication vector computation
  within the UEAU service.
- No key material is written to logs, traces, or cache layers.
- HSM integration is supported for environments requiring FIPS 140-2 Level 3
  compliance.

### 12.3 SUCI De-concealment

The UEID service implements ECIES-based SUCI de-concealment (TS 33.501 §6.12):

- Home network public/private key pair stored securely (Vault or HSM).
- SUCI → SUPI resolution uses ECIES Profile A (Curve25519) or Profile B (secp256r1).
- Private key never leaves the secure enclave / HSM boundary in high-security
  deployments.

---

## 13. Observability and Operations

### 13.1 Metrics (Prometheus)

Each UDM service exposes Prometheus metrics on `/metrics`:

| Metric Category | Key Metrics |
|----------------|-------------|
| **SBI requests** | `udm_sbi_requests_total{service, method, status}` |
| **SBI latency** | `udm_sbi_request_duration_seconds{service, method}` (histogram) |
| **DB queries** | `udm_db_queries_total{table, operation}` |
| **DB latency** | `udm_db_query_duration_seconds{table, operation}` (histogram) |
| **Cache** | `udm_cache_hits_total`, `udm_cache_misses_total`, `udm_cache_hit_ratio` |
| **Connection pools** | `udm_db_pool_active`, `udm_db_pool_idle`, `udm_db_pool_waiting` |
| **Auth operations** | `udm_auth_vectors_generated_total{algorithm}` |
| **Circuit breakers** | `udm_circuit_breaker_state{target}` |
| **Business KPIs** | `udm_registrations_active`, `udm_subscribers_total` |

### 13.2 Distributed Tracing (OpenTelemetry)

- Every SBI request generates a trace with a unique `traceId`.
- Spans cover: HTTP handler → business logic → DB query → response encoding.
- Cross-NF trace context propagation via W3C `traceparent` header.
- Trace sampling: 1% in production, 100% in staging.
- Traces exported to Jaeger or Grafana Tempo.

### 13.3 Structured Logging

All services use Go's `slog` package with JSON output:

```json
{
  "time": "2025-01-15T10:30:00.123Z",
  "level": "INFO",
  "msg": "sdm_request_served",
  "service": "nudm-sdm",
  "supi": "imsi-31026*****901",
  "data_type": "am-data",
  "plmn_id": "310-260",
  "latency_ms": 2.4,
  "trace_id": "abc123def456",
  "region": "us-east-1"
}
```

**PII Handling**: SUPI and GPSI are partially masked in logs (last 6 digits visible).
Full identifiers are available only in debug-level logs, which are disabled in
production.

### 13.4 Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| **UDM High Error Rate** | 5xx rate > 1% for 5 min | Critical |
| **UDM High Latency** | P99 > 100ms for 10 min | Warning |
| **DB Connection Exhaustion** | Pool utilization > 80% for 5 min | Warning |
| **Cache Hit Rate Low** | Hit rate < 50% for 15 min | Warning |
| **Auth SQN Desync** | SQN resync rate > 1% | Critical |
| **Region Health Degraded** | > 30% pods unhealthy | Critical |
| **Certificate Expiry** | TLS cert expires < 7 days | Warning |

---

## 14. Data Model Overview

### 14.1 Core Database Schema

The database schema maps directly to the data types defined in TS 29.505. All
subscriber data is organized under the SUPI as the root identifier:

```
┌──────────────────────────────────────────────────────────┐
│                    SUPI (root key)                        │
│                  imsi-310260000000001                     │
│                                                          │
│  ├── authentication_subscription                         │
│  │     K, OPc, AMF, SQN, auth_method                    │
│  │                                                       │
│  ├── access_and_mobility_data                            │
│  │     GPSI, NSSAI, AM policies, RAT restrictions        │
│  │                                                       │
│  ├── session_management_data [per S-NSSAI, DNN]          │
│  │     PDU session type, SSC mode, QoS parameters        │
│  │                                                       │
│  ├── smf_selection_data                                  │
│  │     Subscribed S-NSSAI → DNN mappings                 │
│  │                                                       │
│  ├── sms_subscription_data                               │
│  │     SMS-over-NAS, SMS-over-IP indicators              │
│  │                                                       │
│  ├── context_data                                        │
│  │     ├── amf_3gpp_registration                         │
│  │     ├── amf_non_3gpp_registration                     │
│  │     ├── smf_registrations[]                           │
│  │     └── smsf_registration                             │
│  │                                                       │
│  ├── event_exposure_subscriptions[]                      │
│  │     Monitoring configs, callback URIs                 │
│  │                                                       │
│  ├── pp_data                                             │
│  │     Expected UE behavior, communication patterns      │
│  │                                                       │
│  └── operator_specific_data                              │
│        Custom key-value extensions                       │
└──────────────────────────────────────────────────────────┘
```

### 14.2 Key Tables

| Table | Primary Key | Approximate Row Size | Access Pattern |
|-------|------------|---------------------|----------------|
| `auth_subscription` | `(supi)` | 500 B | Read: UEAU; Write: provisioning |
| `auth_event` | `(supi, time_stamp)` | 200 B | Write: UEAU; Read: audit |
| `am_data` | `(supi, serving_plmn_id)` | 1 KB | Read: SDM (AMF); Write: provisioning |
| `sm_data` | `(supi, serving_plmn_id, snssai, dnn)` | 500 B | Read: SDM (SMF); Write: provisioning |
| `smf_selection_data` | `(supi, serving_plmn_id)` | 300 B | Read: SDM (AMF); Write: provisioning |
| `sms_data` | `(supi, serving_plmn_id)` | 200 B | Read: SDM (SMSF); Write: provisioning |
| `amf_registration` | `(supi, access_type)` | 500 B | Read/Write: UECM (AMF) |
| `smf_registration` | `(supi, pdu_session_id)` | 400 B | Read/Write: UECM (SMF) |
| `smsf_registration` | `(supi, access_type)` | 300 B | Read/Write: UECM (SMSF) |
| `ee_subscription` | `(ue_identity, subscription_id)` | 500 B | Read/Write: EE (NEF, NWDAF) |
| `pp_data` | `(supi)` | 400 B | Read/Write: PP (NEF) |

### 14.3 Indexing Strategy

| Index | Table | Columns | Purpose |
|-------|-------|---------|---------|
| Primary (hash) | All tables | `supi` | Point lookups by subscriber |
| Secondary | `amf_registration` | `amf_instance_id` | Find all subs registered to an AMF |
| Secondary | `sm_data` | `(serving_plmn_id, snssai)` | Slice-based queries |
| Secondary | `auth_subscription` | `gpsi` | MSISDN-based lookups |
| Covering | `am_data` | `(supi) INCLUDE (nssai, gpsi)` | SDM hot-path optimization |

---

## 15. Deployment Architecture

### 15.1 Kubernetes Namespace Layout

```
Namespace: udm-system
├── nudm-sdm          (Deployment, Service, HPA, PDB)
├── nudm-uecm         (Deployment, Service, HPA, PDB)
├── nudm-ueau         (Deployment, Service, HPA, PDB)
├── nudm-ee           (Deployment, Service, HPA, PDB)
├── nudm-pp           (Deployment, Service, HPA, PDB)
├── nudm-mt           (Deployment, Service, HPA, PDB)
├── nudm-ueid         (Deployment, Service, HPA, PDB)
├── nudm-ssau         (Deployment, Service, HPA, PDB)
├── nudm-niddau       (Deployment, Service, HPA, PDB)
├── nudm-rsds         (Deployment, Service, HPA, PDB)
├── redis-cluster     (StatefulSet)
└── config            (ConfigMaps, Secrets)

Namespace: yugabyte
├── yb-master         (StatefulSet, 3 replicas)
└── yb-tserver        (StatefulSet, 3+ replicas)

Namespace: observability
├── prometheus        (StatefulSet)
├── otel-collector    (DaemonSet)
└── grafana           (Deployment)
```

### 15.2 Resource Requests (Per Pod)

| Service | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---------|------------|-----------|---------------|-------------|
| nudm-sdm | 2 vCPU | 4 vCPU | 4 GB | 8 GB |
| nudm-uecm | 2 vCPU | 4 vCPU | 4 GB | 8 GB |
| nudm-ueau | 2 vCPU | 4 vCPU | 4 GB | 8 GB |
| nudm-ee | 1 vCPU | 2 vCPU | 2 GB | 4 GB |
| nudm-pp | 1 vCPU | 2 vCPU | 2 GB | 4 GB |
| nudm-mt | 1 vCPU | 2 vCPU | 2 GB | 4 GB |
| nudm-ueid | 1 vCPU | 2 vCPU | 2 GB | 4 GB |
| nudm-ssau | 500m | 1 vCPU | 1 GB | 2 GB |
| nudm-niddau | 500m | 1 vCPU | 1 GB | 2 GB |
| nudm-rsds | 500m | 1 vCPU | 1 GB | 2 GB |

### 15.3 Rolling Update Strategy

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0        # Never reduce capacity during upgrade
    maxSurge: 25%            # Add 25% extra pods before draining old
```

Combined with Pod Disruption Budgets:

```yaml
spec:
  minAvailable: 75%          # At least 75% of pods always running
```

### 15.4 Helm Chart Structure

```
charts/udm/
├── Chart.yaml
├── values.yaml                 # Default configuration
├── values-production.yaml      # Production overrides
├── templates/
│   ├── _helpers.tpl
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── serviceaccount.yaml
│   ├── networkpolicy.yaml
│   └── services/
│       ├── sdm-deployment.yaml
│       ├── sdm-service.yaml
│       ├── sdm-hpa.yaml
│       ├── sdm-pdb.yaml
│       ├── uecm-deployment.yaml
│       ├── ...
│       └── rsds-pdb.yaml
└── tests/
    └── test-connection.yaml
```

---

## 16. References

### 16.1 3GPP Specifications

| Specification | Document | Description |
|--------------|----------|-------------|
| TS 23.003 | `docs/3gpp/23003-i70.docx` | Numbering, Addressing and Identification |
| TS 23.501 | `docs/3gpp/23501-ic0.docx` | System Architecture for the 5G System |
| TS 23.502 | `docs/3gpp/23502-id0.docx` | Procedures for the 5G System |
| TS 23.503 | `docs/3gpp/23503-ib0.docx` | Policy and Charging Control Framework |
| TS 29.500 | `docs/3gpp/29500-ia0.docx` | Technical Realization of SBA |
| TS 29.503 | `docs/3gpp/29503-ic0.docx` | Nudm Services |
| TS 29.505 | `docs/3gpp/29505-i80.docx` | Usage of the Unified Data Repository |
| TS 33.501 | `docs/3gpp/33501-ia0.docx` | Security Architecture and Procedures |

### 16.2 OpenAPI Specifications

| File | Service |
|------|---------|
| `docs/3gpp/TS29503_Nudm_SDM.yaml` | Subscriber Data Management v2.3.6 |
| `docs/3gpp/TS29503_Nudm_UECM.yaml` | UE Context Management v1.3.3 |
| `docs/3gpp/TS29503_Nudm_PP.yaml` | Parameter Provision v1.3.3 |
| `docs/3gpp/TS29503_Nudm_EE.yaml` | Event Exposure v1.3.1 |
| `docs/3gpp/TS29503_Nudm_UEAU.yaml` | UE Authentication v1.3.2 |
| `docs/3gpp/TS29503_Nudm_SSAU.yaml` | Service Specific Authorization v1.1.1 |
| `docs/3gpp/TS29503_Nudm_NIDDAU.yaml` | NIDD Authorization v1.2.0 |
| `docs/3gpp/TS29503_Nudm_MT.yaml` | Mobile Terminated v1.2.0 |
| `docs/3gpp/TS29503_Nudm_RSDS.yaml` | Report SM Delivery Status v1.2.0 |
| `docs/3gpp/TS29503_Nudm_UEID.yaml` | UE Identifier v1.0.0 |
| `docs/3gpp/TS29505_Subscription_Data.yaml` | Subscription Data Schema |

### 16.3 Technology References

| Technology | Documentation |
|-----------|--------------|
| Go | https://go.dev/doc/ |
| YugabyteDB | https://docs.yugabyte.com/ |
| Kubernetes | https://kubernetes.io/docs/ |
| Istio | https://istio.io/latest/docs/ |
| OpenTelemetry | https://opentelemetry.io/docs/ |
| Prometheus | https://prometheus.io/docs/ |
| Redis | https://redis.io/docs/ |

---

*End of Document*
