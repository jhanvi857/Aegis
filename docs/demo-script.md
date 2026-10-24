# Aegis Live Demo Walkthrough & Operator Runbook

This document provides a step-by-step presentation walkthrough for demonstrating the Aegis Self-Healing Distributed Systems Platform end-to-end.

---

## 1. Architecture & Port Mapping Reference

| Service | Port | Description |
|---|:---:|---|
| **Frontend Observability Console (React + Vite)** | `:5173` | Unified Mission Control dashboard, dynamic topology graph, chaos controls, prediction telemetry, and approval gate. |
| **Aegis Control Plane (`api-gateway`)** | `:8000` | Orchestrates telemetry, dynamic topology registry, safety limits, autonomous loops, and recovery execution. |
| **Python ML Inference Server** | `:50051` | Unified TGNN inference engine serving failure prediction, root-cause localization, and propagation paths over gRPC. |
| **Go Recovery Engine** | `:8092` | Manages container/pod actions via orchestrator adapters (Docker / K8s) and executes the approval gate. |
| **Microservice Mesh (`playground`)** | `:8080`–`:8084` | Generic service mesh: `gateway` (:8080), `node-a` (:8081), `node-b` (:8082), `node-c` (:8083), `node-d` (:8084). |
| **PostgreSQL & Redis** | `:5432`, `:6379` | Durable store for dynamic topologies, execution history, users, sessions, API keys, and audit logs. |

### Operator Credentials
- **Administrator**: Configured via `AEGIS_BOOTSTRAP_ADMIN_EMAIL` and `AEGIS_BOOTSTRAP_ADMIN_PASSWORD` in `.env` (e.g. `admin@enterprise.io`, password must satisfy 12+ char complexity). On first login, forced password rotation is flagged via `mustChangePassword`.
- **Operator**: Provisioned via Admin user management or machine API keys (`aegis_ak_*`).
- **Viewer**: Standard read-only observation role.

---

## 2. Environment Startup

### Step 2.1 — Start Infrastructure Dependencies
```bash
docker-compose up -d postgres redis kafka
```

### Step 2.2 — Start the Go Backend Control Plane
In a terminal, start the Go API gateway:
```bash
cd go-services
go run ./api-gateway
```
*Expected log:* `[api-gateway] Control plane listening on http://localhost:8000`

### Step 2.3 — Start the Python ML Inference Engine
In a second terminal, launch the TGNN serving daemon:
```bash
cd python-ml
python serving/grpc_server.py
```
*Expected log:* `[grpc-server] TGNN Engine loaded and ready for inference requests.`

### Step 2.4 — Launch the Frontend Observability Console
In a third terminal, start the Vite development server:
```bash
cd frontend
npm run dev
```
Open your browser at **`http://localhost:5173`** and log in as `admin@aegis.local`.

---

## 3. Mission Control Dashboard Overview

The **Mission Control Dashboard** (`/`) is the central presentation hub uniting all core components into a single screen:

1. **Interactive Demo Presets Toolbar (Top)**:
   - `CPU Cascade`: Injects CPU core saturation on `node-c`.
   - `Latency Gate`: Injects 950ms latency deadlock on `node-b`.
   - `Memory SCC`: Injects memory leak on `node-d` to show bottom-up SCC DAG recovery.
   - `Reset`: Immediately halts all faults and restores healthy baseline.
2. **Top Metric Cards**: Real-time System Health, System Failure Risk ($[0\%, 100\%]$), TGNN Prediction Horizon countdown, Monitored Services count, and Autonomous Self-Healing status (`ACTIVE` vs `PAUSED`).
3. **Interactive Topology Graph (`TopologyGraph`)**: Live auto-laid-out DAG graph with animated edge pulses, health halos, root-cause halos, and blast-radius rings.
4. **Prediction Engine (`PredictionPanel`)**: Real-time failure probability gauge, confidence score, root-cause entity with telemetry evidence, propagation cascade sequence, and 1-click remediation.
5. **Recovery Log & Autonomous Controls (`RecoveryLog`)**: Autonomous mode toggle, in-flight 10-second verification window timer, chronological recovery records with risk delta ($92\% \to 12\%$), and 1-click rollback.
6. **Chaos Controls (`ChaosControls`)**: 10 fault injectors with target selection, duration slider, and live active countdowns.

---

## 4. Demo Scenario 1: Autonomous Closed-Loop Self-Healing (CPU Cascade)

### Goal
Demonstrate how sustained CPU load causes queuing, triggers the TGNN failure prediction head, and executes an automated, dependency-safe `SCALE_UP` remediation without requiring human intervention.

### Procedure
1. On the **Mission Control Dashboard**, click the **`CPU Cascade`** preset button (or inject `cpu_stress` on `node-c` via `ChaosControls`).
2. **Watch the live transition**:
   - `node-c` turns degraded/critical (CPU $> 85\%$, latency $> 400\text{ms}$).
   - **`TopologyGraph`** pulses edges in amber/crimson and highlights `node-c` with a pulsing red root-cause halo.
   - Downstream dependent nodes highlight in purple with blast-radius badges.
