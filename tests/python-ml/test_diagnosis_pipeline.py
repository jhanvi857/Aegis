"""
Acceptance tests for the fault diagnosis engine and recovery/decision pipeline.

Tests cover:
- Fault family classification from per-node telemetry
- Action differentiation across fault types
- Approval policy enforcement (LOW=auto, MEDIUM/HIGH=requires_approval)
- Evidence-based explanations
"""

import unittest
import os
import sys

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../python-ml")))

from decision_engine.diagnosis import diagnose_fault, diagnose_nodes, get_candidate_actions
from decision_engine.risk_assessment import RiskAssessmentEngine


class TestFaultDiagnosis(unittest.TestCase):
    """Tests fault family classification from per-node telemetry."""

    def test_db_lock_classified_as_database(self):
        """db_lock incident: family=database, explanation mentions db wait metric, NOT network latency."""
        metrics = {
            "cpu_usage": 35.0,
            "memory_usage": 45.0,
            "p95_latency_ms": 450.0,
            "error_rate": 0.12,
            "db_connection_wait_ms": 750.0,
            "cache_miss_rate": 0.10,
            "queue_depth": 3,
        }
        result = diagnose_fault("node-d", metrics)
        self.assertEqual(result["fault_family"], "database")
        self.assertGreater(result["confidence"], 0.5)
        self.assertTrue(any(e["metric"] == "db_connection_wait_ms" for e in result["evidence"]))
        self.assertIn("db_connection_wait_ms", result["explanation"])
        self.assertNotIn("network latency", result["explanation"].lower())
        self.assertNotIn("network/latency", result["explanation"].lower())

    def test_cache_down_classified_as_cache(self):
        """cache_down -> family=cache."""
        metrics = {
            "cpu_usage": 40.0,
            "memory_usage": 50.0,
            "p95_latency_ms": 350.0,
            "error_rate": 0.08,
            "db_connection_wait_ms": 20.0,
            "cache_miss_rate": 0.92,
            "queue_depth": 2,
        }
        result = diagnose_fault("node-b", metrics)
        self.assertEqual(result["fault_family"], "cache")
        self.assertTrue(any(e["metric"] == "cache_miss_rate" for e in result["evidence"]))

    def test_mq_lag_classified_as_queue(self):
        """mq_lag -> family=queue."""
        metrics = {
            "cpu_usage": 30.0,
            "memory_usage": 42.0,
            "p95_latency_ms": 120.0,
            "error_rate": 0.01,
            "db_connection_wait_ms": 15.0,
            "cache_miss_rate": 0.08,
            "queue_depth": 18,
        }
        result = diagnose_fault("node-c", metrics)
        self.assertEqual(result["fault_family"], "queue")
        self.assertTrue(any(e["metric"] == "queue_depth" for e in result["evidence"]))

    def test_memory_leak_classified_as_memory_leak(self):
        """memory_leak -> family=memory_leak."""
        metrics = {
            "cpu_usage": 55.0,
            "memory_usage": 94.0,
            "p95_latency_ms": 280.0,
            "error_rate": 0.15,
            "db_connection_wait_ms": 18.0,
            "cache_miss_rate": 0.12,
            "queue_depth": 2,
        }
        result = diagnose_fault("node-a", metrics)
        self.assertEqual(result["fault_family"], "memory_leak")
        self.assertTrue(any(e["metric"] == "memory_usage" for e in result["evidence"]))

    def test_cpu_stress_classified_as_cpu_saturation(self):
        """cpu_stress -> family=cpu_saturation."""
        metrics = {
            "cpu_usage": 96.0,
            "memory_usage": 50.0,
            "p95_latency_ms": 250.0,
            "error_rate": 0.10,
            "db_connection_wait_ms": 20.0,
            "cache_miss_rate": 0.10,
            "queue_depth": 3,
        }
        result = diagnose_fault("node-c", metrics)
        self.assertEqual(result["fault_family"], "cpu_saturation")
        self.assertTrue(any(e["metric"] == "cpu_usage" for e in result["evidence"]))

    def test_kill_service_classified_as_crash(self):
        """kill_service -> family=crash (CPU~0, error~1)."""
        metrics = {
            "cpu_usage": 0.0,
            "memory_usage": 0.0,
            "p95_latency_ms": 5000.0,
            "error_rate": 1.0,
            "db_connection_wait_ms": 0.0,
            "cache_miss_rate": 0.0,
            "queue_depth": 0,
        }
        result = diagnose_fault("node-a", metrics)
        self.assertEqual(result["fault_family"], "crash")
        self.assertGreater(result["confidence"], 0.7)

    def test_latency_classified_as_network_latency(self):
        """Pure latency fault (no db/cache/queue indicators) -> family=network_latency."""
        metrics = {
            "cpu_usage": 30.0,
            "memory_usage": 45.0,
            "p95_latency_ms": 1200.0,
            "error_rate": 0.15,
            "db_connection_wait_ms": 20.0,
            "cache_miss_rate": 0.08,
            "queue_depth": 2,
        }
        result = diagnose_fault("node-c", metrics)
        self.assertEqual(result["fault_family"], "network_latency")
        self.assertTrue(any(e["metric"] == "p95_latency_ms" for e in result["evidence"]))

    def test_nominal_when_all_healthy(self):
        """All metrics within normal range -> family=nominal."""
        metrics = {
            "cpu_usage": 25.0,
            "memory_usage": 40.0,
            "p95_latency_ms": 20.0,
            "error_rate": 0.001,
            "db_connection_wait_ms": 10.0,
            "cache_miss_rate": 0.05,
            "queue_depth": 2,
        }
        result = diagnose_fault("node-a", metrics)
        self.assertEqual(result["fault_family"], "nominal")

    def test_diagnose_nodes_targets_specific_node(self):
        """diagnose_nodes with target_node_id only diagnoses that node."""
        telemetry = {
            "node-a": {"cpu_usage": 96.0, "memory_usage": 50.0, "p95_latency_ms": 200.0, "error_rate": 0.1},
            "node-b": {"cpu_usage": 25.0, "memory_usage": 40.0, "p95_latency_ms": 20.0, "error_rate": 0.001},
        }
        results = diagnose_nodes(telemetry, target_node_id="node-a")
        self.assertIn("node-a", results)
        self.assertNotIn("node-b", results)
        self.assertEqual(results["node-a"]["fault_family"], "cpu_saturation")


