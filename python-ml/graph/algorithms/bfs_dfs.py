"""
Connectivity analysis via BFS and DFS
Computes reachability matrices, blast radius boundaries, and failure cascade paths.
"""

from typing import Dict, Set, List, Optional
import networkx as nx


def compute_reachability(graph: nx.DiGraph) -> Dict[str, Set[str]]:
    """
    Computes reachable downstream nodes for each node in the graph.
    Returns mapping: {node_id: set(downstream_node_ids)}.
    """
    reachability: Dict[str, Set[str]] = {}
    for node in graph.nodes():
        reachability[node] = set(nx.descendants(graph, node))
    return reachability


def compute_blast_radius(graph: nx.DiGraph, root_node: str) -> Set[str]:
    """
    Computes the set of all nodes that are directly or transitively
    impacted if root_node degrades or fails.
    """
    if root_node not in graph:
        return set()
    return set(nx.descendants(graph, root_node)) | {root_node}


def compute_upstream_dependencies(graph: nx.DiGraph) -> Dict[str, Set[str]]:
    """
    Computes upstream callers (ancestors) that depend on each node.
    Returns mapping: {node_id: set(upstream_caller_ids)}.
    """
    ancestors: Dict[str, Set[str]] = {}
    for node in graph.nodes():
        ancestors[node] = set(nx.ancestors(graph, node))
    return ancestors


def find_cascade_paths(
    graph: nx.DiGraph, source: str, target: str, cutoff: Optional[int] = 10
) -> List[List[str]]:
    """
    Finds all directed paths from source to target along which a failure or latency spike
    can propagate through the dependency topology.
    """
    if source not in graph or target not in graph:
        return []
    try:
        return list(nx.all_simple_paths(graph, source=source, target=target, cutoff=cutoff))
    except Exception:
        return []
