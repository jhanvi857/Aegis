# Dynamic Topology & Real Telemetry Architecture (Phase 3)

## Overview
Aegis Phase 3 elevates the platform from fixed-topology demonstration models into a multi-cluster, multi-tenant production engine capable of monitoring **any number (N) of cloud-hosted services**.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Target Infrastructure                                 │
│  [Kubernetes / Nomad / Bare-Metal]                                          │
│                                                                             │
│  ┌─────────────────────────┐         ┌─────────────────────────┐            │
│  │  Aegis Telemetry Agent  │         │  OpenTelemetry Services │            │
│  │  (DaemonSet/Deployment) │         │  (Trace Exporters)      │            │
│  └────────────┬────────────┘         └────────────┬────────────┘            │
│               │ (Outbound Only)                   │                         │
│               │ X-API-Key: aegis_ak_*             │ /api/telemetry/spans    │
│               ▼                                   ▼                         │
│  ┌─────────────────────────────────────────────────────────────┐            │
│  │                    Aegis API Gateway                        │            │
│  │             (Multi-replica Control Plane)                   │            │
│  └──────────────┬───────────────────────────────┬──────────────┘            │
│                 │                               │                           │
│                 ▼                               ▼                           │
│  ┌─────────────────────────────┐  ┌────────────────────────────┐            │
│  │ PostgreSQL Service Registry │  │ Kafka Partitioned Streams  │            │
│  │ - topology_services         │  │ Key: cluster_id/service_id │            │
│  │ - topology_edges (override) │  │                            │            │
│  │ - topology_versions         │  └─────────────┬──────────────┘            │
│  └─────────────────────────────┘                │                           │
│                                                 ▼                           │
│                                   ┌────────────────────────────┐            │
│                                   │ Python ML / TGNN Pipeline  │            │
│                                   └────────────────────────────┘            │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 1. PostgreSQL Service Registry Schema

Dynamic topologies are persisted in PostgreSQL across three core tables, auto-migrated on startup:

### `topology_services`
Stores metadata, ownership, service tiers, and operational capabilities:
- `id` (VARCHAR(128), PK): Service identifier (e.g. `order-service`, `gateway`).
- `name` (VARCHAR(255)): Human-readable service label.
- `type` (VARCHAR(64)): Service category (`gateway`, `service`, `database`, `cache`, `queue`).
- `namespace` (VARCHAR(128)): Kubernetes or deployment namespace (e.g. `production`, `payments`).
- `cluster_id` (VARCHAR(128)): Multi-cluster identifier (e.g. `us-east-1-prod`).
- `tier` (VARCHAR(64)): Criticality tier (`tier-0`, `tier-1`, `tier-2`, `tier-3`).
- `capabilities` (VARCHAR(255)): Supported self-healing actions (e.g. `scale,restart,reroute`).
- `owner` (VARCHAR(255)): Responsible engineering squad or contact.
- `version`, `replicas`, `max_replicas`, `status`, `cpu`, `memory`, `latency`, `rps`, `error_rate`, `queue_depth`.

### `topology_edges`
Stores directed dependencies with confidence metrics and override protection:
- `source_service_id`, `target_service_id`, `namespace`, `cluster_id` (Unique composite key).
- `relationship` (VARCHAR(64)): `http_client`, `grpc_client`, `mq_producer`, `cache_lookup`, `db_query`.
- `confidence` (FLOAT): Auto-discovery confidence score ($[0.0, 1.0]$).
- `is_manual_override` (BOOLEAN): When `true`, auto-discovery is prohibited from degrading confidence or changing relationship type.
- `calls_per_sec`, `latency_ms`, `error_rate`, `sample_count`.

### `topology_versions`
Audit snapshots capturing complete point-in-time topology revisions:
- `id` (VARCHAR(128), PK): Unique version ID (e.g. `ver-1740000000`).
- `version` (INT): Monotonic revision number.
- `description` (TEXT): Rationale or deployment trigger.
- `snapshot` (JSONB): Complete serialization of all services and edges at that version.
- `created_by` (VARCHAR(255)): Operator/admin email or automated process.

---

## 2. OpenTelemetry Trace Span Auto-Discovery & Manual Overrides

The Gateway exposes `POST /api/telemetry/spans` allowing OpenTelemetry collectors and tracer agents to submit trace spans:

### Confidence Scoring Algorithm
When a span connects `sourceServiceId` and `targetServiceId`:
1. If the edge does not exist, it is created with an initial baseline confidence:
   $$\text{Confidence}_0 = 0.75$$
2. As subsequent spans are ingested, the sample count $N$ increments, and confidence asymptotically approaches $0.99$:
   $$\text{Confidence}_N = \min\left(0.99,\, 0.75 + \frac{N}{100} \times 0.24\right)$$
3. **Manual Override Protection**: If an operator explicitly creates or marks an edge with `isManualOverride = true`:
   - Confidence remains locked at $1.0$.
   - The operator's declared relationship (e.g. `grpc_client`) is never overwritten by discovered trace spans.

---

## 3. Outbound-Only Telemetry Agent & Packaging

The telemetry agent is designed for restrictive corporate enterprise environments where no inbound ports may be opened into production clusters:

- **Protocol**: Outbound-only HTTPS/gRPC to Aegis Gateway, or Kafka producer.
- **Authentication**: Authenticated via machine API keys (`X-API-Key: aegis_ak_*`) generated by admins in the API Gateway.
- **Tenant & Cluster Isolation**: Kafka partition keys are strictly formatted as:
  $$\text{PartitionKey} = \text{cluster\_id} + \text{"/"} + \text{service\_id}$$
  Ensuring message ordering per service while isolating multi-tenant data pipelines.
- **Packaging**:
  - `go-services/telemetry-agent/Dockerfile`: Multi-stage, unprivileged non-root image (`USER 10001:10001`, minimal Alpine base).
  - `deploy/helm/aegis-agent/`: Production-grade Helm chart with ConfigMaps, Secrets, resource quotas, and security contexts (`readOnlyRootFilesystem: true`, `drop: [ALL]`).

---

## 4. `DEMO_MODE` Gating vs Live Cluster Mode

Aegis enforces a strict separation between simulated demo jitter and live cluster monitoring:

- **Flag**: Controlled via environment variable `AEGIS_DEMO_MODE` / `DEMO_MODE` (defaults strictly to `false`), or dynamically toggled via `POST /api/system/mode`.
- **Behavior in Real Mode (`DemoMode == false`)**:
  - Random metric jitter (`math/rand`) is entirely disabled.
  - Telemetry reflects solely genuine probed microservice `/health` endpoints, Prometheus remote metrics, and OpenTelemetry spans.
  - UI displays an emerald status pill: `LIVE CLUSTER (cluster-primary)`.
- **Behavior in Demo Mode (`DemoMode == true`)**:
  - Controlled background jitter is enabled for offline conference demonstrations.
  - UI displays an amber warning pill: `DEMO SIMULATION`.

---

## 5. Multi-Replica State Synchronization

To run the API Gateway across multiple replicas behind an external load balancer:
1. All dynamic service CRUD operations and trace spans write through to the PostgreSQL store.
2. Background tickers in each gateway replica synchronize registered services and edge revisions from PostgreSQL on a 5-second interval.
3. TGNN and decision engines evaluate the dynamic system graph without requiring single-instance in-memory affinity.
