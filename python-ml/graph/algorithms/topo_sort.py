"""
Topological sort on SCC-condensed graph
Used in recovery planning (Phase 4), guarantees DAG property even in cyclic topologies.
"""

from typing import List, Dict, Optional
import networkx as nx
from ..condensation import condense_graph


def topological_sort_condensed(graph: nx.DiGraph) -> List[List[str]]:
    """
    Condenses the graph to a DAG and returns a list of SCC component node lists in topological order.
    Guarantees no cycle errors even if raw dependency graph contains feedback loops.
    """
    if graph.number_of_nodes() == 0:
        return []

    condensed_dag, super_to_members, _ = condense_graph(graph)
    order = list(nx.topological_sort(condensed_dag))
    return [super_to_members[super_node] for super_node in order]


def topological_sort_flattened(
    graph: nx.DiGraph,
    criticality_scores: Optional[Dict[str, float]] = None,
) -> List[str]:
    """
    Returns a flattened 1D list of individual node IDs in topological order.
    For SCC components containing multiple nodes (cycles), nodes within the component
    are tie-broken using their criticality scores (highest criticality first).
    """
    condensed_order = topological_sort_condensed(graph)
    flattened: List[str] = []

    for component in condensed_order:
        if len(component) == 1:
            flattened.append(component[0])
        else:
            if criticality_scores:
                sorted_members = sorted(
                    component,
                    key=lambda n: criticality_scores.get(n, 0.0),
                    reverse=True,
                )
            else:
                sorted_members = sorted(component)
            flattened.extend(sorted_members)

    return flattened
