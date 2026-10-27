"""Offline incident-aware detection and metric-only RCA comparison for RCAEval RE1."""
from __future__ import annotations

import argparse
import csv
import json
import math
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
import sys

import numpy as np
from sklearn.ensemble import IsolationForest, RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.metrics import precision_recall_fscore_support
from sklearn.model_selection import GroupKFold, LeaveOneGroupOut
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from detection.service_detector import ServiceDetector

WINDOW_SECONDS = 10


def read_case(case: dict, root: Path) -> list[dict]:
    path = Path(case["output"])
    if not path.is_absolute():
        path = root / path
    # Aggregate the full one-second captures into fixed ten-second service windows.
    # Bins are aligned to inject_time so no feature window straddles the label boundary.
    inject = float(case["inject_time"])
    values: dict[tuple[int, str], dict[str, list[float]]] = defaultdict(dict)
    with path.open(newline="", encoding="utf-8") as f:
        for row in csv.DictReader(f):
            try:
                value = float(row["value"])
                if math.isfinite(value):
                    timestamp = float(row["timestamp"])
                    bucket = math.floor((timestamp - inject) / WINDOW_SECONDS)
                    metric = values[(bucket, row["service_id"])].setdefault(row["metric"], [0.0, 0.0])
                    metric[0] += value
                    metric[1] += 1
            except (ValueError, KeyError):
                pass
    rows = []
    for (bucket, service), metrics in sorted(values.items()):
        timestamp = inject + bucket * WINDOW_SECONDS + WINDOW_SECONDS / 2
        rows.append({"case_id": case["case_id"], "system": case["system"], "service": service,
                     "timestamp": timestamp, "y": int(bucket >= 0),
                     "root": case.get("root_cause_service"),
                     "metrics": {name: total / count for name, (total, count) in metrics.items() if count}})
    return rows


def feature_matrix(rows: list[dict], columns: list[str] | None = None):
    if columns is None:
        columns = sorted({f"{m}" for row in rows for m in row["metrics"]})
    # Missingness is retained as explicit binary indicators; NaN is imputed by fold-local medians.
    x = np.full((len(rows), len(columns) * 2), np.nan, dtype=float)
    for i, row in enumerate(rows):
        for j, name in enumerate(columns):
            if name in row["metrics"]:
                x[i, j] = row["metrics"][name]
                x[i, len(columns) + j] = 0.0
            else:
                x[i, len(columns) + j] = 1.0
    return x, columns


def detector_predictions(rows: list[dict]) -> tuple[np.ndarray, np.ndarray]:
    pred = np.zeros(len(rows), dtype=int)
    score = np.zeros(len(rows), dtype=float)
    by_case: dict[str, list[int]] = defaultdict(list)
    for i, row in enumerate(rows):
        by_case[row["case_id"]].append(i)
    for indices in by_case.values():
        detector = ServiceDetector()
        for i in sorted(indices, key=lambda j: (rows[j]["timestamp"], rows[j]["service"])):
            row = rows[i]
            signals = {}
            for name, value in row["metrics"].items():
                if name == "cpu":
                    signals["cpu_usage"] = value
                elif name == "error":
                    signals["error_rate"] = value
                elif name.startswith("latency-") or name == "latency":
                    suffix = name.removeprefix("latency-")
                    signal = {"95": "p95_latency_ms", "99": "p99_latency_ms"}.get(suffix, "latency_ms")
                    signals[signal] = value * 1000.0
            result = detector.observe(row["service"], signals,
                                      observed_at=datetime.fromtimestamp(row["timestamp"], timezone.utc))
            pred[i] = int(bool(result["evidence"]))
            score[i] = float(result["score"])
    return pred, score