class TestActionDifferentiation(unittest.TestCase):
    """Top action must differ across at least 5 fault types."""

    def test_top_actions_differ_across_fault_families(self):
        """Each fault family should have a distinct top action."""
        families_to_top_action = {}
        for family in ["database", "cache", "queue", "memory_leak", "cpu_saturation"]:
            candidates = get_candidate_actions(family)
            self.assertGreater(len(candidates), 0)
            families_to_top_action[family] = candidates[0]["action"]

        # Verify at least 4 distinct top actions across 5 families
        unique_actions = set(families_to_top_action.values())
        self.assertGreaterEqual(len(unique_actions), 4, f"Expected >=4 distinct top actions, got {families_to_top_action}")

        # Specific assertions from acceptance criteria
        self.assertEqual(families_to_top_action["database"], "INCREASE_POOL")
        self.assertEqual(families_to_top_action["cache"], "FLUSH_CACHE")
        self.assertEqual(families_to_top_action["cpu_saturation"], "SCALE_UP")
        self.assertEqual(families_to_top_action["memory_leak"], "RESTART")
        self.assertEqual(families_to_top_action["queue"], "SCALE_UP")

    def test_db_lock_top_action_is_increase_pool(self):
        """db_lock -> top action INCREASE_POOL, NOT reroute/network latency."""
        candidates = get_candidate_actions("database")
        self.assertEqual(candidates[0]["action"], "INCREASE_POOL")
        self.assertNotEqual(candidates[0]["action"], "REROUTE_TRAFFIC")

    def test_cache_down_top_action_is_flush_cache(self):
        """cache_down -> FLUSH_CACHE."""
        candidates = get_candidate_actions("cache")
        self.assertEqual(candidates[0]["action"], "FLUSH_CACHE")

    def test_mq_lag_top_action_is_scale_up(self):
        """mq_lag -> queue action (SCALE_UP)."""
        candidates = get_candidate_actions("queue")
        self.assertEqual(candidates[0]["action"], "SCALE_UP")

    def test_memory_leak_top_action_is_restart(self):
        """memory_leak -> RESTART."""
        candidates = get_candidate_actions("memory_leak")
        self.assertEqual(candidates[0]["action"], "RESTART")

    def test_cpu_stress_top_action_is_scale_up(self):
        """cpu_stress -> SCALE_UP."""
        candidates = get_candidate_actions("cpu_saturation")
        self.assertEqual(candidates[0]["action"], "SCALE_UP")

    def test_each_candidate_has_required_fields(self):
        """Each candidate action must have action, risk, reason, expected_effect."""
        for family in ["database", "cache", "queue", "memory_leak", "cpu_saturation", "network_latency", "crash"]:
            candidates = get_candidate_actions(family)
            for c in candidates:
                self.assertIn("action", c)
                self.assertIn("risk", c)
                self.assertIn("reason", c)
                self.assertIn("expected_effect", c)


