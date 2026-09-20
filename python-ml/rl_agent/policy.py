"""
Policy Network for Reinforcement Learning Recovery Agent (Phase 6)
PyTorch Deep Q-Network / Policy for selecting optimal microservice remediation actions.
"""

import os
import random
from typing import Union
import numpy as np
import torch
import torch.nn as nn
import torch.nn.functional as F


class RecoveryPolicyNetwork(nn.Module):
    """
    Deep Q-Network parameterized to map microservice state representation vectors
    to expected remediation action returns.
    """

    def __init__(self, state_dim: int, action_dim: int, hidden_dim: int = 128):
        super().__init__()
        self.state_dim = state_dim
        self.action_dim = action_dim

        self.net = nn.Sequential(
            nn.Linear(state_dim, hidden_dim),
            nn.LayerNorm(hidden_dim),
            nn.ReLU(),
            nn.Linear(hidden_dim, hidden_dim // 2),
            nn.LayerNorm(hidden_dim // 2),
            nn.ReLU(),
            nn.Linear(hidden_dim // 2, action_dim),
        )

    def forward(self, state: torch.Tensor) -> torch.Tensor:
        """Forward pass returns Q-values for all discrete actions."""
        return self.net(state)

    def select_action(self, state: Union[np.ndarray, torch.Tensor], epsilon: float = 0.0) -> int:
        """Epsilon-greedy action selection."""
        if random.random() < epsilon:
            return random.randint(0, self.action_dim - 1)

        self.eval()
        with torch.no_grad():
            if isinstance(state, np.ndarray):
                state_t = torch.from_numpy(state).float().unsqueeze(0)
            elif state.dim() == 1:
                state_t = state.unsqueeze(0)
            else:
                state_t = state

            q_values = self.forward(state_t)
            action = int(torch.argmax(q_values, dim=1).item())
        return action

    def save(self, filepath: str):
        """Saves model weights to checkpoint file."""
        os.makedirs(os.path.dirname(os.path.abspath(filepath)), exist_ok=True)
        torch.save(self.state_dict(), filepath)

    def load(self, filepath: str):
        """Loads model weights from checkpoint file."""
        if os.path.exists(filepath):
            self.load_state_dict(torch.load(filepath, map_location="cpu"))
