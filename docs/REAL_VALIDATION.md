# ShopKart Real-Telemetry Validation

## Scope and honesty statement

This report records the Phase 2 run against the separate ShopKart repository. The
run collected observed Prometheus samples during strict pre-injection windows. It
did not fabricate telemetry, use service-status fields as model features, or
claim model accuracy where the data contract was not compatible.

The run did **not** complete TGNN fine-tuning or a valid comparison against LSTM,
Isolation Forest, and the heuristic baseline. The current evaluation pipeline
expects the existing Aegis five-node, eight-feature tensor contract, while
ShopKart exposed ten service IDs and seven observed Prometheus metric families.
No invalid cross-topology evaluation was reported.

## Exact environment and commands

ShopKart was run from:

```text
C:\Users\family\OneDrive\Desktop\ShopKart\shopkart
```

The Aegis collector was run from:

```text
C:\Users\family\OneDrive\Desktop\Aegis
```

The following commands were approved before execution:

```powershell
Set-Location 'C:\Users\family\OneDrive\Desktop\ShopKart\shopkart'
docker version
docker compose ps
.\run.ps1 infra
.\run.ps1 backend
Invoke-WebRequest http://localhost:9000/readyz -UseBasicParsing
Invoke-WebRequest http://localhost:9003/readyz -UseBasicParsing
Invoke-WebRequest http://localhost:9091/-/ready -UseBasicParsing
```

Each real pre-injection capture used the fail-closed collector:

```powershell
Set-Location 'C:\Users\family\OneDrive\Desktop\Aegis'
python dataset/live_collector.py --source prometheus `
  --prometheus-url http://localhost:9091 `
  --output dataset/live_episodes/<episode>-pre.json `
  --duration-sec 30 --interval-sec 5
```

The baseline capture used the same collector with `60` seconds and `5` second
polling:

```powershell
python dataset/live_collector.py --source prometheus `
  --prometheus-url http://localhost:9091 `
  --output dataset/live_episodes/shopkart-baseline.json `
  --duration-sec 60 --interval-sec 5
```

The documented baseline load command was:

```powershell
Set-Location 'C:\Users\family\OneDrive\Desktop\ShopKart\shopkart'
.\run.ps1 load-baseline
```

## Observed baseline results

The k6 output reported:

| Measure | Observed result |
|---|---:|
| Completed iterations | 150 |
| Total requests completed | 870 |
| p95 latency | 131.93 ms |
| HTTP error rate | 3.45% |
| k6 threshold | `< 2.0%` HTTP errors |
| Threshold result | **Failed** |

k6 also emitted `thresholds on metrics 'http_req_failed' have been crossed`.
The summary handler emitted a separate output-format error after reporting the
metrics; that is recorded rather than silently ignored.

The baseline artifact contains 433 observed Prometheus samples, is marked
`pre_injection_only=true`, and was written to
`dataset/live_episodes/shopkart-baseline.json`.

## Episode artifacts

Every listed pre-injection artifact contains 222 observed samples from a
30-second window at 5-second polling. The collector did not use its Kafka
simulation fallback.

| Artifact | Attempted fault | Script result | Evidence status |
|---|---|---|---|
| `shopkart-cache_down-pre.json` | Redis cache outage | Existing script stopped and restored `shopkart-redis` | Executed and reverted |
| `shopkart-db_lock-pre.json` | PostgreSQL row lock | Existing script acquired and terminated the lock transaction | Executed and reverted |
| `shopkart-slow_query-pre.json` | 12 slow PostgreSQL queries | Existing script started and terminated 12 queries | Executed and reverted |
| `shopkart-memory_pressure-pre.json` | Payments memory leak | Existing Git Bash script failed with curl exit 7 | Not executed by script; endpoint cleaned up separately |
| `shopkart-crash-pre.json` | Inventory crash | Script found no `shopkart-inventory` container | Not executed; ShopKart backend was native |
| `shopkart-netem-pre.json` | Bank-mock network delay | Script entered fallback, but suppresses curl errors | Not verified as applied |

The memory script's endpoint was verified separately with PowerShell and returned
`HTTP 200 {"status":"leak_enabled"}`; it was then disabled and cleared with
the matching endpoint. This was cleanup/diagnosis, not a replacement claim that
the existing script succeeded.

The crash script and its revert target Docker service containers, while the
approved ShopKart runner starts services natively. The existing script therefore
cannot inject that fault in this deployment mode.

## Telemetry dimensions

The observed ShopKart service IDs were:

```text
cart, catalog, checkout, edge-gateway, external-bank-mock,
inventory, notifications, payments, reviews, search
```

The observed metric families were:

```text
consumer_lag
db_pool_connections_wait_count
go_goroutines
http_requests_in_flight
process_resident_memory_bytes
queue_depth
worker_pool_utilization_ratio
```

The artifacts were captured through the current collector using the default
Aegis topology for graph initialization. That topology has five nodes and does
not describe ShopKart's ten-node dependency graph. The raw Prometheus samples
are therefore retained as evidence, but they are not presented as TGNN input
or as a valid supervised test set.

The CLI invocations did not pass `--fault-node` or `--fault-type`, so the JSON
metadata in these artifacts says `nominal`. The experiment identities in the
table above come from the command sequence and script outcome, not from
telemetry labels. These files must not be used as labeled training data without
an explicit episode manifest and a ShopKart topology/feature adapter.

## Model evaluation status

No current TGNN, LSTM, Isolation Forest, or heuristic metrics are reported for
ShopKart. Running the existing `python-ml/training/evaluate_baselines.py` would
evaluate the pre-existing synthetic train/test split, not these artifacts, and
would violate the requested real-data comparison.

Required next work before a valid benchmark:

1. Add a ShopKart topology adapter or trace-derived graph with the ten service
   IDs and verified dependency edges.
2. Define a documented mapping from ShopKart Prometheus metrics to the model's
   feature contract without using status or fault labels as features.
3. Add an episode manifest containing injection timestamp, fault type, target,
   revert timestamp, and capture boundaries.
4. Capture both nominal and successfully injected episodes with labels, then
   split by episode before evaluating or fine-tuning.
5. Run the TGNN, LSTM, Isolation Forest, and heuristic baseline on that
   compatible dataset and publish failures as well as successes.

## Cleanup

The approved cleanup commands were run:

```powershell
Set-Location 'C:\Users\family\OneDrive\Desktop\ShopKart\shopkart'
.\run.ps1 revert-all
.\run.ps1 down
```

The native backend process was stopped after infrastructure cleanup. No ShopKart
business-logic files were modified.
