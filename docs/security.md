# FleetPulse Security & Threat Model

## 1. Authentication & Credential Storage

### Bcrypt Password Hashing
- User passwords are never stored in plaintext. They are salted and hashed using **bcrypt** with a default work factor cost of 10 (`golang.org/x/crypto/bcrypt`).
- Timing-safe comparisons (`bcrypt.CompareHashAndPassword`) prevent timing attack side channels during authentication.

### JSON Web Tokens (JWT)
- **Algorithm Pinning**: The API Gateway strictly verifies HMAC-SHA256 (`HS256`) signatures using an in-process secret key (`JWT_SECRET`). Tokens signed with `none` or asymmetric public keys are rejected immediately.
- **Short-Lived Access Tokens**: Access tokens expire after 15 minutes.
- **Refresh Token Rotation**: Refresh tokens have a 7-day TTL and are rotated upon use.
- **Payload Claims**:
  - `sub`: Unique user UUID.
  - `org_id`: Tenant UUID strictly tied to the user's organization.
  - `role`: Role string (`admin`, `operator`, `viewer`).
  - `exp`, `iat`, `iss`: Standard RFC 7519 temporal bounds.

---

## 2. Multi-Tenant Isolation Architecture

FleetPulse enforces strict multi-tenant isolation at every layer:

```
[REST Request + Bearer JWT]
          │
          ▼
┌─────────────────────────┐
│       API Gateway       │  Validates JWT & Extracts claims.org_id
└───────────┬─────────────┘
            │
            │ Inject gRPC Metadata: "x-org-id: <org_id>"
            ▼
┌─────────────────────────┐
│   Downstream Services   │  Extracts x-org-id via UnaryServerTenantInterceptor
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│   PostgreSQL / Memory   │  WHERE org_id = $1 (Strict scoping)
└─────────────────────────┘
```

1. **No Client Control over Tenant ID**: Clients cannot pass `org_id` in request query parameters or JSON bodies to access resources. The Gateway exclusively derives `org_id` from the cryptographically verified JWT claims.
2. **gRPC Context Propagation**: Outbound gRPC calls forward `x-org-id` metadata via `grpcx.UnaryClientTenantInterceptor`.
3. **Database Scoping**: All relational tables enforce foreign keys to `organizations(id)` and queries mandate `org_id = $1`. Keyset pagination indexes explicitly lead with `org_id` (`idx_devices_keyset ON devices(org_id, created_at DESC, id DESC)`).

---

## 3. Role-Based Access Control (RBAC)

FleetPulse implements a strict 3-tier hierarchical RBAC matrix:

| Action / Endpoint | Viewer | Operator | Admin |
|:---|:---:|:---:|:---:|
| `GET /api/v1/devices` (List Inventory) | ✅ | ✅ | ✅ |
| `GET /api/v1/devices/{id}` (Device Detail) | ✅ | ✅ | ✅ |
| `GET /api/v1/fleet/summary` (Fleet Overview) | ✅ | ✅ | ✅ |
| `GET /api/v1/alarms` (List Alarms) | ✅ | ✅ | ✅ |
| `POST /api/v1/alarms/{id}/acknowledge` | ❌ | ✅ | ✅ |
| `POST /api/v1/workflows` (Create Job) | ❌ | ✅ | ✅ |
| `POST /api/v1/workflows/{id}/cancel` | ❌ | ✅ | ✅ |
| `POST /api/v1/devices/claims` (Create Claim) | ❌ | ❌ | ✅ |

Unauthorized calls are rejected with `403 Forbidden` (`FORBIDDEN_ROLE`).

---

## 4. Rate Limiting & Denial of Service (DoS) Protection

- **Distributed Token Bucket**: Powered by atomic Redis Lua scripts (`pkg/redis` + `services/gateway/internal/ratelimit`).
- **Granular Scoping**: Rate limits are enforced both per-tenant (`ratelimit:org:<org_id>`) and per-IP (`ratelimit:ip:<ip_address>`).
- **HTTP 429 Responses**: Requests exceeding burst capacity receive `HTTP 429 Too Many Requests` with RFC 6585 `Retry-After: <seconds>` headers.
- **Fail-Open In-Memory Fallback**: If Redis becomes temporarily unreachable, rate limiting gracefully degrades to local thread-safe token buckets (`pkg/ds/tokenbucket`).

---

## 5. Input Validation & SQL Injection Defenses

- All SQL interactions utilize parameterized queries (`$1, $2, ...`) via `jackc/pgx/v5`. No dynamic string concatenation is permitted in SQL paths.
- JSON request bodies have size limits and strict schema decoding.
- Unrecognized or malformed JSON payloads return `HTTP 400 Bad Request` with structured error details.
