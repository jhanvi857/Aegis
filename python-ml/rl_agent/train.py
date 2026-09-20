"""
Reinforcement Learning Policy Training Loop (Phase 6 Stretch Goal)
Trains a Deep Q-Network policy on MicroserviceChaosEnv to learn autonomous
remediation sequencing under cascading failure dynamics.
"""

import os
import random
import logging
from collections import deque
from typing import Dict, Any
import numpy as np
import torch
import torch.nn as nn
import torch.optim as optim

try:
    from .env import MicroserviceChaosEnv
    from .policy import RecoveryPolicyNetwork
except (ImportError, ValueError):
    from env import MicroserviceChaosEnv
    from policy import RecoveryPolicyNetwork

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)


def train_rl_agent(
    episodes: int = 60,
    topology_path: str = "configs/topology.yaml",
    checkpoint_path: str = "python-ml/models/checkpoints/rl_policy_best.pt",
    seed: int = 42,
) -> Dict[str, Any]:
    """
    Trains RecoveryPolicyNetwork via Deep Q-Learning on MicroserviceChaosEnv.
    """
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)

    env = MicroserviceChaosEnv(topology_path=topology_path, max_steps=20)
    state_dim = env.state_dim
    action_dim = env.action_space_size

    logger.info(f"[rl-train] Initializing RL Training: State Dim={state_dim}, Action Dim={action_dim}")

    policy_net = RecoveryPolicyNetwork(state_dim=state_dim, action_dim=action_dim, hidden_dim=64)
    target_net = RecoveryPolicyNetwork(state_dim=state_dim, action_dim=action_dim, hidden_dim=64)
    target_net.load_state_dict(policy_net.state_dict())
    target_net.eval()

    optimizer = optim.Adam(policy_net.parameters(), lr=1e-3)
    loss_fn = nn.SmoothL1Loss()
    replay_buffer = deque(maxlen=5000)

    batch_size = 32
    gamma = 0.95
    epsilon = 1.0
    epsilon_min = 0.05
    epsilon_decay = 0.94
    target_update_freq = 5

    best_reward = -float("inf")
    episode_rewards = []
    success_count = 0

    for ep in range(1, episodes + 1):
        state = env.reset()
        ep_reward = 0.0
        done = False

        while not done:
            action = policy_net.select_action(state, epsilon=epsilon)
            next_state, reward, done, info = env.step(action)

            replay_buffer.append((state, action, reward, next_state, done))
            state = next_state
            ep_reward += reward

            # Optimize policy network if replay buffer has sufficient transitions
            if len(replay_buffer) >= batch_size:
                batch = random.sample(replay_buffer, batch_size)
                b_state = torch.tensor(np.array([t[0] for t in batch]), dtype=torch.float32)
                b_action = torch.tensor([t[1] for t in batch], dtype=torch.long).unsqueeze(1)
                b_reward = torch.tensor([t[2] for t in batch], dtype=torch.float32).unsqueeze(1)
                b_next_state = torch.tensor(np.array([t[3] for t in batch]), dtype=torch.float32)
                b_done = torch.tensor([float(t[4]) for t in batch], dtype=torch.float32).unsqueeze(1)

                # Current Q-values
                current_q = policy_net(b_state).gather(1, b_action)

                # Target Q-values
                with torch.no_grad():
                    next_max_q = target_net(b_next_state).max(1, keepdim=True)[0]
                    target_q = b_reward + (1.0 - b_done) * gamma * next_max_q

                loss = loss_fn(current_q, target_q)

                optimizer.zero_grad()
                loss.backward()
                optimizer.step()

        episode_rewards.append(ep_reward)
        if info.get("all_healthy", False):
            success_count += 1

        # Decay epsilon
        epsilon = max(epsilon_min, epsilon * epsilon_decay)

        # Update target network
        if ep % target_update_freq == 0:
            target_net.load_state_dict(policy_net.state_dict())

        if ep % 10 == 0 or ep == episodes:
            avg_10 = np.mean(episode_rewards[-10:])
            logger.info(
                f"[rl-train] Episode {ep}/{episodes} | Ep Reward: {ep_reward:.1f} | "
                f"Avg Reward (last 10): {avg_10:.1f} | Epsilon: {epsilon:.2f} | "
                f"Success Rate: {success_count / ep * 100:.1f}%"
            )

        if ep_reward > best_reward:
            best_reward = ep_reward
            policy_net.save(checkpoint_path)

    logger.info(f"[rl-train] RL Training complete. Best checkpoint saved to {checkpoint_path}")

    return {
        "episodes": episodes,
        "best_reward": best_reward,
        "avg_final_reward": float(np.mean(episode_rewards[-10:])),
        "total_successes": success_count,
        "checkpoint_path": checkpoint_path,
    }


if __name__ == "__main__":
    train_rl_agent(episodes=50)
