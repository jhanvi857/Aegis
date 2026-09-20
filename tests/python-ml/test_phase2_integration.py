"""
Phase 2 End-to-End Integration Test
Verifies complete flow: Simulated Chaos Telemetry -> GraphPreprocessingService -> Live Graph Representation
"""

import unittest
import os
import sys

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer


class TestPhase2Integration(unittest.TestCase):
    def setUp(self):
        self.service = GraphPreprocessingService(
            topology_path="configs/topology.yaml",
            structural_ttl_sec=0.1,
        )
        self.consumer = TelemetryConsumer()

    def tearDown(self):
        self.service.stop()

    def test_baseline_graph_representation(self):
        rep = self.service.get_latest_representation()
        self.assertIsNotNone(rep)

        # Verify topological ordering from gateway to sink nodes
        topo_order = rep["topological_order"]
        self.assertEqual(topo_order[0], "gateway")
        self.assertIn("node-a", topo_order)
        self.assertIn("node-d", topo_order)

        # In baseline, all nodes are healthy
        for node_info in rep["snapshot"]["nodes"]:
            self.assertEqual(node_info["status"], "healthy")

    def test_chaos_injection_propagates_to_representation(self):
        # 1. Generate a chaos batch with latency injection on node-c
        chaos_batch = self.consumer.generate_simulated_batch(
            fault_node="node-c",
            fault_type="latency",
        )

        # 2. Process batch through the service
        updated_rep = self.service.process_telemetry_batch(chaos_batch)

        # 3. Verify node-c is degraded
        node_c_data = next(
            n for n in updated_rep["snapshot"]["nodes"] if n["id"] == "node-c"
        )
        self.assertIn(node_c_data["status"], ["degraded", "critical"])
        self.assertGreaterEqual(node_c_data["metrics"]["p95_latency_ms"], 500.0)

        # 4. Verify feature vector reflects latency degradation in index 2 (latency_norm)
        feat_vec = updated_rep["feature_matrix"]["node-c"]
        self.assertEqual(len(feat_vec), 8)
        self.assertGreaterEqual(feat_vec[2], 0.5)

        # 5. Verify edge between node-a and node-c reflects high latency
        edge_ac = next(
            e for e in updated_rep["snapshot"]["edges"]
            if e["source"] == "node-a" and e["target"] == "node-c"
        )
        self.assertGreaterEqual(edge_ac["latency_ms"], 500.0)


if __name__ == "__main__":
    unittest.main()
