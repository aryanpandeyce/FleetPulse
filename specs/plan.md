# FleetPulse Technical Architecture & Execution Plan

**Version:** 1.0.0  
**Status:** Approved  
**Classification:** Core System Architecture Document  

---

## 1. System Architecture Overview

FleetPulse is structured as a cloud-native, microservices-based distributed platform designed for horizontal scalability, multi-tenant isolation, and fault tolerance.

### 1.1 High-Level Component Architecture (Mermaid)

```mermaid
flowchart TB
    subgraph Clients["Clients & External Edge"]
        DASH["React Web Dashboard\n(Vite + Tailwind + Recharts)"]
        CLI["Admin / Operator Tools\n(cURL / k6 / Scripts)"]
    end

    subgraph Edge["Gateway Layer"]
        GW["API Gateway (Chi REST)\n- JWT Auth & RBAC\n- Multi-Tenant Context\n- Redis Token-Bucket Limiter\n- Keyset Pagination Proxy"]
    end

    subgraph Services["Core Microservices (Go 1.22+)"]
        INV["Inventory Service (gRPC)\n- Hardware Inventory\n- Device Claim Flow\n- Server Profiles\n- Compliance Engine\n- LRU Cache"]
        TEL["Telemetry Service (gRPC + Consumer)\n- NATS Consumer Group\n- EWMA & Rolling Z-Score\n- Stateful Alarm Engine\n- Redis Ring Buffer & Rollups"]
        WF["Workflow Service (gRPC + Scheduler)\n- Priority Scheduler (Min-Heap)\n- Concurrency Throttling\n- Idempotency & Retries\n- Leader Lease (Redis)"]
    end

    subgraph Simulation["Edge Simulation Layer"]
        SIM["Device Simulator (N Servers)\n- Redfish REST Server\n- Random Walk Telemetry\n- Fault Injection API\n- Consistent-Hash Connector"]
    end

    subgraph State["State & Message Bus Infrastructure"]
        PG[("PostgreSQL 16\n(Relational State &\nTime-series Rollups)")]
        REDIS[("Redis 7\n(Ring Buffers, Rate Limits,\nLRU & Leader Leases)")]
        NATS[("NATS JetStream 2.10\n(High-Throughput Bus\ntelemetry.> & events)")]
        LOCALSTACK[("LocalStack (AWS)\n(S3 Firmware Bundles,\nSQS DLQ & Events)")]
    end

    DASH -->|REST / HTTPS| GW
    CLI -->|REST / HTTPS| GW

    GW -->|gRPC org_id metadata| INV
    GW -->|gRPC org_id metadata| TEL
    GW -->|gRPC org_id metadata| WF
    GW -.->|Token Bucket / Cache| REDIS

    SIM -->|Auto-Claim HTTP/gRPC| INV
    SIM -->|Publish Telemetry| NATS
    SIM -.->|Simulated Redfish| WF

    NATS -->|Stream Consume telemetry.>| TEL
    TEL -->|Write Buffer| REDIS
    TEL -->|Persist Rollups & Alarms| PG
    TEL -.->|Check Device Metadata| INV

    INV -->|Relational Queries| PG
    INV -->|Publish device.claimed| NATS

    WF -->|Jobs State & History| PG
    WF -->|Leader Lease| REDIS
    WF -->|Publish job.events| NATS
    WF -->|Upload / Fetch Manifests| LOCALSTACK
```

---

## 2. Technology Choices & Justification

### 2.1 Single Go Module vs. Go Workspace (`go.work`)
* **Decision:** **Single Go Module (`go.mod`) with clean internal package boundaries.**
* **Justification:** While multi-module Go workspaces offer independent versioning, they significantly complicate CI pipelines, dependency caching, local multi-container Docker image builds, and cross-package testing (`go test -race ./...`). A single root module with rigorous directory scoping (`pkg/` for shared libraries, `services/<name>/cmd` for binary entrypoints, and `services/<name>/internal` for private domains) guarantees atomic refactoring, zero version drift between internal services, fast reproducible builds, and seamless Docker container caching.

