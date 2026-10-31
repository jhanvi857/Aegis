import os
import sys
import unittest
from datetime import datetime, timedelta, timezone

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
from detection.service_detector import ServiceDetector, group_metric_points


class TestServiceDetector(unittest.TestCase):
    def setUp(self):
        self.detector = ServiceDetector(min_baseline_samples=4, history_limit=8)
        self.start = datetime(2026, 1, 1, tzinfo=timezone.utc)

    def test_cold_start_uses_only_configured_threshold_evidence(self):
        result = self.detector.observe("svc-a", {"cpu_usage": 95}, self.start, window_seconds=10)
        self.assertEqual(result["status"], "scored")
        self.assertEqual(result["severity"], "warning")
        self.assertEqual(result["evidence"][0]["method"], "threshold")
        self.assertEqual(result["evidence"][0]["baseline_samples"], 0)
        self.assertNotIn("error_rate", result["signals_observed"])

    def test_rolling_baseline_detects_a_shift_without_service_specific_models(self):
        for i, value in enumerate((48, 49, 50, 51)):
            self.detector.observe("svc-a", {"latency_ms": value}, self.start + timedelta(seconds=i))
        result = self.detector.observe("svc-a", {"latency_ms": 56}, self.start + timedelta(seconds=4))
        self.assertEqual(result["severity"], "warning")
        self.assertEqual(result["evidence"][0]["method"], "threshold+rolling_mad")
        self.assertEqual(result["evidence"][0]["baseline_samples"], 4)

    def test_flagged_observations_do_not_poison_rolling_baseline(self):
        for i, value in enumerate((40, 42, 39, 41)):
            self.detector.observe("svc-a", {"latency_ms": value}, self.start + timedelta(seconds=i))
        flagged = self.detector.observe("svc-a", {"latency_ms": 600}, self.start + timedelta(seconds=4))
        after = self.detector.observe("svc-a", {"latency_ms": 100}, self.start + timedelta(seconds=5))
        self.assertTrue(flagged["evidence"])
        self.assertEqual(after["evidence"][0]["baseline_samples"], 4)

    def test_service_histories_are_isolated(self):
        for i in range(4):
            self.detector.observe("svc-a", {"latency_ms": 40}, self.start + timedelta(seconds=i))
        result = self.detector.observe("svc-b", {"latency_ms": 350}, self.start + timedelta(seconds=5))
        self.assertEqual(result["evidence"][0]["baseline_samples"], 0)

    def test_invalid_values_and_unknown_features_are_not_scored(self):
        result = self.detector.observe(
            "svc-a", {"cpu_usage": float("nan"), "custom_signal": 100}, self.start
        )
        self.assertEqual(result["status"], "insufficient_data")
        self.assertEqual(result["evidence"], [])

    def test_grouping_rejects_invalid_points(self):
        grouped = group_metric_points([
            {"service_id": "svc-a", "metric_name": "cpu_usage", "value": 20},
            {"service_id": "svc-a", "metric_name": "memory_usage", "value": "bad"},
            {"service_id": "", "metric_name": "cpu_usage", "value": 10},
        ])
        self.assertEqual(grouped, {"svc-a": {"cpu_usage": 20.0}})

    def test_grouping_uses_latest_timestamp_for_duplicate_signal(self):
        grouped = group_metric_points([
            {"service_id": "svc-a", "metric_name": "cpu_usage", "value": 80, "timestamp_unix_nano": 20},
            {"service_id": "svc-a", "metric_name": "cpu_usage", "value": 10, "timestamp_unix_nano": 10},
        ])
        self.assertEqual(grouped, {"svc-a": {"cpu_usage": 80.0}})

    def test_yaml_thresholds_support_low_direction_at_cold_start(self):
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            config = Path(tmp) / "rules.yaml"
            config.write_text(
                "min_baseline_samples: 4\nsignals:\n  healthy_workers:\n    threshold: 2\n    direction: low\n    unit: workers\n",
                encoding="utf-8",
            )
            result = ServiceDetector.from_yaml(config).observe("arbitrary", {"healthy_workers": 1}, self.start)
        self.assertEqual(result["status"], "scored")
        self.assertEqual(result["severity"], "warning")
        self.assertEqual(result["evidence"][0]["threshold"], 2.0)

    def test_yaml_thresholds_reject_invalid_direction(self):
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            config = Path(tmp) / "rules.yaml"
            config.write_text("signals:\n  cpu_usage:\n    threshold: 10\n    direction: sideways\n", encoding="utf-8")
            with self.assertRaises(ValueError):
                ServiceDetector.from_yaml(config)

    def test_service_name_slo_overrides_global_threshold(self):
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            config = Path(tmp) / "rules.yaml"
            config.write_text(
                "signals:\n  latency_ms:\n    threshold: 300\n    direction: high\n"
                "services:\n  api-one:\n    signals:\n      latency_ms:\n        threshold: 100\n        direction: high\n        unit: ms\n",
                encoding="utf-8",
            )
            detector = ServiceDetector.from_yaml(config)
            overridden = detector.observe("api-one", {"latency_ms": 150}, self.start)
            defaulted = detector.observe("api-two", {"latency_ms": 150}, self.start)
        self.assertEqual(overridden["evidence"][0]["threshold"], 100)
        self.assertEqual(defaulted["evidence"], [])


if __name__ == "__main__":
    unittest.main()
