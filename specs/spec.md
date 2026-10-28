# FleetPulse System Specification

**Version:** 1.0.0  
**Status:** Approved  
**Classification:** Core System Specification  

---

## 1. Product Overview

FleetPulse is a cloud-native, multi-tenant server fleet management platform modeled after enterprise infrastructure control planes such as Cisco Intersight. FleetPulse enables IT operations teams, managed service providers, and data center engineers to register, monitor, audit, and orchestrate large fleets of bare-metal and rack-mount compute servers (simulated UCS C-series and X-series servers).

The platform addresses four core operational pillars:
1. **Inventory & Asset Management:** Centralized device claiming, hardware inventory, firmware bundle tracking, and desired-state server profiles.
2. **Real-Time Telemetry & Aggregation:** High-throughput streaming of thermal, power, fan, and memory health metrics over NATS JetStream, with Redis in-memory ring buffers and historical rollups.
3. **Proactive Analytics & Alarms:** Configurable stateful alarm threshold evaluation coupled with pure Go unsupervised anomaly detection (EWMA control charts and rolling z-scores).
4. **Resilient Asynchronous Workflows:** Distributed, priority-driven execution of firmware upgrades, rolling batch deployments, and drift remediation with crash-recovery and idempotency.

---

## 2. Personas and User Roles

| Persona | Role | Key Responsibilities |
|---|---|---|
| **Org Admin** | `admin` | Organization management, user provisioning, device claiming, firmware bundle approval, policy definition, and system configuration. |
| **Fleet Operator** | `operator` | Fleet health monitoring, alarm triage and acknowledgment, initiating firmware upgrades, triggering drift remediation jobs. |
| **System Viewer** | `viewer` | Read-only access to inventory, telemetry charts, compliance status, and job logs. |

---

## 3. User Stories

### 3.1 Device Registration & Claiming
- **US-01:** *As an Org Admin*, I want to generate a secure, single-use, time-bound claim code so that a newly deployed server or virtual simulator can securely register itself into my tenant without exposing long-lived credentials.
- **US-02:** *As a Device Connector*, I want to submit my serial number, model, and claim code upon boot so that I am authenticated and transitioned to a `claimed` state under the target organization.

### 3.2 Inventory & Fleet Management
- **US-03:** *As an Operator*, I want to browse fleet inventory using keyset (cursor) pagination and multi-attribute filters (model, health status, firmware version, tags, connection state) so that I can efficiently navigate fleets of 10,000+ servers without database degradation.
- **US-04:** *As an Org Admin*, I want to define Server Profiles specifying target firmware versions, boot orders, and power policies, and attach them to devices or device pools.
- **US-05:** *As an Operator*, I want to view compliance drift reports that contrast actual hardware state against assigned profiles and highlight discrepancies.

### 3.3 Real-Time Telemetry & Alarm Management
- **US-06:** *As an Operator*, I want to view live sensor telemetry (inlet/CPU temperatures, fan RPMs, PSU wattages, memory error counts) updated every 5 seconds with minimal latency.
- **US-07:** *As an Org Admin*, I want to configure stateful alarm rules (e.g., `inlet_temp > 35°C for 3 consecutive samples`) so that transient spikes do not produce alert fatigue.
- **US-08:** *As an Operator*, I want to acknowledge active alarms, track alarm lifecycle transitions (`raised` -> `acknowledged` -> `cleared`), and view deduplicated active alarms.

### 3.4 Unsupervised Anomaly Detection
- **US-09:** *As an Operator*, I want the system to continuously calculate rolling z-scores and EWMA statistics on telemetry streams to flag anomalous deviations before hard failure thresholds are breached, complete with human-readable explanations.

### 3.5 Workflow Orchestration & Remediation
- **US-10:** *As an Operator*, I want to trigger asynchronous firmware upgrade jobs across individual servers or batches with customizable concurrency limits, priority levels, and automatic rollback/abort upon failure thresholds.
- **US-11:** *As an Operator*, I want to initiate automated drift remediation to realign drifted servers with their assigned profiles.
- **US-12:** *As a Platform Engineer*, I want workflows to guarantee at-least-once execution with idempotency keys and automatic recovery of orphaned jobs after service crashes.

