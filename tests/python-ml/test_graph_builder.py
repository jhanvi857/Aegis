"""
Unit tests for GraphBuilder (Phase 2)
"""

import unittest
import os
import sys
import networkx as nx

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
from graph.graph_builder import GraphBuilder


class TestGraphBuilder(unittest.TestCase):
    def setUp(self):
        self.topology_path = "configs/topology.yaml"
        self.builder = GraphBuilder(topology_path=self.topology_path)

    def test_load_topology(self):
        self.assertGreater(self.builder.graph.number_of_nodes(), 0)
        self.assertGreater(self.builder.graph.number_of_edges(), 0)
        self.assertIn("gateway", self.builder.graph)
        self.assertIn("node-a", self.builder.graph)
        self.assertIn("node-d", self.builder.graph)

        # Verify default edge properties
        edge_data = self.builder.graph.edges["gateway", "node-a"]
        self.assertEqual(edge_data.get("relationship"), "calls")
        self.assertIn("latency_ms", edge_data)

    def test_dynamic_telemetry_update_normal(self):
        batch = {
            "timestamp_unix_nano": 1000000000,
            "metrics": [
                {"service_id": "node-a", "metric_name": "cpu_usage", "value": 35.0},
                {"service_id": "node-a", "metric_name": "p95_latency_ms", "value": 30.0},
                {"service_id": "node-a", "metric_name": "error_rate", "value": 0.002},
            ],
            "logs": [],
        }
        updated = self.builder.update_with_telemetry(batch)
        self.assertGreaterEqual(updated, 1)

        node_data = self.builder.graph.nodes["node-a"]
        self.assertEqual(node_data["status"], "healthy")
        self.assertEqual(node_data["metrics"]["cpu_usage"], 35.0)

    def test_health_degradation_and_critical(self):
        # Degraded by CPU stress
        degraded_batch = {
            "timestamp_unix_nano": 2000000000,
            "metrics": [
                {"service_id": "node-b", "metric_name": "cpu_usage", "value": 89.5},
                {"service_id": "node-b", "metric_name": "error_rate", "value": 0.02},
            ],
        }
        self.builder.update_with_telemetry(degraded_batch)
        self.assertEqual(self.builder.graph.nodes["node-b"]["status"], "degraded")

        # Critical by high error rate
        critical_batch = {
            "timestamp_unix_nano": 3000000000,
            "metrics": [
                {"service_id": "node-b", "metric_name": "error_rate", "value": 0.25},
                {"service_id": "node-b", "metric_name": "p95_latency_ms", "value": 3500.0},
            ],
        }
        self.builder.update_with_telemetry(critical_batch)
        self.assertEqual(self.builder.graph.nodes["node-b"]["status"], "critical")

        # Verify edge latency updated on callers to node-b
        if self.builder.graph.has_edge("node-a", "node-b"):
            self.assertEqual(self.builder.graph.edges["node-a", "node-b"]["latency_ms"], 3500.0)

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
