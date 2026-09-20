"""
Criticality scoring via centrality algorithms
Measures structural importance, bottleneck risk, and traffic criticality of nodes.
Implements first-principles Brandes' algorithm (2001) for directed betweenness centrality.
"""

from typing import Dict, Any, Optional, List
from collections import deque
import networkx as nx


def brandes_betweenness_centrality(graph: nx.DiGraph, normalized: bool = True) -> Dict[str, float]:
    """
    Computes betweenness centrality for all nodes in a directed graph using Brandes' algorithm (2001).
    
    Complexity: O(V * E) time and O(V + E) space for unweighted directed graphs.
    
    For each source vertex s:
    1. Single-source shortest paths via BFS calculates sigma[v] (number of shortest paths)
       and d[v] (distance from s).
    2. Traversal stack S stores vertices in order of non-decreasing distance.
    3. Backward dependency accumulation calculates pair dependencies:
       delta[v] = sum_{w: v in P(w)} (sigma[v] / sigma[w]) * (1 + delta[w])
    4. Normalizes by 1 / ((N - 1) * (N - 2)) for directed graphs with N > 2.
    """
    nodes = list(graph.nodes())
    cb: Dict[str, float] = {v: 0.0 for v in nodes}
    num_nodes = len(nodes)

    if num_nodes <= 2:
        return cb

    for s in nodes:
        stack: List[str] = []
        pred: Dict[str, List[str]] = {w: [] for w in nodes}
        sigma: Dict[str, float] = {w: 0.0 for w in nodes}
        sigma[s] = 1.0
        dist: Dict[str, int] = {w: -1 for w in nodes}
        dist[s] = 0
        queue: deque = deque([s])

        # BFS shortest path search
        while queue:
            v = queue.popleft()
            stack.append(v)
            for w in graph.successors(v):
                if dist[w] < 0:
                    dist[w] = dist[v] + 1
                    queue.append(w)
                if dist[w] == dist[v] + 1:
                    sigma[w] += sigma[v]
                    pred[w].append(v)

        # Backward dependency accumulation
        delta: Dict[str, float] = {w: 0.0 for w in nodes}
        while stack:
            w = stack.pop()
            for v in pred[w]:
                if sigma[w] > 0:
                    delta[v] += (sigma[v] / sigma[w]) * (1.0 + delta[w])
            if w != s:
                cb[w] += delta[w]

    # Normalization for directed graphs: (N - 1) * (N - 2)
    if normalized and num_nodes > 2:
        scale = 1.0 / ((num_nodes - 1) * (num_nodes - 2))
        for v in cb:
            cb[v] *= scale

    return cb


def compute_centrality_metrics(graph: nx.DiGraph) -> Dict[str, Dict[str, float]]:
    """
    Computes a comprehensive dictionary of centrality metrics per node:
    - betweenness: how often a node sits on shortest paths between other nodes (Brandes 2001)
    - in_degree: number of services calling this node (dependency load)
    - out_degree: number of downstream services this node depends on
    - degree: total connections
    """
    if graph.number_of_nodes() == 0:
        return {}

    num_nodes = graph.number_of_nodes()
    norm = max(1, num_nodes - 1)

    betweenness = brandes_betweenness_centrality(graph, normalized=True)
    in_degrees = {n: d / norm for n, d in graph.in_degree()}
    out_degrees = {n: d / norm for n, d in graph.out_degree()}

    metrics: Dict[str, Dict[str, float]] = {}
    for node in graph.nodes():
        metrics[node] = {
            "betweenness": round(betweenness.get(node, 0.0), 6),
            "in_degree": in_degrees.get(node, 0.0),
            "out_degree": out_degrees.get(node, 0.0),
            "total_degree": (graph.in_degree(node) + graph.out_degree(node)) / max(1, 2 * norm),
        }
    return metrics


def compute_criticality_scores(
    graph: nx.DiGraph,
    alpha: float = 0.5,
    beta: float = 0.3,
    gamma: float = 0.2,
) -> Dict[str, float]:
    """
    Computes unified criticality score (0.0 to 1.0) for each node in the graph.
    Combines:
    - betweenness centrality (alpha): structural bridge importance
    - in-degree centrality (beta): number of upstream callers vulnerable to this node
    - out-degree centrality (gamma): vulnerability to downstream failures
    """
    if graph.number_of_nodes() == 0:
        return {}

    metrics = compute_centrality_metrics(graph)
    scores: Dict[str, float] = {}

    for node, m in metrics.items():
        score = (
            alpha * m["betweenness"]
            + beta * m["in_degree"]
            + gamma * m["out_degree"]
        )
        # Normalize to [0.0, 1.0]
        scores[node] = round(min(1.0, max(0.0, score)), 4)

    return scores