def score_rows(model, rows, x, train_idx, test_idx, y):
    train_y = y[train_idx]
    if len(np.unique(train_y)) < 2:
        return None
    if model == "Isolation Forest":
        normal = x[train_idx[train_y == 0]]
        pipe = make_pipeline(SimpleImputer(strategy="median", add_indicator=False), StandardScaler(),
                             IsolationForest(n_estimators=100, contamination="auto", random_state=17, n_jobs=2))
        pipe.fit(normal)
        raw = pipe[-1].decision_function(pipe[:-1].transform(x[test_idx]))
        return (raw < 0).astype(int), -raw
    if model == "Random Forest":
        estimator = RandomForestClassifier(n_estimators=60, class_weight="balanced", random_state=17,
                                           min_samples_leaf=2, n_jobs=2)
    elif model == "XGBoost":
        from xgboost import XGBClassifier
        positives = max(int(train_y.sum()), 1)
        estimator = XGBClassifier(n_estimators=60, max_depth=4, learning_rate=.05,
                                  subsample=.9, colsample_bytree=.9, reg_lambda=1,
                                  scale_pos_weight=(len(train_y) - positives) / positives,
                                  eval_metric="logloss", random_state=17, n_jobs=2)
    else:
        raise ValueError(model)
    pipe = make_pipeline(SimpleImputer(strategy="median", add_indicator=False), estimator)
    pipe.fit(x[train_idx], train_y)
    probability = pipe.predict_proba(x[test_idx])[:, 1]
    return (probability >= .5).astype(int), probability


def rca_hits(rows, indices, pred, score):
    by_case: dict[str, dict[str, list[tuple[float, float]]]] = defaultdict(lambda: defaultdict(list))
    root: dict[str, str | None] = {}
    for local, original in enumerate(indices):
        row = rows[original]
        root[row["case_id"]] = row["root"]
        if pred[local]:
            by_case[row["case_id"]][row["service"]].append((row["timestamp"], float(score[local])))
    output = []
    cases_in_fold = {rows[index]["case_id"] for index in indices}
    for case_id in cases_in_fold:
        service_hits = by_case.get(case_id, {})
        truth = root.get(case_id)
        if not truth:
            continue
        ranked = sorted(service_hits, key=lambda s: (min(t for t, _ in service_hits[s]),
                                                       -max(v for _, v in service_hits[s]), s))
        output.append((int(bool(ranked) and ranked[0] == truth), int(truth in ranked[:3])))
    return output


def metrics(y, pred):
    p, r, f, _ = precision_recall_fscore_support(y, pred, average="binary", zero_division=0)
    return {"precision": float(p), "recall": float(r), "f1": float(f)}


