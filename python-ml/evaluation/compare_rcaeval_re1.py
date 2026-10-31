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

import joblib
import numpy as np
from sklearn.ensemble import IsolationForest, RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.metrics import average_precision_score, precision_recall_fscore_support
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
        rows.append({
            "case_id": case["case_id"],
            "system": case["system"],
            "service": service,
            "timestamp": timestamp,
            "bucket": bucket,
            "root": case.get("root_cause_service"),
            "metrics": {name: total / count for name, (total, count) in metrics.items() if count},
        })
    return rows


def get_cached_raw_rows(cases: list[dict], root: Path, cache_path: Path, force_refresh: bool = False) -> list[dict]:
    if not force_refresh and cache_path.is_file():
        try:
            return joblib.load(cache_path)
        except Exception as exc:
            print(f"Warning: could not load cache from {cache_path}: {exc}. Rebuilding...", flush=True)

    rows = []
    for c in cases:
        rows.extend(read_case(c, root))
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        joblib.dump(rows, cache_path, compress=3)
    except Exception as exc:
        print(f"Warning: could not write cache to {cache_path}: {exc}", flush=True)
    return rows


def filter_rows_by_labels(raw_rows: list[dict], label_mode: str) -> list[dict]:
    output = []
    for r in raw_rows:
        bucket = r["bucket"]
        service = r["service"]
        root = r["root"]
        if label_mode == "strict":
            if bucket < 0:
                item = dict(r)
                item["y"] = 0
                output.append(item)
            elif bucket >= 0 and service == root:
                item = dict(r)
                item["y"] = 1
                output.append(item)
            # Other services post-inject are excluded from strict mode
        elif label_mode == "original":
            item = dict(r)
            item["y"] = int(bucket >= 0)
            output.append(item)
        else:
            raise ValueError(f"Unknown label mode: {label_mode}")
    return output


def feature_matrix(rows: list[dict], columns: list[str] | None = None):
    if columns is None:
        columns = sorted({f"{m}" for row in rows for m in row["metrics"]})
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
            result = detector.observe(
                row["service"],
                signals,
                observed_at=datetime.fromtimestamp(row["timestamp"], timezone.utc),
            )
            pred[i] = int(bool(result["evidence"]))
            score[i] = float(result["score"])
    return pred, score


def score_rows(model: str, rows: list[dict], x: np.ndarray, train_idx: np.ndarray, test_idx: np.ndarray, y: np.ndarray):
    train_y = y[train_idx]
    if len(np.unique(train_y)) < 2:
        return None
    if model == "Isolation Forest":
        normal = x[train_idx[train_y == 0]]
        pipe = make_pipeline(
            SimpleImputer(strategy="median", add_indicator=False),
            StandardScaler(),
            IsolationForest(n_estimators=100, contamination="auto", random_state=17, n_jobs=2),
        )
        pipe.fit(normal)
        raw = pipe[-1].decision_function(pipe[:-1].transform(x[test_idx]))
        return (raw < 0).astype(int), -raw
    if model == "Random Forest":
        estimator = RandomForestClassifier(
            n_estimators=60, class_weight="balanced", random_state=17, min_samples_leaf=2, n_jobs=2
        )
    elif model == "XGBoost":
        from xgboost import XGBClassifier
        positives = max(int(train_y.sum()), 1)
        estimator = XGBClassifier(
            n_estimators=60,
            max_depth=4,
            learning_rate=0.05,
            subsample=0.9,
            colsample_bytree=0.9,
            reg_lambda=1,
            scale_pos_weight=(len(train_y) - positives) / positives,
            eval_metric="logloss",
            random_state=17,
            n_jobs=2,
        )
    else:
        raise ValueError(model)
    pipe = make_pipeline(SimpleImputer(strategy="median", add_indicator=False), estimator)
    pipe.fit(x[train_idx], train_y)
    probability = pipe.predict_proba(x[test_idx])[:, 1]
    return (probability >= 0.5).astype(int), probability


def rca_hits(rows: list[dict], indices: np.ndarray, pred: np.ndarray, score: np.ndarray):
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
        ranked = sorted(
            service_hits,
            key=lambda s: (min(t for t, _ in service_hits[s]), -max(v for _, v in service_hits[s]), s),
        )
        output.append((int(bool(ranked) and ranked[0] == truth), int(truth in ranked[:3])))
    return output


