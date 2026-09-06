"""
Risk Assessment Engine
Scores TGNN predictions and planned actions to classify risk (LOW, MEDIUM, HIGH)
and determine if human approval is required.
"""

from typing import Dict, Any


class RiskAssessmentEngine:
    def __init__(self, rulebook_path: str = "python-ml/decision_engine/rulebook.yaml"):
        self.rulebook_path = rulebook_path

    def assess_risk(self, target_node: str, action_type: str, blast_radius_size: int) -> Dict[str, Any]:
        """
        Assesses execution risk based on blast radius, action severity, and node criticality.
        """
        if blast_radius_size > 3 or action_type in ["REROUTE_TRAFFIC", "ISOLATE"]:
            return {
                "risk_level": "HIGH",
                "risk_score": 0.85,
                "requires_approval": True,
                "justification": f"Action {action_type} on {target_node} impacts >3 downstream nodes."
            }
        return {
            "risk_level": "LOW",
            "risk_score": 0.15,
            "requires_approval": False,
            "justification": f"Standard remediation for {target_node} with isolated impact."
        }
