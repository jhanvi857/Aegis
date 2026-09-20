"""
LSTM Baseline (Phase 3)
Sequence-only temporal neural baseline using PyTorch.
Models multivariate time series metrics over time without spatial graph topology
or message passing, serving as the temporal non-graph comparative baseline.
"""

from typing import Dict, Any, Tuple
import torch
import torch.nn as nn
import torch.nn.functional as F


class LSTMBaseline(nn.Module):
    """
    Standard PyTorch LSTM operating on flattened multivariate time series across nodes.
    Lacks graph structure and message passing awareness.
    """

    def __init__(
        self,
        num_nodes: int = 5,
        feature_dim: int = 8,
        hidden_dim: int = 64,
        num_layers: int = 2,
        dropout: float = 0.1,
    ):
        super().__init__()
        self.num_nodes = num_nodes
        self.feature_dim = feature_dim
        self.input_dim = num_nodes * feature_dim
        self.hidden_dim = hidden_dim

        self.lstm = nn.LSTM(
            input_size=self.input_dim,
            hidden_size=hidden_dim,
            num_layers=num_layers,
            batch_first=True,
            dropout=dropout if num_layers > 1 else 0.0,
        )

        # Output heads with sequence pooling over hidden_dim * 2
        self.failure_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
            nn.Sigmoid(),
        )

        self.root_cause_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, num_nodes + 1),  # Nodes + nominal class
        )

    def forward(self, node_features: torch.Tensor) -> Dict[str, torch.Tensor]:
        """
        node_features: (B, T, N, D)
        Flattens into (B, T, N * D) and extracts temporal sequence representation.
        """
        batch_size, seq_len, num_nodes, feat_dim = node_features.shape
        flat_seq = node_features.view(batch_size, seq_len, num_nodes * feat_dim)

        out_seq, _ = self.lstm(flat_seq)  # (B, T, hidden_dim)

        # Sequence pooling: captures multi-step precursor slopes across T
        mean_pool = out_seq.mean(dim=1)
        max_pool, _ = out_seq.max(dim=1)
        pooled = torch.cat([mean_pool, max_pool], dim=-1)  # (B, hidden_dim * 2)

        failure_prob = self.failure_head(pooled).squeeze(-1)  # (B,)
        rc_logits = self.root_cause_head(pooled)              # (B, N+1)
        rc_probs = F.softmax(rc_logits, dim=-1)

        return {
            "cluster_failure_prob": failure_prob,
            "root_cause_logits": rc_logits,
            "root_cause_probs": rc_probs,
        }