def compute_metrics(y: np.ndarray, pred: np.ndarray, score: np.ndarray):
    p, r, f, _ = precision_recall_fscore_support(y, pred, average="binary", zero_division=0)
    pr_auc = float(average_precision_score(y, score)) if len(np.unique(y)) > 1 else 0.0
    return {"precision": float(p), "recall": float(r), "f1": float(f), "pr_auc": pr_auc}


def evaluate_label_mode(rows: list[dict], label_mode: str, folds: int = 3):
    x, _ = feature_matrix(rows)
    y = np.array([r["y"] for r in rows], dtype=int)
    groups = np.array([r["case_id"] for r in rows])
    systems = np.array([r["system"] for r in rows])
    unique_cases = np.unique(groups)

    splitters = []
    # Headline is leave_one_system_out; case_group_kfold is secondary
    if len(np.unique(systems)) > 1:
        splitters.append(("leave_one_system_out", list(LeaveOneGroupOut().split(x, y, systems))))
    splitters.append(("case_group_kfold", list(GroupKFold(n_splits=min(folds, len(unique_cases))).split(x, y, groups))))

    model_names = ["ServiceDetector", "Isolation Forest", "Random Forest", "XGBoost"]
    result = {
        "label_mode": label_mode,
        "cases": len(unique_cases),
        "windows": len(rows),
        "systems": sorted(set(systems)),
        "protocols": {},
    }

    for protocol, iterator in splitters:
        fold_results = {model: [] for model in model_names}
        for fold, (train, test) in enumerate(iterator, 1):
            print(f"[{label_mode}] {protocol}: fold {fold}/{len(iterator)}", flush=True)
            det_pred, det_score = detector_predictions([rows[i] for i in test])
            fold_results["ServiceDetector"].append((
                compute_metrics(y[test], det_pred, det_score),
                rca_hits(rows, test, det_pred, det_score),
            ))
            for name in model_names[1:]:
                print(f"[{label_mode}] {protocol}: fold {fold}/{len(iterator)}, {name}", flush=True)
                scored = score_rows(name, rows, x, train, test, y)
                if scored is None:
                    continue
                pred, score = scored
                fold_results[name].append((
                    compute_metrics(y[test], pred, score),
                    rca_hits(rows, test, pred, score),
                ))

        protocol_results = {}
        for name, folds_data in fold_results.items():
            if not folds_data:
                continue
            summary = {}
            for key in ("precision", "recall", "f1", "pr_auc"):
                vals = [item[0][key] for item in folds_data]
                summary[key] = {
                    "mean": float(np.mean(vals)),
                    "std": float(np.std(vals, ddof=1)) if len(vals) > 1 else 0.0,
                }
            rca = [hit for _, hits in folds_data for hit in hits]
            summary["rca_cases"] = len(rca)
            for k, index in (("top1", 0), ("top3", 1)):
                fold_rates = [float(np.mean([hit[index] for hit in hits])) for _, hits in folds_data if hits]
                summary[k] = {
                    "mean": float(np.mean(fold_rates)) if fold_rates else None,
                    "std": float(np.std(fold_rates, ddof=1)) if len(fold_rates) > 1 else 0.0 if fold_rates else None,
                }
            protocol_results[name] = summary
        result["protocols"][protocol] = protocol_results

    return result


