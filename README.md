# Aegis

Aegis observes application telemetry, builds a directed service dependency graph, detects service-level anomalies, explains likely incident origins with available evidence, and prepares policy-checked recovery recommendations.

## Current architecture

- The Go control plane exposes the API, service registry, OTLP HTTP intake, topology storage, authorization, and recovery policy endpoints.
- Applications can export OpenTelemetry traces, metrics, and logs. `service.name` is the service identity. The optional Prometheus pull worker supports configured/discovered scrape targets.
- Trace spans can suggest caller-to-dependency edges. Configured edges are kept distinct from observed edges and should be reviewed as topology evidence, not proof of permanent dependency.
- The Python service uses configurable threshold rules and per-service rolling robust baselines. Cold-start observations use explicit thresholds; unavailable metrics remain unavailable.
- RCA is separate from anomaly detection. Candidate ranking includes signal evidence and explicitly remains uncertain without useful temporal or trace evidence.
- Trace-based RCA is implemented, not validated: RE2-OB trace payloads are not available in the local dataset. No RCA accuracy number is claimed; the RE1 evaluator measures incident detection only.
- BFS reports topology reachability as potential impact. Tarjan SCC and SCC-aware topological ordering support cycle findings and recovery planning.
- Recovery execution stays in the authorized Go recovery engine. A recommendation is not an approval, attempt, or verified recovery.

## Connect an application

See [docs/connect.md](docs/connect.md) for the OpenTelemetry and Prometheus integration contract and example exporter configuration. An application does not need Aegis source changes or a manually maintained service list. Demo workloads are optional fixtures.

Detection thresholds and rolling-baseline settings are configured through `AEGIS_DETECTION_CONFIG_FILE`; the supplied example is mounted into the Python service by Compose. The offline RCAEval RE1 preparation and detection-only evaluation commands are documented in [docs/datasets.md](docs/datasets.md).

## Run locally

Copy `.env.example` to `.env`, set unique credentials and secrets, then start the core stack:

```powershell
docker compose --profile core up --build
```

The control-plane API is available on port `8000`; OTLP HTTP intake is mapped to port `4318`. The dashboard development server can be started with `npm run theme:dev` from `frontend/`.

## Validation

Python graph, detector, and baseline-engine tests:

```powershell
python -m pytest tests/python-ml/test_service_detector.py tests/python-ml/test_baseline_engine.py tests/python-ml/test_graph_builder.py tests/python-ml/test_graph_algorithms.py tests/python-ml/test_condensation_representation.py -q
```

Go tests are run from `go-services/` with `go test ./...`. A passing unit suite does not establish a production-ready recovery integration; verify the connected runtime and health checks for your environment.

## Known limits

- Rolling baselines are in-process and restart with the Python service. They are not a trained or calibrated failure-probability model.
- Static thresholds and current RCA ranking need evaluation against incident-aware RCAEval splits before any benchmark claim is made.
- RCA candidate rankings are uncalibrated. Trace-based RCA is implemented, not validated against RE2-OB because its trace payloads are absent locally; no RCA accuracy number is claimed.
- Recovery depends on a configured orchestrator adapter, action capabilities, permissions, approval policy, and post-action health verification.
