"""
Combines Connectivity, Criticality, and Dependency groupings into unified Graph Representation.
Orchestrates the sequential graph preprocessing pipeline that runs strictly before TGNN inference.
"""

from typing import Dict, Any, List, Optional
import time
import json
import networkx as nx

from .algorithms.bfs_dfs import compute_reachability, compute_blast_radius
from .algorithms.centrality import compute_criticality_scores, compute_centrality_metrics
from .algorithms.scc import compute_scc_groups, has_cycles
from .algorithms.topo_sort import topological_sort_condensed, topological_sort_flattened
from .condensation import condense_graph


class GraphRepresentationBuilder:
    """
    Executes the authoritative sequential preprocessing pipeline:
    System Graph -> Connectivity -> Criticality -> SCC Condensation -> Topo Sort -> Graph Representation.
    """

    def __init__(self, structural_cache_ttl_sec: float = 5.0):
        self.structural_cache_ttl_sec = structural_cache_ttl_sec
        self._cached_structural_time = 0.0
        self._cached_structural_data: Optional[Dict[str, Any]] = None

    def build_representation(
        self,
        graph: nx.DiGraph,
        force_recompute_structural: bool = False,
    ) -> Dict[str, Any]:
        """
        Executes sequential preprocessing pipeline and formats output matching proto/graph.proto:
        1. Connectivity (BFS/DFS downstream reachability)
        2. Criticality (Degree & Betweenness Centrality)
        3. Dependency Grouping (SCC Cycle detection)
        4. Condensation (DAG condensation)
        5. Topological Sorting (SCC-ordered recovery sequence)
        6. Numerical Feature Tensor Matrix (Node features for TGNN)
        """
        now = time.time()
        needs_structural_recompute = (
            force_recompute_structural
            or self._cached_structural_data is None
            or (now - self._cached_structural_time) >= self.structural_cache_ttl_sec
        )

        if needs_structural_recompute:
            reachability = compute_reachability(graph)
            centrality_scores = compute_criticality_scores(graph)
            centrality_metrics = compute_centrality_metrics(graph)
            scc_groups = compute_scc_groups(graph)
            has_cyclic_deps = has_cycles(graph)

            # Condensation and Topological Sort
            condensed_dag, super_to_members, member_to_super = condense_graph(graph)
            topo_order_condensed = topological_sort_condensed(graph)
            topo_order_flattened = topological_sort_flattened(graph, centrality_scores)

            self._cached_structural_data = {
                "reachability": {k: list(v) for k, v in reachability.items()},
                "centrality_scores": centrality_scores,
                "centrality_metrics": centrality_metrics,
                "scc_groups": [list(c) for c in scc_groups],
                "has_cycles": has_cyclic_deps,
                "topological_order": topo_order_flattened,
                "condensed_order": topo_order_condensed,
                "super_to_members": super_to_members,
                "member_to_super": member_to_super,
                "condensed_num_nodes": condensed_dag.number_of_nodes(),
                "condensed_num_edges": condensed_dag.number_of_edges(),
            }
            self._cached_structural_time = now

        structural = self._cached_structural_data

        # Construct GraphSnapshot payload matching proto/graph.proto:GraphSnapshot
        snapshot = self._build_snapshot(graph)

        # Build numerical node feature matrix for TGNN input
        node_features = self._build_node_feature_matrix(
            graph,
            structural["centrality_scores"],
            structural["centrality_metrics"],
        )

        # Format SCC components list matching proto/graph.proto:StronglyConnectedComponent
        scc_components_proto = []
        for idx, members in enumerate(structural["scc_groups"]):
            scc_components_proto.append({
                "component_id": f"scc-{idx}",
                "node_ids": members,
            })

        return {
            "snapshot": snapshot,
            "topological_order": structural["topological_order"],
            "centrality_scores": structural["centrality_scores"],
            "scc_components": scc_components_proto,
            "feature_matrix": node_features,
            "has_cycles": structural["has_cycles"],
            "reachability": structural["reachability"],
            "condensed_order": structural["condensed_order"],
            "computed_at_unix_nano": int(now * 1e9),
        }

    def _build_snapshot(self, graph: nx.DiGraph) -> Dict[str, Any]:
        """Constructs protobuf-compatible GraphSnapshot dictionary."""
        nodes = []
        for node_id, data in graph.nodes(data=True):
            nodes.append({
                "id": node_id,
                "name": data.get("name", node_id),
                "type": data.get("type", "service"),
                "status": data.get("status", "healthy"),
                "metadata": {str(k): str(v) for k, v in data.get("metadata", {}).items()},
                "metrics": {str(k): float(v) for k, v in data.get("metrics", {}).items()},
            })

        edges = []
        for src, dst, data in graph.edges(data=True):
            edges.append({
                "source": src,
                "target": dst,
                "relationship": data.get("relationship", "calls"),
                "latency_ms": float(data.get("latency_ms", 0.0)),
                "error_rate": float(data.get("error_rate", 0.0)),
                "throughput": float(data.get("throughput", 0.0)),
            })

        return {
            "timestamp_unix_nano": int(time.time() * 1e9),
            "nodes": nodes,
            "edges": edges,
        }

    def _build_node_feature_matrix(
        self,
        graph: nx.DiGraph,
        centrality_scores: Dict[str, float],
        centrality_metrics: Dict[str, Dict[str, float]],
    ) -> Dict[str, List[float]]:
        """
        Builds normalized numerical feature vector for each node:
        [cpu_norm, mem_norm, latency_norm, error_rate, in_degree_norm, out_degree_norm, centrality, status_code]
        Used directly by TGNN input layers in Phase 3.
        """
        features: Dict[str, List[float]] = {}
        status_encoding = {"healthy": 0.0, "degraded": 0.5, "critical": 1.0}

        for node_id in graph.nodes():
            data = graph.nodes[node_id]
            metrics = data.get("metrics", {})

            cpu = metrics.get("cpu_usage", 20.0) / 100.0
            mem = metrics.get("memory_usage", 40.0) / 100.0
            lat = min(1.0, metrics.get("p95_latency_ms", 25.0) / 1000.0)
            err = min(1.0, metrics.get("error_rate", 0.001))

            c_metrics = centrality_metrics.get(node_id, {})
            in_deg = c_metrics.get("in_degree", 0.0)
            out_deg = c_metrics.get("out_degree", 0.0)
            crit = centrality_scores.get(node_id, 0.0)

            status = status_encoding.get(data.get("status", "healthy"), 0.0)

            feature_vec = [
                round(cpu, 4),
                round(mem, 4),
                round(lat, 4),
                round(err, 4),
                round(in_deg, 4),
                round(out_deg, 4),
                round(crit, 4),
                round(status, 2),
            ]
            features[node_id] = feature_vec

        return features
