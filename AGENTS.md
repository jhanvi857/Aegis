# Aegis

> Read this file before doing anything else in this repo. It contains the architecture, folder structure, and phase plan. Don't re-derive the design from scratch — follow what's here, and update this file if a design decision changes.

## What Aegis is

Aegis is a **self-healing distributed systems platform**: it observes a live microservice topology, predicts failures before they happen, identifies root cause, predicts how a failure will propagate, and automatically (or semi-automatically) executes recovery actions.

```
Detect → Predict → Explain → Fix
```

**Critical design constraint: Aegis must be industry-agnostic.** It is not built for e-commerce, streaming, banking, or any specific domain. The entire system operates on a generic dependency graph (`topology.yaml`) — swap that config and Aegis points at a completely different system with zero code changes. Do not hardcode domain assumptions (e.g. "payment service", "order queue") into any core logic — those only appear as *example* instantiations in the demo topology.

## Tech stack

| Layer | Language | Why |
|---|---|---|
| Distributed systems (playground services, chaos engine, telemetry agents, recovery execution) | **Go** | concurrency model fits a simulated microservice mesh + chaos injection well |
| ML / graph analysis / decision engine | **Python** | TGNN, graph algorithms, ML ecosystem |
| Frontend | **React** | dashboard, topology viz, controls |
| Go ↔ Python contract | **gRPC + Protobuf** (`/proto`) | typed, low-latency request/response calls |
| Streaming telemetry | **Kafka** | Go collectors publish, Python pipeline consumes |

## Pipeline architecture (authoritative — do not deviate without updating this doc)

Graph preprocessing runs **sequentially and completes before** the TGNN runs. It is NOT a parallel branch alongside the TGNN — its output (Graph Representation) is a direct input to the TGNN.

```
Telemetry
   │
   ▼
Build System Graph
   │
   ▼
Graph Preprocessing
   │
   ┌──────────────┼──────────────┐
   │              │              │
Connectivity   Criticality   Dependencies
   │              │              │
 BFS/DFS       Centrality        SCC
   │              │              │
   └──────────────┼──────────────┘
                  ▼
          Graph Representation
                  │
       + Metrics / Logs / Traces
                  │
                  ▼
                TGNN
                  │
     ┌────────────┼────────────┐
     ▼            ▼            ▼
  Failure      Root Cause   Propagation
 Prediction     Analysis     Prediction
     │            │            │
     └────────────┼────────────┘
                  ▼
           Decision Engine
                  │
                  ▼
           Recovery Planner
                  │
                  ▼
         Topological / DAG
             Ordering
                  │
                  ▼
          Recovery Actions
```

Notes on the graph algorithms:
- Topological sort runs on the **SCC-condensed graph** (each strongly connected component collapsed to one node) — raw topological sort assumes a DAG and will fail on cycles, which real service graphs have (retries, feedback loops).
- Decide explicitly whether graph recompute (centrality/SCC) is periodic (every N seconds) or incremental (on graph deltas) before scaling this up — don't leave this implicit.

## Folder structure

