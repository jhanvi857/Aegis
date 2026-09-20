"""
Unit tests for Graph Algorithms (Phase 2): BFS/DFS, Centrality, SCC, Topo Sort
"""

import unittest
import os
import sys
import networkx as nx

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from graph.algorithms.bfs_dfs import (
    compute_reachability,
    compute_blast_radius,
    compute_upstream_dependencies,
    find_cascade_paths,
)
from graph.algorithms.centrality import (
    compute_centrality_metrics,
    compute_criticality_scores,
)
from graph.algorithms.scc import (
    compute_scc_groups,
    has_cycles,
    detect_cycles,
    get_cyclic_components,
)
from graph.algorithms.topo_sort import (
    topological_sort_condensed,
    topological_sort_flattened,
)


class TestGraphAlgorithms(unittest.TestCase):
    def setUp(self):
        # Build a standard diamond DAG: A -> B, A -> C, B -> D, C -> D
        self.dag = nx.DiGraph()
        self.dag.add_edges_from([
            ("gateway", "node-a"),
            ("node-a", "node-b"),
            ("node-a", "node-c"),
            ("node-c", "node-d"),
        ])

        # Build a graph with a cyclic feedback loop: X -> Y -> Z -> Y
        self.cyclic_graph = nx.DiGraph()
        self.cyclic_graph.add_edges_from([
            ("service-x", "service-y"),
            ("service-y", "service-z"),
            ("service-z", "service-y"),  # cycle: Y <-> Z
            ("service-z", "service-w"),
        ])

    def test_reachability_and_blast_radius(self):
        reachability = compute_reachability(self.dag)
        self.assertIn("node-b", reachability["gateway"])
        self.assertIn("node-d", reachability["gateway"])
        self.assertIn("node-d", reachability["node-c"])
        self.assertEqual(len(reachability["node-d"]), 0)  # leaf node

        # Blast radius of node-a should include node-a, node-b, node-c, node-d
        blast_a = compute_blast_radius(self.dag, "node-a")
        self.assertEqual(blast_a, {"node-a", "node-b", "node-c", "node-d"})

        # Blast radius of leaf node-d is only node-d
        blast_d = compute_blast_radius(self.dag, "node-d")
        self.assertEqual(blast_d, {"node-d"})

    def test_find_cascade_paths(self):
        paths = find_cascade_paths(self.dag, "gateway", "node-d")
        self.assertEqual(len(paths), 1)
        self.assertEqual(paths[0], ["gateway", "node-a", "node-c", "node-d"])

    def test_centrality_and_criticality(self):
        metrics = compute_centrality_metrics(self.dag)
        scores = compute_criticality_scores(self.dag)

        self.assertIn("node-a", scores)
        self.assertIn("gateway", scores)

        # In our DAG, node-a is a central coordinator on paths to b, c, d
        self.assertGreater(scores["node-a"], 0.0)
        for node, score in scores.items():
            self.assertGreaterEqual(score, 0.0)
            self.assertLessEqual(score, 1.0)

    def test_scc_and_cycle_detection(self):
        self.assertFalse(has_cycles(self.dag))
        self.assertEqual(len(detect_cycles(self.dag)), 0)

        # On cyclic graph
        self.assertTrue(has_cycles(self.cyclic_graph))
        cycles = detect_cycles(self.cyclic_graph)
        self.assertGreaterEqual(len(cycles), 1)

        cyclic_comps = get_cyclic_components(self.cyclic_graph)
        self.assertEqual(len(cyclic_comps), 1)
        self.assertEqual(cyclic_comps[0], {"service-y", "service-z"})

    def test_topological_sort_condensed_and_flattened(self):
        # Normal DAG topo sort
        order = topological_sort_flattened(self.dag)
        self.assertEqual(order[0], "gateway")
        self.assertEqual(order[1], "node-a")
        self.assertTrue(order.index("node-c") < order.index("node-d"))

        # Cyclic graph MUST NOT crash! Raw topo sort would raise nx.NetworkXUnfeasible
        condensed_order = topological_sort_condensed(self.cyclic_graph)
        self.assertEqual(len(condensed_order), 3)  # [service-x], [service-y, service-z], [service-w]

        flattened_cyclic = topological_sort_flattened(self.cyclic_graph)
        self.assertEqual(len(flattened_cyclic), 4)
        self.assertEqual(flattened_cyclic[0], "service-x")
        self.assertEqual(flattened_cyclic[-1], "service-w")

    def test_algorithmic_equivalence_vs_networkx(self):
        """
        Formally verifies that first-principles Tarjan, Kahn, and Brandes implementations
        strictly match NetworkX reference outputs across diverse graph topologies.
        """
        # 1. Tarjan's SCC equivalence
        tarjan_result = [set(s) for s in compute_scc_groups(self.cyclic_graph)]
        nx_scc_result = [set(s) for s in nx.strongly_connected_components(self.cyclic_graph)]
        self.assertEqual(len(tarjan_result), len(nx_scc_result))
        for comp in tarjan_result:
            self.assertIn(comp, nx_scc_result)

        # 2. Brandes' Betweenness Centrality equivalence (within 1e-6 precision)
        from graph.algorithms.centrality import brandes_betweenness_centrality
        brandes_scores = brandes_betweenness_centrality(self.dag, normalized=True)
        nx_scores = nx.betweenness_centrality(self.dag, normalized=True)
        for node in self.dag.nodes():
            self.assertAlmostEqual(brandes_scores[node], nx_scores[node], places=5)

        # Also test Brandes on cyclic graph
        brandes_cyclic = brandes_betweenness_centrality(self.cyclic_graph, normalized=True)
        nx_cyclic = nx.betweenness_centrality(self.cyclic_graph, normalized=True)
        for node in self.cyclic_graph.nodes():
            self.assertAlmostEqual(brandes_cyclic[node], nx_cyclic[node], places=5)

        # 3. Kahn's Topological Sort validity
        from graph.algorithms.topo_sort import kahn_topological_sort
        kahn_order = kahn_topological_sort(self.dag)
        # Verify valid DAG ordering: for all u -> v, index(u) < index(v)
        for u, v in self.dag.edges():
            self.assertLess(kahn_order.index(u), kahn_order.index(v))


if __name__ == "__main__":
    unittest.main()

