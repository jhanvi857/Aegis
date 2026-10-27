# Connect any application

Aegis discovers services from telemetry. There is no registration step and no required topology file. The optional topology file is only for display-name, tier, threshold, or probe overrides.

## OpenTelemetry push

Set these variables in an OpenTelemetry SDK or auto-instrumentation agent:

```text
OTEL_EXPORTER_OTLP_ENDPOINT=http://<aegis-host>:4318
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_SERVICE_NAME=<any-name>
OTEL_TRACES_EXPORTER=otlp
OTEL_METRICS_EXPORTER=otlp
OTEL_LOGS_EXPORTER=otlp
OTEL_METRIC_EXPORT_INTERVAL=10000
OTEL_RESOURCE_ATTRIBUTES=service.namespace=<optional>,deployment.environment=<optional>
```

When `AEGIS_INGEST_TOKEN` is configured, also set `OTEL_EXPORTER_OTLP_HEADERS=x-aegis-token=<token>`.
OTLP traces and logs can discover service identity. Traces provide dependency evidence only when the caller relationship is explicit or a parent span in the same batch identifies the client and server services. A sufficient sample of error spans can contribute an error-rate feature; spans do not currently supply latency percentiles or request rate to the service detector. Export those as OTLP metrics when needed. Missing metrics remain missing and are never converted to zero. Resource attributes and source timestamps are retained with service telemetry.

Language examples:

* Java: `java -javaagent:opentelemetry-javaagent.jar -jar app.jar` (traces, metrics, and supported auto-instrumented libraries).
* Node.js: `opentelemetry-instrument node app.js` with `@opentelemetry/auto-instrumentations-node` (traces and selected runtime/HTTP metrics).
* Python: `opentelemetry-instrument python app.py` (traces and instrumented library metrics).
* Go: use the OpenTelemetry Go SDK/exporter (traces and any metrics explicitly registered by the application).
* .NET: use the OpenTelemetry .NET auto-instrumentation environment setup (traces and supported runtime/HTTP metrics).

DB wait requires database pool metrics. Without those metrics, that fault family is not detectable. The same applies to queue depth and cache misses.

## Prometheus pull

Expose a standard `/metrics` endpoint and configure one or more discovery sources:

* `AEGIS_SCRAPE_TARGETS=app-a:8080/metrics,app-b:9090/metrics`
* `AEGIS_SCRAPE_FILE_SD=./targets.yaml` using Prometheus file-sd JSON/YAML
* `AEGIS_PROM_URL=http://prometheus:9090` to discover targets from Prometheus labels
* `AEGIS_CONFIG_FILE=/etc/aegis/connect.yaml` to load a local YAML/JSON target file
* `AEGIS_CONFIG_URL=https://example.invalid/aegis-connect.yaml` to load a remote YAML/JSON target file
* Kubernetes annotations: `prometheus.io/scrape=true`, `prometheus.io/port`, and optional `prometheus.io/path`; `aegis.io/service` is preferred as the service name.
* Docker labels: `aegis.scrape=true` and `aegis.port`, only when the Docker socket is mounted.

Metric names can be mapped in `configs/metric_mapping.yaml` from exact Prometheus metric names to canonical signals (`cpu`, `memory`, `error_rate`, `rps`, `latency`, `queue_depth`, or `db_wait`). Compose mounts this file into the gateway. CPU and memory fractions in 0..1 convert to percent; all other values pass through unchanged, so configure units to match the selected detector rule. Counters/status codes are never converted to rates or ratios implicitly. A failed scrape is `unknown`; an explicit Prometheus `up{...} 0` is `offline`.

The Python detector loads the mounted `AEGIS_DETECTION_CONFIG` file. Global signal thresholds and rolling-baseline settings apply by default; optional `services.<service.name>.signals` entries override thresholds for that exact OTel service identity. Keep this deployment-owned file versioned with the application SLOs. Units must match the selected signal mapping.

An optional connection file can contain:

```yaml
targets:
  - url: http://host.docker.internal:9000/metrics
  - url: http://host.docker.internal:9001/metrics
```

Set either `AEGIS_CONFIG_FILE` or `AEGIS_CONFIG_URL`, then restart the Aegis gateway. This is an optional convenience for Prometheus pull configuration; OTLP services still appear automatically from their first export and do not require a file or registration step.

## Networking

Place Aegis and applications on the same Compose network and use the application container name, not `localhost`. From a container reaching a host service, use `host.docker.internal`. Kubernetes workloads should use service DNS or pod discovery.

## Signals and status

OTLP signals retain their source timestamps. A registered service without a fresh timestamped signal is `unknown`; registration or process reachability alone does not mean healthy. Prometheus status uses the scrape result and `up` metric. Telemetry older than twice the configured scrape/export interval is stale and excluded from current detection. Services past `SERVICE_TTL` are removed. `EDGE_TTL` removes dependency edges that have not been observed recently. Missing measurements stay missing; the dashboard shows unknown or degraded coverage when telemetry is absent or partial.

Recovery dispatch is separate from recovery verification. The UI may report `Verifying` after dispatch; only fresh post-action health evidence can result in `Verified`. Missing post-action evidence remains `Unverified` and is not treated as success.

## RCA validation scope

The live service ranks anomaly candidates with signal evidence, temporal ordering, and available trace/log references. Rankings are uncalibrated and do not establish causality. This checkout does not contain RE2-OB trace payloads, so **trace-based RCA is not validated**. RE1 metric captures support detection evaluation only; they do not validate RCA accuracy.
