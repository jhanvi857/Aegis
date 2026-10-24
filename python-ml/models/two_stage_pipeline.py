"""
Two-Stage Scalable Anomaly Detection & Failure Prediction Pipeline (Phase 4)
Stage 1: Cheap O(N) per-node statistical anomaly detector across all N services.
Stage 2: Dynamic k-hop subgraph extraction + Sparse-TGNN inference on the anomalous neighborhood.
"""

from typing import Dict, Any, List, Tuple, Set, Optional
import collections
import torch
import torch.nn.functional as F

from models.tgnn.sparse_model import SparseTGNN


class LightweightAnomalyDetector:
    """
    Stage 1 Filter: Cheap vectorized O(N) anomaly detector.
    Evaluates metric telemetry vectors against baseline distribution thresholds.
    """

    def __init__(
        self,
        cpu_threshold: float = 75.0,
        mem_threshold: float = 80.0,
        latency_threshold_ms: float = 250.0,
        error_rate_threshold: float = 0.05,
    ):
        self.cpu_threshold = cpu_threshold
        self.mem_threshold = mem_threshold
        self.latency_threshold_ms = latency_threshold_ms
        self.error_rate_threshold = error_rate_threshold

    def score_nodes(self, node_metrics: Dict[str, Dict[str, float]]) -> Dict[str, float]:
        """
        node_metrics: {service_id: {"cpu": float, "memory": float, "latency": float, "error_rate": float}}
        returns: {service_id: anomaly_score in [0, 1]}
        """
        scores: Dict[str, float] = {}
        for svc_id, m in node_metrics.items():
            cpu = m.get("cpu", 0.0)
            mem = m.get("memory", 0.0)
            lat = m.get("latency", 0.0)
            err = m.get("error_rate", 0.0)

            # Normalized penalty ratios
            cpu_score = min(1.0, max(0.0, (cpu - 50.0) / 45.0))
            mem_score = min(1.0, max(0.0, (mem - 50.0) / 45.0))
            lat_score = min(1.0, max(0.0, (lat - 50.0) / 350.0))
            err_score = min(1.0, max(0.0, err / 0.20))

            composite = 0.30 * cpu_score + 0.25 * mem_score + 0.25 * lat_score + 0.20 * err_score
            scores[svc_id] = float(composite)
        return scores

    def filter_anomalous_seeds(
        self,
        scores: Dict[str, float],
        threshold: float = 0.35,
        max_seeds: int = 5,
    ) -> List[str]:
        """Returns sorted list of anomalous service IDs exceeding threshold."""
        flagged = [(svc, s) for svc, s in scores.items() if s >= threshold]
        flagged.sort(key=lambda x: x[1], reverse=True)
        return [svc for svc, _ in flagged[:max_seeds]]


def extract_k_hop_subgraph(
    seeds: List[str],
    adj_out: Dict[str, List[str]],
    adj_in: Dict[str, List[str]],
    k_hops: int = 2,
    all_service_ids: Optional[List[str]] = None,
) -> Tuple[List[str], torch.Tensor]:
    """
    Extracts k-hop neighborhood around seed nodes (bidirectional: dependencies and dependents).
    Returns:
        subgraph_nodes: list of unique service IDs in the subgraph
        subgraph_edge_index: (2, E_sub) tensor of remapped edges
    """
    subgraph_set: Set[str] = set(seeds)
    frontier: Set[str] = set(seeds)

    for _ in range(k_hops):
        next_frontier: Set[str] = set()
        for node in frontier:
            for dep in adj_out.get(node, []):
                if dep not in subgraph_set:
                    subgraph_set.add(dep)
                    next_frontier.add(dep)
            for parent in adj_in.get(node, []):
                if parent not in subgraph_set:
                    subgraph_set.add(parent)
                    next_frontier.add(parent)
        frontier = next_frontier
        if not frontier:
            break

    subgraph_nodes = sorted(list(subgraph_set))
    node_to_idx = {nid: idx for idx, nid in enumerate(subgraph_nodes)}

    edges_src: List[int] = []
    edges_dst: List[int] = []
    for u in subgraph_nodes:
        for v in adj_out.get(u, []):
            if v in node_to_idx:
                edges_src.append(node_to_idx[u])
                edges_dst.append(node_to_idx[v])

    if edges_src:
        edge_index = torch.tensor([edges_src, edges_dst], dtype=torch.long)
    else:
        edge_index = torch.empty((2, 0), dtype=torch.long)

    return subgraph_nodes, edge_index


