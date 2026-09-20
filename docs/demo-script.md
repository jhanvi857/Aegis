# Aegis Live Demo Walkthrough & Operator Runbook

This document provides a step-by-step walkthrough for demonstrating the Aegis Self-Healing Distributed Systems Platform end-to-end.

---

## 1. Architecture & Port Mapping Reference

| Service | Port | Description |
|---|:---:|---|
| **Frontend Dashboard (React + Vite)** | `:5173` | Real-time topology visualization, chaos triggers, prediction telemetry, and approval controls. |
| **Aegis Control Plane (`api-gateway`)** | `:8000` | Orchestrates live telemetry, chaos injection, predictions, and recovery execution. |
| **Python ML Inference Server** | `:50051` | Unified TGNN inference engine serving failure prediction, root-cause localization, and propagation paths. |
| **Go Recovery Engine** | `:8092` | Manages container/pod actions via orchestrator adapters and executes the approval gate. |
| **Microservice Mesh (`playground`)** | `:8080`–`:8084` | Generic service mesh: `gateway` (:8080), `node-a` (:8081), `node-b` (:8082), `node-c` (:8083), `node-d` (:8084). |

---

## 2. Environment Startup

### Step 2.1 — Start the Backend Control Plane
In a terminal, start the Go API gateway:
```bash
cd go-services
go run ./api-gateway
```
*Expected log:* `[api-gateway] Control plane listening on http://localhost:8000`

### Step 2.2 — Start the Python ML Inference Engine
In a second terminal, launch the TGNN serving daemon:
```bash
cd python-ml
python serving/grpc_server.py
```
*Expected log:* `[grpc-server] TGNN Engine loaded and ready for inference requests.`

### Step 2.3 — Launch the Frontend Observability Console
In a third terminal, start Vite development server:
```bash
cd frontend
npm run dev
```
Open your browser at **`http://localhost:5173`**.

---

## 3. Demo Scenario 1: Autonomous Remediation (CPU Saturation Cascade)

### Goal
Demonstrate how sustained CPU load causes queuing, triggers the TGNN failure prediction head, and executes an automated, dependency-safe `SCALE_UP` remediation without requiring human intervention.

### Procedure
1. On the dashboard, navigate to **Chaos Studio** (`/chaos`).
2. Select target service: `node-c`.
3. Choose injector: **CPU Stress (`cpu_stress`)**.
4. Set duration to `30s` and click **Inject Fault**.
5. Switch to **Live Topology Graph** (`/topology`):
   - Observe `node-c` transitioning to `degraded` (CPU $>85\%$, latency $>400\text{ms}$).
   - Notice downstream and upstream links highlighting the propagation cascade.
6. Open **Predictions Panel** (`/predictions`):
   - TGNN predicts impending failure with $>90\%$ probability.
   - Root cause identified: `node-c` (contributing metric: `cpu_usage_percent`).
   - Decision engine matches `RULE_HIGH_CPU_CASCADE` from `rulebook.yaml`.
   - Risk score evaluated at `0.35` (`MEDIUM` risk $\rightarrow$ auto-approved).
7. Go to **Remediation Console** (`/recovery`):
   - Click **Execute Recommended Remediation** (or watch autonomous trigger).
   - Replica count increases (`replicas: 1 -> 3`).
   - Health status recovers to `healthy` and latency normalizes back to nominal $\sim 15\text{ms}$.

---

## 4. Demo Scenario 2: High-Risk Action & Human-in-the-Loop Approval Gate

### Goal
Demonstrate how catastrophic latency spikes or network deadlocks trigger high-impact actions (`REROUTE_TRAFFIC`), which are intercepted by the **Approval Gate** to prevent unauthorized traffic diversion.

### Procedure
1. In **Chaos Studio**, select target `gateway` or `node-a`.
2. Choose injector: **Latency Injection (`latency`)**.
3. Set injected delay to `1500ms` and duration to `45s`. Click **Inject Fault**.
4. Observe the **Incident Alert** banner at the top of the navbar:
   - Status transitions to `critical` with P99 latency $> 1000\text{ms}$.
   - Blast radius expands to downstream nodes.
5. In **Predictions**:
   - Primary root cause: `node-a`.
   - Decision engine recommends `REROUTE_TRAFFIC` to isolate the failure.
   - Risk assessment flags **`HIGH RISK`** (score $\ge 0.70$ / critical traffic impact).
   - Approval Gate holds action in `pending_approval` state.
6. Check **Pending Approvals** in the Remediation view:
   - Displays plan ID, risk justification, and affected downstream services.
   - Click **Authorize & Execute Plan**.
   - Action completes: traffic is rerouted, drain completes in $10\text{s}$, and overall risk drops from $92\%$ to $12\%$.

---

## 5. Demo Scenario 3: Memory Leak & Topological Bottom-Up Recovery

### Goal
Demonstrate how memory exhaustion causes cascading degradation and how the **SCC Condensation Topological Planner** orders remediation actions so bottom-up dependencies heal before calling services.

### Procedure
1. Inject **Memory Leak (`memory_leak`)** on persistence sink `node-d`.
2. As `node-d` memory exceeds $90\%$, upstream processors `node-c` and `node-a` degrade due to connection wait times.
3. TGNN identifies `node-d` as root cause, with `node-c` in propagation path.
4. Recovery plan generates 2-step dependency sequence:
   - **Step 1**: Restart container `node-d` (dependency sink recovers first).
   - **Step 2**: Reconnect / restart `node-c` (caller recovers second).
5. Execute plan: verify in the event log that Step 1 executes strictly before Step 2.
6. Both nodes return to green `healthy` state.

---

## 6. Verification Commands (CLI)

Verify system state directly via HTTP:

```bash
# 1. Check mesh health
curl -s http://localhost:8000/api/telemetry/services | jq .

# 2. Inject latency chaos via API
curl -s -X POST http://localhost:8000/api/chaos/inject \
  -H "Content-Type: application/json" \
  -d '{"faultType":"latency","targetServiceId":"node-b","durationSeconds":30}' | jq .

# 3. Fetch latest TGNN prediction & root cause
curl -s http://localhost:8000/api/prediction/latest | jq .

# 4. Fetch recovery recommendations and risk scores
curl -s http://localhost:8000/api/recovery/recommendations | jq .

# 5. Execute remediation
curl -s -X POST http://localhost:8000/api/recovery/execute \
  -H "Content-Type: application/json" \
  -d '{"actionType":"restart","targetServiceId":"node-b"}' | jq .
```
