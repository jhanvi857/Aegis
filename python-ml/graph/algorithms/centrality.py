"""
Criticality scoring via centrality algorithms
Measures structural importance, bottleneck risk, and traffic criticality of nodes.
"""

from typing import Dict, Any, Optional
import networkx as nx


def compute_centrality_metrics(graph: nx.DiGraph) -> Dict[str, Dict[str, float]]:
    """
    Computes a comprehensive dictionary of centrality metrics per node:
    - betweenness: how often a node sits on shortest paths between other nodes
    - in_degree: number of services calling this node (dependency load)
    - out_degree: number of downstream services this node depends on
    - degree: total connections
    """
    if graph.number_of_nodes() == 0:
        return {}

    num_nodes = graph.number_of_nodes()
    norm = max(1, num_nodes - 1)

    betweenness = nx.betweenness_centrality(graph)
    in_degrees = {n: d / norm for n, d in graph.in_degree()}
    out_degrees = {n: d / norm for n, d in graph.out_degree()}

    metrics: Dict[str, Dict[str, float]] = {}
    for node in graph.nodes():
        metrics[node] = {
            "betweenness": betweenness.get(node, 0.0),
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
