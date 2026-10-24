# Aegis Service & Dependency Builder

The **Service & Dependency Builder** allows logged-in operators and administrators to dynamically define, observe, and connect microservices and their dependencies directly from the Aegis control plane.

---

## 1. Core Principles

- **Manual Graph as Source of Truth**: User-configured topology and accepted dependencies are definitive. Trace auto-discovery never overwrites manual edges.
- **Trace Discovery as Suggestions**: Distributed trace discovery proposes *suggested* edges. Operators accept or reject suggestions. Rejected suggestions permanently stick and are never re-created unless explicitly reset.
- **Telemetry-Gated Health**: Services without a valid telemetry source are categorized as **Unconnected** (`unconnected`) and cannot be predicted on.
- **Recovery Binding as Action Boundary**: Services without a valid recovery binding are strictly **Observe-Only**. Autonomous or manual remediation actions cannot execute against unbound services.
- **Zero Credential Exposure**: Sensitive telemetry tokens and passwords are encrypted at rest with AES-256-GCM and redacted from all API reads, exports, and audit logs.
- **Idempotent Versioning**: Every creation, mutation, and deletion updates the active topology version, maintaining a full history for instant rollback.

---

## 2. Walkthrough: Defining & Connecting Services

### Step 1: Add a Service to the Catalog
1. Navigate to **Services** in the sidebar.
2. Click **+ Add Service** (requires Operator or Admin role).
3. **Basics**:
   - `id`: Kebab-case identifier (e.g., `checkout-service`).
   - `name`: Display name (e.g., `Checkout Service`).
   - `namespace`: Logical deployment namespace (e.g., `prod`).
   - `tier`: `tier-1` (mission critical), `tier-2` (standard), or `tier-3` (background).
   - `owner`: Team or email identifier.
   - `description` and `tags`.

### Step 2: Configure Telemetry Ingestion
Choose one of three ingestion engines:
- **Prometheus**: Provide base URL (`http://prometheus:9090`), job/selector, and optional PromQL overrides for CPU, Memory, Latency (p95), Error Rate, and Queue Depth.
- **Kubernetes**: Provide cluster ID, namespace, workload kind (`Deployment`, `StatefulSet`, `DaemonSet`), and workload name.
- **Agent Push (OTel/Sidecar)**: System issues an API key reference and presents a copyable Docker/Helm deployment snippet.
- **Mandatory Connection Test**: Click **Test Connection** before saving. Aegis executes an SSRF-safe probe, validating metric availability, recent timestamps, and reachability.

### Step 3: Configure Recovery Binding (Optional)
- Set `runtime` to `kubernetes` or `docker`.
- Specify the target workload identifier (e.g., `prod/checkout-service`).
- Select permitted self-healing actions from:
  - `scale`: Replica scale-out.
  - `restart`: Pod/container restart.
  - `increase_pool`: Database connection pool expansion.
  - `flush_cache`: Cache eviction.
  - `reroute`: Traffic diversion.

### Step 4: Select Monitoring Mode
- **Observe**: Collects telemetry and visualizes health; no recommendations or execution (Default).
- **Recommend**: TGNN predicts root-causes and generates topologically-ordered recovery plans for operator approval.
- **Auto-Remediate** (*Admin Only*): Aegis autonomously dispatches approved low-risk recovery plans within blast radius budgets. Disabled if no recovery binding is configured.
- **Ignore**: Telemetry dropped at ingestion. Node remains in the graph as an unmonitored dependency for structural propagation.

### Step 5: Draw Dependencies in the Graph Builder
1. Navigate to **Dependency Graph** in the sidebar.
2. Drag connection lines between service nodes.
3. Select the dependency relationship: `sync` (HTTP/gRPC), `async` (event queue), `db` (database access), or `cache` (in-memory cache).
4. Click **Auto Layout** to organize nodes into hierarchical DAG levels based on topological in-degree and SCC condensation.

---

## 3. Masked-Dependency Accuracy Trade-off

When a dependent service is set to `ignore` mode or lacks telemetry, Aegis marks it as an **Unmonitored Dependency** (rendered with hatched styling).

