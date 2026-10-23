# Aegis Decision Engine & Recovery Planning Architecture (Phase 4)

## Overview

Phase 4 completes the closed-loop autonomous self-healing capability of Aegis:
```
Observe Telemetry → Detect Anomalies → TGNN Prediction → Decision Engine → DAG-Ordered Plan → Safe Execution → Verification Window
```

Aegis is strictly domain- and industry-agnostic. All remediation logic, risk evaluation, and dependency ordering operate on generic graph structures and dynamic telemetry metrics.

---

## Architecture Pipeline

```
           Live Telemetry / Anomalous State
                         │
                         ▼
             TGNN Root Cause & Propagation
                         │
                         ▼
        Decision Engine (python-ml/decision_engine)
        - Condition matching via rulebook.yaml
        - Base risk calculation
        - Blast radius & criticality escalation
                         │
                         ▼
      DAG Recovery Planner (python-ml/recovery_planner)
        - Extract affected subgraph
        - Strongly Connected Components (SCC) Condensation (Tarjan)
        - Bottom-up Topological Sort (leaves / dependencies first)
        - Emit protobuf RecoveryPlan (recovery.proto)
                         │
                         ▼
             Control Plane / Recovery Engine
             (Autonomous Loop or Operator Approval)
                         │
        ┌────────────────┴────────────────┐
        ▼                                 ▼
   Low Risk (< 0.40)              High Risk (>= 0.40)
   & Auto-Approved                Held at Approval Gate
        │                         (Awaits Operator POST /api/recovery/approve)
        │                                 │
        └────────────────┬────────────────┘
                         ▼
              Safety Limits Enforcement
         - Per-service cooldown (30s)
         - Max concurrent dispatches (2)
         - Blast-radius budget (5 nodes)
                         │
                         ▼
            Orchestrator Execution (Docker / K8s)
                         │
                         ▼
        Telemetry Verification Window (10-30s)
         - Real metric validation
         - Persist execution & audit log
         - Automatic cooldown release
```

---

## 1. Decision Engine (`python-ml/decision_engine/`)

### Rulebook (`rulebook.yaml`)
Remediation strategies are declaratively specified in `configs/rulebook.yaml` and `python-ml/decision_engine/rulebook.yaml`. Conditions evaluate anomaly indicators across fault families:

| Fault Family | Primary Condition | Recommended Action | Secondary / Escalation | Base Risk | Reversible |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Memory Leak** | Memory > 85%, Error Rate > 5% | `RESTART` | `SCALE_UP` | 0.20 | No |
| **CPU Saturation** | CPU > 85%, RPS > 500, Latency > 300ms | `SCALE_UP` | `REROUTE_TRAFFIC` | 0.35 | Yes |
| **Cache Degradation**| Cache Miss > 60%, Latency > 200ms | `FLUSH_CACHE` | `SCALE_UP` | 0.40 | No |
| **DB Pool Exhaustion**| DB Wait > 150ms, Thread count > 80 | `INCREASE_POOL` | `RESTART` | 0.35 | Yes |
| **Latency Deadlock** | Latency > 800ms, Error Rate > 15% | `REROUTE_TRAFFIC`| `RESTART` | 0.50 | Yes |
| **Cascading Failure**| Error Rate > 40%, Cascade Depth >= 2 | `REROUTE_TRAFFIC`| `ISOLATE` | 0.50 | Yes |

### Risk Assessment Formula
The risk assessment engine calculates risk deterministically:

$$\text{RiskScore} = \min\left(1.0, \text{BaseSeverity} + \text{BlastPenalty} + \text{CriticalityPenalty}\right)$$

Where:
- $\text{BlastPenalty} = \min(0.30, 0.05 \times \text{ReachableNodes})$
- $\text{CriticalityPenalty} = 0.20$ (Tier-1 / High Centrality), $0.10$ (Tier-2), $0.0$ (Tier-3)
- If $\text{RiskScore} \ge 0.40$ or action is non-reversible with blast radius $> 1$, `requires_approval = True`.

---

## 2. SCC-Condensed Dependency Ordering (`python-ml/recovery_planner/`)

Distributed systems often contain feedback loops, bidirectional health checks, or cyclic dependencies. Standard topological sorting fails or throws errors on cyclic graphs.

### Bottom-Up SCC Condensation Algorithm:
1. **Affected Subgraph Extraction:** Isolate root-cause nodes and all downstream propagation nodes.
2. **Strongly Connected Component (SCC) Decomposition:** Tarjan's linear-time algorithm groups mutually recursive nodes into condensed supernodes $C_1, C_2, \dots, C_k$.
3. **Condensation DAG Construction:** Edges between components are collapsed into a directed acyclic graph (DAG).
4. **Topological Sort:** Kahn's or DFS-based ordering on the condensation DAG ensures that **dependencies are sorted before dependents** (bottom-up remediation).
5. **Flattening:** Individual microservices within each component are unpacked in topological sequence, ensuring dependent callers are never restarted before the underlying callee recovers.

---

## 3. Autonomous Execution & Safety Limits (`go-services/api-gateway/`)

### Closed-Loop Autonomous Loop
- Enabled by default (`AEGIS_AUTONOMOUS_RECOVERY=true`).
- In `State.runTicker()`, the control plane checks for degraded or critical services.
- If recommendations exist and no actions are currently in-flight, it invokes `ExecuteAutonomousRecovery`:
  - **Policy check:** Verifies if action requires operator approval.
  - **Approval Gate:** If `RequiresApproval == true`, the plan is marked `PENDING_APPROVAL` and held at the gate.
  - **Safety check:** Enforces per-service cooldown, concurrent dispatch limits, and blast radius budgets.
  - **Remediation dispatch:** Halts active faults, resets microservice metrics, triggers orchestrator adapters.
  - **Verification:** Begins an asynchronous telemetry verification window (10s) before marking `SUCCESS` or triggering secondary escalation.
  - **Audit trail:** Records immutable audit logs in PostgreSQL / memory store.

### API Endpoints
- `GET /api/recovery/autonomous`: Returns current status (`enabled`, `activeDispatches`, `cooldownSeconds`, `maxConcurrent`).
- `POST /api/recovery/autonomous`: Enables or disables autonomous remediation mode (Operator / Admin only).
- `POST /api/recovery/execute`: Executes a specific recovery action (respects approval gate & safety limits).
- `POST /api/recovery/approve`: Approves a pending high-risk plan (Operator / Admin only).
- `GET /api/recovery/policy-check`: Pre-flight risk evaluation and safety limit validation.
