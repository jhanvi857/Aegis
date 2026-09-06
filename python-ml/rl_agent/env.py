"""
Simulated Gym environment for Aegis recovery RL agent (Phase 6)
"""

from typing import Tuple, Dict, Any


class MicroserviceChaosEnv:
    def __init__(self, topology_path: str = "configs/topology.yaml"):
        self.topology_path = topology_path
        self.state = None

    def reset(self) -> Dict[str, Any]:
        """Resets the environment to nominal state"""
        self.state = {"status": "HEALTHY"}
        return self.state

    def step(self, action: int) -> Tuple[Dict[str, Any], float, bool, Dict[str, Any]]:
        """Applies remediation action, returns (next_state, reward, done, info)"""
        reward = 1.0
        done = True
        return self.state, reward, done, {}
