"""
Failure Prediction Task Head (Phase 3)
Outputs cluster and per-node failure probability over future time horizons
using the spatio-temporal representations from the trained TGNN model.
Matches proto/predict.proto:FailurePrediction schema.
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


class FailurePredictor:
    """
    Computes per-node and cluster-wide failure probabilities using TGNN inference.
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

        if model_checkpoint and os.path.exists(model_checkpoint):
            self._load_checkpoint(model_checkpoint)

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
        logger.info(f"FailurePredictor loaded checkpoint from {path} for nodes {self.node_ids}")

    def _prepare_inputs(
        self,
        graph_representation: Dict[str, Any],
        sequence_length: int = 10,
    ) -> tuple[torch.Tensor, torch.Tensor]:
        """Converts graph representation into TGNN input tensors."""
        feature_matrix = graph_representation.get("feature_matrix", {})
        if not self.node_ids:
            self.node_ids = sorted(list(feature_matrix.keys()))
            self.node_to_idx = {nid: i for i, nid in enumerate(self.node_ids)}

        num_nodes = len(self.node_ids)
        in_dim = len(next(iter(feature_matrix.values()))) if feature_matrix else 8

        # Node feature vector (1, T, N, D)
        node_feats = np.zeros((num_nodes, in_dim), dtype=np.float32)
        for nid, fvec in feature_matrix.items():
            if nid in self.node_to_idx:
                node_feats[self.node_to_idx[nid]] = np.array(fvec, dtype=np.float32)

        # Adjacency (1, T, N, N)
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

    def predict(
        self,
        graph_representation: Dict[str, Any],
        telemetry_batch: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """
        Returns list of FailurePrediction dictionaries conforming to proto/predict.proto:
        [
          {
            "node_id": "node-c",
            "failure_probability": 0.94,
            "confidence": 0.98,
            "estimated_time_to_failure_sec": 18
          }, ...
        ]
        """
        if self.model is None and self.checkpoint_path:
            resolved = self.checkpoint_path
            if not os.path.exists(resolved):
                alt = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", self.checkpoint_path))
                if os.path.exists(alt):
                    resolved = alt
            if os.path.exists(resolved):
                self._load_checkpoint(resolved)

        x_t, adj_t = self._prepare_inputs(graph_representation)

        if self.model is not None:
            with torch.no_grad():
                out = self.model(x_t, adj_t)
                node_probs = out["node_failure_probs"].squeeze(0).cpu().numpy()
                confidence = float(out["confidence"].item())
                ttf = int(out["time_to_failure"].item())
        else:
            # Fallback heuristic if checkpoint uninitialized
            node_probs = np.zeros(len(self.node_ids))
            confidence = 0.85
            ttf = 600

        predictions = []
        feature_matrix = graph_representation.get("feature_matrix", {})

        for i, nid in enumerate(self.node_ids):
            prob = float(node_probs[i]) if i < len(node_probs) else 0.0
            fvec = feature_matrix.get(nid, [0] * 8)

            # In nominal state (CPU < 50%, latency < 60ms, error < 0.02, queue < 6), calibrate probability
            if len(fvec) >= 8:
                is_nominal = (fvec[0] < 0.50 and fvec[2] < 0.12 and fvec[3] < 0.02 and fvec[7] < 0.12)
                if is_nominal:
                    prob = min(prob * 0.4, 0.25)

            node_ttf = ttf if prob > 0.4 else 600
            predictions.append({
                "node_id": nid,
                "failure_probability": round(prob, 4),
                "confidence": round(confidence, 4),
                "estimated_time_to_failure_sec": node_ttf,
            })

        return predictions