---

## 4. Functional Requirements

### 4.1 Device Simulator (`services/device-simulator`)
- **FR-SIM-01:** Support spawning $N$ virtual servers (default 1,000, configurable up to 10,000 via `SIM_DEVICE_COUNT`).
- **FR-SIM-02:** Provide realistic simulated hardware models: `UCS-C220-M7`, `UCS-C240-M7`, and `UCS-X210c`.
- **FR-SIM-03:** Maintain internal state per device: `serial`, `model`, `firmware_version`, `power_state`, `health` (OK/Warning/Critical), and sensor readings.
- **FR-SIM-04:** Implement random-walk telemetry generator with seeded RNG (`SIM_SEED`) producing:
  - `inlet_temp_celsius` (normal range: 18.0 - 27.0)
  - `cpu_temp_celsius` (normal range: 40.0 - 75.0)
  - `fan_rpm` (4 sensors, normal range: 4,000 - 9,500)
  - `power_watts` (2 PSUs, normal range: 120.0 - 380.0)
  - `memory_errors_correctable` (cumulative counter)
- **FR-SIM-05:** Expose Redfish-compatible HTTP endpoints:
  - `GET /redfish/v1/Systems/{serial}`
  - `GET /redfish/v1/Systems/{serial}/Thermal`
  - `GET /redfish/v1/Systems/{serial}/Power`
  - `GET /redfish/v1/Systems/{serial}/Firmware`
  - `POST /redfish/v1/Systems/{serial}/Actions/FirmwareUpdate`
- **FR-SIM-06:** Provide a Fault Injection API (`POST /sim/faults`) to dynamically inject:
  - Thermal spike (`temperature_spike`)
  - Fan failure (`fan_failure`)
  - Memory error storm (`memory_error_storm`)
  - Device disconnect / offline (`offline`)
- **FR-SIM-07:** Integrated device connector that auto-claims devices against inventory-service and streams telemetry samples to NATS JetStream subject `telemetry.<tenant_id>.<serial>` every `SIM_INTERVAL` (default 5s) using consistent-hash sharded worker pools.

### 4.2 API Gateway (`services/gateway`)
- **FR-GW-01:** Expose public REST API adhering to OpenAPI 3.0 under `/api/v1`.
- **FR-GW-02:** Implement JWT authentication (configurable HS256/RS256) validating `sub`, `org_id`, and `role` claims with token expiration (15m access, rotating refresh tokens).
- **FR-GW-03:** Enforce Role-Based Access Control (`admin`, `operator`, `viewer`) on all protected routes.
- **FR-GW-04:** Enforce strict tenant isolation: propagate `org_id` in gRPC metadata for downstream calls; reject any cross-tenant resource access.
- **FR-GW-05:** Implement a distributed Token Bucket rate limiter using Redis Lua script for atomic state updates with in-memory fallback. Enforce per-org and per-IP rate limits with standard `429 Too Many Requests` and `Retry-After` headers.
- **FR-GW-06:** Middleware pipeline: Request ID generation (`X-Request-ID`), structured access logging, panic recovery, CORS handling, and health/readiness endpoints (`/healthz`, `/readyz`, `/metrics`).
- **FR-GW-07:** Native credential store for demo organizations and users with bcrypt password hashing.

### 4.3 Inventory Service (`services/inventory-service`)
- **FR-INV-01:** Manage relational persistence for `organizations`, `devices`, `device_claims`, `firmware_bundles`, `server_profiles`, and `compliance_results` using PostgreSQL.
- **FR-INV-02:** Device Claim Lifecycle:
  - Admin generates claim code with configurable TTL (default 15 minutes).
  - Simulator submits serial + claim code.
  - Device is bound to organization; claim code is invalidated (single-use).
  - Publish `inventory.device.claimed` event to NATS.
- **FR-INV-03:** Keyset (cursor-based) pagination for device queries:
  - Deterministic ordering by `(created_at, id)`.
  - Filter support for `model`, `health`, `firmware_version`, `tag`, and `connection_state`.
  - High-performance indexed query execution.
