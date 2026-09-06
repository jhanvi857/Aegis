"""
Builds executable RecoveryPlan protobuf messages (Phase 4)
"""

import time
import uuid
from typing import Dict, Any, List
import networkx as nx
from .dependency_order import compute_recovery_order


class RecoveryPlanBuilder:
    def __init__(self):
        pass

    def build_plan(self, graph: nx.DiGraph, root_causes: List[Dict[str, Any]], propagation_set: List[str]) -> Dict[str, Any]:
        """
        Builds dependency-ordered recovery plan
        """
        affected = set(propagation_set)
        for rc in root_causes:
            affected.add(rc["node_id"])

        order = compute_recovery_order(graph, affected)
        actions = []
        for i, node_id in enumerate(order):
            actions.append({
                "action_id": f"act-{uuid.uuid4().hex[:8]}",
                "action_type": "RESTART",
                "target_node_id": node_id,
                "execution_order": i + 1,
                "parameters": {"grace_period_sec": "5"}
            })

        return {
            "plan_id": f"plan-{uuid.uuid4().hex[:8]}",
            "actions": actions,
            "created_at_unix_nano": int(time.time() * 1e9),
            "risk": {
                "risk_level": "LOW",
                "risk_score": 0.1,
                "requires_approval": False,
                "justification": f"Automated restart ordered along dependency chain for {len(actions)} nodes"
            }
        }
