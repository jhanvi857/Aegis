"""
Risk Assessment and Decision Engine (Phase 4)
Evaluates anomalous telemetry metrics against rulebook specifications, selects remediation
actions, and computes risk scores and approval gates.
"""

import os
import yaml
from typing import Dict, Any, List, Optional

try:
    from .diagnosis import diagnose_fault, get_candidate_actions
except (ImportError, ValueError):
    try:
        from decision_engine.diagnosis import diagnose_fault, get_candidate_actions
    except (ImportError, ValueError):
        diagnose_fault = None
        get_candidate_actions = None


class RiskAssessmentEngine:
    """
    Evaluates observed system anomalies against rulebook specifications to prescribe
    remediation actions and determine risk boundaries.
    """

    def __init__(self, rulebook_path: Optional[str] = None):
        if rulebook_path is None:
            # Resolve relative to current file
            base_dir = os.path.dirname(os.path.abspath(__file__))
            rulebook_path = os.path.join(base_dir, "rulebook.yaml")
        self.rulebook_path = rulebook_path
        self.rules = self._load_rules()

    def _load_rules(self) -> List[Dict[str, Any]]:
        """Loads and parses rulebook.yaml with graceful fallback defaults."""
        if os.path.exists(self.rulebook_path):
            try:
                with open(self.rulebook_path, "r", encoding="utf-8") as f:
                    data = yaml.safe_load(f)
                    if isinstance(data, dict) and "rules" in data:
                        return data["rules"]
            except Exception:
                pass

        # Fallback default rules
        return [
            {
                "id": "RULE_OOM_LEAK",
                "condition": {"metric": "memory_usage_percent", "operator": ">=", "threshold": 90.0},
                "action": {"type": "RESTART", "risk": "LOW", "approval_required": False, "parameters": {"grace_period_sec": "5"}},
            },
            {
                "id": "RULE_HIGH_CPU_CASCADE",
                "condition": {"metric": "cpu_usage_percent", "operator": ">=", "threshold": 85.0},
                "action": {"type": "SCALE_UP", "risk": "MEDIUM", "approval_required": False, "parameters": {"scale_increment": "2"}},
            },
            {
                "id": "RULE_HIGH_LATENCY_DEADLOCK",
                "condition": {"metric": "latency_p99_ms", "operator": ">=", "threshold": 1000.0},
                "action": {"type": "REROUTE_TRAFFIC", "risk": "HIGH", "approval_required": True, "parameters": {"drain_timeout_sec": "10"}},
            },
            {
                "id": "RULE_CACHE_DEGRADATION",
                "condition": {"metric": "cache_miss_rate", "operator": ">=", "threshold": 0.70},
                "action": {"type": "FLUSH_CACHE", "risk": "LOW", "approval_required": False, "parameters": {"async_flush": "true"}},
            },
            {
                "id": "RULE_DB_POOL_EXHAUSTION",
                "condition": {"metric": "db_connection_wait_ms", "operator": ">=", "threshold": 500.0},
                "action": {"type": "INCREASE_POOL", "risk": "MEDIUM", "approval_required": False, "parameters": {"pool_increment": "10"}},
            },
            {
                "id": "RULE_RUNAWAY_ERROR_CASCADE",
                "condition": {"metric": "error_rate", "operator": ">=", "threshold": 0.50},
                "action": {"type": "ISOLATE", "risk": "HIGH", "approval_required": True, "parameters": {"circuit_break": "open"}},
            },
        ]

    def evaluate_condition(self, condition: Dict[str, Any], metrics: Dict[str, float]) -> bool:
        """Evaluates a single rule condition against observed metrics."""
        metric_name = condition.get("metric")
        if metric_name not in metrics:
            return False

        value = float(metrics[metric_name])
        threshold = float(condition.get("threshold", 0.0))
        op = condition.get("operator", ">=")

        if op == ">=":
            return value >= threshold
        elif op == ">":
            return value > threshold
        elif op == "<=":
            return value <= threshold
        elif op == "<":
            return value < threshold
        elif op == "==":
            return abs(value - threshold) < 1e-6
        return False

    def match_rule(self, metrics: Dict[str, float]) -> Optional[Dict[str, Any]]:
        """Finds the first matching rule for the observed telemetry metrics."""
        for rule in self.rules:
            cond = rule.get("condition", {})
            if self.evaluate_condition(cond, metrics):
                return rule
        return None

    def assess_risk(
        self,
        target_node: str,
        action_type: str,
        blast_radius_size: int = 0,
        node_criticality: float = 0.5,
    ) -> Dict[str, Any]:
        """
        Assesses execution risk based on blast radius, action severity, and node criticality.
        High risk triggers human-in-the-loop approval gate.
        """
        # Base risk weights by action type
        severity_map = {
            "RESTART": 0.20,
            "SCALE_UP": 0.35,
            "FLUSH_CACHE": 0.25,
            "INCREASE_POOL": 0.30,
            "REROUTE_TRAFFIC": 0.75,
            "ISOLATE": 0.85,
        }
        base_severity = severity_map.get(action_type.upper(), 0.30)

        # Blast radius amplifier: each downstream node adds risk
        radius_penalty = min(0.35, blast_radius_size * 0.10)

        # Criticality amplifier (0.0 to 0.15)
        criticality_penalty = node_criticality * 0.15

        risk_score = round(min(1.0, base_severity + radius_penalty + criticality_penalty), 3)

        # High risk threshold: score >= 0.70 OR high-impact action OR blast radius > 3
        is_high_risk = (
            risk_score >= 0.70
            or action_type.upper() in ["REROUTE_TRAFFIC", "ISOLATE"]
            or blast_radius_size > 3
        )

        if is_high_risk:
            risk_level = "HIGH"
            requires_approval = True
            justification = (
                f"Action {action_type} on {target_node} carries risk score {risk_score:.2f} "
                f"(blast radius: {blast_radius_size} downstream nodes, criticality: {node_criticality:.2f}). "
                f"Requires human authorization."
            )
        elif risk_score >= 0.40:
            risk_level = "MEDIUM"
            requires_approval = True
            justification = (
                f"Moderate risk ({risk_score:.2f}) action {action_type} on {target_node}. "
                f"Requires human approval before execution."
            )
        else:
            risk_level = "LOW"
            requires_approval = False
            justification = (
                f"Low risk ({risk_score:.2f}) standard remediation {action_type} on {target_node}. Safe for autonomous execution."
            )

        return {
            "risk_level": risk_level,
            "risk_score": risk_score,
            "requires_approval": requires_approval,
            "justification": justification,
        }

    def recommend_action(
        self,
        target_node: str,
        metrics: Dict[str, float],
        blast_radius_size: int = 0,
        node_criticality: float = 0.5,
        fault_family: Optional[str] = None,
    ) -> Dict[str, Any]:
        """
        Synthesizes fault diagnosis, rule evaluation, and risk assessment to produce a concrete recommendation.
        Diagnosis runs FIRST so specific fault families (e.g. database contention, crash) are never shadowed by
        broad fallback rule conditions (e.g. latency >= 1000 or error_rate >= 0.50).
        """
        family = fault_family
        if (not family or family == "nominal") and diagnose_fault is not None:
            diag = diagnose_fault(target_node, metrics)
            family = diag.get("fault_family")

        matched_rule_id = None
        params = {}

        # 1. Primary: If a specific fault family is identified, use fault_family_actions
        if family and family != "nominal" and get_candidate_actions is not None:
            candidates = get_candidate_actions(family)
            if candidates:
                action_type = candidates[0]["action"]
                matched_rule_id = f"RULE_{family.upper()}"
                params = {"fault_family": family}
                # Check if rulebook has tuned parameters for this action
                for rule in self.rules:
                    if rule.get("action", {}).get("type") == action_type:
                        params.update(rule["action"].get("parameters", {}))
                        matched_rule_id = rule.get("id", matched_rule_id)
                        break
            else:
                action_type = "RESTART"
                params = {"grace_period_sec": "5"}
        else:
            # 2. Secondary: If no specific fault family was detected, match generic rulebook rules
            matched = self.match_rule(metrics)
            if matched:
                action_info = matched["action"]
                action_type = action_info["type"]
                params = dict(action_info.get("parameters", {}))
                matched_rule_id = matched["id"]
            else:
                # 3. Default fallback
                action_type = "RESTART"
                params = {"grace_period_sec": "5"}

        risk = self.assess_risk(target_node, action_type, blast_radius_size, node_criticality)

        return {
            "target_node": target_node,
            "action_type": action_type,
            "parameters": params,
            "risk": risk,
            "matched_rule_id": matched_rule_id,
            "fault_family": family,
        }