- **FR-INV-04:** Optimistic concurrency control using a `version` column on mutable records.
- **FR-INV-05:** Compliance Engine:
  - Compare device active state (`firmware_version`, `power_policy`, `boot_order`) with attached `server_profile`.
  - Generate `compliance_results` listing specific drift details.
  - Trigger evaluation on profile updates and periodically.
  - Publish `inventory.drift.detected` on state changes.
- **FR-INV-06:** Thread-safe in-memory LRU cache (`pkg/ds/lru`) to accelerate hot device lookups.

### 4.4 Telemetry Service (`services/telemetry-service`)
- **FR-TEL-01:** Ingest telemetry messages from NATS JetStream subject `telemetry.>` using durable consumers and queue groups for load distribution.
- **FR-TEL-02:** Store the latest $N$ telemetry samples per device in Redis ring buffers (sliding 1-hour window) for immediate real-time dashboard queries.
- **FR-TEL-03:** Aggregate metrics into PostgreSQL rollup tables (1-minute and 5-minute averages, min, max, sample count).
- **FR-TEL-04:** Stateful Alarm Evaluation Engine:
  - Maintain sliding sample windows per metric per device.
  - Support rules such as `inlet_temp > threshold for N consecutive samples`.
  - State machine: `raised` -> `acknowledged` -> `cleared`.
  - Deduplicate active alarms for the same device and rule.
- **FR-TEL-05:** Unsupervised Anomaly Detection:
  - Compute Exponentially Weighted Moving Average (EWMA) and rolling standard deviation per metric per device.
  - Calculate rolling z-score: $z = \frac{x - \mu}{\sigma}$.
  - Flag anomaly if $|z| > \text{threshold}$ (default 3.0) after a warm-up period of 30 samples.
  - Raise `AnomalyDetected` alarm with metadata (metric, observed, expected range, z-score).
- **FR-TEL-06:** Expose gRPC API for `GetDeviceTelemetry`, `ListAlarms`, `AcknowledgeAlarm`, `GetFleetHealthSummary`, and `StreamAlarms`.
- **FR-TEL-07:** Implement backpressure with bounded worker channels and forward poison/unparseable messages to a dead-letter subject (`telemetry.dlq`).

### 4.5 Workflow Service (`services/workflow-service`)
- **FR-WF-01:** Orchestrate asynchronous jobs: `FirmwareUpgrade`, `DriftRemediation`, and `PowerCycle`.
- **FR-WF-02:** Job Entity Lifecycle: `queued` -> `running` -> `succeeded` / `failed` / `cancelled`.
- **FR-WF-03:** Generic Binary Min-Heap Priority Scheduler (`pkg/ds/minheap`):
  - Order by priority (High: 1, Normal: 2, Low: 3), tie-broken by `created_at` (FIFO).
  - Enforce concurrency caps per tenant and across the entire platform.
- **FR-WF-04:** Idempotency Protection:
  - Require an `idempotency_key` on job submission.
  - If a job with the same tenant and idempotency key exists, return the existing job.
- **FR-WF-05:** Rolling Batch Upgrades:
  - Process targets in batches of size $B$.
  - Abort workflow if failure percentage exceeds `max_failure_percent` (e.g., 20%).
  - Support manual pause, resume, and cancellation.
- **FR-WF-06:** Failure Handling & Retries:
  - Exponential backoff with full jitter on transient step failures.
  - Route permanently failed jobs to Dead Letter Queue (NATS subject and AWS SQS DLQ via LocalStack).
- **FR-WF-07:** Leader Election & Crash Recovery:
  - Redis-based lease lock for leader election to coordinate background sweeps.
  - Automatic reaper for jobs stuck in `running` status beyond timeout threshold upon instance recovery.
- **FR-WF-08:** AWS S3 Integration (LocalStack):
  - Store and retrieve firmware bundle manifests and artifacts.

---

## 5. Non-Functional Requirements

