# Aegis Security Architecture & Hardening Guide

## Overview

In Phase 1 hardening, Aegis replaces in-memory mock authentication and open endpoints with production-grade security:
1. **Server-Side Role-Based Access Control (RBAC)** across all API routes.
2. **Argon2id Password Hashing** with per-user salt and tuned cryptographic parameters.
3. **Durable Persistence in PostgreSQL** with automated schema migration.
4. **Session Management** with TTL expiration, logout revocation, and rate-limited brute-force lockout.
5. **Scoped Machine API Keys** for agents and orchestrators.
6. **Append-Only Audit Logging** for all system mutations and authorizations.

---

## 1. RBAC Hierarchy & Permissions Matrix

Aegis implements a 3-tier hierarchical RBAC model:

| Role | Hierarchy Level | Capabilities | Allowed Routes |
|---|---|---|---|
| **Viewer** | 1 | Read-only inspection of topology, live metrics, predictions, active alerts, and recovery plans. | `GET /api/topology*`, `GET /api/telemetry/*`, `GET /api/predict*`, `GET /api/recovery/*`, `GET /api/chaos/active`, `GET /api/experiments`, `GET /api/auth/me`, `POST /api/auth/logout` |
| **Operator** | 2 | All Viewer permissions + ability to switch mesh topologies, approve/reject recovery plans, execute low-to-medium risk recovery actions, and view audit trails. | All Viewer routes + `POST /api/topology/select`, `POST /api/recovery/approve`, `POST /api/recovery/reject`, `POST /api/recovery/execute` (low/med risk), `GET /api/audit-logs` |
| **Admin** | 3 | All Operator permissions + full chaos fault injection, process termination, high-risk recovery action execution, user role elevation/demotion, and machine API key generation/revocation. | All routes + `POST /api/chaos/inject`, `POST /api/chaos/stop/*`, `POST /api/recovery/execute` (high risk), `GET/PUT /api/users*`, `GET/POST/DELETE /api/keys*`, `POST /api/auth/revoke` |

### Server-Enforced Registration Rule
- **Role is NEVER taken from client signup input.**
- The initial registered user (or matching `AEGIS_BOOTSTRAP_ADMIN_EMAIL`) is granted `admin`.
- All subsequent registrations default to `viewer`.
- Roles can only be changed by an active `admin` via `PUT /api/users/{id}/role`.

---

## 2. Password Hashing Specification (Argon2id)

In accordance with RFC 9106, passwords are protected using the memory-hard `argon2id` algorithm:
- **Algorithm:** Argon2id (hybrid data-dependent / data-independent memory access)
- **Memory (m):** 64 MB (65,536 KiB)
- **Iterations (t):** 1 pass
- **Parallelism (p):** 2 threads
- **Salt Length:** 16 cryptographically secure random bytes (CSPRNG)
- **Key Length:** 32 bytes (256 bits)
- **Comparison:** Constant-time (`subtle.ConstantTimeCompare`) to prevent timing side-channel attacks.

---

## 3. Account Lockout & Session Security

- **Login Lockout Policy:** 5 consecutive failed login attempts lock the account for 15 minutes. HTTP `429 Too Many Requests` is returned with remaining duration.
- **Session Revocation:** Sessions are stored in the `sessions` table. Calling `POST /api/auth/logout` or `POST /api/auth/revoke` records `revoked_at`, immediately invalidating the bearer token across all gateway instances.

---

## 4. Machine API Keys

Automated agents (e.g. `telemetry-agent`, CI pipelines, external orchestrators) authenticate using machine API keys:
- **Prefix:** `aegis_ak_<prefix>`
- **Storage:** Key hashes (`SHA-256`) are stored in PostgreSQL; raw keys are revealed **only once** upon generation.
- **Header:** Sent as `X-API-Key: aegis_ak_...` or `Authorization: Bearer aegis_ak_...`.
- **Granular Scopes:** e.g. `telemetry:write`, `chaos:inject`, `recovery:execute`, or `*`.

---

## 5. Append-Only Audit Logging

All system mutations are permanently recorded in PostgreSQL (`audit_logs` table):
- **Fields Captured:** `timestamp`, `actor_id`, `actor_email`, `actor_role`, `action`, `target_service_id`, `risk_level`, `approval_status`, `payload`, `result_status`, `error_message`, `ip_address`.
- **Queryable via:** `GET /api/audit-logs?limit=50` (Operator & Admin).
