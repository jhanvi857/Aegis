"""
Root Cause Analysis Task Head (Phase 3)
Ranks nodes most likely to be the original source of an anomaly or failure cascade.
Matches proto/predict.proto:RootCause schema:
  message RootCause {
    string node_id = 1;
    double probability = 2;
    string explanation = 3;
    repeated string contributing_metrics = 4;
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


class RootCauseAnalyzer:
    """
    Computes root cause candidate rankings with causal metric attribution using TGNN.
    """

    FEATURE_NAMES = [
        "cpu_usage",
        "memory_usage",
        "p95_latency_ms",
        "error_rate",
        "in_degree",
        "out_degree",
        "centrality",
        "status",
    ]

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
            resolved_path = model_checkpoint
            if not os.path.exists(resolved_path):
                alt = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", model_checkpoint))
                if os.path.exists(alt):
                    resolved_path = alt
            if os.path.exists(resolved_path):
                self.checkpoint_path = resolved_path
                self._load_checkpoint(resolved_path)

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

    def analyze(
        self,
        graph_representation: Dict[str, Any],
        telemetry_batch: Optional[Dict[str, Any]] = None,
        top_k: int = 3,
    ) -> List[Dict[str, Any]]:
        """
        Ranks root cause candidates with explanations and contributing metrics.
        """
        if self.model is None and self.checkpoint_path and os.path.exists(self.checkpoint_path):
            self._load_checkpoint(self.checkpoint_path)

        x_t, adj_t = self._prepare_inputs(graph_representation)

        if self.model is not None:
            with torch.no_grad():
                out = self.model(x_t, adj_t)
                rc_probs = out["root_cause_probs"].squeeze(0).cpu().numpy()
        else:
            rc_probs = np.ones(len(self.node_ids) + 1) / (len(self.node_ids) + 1)

        # Check if nominal class (last index) has highest probability
        num_services = len(self.node_ids)
        service_probs = rc_probs[:num_services]

        feature_matrix = graph_representation.get("feature_matrix", {})
        metric_anomalies = np.zeros(num_services)
        for i, nid in enumerate(self.node_ids):
            f = feature_matrix.get(nid, [0] * 8)
            dev = max(0.0, f[0] - 0.5) + max(0.0, f[2] - 0.2) + max(0.0, f[3] - 0.02) + max(0.0, f[7] - 0.1)
            metric_anomalies[i] = dev

        # Combine model probabilities with telemetry anomaly scores
        ranking_scores = service_probs + (metric_anomalies * 0.5)
        ranked_indices = np.argsort(ranking_scores)[::-1]

        candidates = []

        for rank, idx in enumerate(ranked_indices[:top_k]):
            nid = self.node_ids[idx]
            prob = float(service_probs[idx])

            # Determine contributing metrics from feature vector
            # [cpu_norm, mem_norm, latency_norm, error_rate, in_deg, out_deg, centrality, status]
            contributing = []
            fvec = feature_matrix.get(nid, [0] * 8)

            if len(fvec) >= 4:
                if fvec[0] > 0.6:
                    contributing.append("cpu_usage_elevated")
                if fvec[1] > 0.7:
                    contributing.append("memory_leak_pressure")
                if fvec[2] > 0.3:
                    contributing.append("high_p95_latency")
                if fvec[3] > 0.05:
                    contributing.append("anomalous_error_rate")

            if not contributing:
                contributing.append("nominal_telemetry_variance")

            # Formulate causal explanation
            if "high_p95_latency" in contributing and "anomalous_error_rate" in contributing:
                explanation = f"Upstream bottleneck on {nid}: extreme latency and error cascade"
            elif "cpu_usage_elevated" in contributing:
                explanation = f"Compute saturation on {nid}: CPU exhaustion causing backlog"
            elif "memory_leak_pressure" in contributing:
                explanation = f"Memory pressure on {nid}: potential leak nearing allocation limit"
            else:
                explanation = f"Telemetry deviation identified on service {nid}"

            candidates.append({
                "node_id": nid,
                "probability": round(prob, 4),
                "explanation": explanation,
                "contributing_metrics": contributing,
            })

        return candidates
