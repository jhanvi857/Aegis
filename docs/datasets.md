# Dataset inspection and evaluation

## Local RCAEval contents inspected

The repository contains `dataset/external/rcaeval_sample/cases.parquet` (735 case-metadata rows), four small RE1-OB metric captures, and extracted RE1-SS/RE1-TT CSV captures. The sample manifest columns include `suite`, `system`, `root_cause_service`, `fault`, `inject_time`, `time_start`, `time_end`, `n_timesteps`, `has_traces`, and `has_logs`. These are incident-level metadata labels; `root_cause_service` is a service name and `fault` is a fault category. `inject_time`, `time_start`, and `time_end` are epoch seconds.

An inspected RE1-OB metrics Parquet contains 4,201 rows at one-second resolution, a `time` column, and wide per-service columns grouped by resource (`*_cpu`, `*_mem`), load, latency, and error. The local SS/TT RE1 CSVs are also wide Prometheus-style service/system metrics with one-second Unix timestamps. They do not contain a dependency graph in the metric rows; topology must be sourced separately. The sample case manifest marks the inspected RE1-OB examples as having no traces or logs. It advertises RE2 cases and trace availability in case metadata, but **no RE2-OB trace payloads are present locally**. Trace-based RCA is therefore **not validated**. The extracted full RE1 data here contains SS and TT, not OB.

This inspection supports incident-level detection evaluation on selected RE1 cases after validation of each case's files and mappings. It does not establish that the current service detector is benchmarked. The small checked-in RE1-OB sample is not sufficient evidence for an RCA result, and the local copy is insufficient to validate the requested RE2-OB trace-based RCA path. Do not substitute RE1 metrics or the sample manifest for RE2 trace evidence.

## Evaluation rules

- Keep the user's primary dataset choices: RCAEval RE1 for detection and RE2-OB for trace-based RCA. Keep the optional Kaggle dataset separate; never merge its rows into RCAEval.
- Split by incident/case. Every time window from one injection stays in one split. Also report cross-system generalization separately.
- Use `inject_time` for detection labels only after verifying timestamp units and boundary conventions in the selected case. Use `root_cause_service` as incident-level RCA ground truth only when it maps unambiguously to a service in that case.
- Convert wide columns to service-level observations with a versioned mapping. Preserve units and missingness; do not interpret missing values as zero. Audit counter-versus-gauge semantics before deriving rates.
- Treat topology as unavailable unless it is supplied by a trace or a separately documented system topology. Never infer it from column co-occurrence.
- Report case coverage, missing signals, timestamp gaps, class balance, mappings, and the incident split manifest. Fit statistical baselines and transformations on training cases only.
- The production detector is baseline-first; no model performance is claimed until a reproducible incident-aware evaluation pipeline and held-out results exist.

Offline preparation and evaluation belong under `dataset/` and `python-ml/evaluation/`, separate from production ingestion and serving.

## Reproducible RE1 preparation and detection evaluation

For an extracted RE1 tree, run from the repository root:

```powershell
python dataset/preprocess_rcaeval.py --input dataset/external/rcaeval_re1/SS/RE1-SS --output dataset/processed/rcaeval_re1_ss
python python-ml/evaluation/evaluate_rcaeval_re1.py dataset/processed/rcaeval_re1_ss/manifest.json --split test --output dataset/processed/rcaeval_re1_ss/test_report.json
```

The preprocessor emits long-form service observations, per-signal missingness and invalid-timestamp counts, unmapped columns, duplicate timestamps, timestamp gaps, injection-boundary coverage, metadata, and deterministic case-level train/validation/test assignments. Each incident case is assigned as a unit; none of its windows are split across partitions. Review split counts: small or imbalanced inputs can leave a split empty or underrepresented. The evaluator reports incident detection rate, missed incidents, detection delay, pre-injection alert windows/rate, and missingness. It does not claim per-service classification or RCA accuracy. The captures do not include labeled normal incidents or per-service anomaly windows, so specificity, precision, and per-service classification metrics are unavailable. It maps CPU, error, and latency quantiles only. The inspected RE1-SS/TT latency values are seconds and are converted to milliseconds; confirm units for every other system release before interpreting results. Memory values are not normalized without reliable denominators, and unsupported signals are left out. Neither step infers topology from metric co-occurrence.

These commands produce EDA and detection summaries for the available RE1 captures but do not train a learned production model. Current labels are incident-level injection boundaries, not reliable per-service normal/anomalous windows; they do not establish per-service training labels. The production detector is the shared configurable threshold plus rolling median/MAD baseline; it maintains bounded per-service history, excludes flagged observations from baseline updates, and cold-starts on static thresholds. A test report is an evaluation of that baseline, not a training score. No benchmark metric is claimed until these commands have actually been run and their generated report reviewed. No RCA accuracy is claimed. **Trace-based RCA is not validated because RE2-OB trace payloads are absent from the local data.**
