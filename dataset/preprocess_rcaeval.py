"""Prepare local RCAEval RE1 captures for incident-aware baseline evaluation.

Input is an extracted RE1 system tree containing per-incident ``simple_data.csv``
and ``inject_time.txt`` files. Output is long-form service observations, one CSV
per incident, plus an EDA and case-level split manifest. No missing value is
imputed and all windows from an incident remain in one split.
"""
from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
from collections import Counter
from statistics import median
from pathlib import Path
from typing import Dict, Iterable, List, Optional, Tuple

METRIC_SUFFIXES = (
    "latency-50", "latency-90", "latency-95", "latency-99",
    "workload", "error", "cpu", "mem", "memory", "latency",
)
FAULT_SUFFIXES = ("slow_query", "thread_exhaustion", "memory_pressure", "cpu", "delay", "disk", "loss", "mem")


def parse_number(raw: str) -> Optional[float]:
    try:
        value = float(raw)
    except (TypeError, ValueError):
        return None
    return value if math.isfinite(value) else None


def parse_metric_column(column: str) -> Optional[Tuple[str, str]]:
    if column.lower() == "time":
        return None
    for suffix in sorted(METRIC_SUFFIXES, key=len, reverse=True):
        marker = "_" + suffix
        if column.lower().endswith(marker):
            service = column[:-len(marker)].strip()
            return (service, suffix) if service else None
    return None


def case_labels(case_dir: Path) -> Tuple[str, str]:
    name = case_dir.parent.name
    for fault in sorted(FAULT_SUFFIXES, key=len, reverse=True):
        suffix = "_" + fault
        if name.endswith(suffix):
            return name[:-len(suffix)], fault
    return "", "unknown"


def discover_cases(root: Path) -> Iterable[Tuple[Path, Path, Path]]:
    for inject_file in sorted(root.rglob("inject_time.txt")):
        case_dir = inject_file.parent
        csv_file = case_dir / "simple_data.csv"
        if not csv_file.exists():
            csv_file = case_dir / "data.csv"
        if csv_file.exists():
            yield case_dir, inject_file, csv_file


def process_case(case_dir: Path, inject_file: Path, source_csv: Path, root: Path, output: Path) -> Dict[str, object]:
    try:
        inject_time = float(inject_file.read_text(encoding="utf-8").strip())
    except (ValueError, OSError) as exc:
        raise ValueError(f"invalid injection timestamp: {inject_file}") from exc
    root_cause, fault = case_labels(case_dir)
    system = next((part for part in case_dir.parts if part.startswith("RE1-")), "unknown")
    case_id = "/".join(case_dir.relative_to(root).parts)
    digest = hashlib.sha256(case_id.encode()).hexdigest()
    bucket = int(digest[:8], 16) % 100
    split = "train" if bucket < 70 else "validation" if bucket < 85 else "test"
    destination = output / split / system / digest[:16] / "observations.csv"
    destination.parent.mkdir(parents=True, exist_ok=True)

    feature_columns: List[Tuple[str, str]] = []
    rows_written = 0
    missing = Counter()
    unmapped_columns: List[str] = []
    metric_counts = Counter()
    timestamps = []
    services = set()
    with source_csv.open(newline="", encoding="utf-8-sig") as src, destination.open("w", newline="", encoding="utf-8") as dst:
        reader = csv.DictReader(src)
        if not reader.fieldnames or "time" not in reader.fieldnames:
            raise ValueError(f"expected a time column in {source_csv}")
        feature_columns = [(column, *mapping) for column in reader.fieldnames if (mapping := parse_metric_column(column))]
        unmapped_columns = [column for column in reader.fieldnames if column.lower() != "time" and parse_metric_column(column) is None]
        writer = csv.DictWriter(dst, fieldnames=("case_id", "system", "timestamp", "service_id", "metric", "value", "post_injection", "root_cause_service", "fault"))
        writer.writeheader()
        for row in reader:
            timestamp = parse_number(row.get("time", ""))
            if timestamp is None:
                missing["invalid_timestamp"] += 1
                continue
            timestamps.append(timestamp)
            for column, service_id, metric in feature_columns:
                value = parse_number(row.get(column, ""))
                if value is None:
                    missing[f"{service_id}.{metric}"] += 1
                    continue
                services.add(service_id)
                metric_counts[metric] += 1
                writer.writerow({"case_id": case_id, "system": system, "timestamp": f"{timestamp:.6f}", "service_id": service_id,
                                 "metric": metric, "value": f"{value:.12g}", "post_injection": int(timestamp >= inject_time),
                                 "root_cause_service": root_cause, "fault": fault})
                rows_written += 1
    ordered_timestamps = sorted(timestamps)
    gaps = [right - left for left, right in zip(ordered_timestamps, ordered_timestamps[1:])]
    return {"case_id": case_id, "system": system, "fault": fault, "root_cause_service": root_cause or None,
            "inject_time": inject_time, "split": split, "source": str(source_csv.relative_to(root)), "output": str(destination.relative_to(output)),
            "services": sorted(services), "service_count": len(services), "rows": rows_written,
            "feature_columns": len(feature_columns), "unmapped_columns": unmapped_columns,
            "metric_counts": dict(metric_counts), "timestamp_start": min(timestamps) if timestamps else None,
            "timestamp_end": max(timestamps) if timestamps else None,
            "duplicate_timestamps": len(ordered_timestamps) - len(set(ordered_timestamps)),
            "timestamp_gap_count": sum(gap > 1.5 for gap in gaps),
            "median_timestamp_step_seconds": median(gaps) if gaps else None,
            "metric_value_coverage": rows_written / (len(timestamps) * len(feature_columns)) if timestamps and feature_columns else None,
            "pre_injection_timestamps": sum(timestamp < inject_time for timestamp in timestamps),
            "post_injection_timestamps": sum(timestamp >= inject_time for timestamp in timestamps),
            "missing_or_invalid": dict(missing)}


