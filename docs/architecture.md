# Architecture

Aegis is an application-agnostic control plane. It receives OTLP metrics, traces, and logs and can scrape configured Prometheus targets. Services are discovered from `service.name`; topology can be supplemented by explicit configuration.

## Data flow

1. Validate and ingest signals with identity and source timestamps.
2. Update service last-seen state and preserve missing measurements as missing.
3. Build a directed graph using `caller -> dependency` edges. Keep configured and observed provenance separate and expire stale observations.
4. Score service windows with shared threshold rules and rolling robust baselines. Only fresh observed signals enter scoring. Cold start uses configured thresholds; missing signals remain absent.
5. Group temporally close anomalies only when shared trace or direct topology evidence supports correlation. Rank candidates from direct signal and available temporal/trace/log evidence. Return evidence references and explicit uncertainty; rankings are uncalibrated and topology alone does not establish causality.
6. Use BFS for potential reachable impact, Tarjan SCC for cycles, and topological sort on the SCC-condensed graph for dependency-aware planning.
7. Validate recommendations against action capability and policy. Require approval when configured, execute through the Go recovery engine, and record dispatch separately from verification. Only fresh post-action health signals can verify recovery; absent evidence remains unverified.

The Python service returns baseline detections, incidents, candidate rankings, evidence references, uncertainty, and recommendations. It does not execute recovery. The Go recovery engine remains the authorization boundary. A service with no fresh timestamped telemetry remains unknown and is excluded from inference.

## Edge and cycle semantics

An edge `A -> B` means A calls or otherwise depends on B. A failure at B can affect callers of B; outgoing BFS from A describes A's dependencies. Consumers must choose traversal direction according to the question and label the result. Cycles are topology findings, not automatic failures. Recovery ordering operates on SCC components and never pretends a cyclic raw graph has a topological order.

## Current limits

The rolling baseline is process-local. RCA candidate rankings are not calibrated probabilities. Local dataset files do not include RE2-OB trace payloads, so **trace-based RCA is not validated** and no RCA accuracy claim is supported. RE1 preparation and detection evaluation are scripts; no score should be claimed unless the held-out evaluation was run and its report reviewed. End-to-end recovery verification depends on fresh post-action telemetry and the configured orchestrator adapter.
