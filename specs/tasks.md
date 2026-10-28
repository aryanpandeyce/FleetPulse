# FleetPulse Implementation Task Backlog

**Methodology:** Spec-Driven Development (SDD)  
**Tracking Rule:** Checkboxes must be marked `[x]` upon verified completion including tests.

---

## Phase 0: Specifications & Foundations

- [x] **TASK-P0-01**: Ratify Engineering Constitution, System Specification, Technical Plan, and Task Backlog
  - **Files Touched**: `specs/constitution.md`, `specs/spec.md`, `specs/plan.md`, `specs/tasks.md`
  - **Acceptance Criteria**: All 4 documents created with complete architecture, models, APIs, and criteria.
  - **Priority**: P0
  - **Dependencies**: None

---

## Phase 1: Core P0 Implementation

### Data Structures & Algorithms (`pkg/ds`)
- [x] **TASK-P0-02**: Implement Consistent Hash Ring with Virtual Nodes
  - **Files Touched**: `pkg/ds/consistenthash/ring.go`, `pkg/ds/consistenthash/ring_test.go`
  - **Acceptance Criteria**: Virtual node hashing, binary search node lookup, distribution variance $\le 15\%$ on 1,000 keys, unit tests with `-race`, benchmarks.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

- [x] **TASK-P0-03**: Implement Generic Binary Min-Heap Priority Queue
  - **Files Touched**: `pkg/ds/minheap/minheap.go`, `pkg/ds/minheap/minheap_test.go`
  - **Acceptance Criteria**: Custom implementation without `container/heap`, generic comparator, $O(\log N)$ push/pop, $O(1)$ peek, comprehensive unit tests and benchmarks.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

- [x] **TASK-P0-04**: Implement Token Bucket Rate Limiter with Mockable Clock
  - **Files Touched**: `pkg/ds/tokenbucket/limiter.go`, `pkg/ds/tokenbucket/limiter_test.go`
  - **Acceptance Criteria**: Fractional refill calculation, thread-safe consumption, burst capacity enforcement, fake clock testing, zero external deps.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

- [x] **TASK-P0-05**: Implement Thread-Safe LRU Cache
  - **Files Touched**: `pkg/ds/lru/lru.go`, `pkg/ds/lru/lru_test.go`
  - **Acceptance Criteria**: Doubly linked list + map, thread-safe with `sync.RWMutex`, $O(1)$ get/put/eviction, hit/miss statistics, optional TTL, tests + benchmarks.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

- [x] **TASK-P0-06**: Implement EWMA & Rolling Z-Score Analytics Engine
  - **Files Touched**: `pkg/ds/slidingwindow/ewma.go`, `pkg/ds/slidingwindow/ewma_test.go`
  - **Acceptance Criteria**: Exponential smoothing, rolling variance (Welford or sliding window), z-score calculation, warm-up period enforcement, unit tests with synthetic anomaly spikes.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

### Foundational Shared Libraries & API Contracts
- [x] **TASK-P0-07**: Implement Foundational Infrastructure Packages
  - **Files Touched**: `pkg/logging/logger.go`, `pkg/auth/jwt.go`, `pkg/httpx/middleware.go`, `pkg/grpcx/interceptors.go`, `pkg/metrics/metrics.go`
  - **Acceptance Criteria**: `slog` JSON formatting with context request IDs, JWT signing/claims parsing, HTTP recovery/CORS/request ID middlewares, gRPC interceptors for `org_id` metadata, Prometheus RED collectors.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-01

- [x] **TASK-P0-08**: Define Protocol Buffers & Public OpenAPI Specification
  - **Files Touched**: `api/proto/inventory.proto`, `api/proto/telemetry.proto`, `api/proto/workflow.proto`, `api/openapi.yaml`, `api/proto/*.pb.go`
  - **Acceptance Criteria**: Complete RPC definitions for all 3 internal services; OpenAPI 3.0 yaml defining all public Gateway endpoints; generated Go gRPC stubs.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-07

