"""
Computes safe recovery sequence using topological sort over SCC-condensed subgraph (Phase 4).
Ensures cycle-safe ordering and resolves dependency order so underlying services heal before callers.
"""

from typing import List, Set, Optional, Dict
import networkx as nx

try:
    from ..graph.algorithms.topo_sort import topological_sort_condensed
except (ImportError, ValueError):
    from graph.algorithms.topo_sort import topological_sort_condensed


def compute_recovery_groups(
    graph: nx.DiGraph,
    affected_nodes: Set[str],
    heal_dependencies_first: bool = True,
) -> List[List[str]]:
    """Return SCC groups in recovery order; members of a cyclic group are unordered.

    Edges use caller -> dependency, so the condensed graph is reversed for
    dependency-first recovery. Keeping SCC members grouped prevents callers
    from mistaking an arbitrary within-cycle tie-break for a valid topo order.
    """
    valid_nodes = sorted(set(affected_nodes).intersection(graph.nodes()))
    if not valid_nodes:
        return []
    subgraph = nx.DiGraph()
    subgraph.add_nodes_from((node, dict(graph.nodes[node])) for node in valid_nodes)
    selected = set(valid_nodes)
    subgraph.add_edges_from(sorted((source, target, dict(attrs)) for source, target, attrs in graph.edges(data=True) if source in selected and target in selected))
    sort_graph = subgraph.reverse(copy=True) if heal_dependencies_first else subgraph
    groups = topological_sort_condensed(sort_graph)
    return [sorted(group) for group in groups]


def compute_recovery_order(
    graph: nx.DiGraph,
    affected_nodes: Set[str],
    heal_dependencies_first: bool = True,
    criticality_scores: Optional[Dict[str, float]] = None,
) -> List[str]:
    """Flatten SCC recovery groups for display; cycles have no valid inner order.

    Dependency-first order reverses caller -> dependency edges. Do not use this
    display helper to authorize sequential execution when a cyclic group exists.
    """
    groups = compute_recovery_groups(graph, affected_nodes, heal_dependencies_first)
    ordered_nodes: List[str] = []
    for group in groups:
        if len(group) > 1 or graph.has_edge(group[0], group[0]):
            ordered_nodes.extend(sorted(group, key=lambda node: (-(criticality_scores or {}).get(node, 0.0), node)))
        else:
            ordered_nodes.extend(group)

    seen = set(ordered_nodes)
    for n in sorted(affected_nodes):
        if n not in seen:
            ordered_nodes.append(n)

    return ordered_nodes