def evaluate(manifest_path: Path, folds: int = 5):
    manifest_path = Path(manifest_path)
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    all_cases = manifest["cases"]
    cases = [c for c in all_cases if c.get("root_cause_service")]
    rows = [r for c in cases for r in read_case(c, manifest_path.parent)]
    print(f"Loaded {len(rows)} service windows from {len(cases)} incidents", flush=True)
    x, _ = feature_matrix(rows)
    y = np.array([r["y"] for r in rows], dtype=int)
    groups = np.array([r["case_id"] for r in rows])
    systems = np.array([r["system"] for r in rows])
    unique_cases = np.unique(groups)
    splitters = [("case_group_kfold", GroupKFold(n_splits=min(folds, len(unique_cases))).split(x, y, groups))]
    if len(np.unique(systems)) > 1:
        splitters.append(("leave_one_system_out", LeaveOneGroupOut().split(x, y, systems)))
    model_names = ["ServiceDetector", "Isolation Forest", "Random Forest", "XGBoost"]
    result = {"data": {"cases": len(cases), "windows": len(rows), "systems": sorted(set(systems)),
                        "label_rule": "timestamp >= inject_time is faulty; earlier windows are normal",
                        "features": f"mean numeric metrics per {WINDOW_SECONDS}-second service window plus explicit per-metric missingness indicators; fold-local median imputation for estimators",
                        "traces_used": False}, "protocols": {}}
    for protocol, iterator in splitters:
        fold_results = {model: [] for model in model_names}
        for fold, (train, test) in enumerate(iterator, 1):
            print(f"{protocol}: fold {fold}", flush=True)
            det_pred, det_score = detector_predictions([rows[i] for i in test])
            # detector_predictions indexes a test-only row list and resets histories per case.
            fold_results["ServiceDetector"].append((metrics(y[test], det_pred), rca_hits(rows, test, det_pred, det_score)))
            for name in model_names[1:]:
                print(f"{protocol}: fold {fold}, {name}", flush=True)
                try:
                    scored = score_rows(name, rows, x, train, test, y)
                except ImportError as exc:
                    raise RuntimeError("XGBoost is required: install xgboost before running this comparison") from exc
                if scored is None:
                    continue
                pred, score = scored
                fold_results[name].append((metrics(y[test], pred), rca_hits(rows, test, pred, score)))
        protocol_results = {}
        for name, folds_data in fold_results.items():
            if not folds_data:
                continue
            summary = {}
            for key in ("precision", "recall", "f1"):
                vals = [item[0][key] for item in folds_data]
                summary[key] = {"mean": float(np.mean(vals)), "std": float(np.std(vals, ddof=1)) if len(vals) > 1 else 0.0}
            rca = [hit for _, hits in folds_data for hit in hits]
            summary["rca_cases"] = len(rca)
            for k, index in (("top1", 0), ("top3", 1)):
                fold_rates = [float(np.mean([hit[index] for hit in hits])) for _, hits in folds_data if hits]
                summary[k] = {"mean": float(np.mean(fold_rates)) if fold_rates else None,
                              "std": float(np.std(fold_rates, ddof=1)) if len(fold_rates) > 1 else 0.0 if fold_rates else None}
            protocol_results[name] = summary
        result["protocols"][protocol] = protocol_results
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path, nargs="+")
    parser.add_argument("--folds", type=int, default=3)
    parser.add_argument("--output", type=Path, default=Path("docs/results.md"))
    args = parser.parse_args()
    cases = []
    for manifest_path in args.manifest:
        part = json.loads(manifest_path.read_text(encoding="utf-8"))
        for case in part["cases"]:
            case = dict(case)
            output = Path(case["output"])
            case["output"] = str((manifest_path.parent / output).resolve())
            cases.append(case)
    combined = args.output.parent / ".rcaeval_re1_combined_manifest.json"
    combined.parent.mkdir(parents=True, exist_ok=True)
    combined.write_text(json.dumps({"cases": cases}, indent=2), encoding="utf-8")
    try:
        result = evaluate(combined, args.folds)
    finally:
        combined.unlink(missing_ok=True)
    def fmt(v): return f"{v['mean']:.3f} ± {v['std']:.3f}" if v['mean'] is not None else "n/a"
    lines = ["# RCAEval RE1 offline comparison", "",
             f"Cases: {result['data']['cases']}; windows: {result['data']['windows']}; systems: {', '.join(result['data']['systems'])}.",
             "Labels: timestamp >= inject_time is faulty; earlier windows are normal. Metrics only; traces are not used.",
             "Detection precision, recall and F1 are fold means ± sample standard deviation. RCA top-k is the fraction of labeled cases whose root_cause_service ranks in the first k services.", ""]
    for protocol, models in result["protocols"].items():
        lines += [f"## {protocol.replace('_', ' ')}", "", "| Model | Precision | Recall | F1 | RCA top-1 | RCA top-3 | Labeled RCA cases |", "|---|---:|---:|---:|---:|---:|---:|"]
        for model, m in models.items():
            lines.append(f"| {model} | {fmt(m['precision'])} | {fmt(m['recall'])} | {fmt(m['f1'])} | {fmt(m['top1'])} | {fmt(m['top3'])} | {m['rca_cases']} |")
        lines.append("")
    lines += ["## Method and limitations", "", f"A case is the grouping unit: all windows from one case stay in one fold. Metric samples are averaged into fixed {WINDOW_SECONDS}-second service windows, with bins aligned to the injection boundary. Leave-one-system-out evaluates OB, SS and TT separately when each is held out. Features are per-service metric windows with missing-value indicators; model imputation is fit on training data only. Isolation Forest is fit only on normal training windows. Random Forest uses balanced class weights; XGBoost uses a training-fold class ratio.",
              "RCA ranking uses metric anomaly onset (first predicted anomalous service window), then that service's maximum anomaly score as a tie-break. This is a metric-only heuristic, does not use traces or topology, and is not a causal claim. Labels derive from the injection timestamp and do not establish exact per-service fault onset. Results are limited by the small RE1 case set and cannot establish generalization beyond these captures.", ""]
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text("\n".join(lines), encoding="utf-8")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