### Design Trade-Off Analysis
- **Structural Integrity**: The node and its directed edges are retained in the Graph Representation. Algorithms like Tarjan's Strongly Connected Components (SCC) and DAG condensation continue to operate on the true system topology.
- **Cascade Tracking**: If an upstream service degrades, propagation prediction traces the failure path through the unmonitored node to downstream services.
- **Imputation vs. Real Features**: Because live metric vectors (CPU, memory, latency) are unavailable for unmonitored nodes, feature embeddings are set to neutral baseline tensors. While topological propagation accuracy remains high, fine-grained root-cause confidence for that specific unmonitored node is attenuated. Operators should prioritize connecting telemetry sources to critical path nodes.

---

## 4. Security & Cryptographic Architecture

### SSRF Protection (`ssrf.go`)
External telemetry URLs (e.g., Prometheus base URLs) undergo rigorous validation:
- **Blocked IP Ranges**: RFC1918 private IPv4 (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), loopback (`127.0.0.0/8`, `::1`), link-local (`169.254.0.0/16`), and IPv6 unique local (`fc00::/7`).
- **Cloud Metadata Shielding**: Blocks `169.254.169.254`, `metadata.google.internal`, `.local`, `.internal`, and `.localhost`.
- **Enforced Caps**: 5-second connection timeouts, 1MB response size limits, and safe redirect validation.
- **Admin Override**: In private test environments, internal addresses can be permitted by explicitly setting `AEGIS_ALLOW_INTERNAL_METRICS=true`.

### Credential Encryption (`crypto.go`)
- Sensitive tokens and passwords are encrypted using **AES-256-GCM** with a key derived from `AEGIS_ENCRYPTION_KEY`.
- Ciphertexts are stored with an `enc::` prefix.
- All API responses (`GET /api/registry/services`, `GET /api/registry/export`) redact credentials (`hasAuth: true`, tokens/passwords omitted).

### RBAC Matrix
| Capability | Viewer | Operator | Admin |
| :--- | :---: | :---: | :---: |
| View services, graph, versions, validation | Yes | Yes | Yes |
| Export topology (YAML/JSON/CSV) | Yes | Yes | Yes |
| Create/Edit/Delete services & edges | No | Yes | Yes |
| Accept / Reject discovery suggestions | No | Yes | Yes |
| Set mode to `observe`, `recommend`, `ignore` | No | Yes | Yes |
| Set mode to `auto_remediate` | No | No | Yes |
| Configure `recovery_binding` | No | No | Yes |
| Configure telemetry credentials | No | No | Yes |

---

## 5. Control Plane API Reference

### Service Management
- `GET /api/registry/services`: List services with filtering by namespace, cluster, tier.
- `GET /api/registry/services/{id}`: Get service details (credentials redacted).
- `POST /api/registry/services`: Create service (cannot create directly with `auto_remediate`).
- `PUT /api/registry/services/{id}`: Update service attributes and mode.
- `DELETE /api/registry/services/{id}`: Delete service and associated edges.
- `POST /api/registry/services/{id}/test-connection`: Validate telemetry source connectivity.

### Edge & Suggestion Management
- `GET /api/registry/edges`: List active topology edges.
- `POST /api/registry/edges`: Create manual dependency edge (`sync`, `async`, `db`, `cache`).
- `DELETE /api/registry/edges/{source}/{target}`: Remove edge.
- `GET /api/registry/suggestions`: List trace auto-discovered suggestions.
- `POST /api/registry/suggestions/{id}/accept`: Promote suggested edge to active topology.
- `POST /api/registry/suggestions/{id}/reject`: Permanently reject suggestion.

### Validation & Bulk Operations
- `POST /api/registry/validate`: Graph audit detecting duplicate IDs, self-loops, orphan nodes, SCC cycle warnings, unmonitored dependencies, and missing bindings.
- `POST /api/registry/import?dryRun=true`: Preview additions, modifications, and deletions without applying.
- `POST /api/registry/import`: Apply topology changes and increment version.
- `GET /api/registry/export?format=yaml|json|csv`: Export graph with credentials redacted.
- `GET /api/registry/versions`: List topology snapshot history.
- `POST /api/registry/rollback`: Rollback control plane to target version.