def make_selection_decision(strict_results: dict) -> dict:
    loso = strict_results["protocols"].get("leave_one_system_out", {})
    if not loso:
        raise ValueError("Cannot perform model selection without leave_one_system_out protocol results.")

    detector = loso.get("ServiceDetector")
    if not detector:
        raise ValueError("ServiceDetector results missing from leave_one_system_out protocol.")

    det_f1_mean = detector["f1"]["mean"]
    det_f1_std = detector["f1"]["std"]
    det_threshold = det_f1_mean + det_f1_std

    trained_models = ["Isolation Forest", "Random Forest", "XGBoost"]
    model_simplicity = {"Isolation Forest": 3, "Random Forest": 2, "XGBoost": 1}

    ranked_trained = []
    for model_name in trained_models:
        if model_name in loso:
            m = loso[model_name]
            f1_mean = m["f1"]["mean"]
            top3_mean = m["top3"]["mean"] if m["top3"]["mean"] is not None else 0.0
            simplicity = model_simplicity.get(model_name, 0)
            ranked_trained.append({
                "model": model_name,
                "f1_mean": f1_mean,
                "f1_std": m["f1"]["std"],
                "top3_mean": top3_mean,
                "simplicity": simplicity,
                "summary": m,
            })

    ranked_trained.sort(key=lambda x: (x["f1_mean"], x["top3_mean"], x["simplicity"]), reverse=True)
    best_trained = ranked_trained[0] if ranked_trained else None

    # Clear margin rule: best trained model must beat ServiceDetector by > 1 std of ServiceDetector
    clear_margin = (best_trained is not None) and (best_trained["f1_mean"] > det_threshold)

    if clear_margin:
        selected_model = best_trained["model"]
        decision_reason = (
            f"Selected trained model '{selected_model}' because its strict-LOSO mean F1 ({best_trained['f1_mean']:.3f}) "
            f"exceeded the ServiceDetector baseline ({det_f1_mean:.3f} ± {det_f1_std:.3f}, threshold {det_threshold:.3f}) "
            f"by more than 1 standard deviation."
        )
    else:
        selected_model = "ServiceDetector"
        if best_trained:
            decision_reason = (
                f"Selected 'ServiceDetector' baseline because best trained model '{best_trained['model']}' "
                f"(strict-LOSO mean F1 {best_trained['f1_mean']:.3f}) did NOT beat ServiceDetector "
                f"({det_f1_mean:.3f} ± {det_f1_std:.3f}, required margin > {det_threshold:.3f}) by a clear margin (> 1 std)."
            )
        else:
            decision_reason = "Selected 'ServiceDetector' baseline as no trained models qualified."

    return {
        "selected_model": selected_model,
        "is_baseline": selected_model == "ServiceDetector",
        "detector_f1_mean": det_f1_mean,
        "detector_f1_std": det_f1_std,
        "detector_threshold_required": det_threshold,
        "best_trained_model": best_trained["model"] if best_trained else None,
        "best_trained_f1_mean": best_trained["f1_mean"] if best_trained else None,
        "clear_margin_achieved": clear_margin,
        "decision_reason": decision_reason,
        "selection_rule": (
            "Pick model with best mean strict-LOSO F1; ties broken by RCA top-3, then simpler model. "
            "Selection uses CV results only. If best trained model does not beat ServiceDetector by > 1 std, "
            "keep ServiceDetector as the live engine."
        ),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path, nargs="+")
    parser.add_argument("--folds", type=int, default=3)
    parser.add_argument("--labels", choices=["strict", "original", "both"], default="both")
    parser.add_argument("--output", type=Path, default=Path("docs/results.md"))
    parser.add_argument("--metadata-output", type=Path, default=Path("docs/model_selection.json"))
    parser.add_argument("--cache-file", type=Path, default=Path("dataset/processed/.rcaeval_windows_cache.joblib"))
    parser.add_argument("--force-refresh", action="store_true")
    args = parser.parse_args()

    cases = []
    for manifest_path in args.manifest:
        part = json.loads(manifest_path.read_text(encoding="utf-8"))
        for case in part["cases"]:
            if not case.get("root_cause_service"):
                continue
            case = dict(case)
            output = Path(case["output"])
            case["output"] = str((manifest_path.parent / output).resolve())
            cases.append(case)

    print(f"Loading/caching raw service windows for {len(cases)} cases...", flush=True)
    raw_rows = get_cached_raw_rows(cases, Path("."), args.cache_file, force_refresh=args.force_refresh)
    print(f"Total raw service windows loaded: {len(raw_rows)}", flush=True)

    modes_to_eval = ["strict", "original"] if args.labels == "both" else [args.labels]
    all_evaluations = {}

    for mode in modes_to_eval:
        print(f"\n=== Evaluating label mode: {mode} ===", flush=True)
        filtered = filter_rows_by_labels(raw_rows, mode)
        print(f"Windows in {mode} mode: {len(filtered)}", flush=True)
        all_evaluations[mode] = evaluate_label_mode(filtered, mode, folds=args.folds)

    selection_metadata = None
    if "strict" in all_evaluations:
        selection_metadata = make_selection_decision(all_evaluations["strict"])

    # Format Markdown Report
    def fmt(v):
        if not v or v.get("mean") is None:
            return "n/a"
        return f"{v['mean']:.3f} ± {v['std']:.3f}"

    lines = [
        "# RCAEval RE1 Offline Comparison and Model Selection",
        "",
        "## SELECTION RULE",
        "> " + (selection_metadata["selection_rule"] if selection_metadata else "Evaluation rule"),
        "",
    ]

    if selection_metadata:
        lines += [
            "## SELECTION DECISION",
            f"**Selected Engine**: `{selection_metadata['selected_model']}`",
            "",
            f"- **Baseline ServiceDetector F1 (strict LOSO)**: {selection_metadata['detector_f1_mean']:.3f} ± {selection_metadata['detector_f1_std']:.3f}",
            f"- **Threshold for Trained Model (F1 > +1 std)**: {selection_metadata['detector_threshold_required']:.3f}",
            f"- **Best Trained Model**: `{selection_metadata['best_trained_model']}` (F1: {selection_metadata['best_trained_f1_mean']:.3f})" if selection_metadata['best_trained_model'] else "",
            f"- **Clear Margin Achieved**: {selection_metadata['clear_margin_achieved']}",
            f"- **Decision Summary**: {selection_metadata['decision_reason']}",
            "",
        ]

    for mode in (["strict", "original"] if "strict" in all_evaluations and "original" in all_evaluations else modes_to_eval):
        res = all_evaluations[mode]
        is_headline = (mode == "strict")
        header_tag = " (Headline)" if is_headline else ""
        lines += [
            f"## {mode.capitalize()} Labels{header_tag}",
            "",
            f"- Cases: {res['cases']}, Windows: {res['windows']}, Systems: {', '.join(res['systems'])}",
            f"- Label definition: {'Only root_cause_service post-inject windows are positive; other services post-inject excluded' if mode == 'strict' else 'All service windows post-inject are positive'}.",
            "",
        ]
        # Order protocols: LOSO headline first, then case_group_kfold
        proto_order = [p for p in ["leave_one_system_out", "case_group_kfold"] if p in res["protocols"]]
        for proto in proto_order:
            models = res["protocols"][proto]
            lines += [
                f"### {proto.replace('_', ' ').title()}",
                "",
                "| Model | Precision | Recall | F1 | PR-AUC | RCA top-1 | RCA top-3 | Labeled Cases |",
                "|---|---:|---:|---:|---:|---:|---:|---:|",
            ]
            for model_name, m in models.items():
                lines.append(
                    f"| {model_name} | {fmt(m['precision'])} | {fmt(m['recall'])} | {fmt(m['f1'])} | "
                    f"{fmt(m['pr_auc'])} | {fmt(m['top1'])} | {fmt(m['top3'])} | {m['rca_cases']} |"
                )
            lines.append("")

    lines += [
        "## Methodology and Honest Evaluation",
        "",
        "- **Grouping**: Leave-One-System-Out (LOSO) leaves out an entire microservice system (OB, SS, or TT) for testing, trained only on the other two. GroupKFold groups by incident case.",
        "- **Strict Labels**: Eliminates ambiguous symptom propagation windows from training and testing, focusing strictly on identifying the genuine root-cause failure window.",
        "- **RCA Ranking**: Evaluates metric anomaly onset (first predicted anomalous service window) and breaks ties with peak anomaly score.",
        "- **Fallback Policy**: If no trained classifier beats ServiceDetector by a clear margin (> 1 std), the system strictly retains ServiceDetector for live operations.",
        "",
    ]

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text("\n".join(lines), encoding="utf-8")
    print(f"\nWritten comparison results to {args.output}", flush=True)

    metadata_payload = {
        "timestamp_utc": datetime.now(timezone.utc).isoformat(),
        "selection": selection_metadata,
        "evaluations": all_evaluations,
    }
    args.metadata_output.parent.mkdir(parents=True, exist_ok=True)
    args.metadata_output.write_text(json.dumps(metadata_payload, indent=2), encoding="utf-8")
    print(f"Written metadata JSON to {args.metadata_output}", flush=True)


if __name__ == "__main__":
    main()