### 2.2 Inbound Routing: Chi vs. Gin vs. Fiber
* **Decision:** `github.com/go-chi/chi/v5`
* **Justification:** Chi is 100% compliant with standard `net/http` handlers and contexts (`http.HandlerFunc`). Unlike Fiber (which is based on Fasthttp and introduces context lifecycle incompatibilities) or Gin (which enforces custom context types and heavy abstractions), Chi allows seamless integration of standard middleware (CORS, Prometheus metrics, tracing, context propagation) with negligible memory allocation overhead.

### 2.3 Persistence: `pgx/v5` vs. GORM vs. `database/sql`
* **Decision:** `github.com/jackc/pgx/v5` (native connection pooling via `pgxpool`)
* **Justification:** GORM introduces hidden queries, reflection overhead, and unpredictable query generation for keyset cursor pagination and complex JSONB queries. `pgx` is the fastest PostgreSQL driver in Go, supports binary protocol encoding, native JSONB conversion, connection pool health checks, and explicit SQL query inspection (`EXPLAIN ANALYZE`).

### 2.4 Event Bus: NATS JetStream vs. Apache Kafka vs. RabbitMQ
* **Decision:** NATS JetStream (`nats.go`)
* **Justification:** NATS JetStream is written in Go, executes with single-digit millisecond latency, has a tiny memory footprint (<30MB RAM), and provides built-in persistence, stream de-duplication, subject-based filtering (`telemetry.<tenant>.<serial>`), durable consumer groups, and horizontal queue scalability without the operational weight or Zookeeper/KRaft complexity of Kafka.

### 2.5 Caching & In-Memory State: Redis 7
* **Decision:** `github.com/redis/go-redis/v9`
* **Justification:** Redis delivers sub-millisecond sorted sets (used for sliding window telemetry ring buffers), atomic Lua script execution for rate limiting, distributed leases for leader election, and low-latency caching.

---

## 3. Package Layout & Directory Structure

```
fleetpulse/
├── go.mod                      # Single root module: github.com/fleetpulse/fleetpulse
├── go.sum
├── Makefile                    # Standard developer workflows: build, test, lint, run, demo
├── specs/                      # Engineering constitution, specs, plan, task backlog, verification
│   ├── constitution.md
│   ├── spec.md
│   ├── plan.md
│   ├── tasks.md
│   └── verification.md
├── pkg/                        # Reusable foundational packages (Zero internal cross-imports)
│   ├── ds/                     # Core Data Structures & Algorithms
│   │   ├── consistenthash/     # Virtual-node consistent hash ring
│   │   ├── minheap/            # Generic binary priority min-heap
│   │   ├── tokenbucket/        # Atomic token-bucket rate limiter with mockable clock
│   │   ├── lru/                # Thread-safe doubly-linked list LRU cache
│   │   └── slidingwindow/      # EWMA, rolling variance, and z-score utilities
│   ├── auth/                   # JWT creation, claims validation, RBAC context helpers
│   ├── logging/                # slog structured JSON formatter with context propagation
│   ├── metrics/                # Standardized Prometheus metrics collectors (RED metrics)
│   ├── httpx/                  # Middleware (RequestID, Recovery, CORS, Timeout, RateLimit)
│   └── grpcx/                  # gRPC interceptors for org_id metadata, errors, metrics
├── api/
│   ├── proto/                  # Protobuf service definitions
│   │   ├── inventory.proto
│   │   ├── telemetry.proto
│   │   └── workflow.proto
│   └── openapi.yaml            # Public Gateway REST API contract (OpenAPI 3.0)
├── services/
│   ├── device-simulator/       # Simulated hardware fleet & connector
│   │   ├── cmd/main.go
│   │   └── internal/{api,model,engine,connector}
│   ├── gateway/                # Reverse proxy, auth, tenant isolation, rate limiter
│   │   ├── cmd/main.go
│   │   └── internal/{api,middleware,client,auth}
│   ├── inventory-service/      # Inventory, claims, profiles, compliance
│   │   ├── cmd/main.go
│   │   └── internal/{domain,repo,service,api}
│   ├── telemetry-service/      # Telemetry ingest, alarms, anomaly detection
│   │   ├── cmd/main.go
│   │   └── internal/{domain,consumer,engine,repo,api}
│   └── workflow-service/       # Job queue, min-heap scheduler, S3/SQS client, leader
│       ├── cmd/main.go
│       └── internal/{domain,scheduler,executor,repo,api}
├── deploy/
│   ├── docker-compose.yml      # Complete self-contained stack
│   ├── Dockerfile.*            # Multi-stage container builds
│   ├── helm/fleetpulse/        # Production Helm chart (HPA, PDB, Ingress)
│   ├── terraform/              # LocalStack AWS resources (S3, SQS)
│   └── grafana/                # Provisioned dashboard JSONs
├── dashboard/                  # React 18 + TypeScript + Vite + Tailwind + Recharts
├── loadtest/                   # k6 load testing scripts and recorded output
└── scripts/                    # Migration scripts, demo scenario runner, seed generator
```

