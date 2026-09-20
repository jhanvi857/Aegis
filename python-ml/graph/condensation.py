"""
Strongly Connected Component (SCC) graph condensation
Collapses cyclic dependency components into single super-nodes to produce a guaranteed DAG.
Uses first-principles Tarjan's SCC algorithm from scc.py.
"""

from typing import Tuple, Dict, List, Set, Any
import networkx as nx
from .algorithms.scc import tarjan_scc


def condense_graph(
    graph: nx.DiGraph,
) -> Tuple[nx.DiGraph, Dict[int, List[str]], Dict[str, int]]:
    """
    Condenses graph into a Directed Acyclic Graph (DAG) using Tarjan's SCC algorithm.
    
    Each strongly connected component is collapsed into a single integer super-node.
    Inter-component directed edges are preserved, while intra-component cycles are contracted.
    
    Returns:
    1. condensed_dag: nx.DiGraph guaranteed to be a DAG (nx.is_directed_acyclic_graph == True).
    2. super_to_members: Dict[super_node_id, List[original_node_ids]]
    3. member_to_super: Dict[original_node_id, super_node_id]
    """
    scc_list = tarjan_scc(graph)

    super_to_members: Dict[int, List[str]] = {}
    member_to_super: Dict[str, int] = {}

    for i, component in enumerate(scc_list):
        nodes = sorted(list(component))
        super_to_members[i] = nodes
        for node in nodes:
            member_to_super[node] = i

    # Construct condensed DAG directly from Tarjan's components
    condensed_dag = nx.DiGraph()
    for super_id in super_to_members:
        condensed_dag.add_node(super_id)

    for u, v in graph.edges():
        if u in member_to_super and v in member_to_super:
            super_u = member_to_super[u]
            super_v = member_to_super[v]
            if super_u != super_v:
                condensed_dag.add_edge(super_u, super_v)

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

