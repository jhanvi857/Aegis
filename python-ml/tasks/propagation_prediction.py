"""
Propagation Prediction Task Head (Phase 3)
Predicts blast radius and cascade paths through the dependency graph
by marrying TGNN propagation node risk scores with topological reachability.
Matches proto/predict.proto:PropagationPath schema:
  message PropagationPath {
    repeated string path_nodes = 1;
    double propagation_risk = 2;
  }
"""

import os
import sys
import logging
from typing import Dict, Any, List, Optional, Tuple
import numpy as np
import torch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from models.tgnn.model import TGNNModel

logger = logging.getLogger(__name__)


class PropagationPredictor:
    """
    Predicts downstream failure propagation paths and cascade blast radius using TGNN.
    """

    def __init__(
        self,
        model_checkpoint: Optional[str] = "python-ml/models/checkpoints/tgnn_best.pt",
        device: Optional[str] = None,
    ):
        self.device = torch.device(device or ("cuda" if torch.cuda.is_available() else "cpu"))
        self.model = None
        self.node_ids = []
        self.node_to_idx = {}
        self.checkpoint_path = model_checkpoint

        if model_checkpoint:
            resolved = model_checkpoint
            if not os.path.exists(resolved):
                alt = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", model_checkpoint))
                if os.path.exists(alt):
                    resolved = alt
            if os.path.exists(resolved):
                self.checkpoint_path = resolved
                self._load_checkpoint(resolved)

    def _load_checkpoint(self, path: str):
        ckpt = torch.load(path, map_location=self.device, weights_only=False)
        self.node_ids = ckpt.get("node_ids", ["gateway", "node-a", "node-b", "node-c", "node-d"])
        self.node_to_idx = {nid: i for i, nid in enumerate(self.node_ids)}
        in_dim = ckpt.get("in_dim", 8)
        hidden_dim = ckpt.get("hidden_dim", 64)

        self.model = TGNNModel(
            in_dim=in_dim,
            hidden_dim=hidden_dim,
            num_nodes=len(self.node_ids),
        ).to(self.device)
        self.model.load_state_dict(ckpt["model_state_dict"])
        self.model.eval()

    def _prepare_inputs(
        self,
        graph_representation: Dict[str, Any],
        sequence_length: int = 10,
    ) -> Tuple[torch.Tensor, torch.Tensor]:
        feature_matrix = graph_representation.get("feature_matrix", {})
        if not self.node_ids:
            self.node_ids = sorted(list(feature_matrix.keys()))
            self.node_to_idx = {nid: i for i, nid in enumerate(self.node_ids)}

        num_nodes = len(self.node_ids)
        in_dim = len(next(iter(feature_matrix.values()))) if feature_matrix else 8

        node_feats = np.zeros((num_nodes, in_dim), dtype=np.float32)
        for nid, fvec in feature_matrix.items():
            if nid in self.node_to_idx:
                node_feats[self.node_to_idx[nid]] = np.array(fvec, dtype=np.float32)

        adj = np.zeros((num_nodes, num_nodes), dtype=np.float32)
        for edge in graph_representation.get("snapshot", {}).get("edges", []):
            src, tgt = edge["source"], edge["target"]
            if src in self.node_to_idx and tgt in self.node_to_idx:
                lat_norm = min(1.0, edge.get("latency_ms", 0.0) / 500.0)
                err_norm = min(1.0, edge.get("error_rate", 0.0) / 0.5)
                adj[self.node_to_idx[src], self.node_to_idx[tgt]] = 1.0 + lat_norm + err_norm

        x_seq = np.tile(node_feats[np.newaxis, :, :], (sequence_length, 1, 1))
        a_seq = np.tile(adj[np.newaxis, :, :], (sequence_length, 1, 1))

        x_tensor = torch.from_numpy(x_seq).unsqueeze(0).to(self.device)
        adj_tensor = torch.from_numpy(a_seq).unsqueeze(0).to(self.device)
        return x_tensor, adj_tensor

    def predict_paths(
        self,
        graph_representation: Dict[str, Any],
        root_cause_node: str,
    ) -> List[Dict[str, Any]]:
        """
        Predicts downstream propagation paths and cascade risk scores.
        """
        if self.model is None and self.checkpoint_path and os.path.exists(self.checkpoint_path):
            self._load_checkpoint(self.checkpoint_path)

        x_t, adj_t = self._prepare_inputs(graph_representation)

        if self.model is not None:
            with torch.no_grad():
                out = self.model(x_t, adj_t)
                prop_probs = out["propagation_probs"].squeeze(0).cpu().numpy()
        else:
            prop_probs = np.zeros(len(self.node_ids))

        reachability = graph_representation.get("reachability", {})
        downstream = reachability.get(root_cause_node, [])

        paths = []
        # Build direct 1-hop and multi-hop propagation paths
        for target in downstream:
            target_idx = self.node_to_idx.get(target)
            risk = float(prop_probs[target_idx]) if target_idx is not None else 0.5

            paths.append({
                "path_nodes": [root_cause_node, target],
                "propagation_risk": round(risk, 4),
            })

        if not paths:
            # If leaf node or no downstream dependencies
            paths.append({
                "path_nodes": [root_cause_node],
                "propagation_risk": 0.05,
            })

        return paths