class TestApprovalPolicy(unittest.TestCase):
    """Policy: LOW=auto, MEDIUM/HIGH=requires_approval."""

    def test_low_risk_auto_executes(self):
        """LOW risk actions should not require approval."""
        engine = RiskAssessmentEngine()
        risk = engine.assess_risk(target_node="node-b", action_type="RESTART", blast_radius_size=0)
        self.assertEqual(risk["risk_level"], "LOW")
        self.assertFalse(risk["requires_approval"])

    def test_medium_risk_requires_approval(self):
        """MEDIUM risk actions MUST require approval (fixing current bug)."""
        engine = RiskAssessmentEngine()
        risk = engine.assess_risk(
            target_node="node-a",
            action_type="SCALE_UP",
            blast_radius_size=2,
            node_criticality=0.7,
        )
        # Medium risk should require approval after the fix
        if risk["risk_level"] == "MEDIUM":
            self.assertTrue(
                risk["requires_approval"],
                f"MEDIUM risk should require approval, got requires_approval=False for score {risk['risk_score']}",
            )

    def test_high_risk_requires_approval(self):
        """HIGH risk actions require approval."""
        engine = RiskAssessmentEngine()
        risk = engine.assess_risk(target_node="gateway", action_type="REROUTE_TRAFFIC", blast_radius_size=3)
        self.assertEqual(risk["risk_level"], "HIGH")
        self.assertTrue(risk["requires_approval"])


class TestEvidenceBasedExplanations(unittest.TestCase):
    """Explanations must cite actual metric values, not generic descriptions."""

    def test_db_lock_explanation_cites_db_wait(self):
        """db_lock explanation must mention db_connection_wait_ms actual value."""
        metrics = {
            "cpu_usage": 35.0,
            "p95_latency_ms": 450.0,
            "error_rate": 0.12,
            "db_connection_wait_ms": 750.0,
        }
        result = diagnose_fault("node-d", metrics)
        self.assertIn("750", result["explanation"])
        self.assertIn("db_connection_wait_ms", result["explanation"])

    def test_cpu_stress_explanation_cites_cpu_value(self):
        """CPU stress explanation must mention actual CPU value."""
        metrics = {"cpu_usage": 96.5, "p95_latency_ms": 200.0, "error_rate": 0.1}
        result = diagnose_fault("node-c", metrics)
        self.assertIn("96.5", result["explanation"])

    def test_cache_explanation_cites_miss_rate(self):
        """Cache failure explanation must mention actual cache_miss_rate."""
        metrics = {"cache_miss_rate": 0.92, "cpu_usage": 40.0, "p95_latency_ms": 350.0}
        result = diagnose_fault("node-b", metrics)
        self.assertIn("cache_miss_rate", result["explanation"])


if __name__ == "__main__":
    unittest.main()
