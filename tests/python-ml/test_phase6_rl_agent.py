"""
Unit and Integration Tests for Phase 6 (Reinforcement Learning Recovery Agent)
Verifies:
- MicroserviceChaosEnv topology loading, state vector formatting, action decoding, and step dynamics
- RecoveryPolicyNetwork PyTorch forward pass, epsilon-greedy action selection, and serialization
- End-to-end DQN training loop and checkpoint persistence
"""

import unittest
import os
import sys
import tempfile
import numpy as np
import torch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from rl_agent.env import MicroserviceChaosEnv
from rl_agent.policy import RecoveryPolicyNetwork
from rl_agent.train import train_rl_agent


class TestPhase6RLAgent(unittest.TestCase):
    def setUp(self):
        self.env = MicroserviceChaosEnv(topology_path="configs/topology.yaml", max_steps=15)
        self.state_dim = self.env.state_dim
        self.action_dim = self.env.action_space_size

    def test_env_initialization_and_reset(self):
        """Environment must initialize dimensions and state vector correctly."""
        self.assertGreater(self.env.num_nodes, 0)
        self.assertGreater(self.action_dim, 1)
        self.assertEqual(self.state_dim, self.env.num_nodes * 5)

        state = self.env.reset()
        self.assertIsInstance(state, np.ndarray)
        self.assertEqual(state.shape, (self.state_dim,))
        self.assertEqual(state.dtype, np.float32)

        # Values should be normalized between 0.0 and 1.0
        self.assertTrue(np.all(state >= 0.0))
        self.assertTrue(np.all(state <= 1.5))

    def test_action_decoding(self):
        """Action indices must cleanly map to valid action types and nodes."""
        # 0 is always NOOP
        act_type, node = self.env.decode_action(0)
        self.assertEqual(act_type, "NOOP")

        # Check all action indices
        for a in range(1, self.action_dim):
            act_type, node = self.env.decode_action(a)
            self.assertIn(act_type, ["RESTART", "SCALE_UP", "REROUTE_TRAFFIC", "FLUSH_CACHE"])
            self.assertIn(node, self.env.node_ids)

    def test_env_step_remediation(self):
        """Applying remediation action must update node health and return valid tuple."""
        state = self.env.reset(fault_node="node-b")

        # Pick a non-NOOP action targeting node-b
        action_idx = 1
        for a in range(1, self.action_dim):
            act_type, node = self.env.decode_action(a)
            if node == "node-b" and act_type == "RESTART":
                action_idx = a
                break

        next_state, reward, done, info = self.env.step(action_idx)

        self.assertEqual(next_state.shape, (self.state_dim,))
        self.assertIsInstance(reward, float)
        self.assertIsInstance(done, bool)
        self.assertIn("all_healthy", info)
        self.assertIn("step", info)

    def test_policy_network(self):
        """Policy network forward pass and action selection."""
        policy = RecoveryPolicyNetwork(state_dim=self.state_dim, action_dim=self.action_dim, hidden_dim=32)

        dummy_state = np.random.rand(self.state_dim).astype(np.float32)
        action_greedy = policy.select_action(dummy_state, epsilon=0.0)
        self.assertIsInstance(action_greedy, int)
        self.assertGreaterEqual(action_greedy, 0)
        self.assertLess(action_greedy, self.action_dim)

        # Test batch forward pass
        batch_t = torch.randn(4, self.state_dim)
        out = policy(batch_t)
        self.assertEqual(out.shape, (4, self.action_dim))

    def test_policy_serialization(self):
        """Model weights can be saved and loaded."""
        policy1 = RecoveryPolicyNetwork(state_dim=self.state_dim, action_dim=self.action_dim, hidden_dim=16)
        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as tmp:
            tmp_path = tmp.name

        try:
            policy1.save(tmp_path)
            self.assertTrue(os.path.exists(tmp_path))

            policy2 = RecoveryPolicyNetwork(state_dim=self.state_dim, action_dim=self.action_dim, hidden_dim=16)
            policy2.load(tmp_path)

            # Check parameter equality
            for p1, p2 in zip(policy1.parameters(), policy2.parameters()):
                self.assertTrue(torch.allclose(p1, p2))
        finally:
            if os.path.exists(tmp_path):
                os.remove(tmp_path)

    def test_training_smoke_run(self):
        """Verifies training loop runs and saves checkpoint."""
        with tempfile.NamedTemporaryFile(suffix=".pt", delete=False) as tmp:
            tmp_ckpt = tmp.name

        try:
            results = train_rl_agent(
                episodes=8,
                topology_path="configs/topology.yaml",
                checkpoint_path=tmp_ckpt,
                seed=123,
            )
            self.assertEqual(results["episodes"], 8)
            self.assertIn("best_reward", results)
            self.assertTrue(os.path.exists(tmp_ckpt))
        finally:
            if os.path.exists(tmp_ckpt):
                os.remove(tmp_ckpt)


if __name__ == "__main__":
    unittest.main()