- **NFR-PERF-01 (Throughput):** Telemetry ingestion pipeline must sustain at least 2,000 telemetry messages/second on standard development hardware.
- **NFR-PERF-02 (Latency):** Public REST API P95 latency must remain under 150ms for cached/indexed read requests under normal load.
- **NFR-SCAL-01 (Fleet Scale):** System architecture must support up to 10,000 concurrently connected simulated servers.
- **NFR-AVAIL-01 (High Availability):** All core services must be stateless with horizontal replica support; single pod termination must not cause request dropped errors.
- **NFR-SEC-01 (Tenant Isolation):** Zero cross-tenant data leakage. Every data query must enforce tenant filtering at the database layer.
- **NFR-SEC-02 (Authentication & Passwords):** User passwords hashed using bcrypt (cost factor 12). JWTs verified with strict algorithm and signature validation.
- **NFR-TEST-01 (Coverage):** Backend Go code must maintain $\ge 80\%$ test statement coverage across all `internal/` packages, verified with `go test -race`.

---

## 6. Data Models & Schemas

### 6.1 PostgreSQL Relational Schema

```sql
-- Organizations (Tenants)
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Devices
CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    serial VARCHAR(64) NOT NULL UNIQUE,
    model VARCHAR(64) NOT NULL,
    firmware_version VARCHAR(32) NOT NULL,
    power_state VARCHAR(16) NOT NULL DEFAULT 'on',
    health_status VARCHAR(16) NOT NULL DEFAULT 'ok' CHECK (health_status IN ('ok', 'warning', 'critical')),
    connection_state VARCHAR(16) NOT NULL DEFAULT 'connected' CHECK (connection_state IN ('connected', 'disconnected')),
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INT NOT NULL DEFAULT 1,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_devices_org_created ON devices(org_id, created_at DESC, id DESC);
CREATE INDEX idx_devices_org_model ON devices(org_id, model);
CREATE INDEX idx_devices_org_health ON devices(org_id, health_status);
CREATE INDEX idx_devices_tags ON devices USING GIN (tags);

-- Device Claims
CREATE TABLE device_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    claim_code VARCHAR(64) NOT NULL UNIQUE,
    serial VARCHAR(64),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'claimed', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ
);
CREATE INDEX idx_device_claims_code ON device_claims(claim_code) WHERE status = 'pending';

-- Server Profiles
CREATE TABLE server_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    target_firmware VARCHAR(32) NOT NULL,
    power_policy VARCHAR(32) NOT NULL DEFAULT 'performance',
    boot_order JSONB NOT NULL DEFAULT '["nvme", "pxe"]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, name)
);

-- Server Profile Device Assignments
CREATE TABLE device_profile_assignments (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES server_profiles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Compliance Results
CREATE TABLE compliance_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES server_profiles(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL CHECK (status IN ('compliant', 'drifted')),
    drift_details JSONB NOT NULL DEFAULT '[]'::jsonb,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_compliance_device ON compliance_results(device_id, evaluated_at DESC);

-- Alarms
CREATE TABLE alarms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rule_name VARCHAR(128) NOT NULL,
    severity VARCHAR(16) NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    status VARCHAR(16) NOT NULL DEFAULT 'raised' CHECK (status IN ('raised', 'acknowledged', 'cleared')),
    message TEXT NOT NULL,
    metric_name VARCHAR(64),
    current_value DOUBLE PRECISION,
    threshold_value DOUBLE PRECISION,
    raised_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID REFERENCES users(id),
    cleared_at TIMESTAMPTZ
);
CREATE INDEX idx_alarms_org_status ON alarms(org_id, status, raised_at DESC);
CREATE INDEX idx_alarms_device_rule ON alarms(device_id, rule_name) WHERE status != 'cleared';

-- Telemetry Rollups (Partitioned or Indexed by Time)
CREATE TABLE telemetry_rollups (
    id BIGSERIAL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    bucket_interval VARCHAR(8) NOT NULL CHECK (bucket_interval IN ('1m', '5m')),
    window_start TIMESTAMPTZ NOT NULL,
    avg_inlet_temp DOUBLE PRECISION NOT NULL,
    max_inlet_temp DOUBLE PRECISION NOT NULL,
    avg_cpu_temp DOUBLE PRECISION NOT NULL,
    max_cpu_temp DOUBLE PRECISION NOT NULL,
    avg_power_watts DOUBLE PRECISION NOT NULL,
    max_power_watts DOUBLE PRECISION NOT NULL,
    avg_fan_rpm DOUBLE PRECISION NOT NULL,
    min_fan_rpm DOUBLE PRECISION NOT NULL,
    sample_count INT NOT NULL,
    PRIMARY KEY (id, window_start)
);
CREATE INDEX idx_telemetry_rollups_query ON telemetry_rollups(device_id, bucket_interval, window_start DESC);

-- Workflow Jobs
CREATE TABLE workflow_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL CHECK (type IN ('firmware_upgrade', 'drift_remediation', 'power_cycle')),
    priority INT NOT NULL DEFAULT 2, -- 1=High, 2=Normal, 3=Low
    state VARCHAR(16) NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    targets JSONB NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key VARCHAR(128) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    step_history JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    UNIQUE(org_id, idempotency_key)
);
CREATE INDEX idx_workflow_jobs_state ON workflow_jobs(state, priority, created_at ASC);
```

