# Architecture audit

## Current data flow

Applications export OTLP/HTTP telemetry to the Go API gateway (`/v1/traces`, `/v1/metrics`, `/v1/logs`). Optional Prometheus discovery and scrape support runs in the gateway. Resource `service.name` creates service identity records; spans with defensible caller/callee evidence update observed dependency edges. The gateway retains recent telemetry and forwards service metrics, traces, and logs to the Python gRPC prediction service. The Python path applies shared per-service threshold and rolling-baseline detection, then correlates anomalies, ranks RCA candidates with evidence/uncertainty, computes graph reachability, and returns recommendations. The gateway exposes topology, detection, incident, and recovery APIs used by the React dashboard. Recovery is separately policy-gated and dispatched to configured execution integrations; post-action telemetry, not command acknowledgement, is required for verification.

## Audit findings

- **Works by implementation inspection:** versioned protobuf contracts, Go API and storage abstractions, authentication and authorization middleware, OTLP/HTTP endpoints, Prometheus scrape configuration, generic service identity, directed service graph types, Python baseline/detector modules, graph algorithms, dashboard API clients, and recovery policy/execution modules.
- **Implemented with limitations:** telemetry auto-discovery is keyed by `service.name`; sparse metric fields preserve explicit zero and missingness; trace-derived dependencies rely on in-batch parent/server-span evidence; observed topology remains time-bounded evidence. Detection uses configurable thresholds and shared rolling baselines, but baseline state is process-local. RCA is a transparent ranking and does not establish causality. Recovery supports only configured capabilities and cannot prove resolution without fresh post-action health evidence.
- **Missing or not demonstrated:** persistent/HA telemetry storage and detector state, broad OTel semantic-convention coverage, a complete authenticated integration validation UI, deployment-level recovery verification against real orchestrators, RE2-OB trace payload evaluation, incident-aware performance results, and end-to-end operation against a user-owned runtime. The checked-in RCAEval sample does not contain the required RE2-OB trace payloads.
- **Refactoring priorities:** keep telemetry normalization and missingness consistent across ingestion and detection; make incident lifecycle and recovery status typed/versioned across the Go/Python/UI boundary; centralize action capability checks so recommendations cannot imply unavailable executors; keep data freshness and provenance visible in APIs and UI.
- **Preserve:** Go control plane/storage/auth, OTLP/Prometheus integrations, Python serving boundary, protobuf contracts, generic topology and UI components, append-only audit interface, and existing graph algorithms. Demo applications remain optional fixtures.

## Implementation sequence

1. Establish the generic OTLP onboarding and truthful service discovery contract.
2. Preserve signal presence, timestamps, provenance, and freshness through detection inputs.
3. Maintain directed graph provenance and implement/test BFS, DFS, Tarjan SCC, and SCC-aware topological ordering.
4. Correlate service anomalies and rank RCA candidates using temporal/trace/log evidence with explicit uncertainty.
5. Enforce action binding, permissions, approval, bounded dispatch, verification, and audit state transitions.
6. Make dashboard states reflect APIs, including empty, stale, unknown, uncertain, and unverified states.
7. Run one consolidated test/build/validation pass and report unverified deployment assumptions separately.

The production inference path is baseline-first. Dataset preparation/evaluation remains separate from production serving, and no dataset performance claim is made without incident-aware held-out evaluation.

## Graph and recovery ordering contract

Edges use `caller -> dependency`. BFS reachability is potential impact only. Tarjan SCC groups expose cycles; a node-level topological order is empty for cyclic input, while the SCC condensation order remains available for inspection. Recovery order reverses caller-to-dependency edges so dependencies precede callers. If the affected subgraph contains a cycle, the planner returns `blocked_topology` with the cycle groups and no sequential actions rather than flattening the cycle into a false order.