```
aegis/
├── AGENTS.md                        # this file
├── README.md
├── docker-compose.yml
├── Makefile
├── .env.example
│
├── proto/                           # SHARED CONTRACT — source of truth for Go/Python types.
│   │                                 # Edit these first when adding any new cross-language message.
│   ├── telemetry.proto              # log/metric/trace message shapes
│   ├── graph.proto                  # GraphSnapshot, Node, Edge, GraphRepresentation
│   ├── predict.proto                # TGNN request/response (FailurePrediction, RootCause, Propagation)
│   └── recovery.proto               # RecoveryPlan, RecoveryAction, RiskAssessment
│
├── go-services/
│   ├── playground/                  # simulated distributed system — GENERIC nodes, not domain-named
│   │   ├── gateway/                 # entry point, routes to nodes per topology.yaml
│   │   ├── node-a/                  # generic service scaffold
│   │   ├── node-b/
│   │   ├── node-c/
│   │   ├── node-d/
│   │   └── service-template/        # copy this to add a new node; exposes /health /metrics + traces
│   │
│   ├── telemetry-agent/             # collects logs/metrics/traces from playground services
│   │   ├── collector.go
│   │   ├── kafka_publisher.go       # publishes to Kafka using telemetry.proto schema
│   │   └── main.go
│   │
│   ├── chaos-engine/
│   │   ├── injectors/
│   │   │   ├── latency.go
│   │   │   ├── kill_service.go
│   │   │   ├── cpu_stress.go
│   │   │   ├── memory_leak.go
│   │   │   ├── packet_loss.go
│   │   │   ├── mq_lag.go
│   │   │   ├── cache_down.go
│   │   │   ├── db_lock.go
│   │   │   ├── slow_query.go
│   │   │   └── thread_exhaustion.go
│   │   ├── scenarios/                # composable fault scenarios (yaml)
│   │   ├── scheduler.go              # orchestrates chaos episodes
│   │   └── episode_logger.go         # records what was injected + when, for dataset labeling
│   │
│   ├── recovery-engine/
│   │   ├── grpc_client.go            # receives RecoveryPlan from Python decision layer
│   │   ├── actions/
│   │   │   ├── restart.go
│   │   │   ├── scale.go
│   │   │   ├── reroute_traffic.go
│   │   │   ├── flush_cache.go
│   │   │   └── increase_pool.go
│   │   ├── executor.go               # executes RecoveryPlan steps in order via orchestrator-adapter
│   │   └── approval_gate.go          # blocks high-risk actions pending human approval
│   │
│   ├── orchestrator-adapter/         # decouples Aegis from any one runtime
│   │   ├── docker_adapter.go
│   │   └── k8s_adapter.go
│   │
│   ├── api-gateway/                  # Aegis's own control-plane API (serves the React frontend)
│   │   ├── routes/
│   │   │   ├── topology.go
│   │   │   ├── predict.go            # proxies to Python gRPC server
│   │   │   ├── chaos.go
│   │   │   └── recovery.go
│   │   └── main.go
│   │
│   └── shared/                       # generated proto stubs + common Go types
│       └── pb/                       # protoc-gen-go output lands here
│
├── python-ml/
│   ├── ingestion/
│   │   └── kafka_consumer.py         # consumes telemetry.proto messages from Kafka
│   │
│   ├── graph/
│   │   ├── graph_builder.py          # telemetry → System Graph
│   │   ├── condensation.py           # SCC condensation (feeds topo sort)
│   │   ├── representation.py         # combines connectivity+criticality+dependencies → Graph Representation
│   │   └── algorithms/
│   │       ├── bfs_dfs.py            # connectivity
│   │       ├── centrality.py         # criticality
│   │       ├── scc.py                # dependency grouping
│   │       └── topo_sort.py          # runs on condensed graph — used in recovery planning, not pre-TGNN
│   │
│   ├── models/
│   │   ├── tgnn/                     # PRIMARY model — consumes Graph Representation + telemetry
│   │   ├── log_transformer/          # log line embeddings feed TGNN as node/edge features
│   │   ├── isolation_forest/         # baseline, for comparison against TGNN
│   │   └── lstm/                     # baseline, sequence-only (no graph structure)
│   │
│   ├── tasks/                        # TGNN's three output heads
│   │   ├── failure_prediction.py
│   │   ├── root_cause.py
│   │   └── propagation_prediction.py
│   │
│   ├── decision_engine/
│   │   ├── risk_assessment.py        # scores TGNN output → low/high risk
│   │   └── rulebook.yaml             # condition → action mappings (rule-based, phase 4)
│   │
│   ├── recovery_planner/
│   │   ├── dependency_order.py       # topo sort on SCC-condensed affected subgraph
│   │   └── plan_builder.py           # emits RecoveryPlan (recovery.proto) → sent to Go via gRPC
│   │
│   ├── rl_agent/                     # STRETCH GOAL ONLY (phase 6) — do not build before phase 4 is solid
│   │   ├── env.py
│   │   ├── policy.py
│   │   └── train.py
│   │
│   ├── serving/
│   │   └── grpc_server.py            # exposes Predict / Plan RPCs to Go, implements proto/*.proto interfaces
│   │
│   ├── training/
│   │   ├── train_tgnn.py
│   │   └── evaluate_baselines.py
│   │
│   └── shared/                       # generated Python proto stubs
│       └── pb/
│
├── dataset/
│   ├── raw/                          # raw episode captures from chaos runs (telemetry + labels)
│   ├── processed/                    # graph snapshots + feature tensors, ready for training
│   ├── generator.py                  # turns raw episodes into labeled training examples
│   └── splits/                       # train/val/test
│
├── frontend/                         # React
│   ├── src/
│   │   ├── components/
│   │   │   ├── TopologyGraph/        # live graph viz, reads from api-gateway/routes/topology.go
│   │   │   ├── ChaosControls/        # trigger injectors via chaos.go
│   │   │   ├── PredictionPanel/      # shows failure/root-cause/propagation output
│   │   │   └── RecoveryLog/          # shows executed RecoveryPlan steps + outcomes
│   │   ├── pages/
│   │   └── api/                      # client for api-gateway
│   └── public/
│
├── configs/
│   ├── topology.example.yaml         # sample dependency graph — swap per deployment
│   ├── model_config.yaml
│   └── rulebook.example.yaml
│
├── notebooks/                        # EDA, model iteration — not production code
│
├── scripts/
│   ├── gen_proto.sh                  # regenerates Go + Python stubs from /proto
│   ├── run_episode.sh                # runs one chaos episode end-to-end
│   ├── seed_dataset.sh
│   └── deploy.sh
│
├── tests/
│   ├── go-services/
│   ├── python-ml/
│   └── frontend/
│
└── docs/
    ├── architecture.md               # long-form version of this file's pipeline section
    ├── topology-spec.md              # documents topology.yaml format — this is what makes Aegis domain-agnostic
    └── demo-script.md
```

