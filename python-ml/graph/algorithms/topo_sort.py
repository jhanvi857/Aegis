"""
Topological sort on SCC-condensed graph
Used in recovery planning (Phase 4), guarantees DAG property even in cyclic topologies.
Implements first-principles Kahn's algorithm (1962) with in-degree tracking and queue processing.
"""

from typing import List, Dict, Optional, Any
from collections import deque
import networkx as nx
from ..condensation import condense_graph


def kahn_topological_sort(dag: nx.DiGraph) -> List[Any]:
    """
    Computes topological ordering on a Directed Acyclic Graph (DAG) using Kahn's algorithm (1962).
    
    Complexity: O(V + E) time and O(V) space.
    
    1. Computes in-degree for all vertices.
    2. Initializes a FIFO queue with all vertices having in-degree 0.
    3. Repeatedly dequeues a vertex, appends to the sorted order, and decrements in-degrees of neighbors.
    4. Enqueues neighbors whose in-degree drops to 0.
    5. Raises ValueError if a cycle is detected (visited count < total vertices).
    """
    if dag.number_of_nodes() == 0:
        return []

    in_degree: Dict[Any, int] = {node: dag.in_degree(node) for node in dag.nodes()}
    # Deterministic queue initialization (sorted for reproducibility)
    zero_in = sorted([node for node, deg in in_degree.items() if deg == 0], key=lambda x: str(x))
    queue = deque(zero_in)
    order: List[Any] = []

    while queue:
        u = queue.popleft()
        order.append(u)

        for v in dag.successors(u):
            in_degree[v] -= 1
            if in_degree[v] == 0:
                queue.append(v)

    if len(order) != dag.number_of_nodes():
        raise ValueError(
            f"Graph contains a directed cycle: Kahn's algorithm visited {len(order)} of {dag.number_of_nodes()} nodes."
        )

    return order


def topological_sort_condensed(graph: nx.DiGraph) -> List[List[str]]:
    """
    Condenses the graph to a DAG and returns a list of SCC component node lists in topological order.
    Guarantees no cycle errors even if raw dependency graph contains feedback loops.
    """
    if graph.number_of_nodes() == 0:
        return []

    condensed_dag, super_to_members, _ = condense_graph(graph)
    order = kahn_topological_sort(condensed_dag)
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

