# Aegis TGNN — reference architecture

This is a reference implementation you adapt into `python-ml/models/tgnn/`,
not a drop-in final module — it doesn't know your exact Kafka schema or
`topology.yaml` layout, so wire those parts (marked `# TODO`) to your real
`graph_builder.py` / `episode_logger.go` output.

## 1. What defines "an issue" (the label)

Two different things — don't conflate them:

**Training labels — use ground truth, not thresholds.** You control the
chaos engine. `episode_logger.go` already knows the true fault window per
episode (injector, target node, start/end time) and, if you log propagation,
which downstream nodes actually degraded. That's your label source. Don't
invent heuristic thresholds to generate training labels — you'd just be
teaching the model to reproduce your heuristic instead of learning real
failure signatures.

**Label with a horizon, not the injection timestamp.** If "failure" starts
exactly at injection time, you're training the model to predict the past.
Instead, for a window ending at time `t`, label it positive if degradation
occurs in `(t, t + horizon]` (e.g. horizon = 30–60s). This is what makes the
model *predictive* instead of a fancy anomaly detector. See
`build_labels()` in `dataset_builder.py`.

**Runtime detection (no injector ground truth available)** — a baseline to
sanity-check the model against, and useful during early debugging:
- Error rate > 5% over a trailing 30s window
- p95 latency > 3x that service's own rolling 15-min baseline (per-service,
  not a global number — services have very different natural latencies)
- 3+ consecutive failed health checks
- Composite anomaly score: z-score > 3 across **2+ metrics simultaneously**
  (single-metric thresholds are noisy; requiring agreement cuts false
  positives a lot)

## 2. Feature set (per node, per timestep)

Raw metrics, mapped to your 10 injectors so nothing is invisible to the model:

| Injector | Leading metrics |
|---|---|
| latency | p95/p99 latency, latency variance |
| kill_service | health-check failure streak, connection-refused rate |
| cpu_stress | CPU utilization, run-queue length |
| memory_leak | memory usage **slope**, GC frequency/duration |
| packet_loss | retry rate, TCP retransmits |
| mq_lag | consumer lag, queue depth vs drain rate |
| cache_down | cache hit ratio, backend DB query rate |
| db_lock | lock wait time, connection-pool saturation |
| slow_query | query duration percentiles, DB CPU |
| thread_exhaustion | active threads / pool size, rejection rate |

For each raw metric, engineer (see `compute_engineered_features`):
rolling mean/std/EWMA (30s/1min/5min windows), rate of change (slope —
your earliest signal for memory leaks and cascades), and z-score vs that
service's own historical baseline.

Graph-level features come from work you've already built:
`centrality.py` → node criticality score, `scc.py` → cycle membership
(higher risk — failure can loop back), edge features → call rate,
edge latency, edge error rate between service pairs. Edge features matter
because a service can look healthy on its own metrics while actually
failing because its *dependency* is degrading — only the edge/propagation
view catches that.

## 3. Why GAT + GRU, not GCN / Transformer / continuous-time TGN

- **GCN**: aggregates neighbors with fixed, structure-only weights
  (degree normalization). Treats every dependency as equally important —
  wrong here, since a DB dependency matters more than a logging sidecar.
- **GAT / GATv2**: learns per-edge attention — which neighbors actually
  matter, right now. Good fit.
- **Graph Transformer** (full pairwise attention): more expressive, but
  needs far more data than a few dozen chaos episodes provide, and throws
  away the useful inductive bias your dependency edges already encode.
  Overfits on small graphs.
- **Continuous-time TGNN** (TGN, JODIE — memory modules for async events):
  built for huge dynamic graphs (social networks, billions of events).
  Your telemetry is already sampled at a fixed cadence — you don't need
  the extra complexity.

**Chosen: discrete-time snapshots, GAT spatial layer + GRU temporal layer.**
Matches your telemetry's natural sampling interval, right amount of
expressiveness for a small directed graph and limited episode count.

This repo implements GAT as a **custom dense batched layer** (operates on
`[batch, nodes, features]` tensors with an adjacency mask) instead of using
`torch_geometric`'s sparse batching. For a graph with tens of nodes this is
just as fast, far easier to debug, and — importantly for Kaggle — avoids
`torch-geometric`/`torch-scatter` CUDA-version pinning issues, which break
constantly on shared notebook environments. Only base PyTorch is required.

**Edge direction matters.** Your call graph is directed (A calls B).
Failures propagate along call direction; queue/backpressure can propagate
the reverse way. Start with forward-only edges for v1 (`adj_mask[i,j]=1`
if `j` calls `i`, i.e. `j` is upstream of `i`); add a second reverse-edge
type later if backpressure modeling matters.

## 4. Files

- `dataset_builder.py` — turns raw per-episode telemetry into labeled
  snapshot sequences, with an episode-level train/val/test split.
- `tgnn_model.py` — `DenseGATLayer`, `TemporalGATGRU` model, three task
  heads (failure / root-cause / propagation).
- `train_kaggle.py` — training loop with mixed precision, checkpointing,
  and early stopping.

## 5. Dataset construction checklist

1. Run chaos episodes systematically: vary injector type, target node,
   magnitude, duration, and background traffic pattern — traffic shape
   changes what a memory leak looks like under load vs idle.
2. **Split by episode, not by timestep.** Random timestep splits leak
   adjacent windows from the same episode into train and test, and your
   validation metrics will lie to you.
3. Class imbalance is real — healthy windows vastly outnumber pre-failure
   ones. Use weighted `BCEWithLogitsLoss` (`pos_weight`) rather than naive
   oversampling, which just duplicates near-identical windows.
4. Once the pipeline works, consider a generalization test: train on
   single-fault episodes, evaluate on unseen multi-fault combinations —
   tells you whether the model learned propagation dynamics or just
   memorized injector signatures.

## 6. Training on Kaggle (no local GPU)

1. **Debug locally on CPU first**, with a tiny synthetic subset (a few
   episodes, a few epochs). Get tensor shapes and the loss curve sane
   before touching Kaggle's GPU quota (Kaggle GPU is ~30h/week — don't
   burn it on shape bugs).
2. New notebook → Settings → Accelerator → GPU (T4 x2 or P100).
   Since this reference avoids `torch_geometric`, there's nothing extra to
   install — Kaggle's preinstalled `torch` is enough.
3. Upload your generated dataset (snapshot tensors + labels) as a Kaggle
   Dataset, attach it as a notebook input.
4. Train with mixed precision (`torch.cuda.amp`) — included in
   `train_kaggle.py`.
5. Checkpoint every epoch to `/kaggle/working/checkpoints/`. Kaggle
   sessions can idle-timeout; "Save Version" on commit persists
   `/kaggle/working/` as notebook output. To resume, add your last
   checkpoint (as a dataset) as an input to a new session.
6. Start with a short run (2–3 epochs) to confirm loss goes down and
   checkpointing works, before committing to a long run.