### Microservices Implementation
- [ ] **TASK-P0-09**: Implement Inventory Service & PostgreSQL Migrations
  - **Files Touched**: `services/inventory-service/...`, `scripts/migrations/001_initial_schema.sql`
  - **Acceptance Criteria**: SQL schema migrations with keyset pagination indexes, domain models, pgx repository, LRU cache integration, claim flow with single-use verification, compliance engine detecting drift, gRPC server, table-driven unit tests.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-05, TASK-P0-08

- [ ] **TASK-P0-10**: Implement API Gateway with Multi-Tenancy & Rate Limiting
  - **Files Touched**: `services/gateway/...`
  - **Acceptance Criteria**: Public Chi router, bcrypt user authentication, JWT issuance & validation, RBAC middleware, gRPC clients forwarding `org_id` metadata, Redis token-bucket rate limiting with in-memory fallback, tenant isolation tests.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-04, TASK-P0-07, TASK-P0-09

- [ ] **TASK-P0-11**: Implement Device Simulator with Redfish REST & NATS Connector
  - **Files Touched**: `services/device-simulator/...`
  - **Acceptance Criteria**: Spawns $N$ simulated servers (`UCS-C220-M7`, `UCS-C240-M7`, `UCS-X210c`), seeded random-walk telemetry, Redfish REST API, fault injection API (`POST /sim/faults`), NATS JetStream publisher with consistent-hash worker pool sharding.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-02, TASK-P0-08

- [ ] **TASK-P0-12**: Implement Telemetry Service & Anomaly Detection
  - **Files Touched**: `services/telemetry-service/...`
  - **Acceptance Criteria**: NATS JetStream consumer with durable queue group, Redis ring buffer storage (last 120 samples), stateful consecutive alarm rule evaluation, EWMA + z-score anomaly detector flagging spikes, gRPC server, unit + benchmark tests.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-06, TASK-P0-08

- [ ] **TASK-P0-13**: Implement Workflow Service with Priority Min-Heap Scheduler
  - **Files Touched**: `services/workflow-service/...`
  - **Acceptance Criteria**: Min-heap priority scheduling queue, idempotency key deduplication, exponential backoff retries, firmware upgrade execution against simulated Redfish endpoints, gRPC API, unit tests.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-03, TASK-P0-08

### Full Stack Composition, CI & Developer Experience
- [ ] **TASK-P0-14**: Full Stack Docker Compose, Seed Data & Makefile
  - **Files Touched**: `deploy/docker-compose.yml`, `scripts/seed.sql`, `Makefile`
  - **Acceptance Criteria**: Multi-service Docker Compose (Postgres, Redis, NATS, LocalStack, Gateway, Inventory, Telemetry, Workflow, Simulator), seed script populating 2 orgs and 3 users each, working `make up`, `make test`, `make lint`.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-09, TASK-P0-10, TASK-P0-11, TASK-P0-12, TASK-P0-13

- [ ] **TASK-P0-15**: Multi-Stage Dockerfiles & GitHub Actions CI with Coverage Gate
  - **Files Touched**: `deploy/Dockerfile.*`, `.github/workflows/ci.yml`
  - **Acceptance Criteria**: Non-root alpine/distroless Dockerfiles; CI workflow running `golangci-lint`, `go test -race -cover`, enforcing $\ge 80\%$ statement coverage gate on `internal/` packages.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-14

- [ ] **TASK-P0-16**: React + Vite + TypeScript Dashboard with Demo Mode
  - **Files Touched**: `dashboard/...`, `.github/workflows/dashboard-pages.yml`
  - **Acceptance Criteria**: Responsive modern UI with Tailwind + Recharts; Fleet Overview (health donut, model distribution, alarm trends), Devices list with filters, Device Detail with live charts & anomaly badges, Alarms management, Workflows; built-in deterministic Demo Mode for GitHub Pages.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-08

- [ ] **TASK-P0-17**: Comprehensive Production README & Technical Documentation
  - **Files Touched**: `README.md`, `docs/dsa.md`, `docs/security.md`
  - **Acceptance Criteria**: Professional pitch, badges, architecture Mermaid diagram, tech stack table, 3-command quick start, design trade-offs, DSA section with complexity, reproducible benchmark steps, coverage notes.
  - **Priority**: P0
  - **Dependencies**: TASK-P0-14, TASK-P0-16

