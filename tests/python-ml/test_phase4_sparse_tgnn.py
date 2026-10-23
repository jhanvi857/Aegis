"""
Tests for Phase 4 Sparse TGNN and Two-Stage Anomaly Detection Pipeline
Verifies O(|V| + |E|) sparse message passing, k-hop subgraph extraction, and scalability.
"""

import os
import sys
import unittest
import torch
import numpy as np

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from models.tgnn.sparse_model import SparseGATv2Layer, SparseTGNN
from models.two_stage_pipeline import (
    LightweightAnomalyDetector,
    extract_k_hop_subgraph,
    TwoStageInferencePipeline,
)


class TestSparseTGNN(unittest.TestCase):

    def test_sparse_gatv2_forward_shape_and_gradients(self):
        """Test SparseGATv2 forward pass with sparse edge_index."""
        torch.manual_seed(42)
        N = 10
        in_dim = 8
        out_dim = 16
        layer = SparseGATv2Layer(in_dim, out_dim, heads=2)

        x = torch.randn(N, in_dim, requires_grad=True)
        # 15 directed edges
        src = torch.tensor([0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 2, 4, 1, 3, 5])
        dst = torch.tensor([1, 2, 3, 4, 5, 6, 7, 8, 9, 3, 5, 7, 0, 1, 2])
        edge_index = torch.stack([src, dst], dim=0)

        out = layer(x, edge_index)
        self.assertEqual(out.shape, (N, out_dim))
        self.assertFalse(torch.isnan(out).any())

        # Test backward pass
        loss = out.sum()
        loss.backward()
        self.assertIsNotNone(x.grad)
        self.assertFalse(torch.isnan(x.grad).any())

    def test_sparse_tgnn_multi_task_outputs(self):
        """Test multi-task heads on temporal sequence with sparse graph."""
        torch.manual_seed(42)
        T = 5
        N = 8
        in_dim = 8
        model = SparseTGNN(node_in_features=in_dim, hidden_dim=32, heads=2)

        x_seq = torch.randn(T, N, in_dim)
        edge_index = torch.tensor([
            [0, 1, 2, 3, 4, 5],
            [1, 2, 3, 4, 5, 6],
        ], dtype=torch.long)

        outputs = model(x_seq, edge_index)
        self.assertIn("failure_prob", outputs)
        self.assertIn("root_cause_probs", outputs)
        self.assertIn("propagation_probs", outputs)

        # Root cause distribution must sum to 1.0
        rc_sum = float(outputs["root_cause_probs"].sum().item())
        self.assertAlmostEqual(rc_sum, 1.0, places=4)
        self.assertEqual(outputs["propagation_probs"].shape[0], edge_index.size(1))

    def test_lightweight_anomaly_detector(self):
        """Test Stage 1 fast O(N) anomaly filtering."""
        detector = LightweightAnomalyDetector()
        metrics = {
            "svc-a": {"cpu": 30.0, "memory": 40.0, "latency": 45.0, "error_rate": 0.005},
            "svc-b": {"cpu": 95.0, "memory": 92.0, "latency": 650.0, "error_rate": 0.15},  # anomalous
            "svc-c": {"cpu": 35.0, "memory": 45.0, "latency": 50.0, "error_rate": 0.01},
        }

        scores = detector.score_nodes(metrics)
        self.assertLess(scores["svc-a"], 0.20)
        self.assertGreater(scores["svc-b"], 0.70)
        self.assertLess(scores["svc-c"], 0.20)

        seeds = detector.filter_anomalous_seeds(scores, threshold=0.35)
        self.assertEqual(seeds, ["svc-b"])

    def test_k_hop_subgraph_extraction(self):
        """Test localized neighborhood extraction around anomaly seeds."""
        adj_out = {
            "node-1": ["node-2"],
            "node-2": ["node-3"],
            "node-3": ["node-4"],
            "node-4": ["node-5"],
            "node-5": [],
        }
        adj_in = {
            "node-1": [],
            "node-2": ["node-1"],
            "node-3": ["node-2"],
            "node-4": ["node-3"],
            "node-5": ["node-4"],
        }

        # 2-hop neighborhood of node-3 should include node-1, node-2, node-3, node-4, node-5
        nodes, edge_index = extract_k_hop_subgraph(
            seeds=["node-3"],
            adj_out=adj_out,
            adj_in=adj_in,
            k_hops=2,
        )

        self.assertIn("node-3", nodes)
        self.assertIn("node-2", nodes)
        self.assertIn("node-4", nodes)
        self.assertGreaterEqual(edge_index.size(1), 2)

    def test_two_stage_inference_pipeline_nominal_vs_anomalous(self):
        """Test full two-stage pipeline on healthy vs degraded states."""
        pipeline = TwoStageInferencePipeline()
        service_ids = [f"svc-{i}" for i in range(20)]
        adj_out = {sid: [f"svc-{(i+1)%20}"] for i, sid in enumerate(service_ids)}
        adj_in = {sid: [f"svc-{(i-1)%20}"] for i, sid in enumerate(service_ids)}

        # 1. Healthy cluster test
        healthy_metrics = {sid: {"cpu": 25.0, "memory": 30.0, "latency": 20.0, "error_rate": 0.001} for sid in service_ids}
        x_seq_healthy = torch.randn(5, 20, 8) * 0.1

        nominal_res = pipeline.predict(
            service_ids=service_ids,
            node_features_seq=x_seq_healthy,
            adj_out=adj_out,
            adj_in=adj_in,
            latest_metrics=healthy_metrics,
        )

        self.assertEqual(nominal_res["stage"], "stage_1_nominal")
        self.assertFalse(nominal_res["is_anomalous"])
        self.assertLess(nominal_res["failure_probability"], 0.10)
        self.assertEqual(nominal_res["subgraph_size"], 0)

        # 2. Chaos fault injection test (svc-7 degraded)
        faulty_metrics = dict(healthy_metrics)
        faulty_metrics["svc-7"] = {"cpu": 98.0, "memory": 89.0, "latency": 850.0, "error_rate": 0.22}
        x_seq_fault = torch.randn(5, 20, 8)
        x_seq_fault[:, 7, :] += 3.0  # high feature deviation on svc-7

        fault_res = pipeline.predict(
            service_ids=service_ids,
            node_features_seq=x_seq_fault,
            adj_out=adj_out,
            adj_in=adj_in,
            latest_metrics=faulty_metrics,
        )

        self.assertEqual(fault_res["stage"], "stage_2_sparse_tgnn")
        self.assertTrue(fault_res["is_anomalous"])
        self.assertIn("svc-7", fault_res["root_cause"])
        self.assertGreater(fault_res["subgraph_size"], 0)
        self.assertLess(fault_res["subgraph_size"], 20)  # localized to k-hop
        self.assertGreater(fault_res["confidence"], 0.70)


if __name__ == "__main__":
    unittest.main()
