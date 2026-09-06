"""
Computes safe recovery sequence using topological sort over SCC-condensed subgraph
"""

from typing import List, Set
import networkx as nx
from ..graph.algorithms.topo_sort import topological_sort_condensed


def compute_recovery_order(graph: nx.DiGraph, affected_nodes: Set[str]) -> List[str]:
    """
    Extracts the subgraph of affected nodes and performs topological sort on its SCC condensation.
    Returns flattened list of node IDs in strictly dependency-safe execution order.
    """
    subgraph = graph.subgraph(affected_nodes).copy()
    if subgraph.number_of_nodes() == 0:
        return []

    condensed_order = topological_sort_condensed(subgraph)
    flattened = [node for group in condensed_order for node in group]
    return flattened
