"""
Combines connectivity, criticality, and dependency groupings into graph representation.
This module performs conventional graph analysis.
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
        6. Protobuf-compatible snapshot for downstream analysis and APIs
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
            # Cyclic graphs have no node-level topological order; their SCC
            # groups are returned separately for explicit handling.
            topo_order_flattened = [] if has_cyclic_deps else topological_sort_flattened(graph, centrality_scores)

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
                "status": data.get("status", "unknown"),
                "metadata": {str(k): str(v) for k, v in data.get("metadata", {}).items()},
                "metrics": {str(k): float(v) for k, v in data.get("metrics", {}).items()},
            })

        edges = []
        for src, dst, data in graph.edges(data=True):
            edge = {
                "source": src,
                "target": dst,
                "relationship": data.get("relationship", "calls"),
            }
            for metric in ("latency_ms", "error_rate", "throughput"):
                if metric in data:
                    edge[metric] = float(data[metric])
            edges.append(edge)

        return {
            "timestamp_unix_nano": int(time.time() * 1e9),
            "nodes": nodes,
            "edges": edges,
        }
