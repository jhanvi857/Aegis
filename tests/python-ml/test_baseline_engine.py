import os
import sys
import unittest
from datetime import datetime, timezone
from unittest.mock import patch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
from serving.baseline_engine import PredictionEngine
from serving.incident_models import IncidentModels


class TestBaselineEngine(unittest.TestCase):
    def setUp(self):
        self.engine = PredictionEngine()

    def test_missing_telemetry_is_not_reported_as_anomaly(self):
        result = self.engine.predict({
            "node_ids": ["svc-a"],
            "snapshot": {"nodes": [{"id": "svc-a", "metrics": {}}], "edges": []},
        })
        self.assertEqual(result["engine"], "baseline")
        self.assertEqual(result["failure_predictions"], [])
        self.assertEqual(result["root_causes"], [])
        self.assertIsNone(result["recovery_plan"])
        self.assertTrue(len(result["fallback_reason"]) > 0)

    def test_detected_symptom_has_evidence_and_rca_stays_uncertain(self):
        engine = PredictionEngine()
        engine.incident_models = None
        engine.incident_models_load_error = "forced baseline for test"
        result = engine.predict({
            "node_ids": ["caller", "dependency"],
            "snapshot": {
                "nodes": [
                    {"id": "caller", "metrics": {"error_rate": 0.01}},
                    {"id": "dependency", "metrics": {"error_rate": 0.20}},
                ],
                "edges": [{"source": "caller", "target": "dependency", "relationship": "calls"}],
            },
        })
        self.assertEqual(result["engine"], "baseline")
        self.assertEqual(len(result["root_causes"]), 1)
        candidate = result["root_causes"][0]
        self.assertEqual(candidate["node_id"], "dependency")
        self.assertIn("error_rate", candidate["contributing_metrics"])
        self.assertGreater(candidate["ranking_score"], 0)
        self.assertEqual(candidate["probability"], 0.0)
        self.assertIn("does not establish causality", candidate["explanation"])
        self.assertIn("inconclusive", candidate["uncertainty"])
        plan = result["recovery_plan"]
        if plan:
            self.assertTrue(all(action["target_node_id"] == "dependency" for action in plan["actions"]))

    def test_trained_model_inference_path(self):
        result = self.engine.predict({
            "node_ids": ["worker"],
            "snapshot": {
                "nodes": [{
                    "id": "worker",
                    "status": "degraded",
                    "metrics": {
                        "cpu_usage": 95.0,
                        "p95_latency_ms": 1200.0,
                        "error_rate": 0.15,
                    },
                }],
                "edges": [],
            },
        })
        self.assertTrue(result["engine"].startswith("trained:"))
        self.assertEqual(result["fallback_reason"], "")
        self.assertTrue(len(result["failure_predictions"]) > 0)
        self.assertTrue(len(result["root_causes"]) > 0)

    def test_fallback_when_artifact_missing(self):
        with patch.dict(os.environ, {"AEGIS_MODEL_PATH": "non_existent_model.joblib"}):
            engine = PredictionEngine()
            result = engine.predict({
                "node_ids": ["worker"],
                "snapshot": {
                    "nodes": [{"id": "worker", "metrics": {"cpu_usage": 95.0, "latency_ms": 1000.0}}],
                    "edges": [],
                },
            })
            self.assertEqual(result["engine"], "baseline")
            self.assertIn("artifact file not found", result["fallback_reason"])

    def test_fallback_when_coverage_below_minimum(self):
        engine = PredictionEngine()
        result = engine.predict({
            "node_ids": ["worker"],
            "snapshot": {
                "nodes": [{"id": "worker", "metrics": {"cpu_usage": 95.0}}],
                "edges": [],
            },
        })
        self.assertEqual(result["engine"], "baseline")
        self.assertIn("coverage", result["fallback_reason"])

    def test_fallback_when_artifact_corrupt(self):
        import tempfile
        with tempfile.NamedTemporaryFile(suffix=".joblib", delete=False) as f:
            f.write(b"not a valid joblib file content")
            corrupt_path = f.name
        try:
            with patch.dict(os.environ, {"AEGIS_MODEL_PATH": corrupt_path}):
                engine = PredictionEngine()
                self.assertIsNone(engine.incident_models)
                self.assertIn("corrupt or invalid artifact", engine.incident_models_load_error)
                result = engine.predict({
                    "node_ids": ["worker"],
                    "snapshot": {"nodes": [{"id": "worker", "metrics": {"cpu_usage": 90.0}}], "edges": []},
                })
                self.assertEqual(result["engine"], "baseline")
        finally:
            if os.path.exists(corrupt_path):
                os.remove(corrupt_path)

    def test_bfs_impact_is_potential_not_a_failure_assertion(self):
        impact = PredictionEngine._blast_radius(
            __import__("networkx").DiGraph([("a", "b"), ("b", "c"), ("x", "c")]), ["a"]
        )
        self.assertEqual([item["path_nodes"] for item in impact], [["a", "b"], ["a", "b", "c"]])
        self.assertTrue(all(item["potential_impact"] for item in impact))
        self.assertTrue(all(item["propagation_risk"] == 0 for item in impact))


if __name__ == "__main__":
    unittest.main()
