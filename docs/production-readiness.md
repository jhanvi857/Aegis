# Phase 7 — Production Readiness Specification

This document describes the operational hardening and production readiness architecture implemented in Aegis.

---

## 1. Centralized Configuration Management

All runtime parameters are consolidated into typed configuration structures (`config.Config`) loaded from environment variables and validated at startup before any network listener binds.

### Key Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `API_GATEWAY_PORT` | `8000` | HTTP/HTTPS control plane listener port |
| `AEGIS_ENV` | `development` | Environment mode (`development`, `staging`, `production`) |
| `AEGIS_LOG_LEVEL` | `info` | Minimum log level (`debug`, `info`, `warn`, `error`) |
| `DATABASE_URL` | `""` | PostgreSQL connection DSN for persistent storage |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis caching and state bus connection URI |
| `PYTHON_GRPC_ADDR` | `localhost:50051` | Downstream Python TGNN inference gRPC endpoint |
| `AEGIS_TLS_ENABLED` | `false` | Enables TLS termination on the HTTP listener |
| `AEGIS_TLS_CERT_FILE` | `""` | Path to TLS X.509 certificate file (required in prod TLS) |
| `AEGIS_TLS_KEY_FILE` | `""` | Path to TLS private key file (required in prod TLS) |
| `AEGIS_CORS_ALLOWED_ORIGINS` | `http://localhost:5173,...` | Comma-separated list of trusted origins |
| `AEGIS_SHUTDOWN_TIMEOUT_SEC`| `15` | Maximum seconds to drain in-flight requests |
| `AEGIS_AUTONOMOUS_RECOVERY` | `true` | Master switch for automated remediation execution |

### Strict Production Validation Rules
When `AEGIS_ENV=production`:
1. Wildcard CORS origin (`*`) is strictly forbidden and causes startup panic.
2. If `AEGIS_TLS_ENABLED=true`, missing `AEGIS_TLS_CERT_FILE` or `AEGIS_TLS_KEY_FILE` halts initialization.
3. Every configured origin must be a valid RFC 3986 URI.
4. Default bootstrap credentials (`admin@aegis.local` / `AdminPass#2026`) emit high-priority audit warnings.

---

## 2. Health & Readiness Probes

Aegis implements standard Kubernetes-compatible probe endpoints:

### Liveness Probe (`GET /healthz`)
- **Port:** Control plane HTTP/HTTPS listener
- **Response:** `200 OK`
- **Payload:** `{"status":"UP","service":"aegis-api-gateway"}`
- **Purpose:** Verifies that the Go runtime and HTTP server process are active.

### Readiness Probe (`GET /readyz`)
- **Port:** Control plane HTTP/HTTPS listener
- **Response:** `200 OK` if all critical dependencies are operational; `503 Service Unavailable` if unready.
- **Dependencies Evaluated Concurrently:**
  - `storage`: PostgreSQL or in-memory persistence layer operational.
  - `state_engine`: In-memory service mesh model active and loaded.
  - `ml_inference`: Python TGNN gRPC downstream reachable (degraded status allows rulebook fallback).

---

## 3. Structured JSON Logging

All access logs and system events are emitted in JSON format via `telemetry.StructuredLogger` with standardized fields:

```json
{
  "timestamp": "2026-10-22T13:30:00.123456789Z",
  "level": "INFO",
  "component": "http-access",
  "message": "GET /api/topology completed in 4.5ms",
  "traceId": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "callerId": "admin@aegis.local",
  "durationMs": 4.52,
  "httpStatus": 200,
  "fields": {
    "method": "GET",
    "path": "/api/topology",
    "remote_ip": "10.0.0.1"
  }
}
```

---

## 4. Native Prometheus Metrics (`GET /metrics`)

Aegis exports Prometheus metrics:

- `aegis_http_requests_total{method,path,status}`: Counter of all processed HTTP requests.
- `aegis_http_request_duration_seconds{method,path,le}`: Histogram of request processing latency.
- `aegis_remediations_total{action_type,status}`: Counter of dispatched recovery actions.
- `aegis_remediation_verifications_total{outcome}`: Counter of verification results (`VERIFIED`, `INEFFECTIVE`, `ROLLED_BACK`).
- `aegis_monitored_services_total{status}`: Current gauge count of monitored microservices by status (`healthy`, `degraded`, `critical`).
- `aegis_active_dispatches`: Gauge of currently executing recovery operations under safety limits.

---

## 5. Graceful Shutdown

On receiving `SIGINT` or `SIGTERM`:
1. Logs signal reception and initiates shutdown timeout context (`AEGIS_SHUTDOWN_TIMEOUT_SEC`).
2. Stops accepting new incoming connections.
3. Allows in-flight HTTP requests and remediation verifications to conclude cleanly.
4. Closes database connection pools and gRPC channels.

---

## 6. High-Scale Benchmark (500 Services)

Verified via `scripts/load_test_500_services.go`:

| Metric | Measured Value | SLA Target | Result |
|---|---|---|---|
| Ingestion Time (500 nodes, 1,680 edges) | 567 µs | < 50 ms | PASS |
| Memory Allocated (500 nodes) | 0.33 MB | < 50 MB | PASS |
| Tarjan SCC Condensation p99 | 1.28 ms | < 10 ms | PASS |
| Blast Radius Upstream BFS p99 | < 1 ms | < 5 ms | PASS |

---

## 7. Google Online Boutique Kubernetes Validation

A full Kubernetes deployment and fault injection runbook is located in `deploy/online-boutique/runbook.md`, verifying the full autonomous cycle on 11 production microservices:
1. Target microservices deployed in namespace `online-boutique`.
2. Outbound `aegis-agent` daemonset deployed via Helm chart `deploy/helm/aegis-agent`.
3. Chaos injection on `cartservice` and `redis-cart` triggers TGNN root-cause isolation.
4. Decision engine evaluates risk, executes automated restart/scale actions, and verifies resolution.
