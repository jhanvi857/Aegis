"""
Recovery Plan Builder (Phase 4)
Generates dependency-ordered, rule-evaluated RecoveryPlan protobuf structures from
TGNN failure predictions, root-cause analysis, and topological DAG condensation.
"""

import time
import uuid
from typing import Dict, Any, List, Optional, Set
import networkx as nx

try:
    from .dependency_order import compute_recovery_order
    from ..decision_engine.risk_assessment import RiskAssessmentEngine
except (ImportError, ValueError):
    from recovery_planner.dependency_order import compute_recovery_order
    from decision_engine.risk_assessment import RiskAssessmentEngine


class RecoveryPlanBuilder:
    """
    Builds structured, dependency-safe recovery plans conforming to proto/recovery.proto.
    Integrates TGNN prediction task heads with the rulebook decision engine.
    """

    def __init__(self, rulebook_path: Optional[str] = None):
        self.risk_engine = RiskAssessmentEngine(rulebook_path=rulebook_path)

    def build_plan(
        self,
        graph: nx.DiGraph,
        root_causes: List[Dict[str, Any]],
        propagation_set: Optional[List[str]] = None,
        telemetry_by_node: Optional[Dict[str, Dict[str, float]]] = None,
    ) -> Dict[str, Any]:
        """
        Builds a complete, dependency-ordered RecoveryPlan dictionary.
        """
        now_ns = int(time.time() * 1e9)
        prop_nodes = propagation_set or []
        telemetry_by_node = telemetry_by_node or {}

        # 1. Identify all affected nodes
        affected_set: Set[str] = set(prop_nodes)
        rc_map: Dict[str, Dict[str, Any]] = {}
        for rc in root_causes:
            node_id = rc.get("node_id")
            if node_id:
                affected_set.add(node_id)
                rc_map[node_id] = rc

        if not affected_set:
            return {
                "plan_id": f"plan-empty-{uuid.uuid4().hex[:8]}",
                "actions": [],
                "created_at_unix_nano": now_ns,
                "risk": {
                    "risk_level": "LOW",
                    "risk_score": 0.0,
                    "requires_approval": False,
                    "justification": "No anomalous or degraded nodes detected.",
                },
            }

        # 2. Compute dependency-safe execution sequence
        ordered_node_ids = compute_recovery_order(
            graph,
            affected_nodes=affected_set,
            heal_dependencies_first=True,
        )

        # 3. Determine recommended action per affected node
        actions = []
        overall_requires_approval = False
        max_risk_score = 0.0
        plan_justifications = []

        for order_idx, node_id in enumerate(ordered_node_ids, start=1):
            # Extract real observed metrics for this node
            node_metrics = dict(telemetry_by_node.get(node_id, {}))

            # If telemetry was not passed directly, check graph node telemetry/metrics
            if not node_metrics and graph.has_node(node_id):
                node_data = graph.nodes[node_id]
                node_metrics.update(node_data.get("telemetry", {}))
                node_metrics.update(node_data.get("metrics", {}))

            # If root-cause item provides structured measured metric values, incorporate them
            if node_id in rc_map:
                rc_item = rc_map[node_id]
                for key in ["observed_metrics", "metric_values", "metrics"]:
                    if key in rc_item and isinstance(rc_item[key], dict):
                        for m_name, m_val in rc_item[key].items():
                            if m_name not in node_metrics:
                                node_metrics[m_name] = float(m_val)

            # Calculate blast radius size for this specific node
            downstream_count = 0
            if graph.has_node(node_id):
                try:
                    downstream_count = len(nx.descendants(graph, node_id))
                except Exception:
                    downstream_count = len(prop_nodes)

            rec = self.risk_engine.recommend_action(
                target_node=node_id,
                metrics=node_metrics,
                blast_radius_size=downstream_count,
            )

            action_type = rec["action_type"]
            params = rec["parameters"]
            risk_info = rec["risk"]

            if risk_info["requires_approval"]:
                overall_requires_approval = True
            if risk_info["risk_score"] > max_risk_score:
                max_risk_score = risk_info["risk_score"]

            plan_justifications.append(f"{action_type} on {node_id} ({risk_info['risk_level']} risk)")

            actions.append({
                "action_id": f"act-{uuid.uuid4().hex[:8]}",
                "action_type": action_type,
                "target_node_id": node_id,
                "execution_order": order_idx,
                "parameters": params,
                "risk_level": risk_info["risk_level"],
                "risk_score": risk_info["risk_score"],
            })

        # 4. Overall Plan Risk Assessment
        if max_risk_score >= 0.70 or overall_requires_approval or len(actions) > 3:
            plan_risk_level = "HIGH"
            overall_requires_approval = True
        elif max_risk_score >= 0.40:
            plan_risk_level = "MEDIUM"
        else:
            plan_risk_level = "LOW"

        justification_summary = (
            f"Ordered remediation plan with {len(actions)} steps: "
            f"{', '.join(plan_justifications)}. "
            f"Dependencies resolved via SCC condensation."
        )

        return {
            "plan_id": f"plan-{uuid.uuid4().hex[:8]}",
            "actions": actions,
            "created_at_unix_nano": now_ns,
            "risk": {
                "risk_level": plan_risk_level,
                "risk_score": round(max_risk_score, 3),
                "requires_approval": overall_requires_approval,
                "justification": justification_summary,
            },
        }