---

## 4. Data Structures & Algorithms Analysis (`pkg/ds`)

| Data Structure / Algorithm | Package Path | Time Complexity | Space Complexity | Concrete Service Usage |
|---|---|---|---|---|
| **Consistent Hash Ring** | `pkg/ds/consistenthash` | Add Node: $O(V \log(N \cdot V))$\<br\>Lookup: $O(\log(N \cdot V))$ | $O(N \cdot V)$ where $N$=nodes, $V$=virtual nodes | Sharding 10,000 simulated servers across connector worker pools and telemetry consumer queue groups. |
| **Generic Binary Min-Heap** | `pkg/ds/minheap` | Push: $O(\log K)$\<br\>Pop: $O(\log K)$\<br\>Peek: $O(1)$ | $O(K)$ where $K$=queued jobs | Priority job scheduler in `workflow-service` ordering by priority and submission timestamp. |
| **Token Bucket Limiter** | `pkg/ds/tokenbucket` | Take/Allow: $O(1)$ | $O(1)$ per bucket | API Gateway rate limiting per tenant and per IP, executed in-memory and via Redis Lua scripts. |
| **Thread-Safe LRU Cache** | `pkg/ds/lru` | Get: $O(1)$\<br\>Put: $O(1)$\<br\>Evict: $O(1)$ | $O(C)$ where $C$=capacity | Hot device metadata and server profile cache in `inventory-service`. |
| **EWMA & Rolling Z-Score** | `pkg/ds/slidingwindow` | Update: $O(1)$\<br\>Z-Score: $O(1)$ | $O(W)$ where $W$=warm-up window | Real-time anomaly detection in `telemetry-service` tracking sensor deviations without recomputing historical points. |

---

## 5. Component Interaction & Sequence Diagrams

### 5.1 Device Claim & Telemetry Streaming Sequence (Mermaid)

```mermaid
sequenceDiagram
    autonumber
    actor Admin as Org Admin
    participant GW as API Gateway
    participant INV as Inventory Service
    participant SIM as Device Simulator
    participant NATS as NATS JetStream
    participant TEL as Telemetry Service
    participant REDIS as Redis (Ring Buffer)

    Admin->>GW: POST /api/v1/devices/claims
    GW->>INV: gRPC CreateClaimCode(org_id)
    INV-->>GW: Return claim_code (TTL: 15m)
    GW-->>Admin: 201 Created (claim_code)

    SIM->>INV: gRPC ClaimDevice(serial, model, claim_code)
    INV->>INV: Verify single-use code & bind device to org_id
    INV->>NATS: Publish "inventory.device.claimed"
    INV-->>SIM: 200 OK (Claimed, org_id)

    loop Every SIM_INTERVAL (5s) with Jitter
        SIM->>NATS: Publish to "telemetry.{org_id}.{serial}"
        NATS-->>TEL: Durable Consumer Push
        TEL->>REDIS: LPUSH/ZADD sample to "telemetry:ring:{device_id}"
        TEL->>TEL: Evaluate Alarm Rules & Calculate Anomaly Z-Score
    end
```

### 5.2 Anomaly Detection & Stateful Alarm Sequence (Mermaid)