### 6.2 Redis Key Schema
- `ratelimit:{org_id}:{window_epoch}`: Token count string for rate limiting.
- `telemetry:ring:{device_id}`: Sorted set (ZSET) or capped list storing the last 120 raw telemetry samples (score = timestamp unix milliseconds).
- `alarms:active:{device_id}`: Hash of active alarm rule keys.
- `leader:lease:workflow_sweeper`: String key with 10s TTL holding leader instance identity.
- `cache:device:{device_id}`: Cached JSON representation of device details.

### 6.3 NATS JetStream Subjects
- `telemetry.<tenant_id>.<serial>`: Device telemetry streams.
- `inventory.device.claimed`: Emitted when a device completes claim handshake.
- `inventory.drift.detected`: Emitted when a compliance check detects configuration drift.
- `workflow.job.queued`: Emitted when a new workflow job is ready for execution.
- `workflow.job.events`: Job progress updates and state transitions.
- `telemetry.dlq`: Dead-letter queue for unparseable or rejected telemetry messages.
- `workflow.dlq`: Dead-letter queue for exhausted workflow retries.

---

## 7. API Contracts

### 7.1 Public REST API (Gateway)

| Method | Path | Auth / Role | Description |
|---|---|---|---|
| `POST` | `/api/v1/auth/login` | Public | Authenticate user credentials, return JWT access + refresh tokens. |
| `POST` | `/api/v1/auth/refresh` | Public | Refresh expired access token with valid refresh token. |
| `GET` | `/api/v1/devices` | Viewer+ | Keyset-paginated list of devices with query filters. |
| `GET` | `/api/v1/devices/{id}` | Viewer+ | Retrieve single device details, profile, and compliance status. |
| `POST` | `/api/v1/devices/claims` | Admin | Generate a single-use claim code. |
| `GET` | `/api/v1/devices/{id}/telemetry`| Viewer+ | Retrieve recent telemetry samples and aggregated rollups. |
| `GET` | `/api/v1/alarms` | Viewer+ | List alarms filtered by device, severity, status. |
| `POST` | `/api/v1/alarms/{id}/acknowledge` | Operator+ | Acknowledge an active alarm. |
| `GET` | `/api/v1/profiles` | Viewer+ | List server configuration profiles. |
| `POST` | `/api/v1/profiles` | Admin | Create a new server configuration profile. |
| `POST` | `/api/v1/profiles/{id}/assign` | Admin | Assign a profile to device(s). |
| `GET` | `/api/v1/workflows` | Viewer+ | List workflow jobs with state and pagination. |
| `POST` | `/api/v1/workflows` | Operator+ | Submit a new workflow job (idempotent). |
| `GET` | `/api/v1/workflows/{id}` | Viewer+ | Get workflow status and step history. |
| `POST` | `/api/v1/workflows/{id}/cancel` | Operator+ | Cancel a queued or running workflow. |
| `GET` | `/api/v1/fleet/summary` | Viewer+ | Overall fleet health, count by status, anomalous devices. |