## Phase plan

### Phase 0 — Skeleton
**Goal:** services exist and talk to each other; nothing intelligent yet.
- `go-services/playground/`: 4-5 generic nodes (`node-a..d`) + `gateway`, each exposing `/health`, `/metrics`, fake load generation
- `docker-compose.yml` wiring playground + Kafka + Redis + Postgres
- `configs/topology.yaml` schema finalized — this is the generalization contract, get it right early
- `proto/` skeleton files created (even if empty message bodies) + `scripts/gen_proto.sh` working for both Go and Python
- **Exit criteria:** `docker-compose up` brings up all playground nodes, gateway can route between them, `topology.yaml` accurately describes the running graph

### Phase 1 — Telemetry + Chaos
**Goal:** inject a fault, observe it in raw telemetry.
- `telemetry-agent/`: collects logs/metrics/traces, publishes to Kafka via `telemetry.proto`
- `chaos-engine/injectors/`: start with 3-4 (latency, kill_service, cpu_stress) — not all 10
- `episode_logger.go`: records injected fault + timestamp + which node, for later dataset labeling
- **Exit criteria:** running `run_episode.sh` injects a fault and you can see it reflected in Kafka telemetry topics

### Phase 2 — Graph Preprocessing (sequential — hard dependency for Phase 3)
**Goal:** produce a correct Graph Representation from live telemetry.
- `python-ml/ingestion/kafka_consumer.py` consuming telemetry
- `graph/graph_builder.py`: telemetry → System Graph (nodes/edges from `topology.yaml` + live health)
- `graph/algorithms/`: BFS/DFS (connectivity), centrality (criticality), SCC (dependency grouping)
- `graph/condensation.py` + `graph/representation.py`: combine into one Graph Representation
- **Exit criteria:** dashboard's `TopologyGraph` component renders a live, structurally-correct graph that reacts to chaos injection. Do NOT start Phase 3 until this is stable — the TGNN's input shape depends on it.

### Phase 3 — TGNN
**Goal:** working failure prediction, root cause, propagation prediction.
- `dataset/generator.py`: run chaos repeatedly (aim for 1000+ episodes) against a *stable* Phase 2, capture labeled examples
- `models/tgnn/`: primary model, input = Graph Representation + metrics/logs/traces
- `models/isolation_forest/`, `models/lstm/`: baselines — train these too, use them to justify TGNN's value in your writeup
- `tasks/failure_prediction.py`, `tasks/root_cause.py`, `tasks/propagation_prediction.py`
- **Exit criteria:** given a live chaos injection, TGNN outputs a failure probability + root cause + propagation set, and beats both baselines on held-out episodes

### Phase 4 — Decision + Recovery Planning (rule-based)
**Goal:** full loop — chaos → predict → decide → order → recover → observe.
- `decision_engine/risk_assessment.py` + `rulebook.yaml` (no RL yet)
- `recovery_planner/dependency_order.py`: topo sort on SCC-condensed affected subgraph
- `recovery_planner/plan_builder.py`: emits `RecoveryPlan` over gRPC
- `go-services/recovery-engine/`: receives plan, executes ordered actions via `orchestrator-adapter`
- **Exit criteria:** injecting a fault results in an automatically executed, correctly-ordered recovery plan, observable end-to-end without manual intervention

### Phase 5 — Dashboard + Demo Polish
**Goal:** demo-able.
- `frontend/`: TopologyGraph, ChaosControls, PredictionPanel, RecoveryLog fully wired to `api-gateway`
- Demo flow: click "inject fault" → watch prediction appear → watch recovery execute live
- `docs/demo-script.md`: written walkthrough for presentation

### Phase 6 — Stretch (only if Phase 0-5 are solid and time remains)
- `rl_agent/`: replace rulebook with a learned policy — env, reward shaping, training loop
- Additional chaos injector types (remaining from the list of 10)
- `approval_gate.go`: human-in-loop review for high-risk actions

## Conventions

- **Never hardcode domain names** (e.g. "payment-service") in core logic — only in example `topology.yaml` instances used for demos.
- **Proto-first:** any new cross-language message goes in `/proto` first, then regenerate stubs with `scripts/gen_proto.sh`. Don't hand-write parallel Go/Python types for the same data.
- **Kafka for streams, gRPC for request/response.** Don't force continuous telemetry through blocking RPC; don't build a queue for a simple predict-and-return call.
- Topological sort always runs on the **SCC-condensed** graph, never the raw graph.
- Don't start Phase *N+1* work assuming Phase *N*'s interface is final — Phase 2's Graph Representation shape is a hard input contract for Phase 3; changing it later means re-touching the TGNN's input layer.