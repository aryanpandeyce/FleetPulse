# FleetPulse Engineering Constitution

**Status:** Ratified  
**Applies to:** All FleetPulse backend services, libraries, and tools.

This constitution defines the non-negotiable engineering standards and operational principles for the FleetPulse platform. All contributors and automated agents must adhere to these rules without exception.

---

## 1. Language & Dependencies

- **Go 1.22+**: All backend services must be written in Go 1.22 or newer.
- **Standard Library First**: Prefer the standard library (`net/http`, `log/slog`, `context`, `sync`, `time`, `crypto`) over external dependencies.
- **Minimal External Dependencies**: Every external dependency requires technical justification in `plan.md`. Approved foundational third-party packages:
  - Routing: `github.com/go-chi/chi/v5`
  - Database: `github.com/jackc/pgx/v5`
  - Redis: `github.com/redis/go-redis/v9`
  - Messaging: `github.com/nats-io/nats.go`
  - Protobuf/gRPC: `google.golang.org/grpc`, `google.golang.org/protobuf`
  - Metrics: `github.com/prometheus/client_golang`
  - Password hashing: `golang.org/x/crypto/bcrypt`
  - JWT: `github.com/golang-jwt/jwt/v5`
  - AWS SDK: `github.com/aws/aws-sdk-go-v2` (for S3 and SQS with endpoint overrides)

## 2. Architecture & Code Structure

- **Clean Layered Architecture**: Every microservice under `services/<service-name>` must follow strict separation:
  - `cmd/<service-name>/`: Main entrypoint, dependency wiring, configuration loading, process lifecycle.
  - `internal/domain/`: Pure domain entities, business logic, error types, repository interfaces. Zero external dependencies.
  - `internal/repo/`: Data persistence (PostgreSQL with `pgx`, Redis), implementing domain repository interfaces.
  - `internal/service/`: Application use-case coordinators, workflow executors, and business rule orchestration.
  - `internal/api/`: Inbound transport adapters (HTTP handlers, gRPC servers, NATS message consumers).
- **Dependency Injection**: Dependencies must be injected explicitly via constructors (`New...`). Never access global singletons or global state.
- **Package Encapsulation**: Keep internal packages within `internal/` to prevent improper cross-service imports. Shared utilities belong strictly in `pkg/`.

## 3. Code Quality & Error Handling

- **Documentation**: Every exported package, struct, interface, function, and constant must have a descriptive doc comment following Go conventions.
- **Explicit Error Wrapping**: Never return bare errors. Wrap errors with contextual information using `fmt.Errorf("operation failed: %w", err)` to maintain clean error inspection chains.
- **No Panics in Request Paths**: Panics are strictly prohibited in request or message handling pipelines. The gateway and HTTP servers must include panic recovery middleware that logs stack traces structured via `slog` and returns HTTP 500.

## 4. Concurrency & Context Propagation

- **Context Everywhere**: `context.Context` must be the first parameter of every blocking, I/O, or asynchronous function call.
- **Strict Outbound Timeouts**: Every database query, Redis command, gRPC invocation, and HTTP request must be bounded by a context timeout.
- **Goroutine Hygiene**: Every spawned goroutine must have a defined lifetime, termination condition, and recover mechanism. Never leak background goroutines.

## 5. Observability & Logging

- **Structured Logging**: Use `log/slog` producing JSON output.
- **Correlation Context**: Every log record in request handling must include `request_id`, `tenant_id` (or `org_id`), and service name.
- **Standardized Metrics**: RED (Rate, Errors, Duration) metrics must be exposed on `/metrics` for all HTTP and gRPC services using Prometheus collectors.

## 6. Process Lifecycle & Graceful Shutdown

- **Signal Trapping**: All services must trap `SIGINT` and `SIGTERM`.
- **Orderly Teardown**: Upon shutdown signal:
  1. Fail readiness probes (`/readyz`) immediately.
  2. Stop accepting new inbound requests/messages.
  3. Drain active HTTP/gRPC requests within a grace period (default 15s).
  4. Flush in-flight database/Redis writes and message acknowledgments.
  5. Close client connections and exit with code 0.

## 7. Configuration & Twelve-Factor Compliance

- **Environment-Driven Configuration**: All settings must be configurable via environment variables with sensible local defaults.
- **No Hardcoded Secrets**: Credentials, JWT signing keys, and connection strings must never be committed to source control.

## 8. Testing & Coverage Discipline

- **Test Pyramid**:
  - Table-driven unit tests for all domain logic, algorithms (`pkg/ds`), and service layers.
  - Mocked integration tests for repository and API layers.
  - End-to-end integration tests using docker-compose.
- **Race Detection**: All tests must pass with `go test -race ./...`.
- **Enforced Coverage Gate**: Minimum **80% statement coverage** across all `internal/` packages, validated in CI.

## 9. Idempotency & Fault Tolerance

- **Idempotent State Operations**: All retriable mutations (job creation, device claims, alarm state transitions) must accept an idempotency key or guarantee idempotent state transitions.
- **Backpressure & Boundedness**: All worker queues, in-memory buffers, and channel pipelines must be bounded. Document drop or shed policies.

## 10. The Honesty Rule

- **Verifiable Benchmarks**: Benchmark metrics, throughput rates, and latency numbers reported in `README.md` or verification reports must originate from actual reproducible runs on documented hardware.
- **No Fake Functionality**: Never stub or fake functionality to simulate completeness. Unimplemented or degraded capabilities must be explicitly documented under "Known Limitations".
