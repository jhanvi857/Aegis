"""
Strongly Connected Component (SCC) graph condensation
Collapses cyclic dependency components into single super-nodes to produce a guaranteed DAG.
"""

from typing import Tuple, Dict, List, Set, Any
import networkx as nx


def condense_graph(
    graph: nx.DiGraph,
) -> Tuple[nx.DiGraph, Dict[int, List[str]], Dict[str, int]]:
    """
    Condenses graph into a Directed Acyclic Graph (DAG).
    
    Each strongly connected component is collapsed into a single integer super-node.
    
    Returns:
    1. condensed_dag: nx.DiGraph guaranteed to be a DAG (nx.is_directed_acyclic_graph == True).
    2. super_to_members: Dict[super_node_id, List[original_node_ids]]
    3. member_to_super: Dict[original_node_id, super_node_id]
    """
    scc_list = list(nx.strongly_connected_components(graph))

    super_to_members: Dict[int, List[str]] = {}
    member_to_super: Dict[str, int] = {}

    for i, component in enumerate(scc_list):
        nodes = sorted(list(component))
        super_to_members[i] = nodes
        for node in nodes:
            member_to_super[node] = i

    condensed_dag = nx.condensation(graph, scc_list)

    # Annotate super-nodes with aggregated metadata
    for super_id in condensed_dag.nodes():
        members = super_to_members.get(super_id, [])
        is_cyclic = len(members) > 1 or (
            len(members) == 1 and graph.has_edge(members[0], members[0])
        )

        # Aggregate health status across member nodes
        member_statuses = [
            graph.nodes[n].get("status", "healthy") for n in members if n in graph
        ]
        if "critical" in member_statuses:
            status = "critical"
        elif "degraded" in member_statuses:
            status = "degraded"
        else:
            status = "healthy"

        condensed_dag.nodes[super_id]["members"] = members
        condensed_dag.nodes[super_id]["member_count"] = len(members)
        condensed_dag.nodes[super_id]["is_cyclic"] = is_cyclic
        condensed_dag.nodes[super_id]["status"] = status

    return condensed_dag, super_to_members, member_to_super
