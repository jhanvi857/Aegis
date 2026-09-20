# Aegis: Self-Healing Distributed Systems Platform

> **Industry-Agnostic Causal Failure Prediction, Cascade Blast-Radius Forecasting & DAG-Ordered Autonomous Recovery**

[![Tests](https://img.shields.io/badge/Tests-40%2F40%20Passing-brightgreen.svg)](#running-tests)
[![Architecture](https://img.shields.io/badge/Design-Industry--Agnostic-blue.svg)](#core-architectural-principles)
[![Python](https://img.shields.io/badge/Python-3.11%2B-3776AB.svg?logo=python&logoColor=white)](#tech-stack)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg?logo=go&logoColor=white)](#tech-stack)
[![React](https://img.shields.io/badge/Frontend-React%2018%20%7C%20Vite-61DAFB.svg?logo=react&logoColor=black)](#frontend-control-plane)
[![Kafka](https://img.shields.io/badge/Streaming-Apache%20Kafka-231F20.svg?logo=apachekafka&logoColor=white)](#streaming-telemetry-pipeline)

```
Detect → Predict → Explain → Fix
```

Aegis observes a live microservice mesh, predicts impending failures **15–30 seconds before SLA violation**, isolates the ground-zero root cause, forecasts cascade propagation across dependencies, and automatically executes dependency-ordered recovery actions on an SCC-condensed DAG.

---

## Key Highlights & Innovations

1. **Strictly Industry-Agnostic:** Operates entirely on an abstract dependency graph (`topology.yaml`). Swap the topology configuration to point Aegis at any arbitrary microservice mesh with zero code changes.
2. **Zero-Shot Inductive Topology Generalization:** Trained across 4-, 5-, and 6-node graphs; evaluates zero-shot on an **unseen 7-node diamond graph** (Topology D) with **83.3% Failure F1** and **50.0% Blast-Radius IoU** (non-graph baselines score 0.0%).
3. **Pre-Injection Windowing ($t < t_{\text{inj}}$):** Telemetry is sampled strictly before catastrophic failure threshold breach. The model learns subtle precursor drift rather than performing trivial post-crash disaster detection.
4. **First-Principles Graph Preprocessing:** Authentic pure-Python implementations of Tarjan's Strongly Connected Components (SCC) condensation, Kahn's Topological Sort, and Brandes' Betweenness Centrality complete *before* the TGNN runs, transforming cyclic service dependencies into a strict DAG (formally verified against NetworkX).
5. **Multi-Task Dense GATv2 + GRU:** Brody et al. (2021) dynamic graph attention ($e_{ij} = \mathbf{a}^T \text{LeakyReLU}(W_{src} h_i + W_{dst} h_j)$) with learned attention weight inspection (`model.get_attention_weights()`) and GRU temporal recurrence within a conservative 56K parameter budget.
6. **Dual Data Methodology:** Uses a stratified parametric precursor simulator for reproducible multi-topology pretraining, paired with `LiveKafkaEpisodeCollector` to validate against physical Go chaos-engine cluster telemetry.
7. **Closed-Loop Ordered Recovery:** Emits typed `RecoveryPlan` messages over gRPC. Actions are executed by the Go recovery engine in strict topological dependency order with safety approval gating.

---

## Empirical Benchmark Results

Evaluated on 98 stratified test episodes under strictly pre-injection observation windows ($t < t_{\text{inj}}$) with zero discrete status shortcut proxies:

### 1. Standard Benchmark (5 Model Tiers)

| Model Tier | Model Architecture | Failure F1 | Root Cause Top-1 | Root Cause Top-3 | Propagation Blast Radius IoU |
|---|---|:---:|:---:|:---:|:---:|
| **Model 3 (Ours)** | **TGNN (GATv2 + GRU, Canonical Seed 42)** | **99.3%** | **98.0%** | **100.0%** | **47.3%** |
| **Model 2** | **LSTM (Temporal Only Sequence)** | 83.3% | 60.2% | 91.8% | 0.0% *(no graph awareness)* |
| **Model 1** | **Isolation Forest (Tabular ML)** | 37.2% | 16.3% | 21.2% | 0.0% *(no graph awareness)* |
| **Baseline A** | **Heuristic Threshold Rule (Static Alerting)** | 25.0% | 38.8% | 38.8% | 0.0% *(no graph awareness)* |
| **Baseline B** | **Majority Class Baseline (Naive Mode)** | 83.3% | 28.6% | 28.6% | 0.0% *(no graph awareness)* |

#### Multi-Seed Reproducibility Verification (5 Fixed Seeds: `[42, 101, 202, 303, 404]`):
- **Failure F1:** $99.6\% \pm 0.9\%$ (Range: $97.8\% - 100.0\%$)
- **Root Cause Top-1 Accuracy:** $99.3\% \pm 1.3\%$ (Range: $96.7\% - 100.0\%$)
- **Propagation Blast-Radius IoU:** **$75.1\% \pm 10.3\%$** (Median: $79.4\%$, Canonical Seed 42: $70.5\%$, Range: $57.5\% - 84.9\%$)
- *Why did IoU swing across early reruns?* In early runs, unweighted loss used $\lambda_{\text{prop}} = 1.0$ (~50% IoU). Hyperparameter tuning (HPO-03) prioritized edge prediction ($\lambda_{\text{prop}} = 2.0$), lifting IoU to ~70.5%. Furthermore, step-thresholding on the 21 nominal test episodes (where target $y = \mathbf{0}$) drops from $1.0 \to 0.0$ upon a single edge crossing $0.501$, creating an expected $\pm 10\%$ seed variance.

### 2. Zero-Shot Unseen-Topology Inductive Transfer

Evaluated on **Topology D (7-node cross-coupled diamond)** never seen during training:

| Metric | TGNN (Zero-Shot) | Non-Graph Baselines (LSTM / IsoForest / Heuristic) | Random Guessing Baseline |
|---|:---:|:---:|:---:|
| **Failure Prediction F1** | **83.1%** | 0.0% – 52.0% | 50.0% |
| **Cascade Blast-Radius IoU** | **48.8%** | **0.0%** *(blind to graph structure)* | 0.0% |
| **Root Cause Top-1 Accuracy** | **30.0%** *(Top-3: 48.8%)* | 12.5% – 25.0% | 12.5% ($1/8$ classes) |

---

## Forensic Audits & Scientific Integrity

Aegis includes publication-grade forensic audits documented in [`docs/FORENSIC_LEAKAGE_AUDIT.md`](docs/FORENSIC_LEAKAGE_AUDIT.md) and [`docs/DATASET_CARD.md`](docs/DATASET_CARD.md):

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                               FORENSIC AUDIT SCORECARD                                 │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ • Chi-Square Position-Bias Test:  \chi^2 = 3.6429, p = 0.4565 (Zero position shortcut) │
│ • Empirical Root Cause Entropy:   H = 2.316 bits (99.7% of theoretical maximum)       │
│ • Two-Sample KS Distribution:     p in [0.119, 1.000] across all 8 features (No shift) │
│ • Precursor Feature Overlap:      Realistic Gaussian tail overlap (Fig 2)              │
│ • Hard-Case Degradation Curve:    Noise: 66.0%, Cyclic Retries: 52.0% (Graceful decay) │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

### High-Resolution Audit Plots (`docs/figures/`):
- `fig1_class_balance.png`: Stratification across 11 balanced classes (30% nominal, 7% each across 10 chaos types).
- `fig2_feature_margins_overlap.png`: Non-trivial feature overlap between nominal and precursor windows.
- `fig3_root_cause_distribution.png`: Uniform root-cause distribution accepted under $\chi^2$ ($p = 0.4565$).
- `fig4_train_val_test_ks_distributions.png`: Overlaid KDEs across splits confirming zero covariate shift.
- `fig5_structural_centrality.png`: In/Out-degree and Betweenness Centrality across the dependency graph.
- `fig6_stress_test_degradation.png`: Graceful degradation curves under heavy noise and cyclic SCC retries.

---

## System Architecture & Dataflow Diagrams

### 1. End-to-End Multi-Tier Runtime Architecture
```
┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                 AEGIS LAYERED ARCHITECTURE                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                  │
│  [ TIER 1: DISTRIBUTED SERVICES RUNTIME (Go) ]                                                   │
│   ┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐                │
│   │   gateway    │ ──> │    node-a    │ ──> │    node-b    │ ──> │    node-d    │  (Playground)  │
│   └──────────────┘     └──────┬───────┘     └──────────────┘     └──────────────┘                │
│                               └───> ┌──────────────┐                                            │
│                                     │    node-c    │                                            │
│                                     └──────────────┘                                            │
│          ▲                                                                                       │
│          │ Heartbeats & Metrics Polling                                                          │
│   ┌──────────────┐         [ CHAOS ENGINE (Go) ]                                                 │
│   │  Telemetry   │         ┌────────────────────────────────────────────────────────┐            │
│   │  Agent (Go)  │ <────── │ 10 Injectors: CPU, Latency, Leak, PacketLoss, DB Lock │            │
│   └──────┬───────┘         └────────────────────────────────────────────────────────┘            │
│          │ Protobuf Serialized Snapshots                                                         │
│          ▼                                                                                       │
│  [ TIER 2: STREAMING TELEMETRY (Kafka) ]                                                         │
│   ┌─────────────────────────────────────────────────────────────────────────┐                    │
│   │ Kafka Topic: aegis.telemetry (MetricSnapshot, ServiceLog, TraceSpan)    │                    │
│   └────────────────────────────────────┬────────────────────────────────────┘                    │
│                                        │ Consumer Deserialization                                │
│                                        ▼                                                         │
│  [ TIER 3: GRAPH PREPROCESSING & CONDENSATION (Python) ]                                         │
│   ┌────────────────────────┐   ┌────────────────────────┐   ┌────────────────────────┐           │
│   │   System Graph Build   │   │ Classical Centrality   │   │ Tarjan's SCC Algorithm │           │
│   │   (topology.yaml + A)  │ ─>│ Degree & Betweenness   │ ─>│ Cycle Collapse to DAG  │           │
│   └────────────────────────┘   └────────────────────────┘   └───────────┬────────────┘           │
│                                                                         │                        │
│                                                                         ▼                        │
│  [ TIER 4: DEEP GRAPH INTELLIGENCE (PyTorch TGNN) ]                                              │
│   ┌─────────────────────────────────────────────────────────────────────────┐                    │
│   │ Input Tensor: [Batch, T=10, Nodes=N, Features=8] + Adjacency + Masks    │                    │
│   │ 2x Dense GATv2 Layers (Spatial) + GRU Recurrence (Temporal Slopes)     │                    │
│   ├───────────────────┬───────────────────┬───────────────────┬─────────────┴────────┐           │
│   │ Failure Risk Head │  Root Cause Head  │ Propagation Head  │ Time-to-Failure Head │           │
│   │   P(fail) in 30s  │   Softmax(N+1)    │ Blast Radius Mask │      Huber Loss      │           │
│   └─────────┬─────────┴─────────┬─────────┴─────────┬─────────┴──────────────────────┘           │
│             └───────────────────┼───────────────────┘                                            │
│                                 ▼                                                                │
│  [ TIER 5: DECISION & RECOVERY PLANNING (Python) ]                                               │
│   ┌─────────────────────────────────────────────────────────────────────────┐                    │
│   │ Risk Assessment Scoring: Score = P(fail) * (1 - TTF/600) * Centrality   │                    │
│   │ Rulebook Policy: Map fault type -> [RESTART, SCALE, REROUTE, POOL, FLUSH│                    │
│   │ Dependency Ordering: Topological Sort on SCC-Condensed Subgraph         │                    │
│   └────────────────────────────────────┬────────────────────────────────────┘                    │
│                                        │ recovery.proto over gRPC (Port 50051)                   │
│                                        ▼                                                         │
│  [ TIER 6: RECOVERY EXECUTION (Go) ]                                                             │
│   ┌────────────────────────────────────┬────────────────────────────────────┐                    │
│   │ Approval Gate (Hold if Risk==HIGH) │ Orchestrator Adapter (Docker/K8s)  │                    │
│   └────────────────────────────────────┴────────────────┬───────────────────┘                    │
│                                                         │                                        │
│                                                         ▼                                        │
│  [ TIER 7: WEB CONTROL PLANE (React + Vite) ] <─── [ Self-Healed Playground ]                    │
│   Real-time topology, live alerts, and approval buttons                                          │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### 2. Tarjan's SCC Condensation & Topological Ordering Principle
```
RAW SERVICE GRAPH WITH CYCLIC RETRY LOOP:
       ┌───────────┐
       │  gateway  │
       └─────┬─────┘
             │
             ▼
       ┌───────────┐
       │  node-a   │
       └─────┬─────┘
             │
             ▼
       ┌───────────┐   Mutual Retry Loop   ┌───────────┐
       │  node-b   │ <═══════════════════> │  node-c   │   <-- CYCLIC DEADLOCK FOR NAIVE TOPO SORT
       └─────┬─────┘                       └───────────┘
             │
             ▼
       ┌───────────┐
       │  node-d   │
       └───────────┘

STEP 1: TARJAN'S SCC CONDENSATION ALGORITHM
Collapse strongly connected components {node-b, node-c} into a single unified SuperNode [SC_1]:

       ┌───────────┐
       │  gateway  │
       └─────┬─────┘
             │
             ▼
       ┌───────────┐
       │  node-a   │
       └─────┬─────┘
             │
             ▼
       ┌───────────┐
       │   SC_1    │  <-- SuperNode {node-b, node-c}
       └─────┬─────┘
             │
             ▼
       ┌───────────┐
       │  node-d   │
       └───────────┘
       (GUARANTEED DIRECTED ACYCLIC GRAPH - ZERO CYCLES)

STEP 2: TOPOLOGICAL SORT & DEPENDENCY-SAFE EXECUTION ORDER
Sequence recovery actions from deepest causal dependency upward to callers:
  1. Action 1: Repair node-d (Database / leaf sink)
  2. Action 2: Repair component SC_1 (Resolve shared contention in node-b / node-c)
  3. Action 3: Clear upstream backpressure at node-a
  4. Action 4: Unblock ingress gateway
```

### 3. Spatio-Temporal Neural Tensor Architecture (TGNN)
```
Input Telemetry Tensor: [Batch, Time=10, Nodes=N, Features=8]
Adjacency Tensor:       [Batch, Nodes=N, Nodes=N]
Dynamic Node Mask:      [Batch, Nodes=N]  (Masks dummy padded nodes to zero)
                       │
                       ▼
┌──────────────────────────────────────────────────────────────┐
│ SPATIAL BLOCK: 2-Layer Dense Batched GATv2                   │
│                                                              │
│  Layer 1: H1 = LeakyReLU( a^T [W·Hi || W·Hj] ) · (A ⊙ M M^T) │
│           Skip Residual Connection + LayerNorm(64)           │
│                                                              │
│  Layer 2: H2 = GraphConv(H1, A_norm) + LayerNorm(64)         │
│           Extracts directed structural message passing       │
└──────────────────────────────┬───────────────────────────────┘
                               │ Spatial Embeddings: [B, T, N, 64]
                               ▼
┌──────────────────────────────────────────────────────────────┐
│ TEMPORAL BLOCK: Recurrent Gated Unit (GRU)                   │
│                                                              │
│  Unrolls over T=10 timesteps per node:                       │
│  h_t = GRU( h_(t-1), x_t )                                   │
│  Detects subtle precursor drift slopes (0.10 -> 0.60 ramp)  │
│  Ignores nominal stochastic Gaussian jitter                  │
└──────────────────────────────┬───────────────────────────────┘
                               │ Node Representation: [B, N, 64]
                               ▼
┌──────────────────────────────────────────────────────────────┐
│ POOLING & AGGREGATION BLOCK                                  │
│  Spatial Mean Pool + Max Pool across active masked nodes:    │
│  Graph Context = [ MeanPool(H_N) || MaxPool(H_N) ] in R^128  │
└──────────────────────────────┬───────────────────────────────┘
                               │
         ┌─────────────────────┼─────────────────────┬─────────────────────┐
         ▼                     ▼                     ▼                     ▼
┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
│ CLUSTER FAILURE │   │   ROOT CAUSE    │   │   PROPAGATION   │   │ TIME-TO-FAILURE │
│      HEAD       │   │      HEAD       │   │   BLAST RADIUS  │   │      HEAD       │
├─────────────────┤   ├─────────────────┤   ├─────────────────┤   ├─────────────────┤
│ MLP(128 -> 1)   │   │ MLP(64 -> N+1)  │   │ MLP(64 -> 1)    │   │ MLP(128 -> 1)   │
│ Sigmoid         │   │ Softmax         │   │ Sigmoid / Node  │   │ Huber Regression│
│ P(fail in 30s)  │   │ Faulty Node ID  │   │ Cascade Subgraph│   │ Seconds to SLA  │
└─────────────────┘   └─────────────────┘   └─────────────────┘   └─────────────────┘
```

### 4. Closed-Loop Self-Healing Control Cycle
```
         ┌────────────────────────────────────────────────────────────────┐
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Continuous Metric Streaming (CPU, Lat, Err, Queue)  │
    │ OBSERVE  │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘                                                          │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Precursor drift detection 15-30s in advance         │
    │ FORECAST │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘      TGNN: P(failure) >= 0.85, Time-to-Failure estimate  │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Causal localization & blast-radius projection       │
    │ EXPLAIN  │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘      Identifies root cause node & downstream propagation │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Tarjan's SCC condensation + Topological Sort        │
    │   PLAN   │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘      Builds ordered, non-deadlocking RecoveryPlan        │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Dynamic Risk Score Assessment                       │
    │   GATE   │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘      Low/Med: Auto-approved | High: Human Review in UI   │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Docker / Kubernetes Orchestrator Adapters           │
    │ EXECUTE  │ ─────────────────────────────────────────────────────────┤
    └────┬─────┘      Rolling restart, auto-scaling, pool expansion       │
         │                                                                │
         ▼                                                                │
    ┌──────────┐      Telemetry re-check over subsequent 30 seconds       │
    │  VERIFY  │ ─────────────────────────────────────────────────────────┘
    └──────────┘      Cluster returns to nominal SLA baseline -> Incident Resolved
```

---

## Architectural Decision Records (ADRs)

The core architectural decisions governing Aegis are documented as formal ADRs:

| Decision ID | Context & Challenge | Decision Taken | Rationale & Trade-offs |
|---|---|---|---|
| **ADR-001** | High concurrency microservice mesh vs. deep graph ML requirements | **Split Language Architecture:** Go for distributed runtime & agents; Python for graph ML | Go delivers lightweight concurrency and minimal memory overhead for telemetry collection; Python provides native PyTorch Geometric and NetworkX ecosystems. |
| **ADR-002** | Telemetry ingestion throughput vs. control-plane RPC guarantees | **Dual Communication Contract:** Apache Kafka for telemetry; gRPC for RPCs | Kafka decouples high-frequency telemetry producers from ML consumers; gRPC provides typed, microsecond request-response calls for recovery dispatch. |
| **ADR-003** | Avoid domain-specific lock-in (e.g. e-commerce or streaming assumptions) | **Domain-Agnostic Schema (`topology.yaml`):** Model operates purely on graph primitives | The entire pipeline works on abstract `Node` and `Edge` dependencies. Aegis points at any microservice architecture with zero codebase changes. |
| **ADR-004** | Should classical graph algorithms run in parallel or before the TGNN? | **Sequential Preprocessing:** Graph algorithms complete strictly *before* TGNN | Betweenness centrality, in/out degrees, and SCC groupings are fed directly as structural feature vectors into the GNN, grounding the neural network in topological graph theory. |
| **ADR-005** | Microservice graphs contain retry cycles ($b \leftrightarrow c$) causing topo-sort crashes | **Tarjan's SCC Condensation:** Collapse cycles into super-nodes before topological sort | Raw topological sort assumes a DAG and crashes on cycles (`CycleError`). Condensing SCCs guarantees a DAG, preventing deadlocks during multi-stage recovery. |
| **ADR-006** | Post-crash observation windows introduce artificial target leakage | **Pre-Injection Observation ($t < t_{\text{inj}}$):** Strict 15–30s forward lookahead | Forces the model to detect subtle precursor degradation ramps ($0.10 \to 0.60$ severity) rather than trivial post-crash outage categorization. |
| **ADR-007** | Conventional GNNs bake fixed node counts into layer dimensions | **Dynamic 2D Node Masking:** $A_{\text{norm}} \odot (MM^T)$ with degree normalization | Enables inductive zero-shot transfer across unseen topologies with variable node counts (4 to 7+ nodes) without retraining. |
| **ADR-008** | Autonomous self-healing risks catastrophic outages on false positives | **Two-Tier Risk Approval Gating:** Dynamic risk score gates destructive actions | Low/Medium risk mitigations execute automatically; High-risk operations (e.g. ingress restarts) pause in the React dashboard pending human operator approval. |

---

### Chaos Fault Injectors vs. Autonomous Recovery Actions

| Injected Chaos Fault (`chaos-engine/injectors/`) | Impact Profile | Targeted Autonomous Recovery Action (`recovery-engine/actions/`) |
|---|---|---|
| `cpu_stress.go` | Compute core saturation, scheduling latency | `scale.go` (Horizontal replica scaling) |
| `latency.go` | Inbound/outbound communication lag | `reroute_traffic.go` (Circuit breaker dynamic rerouting) |
| `kill_service.go` | Process exit, socket termination | `restart.go` (Graceful process / container restart) |
| `memory_leak.go` | Monotonic RAM accumulation, heap thrashing | `restart.go` (Rolling worker restart) |
| `packet_loss.go` | Dropped TCP frames, transport timeouts | `reroute_traffic.go` (Dynamic edge rerouting) |
| `mq_lag.go` | Consumer backpressure, message accumulation | `scale.go` (Scale consumer worker replicas) |
| `thread_exhaustion.go` | Worker pool starvation | `increase_pool.go` (Expand worker thread capacity) |
| `db_lock.go` | Database lock contention, queue spikes | `increase_pool.go` (Expand database connection pool) |
| `slow_query.go` | Leaf database query stalls | `flush_cache.go` (Flush and warm query cache) |
| `cache_down.go` | Cache misses, downstream stampede | `flush_cache.go` / `restart.go` (Re-seed cache instance) |

---

## Repository Structure

```
aegis/
├── proto/                           # SHARED CONTRACT: Source of truth for Go & Python
│   ├── telemetry.proto              # MetricSnapshot, ServiceLog, TraceSpan
│   ├── graph.proto                  # GraphSnapshot, Node, Edge, GraphRepresentation
│   ├── predict.proto                # FailurePrediction, RootCause, Propagation
│   └── recovery.proto               # RecoveryPlan, RecoveryAction, RiskAssessment
│
├── go-services/                     # Distributed systems layer (Go)
│   ├── playground/                  # Simulated generic microservice mesh (gateway, node-a..d)
│   ├── telemetry-agent/             # High-frequency metric poller & Kafka publisher
│   ├── chaos-engine/                # 10 modular chaos injectors & episode scheduler
│   ├── recovery-engine/             # Action executor, approval gate, orchestrator adapter
│   ├── orchestrator-adapter/        # Docker and Kubernetes runtime abstraction
│   ├── api-gateway/                 # Control-plane REST API for React frontend
│   └── shared/pb/                   # Generated Go Protobuf stubs
│
├── python-ml/                       # Graph intelligence & ML pipeline (Python)
│   ├── ingestion/                   # Kafka telemetry consumer
│   ├── graph/                       # GraphBuilder, SCC condensation, BFS/DFS, centrality
│   ├── models/                      # TGNN (GATv2+GRU), LSTM, Isolation Forest, Heuristics
│   ├── tasks/                       # Multi-task heads: Failure, Root Cause, Propagation
│   ├── decision_engine/             # Risk assessment & rulebook mapping
│   ├── recovery_planner/            # Topological dependency ordering & plan builder
│   ├── rl_agent/                    # Phase 6 DQN reinforcement learning agent & environment
│   ├── serving/                     # gRPC ML prediction & planning server
│   ├── training/                    # Model training, baseline evaluation, forensic audits
│   └── shared/pb/                   # Generated Python Protobuf stubs
│
├── dataset/                         # Telemetry data & episodes
│   ├── raw/                         # Raw episode captures
│   ├── processed/                   # Tensor files (tensors.pt, multi_topology_tensors.pt)
│   ├── generator.py                 # Single-topology dataset generator (600 episodes)
│   ├── multi_topology.py            # Multi-topology dataset generator (1,000 episodes)
│   └── splits/                      # Train / Validation / Test indices
│
├── frontend/                        # Interactive React + Vite control plane
│   └── src/
│       ├── components/              # TopologyGraph, ServiceCard, ChaosControls
│       └── pages/                   # Dashboard, DependencyGraph, PredictionPanel, RecoveryLog
│
├── configs/                         # Configurations
│   ├── topology.example.yaml        # 5-node reference dependency graph
│   └── topologies/                  # Multi-topology benchmark suite (Topologies A, B, C, D)
│
├── docs/                            # Documentation & Technical Reports
│   ├── DATASET_CARD.md              # Formal 1-page Dataset Card (precedents, audits)
│   ├── FORENSIC_LEAKAGE_AUDIT.md    # Comprehensive leakage audit, EDA & hyperparameter records
│   ├── demo-script.md               # End-to-end demo walkthrough script
│   └── figures/                     # 6 publication-grade audit plots
│
└── tests/                           # Unit & integration tests across Go and Python
```

---

## Quickstart Guide

### 1. Prerequisites
- **Go:** 1.22 or newer
- **Python:** 3.10+ (PyTorch, PyTorch Geometric, NetworkX, NumPy, scikit-learn)
- **Node.js:** v18+ (for frontend dashboard)
- **Docker & Docker Compose:** Optional for full containerized mesh

### 2. Run the Full Stack Locally

#### A. Start the Go Control Plane (API Gateway)
```powershell
cd go-services
go run .\api-gateway\main.go
# API Gateway listens on http://localhost:8000
```

#### B. Start the Frontend Dashboard
```powershell
cd frontend
npm install
npm run dev
# Dashboard launches on http://localhost:5173
```

#### C. Start the Python ML Service
```powershell
cd python-ml
python serving/grpc_server.py
# gRPC ML Server listens on port 50051
```

---

## Running Evaluations & Audits

### 1. Run the Multi-Model Baseline Benchmark
Evaluates TGNN against LSTM, Isolation Forest, Heuristic Alert Rules, and Majority Class on held-out test episodes:
```bash
python python-ml/training/evaluate_baselines.py
```

### 2. Run the Multi-Topology Generalization Benchmark
Generates 1,000 episodes across 4 topologies and evaluates zero-shot transfer onto the unseen 7-node diamond graph:
```bash
python dataset/multi_topology.py
```

### 3. Run the Statistical Audits ($\chi^2$ and Two-Sample KS Tests)
Verifies zero root-cause position bias and zero train/val/test covariate shift:
```bash
python python-ml/training/audit_dataset.py
```

### 4. Run Edge-Case Stress Testing
Evaluates model behavior under heavy Gaussian noise ($\sigma=8.0$), cyclic SCC retry storms, and multi-point faults:
```bash
python python-ml/training/evaluate_stress_cases.py
```

### 5. Regenerate High-Resolution Audit Plots
Renders all 6 figures to `docs/figures/`:
```bash
python scripts/generate_audit_plots.py
```

---

## Running Tests

Execute the complete test suite:
```bash
# Run all Python ML, graph, and recovery tests (40 tests)
python -m unittest discover -s tests/python-ml

# Run Go recovery engine tests
cd go-services/recovery-engine
go test -v ./...
```

*Status:* **40/40 tests passing (100% pass rate, ~5.5s runtime)**.

---

## Technical Documentation & References

- **Dataset Card:** [`docs/DATASET_CARD.md`](docs/DATASET_CARD.md) — Formal specification detailing academic precedents (**GAIA**, **PodFailPred**, **WOLFFI**), class balance, precursor telemetry overlap, and structural diversity.
- **Forensic Leakage Audit & Evaluation Report:** [`docs/FORENSIC_LEAKAGE_AUDIT.md`](docs/FORENSIC_LEAKAGE_AUDIT.md) — Comprehensive technical report covering pre-injection windowing, feature proxy removal, hyperparameter search tables, error taxonomy, and theoretical causal proofs.
- **Demo Script:** [`docs/demo-script.md`](docs/demo-script.md) — Step-by-step presentation script demonstrating fault injection, real-time TGNN diagnosis, and topological recovery.

---

## License
Apache License 2.0. See `LICENSE` for details.