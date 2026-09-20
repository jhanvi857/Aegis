"""
Temporal Graph Neural Network (TGNN) Architecture (Phase 3)
Combines Spatial Graph Convolutions across dynamic dependency edges with
Temporal GRU sequence modeling to perform multi-task failure prediction,
root cause isolation, and propagation cascade forecasting.
"""

import math
from typing import Dict, Any, Tuple, Optional
import torch
import torch.nn as nn
import torch.nn.functional as F


class GraphConvLayer(nn.Module):
    """
    Spatially propagates node features across weighted, directed graph adjacency matrices.
    Applies symmetric or row normalization with self-loops and residual connections.
    """

    def __init__(self, in_features: int, out_features: int, dropout: float = 0.1):
        super().__init__()
        self.in_features = in_features
        self.out_features = out_features
        self.weight = nn.Linear(in_features, out_features, bias=False)
        self.bias = nn.Parameter(torch.zeros(out_features))
        self.skip = nn.Linear(in_features, out_features) if in_features != out_features else nn.Identity()
        self.norm = nn.LayerNorm(out_features)
        self.dropout = nn.Dropout(dropout)

    def forward(self, x: torch.Tensor, adj: torch.Tensor, node_mask: Optional[torch.Tensor] = None) -> torch.Tensor:
        """
        x: (B, N, in_features)
        adj: (B, N, N)
        node_mask: (B, N) optional binary mask (1 for active nodes, 0 for padded dummy nodes)
        returns: (B, N, out_features)
        """
        batch_size, num_nodes, _ = x.shape

        # Add self-loops to adjacency
        eye = torch.eye(num_nodes, device=adj.device, dtype=adj.dtype).unsqueeze(0).expand(batch_size, -1, -1)
        a_hat = adj + eye

        if node_mask is not None:
            # Mask out connections involving padded dummy nodes
            mask2d = node_mask.unsqueeze(1) * node_mask.unsqueeze(2)  # (B, N, N)
            a_hat = a_hat * mask2d

        # Row normalization: D^-1 * A_hat
        deg = a_hat.sum(dim=-1, keepdim=True).clamp(min=1e-6)
        a_norm = a_hat / deg

        # Message passing: A_norm @ (x @ W)
        support = self.weight(x)
        out = torch.bmm(a_norm, support) + self.bias
        out = F.leaky_relu(out, negative_slope=0.1)
        out = self.norm(out + self.skip(x))
        if node_mask is not None:
            out = out * node_mask.unsqueeze(-1)
        return self.dropout(out)


