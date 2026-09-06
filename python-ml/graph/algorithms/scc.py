"""
Dependency grouping via Strongly Connected Components (SCC)
Identifies cyclical dependencies (retries, circular RPCs) and isolates dependency clusters.
"""

from typing import List, Set, Dict, Any
import networkx as nx


def compute_scc_groups(graph: nx.DiGraph) -> List[Set[str]]:
    """
    Finds all strongly connected components in the dependency graph using Tarjan's algorithm.
    Returns list of sets, where each set represents an SCC.
    """
    return [c for c in nx.strongly_connected_components(graph)]


def has_cycles(graph: nx.DiGraph) -> bool:
    """
    Checks if the raw dependency graph contains any directed cycles.
    """
    return not nx.is_directed_acyclic_graph(graph)


def detect_cycles(graph: nx.DiGraph) -> List[List[str]]:
    """
    Returns all simple elementary cycles detected in the graph.
    Useful for alerting operators to circular RPC dependencies.
    """
    try:
        return list(nx.simple_cycles(graph))
    except Exception:
        return []


def get_cyclic_components(graph: nx.DiGraph) -> List[Set[str]]:
    """
    Returns only the SCC groups that contain multiple nodes (size > 1)
    or single nodes with self-loops.
    """
    cyclic_groups: List[Set[str]] = []
    for comp in nx.strongly_connected_components(graph):
        if len(comp) > 1:
            cyclic_groups.append(comp)
        elif len(comp) == 1:
            node = next(iter(comp))
            if graph.has_edge(node, node):
                cyclic_groups.append(comp)
    return cyclic_groups
