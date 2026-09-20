"""
Unit and Integration Tests for Baseline Models & Multi-Topology Generalization
Verifies:
1. HeuristicThresholdBaseline and MajorityClassBaseline predict expected output formats.
2. TGNNModel correctly utilizes node_mask to mask padded dummy nodes.
3. Multi-topology dataset generation and unseen cross-topology benchmark metrics.
"""

import os
import sys
import unittest
import numpy as np
import torch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from models.heuristic.heuristic_baseline import HeuristicThresholdBaseline, MajorityClassBaseline
from models.tgnn.model import TGNNModel
from dataset.multi_topology import MultiTopologyGenerator, MultiTopologyDataset, TOPOLOGIES


class TestBaselinesAndMultiTopology(unittest.TestCase):

    def setUp(self):
        np.random.seed(42)
        torch.manual_seed(42)

    def test_01_heuristic_threshold_baseline(self):
        """Test static threshold alerting baseline."""
        baseline = HeuristicThresholdBaseline(cpu_threshold=0.6, lat_threshold=0.15)
        
        # Nominal batch: (B=2, T=10, N=5, D=8) with low metrics
        nom_x = np.zeros((2, 10, 5, 8), dtype=np.float32)
        nom_x[..., 0] = 0.25 # 25% CPU
        nom_x[..., 2] = 0.05 # 25ms latency
        
        preds_nom = baseline.predict(nom_x)
        self.assertEqual(preds_nom["failure_probs"][0], 0.0)
        self.assertEqual(preds_nom["failure_probs"][1], 0.0)
        # Root cause for nominal should be index N=5
        self.assertEqual(np.argmax(preds_nom["root_cause_probs"][0]), 5)

        # Fault batch: node 2 breaches CPU threshold
        fault_x = np.copy(nom_x)
        fault_x[0, 5:, 2, 0] = 0.85 # 85% CPU on node 2
        preds_fault = baseline.predict(fault_x)
        self.assertEqual(preds_fault["failure_probs"][0], 1.0)
        self.assertEqual(np.argmax(preds_fault["root_cause_probs"][0]), 2)
        self.assertEqual(preds_fault["propagation_probs"].shape, (2, 5))

    def test_02_majority_class_baseline(self):
        """Test naive majority class baseline."""
        maj = MajorityClassBaseline()
        y_fail = np.array([1, 1, 1, 0, 1])
        y_rc = np.array([2, 2, 1, 5, 2])
        maj.fit(y_fail, y_rc, num_nodes=5)

        preds = maj.predict(batch_size=4, num_nodes=5)
        self.assertEqual(preds["failure_probs"].tolist(), [1.0, 1.0, 1.0, 1.0])
        self.assertEqual(np.argmax(preds["root_cause_probs"][0]), 2)

    def test_03_tgnn_node_mask_padding(self):
        """Verify TGNN accurately masks zero-padded dummy nodes and excludes them from logits."""
        B, T, N_max, D = 2, 10, 8, 8
        x = torch.randn(B, T, N_max, D)
        adj = torch.rand(B, T, N_max, N_max)

        # Active: Sample 0 has 5 nodes, Sample 1 has 4 nodes
        mask = torch.zeros(B, N_max)
        mask[0, :5] = 1.0
        mask[1, :4] = 1.0

        model = TGNNModel(in_dim=D, hidden_dim=32, num_nodes=N_max)
        out = model(x, adj, node_mask=mask)

        # 1. Padded nodes in node_failure_probs must be 0.0
        node_probs = out["node_failure_probs"]
        self.assertEqual(node_probs[0, 5:].sum().item(), 0.0)
        self.assertEqual(node_probs[1, 4:].sum().item(), 0.0)

        # 2. Padded nodes in propagation_probs must be 0.0
        prop_probs = out["propagation_probs"]
        self.assertEqual(prop_probs[0, 5:].sum().item(), 0.0)
        self.assertEqual(prop_probs[1, 4:].sum().item(), 0.0)

        # 3. Softmax probabilities on padded slots must be effectively 0.0 (< 1e-6)
        rc_probs = out["root_cause_probs"]
        self.assertLess(rc_probs[0, 5:N_max].sum().item(), 1e-6)
        self.assertLess(rc_probs[1, 4:N_max].sum().item(), 1e-6)

    def test_04_multi_topology_generation_smoke(self):
        """Smoke test generating small multi-topology episodes and verifies padding shapes."""
        generator = MultiTopologyGenerator(max_nodes=7, sequence_length=5, feature_dim=8)
        topo_b = TOPOLOGIES["topology_b"]
        ep_data = generator.generate_topology_episodes(
            topo_key="topology_b",
            topo_info=topo_b,
            episodes_per_topo=5,
            seed=42,
        )

        self.assertEqual(ep_data["node_features"].shape, (5, 5, 7, 8))
        self.assertEqual(ep_data["adjacency"].shape, (5, 5, 7, 7))
        self.assertEqual(ep_data["node_mask"].shape, (5, 7))
        # Topology B has 4 real nodes
        self.assertEqual(ep_data["node_mask"][0, :4].sum().item(), 4.0)
        self.assertEqual(ep_data["node_mask"][0, 4:].sum().item(), 0.0)


if __name__ == "__main__":
    unittest.main()
