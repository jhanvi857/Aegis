# Aegis architecture and working rules

Aegis is an application-agnostic resilience control plane. Its production path uses OpenTelemetry ingestion, service-level threshold and rolling statistical detection, a directed dependency graph, conventional graph algorithms, evidence-based RCA, and policy-gated recovery. Keep demo applications optional.

## Contracts

- Use `service.name` as the OpenTelemetry service identity. Preserve resource attributes, timestamps, signal names, and missing-data semantics.
- Do not convert an absent measurement to zero or mark a configured service healthy without fresh evidence.
- Directed edge convention: `caller -> dependency` for RPC calls. Record provenance and freshness; trace sightings are evidence, not permanent edges.
- Separate anomaly detection from RCA. Explain candidate ranking, alternatives, temporal and trace evidence, and uncertainty. Do not claim causality from topology alone.
- BFS reports reachable potential impact. Tarjan SCC reports cycles. Topological ordering runs on the SCC condensation and must explicitly handle cycles. Keep DFS useful and tested.
- Recovery recommendations pass policy validation and approval before execution. Record attempts separately from verified recovery. Never synthesize execution success.
- Do not add neural graph models or domain-specific assumptions to core logic.
- Put cross-language contract changes in `proto/` first and regenerate stubs with `scripts/gen_proto.*`.

## Main modules

- `go-services/api-gateway`: authenticated control plane, OTLP intake, service registry, topology and recovery APIs.
- `go-services/recovery-engine`: authorized execution and approval controls.
- `python-ml/detection`: shared service-level baseline detector.
- `python-ml/graph`: observed/configured topology, BFS/DFS, Tarjan SCC, condensation, and recovery ordering.
- `python-ml/decision_engine` and `recovery_planner`: explainable diagnosis and recommendations.
- `frontend`: backend-backed integration, topology, incident, and recovery views. Empty or stale states must remain explicit.
- `examples/` and `deploy/`: optional application fixtures only.

## Validation rules

- Add tests for graph algorithms, missing/invalid telemetry, cold start, uncertainty, policy gates, and recovery verification.
- Do not claim performance or dataset validity without reproducible incident-aware evaluation.
- Preserve existing work in the shared working tree. Avoid changing unrelated files.
