"""
Sparse Temporal Graph Neural Network (Sparse-TGNN) Architecture (Phase 4)
Uses sparse edge_index message passing (O(|V| + |E|)) instead of dense N x N matrices.
Scales efficiently to hundreds of microservices.
"""

from typing import Dict, Any, Tuple, Optional, List
import torch
import torch.nn as nn
import torch.nn.functional as F


class SparseGATv2Layer(nn.Module):
    """
    Sparse Graph Attention Network v2 (GATv2) layer.
    Operates on node features x: (N, in_features) and edge_index: (2, E).
    Memory complexity: O(|V| + |E|), linear in graph size.
    """

    def __init__(
        self,
        in_features: int,
        out_features: int,
        heads: int = 2,
        dropout: float = 0.1,
        negative_slope: float = 0.2,
    ):
        super().__init__()
        self.in_features = in_features
        self.out_features = out_features
        self.heads = heads
        self.head_dim = out_features // heads
        self.negative_slope = negative_slope

        self.lin_src = nn.Linear(in_features, out_features, bias=False)
        self.lin_dst = nn.Linear(in_features, out_features, bias=False)
        self.attn_vec = nn.Parameter(torch.empty(heads, self.head_dim))
        nn.init.xavier_uniform_(self.attn_vec.unsqueeze(0))

        self.skip = nn.Linear(in_features, out_features) if in_features != out_features else nn.Identity()
        self.norm = nn.LayerNorm(out_features)
        self.dropout = nn.Dropout(dropout)

    def forward(
        self,
        x: torch.Tensor,
        edge_index: torch.Tensor,
        edge_weight: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        """
        x: (N, in_features)
        edge_index: (2, E) directed edges [src, dst]
        edge_weight: optional (E,) edge scalars
        returns: (N, out_features)
        """
        num_nodes = x.size(0)

        # 1. Ensure self-loops exist so nodes attend to themselves
        device = x.device
        loop_index = torch.arange(0, num_nodes, dtype=torch.long, device=device)
        loop_edge = torch.stack([loop_index, loop_index], dim=0)

        # Filter out existing self-loops to prevent duplicate loops
        mask = edge_index[0] != edge_index[1]
        clean_edge_index = edge_index[:, mask]
        all_edge_index = torch.cat([clean_edge_index, loop_edge], dim=1)

        src, dst = all_edge_index[0], all_edge_index[1]
        num_edges = all_edge_index.size(1)

        # 2. Linear projection for source and destination nodes
        h_src = self.lin_src(x).view(num_nodes, self.heads, self.head_dim)
        h_dst = self.lin_dst(x).view(num_nodes, self.heads, self.head_dim)

        # 3. Gather source and destination features for each directed edge
        e_src = h_src[src]  # (E, H, D)
        e_dst = h_dst[dst]  # (E, H, D)

        # 4. GATv2 attention scoring: a^T LeakyReLU(W_src h_i + W_dst h_j)
        e_comb = F.leaky_relu(e_src + e_dst, negative_slope=self.negative_slope)
        # Inner product with attention vector: (E, H)
        alpha = (e_comb * self.attn_vec.unsqueeze(0)).sum(dim=-1)

        # 5. Numerically stable segmented softmax over dst nodes using out-of-place operations
        max_list = []
        for h in range(self.heads):
            m_h = torch.full((num_nodes,), -1e9, device=device)
            m_h = m_h.scatter_reduce(0, dst, alpha[:, h], reduce="amax", include_self=False)
            max_list.append(m_h)
        max_scores = torch.stack(max_list, dim=-1)  # (N, H)
        alpha_norm = alpha - max_scores[dst]
        exp_alpha = torch.exp(alpha_norm)

        # Sum exponentials per destination node
        sum_list = []
        for h in range(self.heads):
            s_h = torch.zeros((num_nodes,), device=device)
            s_h = s_h.scatter_add(0, dst, exp_alpha[:, h])
            sum_list.append(s_h)
        sum_exp = torch.stack(sum_list, dim=-1)
        sum_exp = torch.clamp(sum_exp, min=1e-8)

        # Normalized attention coefficients
        attn_weights = exp_alpha / sum_exp[dst]  # (E, H)
        attn_weights = self.dropout(attn_weights)

        # 6. Aggregate weighted messages into dst nodes
        msg = e_src * attn_weights.unsqueeze(-1)  # (E, H, D)
        out_list = []
        for h in range(self.heads):
            o_h = torch.zeros((num_nodes, self.head_dim), device=device)
            o_h = o_h.index_add(0, dst, msg[:, h])
            out_list.append(o_h)
        out = torch.stack(out_list, dim=1)  # (N, H, D)

        out = out.reshape(num_nodes, self.out_features)
        out = self.norm(self.skip(x) + out)
        return F.elu(out)


class SparseTGNN(nn.Module):
    """
    Sparse Temporal Graph Neural Network.
    Combines Spatial Sparse GATv2 layers with Temporal GRU sequence modeling.
    Complexity: O(|V| + |E|) per time step.
    """

    def __init__(
        self,
        node_in_features: int = 8,
        hidden_dim: int = 32,
        heads: int = 2,
        temporal_window: int = 5,
        dropout: float = 0.1,
    ):
        super().__init__()
        self.node_in_features = node_in_features
        self.hidden_dim = hidden_dim
        self.temporal_window = temporal_window

        # Spatial GAT layers
        self.gat1 = SparseGATv2Layer(node_in_features, hidden_dim, heads=heads, dropout=dropout)
        self.gat2 = SparseGATv2Layer(hidden_dim, hidden_dim, heads=heads, dropout=dropout)

        # Temporal Recurrent Layer
        self.gru = nn.GRU(hidden_dim, hidden_dim, batch_first=True)

        # Multi-task output heads
        self.failure_head = nn.Sequential(
            nn.Linear(hidden_dim, 16),
            nn.ReLU(),
            nn.Linear(16, 1),
            nn.Sigmoid(),
        )

        self.root_cause_head = nn.Sequential(
            nn.Linear(hidden_dim, 16),
            nn.ReLU(),
            nn.Linear(16, 1),
        )

        self.propagation_head = nn.Sequential(
            nn.Linear(hidden_dim * 2, 16),
            nn.ReLU(),
            nn.Linear(16, 1),
            nn.Sigmoid(),
        )

    def forward(
        self,
        x_seq: torch.Tensor,
        edge_index: torch.Tensor,
    ) -> Dict[str, torch.Tensor]:
        """
        x_seq: (T, N, in_features) temporal sequence of node feature snapshots
        edge_index: (2, E) static or snapshot edge list
        returns:
            failure_prob: scalar [0, 1]
            root_cause_probs: (N,) softmax distribution over nodes
            propagation_probs: (E,) cascade probabilities per edge
            node_embeddings: (N, hidden_dim) final temporal embeddings
        """
        T, N, _ = x_seq.shape

        # 1. Process each time slice through Sparse GATv2
        spatial_embeddings = []
        for t in range(T):
            h_t = self.gat1(x_seq[t], edge_index)
            h_t = self.gat2(h_t, edge_index)
            spatial_embeddings.append(h_t)

        # Stack over time: (N, T, hidden_dim)
        h_seq = torch.stack(spatial_embeddings, dim=1)

        # 2. Temporal modeling across time window for each node
        gru_out, h_n = self.gru(h_seq)
        node_embed = h_n.squeeze(0)  # (N, hidden_dim)

        # 3. Graph-level pooling for Failure Prediction
        graph_embed = torch.mean(node_embed, dim=0, keepdim=True)  # (1, hidden_dim)
        failure_prob = self.failure_head(graph_embed).squeeze()

        # 4. Per-node root cause classification logits & softmax
        rc_logits = self.root_cause_head(node_embed).squeeze(-1)  # (N,)
        root_cause_probs = F.softmax(rc_logits, dim=-1)

        # 5. Per-edge propagation prediction
        if edge_index.size(1) > 0:
            src, dst = edge_index[0], edge_index[1]
            edge_feat = torch.cat([node_embed[src], node_embed[dst]], dim=-1)
            propagation_probs = self.propagation_head(edge_feat).squeeze(-1)
        else:
            propagation_probs = torch.zeros(0, device=x_seq.device)

        return {
            "failure_prob": failure_prob,
            "root_cause_probs": root_cause_probs,
            "propagation_probs": propagation_probs,
            "node_embeddings": node_embed,
        }
