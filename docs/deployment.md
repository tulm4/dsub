# UDM Deployment Architecture

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

## Table of Contents

1. [Deployment Overview](#1-deployment-overview)
2. [Multi-Region Topology](#2-multi-region-topology)
3. [Kubernetes Architecture](#3-kubernetes-architecture)
4. [Container Strategy](#4-container-strategy)
5. [YugabyteDB Deployment](#5-yugabytedb-deployment)
6. [Rolling Upgrades / Zero-Downtime Deployment](#6-rolling-upgrades--zero-downtime-deployment)
7. [Failover Strategy](#7-failover-strategy)
8. [Configuration Management](#8-configuration-management)
9. [CI/CD Pipeline](#9-cicd-pipeline)
10. [Capacity Planning & Auto-Scaling](#10-capacity-planning--auto-scaling)

---

## 1. Deployment Overview

### 1.1 Purpose

This document defines the production deployment architecture for the 5G Unified Data Management (UDM) system. It covers multi-region topology, Kubernetes resource design, zero-downtime upgrade procedures, failover strategies, and operational runbooks required to run a carrier-grade UDM at scale (10M–100M subscribers).

### 1.2 Cloud-Native Deployment Model

The UDM system is deployed as a set of 10 independent Nudm microservices on Kubernetes, following a cloud-native, infrastructure-agnostic model:

| Principle | Implementation |
|-----------|----------------|
| **Containerised workloads** | Each microservice ships as a single stateless container (distroless Go binary) |
| **Declarative configuration** | All resources defined as Kubernetes manifests managed via GitOps |
| **Horizontal scalability** | HPA scales each service independently based on CPU, memory, and custom TPS metrics |
| **Self-healing** | Liveness/readiness probes, PodDisruptionBudgets, and automatic pod rescheduling |
| **Service mesh** | Istio provides mTLS, traffic management, observability, and circuit breaking |
| **Stateless services** | All persistent state lives in YugabyteDB; pods are freely replaceable |

### 1.3 Microservice Inventory

| # | Service | API Root | Traffic Tier | Min Replicas |
|---|---------|----------|--------------|--------------|
| 1 | udm-ueau | `/nudm-ueau/v1` | High | 6 |
| 2 | udm-sdm | `/nudm-sdm/v2` | High | 6 |
| 3 | udm-uecm | `/nudm-uecm/v1` | High | 6 |
| 4 | udm-ee | `/nudm-ee/v1` | Medium | 3 |
| 5 | udm-pp | `/nudm-pp/v1` | Medium | 3 |
| 6 | udm-mt | `/nudm-mt/v1` | Medium | 3 |
| 7 | udm-ssau | `/nudm-ssau/v1` | Low | 2 |
| 8 | udm-niddau | `/nudm-niddau/v1` | Low | 2 |
| 9 | udm-rsds | `/nudm-rsds/v1` | Low | 2 |
| 10 | udm-ueid | `/nudm-ueid/v1` | Low | 2 |

---

## 2. Multi-Region Topology

### 2.1 Three-Region Active-Active Deployment

The UDM system operates in an Active-Active configuration across three geographic regions. Every region serves live traffic simultaneously — there is no standby region.

| Region | Cloud / Zone | Role |
|--------|-------------|------|
| **US-East** | `aws/us-east-1` (3 AZs) | Active — serves eastern Americas |
| **US-West** | `aws/us-west-2` (3 AZs) | Active — serves western Americas and APAC overflow |
| **EU-West** | `aws/eu-west-1` (3 AZs) | Active — serves EMEA, GDPR-resident data |

### 2.2 Regional UDM Cluster Architecture

Each region runs a complete UDM stack:

- **Full set of 10 Nudm microservices** — independently scaled per regional load
- **YugabyteDB tablet servers** — local replicas of the distributed database
- **Istio ingress gateways** — terminate TLS, enforce HTTP/2
- **Monitoring stack** — Prometheus, Grafana, Alertmanager (region-local)
- **Redis cache nodes** — region-local caching for authentication vectors

### 2.3 Global Load Balancing

Traffic reaches the nearest healthy region via a three-tier routing model:

1. **GeoDNS / Anycast** — AWS Route 53 latency-based routing directs NF consumers to the nearest regional UDM endpoint. Health checks on each region's ingress gateway automatically remove unhealthy regions from DNS.
2. **Istio Ingress Gateway** — Per-region gateway terminates TLS 1.3 and routes HTTP/2 requests to the appropriate Nudm service based on URI path prefix.
3. **NRF-based discovery** — Consuming NFs (AMF, AUSF, SMF) discover UDM instances through the NRF, which advertises per-region service profiles with locality and capacity information.

### 2.4 YugabyteDB Geo-Distribution

YugabyteDB runs as a single logical cluster spanning all three regions with a replication factor of 3 (RF=3):

- **One Raft replica per region** — every write is synchronously replicated to all three regions
- **Preferred leaders** — configured per SUPI range so that subscribers are served by their home region with local-read latency
- **Tablet placement** — `cloud.region.zone` placement policy ensures each tablet has exactly one replica in each region
- **Read replicas** — optional read replicas in additional zones for read-heavy SDM workloads

### 2.5 Data Placement Policies

```sql
-- Global tablespace: 3-region replication (RF=3, one replica per region)
CREATE TABLESPACE ts_global WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1}
    ]}'
);

-- Region-local tablespace: 3-zone replication within US-East
CREATE TABLESPACE ts_us_east_local WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1b", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1c", "min_num_replicas": 1}
    ]}'
);
```

**Policy assignment by data type:**

| Data Category | Tablespace | Rationale |
|---------------|-----------|-----------|
| Subscriber profiles (`authentication_data`, `access_mobility_subscription`) | `ts_global` | Must be accessible from any region during roaming |
| Registration state (`amf_registrations`, `smf_registrations`) | `ts_global` | AMF/SMF handovers may cross regions |
| Event subscriptions (`ee_subscriptions`, `sdm_subscriptions`) | `ts_global` | Callbacks may originate from any region |
| Audit / trace logs (`trace_data`) | Region-local (`ts_<region>_local`) | GDPR compliance — data stays in originating region |

### 2.6 Multi-Region Topology Diagram

```
                         ┌──────────────────────┐
                         │   GeoDNS / Route 53   │
                         │  (Latency-based GSLB) │
                         └──────┬───────┬────────┘
                     ┌──────────┘       └──────────┐
                     │                              │
          ┌──────────▼──────────┐        ┌──────────▼──────────┐
          │      US-East        │        │      EU-West        │
          │   aws/us-east-1     │        │   aws/eu-west-1     │
          │                     │        │                     │
          │ ┌─────────────────┐ │        │ ┌─────────────────┐ │
          │ │ Istio Ingress   │ │        │ │ Istio Ingress   │ │
          │ │ Gateway (HTTP/2)│ │        │ │ Gateway (HTTP/2)│ │
          │ └───────┬─────────┘ │        │ └───────┬─────────┘ │
          │         │           │        │         │           │
          │ ┌───────▼─────────┐ │        │ ┌───────▼─────────┐ │
          │ │  UDM Services   │ │        │ │  UDM Services   │ │
          │ │ (10 µservices)  │ │        │ │ (10 µservices)  │ │
          │ │  udm-ueau ×6   │ │        │ │  udm-ueau ×6   │ │
          │ │  udm-sdm  ×6   │ │        │ │  udm-sdm  ×6   │ │
          │ │  udm-uecm ×6   │ │        │ │  udm-uecm ×6   │ │
          │ │  ...            │ │        │ │  ...            │ │
          │ └───────┬─────────┘ │        │ └───────┬─────────┘ │
          │         │           │        │         │           │
          │ ┌───────▼─────────┐ │        │ ┌───────▼─────────┐ │
          │ │  YugabyteDB     │◄├────────┤►│  YugabyteDB     │ │
          │ │  T-Servers ×3   │ │  Raft  │ │  T-Servers ×3   │ │
          │ │  Masters  ×3    │ │  Sync  │ │  Masters  ×3    │ │
          │ └─────────────────┘ │        │ └─────────────────┘ │
          │                     │        │                     │
          │ ┌─────────────────┐ │        │ ┌─────────────────┐ │
          │ │ Redis Cache ×3  │ │        │ │ Redis Cache ×3  │ │
          │ └─────────────────┘ │        │ └─────────────────┘ │
          └─────────────────────┘        └─────────────────────┘
                     │                              │
                     │        ┌──────────┐          │
                     └────────►  US-West ◄──────────┘
                              │aws/us-west-2        │
                              │                     │
                              │ ┌─────────────────┐ │
                              │ │ Istio Ingress   │ │
                              │ │ Gateway (HTTP/2)│ │
                              │ └───────┬─────────┘ │
                              │ ┌───────▼─────────┐ │
                              │ │  UDM Services   │ │
                              │ │ (10 µservices)  │ │
                              │ └───────┬─────────┘ │
                              │ ┌───────▼─────────┐ │
                              │ │  YugabyteDB     │ │
                              │ │  T-Servers ×3   │ │
                              │ │  Masters  ×3    │ │
                              │ └─────────────────┘ │
                              │ ┌─────────────────┐ │
                              │ │ Redis Cache ×3  │ │
                              │ └─────────────────┘ │
                              └─────────────────────┘

  All 3 regions: Active-Active | YugabyteDB RF=3 | Raft consensus
```

---

## 3. Kubernetes Architecture

### 3.1 Namespace Design

```
udm-system        # Istio gateways, cert-manager, RBAC policies
udm-services      # All 10 Nudm microservice Deployments
udm-data          # YugabyteDB StatefulSets, PgBouncer, Redis
udm-monitoring    # Prometheus, Grafana, Alertmanager, OTel Collector
```

| Namespace | Purpose | Resource Quota (per region) |
|-----------|---------|----------------------------|
| `udm-system` | Platform infrastructure (gateways, mesh control) | 8 vCPU / 16 GB |
| `udm-services` | All Nudm microservice pods | 200 vCPU / 400 GB |
| `udm-data` | YugabyteDB, PgBouncer, Redis | 64 vCPU / 256 GB |
| `udm-monitoring` | Observability stack | 16 vCPU / 64 GB |

### 3.2 Per-Service Deployment Specifications

| Service | Replicas (min/max) | CPU Req/Lim | Mem Req/Lim | HPA Target |
|---------|---------------------|-------------|-------------|------------|
| udm-ueau | 6 / 20 | 2 / 4 vCPU | 4 / 8 GB | 60% CPU, 50k TPS |
| udm-sdm | 6 / 24 | 2 / 4 vCPU | 4 / 8 GB | 60% CPU, 200k TPS |
| udm-uecm | 6 / 20 | 2 / 4 vCPU | 4 / 8 GB | 60% CPU, 50k TPS |
| udm-ee | 3 / 10 | 1 / 2 vCPU | 2 / 4 GB | 60% CPU, 30k TPS |
| udm-pp | 3 / 10 | 1 / 2 vCPU | 2 / 4 GB | 60% CPU, 10k TPS |
| udm-mt | 3 / 10 | 1 / 2 vCPU | 2 / 4 GB | 60% CPU, 5k TPS |
| udm-ueid | 2 / 8 | 1 / 2 vCPU | 2 / 4 GB | 60% CPU |
| udm-ssau | 2 / 6 | 500m / 1 vCPU | 1 / 2 GB | 60% CPU |
| udm-niddau | 2 / 6 | 500m / 1 vCPU | 1 / 2 GB | 60% CPU |
| udm-rsds | 2 / 6 | 500m / 1 vCPU | 1 / 2 GB | 60% CPU |

### 3.3 Service Mesh (Istio) Configuration

All UDM services run within an Istio mesh with strict mTLS:

```yaml
# PeerAuthentication — enforce mTLS across the UDM namespace
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: udm-strict-mtls
  namespace: udm-services
spec:
  mtls:
    mode: STRICT
---
# DestinationRule — connection pool and outlier detection
apiVersion: networking.istio.io/v1
kind: DestinationRule
metadata:
  name: udm-ueau-dr
  namespace: udm-services
spec:
  host: udm-ueau.udm-services.svc.cluster.local
  trafficPolicy:
    connectionPool:
      http:
        h2UpgradePolicy: UPGRADE
        maxRequestsPerConnection: 1000
        http2MaxRequests: 10000
      tcp:
        maxConnections: 2048
        connectTimeout: 250ms
    outlierDetection:
      consecutive5xxErrors: 3
      interval: 10s
      baseEjectionTime: 30s
      maxEjectionPercent: 30
    loadBalancer:
      simple: LEAST_REQUEST
```

### 3.4 Ingress / Gateway Configuration

```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  name: udm-gateway
  namespace: udm-system
spec:
  selector:
    istio: ingressgateway
  servers:
    - port:
        number: 443
        name: https
        protocol: HTTPS
      tls:
        mode: SIMPLE
        credentialName: udm-tls-cert
        minProtocolVersion: TLSV1_3
      hosts:
        - "udm.us-east.5gcore.example.com"
        - "udm.us-west.5gcore.example.com"
        - "udm.eu-west.5gcore.example.com"
---
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: udm-routing
  namespace: udm-system
spec:
  hosts:
    - "udm.us-east.5gcore.example.com"
  gateways:
    - udm-gateway
  http:
    - match:
        - uri:
            prefix: /nudm-ueau/v1
      route:
        - destination:
            host: udm-ueau.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-sdm/v2
      route:
        - destination:
            host: udm-sdm.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-uecm/v1
      route:
        - destination:
            host: udm-uecm.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-ee/v1
      route:
        - destination:
            host: udm-ee.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-pp/v1
      route:
        - destination:
            host: udm-pp.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-mt/v1
      route:
        - destination:
            host: udm-mt.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-ssau/v1
      route:
        - destination:
            host: udm-ssau.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-niddau/v1
      route:
        - destination:
            host: udm-niddau.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-rsds/v1
      route:
        - destination:
            host: udm-rsds.udm-services.svc.cluster.local
            port:
              number: 8080
    - match:
        - uri:
            prefix: /nudm-ueid/v1
      route:
        - destination:
            host: udm-ueid.udm-services.svc.cluster.local
            port:
              number: 8080
```

### 3.5 Pod Topology Spread Constraints

All UDM service pods are spread across availability zones to survive zone-level failures:

```yaml
topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: udm-ueau
  - maxSkew: 1
    topologyKey: kubernetes.io/hostname
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: udm-ueau
```

### 3.6 PodDisruptionBudgets

High-traffic services maintain a minimum of 4 available pods during voluntary disruptions (node drains, upgrades). Medium and low tiers use proportional minimums:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: udm-ueau-pdb
  namespace: udm-services
spec:
  minAvailable: 4
  selector:
    matchLabels:
      app.kubernetes.io/name: udm-ueau
```

| Traffic Tier | PDB `minAvailable` |
|--------------|--------------------|
| High (ueau, sdm, uecm) | 4 |
| Medium (ee, pp, mt) | 2 |
| Low (ssau, niddau, rsds, ueid) | 1 |

### 3.7 Resource Quotas

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: udm-services-quota
  namespace: udm-services
spec:
  hard:
    requests.cpu: "200"
    requests.memory: 400Gi
    limits.cpu: "400"
    limits.memory: 800Gi
    pods: "300"
```

### 3.8 Complete Example — udm-ueau Kubernetes Manifests

```yaml
# Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: udm-ueau
  namespace: udm-services
  labels:
    app.kubernetes.io/name: udm-ueau
    app.kubernetes.io/part-of: udm
    app.kubernetes.io/version: "1.0.0"
spec:
  replicas: 6
  revisionHistoryLimit: 5
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 2
      maxUnavailable: 0
  selector:
    matchLabels:
      app.kubernetes.io/name: udm-ueau
  template:
    metadata:
      labels:
        app.kubernetes.io/name: udm-ueau
        app.kubernetes.io/part-of: udm
        sidecar.istio.io/inject: "true"
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: udm-ueau
      terminationGracePeriodSeconds: 45
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: udm-ueau
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: udm-ueau
      containers:
        - name: udm-ueau
          image: registry.5gcore.example.com/udm/udm-ueau:1.0.0
          ports:
            - name: http2
              containerPort: 8080
              protocol: TCP
            - name: metrics
              containerPort: 9090
              protocol: TCP
          env:
            - name: UDM_LOG_LEVEL
              valueFrom:
                configMapKeyRef:
                  name: udm-ueau-config
                  key: log_level
            - name: UDM_DB_DSN
              valueFrom:
                secretKeyRef:
                  name: udm-db-credentials
                  key: dsn
            - name: UDM_HTTP_PORT
              value: "8080"
            - name: UDM_METRICS_PORT
              value: "9090"
          resources:
            requests:
              cpu: "2"
              memory: 4Gi
            limits:
              cpu: "4"
              memory: 8Gi
          startupProbe:
            httpGet:
              path: /healthz/startup
              port: http2
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 12
          readinessProbe:
            httpGet:
              path: /healthz/ready
              port: http2
            initialDelaySeconds: 5
            periodSeconds: 5
            failureThreshold: 3
            timeoutSeconds: 3
          livenessProbe:
            httpGet:
              path: /healthz/live
              port: http2
            initialDelaySeconds: 15
            periodSeconds: 10
            failureThreshold: 3
            timeoutSeconds: 5
          lifecycle:
            preStop:
              exec:
                command:
                  - /bin/sh
                  - -c
                  - sleep 10 && kill -SIGTERM 1
---
# Service
apiVersion: v1
kind: Service
metadata:
  name: udm-ueau
  namespace: udm-services
  labels:
    app.kubernetes.io/name: udm-ueau
spec:
  type: ClusterIP
  ports:
    - name: http2
      port: 8080
      targetPort: http2
      protocol: TCP
      appProtocol: h2c
    - name: metrics
      port: 9090
      targetPort: metrics
      protocol: TCP
  selector:
    app.kubernetes.io/name: udm-ueau
---
# HorizontalPodAutoscaler
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: udm-ueau-hpa
  namespace: udm-services
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: udm-ueau
  minReplicas: 6
  maxReplicas: 20
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30
      policies:
        - type: Pods
          value: 4
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Percent
          value: 10
          periodSeconds: 120
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 60
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 70
    - type: Pods
      pods:
        metric:
          name: udm_http_requests_per_second
        target:
          type: AverageValue
          averageValue: "8000"
---
# PodDisruptionBudget
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: udm-ueau-pdb
  namespace: udm-services
spec:
  minAvailable: 4
  selector:
    matchLabels:
      app.kubernetes.io/name: udm-ueau
```

---

## 4. Container Strategy

### 4.1 Multi-Stage Docker Builds

All UDM microservices use identical multi-stage build patterns. The build stage compiles a statically linked Go binary; the runtime stage copies it into a minimal distroless image.

### 4.2 Distroless Base Images

Production containers use `gcr.io/distroless/static-debian12:nonroot`:

- No shell, no package manager — minimal attack surface
- Non-root execution (UID 65532)
- Approx. 2 MB base image size
- Final container image: ~15–20 MB per microservice

### 4.3 Image Scanning

Every container image is scanned before deployment:

| Stage | Tool | Policy |
|-------|------|--------|
| CI build | **Trivy** (filesystem scan) | Block on CRITICAL or HIGH with fix available |
| Registry push | **Trivy** (image scan) | SBOM generation; advisory alerts on MEDIUM |
| Runtime | **Trivy Operator** (in-cluster) | Continuous scan; alert on new CVEs |

### 4.4 Image Registry Strategy

```
registry.5gcore.example.com/udm/<service>:<semver>
```

- **Tagging**: Semantic versioning (`1.2.3`) + Git SHA suffix for traceability (`1.2.3-abc1234`)
- **Immutable tags**: Once pushed, a tag is never overwritten
- **Promotion**: Images are promoted across environments by re-tagging (never rebuilt)
- **Retention**: Keep last 20 versions per service; auto-prune untagged manifests after 30 days
- **Replication**: Registry geo-replicated to all three deployment regions

### 4.5 Example Dockerfile

```dockerfile
# ── Build stage ──────────────────────────────────────────────
FROM golang:1.23-bookworm AS builder

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o /bin/udm-ueau ./cmd/udm-ueau

# ── Runtime stage ────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/udm-ueau /udm-ueau

USER nonroot:nonroot
EXPOSE 8080 9090

ENTRYPOINT ["/udm-ueau"]
```

---

## 5. YugabyteDB Deployment

### 5.1 YugabyteDB Kubernetes Operator

YugabyteDB is deployed and managed via the **YugabyteDB Kubernetes Operator**, which automates cluster provisioning, scaling, upgrades, and backup orchestration.

Key operator responsibilities:

- Bootstraps master and tablet server StatefulSets
- Manages rolling upgrades of the database layer
- Handles certificate rotation for node-to-node and client-to-node TLS
- Orchestrates multi-region placement configuration

### 5.2 Multi-Region Cluster Setup

The YugabyteDB cluster is configured as a **single globally-distributed cluster** with RF=3 spanning three cloud regions. Raft consensus ensures strong consistency across all replicas.

| Parameter | Value |
|-----------|-------|
| Replication factor | 3 |
| Regions | `us-east-1`, `us-west-2`, `eu-west-1` |
| Masters per region | 3 (9 total, Raft-elected leader) |
| Tablet servers per region | 3–6 (scaled with subscriber count) |
| Tablet target size | ~4 GB (auto-split when exceeded) |
| Consistency | Strong (Raft) — ACID-compliant |
| Encryption at rest | AES-256-GCM (YugabyteDB TDE) |
| Encryption in transit | TLS 1.3 (node-to-node and client-to-node) |

### 5.3 Tablet Server Configuration

```yaml
tserverGFlags:
  # Placement
  placement_cloud: aws
  placement_region: us-east-1
  placement_zone: us-east-1a
  # Performance
  rocksdb_compact_flush_rate_limit_bytes_per_sec: "268435456"  # 256 MB/s
  tablet_split_low_phase_size_threshold_bytes: "536870912"     # 512 MB
  tablet_split_high_phase_size_threshold_bytes: "4294967296"   # 4 GB
  yb_num_shards_per_tserver: "8"
  ysql_num_shards_per_tserver: "4"
  # Connection handling
  ysql_max_connections: "300"
  # Memory
  default_memory_limit_to_ram_ratio: "0.6"
```

### 5.4 Master Server Configuration

```yaml
masterGFlags:
  placement_cloud: aws
  placement_region: us-east-1
  placement_zone: us-east-1a
  replication_factor: "3"
  load_balancer_max_concurrent_moves: "5"
  load_balancer_max_concurrent_adds: "3"
  enable_load_balancing: "true"
```

### 5.5 Storage Class and PVC Configuration

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: yb-fast-ssd
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  iops: "10000"
  throughput: "500"
  encrypted: "true"
  kmsKeyId: "arn:aws:kms:us-east-1:123456789:key/yb-encryption-key"
volumeBindingMode: WaitForFirstConsumer
allowVolumeExpansion: true
reclaimPolicy: Retain
---
# PVC template used in YugabyteDB StatefulSet
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: datadir
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: yb-fast-ssd
  resources:
    requests:
      storage: 500Gi
```

### 5.6 Backup Strategy

| Schedule | Type | Retention | Target |
|----------|------|-----------|--------|
| Every 6 hours | Incremental (WAL-based) | 7 days | S3 (cross-region replicated) |
| Daily 02:00 UTC | Full snapshot | 30 days | S3 + Glacier |
| On-demand | Full snapshot | Until manually deleted | S3 |
| Pre-upgrade | Full snapshot | 90 days | S3 + Glacier |

**Backup procedure:**

1. YugabyteDB operator triggers `yb-admin create_snapshot` for all YSQL tables
2. Snapshot uploaded to S3 via `yb_backup.py` with server-side encryption (SSE-KMS)
3. Backup manifest registered in a metadata table with checksum for integrity verification
4. Restore tested monthly in a staging environment (automated via CI job)

### 5.7 Example YugabyteDB Cluster Manifest

```yaml
apiVersion: yugabyte.com/v1alpha1
kind: YBCluster
metadata:
  name: udm-yugabytedb
  namespace: udm-data
spec:
  image:
    repository: yugabytedb/yugabyte
    tag: "2024.2.1.0-b185"
  tls:
    enabled: true
    rootCA:
      cert: udm-yb-root-ca
  master:
    replicas: 3
    resources:
      requests:
        cpu: "4"
        memory: 8Gi
      limits:
        cpu: "8"
        memory: 16Gi
    storage:
      size: 100Gi
      storageClass: yb-fast-ssd
    gflags:
      replication_factor: "3"
      load_balancer_max_concurrent_moves: "5"
      enable_load_balancing: "true"
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: yb-master
            topologyKey: topology.kubernetes.io/zone
  tserver:
    replicas: 6
    resources:
      requests:
        cpu: "16"
        memory: 64Gi
      limits:
        cpu: "16"
        memory: 64Gi
    storage:
      size: 500Gi
      storageClass: yb-fast-ssd
    gflags:
      ysql_max_connections: "300"
      yb_num_shards_per_tserver: "8"
      ysql_num_shards_per_tserver: "4"
      default_memory_limit_to_ram_ratio: "0.6"
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: yb-tserver
            topologyKey: topology.kubernetes.io/zone
  replicationInfo:
    liveReplicas:
      numReplicas: 3
      placementBlocks:
        - cloudInfo:
            placementCloud: aws
            placementRegion: us-east-1
            placementZone: us-east-1a
          minNumReplicas: 1
        - cloudInfo:
            placementCloud: aws
            placementRegion: us-west-2
            placementZone: us-west-2a
          minNumReplicas: 1
        - cloudInfo:
            placementCloud: aws
            placementRegion: eu-west-1
            placementZone: eu-west-1a
          minNumReplicas: 1
```

---

## 6. Rolling Upgrades / Zero-Downtime Deployment

### 6.1 Rolling Update Strategy

All UDM microservice Deployments use a rolling update strategy that ensures zero downtime:

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 2          # Spin up 2 extra pods before terminating old ones
    maxUnavailable: 0    # Never reduce below desired replica count
```

**Update sequence:**

1. Kubernetes creates up to `maxSurge` new pods with the updated image
2. New pods pass startup probe (up to 60s) → enter ready state
3. Readiness probe confirms the pod can serve traffic (checked every 5s)
4. Old pods are drained: preStop hook fires → pod marked NotReady → in-flight requests complete
5. Old pod terminates after `terminationGracePeriodSeconds` (45s)
6. Process repeats until all pods are updated

### 6.2 Blue-Green Deployment for Major Versions

For major version upgrades (e.g., v1 → v2 API changes), a full blue-green deployment is used:

1. **Deploy "green" stack** — new Deployment with distinct label (`version: green`) in the same namespace
2. **Smoke test** — run integration tests against green pods via internal Service
3. **Switch traffic** — update the Istio VirtualService to route 100% of traffic to the green Deployment
4. **Monitor** — observe error rates and latency for 15 minutes
5. **Decommission blue** — scale down the old Deployment after validation window
6. **Rollback** — if anomalies detected, revert VirtualService to blue (< 30s switchover)

### 6.3 Canary Deployment for Risk Mitigation

For medium-risk changes, a canary deployment progressively shifts traffic:

```yaml
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: udm-ueau-canary
  namespace: udm-services
spec:
  hosts:
    - udm-ueau.udm-services.svc.cluster.local
  http:
    - route:
        - destination:
            host: udm-ueau.udm-services.svc.cluster.local
            subset: stable
          weight: 90
        - destination:
            host: udm-ueau.udm-services.svc.cluster.local
            subset: canary
          weight: 10
```

**Canary progression:** 5% → 10% → 25% → 50% → 100%, with automated rollback if:

- Error rate (5xx) exceeds 0.1%
- P99 latency exceeds 2× the stable baseline
- Any liveness probe failure in canary pods

### 6.4 Database Schema Migration Strategy

Schema migrations are handled with a **expand-and-contract** pattern to maintain backward compatibility during rolling upgrades:

| Phase | Actions | Risk |
|-------|---------|------|
| **1. Expand** | Add new columns/tables (nullable or with defaults); create new indexes. Both old and new code work. | Low |
| **2. Migrate** | Backfill data for existing rows via async job. | Low |
| **3. Deploy** | Roll out new application version that writes to both old and new schemas. | Medium |
| **4. Contract** | Drop old columns/indexes after all pods run the new version and a validation period passes. | Low |

**Migration tooling:**

- Migrations are versioned SQL files embedded in the Go binary (`embed` package)
- On startup, each pod runs `golang-migrate` to apply pending migrations (idempotent, lock-protected)
- Migration locks use `pg_advisory_lock` — only one pod applies migrations at a time
- Backward-incompatible DDL is forbidden in a single release

### 6.5 Readiness / Liveness Probe Design

| Probe | Endpoint | Checks | Period | Threshold |
|-------|----------|--------|--------|-----------|
| **Startup** | `/healthz/startup` | Binary started, config loaded, DB connection pool initialized | 5s | 12 failures (60s max) |
| **Readiness** | `/healthz/ready` | DB connection healthy, cache warmed, Istio sidecar ready | 5s | 3 failures |
| **Liveness** | `/healthz/live` | Goroutine not deadlocked, memory within bounds, event loop responsive | 10s | 3 failures |

**Key design decisions:**

- Readiness probe fails fast (15s) to remove unhealthy pods from Service endpoints
- Liveness probe is conservative (30s) to avoid unnecessary restarts during transient DB latency
- Startup probe gives pods 60s to initialize (cold cache, DB migration, TLS handshake)

### 6.6 Graceful Shutdown Handling

```
    ┌───────────────────────────────────────────────────────────┐
    │                  Pod Termination Sequence                 │
    │                                                           │
    │  1. SIGTERM received (or preStop hook fires)              │
    │  2. preStop: sleep 10s (allow Istio to drain connections) │
    │  3. Pod marked NotReady (removed from Service endpoints)  │
    │  4. Server stops accepting new connections                │
    │  5. In-flight requests complete (up to 30s grace period)  │
    │  6. DB connection pool drained                            │
    │  7. Prometheus metrics flushed                            │
    │  8. Process exits 0                                       │
    │  9. If still running at 45s: SIGKILL                      │
    └───────────────────────────────────────────────────────────┘
```

The `preStop` hook introduces a 10-second delay before SIGTERM, giving Istio's Envoy sidecar time to receive the endpoint update and stop routing new requests to the terminating pod:

```yaml
lifecycle:
  preStop:
    exec:
      command:
        - /bin/sh
        - -c
        - sleep 10 && kill -SIGTERM 1
```

### 6.7 Rollback Procedures

| Scenario | Action | RTO |
|----------|--------|-----|
| **Bad deployment (< 5 min)** | `kubectl rollout undo deployment/udm-<svc>` | < 2 min |
| **Bad deployment (> 5 min, canary)** | Set canary weight to 0% via VirtualService update | < 30s |
| **Bad DB migration (expand phase)** | Deploy previous app version; expanded schema is backward-compatible | < 5 min |
| **Bad DB migration (contract phase)** | Restore from pre-upgrade snapshot | 15–30 min |
| **Region-wide failure during upgrade** | GeoDNS removes region; remaining 2 regions serve all traffic | < 60s |

---

## 7. Failover Strategy

### 7.1 Node-Level Failover

- **Pod crash** — kubelet restarts the container per `restartPolicy: Always` (RTO: 5–10s)
- **Pod eviction** — Kubernetes scheduler places a replacement pod on another node; HPA maintains desired replicas (RTO: 15–30s)
- **Node failure** — Node controller marks node as `NotReady` after 40s; pods rescheduled to healthy nodes (RTO: 60–90s)
- PodDisruptionBudgets prevent cascading failures during voluntary disruptions

### 7.2 Zone-Level Failover

- **Pod topology spread constraints** ensure pods are evenly distributed across 3 AZs
- Zone failure loses ~1/3 of pods; HPA immediately scales up on remaining zones
- YugabyteDB tablet leaders on the failed zone are re-elected in surviving zones (Raft leader election: ~3s)
- Service mesh automatically excludes unreachable endpoints (Istio outlier detection: 10s)

**RTO: < 30s** for zone-level failure (automatic, no human intervention).

### 7.3 Region-Level Failover

1. **Detection** — Route 53 health checks detect region unresponsiveness (30s interval, 3 failures = 90s)
2. **DNS update** — Route 53 removes the failed region from DNS responses (TTL: 60s)
3. **Traffic rerouting** — NF consumers resolve to surviving regions within DNS TTL window
4. **Database continuity** — YugabyteDB Raft elects new leaders in surviving regions (~3s); writes continue with 2/3 replicas
5. **Capacity surge** — HPA in surviving regions scales up to absorb redirected traffic

**RTO: < 3 minutes** (DNS propagation dominates). **RPO: 0** (synchronous replication, no data loss).

### 7.4 YugabyteDB Leader Re-election

During any failure that impacts a Raft leader:

1. Follower nodes detect missing heartbeats (default: 1s timeout)
2. A follower initiates leader election (randomized election timeout: 1.5–3s)
3. New leader elected with majority vote (2 out of 3 replicas)
4. Client connections to the old leader fail and are transparently retried by the `pgx` driver with follower-reads or leader-redirect
5. Total disruption: **< 5 seconds** for a single tablet's leader failover

### 7.5 RTO / RPO Targets

| Failure Scope | RTO Target | RPO Target | Mechanism |
|---------------|------------|------------|-----------|
| Single pod | < 10s | 0 | Container restart |
| Single node | < 90s | 0 | Pod rescheduling |
| Single AZ | < 30s | 0 | Topology spread + Raft |
| Single region | < 3 min | 0 | GeoDNS failover + Raft |
| Database leader | < 5s | 0 | Raft leader election |
| Full cluster (disaster) | < 30 min | < 6 hours | Restore from S3 backup |

---

## 8. Configuration Management

### 8.1 ConfigMaps for Service Configuration

Each microservice reads non-sensitive configuration from a dedicated ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: udm-ueau-config
  namespace: udm-services
data:
  log_level: "info"
  http_port: "8080"
  metrics_port: "9090"
  db_max_open_conns: "50"
  db_max_idle_conns: "25"
  db_conn_max_lifetime: "300s"
  cache_ttl: "60s"
  shutdown_timeout: "30s"
  nrf_discovery_url: "http://nrf.5gcore.svc.cluster.local:8080/nnrf-disc/v1"
```

Configuration changes trigger a rolling restart via a hash annotation in the Deployment pod template:

```yaml
annotations:
  checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
```

### 8.2 Secrets Management

Sensitive configuration (database credentials, TLS certificates, API keys) is managed through **HashiCorp Vault** with the Vault Agent Injector:

```yaml
annotations:
  vault.hashicorp.com/agent-inject: "true"
  vault.hashicorp.com/role: "udm-ueau"
  vault.hashicorp.com/agent-inject-secret-db-creds: "secret/data/udm/db-credentials"
  vault.hashicorp.com/agent-inject-template-db-creds: |
    {{- with secret "secret/data/udm/db-credentials" -}}
    postgresql://{{ .Data.data.username }}:{{ .Data.data.password }}@yb-tservers.udm-data:5433/udm
    {{- end -}}
```

**Alternative (air-gapped environments):** Bitnami Sealed Secrets encrypt secrets in Git; the Sealed Secrets controller decrypts them in-cluster.

### 8.3 Environment-Specific Overlays (Kustomize)

```
deploy/
├── base/
│   ├── kustomization.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── hpa.yaml
│   └── pdb.yaml
├── overlays/
│   ├── dev/
│   │   ├── kustomization.yaml    # replicas: 1, limits reduced
│   │   └── patch-resources.yaml
│   ├── staging/
│   │   ├── kustomization.yaml    # replicas: 2, production images
│   │   └── patch-resources.yaml
│   └── prod/
│       ├── us-east/
│       │   ├── kustomization.yaml
│       │   └── patch-replicas.yaml
│       ├── us-west/
│       │   ├── kustomization.yaml
│       │   └── patch-replicas.yaml
│       └── eu-west/
│           ├── kustomization.yaml
│           └── patch-replicas.yaml
```

Each overlay customizes:

- **Replica counts** — dev: 1, staging: 2, prod: 6+ (per traffic tier)
- **Resource limits** — dev uses 1/4 of production resources
- **Image tags** — pinned per environment; promoted via CI/CD
- **ConfigMap values** — log level, cache TTL, feature flags

### 8.4 Feature Flags

Feature flags enable phased rollouts and instant kill switches without redeployment:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: udm-feature-flags
  namespace: udm-services
data:
  enable_5g_prose: "false"
  enable_nssaa_flow: "true"
  enable_ue_reachability_notifications: "true"
  subscriber_data_cache_enabled: "true"
  max_sdm_subscriptions_per_ue: "10"
```

Flags are read at startup and optionally hot-reloaded via a file watcher on the mounted ConfigMap volume. Critical flags (e.g., `enable_5g_prose`) require a restart to take effect (enforced by pod annotation hash).

---

## 9. CI/CD Pipeline

### 9.1 Pipeline Overview

```
  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌──────────┐    ┌─────────┐
  │  Build   │───▶│  Test   │───▶│  Scan   │───▶│ Publish  │───▶│ Deploy  │
  │          │    │         │    │         │    │          │    │         │
  │ go build │    │ go test │    │ Trivy   │    │ Registry │    │ ArgoCD  │
  │ lint     │    │ integ.  │    │ CodeQL  │    │ push     │    │ sync    │
  │ vet      │    │ bench   │    │ SBOM    │    │ sign     │    │         │
  └─────────┘    └─────────┘    └─────────┘    └──────────┘    └─────────┘
       │              │              │               │              │
       └──────────────┴──────────────┴───────────────┴──────────────┘
                              Automated Gate
                         (Fail = Pipeline Stops)
```

### 9.2 Build Stage

- **Compile**: `CGO_ENABLED=0 go build` for each of the 10 microservices (parallel matrix build)
- **Lint**: `golangci-lint run` with project-level `.golangci.yml`
- **Vet**: `go vet ./...`
- **Generate**: `go generate ./...` for OpenAPI server stubs, mocks

### 9.3 Test Stage

- **Unit tests**: `go test -race -coverprofile=coverage.out ./...` (target: ≥ 80% coverage)
- **Integration tests**: Docker Compose with YugabyteDB and Redis; test each service's SBI endpoints
- **Contract tests**: Validate OpenAPI spec compliance for all 10 Nudm APIs
- **Benchmark tests**: `go test -bench=.` with regression detection (alert if P99 regresses > 10%)

### 9.4 Scan Stage

- **Trivy**: Container image scan (block CRITICAL/HIGH with available fix)
- **CodeQL**: Static analysis for Go security vulnerabilities
- **SBOM**: Generate Software Bill of Materials (SPDX format) attached to image manifest
- **License check**: Ensure all dependencies use approved OSS licenses

### 9.5 GitOps with ArgoCD

ArgoCD watches the Git repository for manifest changes and reconciles the live cluster state:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: udm-services-us-east
  namespace: argocd
spec:
  project: udm
  source:
    repoURL: https://git.5gcore.example.com/udm/deploy.git
    targetRevision: main
    path: overlays/prod/us-east
  destination:
    server: https://kubernetes.us-east.5gcore.example.com
    namespace: udm-services
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
    retry:
      limit: 3
      backoff:
        duration: 10s
        maxDuration: 3m
        factor: 2
```

Each region has a separate ArgoCD Application pointing to its Kustomize overlay.

### 9.6 Environment Promotion

```
  dev  ──────▶  staging  ──────▶  prod (us-east)  ──▶  prod (us-west)  ──▶  prod (eu-west)
  (auto)        (auto)           (manual gate)         (auto, 15m delay)    (auto, 15m delay)
```

| Stage | Trigger | Validation |
|-------|---------|------------|
| **dev** | Every commit to `main` | Unit tests + integration tests pass |
| **staging** | dev deployment healthy for 10 min | Full E2E test suite, load test (10% prod traffic) |
| **prod (us-east)** | Manual approval via PR merge to `release/` branch | Canary 5% → 10% → 25% → 100% with automated rollback |
| **prod (us-west)** | us-east healthy for 15 min | Automated canary progression |
| **prod (eu-west)** | us-west healthy for 15 min | Automated canary progression |

### 9.7 Automated Rollback Triggers

ArgoCD and the canary controller automatically roll back a deployment if any of the following conditions are met within the observation window:

| Metric | Threshold | Window |
|--------|-----------|--------|
| HTTP 5xx error rate | > 0.1% | 5 min |
| P99 request latency | > 2× baseline | 5 min |
| Pod restart count | > 2 restarts | 10 min |
| Readiness probe failures | > 3 consecutive | 1 min |
| YugabyteDB connection errors | > 5/min | 5 min |

---

## 10. Capacity Planning & Auto-Scaling

### 10.1 Cluster Sizing by Subscriber Count

| Component | 10M Subscribers | 50M Subscribers | 100M Subscribers |
|-----------|-----------------|-----------------|------------------|
| **UDM service pods** (all services) | 30 pods (4 vCPU, 8 GB) | 150 pods | 300 pods |
| **YugabyteDB nodes** (per region) | 3 nodes (16 vCPU, 64 GB, 500 GB SSD) | 9 nodes | 18 nodes |
| **Redis cache nodes** (per region) | 3 nodes (8 vCPU, 32 GB) | 6 nodes | 12 nodes |
| **DB storage per replica** | 99 GB | 495 GB | 990 GB |
| **Peak TPS** | 100K | 500K | 1M |

### 10.2 HPA Metrics

Each service uses a combination of resource and custom metrics for auto-scaling:

```yaml
metrics:
  # Primary: CPU utilization
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60

  # Secondary: Memory utilization
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 70

  # Custom: Transactions per second (from Prometheus via adapter)
  - type: Pods
    pods:
      metric:
        name: udm_http_requests_per_second
      target:
        type: AverageValue
        averageValue: "8000"
```

**Custom metrics pipeline:** Prometheus → Prometheus Adapter → Kubernetes `custom.metrics.k8s.io` API → HPA controller.

### 10.3 VPA Recommendations

Vertical Pod Autoscaler runs in **recommendation-only mode** (never auto-applies) to inform capacity planning:

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: udm-ueau-vpa
  namespace: udm-services
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: udm-ueau
  updatePolicy:
    updateMode: "Off"
  resourcePolicy:
    containerPolicies:
      - containerName: udm-ueau
        minAllowed:
          cpu: "1"
          memory: 2Gi
        maxAllowed:
          cpu: "8"
          memory: 16Gi
```

VPA recommendations are reviewed weekly and used to adjust HPA `minReplicas` and resource requests in the base manifests.

### 10.4 Cluster Autoscaler Integration

The Kubernetes Cluster Autoscaler scales the underlying node pool when pods are unschedulable:

| Parameter | Value |
|-----------|-------|
| Scale-up trigger | Pending pods (unschedulable due to resource constraints) |
| Scale-up cooldown | 60s |
| Scale-down trigger | Node utilization < 50% for 10 min |
| Scale-down cooldown | 300s |
| Max nodes per region | 100 |
| Node instance type | `m6i.4xlarge` (16 vCPU, 64 GB) for services; `r6i.4xlarge` (16 vCPU, 128 GB) for YugabyteDB |

### 10.5 Scale-Out Triggers per Service

| Service | Primary Scale Trigger | Target per Pod | Scale-Out Indicator |
|---------|----------------------|----------------|---------------------|
| udm-sdm | Read TPS | 8,000 RPS | SDM subscription query volume spike |
| udm-ueau | Auth TPS | 8,000 RPS | Mass authentication events (e.g., cell outage recovery) |
| udm-uecm | Registration TPS | 8,000 RPS | AMF/SMF registration storms |
| udm-ee | Subscription count | 5,000 RPS | Event exposure subscription surge |
| udm-pp | Write TPS | 3,000 RPS | Bulk provisioning operations |
| udm-mt | Query TPS | 3,000 RPS | Paging / reachability request bursts |
| udm-ueid | ID resolution TPS | 3,000 RPS | Lawful intercept / SUPI resolution |
| udm-ssau | Auth TPS | 1,500 RPS | Slice-specific authentication events |
| udm-niddau | Auth TPS | 1,500 RPS | NIDD device onboarding waves |
| udm-rsds | Config TPS | 1,500 RPS | SoR configuration pushes |

### 10.6 Scaling Behavior Tuning

HPA scaling behavior is tuned to react quickly to traffic spikes while avoiding flapping during normal fluctuations:

```yaml
behavior:
  scaleUp:
    stabilizationWindowSeconds: 30    # React within 30s to sustained load
    policies:
      - type: Pods
        value: 4                      # Add up to 4 pods at a time
        periodSeconds: 60
  scaleDown:
    stabilizationWindowSeconds: 300   # Wait 5 min before scaling down
    policies:
      - type: Percent
        value: 10                     # Remove at most 10% of pods per cycle
        periodSeconds: 120
```

This asymmetric configuration scales up aggressively (30s window, 4 pods/min) and scales down conservatively (5 min window, 10%/2 min) — critical for telecom workloads where traffic spikes are sudden but traffic dips may be temporary.

---

## Related Documents

- [architecture.md](architecture.md) — High-level UDM system architecture
- [service-decomposition.md](service-decomposition.md) — Microservice design and API mapping
- [data-model.md](data-model.md) — YugabyteDB schema and data placement
- [observability.md](observability.md) — Metrics, tracing, logging, and alerting
- [testing-strategy.md](testing-strategy.md) — Testing pyramid and CI validation
- [sbi-api-design.md](sbi-api-design.md) — SBI API design and endpoint catalog
- [sequence-diagrams.md](sequence-diagrams.md) — 3GPP procedure sequence flows