class TwoStageInferencePipeline:
    """
    Two-Stage Scalable Microservice Failure Prediction Pipeline.
    1. O(N) cheap filtering over all services.
    2. Sparse-TGNN localized inference over k-hop neighborhood only.
    """

    def __init__(
        self,
        node_in_features: int = 8,
        hidden_dim: int = 32,
        k_hops: int = 2,
        anomaly_threshold: float = 0.35,
    ):
        self.detector = LightweightAnomalyDetector()
        self.model = SparseTGNN(node_in_features=node_in_features, hidden_dim=hidden_dim)
        self.model.eval()
        self.k_hops = k_hops
        self.anomaly_threshold = anomaly_threshold
        self.version = "2.1.0-sparse"

    def predict(
        self,
        service_ids: List[str],
        node_features_seq: torch.Tensor,
        adj_out: Dict[str, List[str]],
        adj_in: Dict[str, List[str]],
        latest_metrics: Dict[str, Dict[str, float]],
    ) -> Dict[str, Any]:
        """
        Executes the two-stage inference pipeline.
        service_ids: full list of N microservices
        node_features_seq: (T, N, in_features)
        latest_metrics: {svc: metrics_dict}
        """
        N = len(service_ids)
        svc_to_idx = {sid: i for i, sid in enumerate(service_ids)}

        # Stage 1: O(N) anomaly filter
        anomaly_scores = self.detector.score_nodes(latest_metrics)
        seeds = self.detector.filter_anomalous_seeds(anomaly_scores, threshold=self.anomaly_threshold)

        # Early exit if nominal (Zero-overhead path for healthy clusters)
        if not seeds:
            max_score = max(anomaly_scores.values()) if anomaly_scores else 0.0
            return {
                "engine": "sparse_two_stage",
                "version": self.version,
                "stage": "stage_1_nominal",
                "failure_probability": float(max_score * 0.10),
                "confidence": 0.98,
                "is_anomalous": False,
                "root_cause": "",
                "root_cause_scores": {s: 0.0 for s in service_ids},
                "propagation_edges": [],
                "subgraph_size": 0,
                "total_services": N,
            }

        # Stage 2: Subgraph extraction + Sparse-TGNN inference
        subgraph_nodes, sub_edge_index = extract_k_hop_subgraph(
            seeds=seeds,
            adj_out=adj_out,
            adj_in=adj_in,
            k_hops=self.k_hops,
        )

        sub_indices = [svc_to_idx[sid] for sid in subgraph_nodes if sid in svc_to_idx]
        sub_x_seq = node_features_seq[:, sub_indices, :]  # (T, M, in_features)

        with torch.no_grad():
            outputs = self.model(sub_x_seq, sub_edge_index)

        fail_prob = float(outputs["failure_prob"].item())
        rc_probs = outputs["root_cause_probs"].cpu().numpy()
        prop_probs = outputs["propagation_probs"].cpu().numpy()

        # Map root causes back to original service names
        root_cause_scores = {s: 0.0 for s in service_ids}
        best_rc = seeds[0]
        max_rc_prob = 0.0

        for i, sid in enumerate(subgraph_nodes):
            model_prob = float(rc_probs[i])
            anom_score = anomaly_scores.get(sid, 0.0)
            combined = 0.40 * model_prob + 0.60 * anom_score
            root_cause_scores[sid] = combined
            if combined > max_rc_prob:
                max_rc_prob = combined
                best_rc = sid

        # Map propagation edges
        propagation_edges = []
        if sub_edge_index.size(1) > 0:
            for e_idx in range(sub_edge_index.size(1)):
                src_sub = int(sub_edge_index[0, e_idx].item())
                dst_sub = int(sub_edge_index[1, e_idx].item())
                if src_sub < len(subgraph_nodes) and dst_sub < len(subgraph_nodes):
                    p_val = float(prop_probs[e_idx])
                    if p_val >= 0.40:
                        propagation_edges.append({
                            "source": subgraph_nodes[src_sub],
                            "target": subgraph_nodes[dst_sub],
                            "probability": round(p_val, 3),
                        })

        # Calibrated confidence derived from anomaly margin
        confidence = min(0.99, max(0.70, float(0.80 + 0.19 * max_rc_prob)))

        return {
            "engine": "sparse_two_stage",
            "version": self.version,
            "stage": "stage_2_sparse_tgnn",
            "failure_probability": round(fail_prob, 3),
            "confidence": round(confidence, 3),
            "is_anomalous": True,
            "root_cause": best_rc,
            "root_cause_scores": root_cause_scores,
            "propagation_edges": propagation_edges,
            "subgraph_size": len(subgraph_nodes),
            "total_services": N,
        }
