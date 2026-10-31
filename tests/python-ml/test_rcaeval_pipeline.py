import os
import sys

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from dataset.preprocess_rcaeval import parse_metric_column, prepare
from evaluation.evaluate_rcaeval_re1 import evaluate_manifest


def test_preprocess_keeps_missing_values_missing_and_splits_by_case(tmp_path):
    root = tmp_path / "RE1-SS"
    case = root / "carts_cpu" / "1"
    case.mkdir(parents=True)
    (case / "inject_time.txt").write_text("1002\n")
    (case / "simple_data.csv").write_text(
        "time,carts_cpu,carts_error,carts_latency-90\n"
        "1000,20,0,0.02\n"
        "1001,21,,0.02\n"
        "1002,99,0.2,0.9\n",
        encoding="utf-8",
    )
    output = tmp_path / "processed"
    report = prepare(root, output)
    assert report["case_count"] == 1
    assert report["cases"][0]["root_cause_service"] == "carts"
    assert report["cases"][0]["pre_injection_timestamps"] == 2
    assert report["cases"][0]["post_injection_timestamps"] == 1
    assert report["cases"][0]["metric_value_coverage"] < 1
    assert report["cases"][0]["split"] in {"train", "validation", "test"}
    assert report["cases"][0]["missing_or_invalid"]["carts.error"] == 1
    with open(output / report["cases"][0]["output"], encoding="utf-8") as stream:
        rendered = stream.read()
    assert "carts_error" not in rendered


def test_metric_column_mapping_and_re1_eval_reports_incident_metrics(tmp_path):
    assert parse_metric_column("service-a_latency-90") == ("service-a", "latency-90")
    root = tmp_path / "RE1-TT"
    case = root / "worker_delay" / "1"
    case.mkdir(parents=True)
    (case / "inject_time.txt").write_text("1002\n")
    (case / "simple_data.csv").write_text(
        "time,worker_cpu,worker_latency-90\n"
        "1000,10,0.02\n1001,11,0.02\n1002,99,0.8\n1003,98,0.9\n",
        encoding="utf-8",
    )
    output = tmp_path / "prepared"
    report = prepare(root, output)
    manifest_path = output / "manifest.json"
    summary = evaluate_manifest(manifest_path, report["cases"][0]["split"])
    assert summary["case_count"] == 1
    assert 0 <= summary["incident_detection_rate"] <= 1
    assert summary["results"][0]["window_count"] == 4

