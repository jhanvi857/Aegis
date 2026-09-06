"""
Unit tests for TelemetryConsumer (Phase 2)
"""

import unittest
import os
import sys

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from ingestion.kafka_consumer import TelemetryConsumer


class TestTelemetryConsumer(unittest.TestCase):
    def setUp(self):
        self.consumer = TelemetryConsumer(bootstrap_servers="nonexistent-broker:9092")

    def test_start_fallback_mode(self):
        # When Kafka is unreachable, start() should return False and run in fallback simulation mode
        connected = self.consumer.start()
        self.assertFalse(connected)
        self.assertIsNone(self.consumer.consumer)
        self.assertTrue(self.consumer.is_running)
        self.consumer.close()

    def test_generate_simulated_batch(self):
        batch = self.consumer.generate_simulated_batch(
            node_ids=["gateway", "node-a", "node-b"],
            fault_node="node-a",
            fault_type="cpu_stress",
        )

        self.assertIn("timestamp_unix_nano", batch)
        self.assertIn("metrics", batch)
        self.assertIn("logs", batch)

        # Check metrics for fault node
        node_a_metrics = {
            m["metric_name"]: m["value"]
            for m in batch["metrics"]
            if m["service_id"] == "node-a"
        }
        self.assertIn("cpu_usage", node_a_metrics)
        self.assertGreater(node_a_metrics["cpu_usage"], 90.0)

        # Check error log generated for faulty node
        self.assertGreater(len(batch["logs"]), 0)
        self.assertEqual(batch["logs"][0]["service_id"], "node-a")

    def test_simulate_stream_batches(self):
        self.consumer.is_running = True
        batches = list(self.consumer.simulate_stream(interval_sec=0.01, max_batches=3))
        self.assertEqual(len(batches), 3)
        self.consumer.close()


if __name__ == "__main__":
    unittest.main()
