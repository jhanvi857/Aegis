"""Integration test: generic telemetry updates the graph representation."""
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer


class TestPhase2Integration(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory(dir=os.path.dirname(__file__))
        self.topology_path = os.path.join(self._tmp.name, "topology.yaml")
        with open(self.topology_path, "w", encoding="utf-8") as f:
            f.write("system_name: test-system\nnodes:\n  - id: ingress\n    dependencies: [worker]\n  - id: worker\n    dependencies: []\n")
        self.service = GraphPreprocessingService(topology_path=self.topology_path, structural_ttl_sec=0.0)
        self.consumer = TelemetryConsumer()

    def tearDown(self):
        self.service.stop()
        self._tmp.cleanup()

    def test_baseline_has_unknown_health_without_observations(self):
        rep = self.service.get_latest_representation()
        self.assertEqual({n["status"] for n in rep["snapshot"]["nodes"]}, {"unknown"})
        self.assertEqual(rep["snapshot"]["nodes"][0]["metrics"], {})

    def test_telemetry_updates_generic_service_representation(self):
        batch = self.consumer.generate_simulated_batch(node_ids=["worker"], fault_node="worker", fault_type="latency")
        rep = self.service.process_telemetry_batch(batch)
        worker = next(n for n in rep["snapshot"]["nodes"] if n["id"] == "worker")
        self.assertIn(worker["status"], {"degraded", "critical"})
        self.assertGreaterEqual(worker["metrics"]["p95_latency_ms"], 500.0)
        edge = next(e for e in rep["snapshot"]["edges"] if e["source"] == "ingress" and e["target"] == "worker")
        self.assertGreaterEqual(edge["latency_ms"], 500.0)


if __name__ == "__main__":
    unittest.main()