---

## Phase 2: P1 Capabilities

- [ ] **TASK-P1-01**: Production Helm Chart & Kind Local Cluster Automation
  - **Files Touched**: `deploy/helm/fleetpulse/...`, `deploy/kind-config.yaml`
  - **Acceptance Criteria**: Deployments with probes, HPA, PDB, RollingUpdate strategy, `make kind-up` script.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-14

- [ ] **TASK-P1-02**: Prometheus Metrics Instrumentation & 3 Provisioned Grafana Dashboards
  - **Files Touched**: `deploy/grafana/dashboards/*.json`, `deploy/grafana/provisioning/...`
  - **Acceptance Criteria**: Fleet Overview, Service RED/SLO, and Telemetry & Workflow Pipeline dashboards automatically loaded in Grafana.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-14

- [ ] **TASK-P1-03**: k6 Load Testing Suite with Real Benchmark Execution
  - **Files Touched**: `loadtest/gateway_read.js`, `loadtest/telemetry_storm.js`, `loadtest/workflow_burst.js`, `loadtest/results/*.json`
  - **Acceptance Criteria**: Real k6 test runs executed on local machine, thresholds enforced, actual measured req/s and latencies documented in README.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-14

- [ ] **TASK-P1-04**: Infrastructure as Code with Terraform & LocalStack
  - **Files Touched**: `deploy/terraform/main.tf`, `deploy/terraform/s3.tf`, `deploy/terraform/sqs.tf`
  - **Acceptance Criteria**: Terraform provisions S3 buckets (`fleetpulse-firmware`, telemetry archives) and SQS DLQ queues against LocalStack.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-14

- [ ] **TASK-P1-05**: Redis Lease Leader Election & Stuck Job Crash Recovery
  - **Files Touched**: `services/workflow-service/internal/scheduler/leader.go`, `services/workflow-service/internal/scheduler/leader_test.go`
  - **Acceptance Criteria**: Distributed lease with heartbeats; proven failover test; automatic recovery of jobs stuck in `running` state.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-13

- [ ] **TASK-P1-06**: Circuit Breaker & Dead-Letter Queue Handling
  - **Files Touched**: `pkg/circuitbreaker/breaker.go`, `pkg/circuitbreaker/breaker_test.go`
  - **Acceptance Criteria**: Three-state circuit breaker (Closed, Open, Half-Open), poison messages forwarded to `telemetry.dlq` and `workflow.dlq`.
  - **Priority**: P1
  - **Dependencies**: TASK-P0-07, TASK-P0-12, TASK-P0-13

---

## Phase 3: P2 Capabilities (If Time Permits)

- [ ] **TASK-P2-01**: OpenTelemetry Distributed Tracing (OTLP to Jaeger)
  - **Files Touched**: `pkg/tracing/...`, `deploy/docker-compose.yml`
  - **Priority**: P2
  - **Dependencies**: TASK-P1-02

- [ ] **TASK-P2-02**: Kubernetes NetworkPolicies & Loki Structured Log Aggregation
  - **Files Touched**: `deploy/helm/fleetpulse/templates/networkpolicy.yaml`, `deploy/loki/...`
  - **Priority**: P2
  - **Dependencies**: TASK-P1-01

- [ ] **TASK-P2-03**: Dashboard Live Mode Integration against Gateway REST API
  - **Files Touched**: `dashboard/src/services/api.ts`
  - **Priority**: P2
  - **Dependencies**: TASK-P0-10, TASK-P0-16

- [ ] **TASK-P2-04**: Rolling Batch Upgrades with Pause, Resume & Cancel Controls
  - **Files Touched**: `services/workflow-service/internal/executor/rolling.go`
  - **Priority**: P2
  - **Dependencies**: TASK-P0-13

---

## Phase 4: Verification Against Specification

- [ ] **TASK-P4-01**: Verification Report & Pass/Fail Matrix
  - **Files Touched**: `specs/verification.md`
  - **Acceptance Criteria**: Systematic audit of all AC-01 through AC-22 criteria with pass/fail status and evidence logs.
  - **Priority**: P0
  - **Dependencies**: All completed P0/P1 tasks
