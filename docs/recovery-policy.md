# Aegis Recovery & Remediation Policy Architecture (Phase 2)

## Overview

Aegis implements an industry-agnostic, deterministic risk assessment and verification pipeline governing all automated and manual system remediations. Remediations transition through a formal policy evaluation, approval gate, dispatch, real telemetry verification window, and escalation/rollback cycle:

```
               Remediation Proposal (UI or TGNN)
                              │
                              ▼
           Canonical Policy Check (/api/recovery/policy-check)
              Risk = f(Action, Tier, Blast Radius)
                              │
               ┌──────────────┴──────────────┐
               ▼                             ▼
       Low Risk (< 0.40)           Medium / High Risk (>= 0.40)
       & Auto-Approved             Requires Operator Approval Gate
               │                             │
               └──────────────┬──────────────┘
                              ▼
                   Safety Limit Evaluation
               - Service Cooldown (30s)
               - Max Concurrent In-Flight (2)
               - Global Blast Radius Budget (5)
                              │
                              ▼
                    Dispatch Execution
               (Persist Execution Record)
                              │
                              ▼
            Real Asynchronous Verification Window (10s)
               - Polls live telemetry metrics
               - Status: Pending -> Verifying
                              │
               ┌──────────────┴──────────────┐
               ▼                             ▼
       Healthy Telemetry             Degraded Telemetry
       Status: "Verified"            Status: "Ineffective"
       (Duration & Real Metrics)     Escalate to Secondary Action (rulebook.yaml)
                                     Emit Critical Alert to SecOps
```

---

## 1. Risk Calculation Formula

Risk is computed deterministically by `AssessPolicy(actionType, targetServiceID)` using the formula:

$$\text{RiskScore} = \min\left(1.0, \text{BaseSeverity} + \text{BlastRadiusPenalty} + \text{CriticalityPenalty}\right)$$

### Base Severity (from `rulebook.yaml` / Canonical Rulebook)
- `RESTART`: 0.20 (Reversible: false, Approval: false)
- `SCALE_UP`: 0.35 (Reversible: true, Approval: false)
- `INCREASE_POOL`: 0.35 (Reversible: true, Approval: false)
- `FLUSH_CACHE`: 0.40 (Reversible: false, Approval: true)
- `REROUTE_TRAFFIC`: 0.50 (Reversible: true, Approval: true)
- `ISOLATE`: 0.80 (Reversible: false, Approval: true)

### Blast Radius Penalty
Downstream reachable dependencies computed via BFS:
$$\text{BlastRadiusPenalty} = \min\left(0.30, \text{BlastRadiusSize} \times 0.08\right)$$

### Criticality Penalty
Target tier criticality based on topology role:
- Ingress Gateway (`tier-1-ingress`): Criticality = 0.85 ($\times 0.15 \approx +0.13$)
- Storage/Database (`tier-1-storage`): Criticality = 0.70 ($\times 0.15 \approx +0.11$)
- Standard Service (`tier-2`): Criticality = 0.40 ($\times 0.15 = +0.06$)

### Risk Levels
- **`LOW`**: $\text{RiskScore} < 0.40$
- **`MEDIUM`**: $0.40 \le \text{RiskScore} < 0.70$
- **`HIGH`**: $\text{RiskScore} \ge 0.70$ (or `ISOLATE`)

---

## 2. Safety Limits & Guardrails

| Guardrail | Setting | Behavior when Violated |
| :--- | :--- | :--- |
| **Service Cooldown** | 30 seconds | `429 Too Many Requests` returning remaining seconds (`cooldownRemainingSec`). |
| **Max Concurrent In-Flight** | 2 actions | `429 Too Many Requests` blocking dispatches until active verifications finish. |
| **Global Blast Radius Budget** | 5 nodes | `429 Too Many Requests` preventing concurrent cascade risk across the cluster. |
| **Idempotency** | `Idempotency-Key` header | Returns previously recorded execution immediately with `idempotentReplay: true`. |
| **Dry-Run Mode** | `dryRun: true` or `X-Dry-Run: true` | Validates policy, returns simulation payload without mutating state or triggering cooldown. |

---

## 3. Real Asynchronous Telemetry Verification

Fake static duration (`1.2s`) and hardcoded metrics are replaced with a real verification lifecycle:
- Status transitions: `Verifying` $\to$ `Verified` $\mid$ `Ineffective` $\mid$ `RolledBack`.
- Records real start timestamp, completion timestamp, elapsed duration (`durationMs`, `durationStr`), pre-remediation metrics (`metricsBefore`), and post-remediation metrics (`metricsAfter`).
- If metrics fail to normalize within the window, status marks `Ineffective` and generates an escalation alert using the secondary action from `FaultFamilyEscalations`:
  - `cpu_saturation`: Primary `SCALE_UP` $\to$ Secondary `RESTART`
  - `thread_exhaustion`: Primary `INCREASE_POOL` $\to$ Secondary `RESTART`
  - `memory_leak`: Primary `RESTART` $\to$ Secondary `ISOLATE`
  - `service_failure`: Primary `RESTART` $\to$ Secondary `REROUTE_TRAFFIC`

---

## 4. Rollback Capability

Reversible remediation actions can be reverted via:
```http
POST /api/recovery/rollback/{id}
```
Reverts physical service state (e.g. scale replica decrease, traffic routing normalization), updates the database execution status to `RolledBack`, and logs an audit record.
