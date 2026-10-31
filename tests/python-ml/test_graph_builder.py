"""
Unit tests for GraphBuilder (Phase 2)
"""

import unittest
import os
import sys
import tempfile

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
from graph.graph_builder import GraphBuilder


class TestGraphBuilder(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory(dir=os.path.dirname(__file__))
        self.topology_path = os.path.join(self._tmp.name, "topology.yaml")
        with open(self.topology_path, "w", encoding="utf-8") as f:
            f.write("version: '1'\nsystem_name: test-system\nnodes:\n"
                    "  - id: ingress\n    dependencies: [worker]\n"
                    "  - id: worker\n    dependencies: []\n"
                    "  - id: isolated\n    dependencies: []\n")
        self.builder = GraphBuilder(topology_path=self.topology_path)

    def tearDown(self):
        self._tmp.cleanup()

    def test_load_topology(self):
        self.assertGreater(self.builder.graph.number_of_nodes(), 0)
        self.assertGreater(self.builder.graph.number_of_edges(), 0)
        self.assertIn("ingress", self.builder.graph)
        self.assertIn("worker", self.builder.graph)
        self.assertIn("isolated", self.builder.graph)

        # Configured dependencies carry provenance but no fabricated observations.
        edge_data = self.builder.graph.edges["ingress", "worker"]
        self.assertEqual(edge_data.get("relationship"), "calls")
        self.assertEqual(edge_data.get("provenance"), "configured")
        self.assertNotIn("latency_ms", edge_data)
        self.assertEqual(self.builder.graph.nodes["worker"]["status"], "unknown")
        self.assertEqual(self.builder.graph.nodes["worker"]["metrics"], {})

    def test_partial_telemetry_does_not_fill_missing_signals_with_zero(self):
        self.builder.update_with_telemetry({
            "timestamp_unix_nano": 4_000_000_000,
            "metrics": [{"service_id": "worker", "metric_name": "cpu_usage", "value": 42}],
        })
        node = self.builder.graph.nodes["worker"]
        self.assertEqual(node["status"], "healthy")
        self.assertEqual(node["metrics"], {"cpu_usage": 42.0})
        self.assertNotIn("error_rate", node["metrics"])

    def test_invalid_and_non_finite_measurements_are_ignored(self):
        self.builder.update_with_telemetry({
            "timestamp_unix_nano": 5_000_000_000,
            "metrics": [
                {"service_id": "new-service", "metric_name": "cpu_usage", "value": "bad"},
                {"service_id": "new-service", "metric_name": "memory_usage", "value": float("nan")},
            ],
        })
        self.assertNotIn("new-service", self.builder.graph)

    def test_dynamic_telemetry_update_normal(self):
        batch = {
            "timestamp_unix_nano": 1000000000,
            "metrics": [
                {"service_id": "worker", "metric_name": "cpu_usage", "value": 35.0},
                {"service_id": "worker", "metric_name": "p95_latency_ms", "value": 30.0},
                {"service_id": "worker", "metric_name": "error_rate", "value": 0.002},
            ],
            "logs": [],
        }
        updated = self.builder.update_with_telemetry(batch)
        self.assertGreaterEqual(updated, 1)

        node_data = self.builder.graph.nodes["worker"]
        self.assertEqual(node_data["status"], "healthy")
        self.assertEqual(node_data["metrics"]["cpu_usage"], 35.0)

    def test_health_degradation_and_critical(self):
        # Degraded by CPU stress
        degraded_batch = {
            "timestamp_unix_nano": 2000000000,
            "metrics": [
                {"service_id": "worker", "metric_name": "cpu_usage", "value": 89.5},
                {"service_id": "worker", "metric_name": "error_rate", "value": 0.02},
            ],
        }
        self.builder.update_with_telemetry(degraded_batch)
        self.assertEqual(self.builder.graph.nodes["worker"]["status"], "degraded")

        # Critical by high error rate
        critical_batch = {
            "timestamp_unix_nano": 3000000000,
            "metrics": [
                {"service_id": "worker", "metric_name": "error_rate", "value": 0.25},
                {"service_id": "worker", "metric_name": "p95_latency_ms", "value": 3500.0},
            ],
        }
        self.builder.update_with_telemetry(critical_batch)
        self.assertEqual(self.builder.graph.nodes["worker"]["status"], "critical")

        # Verify edge latency updated on callers to node-b
        self.assertEqual(self.builder.graph.edges["ingress", "worker"]["latency_ms"], 3500.0)

    def test_get_snapshot_dict(self):
        snapshot = self.builder.get_snapshot_dict()
        self.assertIn("timestamp_unix_nano", snapshot)
        self.assertIn("nodes", snapshot)
        self.assertIn("edges", snapshot)
        self.assertEqual(len(snapshot["nodes"]), self.builder.graph.number_of_nodes())
        self.assertEqual(len(snapshot["edges"]), self.builder.graph.number_of_edges())

        first_node = snapshot["nodes"][0]
        self.assertIn("id", first_node)
        self.assertIn("status", first_node)
        self.assertIn("metrics", first_node)


if __name__ == "__main__":
    unittest.main()
