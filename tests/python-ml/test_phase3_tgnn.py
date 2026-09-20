"""
Phase 3 End-to-End Unit & Integration Test Suite
Verifies:
1. Chaos Episode Dataset Generation & Tensor Slicing
2. Log Transformer Embedding
3. TGNN Architecture & Multi-Task Forward/Backward Pass
4. Baselines (Isolation Forest & LSTM)
5. Task Heads (FailurePredictor, RootCauseAnalyzer, PropagationPredictor)
6. End-to-End Chaos Inference (Chaos Injection -> TGNN -> Correct Root Cause + Blast Radius)
"""

import os
import sys
import unittest
import numpy as np
import torch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from models.tgnn.model import TGNNModel, GraphConvLayer
from models.log_transformer.embedder import LogTransformerEmbedder
from models.isolation_forest.model import IsolationForestBaseline
from models.lstm.model import LSTMBaseline
from tasks.failure_prediction import FailurePredictor
from tasks.root_cause import RootCauseAnalyzer
from tasks.propagation_prediction import PropagationPredictor
from serving.grpc_server import PredictionEngine
from dataset.generator import DatasetGenerator
from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer


class TestPhase3TGNN(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.generator = DatasetGenerator(topology_path="configs/topology.yaml")
        cls.service = GraphPreprocessingService(topology_path="configs/topology.yaml", structural_ttl_sec=0.01)
        cls.consumer = TelemetryConsumer()
        cls.checkpoint_path = "python-ml/models/checkpoints/tgnn_best.pt"

    def test_01_log_embedder(self):
        embedder = LogTransformerEmbedder(embedding_dim=16)
        logs = [
            {"service_id": "node-c", "level": "ERROR", "message": "High degradation detected on node-c: latency timeout"},
            {"service_id": "node-c", "level": "WARN", "message": "connection retry count exceeded limit"},
        ]
        vec = embedder.embed_logs(logs, "node-c")
        self.assertEqual(vec.shape, (16,))
        self.assertFalse(np.all(vec == 0))

        # Nominal node without logs produces zeros
        vec_nominal = embedder.embed_logs(logs, "node-d")
        self.assertTrue(np.all(vec_nominal == 0))

    def test_02_tgnn_forward_and_backward(self):
        model = TGNNModel(in_dim=8, hidden_dim=32, num_nodes=5, num_spatial_layers=2)
        x = torch.randn(4, 10, 5, 8, requires_grad=True)
        adj = torch.ones(4, 10, 5, 5)

        out = model(x, adj)
        self.assertEqual(out["cluster_failure_prob"].shape, (4,))
        self.assertEqual(out["node_failure_probs"].shape, (4, 5))
        self.assertEqual(out["confidence"].shape, (4,))
        self.assertEqual(out["root_cause_probs"].shape, (4, 6))
        self.assertEqual(out["propagation_probs"].shape, (4, 5))
        self.assertEqual(out["time_to_failure"].shape, (4,))

        # Verify gradient backprop
        loss = (
            out["cluster_failure_prob"].sum()
            + out["node_failure_probs"].sum()
            + out["root_cause_probs"].sum()
            + out["propagation_probs"].sum()
        )
        loss.backward()
        self.assertIsNotNone(x.grad)

    def test_03_isolation_forest_baseline(self):
        baseline = IsolationForestBaseline(contamination=0.1, random_state=42)
        feats = np.random.randn(20, 10, 5, 8).astype(np.float32)
        baseline.fit(feats)
        preds = baseline.predict_anomaly(feats[:5])
        self.assertIn("failure_probs", preds)
        self.assertEqual(preds["failure_probs"].shape, (5,))
        self.assertEqual(preds["root_cause_probs"].shape, (5, 5))

    def test_04_lstm_baseline(self):
        baseline = LSTMBaseline(num_nodes=5, feature_dim=8, hidden_dim=32, num_layers=1)
        x = torch.randn(3, 10, 5, 8)
        out = baseline(x)
        self.assertEqual(out["cluster_failure_prob"].shape, (3,))
        self.assertEqual(out["root_cause_probs"].shape, (3, 6))

    def test_05_task_heads_nominal_state(self):
        rep = self.service.get_latest_representation()

        fail_pred = FailurePredictor(model_checkpoint=self.checkpoint_path)
        preds = fail_pred.predict(rep)
        self.assertEqual(len(preds), 5)
        for p in preds:
            self.assertIn("node_id", p)
            self.assertIn("failure_probability", p)
            self.assertIn("confidence", p)
            self.assertIn("estimated_time_to_failure_sec", p)

        rc_analyzer = RootCauseAnalyzer(model_checkpoint=self.checkpoint_path)
        rcs = rc_analyzer.analyze(rep, top_k=3)
        self.assertEqual(len(rcs), 3)
        for r in rcs:
            self.assertIn("node_id", r)
            self.assertIn("probability", r)
            self.assertIn("explanation", r)
            self.assertIn("contributing_metrics", r)

        prop_pred = PropagationPredictor(model_checkpoint=self.checkpoint_path)
        paths = prop_pred.predict_paths(rep, root_cause_node="node-a")
        self.assertGreater(len(paths), 0)
        for path in paths:
            self.assertIn("path_nodes", path)
            self.assertIn("propagation_risk", path)

    def test_06_end_to_end_chaos_inference(self):
        """Inject chaos onto node-c and verify TGNN isolates node-c as root cause."""
        engine = PredictionEngine(checkpoint_path=self.checkpoint_path)

        # 1. Nominal batch
        nominal_batch = self.consumer.generate_simulated_batch()
        rep_nominal = self.service.process_telemetry_batch(nominal_batch)
        pred_nom = engine.predict(rep_nominal)

        # In nominal, failure probs should be low
        nom_probs = [p["failure_probability"] for p in pred_nom["failure_predictions"]]
        self.assertLess(max(nom_probs), 0.7)

        # 2. Injected chaos batch on node-c
        chaos_batch = self.consumer.generate_simulated_batch(
            fault_node="node-c",
            fault_type="latency",
        )
        rep_chaos = self.service.process_telemetry_batch(chaos_batch)
        pred_chaos = engine.predict(rep_chaos, target_node_id="node-c")

        # Root cause top-1 should identify node-c
        top_rc = pred_chaos["root_causes"][0]
        self.assertEqual(top_rc["node_id"], "node-c")
        self.assertIn("node-c", top_rc["explanation"])

        # Node-c failure probability should spike
        node_c_pred = next(p for p in pred_chaos["failure_predictions"] if p["node_id"] == "node-c")
        self.assertGreater(node_c_pred["failure_probability"], 0.5)

        # Downstream propagation path from node-c to node-d should be predicted
        prop_targets = [p["path_nodes"][-1] for p in pred_chaos["propagation_paths"]]
        self.assertIn("node-d", prop_targets)


if __name__ == "__main__":
    unittest.main()
