# Aegis System Architecture

## Overview
Aegis is an industry-agnostic, self-healing distributed systems platform.
It observes a live microservice topology, predicts failures before they happen, identifies root causes, predicts how failures propagate, and executes dependency-safe recovery plans.

## Pipeline Lifecycle
```
Telemetry Streams (Kafka)
   │
   ▼
Build System Graph (NetworkX)
   │
   ▼
Graph Preprocessing (BFS/DFS, Centrality, SCC)
   │
   ▼
Graph Representation + Features
   │
   ▼
TGNN (Temporal Graph Neural Network)
   │
   ├── Failure Probability Head
   ├── Root Cause Analysis Head
   └── Propagation Path Prediction Head
   │
   ▼
Decision Engine (Risk Assessment + Rulebook)
   │
   ▼
Recovery Planner (Topological Sort on SCC-condensed Subgraph)
   │
   ▼
Recovery Actions Execution (Go Recovery Engine + Orchestrator Adapter)
```

## Graph Condensation Principle
Real-world microservice topologies frequently exhibit cyclic dependencies (e.g. retry loops, mutual status checks). Standard topological sort requires a Directed Acyclic Graph (DAG) and fails on cycles.

Aegis solves this by running Tarjan's Strongly Connected Components (SCC) algorithm to collapse cyclic components into single super-nodes. Topological sort is then executed on the condensed DAG, guaranteeing deterministic, deadlock-free recovery execution order.