3. **Observe TGNN Prediction**:
   - **`PredictionPanel`** calculates Failure Probability $\ge 91\%$ and Confidence $\sim 98\%$.
   - Root-cause isolated to `node-c` (telemetry evidence: CPU $95.4\%$).
   - Decision Engine matches `RULE_HIGH_CPU_CASCADE` from `rulebook.yaml`.
   - Risk evaluated at $0.35$ (`LOW/MEDIUM` risk $\rightarrow$ auto-approved).
4. **Observe Autonomous Remediation**:
   - If autonomous recovery is `ACTIVE`, Aegis automatically triggers `SCALE_UP` on `node-c`.
   - Active chaos fault is cleared and replica count increases from $1 \to 3$.
   - **`RecoveryLog`** displays in-flight execution and begins the real **10-second Verification Window**.
   - After 10 seconds of verified SLA metrics, status transitions to **`Verified`**.
   - System failure risk drops from $91\% \to 8\%$, and all nodes return to green `healthy`.

---

## 5. Demo Scenario 2: High-Risk Action & Human-in-the-Loop Approval Gate

### Goal
Demonstrate how critical network latency deadlocks trigger high-impact actions (`REROUTE_TRAFFIC`), which are intercepted by the **Approval Gate** to require explicit operator authorization before execution.

### Procedure
1. On the dashboard, click **`Latency Gate`** (or select `latency` on `node-b` for 30s).
2. P99 latency spikes above $950\text{ms}$, triggering critical alerts.
3. **`PredictionPanel`**:
   - Identifies `node-b` as root cause.
   - Recommended action: `REROUTE_TRAFFIC` (Base risk: $0.50$).
   - Blast radius penalty and Tier-1 criticality escalate overall risk to $\mathbf{0.70}$ (`HIGH RISK`).
   - Approval policy flags: **`APPROVAL REQUIRED`**.
4. Action holds in `PENDING_APPROVAL` status at the authority boundary gate.
5. In **Recovery Console** (`/recovery`) or pending queue:
   - Review plan ID, risk justification, and blast radius.
   - Click **Authorize & Execute Plan** (authenticated as Operator/Admin).
   - Traffic is rerouted, drain completes, 10s verification passes, and status transitions to **`Verified`**.

---

## 6. Demo Scenario 3: Memory Leak & Bottom-Up SCC Condensed DAG Recovery

### Goal
Demonstrate how memory exhaustion causes cascading degradation and how the **SCC Condensation Topological Planner** orders remediation actions bottom-up so dependencies heal before calling services.

### Procedure
1. Click **`Memory SCC`** (injects `memory_leak` on database sink `node-d`).
2. As `node-d` memory exceeds $90\%$, upstream callers `node-c` and `node-a` degrade due to connection pool waits.
3. TGNN identifies `node-d` as root cause, with `node-c` in the propagation cascade.
4. Recovery Planner decomposes the affected subgraph into Strongly Connected Components (Tarjan's SCC algorithm) and executes bottom-up topological ordering:
   - **Step 1**: Restart container `node-d` (dependency sink recovers first).
   - **Step 2**: Reconnect / restart `node-c` (calling service recovers second).
5. Verify in **`RecoveryLog`** that Step 1 finishes before Step 2 begins.
6. Memory resets to nominal $<30\%$ and the entire cluster turns healthy.

---

## 7. Demo Scenario 4: Rollback & Verification Failure Handling

### Goal
Demonstrate how reversible actions can be rolled back in one click, and how failed remediations automatically escalate.

### Procedure
1. Locate any completed `Verified` action in **`RecoveryLog`**.
2. Click the **`Rollback`** button.
3. Control plane invokes `POST /api/recovery/rollback/{id}`:
   - Resets replica counts or restores traffic routes.
   - Updates execution status to **`RolledBack`** (purple badge).
   - Records an immutable security audit log entry.

---

## 8. Demo Scenario 5: Dynamic Scalability (Any N Services)

### Goal
Demonstrate that Aegis is completely domain-agnostic and dynamically monitors any generic cloud architecture.

### Procedure
1. Switch to **Settings / Topology** or issue the bulk import API command:
   ```bash
   curl -X POST http://localhost:8000/api/registry/import \
     -H "Content-Type: application/json" \
     -H "Cookie: aegis_session=..." \
     -d @configs/topology.example.yaml
   ```
2. In the navbar, toggle the mode pill or select the imported cluster.
3. Observe **`TopologyGraph`** instantly rendering the expanded 25+ node topology with zero code changes or restarts!

---

## 9. Verification Commands (CLI Quick Reference)

```bash
# 1. Check live mesh telemetry
curl -s http://localhost:8000/api/telemetry/services | jq .

# 2. Inject chaos fault
curl -s -X POST http://localhost:8000/api/chaos/inject \
  -H "Content-Type: application/json" \
  -d '{"faultType":"cpu_stress","targetServiceId":"node-c","durationSeconds":30}' | jq .

# 3. Fetch latest TGNN prediction & root cause
curl -s http://localhost:8000/api/prediction/latest | jq .

# 4. Check autonomous recovery status
curl -s http://localhost:8000/api/recovery/autonomous | jq .

# 5. Check audit logs
curl -s http://localhost:8000/api/audit/logs | jq .
```