```mermaid
sequenceDiagram
    autonumber
    participant TEL as Telemetry Engine
    participant SW as EWMA & Z-Score (pkg/ds)
    participant DB as PostgreSQL (Alarms)
    participant NATS as NATS JetStream
    actor Operator as Fleet Operator
    participant GW as API Gateway

    TEL->>SW: Ingest sample value (e.g. inlet_temp: 38.5C)
    SW->>SW: Update EWMA mean & rolling variance
    SW->>SW: Calculate z = (value - mean) / stddev
    
    alt |z| > 3.0 and samples >= 30 (Anomaly Detected)
        TEL->>DB: Query active alarm for (device_id, "AnomalyDetected")
        alt Not Active
            TEL->>DB: INSERT INTO alarms (severity='critical', status='raised')
            TEL->>NATS: Publish alarm notification
        end
    else Normal Telemetry
        TEL->>DB: Check if previously raised alarm can be auto-cleared
    end

    Operator->>GW: POST /api/v1/alarms/{id}/acknowledge
    GW->>DB: UPDATE alarms SET status='acknowledged', acknowledged_by=user_id
    GW-->>Operator: 200 OK
```

### 5.3 Workflow Execution & Rolling Batch Upgrade (Mermaid)

```mermaid
sequenceDiagram
    autonumber
    actor Operator as Fleet Operator
    participant GW as API Gateway
    participant WF as Workflow Service
    participant HEAP as Min-Heap Priority Queue
    participant SIM as Device Simulator (Redfish)
    participant DB as PostgreSQL (Jobs)

    Operator->>GW: POST /api/v1/workflows (type="firmware_upgrade", priority=1, targets=[...])
    GW->>WF: gRPC CreateJob(org_id, idempotency_key, ...)
    WF->>DB: Check idempotency_key
    WF->>DB: INSERT INTO workflow_jobs (state='queued')
    WF->>HEAP: Push job into Min-Heap
    WF-->>GW: 201 Created (job_id)
    GW-->>Operator: Return Job Details

    loop Scheduler Loop (Worker Pool)
        WF->>HEAP: Pop highest priority job
        WF->>DB: UPDATE workflow_jobs SET state='running'
        
        loop Batches of Size B
            WF->>SIM: POST /redfish/v1/Systems/{serial}/Actions/FirmwareUpdate
            SIM-->>WF: 202 Accepted (Task URI)
            loop Poll until complete
                WF->>SIM: GET /redfish/v1/TaskService/Tasks/{id}
                SIM-->>WF: Progress % / State
            end
            alt Batch Failure Rate > max_failure_percent
                WF->>DB: UPDATE workflow_jobs SET state='failed' (Aborted)
                break
            end
        end

        WF->>DB: UPDATE workflow_jobs SET state='succeeded'
    end
```

---

## 6. Failure Modes & Resilience Strategies

| Failure Mode | Impact | Detection | Automatic Recovery Strategy |
|---|---|---|---|
| **Redis Node Outage** | Rate limiting and ring buffer caching unavailable. | Connection timeout in Redis client. | Gateway falls back to local in-memory token bucket; telemetry service continues batching writes straight to PostgreSQL rollups. |
| **PostgreSQL Transient Failure** | Relational queries and write operations fail. | `pgxpool` ping failures and error returns. | gRPC clients retry with exponential backoff; telemetry service buffers rollups in memory; circuit breaker opens to prevent thundering herd. |
| **NATS Broker Failure / Partition** | Telemetry ingestion and event publishing stalled. | Disconnect event in `nats.go` connection handler. | Simulator connector buffers samples locally up to memory limit; reconnects automatically with exponential backoff and publishes upon restoration. |
| **Workflow Worker Crash** | In-flight jobs left in `running` state indefinitely. | Startup sweep and periodic leader lease heartbeat. | Leader election sweeper discovers jobs in `running` state with `updated_at` older than threshold (5m) and re-enqueues them with incremented attempt count. |
| **Split-Brain Leader Sweeper** | Multiple instances attempt to run scheduled reconciliations. | Expired leases or competing lock acquisition. | Distributed lock via Redis key with monotonic fence token and short TTL (10s), renewed every 3s. PostgreSQL advisory locks as secondary guard. |
| **Slow Telemetry Consumer** | Telemetry queue accumulates lag under load storm. | Prometheus metric `ingest_lag_seconds`. | JetStream queue groups scale horizontally across multiple telemetry-service pods; consistent-hash sharding distributes partitions. |