def prepare(root: Path, output: Path) -> Dict[str, object]:
    cases = [process_case(*case, root, output) for case in discover_cases(root)]
    if not cases:
        raise ValueError(f"no paired inject_time.txt and simple_data.csv/data.csv cases found under {root}")
    fault_counts = Counter(str(case["fault"]) for case in cases)
    system_counts = Counter(str(case["system"]) for case in cases)
    split_counts = Counter(str(case["split"]) for case in cases)
    label_coverage = sum(case["root_cause_service"] is not None for case in cases)
    missing_by_signal = Counter()
    missing_total = 0
    invalid_timestamps = 0
    for case in cases:
        for signal, count in case["missing_or_invalid"].items():
            if signal == "invalid_timestamp":
                invalid_timestamps += int(count)
                continue
            missing_by_signal[signal] += int(count)
            missing_total += int(count)
    total_cells = sum(int(case["feature_columns"]) * (int(case["pre_injection_timestamps"]) + int(case["post_injection_timestamps"])) for case in cases)
    split_cases = {split: [case["case_id"] for case in cases if case["split"] == split] for split in ("train", "validation", "test")}
    report = {"schema_version": "rcaeval-re1-long-v1", "source_root": str(root), "case_count": len(cases),
              "split_counts": dict(split_counts), "system_counts": dict(system_counts), "fault_counts": dict(fault_counts),
              "split_case_ids": split_cases,
              "missingness": {"missing_or_invalid_values": missing_total, "invalid_timestamps": invalid_timestamps,
                              "expected_feature_cells": total_cells,
                              "observed_feature_cells": max(0, total_cells - missing_total),
                              "missing_or_invalid_rate": missing_total / total_cells if total_cells else None,
                              "by_signal": dict(missing_by_signal)},
              "root_cause_service_label_coverage": {"cases_with_service_label": label_coverage, "cases_without_service_label": len(cases) - label_coverage},
              "cases": cases,
              "limitations": ["Labels use the supplied injection timestamp as the post-injection boundary; confirm boundary semantics for each source release.",
                              "Root cause service is parsed only for known fault-name suffixes; unknown cases retain no service label.",
                              "These service observations do not provide topology; do not infer dependency edges from metric columns."]}
    output.mkdir(parents=True, exist_ok=True)
    (output / "manifest.json").write_text(json.dumps(report, indent=2), encoding="utf-8")
    return report


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True, help="Extracted RCAEval RE1 system directory")
    parser.add_argument("--output", type=Path, default=Path("dataset/processed/rcaeval_re1"))
    args = parser.parse_args()
    report = prepare(args.input, args.output)
    print(json.dumps({key: report[key] for key in ("case_count", "split_counts", "system_counts", "fault_counts")}, indent=2))


if __name__ == "__main__":
    main()
