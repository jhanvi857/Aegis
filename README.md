# Aegis: Self-Healing Distributed Systems Platform

Aegis is an industry-agnostic, self-healing distributed systems platform. It observes microservice topologies, predicts failures before they occur via Temporal Graph Neural Networks (TGNN), pinpoints root causes, predicts cascade propagation, and autonomously executes dependency-safe recovery actions.

```
Detect → Predict → Explain → Fix
```

## Tech Stack
- **Distributed Systems (Playground & Control Plane)**: Go
- **Machine Learning & Graph Intelligence**: Python (PyTorch Geometric, NetworkX)
- **RPC & Message Contract**: Protobuf / gRPC (`/proto`)
- **Streaming Telemetry**: Apache Kafka
- **Orchestration**: Docker & Kubernetes Adapters

## Architecture
See detailed architectural specifications in [`AGENTS.md`](./AGENTS.md) and [`docs/architecture.md`](./docs/architecture.md).

## Phase 0: Quickstart

### 1. Launch Cluster
Start the simulated playground microservice mesh and backing infrastructure:
```bash
docker-compose up --build -d
```

### 2. Verify Services
Check health of each node:
```bash
curl http://localhost:8080/health  # Gateway
curl http://localhost:8081/health  # Node A
curl http://localhost:8082/health  # Node B
curl http://localhost:8083/health  # Node C
curl http://localhost:8084/health  # Node D
```

### 3. Route Inter-Service Request
Send a request into the gateway to observe routing across the topology graph (`gateway` → `node-a` → `node-b`, `node-c` → `node-d`):
```bash
curl -X POST http://localhost:8080/route/node-a
```

### 4. Protobuf Generation
Regenerate Go and Python stubs from `.proto` contracts:
```bash
# On Linux/macOS/Git Bash:
bash scripts/gen_proto.sh

# On Windows PowerShell:
.\scripts\gen_proto.ps1
```

## Directory Structure
```
aegis/
├── proto/                           # Shared Protobuf contracts (telemetry, graph, predict, recovery)
├── go-services/                     # Go microservice mesh & control plane
│   ├── playground/                  # Simulated generic distributed nodes (gateway, node-a..d)
│   ├── telemetry-agent/             # Telemetry collection agent
│   ├── chaos-engine/                # Chaos injection engine
│   ├── recovery-engine/             # Recovery executor & approval gate
│   ├── orchestrator-adapter/        # Docker & K8s adapters
│   ├── api-gateway/                 # Control plane API
│   └── shared/                      # Generated Go proto stubs
├── python-ml/                       # Graph intelligence & ML pipeline
│   ├── ingestion/                   # Kafka telemetry consumer
│   ├── graph/                       # Graph builder, condensation, algorithms (BFS/DFS, centrality, SCC)
│   ├── models/                      # TGNN, Log Transformer, and baselines (Isolation Forest, LSTM)
│   ├── tasks/                       # Failure prediction, root cause, propagation prediction
│   ├── decision_engine/             # Risk assessment & rulebook
│   ├── recovery_planner/            # Topological ordering & plan builder
│   ├── rl_agent/                    # RL policy & env (Phase 6)
│   ├── serving/                     # gRPC ML server
│   └── training/                    # Model training scripts
├── dataset/                         # Raw & processed chaos episodes
├── configs/                         # Topology, model, and rulebook configurations
├── scripts/                         # Automation scripts (gen_proto, run_episode, deploy)
├── docs/                            # Architecture, topology specs, demo script
└── docker-compose.yml               # Complete system deployment
```