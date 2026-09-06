"""
Unit tests for Condensation and GraphRepresentationBuilder (Phase 2)
"""

import unittest
import os
import sys
import networkx as nx

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from graph.condensation import condense_graph
from graph.representation import GraphRepresentationBuilder


class TestCondensationAndRepresentation(unittest.TestCase):
    def setUp(self):
        # Graph with cyclic component: A -> B -> C -> B, C -> D
        self.graph = nx.DiGraph()
        self.graph.add_node("node-a", status="healthy", metrics={"cpu_usage": 25.0, "p95_latency_ms": 20.0})
        self.graph.add_node("node-b", status="degraded", metrics={"cpu_usage": 88.0, "p95_latency_ms": 550.0})
        self.graph.add_node("node-c", status="healthy", metrics={"cpu_usage": 30.0, "p95_latency_ms": 35.0})
        self.graph.add_node("node-d", status="healthy", metrics={"cpu_usage": 15.0, "p95_latency_ms": 15.0})

        self.graph.add_edges_from([
            ("node-a", "node-b"),
            ("node-b", "node-c"),
            ("node-c", "node-b"),  # cycle between b and c
            ("node-c", "node-d"),
        ])

        self.rep_builder = GraphRepresentationBuilder(structural_cache_ttl_sec=1.0)

    def test_condense_graph(self):
        condensed_dag, super_to_members, member_to_super = condense_graph(self.graph)

        # Must be a strict DAG
        self.assertTrue(nx.is_directed_acyclic_graph(condensed_dag))
        self.assertEqual(condensed_dag.number_of_nodes(), 3)  # {node-a}, {node-b, node-c}, {node-d}

        # Check super node representing cycle {node-b, node-c}
        super_bc = member_to_super["node-b"]
        self.assertEqual(member_to_super["node-c"], super_bc)
        self.assertEqual(condensed_dag.nodes[super_bc]["member_count"], 2)
        self.assertTrue(condensed_dag.nodes[super_bc]["is_cyclic"])
        # Because node-b is degraded, the super-node status must reflect degraded
        self.assertEqual(condensed_dag.nodes[super_bc]["status"], "degraded")

    def test_build_representation_structure(self):
        rep = self.rep_builder.build_representation(self.graph, force_recompute_structural=True)

        # Check top-level keys matching proto contract
        self.assertIn("snapshot", rep)
        self.assertIn("topological_order", rep)
        self.assertIn("centrality_scores", rep)
        self.assertIn("scc_components", rep)
        self.assertIn("feature_matrix", rep)
        self.assertTrue(rep["has_cycles"])

        # Check topological order: node-a must come first, node-d must come last
        order = rep["topological_order"]
        self.assertEqual(order[0], "node-a")
        self.assertEqual(order[-1], "node-d")

        # Check node feature matrix
        feat_matrix = rep["feature_matrix"]
        self.assertIn("node-b", feat_matrix)
        vec_b = feat_matrix["node-b"]
        self.assertEqual(len(vec_b), 8)  # 8 features per node
        # feature vector index 7 is status (degraded = 0.5)
        self.assertEqual(vec_b[7], 0.5)


if __name__ == "__main__":
    unittest.main()