class TGNNModel(nn.Module):
    """
    Primary multi-task TGNN model supporting dynamic, variable-size graphs:
    Input:
      - node_features: (B, T, N, in_dim)
      - adjacency: (B, T, N, N)
      - node_mask: (B, N) optional binary mask (1 for real nodes, 0 for padded dummy nodes)
    Outputs:
      1. cluster_failure_prob: overall failure risk in [0, 1]
      2. node_failure_probs: per-node failure probability in [0, 1]^N (masked)
      3. confidence: prediction confidence in [0, 1]
      4. root_cause_logits: classification over (N+1) nodes (N = nominal, dummy nodes masked to -1e9)
      5. propagation_probs: blast radius inclusion probability in [0, 1]^N (masked)
      6. time_to_failure: estimated seconds until threshold breach
    """

    def __init__(
        self,
        in_dim: int = 8,
        hidden_dim: int = 64,
        num_nodes: int = 5,
        num_spatial_layers: int = 2,
        dropout: float = 0.1,
    ):
        super().__init__()
        self.in_dim = in_dim
        self.hidden_dim = hidden_dim
        self.num_nodes = num_nodes

        # Spatial layers
        self.spatial_layers = nn.ModuleList()
        current_dim = in_dim
        for _ in range(num_spatial_layers):
            self.spatial_layers.append(GraphConvLayer(current_dim, hidden_dim, dropout=dropout))
            current_dim = hidden_dim

        # Temporal GRU: processes sequence of node embeddings across T steps
        self.temporal_gru = nn.GRU(
            input_size=hidden_dim,
            hidden_size=hidden_dim,
            num_layers=1,
            batch_first=True,
            bidirectional=False,
        )

        # Multi-task heads
        # 1. Failure Prediction
        self.node_failure_head = nn.Sequential(
            nn.Linear(hidden_dim, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
            nn.Sigmoid(),
        )
        self.cluster_failure_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
            nn.Sigmoid(),
        )
        self.confidence_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 16),
            nn.ReLU(),
            nn.Linear(16, 1),
            nn.Sigmoid(),
        )
        self.time_to_failure_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 32),
            nn.ReLU(),
            nn.Linear(32, 1),
            nn.ReLU(),  # TTF cannot be negative
        )

        # 2. Root Cause Identification Head (over N service nodes + 1 nominal class)
        self.root_cause_head = nn.Sequential(
            nn.Linear(hidden_dim, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
        )
        self.nominal_score_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
        )

        # 3. Propagation Prediction Head (probability each node is in downstream cascade)
        self.propagation_head = nn.Sequential(
            nn.Linear(hidden_dim, 32),
            nn.LeakyReLU(0.1),
            nn.Linear(32, 1),
            nn.Sigmoid(),
        )

    def forward(
        self,
        node_features: torch.Tensor,  # (B, T, N, in_dim)
        adjacency: torch.Tensor,      # (B, T, N, N)
        node_mask: Optional[torch.Tensor] = None,  # (B, N)
    ) -> Dict[str, torch.Tensor]:
        batch_size, seq_len, num_nodes, _ = node_features.shape
        if node_mask is None:
            node_mask = torch.ones((batch_size, num_nodes), device=node_features.device, dtype=node_features.dtype)

        # 1. Spatial GNN per timestep
        spatial_embeddings = []
        for t in range(seq_len):
            xt = node_features[:, t, :, :]  # (B, N, in_dim)
            at = adjacency[:, t, :, :]      # (B, N, N)

            ht = xt
            for gconv in self.spatial_layers:
                ht = gconv(ht, at, node_mask=node_mask)
            spatial_embeddings.append(ht)

        # (B, T, N, hidden_dim) -> reshape to (B * N, T, hidden_dim) for node-wise GRU
        spatial_seq = torch.stack(spatial_embeddings, dim=1)
        spatial_seq_reshaped = spatial_seq.permute(0, 2, 1, 3).contiguous().view(
            batch_size * num_nodes, seq_len, self.hidden_dim
        )

        # 2. Temporal sequence modeling
        _, h_n = self.temporal_gru(spatial_seq_reshaped)  # (1, B * N, hidden_dim)
        node_embeddings = h_n.squeeze(0).view(batch_size, num_nodes, self.hidden_dim)  # (B, N, hidden_dim)

        # Mask dummy embeddings
        mask_exp = node_mask.unsqueeze(-1)
        node_embeddings = node_embeddings * mask_exp

        # 3. Graph-level pooling with mask
        mask_sum = node_mask.sum(dim=1, keepdim=True).clamp(min=1.0)
        mean_pool = (node_embeddings * mask_exp).sum(dim=1) / mask_sum  # (B, hidden_dim)
        masked_for_max = torch.where(mask_exp > 0, node_embeddings, torch.full_like(node_embeddings, -1e9))
        max_pool, _ = masked_for_max.max(dim=1)  # (B, hidden_dim)
        graph_pooled = torch.cat([mean_pool, max_pool], dim=-1)  # (B, hidden_dim * 2)

        # 4. Multi-task output heads
        # Task A: Failure Prediction
        node_failures = self.node_failure_head(node_embeddings).squeeze(-1) * node_mask  # (B, N)
        cluster_failure = self.cluster_failure_head(graph_pooled).squeeze(-1)  # (B,)
        confidence = self.confidence_head(graph_pooled).squeeze(-1) * 0.4 + 0.6  # scaled [0.6, 1.0]
        time_to_failure = self.time_to_failure_head(graph_pooled).squeeze(-1)  # (B,)

        # Task B: Root Cause Identification
        node_rc_scores = self.root_cause_head(node_embeddings).squeeze(-1)  # (B, N)
        # Mask dummy nodes with -1e9 so they receive zero probability in softmax
        node_rc_scores = torch.where(node_mask > 0, node_rc_scores, torch.full_like(node_rc_scores, -1e9))
        nominal_score = self.nominal_score_head(graph_pooled)               # (B, 1)
        root_cause_logits = torch.cat([node_rc_scores, nominal_score], dim=-1)  # (B, N + 1)
        root_cause_probs = F.softmax(root_cause_logits, dim=-1)

        # Task C: Propagation Prediction
        propagation_probs = self.propagation_head(node_embeddings).squeeze(-1) * node_mask  # (B, N)

        return {
            "cluster_failure_prob": cluster_failure,
            "node_failure_probs": node_failures,
            "confidence": confidence,
            "time_to_failure": time_to_failure,
            "root_cause_logits": root_cause_logits,
            "root_cause_probs": root_cause_probs,
            "propagation_probs": propagation_probs,
            "node_embeddings": node_embeddings,
        }
