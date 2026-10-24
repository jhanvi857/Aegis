# Aegis Reinforcement Learning & Extended Hardening Architecture (Phase 6)

## Overview

Phase 6 completes the advanced stretch capabilities of Aegis:
1. **Reinforcement Learning Recovery Agent (`python-ml/rl_agent/`)**: Replaces or supplements static heuristic rulebooks with a learned Deep Q-Network (DQN) policy that explores remediation sequencing in simulated failure environments.
2. **Comprehensive Chaos Fault Injector Suite (10 Injectors)**: Physical and simulated fault injectors across CPU, memory, latency, packet drop, message queue backlog, cache outages, DB locks, slow queries, thread pool starvation, and process crashes.
3. **Hardened Human-in-the-Loop Approval Gate (`go-services/recovery-engine/approval_gate.go`)**: Authority boundary enforcing dual-mode low-risk automatic execution versus high-risk operator authorization.

---

## 1. Reinforcement Learning Recovery Architecture (`python-ml/rl_agent/`)

> [!IMPORTANT]
> **Simulation Scope Notice:** The RL recovery agent is trained and evaluated on a simulated environment (`MicroserviceChaosEnv`); it is **not validated on real clusters** and makes no claims of production validation. Production deployments rely on the deterministic DAG-ordered rulebook decision engine.

### Environment: `MicroserviceChaosEnv` (`env.py`)
A simulated, Gym-compatible environment that models microservice network topology, dynamic cascading latency, and remediation responses.

- **State Representation Vector**:
  For $N$ microservices in the topology, each service is represented by a 5-dimensional feature tuple:
  $$s_i = \left[\text{status\_code}, \frac{\text{CPU}}{100}, \frac{\text{Memory}}{100}, \frac{\text{Latency}}{2000}, \text{ErrorRate}\right]$$
  Global state vector dimension:
  $$\dim(\mathcal{S}) = 5 \times N$$

- **Action Space**:
  Discrete action space consisting of:
  - Action `0`: `NOOP` (maintain observation)
  - Action $a \in [1, 4N]$: Discrete mapping to `(Node, ActionType)` where action types include `RESTART`, `SCALE_UP`, `REROUTE_TRAFFIC`, and `FLUSH_CACHE`.
  $$\dim(\mathcal{A}) = 1 + 4N$$

- **Reward Function & Shaping**:
  The reward function incentivizes rapid cluster recovery while penalizing unnecessary or disruptive actions:
  $$R_t = R_{\text{health}} + R_{\text{penalty}} + R_{\text{terminal}}$$
  Where:
  - $R_{\text{health}} = -1.0 \times (\text{Number of degraded or critical nodes})$
  - $R_{\text{penalty}} = -0.5$ for high-impact actions like `RESTART` or `REROUTE_TRAFFIC`
  - $R_{\text{terminal}} = +20.0$ if all microservices achieve nominal SLA health

---

### Policy Network: `RecoveryPolicyNetwork` (`policy.py`)
A Deep Q-Network parameterized with PyTorch:
- Input layer: $\dim(\mathcal{S})$
- Hidden Layer 1: Linear(dim, 128) $\to$ LayerNorm $\to$ ReLU
- Hidden Layer 2: Linear(128, 64) $\to$ LayerNorm $\to$ ReLU
- Output layer: Linear(64, $\dim(\mathcal{A})$) emitting action-value Q-scores: $Q(s, a; \theta)$
- Exploration: $\epsilon$-greedy policy with exponential decay ($\epsilon_{\text{start}} = 1.0, \epsilon_{\text{min}} = 0.05, \gamma = 0.95$).
- Checkpointing: Checkpoints serialize to `python-ml/models/checkpoints/rl_policy_best.pt`.

---

### Training Loop (`train.py`)
- Experience Replay Buffer: Fixed capacity ($5000$ transitions) to break temporal correlation.
- Target Network: Polyak or periodic target weight synchronization every 5 episodes.
- Optimization: Smooth L1 (Huber) loss minimized via Adam optimizer ($lr = 10^{-3}$).

---

## 2. Complete 10-Fault Chaos Engineering Suite

Aegis supports 10 generic fault injectors operating across physical container environments (Docker / Kubernetes) and live telemetry simulation:

| Injector Name | Injector Source | Fault Mechanism | Injected Signal |
| :--- | :--- | :--- | :--- |
| **CPU Stress** | `injectors/cpu_stress.go` | Computational spin-loops on CPU cores | CPU $> 95\%$, Queuing delay |
| **Network Latency** | `injectors/latency.go` | Injected kernel delay & socket jitter | Latency $> 950\text{ms}$ |
| **Service Crash** | `injectors/kill_service.go` | SIGKILL process termination | Connection refused, Error rate $100\%$ |
| **Memory Leak** | `injectors/memory_leak.go` | Heap allocation exhaustion | Memory $> 95\%$, OOMKilled risk |
| **Packet Loss** | `injectors/packet_loss.go` | TCP socket packet drop (25%) | Retransmissions, Timeout errors |
| **Message Queue Lag** | `injectors/mq_lag.go` | Consumer delay & queue accumulation | Queue depth $> 800$, Backlog latency |
| **Cache Down** | `injectors/cache_down.go` | In-memory cache eviction & denial | Cache miss rate $> 90\%$, DB load |
| **DB Lock** | `injectors/db_lock.go` | Exclusive transaction lock contention | DB wait $> 250\text{ms}$, Connection pool stalls |
| **Slow Query** | `injectors/slow_query.go` | Full table scan emulation | Data retrieval latency $> 700\text{ms}$ |
| **Thread Exhaustion**| `injectors/thread_exhaustion.go`| Worker thread pool starvation | Rejected connections, 503 HTTP codes |

---

## 3. Human-in-the-Loop Authority Boundary Gate (`approval_gate.go`)

High-risk or irreversible recovery actions are intercepted by the Go Approval Gate (`go-services/recovery-engine/approval_gate.go`):
- **Deterministic Risk Partitioning**:
  - `LOW` Risk ($< 0.40$): Dispatched autonomously with zero operator delay.
  - `MEDIUM` / `HIGH` Risk ($\ge 0.40$): Held at the gate in `PENDING_APPROVAL` status.
- **Operator Authorization**:
  Requires signed operator authentication via `POST /api/recovery/approve`.
- **Immutable Audit Logging**:
  All approval grants, rejections, and execution outcomes are recorded permanently in the append-only PostgreSQL `audit_logs` table.