### 7.2 Internal gRPC Services
- `InventoryService`: `GetDevice`, `ListDevices`, `CreateClaimCode`, `ClaimDevice`, `EvaluateCompliance`, `GetServerProfile`.
- `TelemetryService`: `GetDeviceTelemetry`, `ListAlarms`, `AcknowledgeAlarm`, `GetFleetSummary`, `StreamAlarms`.
- `WorkflowService`: `CreateJob`, `GetJob`, `CancelJob`, `ListJobs`.

---

## 8. Acceptance Criteria Matrix

| ID | Category | Requirement / Scenario | Verification Criteria |
|---|---|---|---|
| **AC-01** | Architecture | Project layout adheres strictly to clean architecture and monorepo structure. | `cmd/`, `internal/`, `pkg/`, `services/`, `specs/` present and strictly separated. |
| **AC-02** | DSA | `pkg/ds/consistenthash` implements virtual nodes, distribution testing, and benchmark. | Distribution variance $\le 15\%$, benchmark included. |
| **AC-03** | DSA | `pkg/ds/minheap` generic binary priority queue implementation. | $O(\log N)$ push/pop verified with unit tests and benchmark. |
| **AC-04** | DSA | `pkg/ds/tokenbucket` rate limiter with atomic refill logic and clock injection. | Unit test verifying replenishment and burst allowances. |
| **AC-05** | DSA | `pkg/ds/lru` thread-safe LRU cache with eviction logic. | Unit test verifying $O(1)$ get/put, hit/miss tracking, and evictions. |
| **AC-06** | Simulator | Spawns $N$ simulated servers with random-walk telemetry. | Verified via Redfish REST endpoints and NATS telemetry emissions. |
| **AC-07** | Simulator | Fault injection API triggers predictable anomalies. | Calling `POST /sim/faults` immediately affects sensor output. |
| **AC-08** | Gateway | JWT authentication with role enforcement (`admin`, `operator`, `viewer`). | Unauthorized requests return `401`; forbidden routes return `403`. |
| **AC-09** | Gateway | Strict tenant isolation. | Requests for tenant A cannot retrieve or mutate records of tenant B. |
| **AC-10** | Gateway | Token bucket rate limiting returns HTTP 429 when quota exceeded. | Exceeding burst rate yields `429` with valid `Retry-After`. |
| **AC-11** | Inventory | Single-use device claim flow binds device to organization. | Re-submitting an already used claim code returns error. |
| **AC-12** | Inventory | Keyset pagination returns deterministic ordered pages. | Fast response time without offset performance penalty. |
| **AC-13** | Inventory | Compliance engine identifies firmware and config drift. | Deviations from profile produce `drifted` status with diff details. |
| **AC-14** | Telemetry | Ingestion of streaming telemetry over NATS JetStream into Redis ring buffer. | Telemetry samples queryable within 500ms of publication. |
| **AC-15** | Telemetry | Stateful alarm engine triggers alarms on consecutive violations. | Alarm raised after $N$ violations; deduplicated while active. |
| **AC-16** | Telemetry | Pure Go unsupervised anomaly detection via EWMA and z-score. | Synthetic anomaly spikes reliably generate `AnomalyDetected` alarm. |
| **AC-17** | Workflow | Priority scheduler dispatches High priority jobs ahead of Normal. | High priority job queued after Normal job is executed first. |
| **AC-18** | Workflow | Idempotent job submission prevents duplicate executions. | Submitting same `idempotency_key` returns existing job record. |
| **AC-19** | Workflow | Crash recovery re-queues orphaned running jobs upon restart. | Jobs left in `running` state are reconciled upon startup. |
| **AC-20** | Code Quality | Test statement coverage $\ge 80\%$ on all `internal/` packages. | `go test -race -cover` confirms $\ge 80\%$ statement coverage. |
| **AC-21** | Operations | Multi-container Docker Compose runs the complete system. | `make up` boots Postgres, Redis, NATS, LocalStack, and all services cleanly. |
| **AC-22** | Dashboard | React + TypeScript web dashboard displays real-time health and charts. | Demo mode works standalone on GitHub Pages with simulated data. |
