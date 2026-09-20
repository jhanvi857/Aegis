"""
Computes safe recovery sequence using topological sort over SCC-condensed subgraph (Phase 4).
Ensures cycle-safe ordering and resolves dependency order so underlying services heal before callers.
"""

from typing import List, Set, Optional, Dict
import networkx as nx

try:
    from ..graph.algorithms.topo_sort import topological_sort_flattened
except (ImportError, ValueError):
    from graph.algorithms.topo_sort import topological_sort_flattened


def compute_recovery_order(
    graph: nx.DiGraph,
    affected_nodes: Set[str],
    heal_dependencies_first: bool = True,
    criticality_scores: Optional[Dict[str, float]] = None,
) -> List[str]:
    """
    Extracts the subgraph of affected nodes and performs topological sort on its SCC condensation.
    When heal_dependencies_first is True, orders bottom-up dependencies first (e.g. database/sink
    before coordinator before gateway) so upstream callers recover against healthy dependencies.
    """
    if not affected_nodes:
        return []

    valid_nodes = set(affected_nodes).intersection(set(graph.nodes()))
    if not valid_nodes:
        return list(affected_nodes)

    subgraph = graph.subgraph(valid_nodes).copy()
    if subgraph.number_of_nodes() == 0:
        return []

    sort_graph = subgraph.reverse() if heal_dependencies_first else subgraph
    ordered_nodes = topological_sort_flattened(sort_graph, criticality_scores=criticality_scores)

    seen = set(ordered_nodes)
    for n in affected_nodes:
        if n not in seen:
            ordered_nodes.append(n)

    return ordered_nodes
