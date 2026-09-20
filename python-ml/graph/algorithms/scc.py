"""
Dependency grouping via Strongly Connected Components (SCC)
Identifies cyclical dependencies (retries, circular RPCs) and isolates dependency clusters.
Implements first-principles Tarjan's algorithm (1972) using DFS indices, lowlink tracking, and an explicit stack.
"""

from typing import List, Set, Dict, Any, Optional
import networkx as nx


def tarjan_scc(graph: nx.DiGraph) -> List[Set[str]]:
    """
    Computes all Strongly Connected Components (SCCs) using Tarjan's algorithm (1972).
    
    Complexity: O(V + E) time and O(V) auxiliary space.
    
    Each node v is assigned:
    - dfs_index[v]: monotonically increasing discovery timestamp
    - lowlink[v]: lowest dfs_index reachable from v via tree edges and back edges
    """
    index = 0
    dfs_index: Dict[str, int] = {}
    lowlink: Dict[str, int] = {}
    stack: List[str] = []
    on_stack: Set[str] = set()
    sccs: List[Set[str]] = []

    def strongconnect(v: str) -> None:
        nonlocal index
        dfs_index[v] = index
        lowlink[v] = index
        index += 1
        stack.append(v)
        on_stack.add(v)

        # Consider successors of v in the directed graph
        for w in graph.successors(v):
            if w not in dfs_index:
                # Tree edge: w has not yet been visited
                strongconnect(w)
                lowlink[v] = min(lowlink[v], lowlink[w])
            elif w in on_stack:
                # Back edge: w is currently on the stack, hence part of current SCC subtree
                lowlink[v] = min(lowlink[v], dfs_index[w])

        # If v is a root node of an SCC, pop all members off the stack
        if lowlink[v] == dfs_index[v]:
            current_scc: Set[str] = set()
            while True:
                w = stack.pop()
                on_stack.remove(w)
                current_scc.add(w)
                if w == v:
                    break
            sccs.append(current_scc)

    for node in graph.nodes():
        if node not in dfs_index:
            strongconnect(node)

    return sccs


def compute_scc_groups(graph: nx.DiGraph) -> List[Set[str]]:
    """
    Finds all strongly connected components in the dependency graph using Tarjan's algorithm.
    Returns list of sets, where each set represents an SCC.
    """
    return tarjan_scc(graph)


def has_cycles(graph: nx.DiGraph) -> bool:
    """
    Checks if the raw dependency graph contains any directed cycles
    using the computed SCCs (any SCC > 1 or self-loop indicates a cycle).
    """
    return len(get_cyclic_components(graph)) > 0


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
    for comp in tarjan_scc(graph):
        if len(comp) > 1:
            cyclic_groups.append(comp)
        elif len(comp) == 1:
            node = next(iter(comp))
            if graph.has_edge(node, node):
                cyclic_groups.append(comp)
    return cyclic_groups

