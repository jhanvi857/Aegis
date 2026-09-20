"""
Unit and Integration Tests for Phase 4 (Decision Engine & Recovery Planner)
Verifies:
- Rulebook condition matching and action selection
- Risk assessment calculation and approval gating
- Topological ordering over SCC-condensed graphs (both acyclic and cyclic)
- Full RecoveryPlan building conforming to proto/recovery.proto
"""

import unittest
import os
import sys
import networkx as nx

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from decision_engine.risk_assessment import RiskAssessmentEngine
from recovery_planner.dependency_order import compute_recovery_order
from recovery_planner.plan_builder import RecoveryPlanBuilder


class TestPhase4DecisionEngine(unittest.TestCase):
    def setUp(self):
        self.engine = RiskAssessmentEngine()

    def test_rules_loaded(self):
        """Rulebook must load default or yaml rules successfully."""
        self.assertGreaterEqual(len(self.engine.rules), 6)
        rule_ids = {r["id"] for r in self.engine.rules}
        self.assertIn("RULE_OOM_LEAK", rule_ids)
        self.assertIn("RULE_HIGH_CPU_CASCADE", rule_ids)
        self.assertIn("RULE_HIGH_LATENCY_DEADLOCK", rule_ids)
        self.assertIn("RULE_CACHE_DEGRADATION", rule_ids)
        self.assertIn("RULE_DB_POOL_EXHAUSTION", rule_ids)
        self.assertIn("RULE_RUNAWAY_ERROR_CASCADE", rule_ids)

    def test_rule_matching(self):
        """Matches anomalous metrics to appropriate remediation actions."""
        # OOM
        match_oom = self.engine.match_rule({"memory_usage_percent": 94.5})
        self.assertIsNotNone(match_oom)
        self.assertEqual(match_oom["action"]["type"], "RESTART")

        # CPU
        match_cpu = self.engine.match_rule({"cpu_usage_percent": 88.0})
        self.assertIsNotNone(match_cpu)
        self.assertEqual(match_cpu["action"]["type"], "SCALE_UP")

        # Latency
        match_lat = self.engine.match_rule({"latency_p99_ms": 1450.0})
        self.assertIsNotNone(match_lat)
        self.assertEqual(match_lat["action"]["type"], "REROUTE_TRAFFIC")

        # Cache
        match_cache = self.engine.match_rule({"cache_miss_rate": 0.82})
        self.assertIsNotNone(match_cache)
        self.assertEqual(match_cache["action"]["type"], "FLUSH_CACHE")

        # DB pool
        match_pool = self.engine.match_rule({"db_connection_wait_ms": 720.0})
        self.assertIsNotNone(match_pool)
        self.assertEqual(match_pool["action"]["type"], "INCREASE_POOL")

        # Error cascade
        match_err = self.engine.match_rule({"error_rate": 0.65})
        self.assertIsNotNone(match_err)
        self.assertEqual(match_err["action"]["type"], "ISOLATE")

    def test_risk_assessment_low_risk(self):
        """Low risk action on isolated node requires no operator approval."""
        risk = self.engine.assess_risk(target_node="node-b", action_type="RESTART", blast_radius_size=0)
        self.assertEqual(risk["risk_level"], "LOW")
        self.assertFalse(risk["requires_approval"])
        self.assertLess(risk["risk_score"], 0.40)

    def test_risk_assessment_high_risk_actions(self):
        """High-impact actions (REROUTE_TRAFFIC, ISOLATE) require approval."""
        risk_reroute = self.engine.assess_risk(target_node="gateway", action_type="REROUTE_TRAFFIC", blast_radius_size=2)
        self.assertEqual(risk_reroute["risk_level"], "HIGH")
        self.assertTrue(risk_reroute["requires_approval"])

        risk_isolate = self.engine.assess_risk(target_node="node-a", action_type="ISOLATE", blast_radius_size=1)
        self.assertEqual(risk_isolate["risk_level"], "HIGH")
        self.assertTrue(risk_isolate["requires_approval"])

    def test_risk_assessment_blast_radius_escalation(self):
        """Actions affecting >3 downstream nodes escalate to HIGH risk."""
        risk = self.engine.assess_risk(target_node="node-a", action_type="RESTART", blast_radius_size=4)
        self.assertEqual(risk["risk_level"], "HIGH")
        self.assertTrue(risk["requires_approval"])


