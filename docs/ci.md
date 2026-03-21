# CI/CD Pipeline — Current Implementation

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Active |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2026-03-21 |
| **Parent Document** | [testing-strategy.md](testing-strategy.md) §7 |

---

## Table of Contents

1. [Overview](#1-overview)
2. [CI Strategy](#2-ci-strategy)
3. [Pipeline Architecture](#3-pipeline-architecture)
4. [Fast Tests (PR Gate)](#4-fast-tests-pr-gate)
5. [Integration Tests (Main Gate)](#5-integration-tests-main-gate)
6. [YugabyteDB Configuration](#6-yugabytedb-configuration)
7. [Local Development](#7-local-development)
8. [Performance Optimizations](#8-performance-optimizations)
9. [Reliability](#9-reliability)
10. [Future Enhancements](#10-future-enhancements)

---

## 1. Overview

The CI pipeline implements the test execution strategy defined in
[testing-strategy.md](testing-strategy.md) §7 as a GitHub Actions workflow
(`.github/workflows/ci.yml`). It enforces quality gates G1–G3 automatically:

| Gate | Stage | Trigger |
|------|-------|---------|
| G1 — Lint & Static Analysis | `fast-tests` | PR opened/updated |
| G2 — Unit Tests (≥ 80% coverage) | `fast-tests` | PR opened/updated |
| G3 — Integration Tests | `integration-tests` | Merge to `main`, nightly |

---

## 2. CI Strategy

The pipeline uses a **two-tier testing model** optimized for speed on PRs and
thoroughness on the main branch:

```
Pull Request                             Main Branch / Nightly
─────────────                            ─────────────────────
┌─────────────────────┐                  ┌─────────────────────────────┐
│  fast-tests (≤5m)   │                  │  integration-tests (≤15m)   │
│                     │                  │                             │
│  ✓ go mod verify    │                  │  ✓ YugabyteDB (single-node) │
│  ✓ go vet           │                  │  ✓ All 26 migrations apply  │
│  ✓ golangci-lint    │                  │  ✓ Table existence check    │
│  ✓ Unit tests       │                  │  ✓ CRUD operations          │
│  ✓ Coverage report  │                  │  ✓ FK cascade validation    │
│  ✓ Build            │                  │  ✓ Index verification       │
│  ✓ govulncheck      │                  │  ✓ Idempotency check        │
└─────────────────────┘                  └─────────────────────────────┘
```

### Trade-offs

| Aspect | Fast Tests | Integration Tests |
|--------|-----------|-------------------|
| **Speed** | ~3–5 minutes | ~10–15 minutes |
| **DB required** | No | Yes (YugabyteDB single-node) |
| **What's tested** | Code correctness, linting, build | Schema DDL, migrations, FK, indexes, CRUD |
| **What's NOT tested** | DB interactions | Multi-node replication, tablet splitting, cross-region placement |
| **Trigger** | Every PR push | Merge to `main`, nightly (02:00 UTC) |
| **Blocking** | Yes | Yes |

---

## 3. Pipeline Architecture

```yaml
# .github/workflows/ci.yml
on:
  push:
    branches: [main]          # Triggers both fast-tests + integration-tests
  pull_request:
    branches: [main]          # Triggers fast-tests only
  schedule:
    - cron: "0 2 * * *"       # Nightly integration tests
```

### Job Dependency Graph

```
pull_request ──► fast-tests ──► ✅ Merge allowed

push (main) ──┬► fast-tests
              └► integration-tests (YugabyteDB service container)

schedule ────► integration-tests (YugabyteDB service container)
```

---

## 4. Fast Tests (PR Gate)

**Job**: `fast-tests`
**Timeout**: 10 minutes
**Runs on**: `ubuntu-latest`

Steps executed in order:

| Step | Tool | Purpose |
|------|------|---------|
| Checkout | `actions/checkout@v4` | Clone repository |
| Setup Go | `actions/setup-go@v5` | Install Go, enable module cache |
| Verify deps | `go mod verify` | Check module checksums |
| Vet | `go vet ./...` | Static analysis |
| Lint | `golangci/golangci-lint-action@v6` | Code quality checks |
| Unit tests | `go test ./... -race` | All tests, race detector on |
| Coverage | `go test -coverprofile` | Coverage report (target ≥ 80%) |
| Build | `CGO_ENABLED=0 go build ./...` | Static binary build |
| Security | `govulncheck ./...` | Known vulnerability scan |

### Why no database?

Unit tests use the `testing` package and validate embedded SQL via string/regex
checks. The `//go:build integration` tag excludes integration tests from the
default `go test ./...` command, so no YugabyteDB is needed.

---

## 5. Integration Tests (Main Gate)

**Job**: `integration-tests`
**Timeout**: 20 minutes
**Runs on**: `ubuntu-latest`
**Condition**: Only runs on `push` to main or `schedule` events

### YugabyteDB Setup

Uses GitHub Actions [service containers](https://docs.github.com/en/actions/using-containerized-services/about-service-containers)
for a zero-configuration single-node YugabyteDB:

```yaml
services:
  yugabytedb:
    image: yugabytedb/yugabyte:2024.2.3.0-b0
    ports:
      - 5433:5433     # YSQL (PostgreSQL wire protocol)
    options: >-
      --health-cmd "postgres/bin/pg_isready -h localhost -p 5433 -U yugabyte"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 20
      --health-start-period 30s
```

### Why single-node, not a cluster?

| Validated by single-node | Requires multi-node cluster |
|--------------------------|-----------------------------|
| Schema DDL correctness | Tablet splitting behavior |
| Migration ordering | Cross-region Raft replication |
| FK constraints + cascades | Tablespace placement |
| Index creation | Leader election / failover |
| CRUD operations | Read-replica routing |
| JSONB queries | Auto-rebalancing |

Single-node covers all schema-level correctness. Multi-node behavior is validated
in staging environments (future Phase 9–10 work).

### Test Functions

| Test | What it validates |
|------|-------------------|
| `TestIntegrationMigrationsApplyAll` | All 26 migrations apply without error |
| `TestIntegrationSchemaTablesExist` | All 23 tables exist in `udm` schema |
| `TestIntegrationSubscriberCRUD` | INSERT/SELECT/UPDATE/DELETE on subscribers |
| `TestIntegrationForeignKeyCascade` | DELETE subscriber cascades to auth_data |
| `TestIntegrationIndexesExist` | All 18 indexes exist in pg_indexes |
| `TestIntegrationMigrationIdempotency` | Version tracking works correctly |

---

## 6. YugabyteDB Configuration

### Connection String

```
YUGABYTE_DSN=postgres://yugabyte:yugabyte@localhost:5433/yugabyte?sslmode=disable
```

The integration tests read `YUGABYTE_DSN` from the environment, falling back to
the default above. This allows local development with a custom connection string.

### Image Version

`yugabytedb/yugabyte:2024.2.3.0-b0` — the latest stable LTS release. Pin this
version to avoid unexpected breakage from upstream changes.

---

## 7. Local Development

### Running unit tests (no database needed)

```bash
make test
# or
go test ./... -count=1 -timeout 60s -race
```

### Running integration tests locally

1. Start YugabyteDB:
   ```bash
   docker run -d --name yugabyte \
     -p 5433:5433 -p 7000:7000 -p 9000:9000 \
     yugabytedb/yugabyte:2024.2.3.0-b0 \
     bin/yugabyted start --daemon=false
   ```

2. Wait for YSQL readiness:
   ```bash
   until pg_isready -h localhost -p 5433 -U yugabyte; do sleep 2; done
   ```

3. Run integration tests:
   ```bash
   make test-integration
   # or
   go test ./migrations/... -tags=integration -count=1 -timeout 5m -v
   ```

4. Custom connection string:
   ```bash
   YUGABYTE_DSN="postgres://user:pass@dbhost:5433/mydb" make test-integration
   ```

---

## 8. Performance Optimizations

| Optimization | How |
|-------------|-----|
| **Go module cache** | `actions/setup-go@v5` with `cache: true` — caches `$GOPATH/pkg/mod` and `$GOCACHE` |
| **Parallel jobs** | `fast-tests` and `integration-tests` run concurrently on main |
| **Build tag isolation** | Integration tests excluded from fast path via `//go:build integration` |
| **Service container health** | YugabyteDB readiness checked before tests start (no wasted time) |
| **Timeout limits** | 10min for fast-tests, 20min for integration — prevents runaway jobs |

---

## 9. Reliability

| Strategy | Implementation |
|----------|---------------|
| **Deterministic tests** | `-count=1` disables test caching; each run is fresh |
| **Race detection** | `-race` flag on unit tests catches data races |
| **Clean state** | Integration tests drop/recreate schema before each test function |
| **Health checks** | YugabyteDB service container has 20 retries with 10s interval |
| **No flaky patterns** | No artificial delays, no network simulation, no random ports |
| **Timeouts** | Job-level and test-level timeouts prevent hangs |

---

## 10. Future Enhancements

These capabilities will be added in later phases as defined in
[testing-strategy.md](testing-strategy.md) §7:

| Enhancement | Phase | Description |
|-------------|-------|-------------|
| API conformance tests | Phase 3 | Validate all 103 Nudm endpoints against OpenAPI spec |
| Container build + push | Phase 9 | Build and push Docker images on merge to main |
| Multi-node cluster tests | Phase 9 | 3-node YugabyteDB cluster for replication/failover testing |
| Performance baseline | Phase 10 | Weekly load tests with regression detection |
| Nightly telecom scenarios | Phase 11 | Full 5G registration/auth flows |
| Chaos engineering | Phase 11 | Fault injection with Chaos Mesh |
