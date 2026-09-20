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
2. **Zero-Shot Inductive Topology Generalization:** Trained across 4-, 5-, and 6-node graphs; evaluates zero-shot on an **unseen 7-node diamond graph** (Topology D) with **83.1% Failure F1** and **48.8% Blast-Radius IoU** (non-graph baselines score 0.0%).
3. **Pre-Injection Windowing ($t < t_{\text{inj}}$):** Telemetry is sampled strictly before catastrophic failure threshold breach. The model learns subtle precursor drift rather than performing trivial post-crash disaster detection.
4. **Sequential Graph Preprocessing:** Tarjan's Strongly Connected Components (SCC) condensation and Brandes' Betweenness Centrality complete *before* the TGNN runs, transforming cyclic service dependencies into a strict DAG.
5. **Multi-Task Spatio-Temporal GNN:** Dense batched GATv2 layers (spatial message passing) coupled with GRU recurrence (temporal slope tracking) predicting failure probability, root cause attribution, propagation blast radius, and time-to-failure (TTF).
6. **Closed-Loop Ordered Recovery:** Emits typed `RecoveryPlan` messages over gRPC. Actions are executed by the Go recovery engine in strict topological dependency order with safety approval gating.

---

## Empirical Benchmark Results

Evaluated on 90 held-out test episodes under strictly pre-injection observation windows ($t < t_{\text{inj}}$) with zero discrete status shortcut proxies:

### 1. Standard Benchmark (5 Model Tiers)

| Model Tier | Model Architecture | Failure F1 | Root Cause Top-1 | Root Cause Top-3 | Propagation Blast Radius IoU |
|---|---|:---:|:---:|:---:|:---:|
| **Model 3 (Ours)** | **TGNN (Canonical Seed 42)** | **100.0%** | **100.0%** | **100.0%** | **70.5%** *(5-Seed Mean: 75.1% ± 10.3%)* |
| **Model 2** | **LSTM (Temporal Only Sequence)** | 81.0% | 30.0% | 68.9% | 0.0% *(no graph awareness)* |
| **Model 1** | **Isolation Forest (Tabular ML)** | 53.2% | 17.8% | 23.1% | 0.0% *(no graph awareness)* |
| **Baseline A** | **Heuristic Threshold Rule (Static Alerting)** | 43.2% | 44.4% | 44.4% | 0.0% *(no graph awareness)* |
| **Baseline B** | **Majority Class Baseline (Naive Mode)** | 86.8% | 23.3% | 23.3% | 0.0% *(no graph awareness)* |

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

Aegis includes publication-grade forensic audits documented in [`docs/LEAKAGE_AUDIT_AND_VIVA_DEFENSE.md`](docs/LEAKAGE_AUDIT_AND_VIVA_DEFENSE.md) and [`docs/DATASET_CARD.md`](docs/DATASET_CARD.md):

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

## End-to-End System Architecture

```
[ Playground Microservices ] ──(Traffic & Heartbeats)──> [ Telemetry Agent (Go) ]
           │                                                       │
   [ Chaos Engine (Go) ]                                   [ Kafka Topic ]
   (Injects 10 Fault Types)                            (aegis.telemetry via Protobuf)
           │                                                       │
           ▼                                                       ▼
[ Injected Disturbance ]                             [ Python Ingestion Layer ]
(Latency, CPU, Leaks, etc.)                        (Kafka Consumer deserializes pb)
                                                                   │
                                                                   ▼
                                                     [ Phase 2: Graph Preprocessing ]
                                                     • topology.yaml structure
                                                     • BFS/DFS (reachability)
                                                     • Betweenness & Degree Centrality
                                                     • Tarjan's SCC Condensation
                                                                   │
                                                                   ▼
                                                     [ Phase 3: Spatio-Temporal TGNN ]
                                                     • Input: Graph Rep + Metric History
                                                     • GATv2 (spatial) + GRU (temporal)
                                                     • Heads: Fail F1, RCA, Blast Radius
                                                                   │
                                                                   ▼
                                                     [ Phase 4: Decision Engine ]
                                                     • Risk Assessment (Low/Medium/High)
                                                     • Rulebook Action Mapping
                                                                   │
                                                                   ▼
                                                     [ Recovery Planner (Python) ]
                                                     • Topo-sort on SCC-condensed DAG
                                                     • Emits ordered RecoveryPlan (gRPC)
                                                                   │
                                                                   ▼
                                                     [ Recovery Engine (Go) ]
                                                     • Approval Gate (blocks high risk)
                                                     • Orchestrator Adapter (Docker/K8s)
                                                     • Executes Actions in DAG Order
                                                                   │
                                                                   ▼
                                                     [ Playground Self-Heals ]
                                                     • Post-recovery telemetry verifies SLA
```

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
├── docs/                            # Documentation & Academic Defenses
│   ├── DATASET_CARD.md              # Formal 1-page Dataset Card (precedents, audits)
│   ├── LEAKAGE_AUDIT_AND_VIVA_DEFENSE.md # Comprehensive leakage audit & viva defense guide
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

## Academic Documentation & References

- **Dataset Card:** [`docs/DATASET_CARD.md`](docs/DATASET_CARD.md) — Formal specification detailing academic precedents (**GAIA**, **PodFailPred**, **WOLFFI**), class balance, precursor telemetry overlap, and structural diversity.
- **Leakage Audit & Viva Defense Guide:** [`docs/LEAKAGE_AUDIT_AND_VIVA_DEFENSE.md`](docs/LEAKAGE_AUDIT_AND_VIVA_DEFENSE.md) — Comprehensive guide covering pre-injection windowing, feature proxy removal, hyperparameter search tables, error taxonomy, and examiner defense scripts.
- **Demo Script:** [`docs/demo-script.md`](docs/demo-script.md) — Step-by-step presentation script demonstrating fault injection, real-time TGNN diagnosis, and topological recovery.

---

## License
Apache License 2.0. See `LICENSE` for details.