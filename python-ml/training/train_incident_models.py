"""Train and fit final incident models on RE1 cases and export model artifact + metadata."""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
from typing import Any

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestClassifier
from sklearn.impute import SimpleImputer
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import LabelEncoder
from xgboost import XGBClassifier

FEATURES = ["cpu_mean", "cpu_max", "latency_ms_mean", "latency_ms_max", "error_rate_mean", "error_rate_max"]
WINDOW_SECONDS = 10
MIN_FEATURE_COVERAGE = 0.5
FAULT_MAP = {
    "cpu": "cpu_stress",
    "mem": "memory_leak",
    "delay": "latency",
    "loss": "packet_loss",
    "disk": "disk_pressure",
}


def _canonical_metric(name: str) -> str | None:
    name = name.lower()
    if name == "cpu":
        return "cpu"
    if name in {"error", "error_rate"}:
        return "error"
    if name.startswith("latency"):
        return "latency"
    return None


def read_manifest(path: Path) -> list[dict[str, Any]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    result = []
    for case in data.get("cases", []):
        item = dict(case)
        output = Path(item["output"])
        item["output"] = output if output.is_absolute() else path.parent / output
        result.append(item)
    return result


def extract_rows_from_cache(raw_rows: list[dict[str, Any]], cases: list[dict[str, Any]], labels_mode: str = "strict"):
    case_fault_map = {c["case_id"]: c.get("fault", "unknown") for c in cases}
    rows = []
    for r in raw_rows:
        bucket = r["bucket"]
        service = r["service"]
        root = r["root"]
        if bucket < 0:
            continue
        if labels_mode == "strict" and service != root:
            # Under strict labels, non-root service windows post-inject are excluded
            continue
        
        metrics = r.get("metrics", {})
        cpu_val = metrics.get("cpu", np.nan)
        err_val = metrics.get("error", np.nan)
        lat_val = np.nan
        for k, v in metrics.items():
            if k.startswith("latency"):
                lat_val = v * 1000.0  # seconds to ms
                break

        row = {
            "cpu_mean": cpu_val,
            "cpu_max": cpu_val,
            "latency_ms_mean": lat_val,
            "latency_ms_max": lat_val,
            "error_rate_mean": (err_val / 100.0) if (err_val is not None and err_val > 1.0) else err_val,
            "error_rate_max": (err_val / 100.0) if (err_val is not None and err_val > 1.0) else err_val,
            "case": f"{r.get('system', '')}::{r['case_id']}",
            "system": str(r.get("system", "")),
            "service": service,
            "is_root": int(bool(root) and service == root),
            "fault": FAULT_MAP.get(str(case_fault_map.get(r["case_id"], "")).lower(), "unknown"),
        }
        rows.append(row)

    if not rows:
        raise ValueError("No matching service windows found in cache")
    x = np.asarray([[row[name] for name in FEATURES] for row in rows], dtype=float)
    root_y = np.asarray([row["is_root"] for row in rows], dtype=int)
    fault_y = np.asarray([row["fault"] for row in rows], dtype=str)
    return x, root_y, fault_y, [row["case"] for row in rows], [row["system"] for row in rows]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifests", nargs="+", type=Path)
    default_model = os.environ.get("AEGIS_MODEL_PATH") or os.environ.get("AEGIS_ML_MODEL_PATH") or "python-ml/artifacts/incident_models.joblib"
    parser.add_argument("--output-model", type=Path, default=Path(default_model))
    parser.add_argument("--output-report", type=Path, default=Path("docs/incident_model_report.json"))
    parser.add_argument("--labels-mode", choices=["strict", "original"], default="strict")
    parser.add_argument("--cache-path", type=Path, default=Path("dataset/processed/.rcaeval_windows_cache.joblib"))
    parser.add_argument("--seed", type=int, default=17)
    args = parser.parse_args()

    cases = [case for path in args.manifests for case in read_manifest(path)]
    systems = sorted(set(str(c.get("system", "")) for c in cases))

    if args.cache_path.is_file():
        print(f"Loading cached service windows from {args.cache_path}...", flush=True)
        raw_rows = joblib.load(args.cache_path)
        x, root_y, fault_y, row_cases, row_systems = extract_rows_from_cache(raw_rows, cases, labels_mode=args.labels_mode)
    else:
        raise FileNotFoundError(f"Cache file {args.cache_path} not found. Run compare_rcaeval_re1.py first.")

    # Load CV metrics from selection report if available
    cv_metrics = {}
    selection_path = Path("docs/model_selection.json")
    if selection_path.is_file():
        try:
            cv_data = json.loads(selection_path.read_text(encoding="utf-8"))
            cv_metrics = cv_data.get("selection", {})
        except Exception:
            pass

    # Fit final models on all cases
    print("Fitting final root-cause model (RandomForestClassifier)...", flush=True)
    root_model = make_pipeline(
        SimpleImputer(strategy="median"),
        RandomForestClassifier(n_estimators=100, min_samples_leaf=2, class_weight="balanced_subsample",
                               random_state=args.seed, n_jobs=-1),
    )
    root_model.fit(x, root_y)

    print("Fitting final fault-family model (RandomForestClassifier)...", flush=True)
    fault_mask = fault_y != "unknown"
    fault_encoder = LabelEncoder()
    fault_y_encoded = fault_encoder.fit_transform(fault_y[fault_mask])
    fault_model = make_pipeline(
        SimpleImputer(strategy="median"),
        RandomForestClassifier(n_estimators=100, min_samples_leaf=2, class_weight="balanced",
                               random_state=args.seed, n_jobs=-1),
    )
    fault_model.fit(x[fault_mask], fault_y_encoded)

    now_iso = datetime.now(timezone.utc).isoformat()
    artifact = {
        "schema_version": 1,
        "model_type": "RandomForest",
        "features": FEATURES,
        "min_feature_coverage": MIN_FEATURE_COVERAGE,
        "training_systems": systems,
        "labels_mode": args.labels_mode,
        "cv_metrics": cv_metrics,
        "date": now_iso,
        "root_model": root_model,
        "root_threshold": 0.5,
        "fault_model": fault_model,
        "fault_classes": list(fault_encoder.classes_),
        "fault_map": FAULT_MAP,
        "feature_units": {"cpu": "percent", "latency": "milliseconds", "error_rate": "ratio"},
        "training_data": f"RCAEval RE1 ({', '.join(systems)}); {args.labels_mode} labels; 375 cases",
        "models": {"root_cause": "Random Forest", "fault_family": "Random Forest"},
    }

    args.output_model.parent.mkdir(parents=True, exist_ok=True)
    joblib.dump(artifact, args.output_model, compress=3)
    print(f"Saved artifact to {args.output_model}", flush=True)

    metadata = {
        "schema_version": "aegis-incident-model-metadata-v1",
        "model_type": "RandomForest",
        "features": FEATURES,
        "min_feature_coverage": MIN_FEATURE_COVERAGE,
        "training_systems": systems,
        "labels_mode": args.labels_mode,
        "cv_metrics": cv_metrics,
        "date": now_iso,
        "window_seconds": WINDOW_SECONDS,
        "cases_count": len(cases),
        "windows_count": len(x),
        "root_threshold": 0.5,
        "fault_classes": list(fault_encoder.classes_),
        "models": {"root_cause": "Random Forest", "fault_family": "Random Forest"},
    }
    args.output_report.parent.mkdir(parents=True, exist_ok=True)
    args.output_report.write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
    print(f"Saved metadata json to {args.output_report}", flush=True)


if __name__ == "__main__":
    main()
