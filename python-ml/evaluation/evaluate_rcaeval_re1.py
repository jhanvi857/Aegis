"""Incident-aware evaluation of the shared baseline on prepared RCAEval RE1.

This evaluates detection only. RE1's injection timestamp supplies the event
boundary; it does not provide per-service anomaly labels or topology, so this
script does not report service-level accuracy or RCA accuracy.
"""
from __future__ import annotations

import argparse
import csv
import json
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
import sys
from statistics import median
from typing import Any, Dict, List

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from detection.service_detector import ServiceDetector


def normalize_observation(metric: str, raw: float) -> tuple[str, float] | None:
    if metric == "cpu":
        return "cpu_usage", raw
    if metric == "error":
        return "error_rate", raw
    if metric.startswith("latency-") or metric == "latency":
        # RE1 exported latency quantiles in seconds in the inspected SS/TT captures.
        return "latency_ms", raw * 1000.0
    return None


def evaluate_case(case: Dict[str, Any], output_root: Path, detector_factory=ServiceDetector) -> Dict[str, Any]:
    rows_by_timestamp: Dict[float, Dict[str, Dict[str, float]]] = defaultdict(lambda: defaultdict(dict))
    file_path = Path(case["output"])
    if not file_path.is_absolute():
        file_path = output_root / file_path
    with file_path.open(newline="", encoding="utf-8") as stream:
        for row in csv.DictReader(stream):
            converted = normalize_observation(row["metric"], float(row["value"]))
            if converted is None:
                continue
            metric, value = converted
            rows_by_timestamp[float(row["timestamp"])][row["service_id"]][metric] = value

    detector = detector_factory()
    inject_time = float(case["inject_time"])
    first_detected = None
    pre_injection_alert_windows = 0
    post_injection_alert_windows = 0
    times = sorted(rows_by_timestamp)
    for timestamp in times:
        observed_at = datetime.fromtimestamp(timestamp, timezone.utc)
        window_alert = False
        for service_id, features in sorted(rows_by_timestamp[timestamp].items()):
            result = detector.observe(service_id, features, observed_at=observed_at)
            if result["evidence"]:
                window_alert = True
        if window_alert:
            if timestamp < inject_time:
                pre_injection_alert_windows += 1
            else:
                post_injection_alert_windows += 1
                if first_detected is None:
                    first_detected = timestamp

    detected = first_detected is not None
    return {"case_id": case["case_id"], "system": case["system"], "fault": case["fault"],
            "root_cause_service": case.get("root_cause_service"), "split": case["split"], "detected": detected,
            "detection_delay_seconds": first_detected - inject_time if detected else None,
            "pre_injection_alert_windows": pre_injection_alert_windows,
            "post_injection_alert_windows": post_injection_alert_windows,
            "window_count": len(times)}


def evaluate_manifest(manifest_path: Path, split: str = "test") -> Dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    cases = [case for case in manifest["cases"] if case["split"] == split]
    # A case is the incident unit: no windows are independently assigned to splits.
    split_membership: Dict[str, set[str]] = defaultdict(set)
    for case in manifest["cases"]:
        split_membership[case["case_id"]].add(case["split"])
    leaking_cases = sorted(case_id for case_id, memberships in split_membership.items() if len(memberships) > 1)
    if leaking_cases:
        raise ValueError(f"incident cases occur in multiple splits: {leaking_cases[:5]}")
    results = [evaluate_case(case, manifest_path.parent) for case in cases]
    delays = [item["detection_delay_seconds"] for item in results if item["detected"]]
    total_windows = sum(item["window_count"] for item in results)
    pre_alerts = sum(item["pre_injection_alert_windows"] for item in results)
    return {"schema_version": "rcaeval-re1-detection-eval-v1", "split": split,
            "case_count": len(results), "detected_cases": sum(item["detected"] for item in results),
            "incident_detection_rate": sum(item["detected"] for item in results) / len(results) if results else None,
            "missed_incidents": sum(not item["detected"] for item in results),
            "incident_miss_rate": sum(not item["detected"] for item in results) / len(results) if results else None,
            "median_detection_delay_seconds": median(delays) if delays else None,
            "pre_injection_alert_windows": pre_alerts,
            "pre_injection_alert_rate_per_window": pre_alerts / total_windows if total_windows else None,
            "post_injection_alert_windows": sum(item["post_injection_alert_windows"] for item in results),
            "window_count": total_windows,
            "missingness": manifest.get("missingness", {"available": False}),
            "results": results,
            "interpretation": "Incident-level RE1 detection only. Injection boundaries identify incidents, but there are no labeled normal incidents or per-service anomaly windows, so specificity, precision, and per-service classification metrics are unavailable. Thresholds and latency unit conversion must be checked for each system release. No RCA score is computed."}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path, help="manifest.json created by dataset/preprocess_rcaeval.py")
    parser.add_argument("--split", choices=("train", "validation", "test"), default="test")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = evaluate_manifest(args.manifest, args.split)
    rendered = json.dumps(result, indent=2)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered + "\n", encoding="utf-8")
    print(rendered)


if __name__ == "__main__":
    main()