class TestPhase4RecoveryPlanner(unittest.TestCase):
    def setUp(self):
        self.builder = RecoveryPlanBuilder()

        # Build realistic DAG: gateway -> node-a -> node-c -> node-d
        self.dag = nx.DiGraph()
        self.dag.add_edge("gateway", "node-a")
        self.dag.add_edge("node-a", "node-b")
        self.dag.add_edge("node-a", "node-c")
        self.dag.add_edge("node-c", "node-d")

    def test_acyclic_dependency_ordering(self):
        """Dependencies heal bottom-up so callers recover against working downstream nodes."""
        affected = {"gateway", "node-a", "node-c", "node-d"}
        order = compute_recovery_order(self.dag, affected, heal_dependencies_first=True)

        self.assertIn("node-d", order)
        self.assertIn("node-c", order)
        self.assertIn("node-a", order)
        self.assertIn("gateway", order)

        # node-d must precede node-c, node-c must precede node-a, node-a must precede gateway
        idx_d = order.index("node-d")
        idx_c = order.index("node-c")
        idx_a = order.index("node-a")
        idx_gw = order.index("gateway")

        self.assertLess(idx_d, idx_c)
        self.assertLess(idx_c, idx_a)
        self.assertLess(idx_a, idx_gw)

    def test_cyclic_scc_condensation_ordering(self):
        """Cyclic topologies with retry feedback loops must not crash topological sort."""
        cyclic_graph = nx.DiGraph()
        cyclic_graph.add_edge("gateway", "node-a")
        cyclic_graph.add_edge("node-a", "node-b")
        cyclic_graph.add_edge("node-b", "node-c")
        cyclic_graph.add_edge("node-c", "node-b")  # Cycle: node-b <-> node-c
        cyclic_graph.add_edge("node-c", "node-d")

        affected = {"node-a", "node-b", "node-c", "node-d"}
        # Must execute cleanly using SCC condensation without nx.NetworkXUnfeasible cycle exception
        order = compute_recovery_order(cyclic_graph, affected, heal_dependencies_first=True)
        self.assertEqual(len(order), 4)
        self.assertTrue(set(order) == affected)

    def test_build_plan_end_to_end(self):
        """Generates a complete protobuf-compliant RecoveryPlan."""
        root_causes = [
            {
                "node_id": "node-c",
                "probability": 0.96,
                "contributing_metrics": ["high_cpu_usage_percent", "latency_spike"],
            }
        ]
        propagation_set = ["node-d"]
        telemetry = {
            "node-c": {"cpu_usage_percent": 91.0},
            "node-d": {"memory_usage_percent": 92.0},
        }

        plan = self.builder.build_plan(
            graph=self.dag,
            root_causes=root_causes,
            propagation_set=propagation_set,
            telemetry_by_node=telemetry,
        )

        self.assertTrue(plan["plan_id"].startswith("plan-"))
        self.assertEqual(len(plan["actions"]), 2)
        self.assertIn("risk", plan)
        self.assertIn("risk_level", plan["risk"])
        self.assertIn("requires_approval", plan["risk"])
        self.assertIn("justification", plan["risk"])

        # Check action steps
        for action in plan["actions"]:
            self.assertIn("action_id", action)
            self.assertIn(action["action_type"], ["SCALE_UP", "RESTART", "REROUTE_TRAFFIC", "FLUSH_CACHE"])
            self.assertIn(action["target_node_id"], ["node-c", "node-d"])
            self.assertGreaterEqual(action["execution_order"], 1)

    def test_live_prediction_engine_action_selection(self):
        """
        Verifies that PredictionEngine.predict() in the actual serving path
        converts live telemetry batches into telemetry_by_node and selects
        root-cause proportional actions (e.g. SCALE_UP for CPU, INCREASE_POOL for DB)
        rather than falling through to default RESTART.
        """
        from serving.grpc_server import PredictionEngine
        from graph.service import GraphPreprocessingService
        from ingestion.kafka_consumer import TelemetryConsumer

        service = GraphPreprocessingService(topology_path="configs/topology.yaml", structural_ttl_sec=0.01)
        engine = PredictionEngine(checkpoint_path="python-ml/models/checkpoints/tgnn_best.pt")
        consumer = TelemetryConsumer()

        # 1. Pipeline-generated CPU stress batch -> triggers SCALE_UP
        cpu_batch = consumer.generate_simulated_batch(fault_node="node-c", fault_type="cpu_stress")
        rep_cpu = service.process_telemetry_batch(cpu_batch)
        pred_cpu = engine.predict(rep_cpu, telemetry_batch=cpu_batch, target_node_id="node-c")
        plan_cpu = pred_cpu.get("recovery_plan")
        self.assertIsNotNone(plan_cpu)
        self.assertGreater(len(plan_cpu["actions"]), 0)
        action_types_cpu = [a["action_type"] for a in plan_cpu["actions"]]
        self.assertIn("SCALE_UP", action_types_cpu)

        # 2. Pipeline-generated DB lock batch -> natively contains db_connection_wait_ms >= 500ms -> triggers INCREASE_POOL
        db_batch = consumer.generate_simulated_batch(fault_node="node-d", fault_type="db_lock")
        rep_db = service.process_telemetry_batch(db_batch)
        pred_db = engine.predict(rep_db, telemetry_batch=db_batch, target_node_id="node-d")
        plan_db = pred_db.get("recovery_plan")
        self.assertIsNotNone(plan_db)
        action_types_db = [a["action_type"] for a in plan_db["actions"]]
        self.assertIn("INCREASE_POOL", action_types_db)

        # 3. Pipeline-generated cache down batch -> natively contains cache_miss_rate >= 0.70 -> triggers FLUSH_CACHE
        cache_batch = consumer.generate_simulated_batch(fault_node="node-b", fault_type="cache_down")
        rep_cache = service.process_telemetry_batch(cache_batch)
        pred_cache = engine.predict(rep_cache, telemetry_batch=cache_batch, target_node_id="node-b")
        plan_cache = pred_cache.get("recovery_plan")
        self.assertIsNotNone(plan_cache)
        action_types_cache = [a["action_type"] for a in plan_cache["actions"]]
        self.assertIn("FLUSH_CACHE", action_types_cache)


if __name__ == "__main__":
    unittest.main()

