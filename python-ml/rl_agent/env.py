"""
Reinforcement Learning Microservice Environment (Phase 6 Stretch Goal)
Gym-compatible environment simulating microservice topology, chaos fault cascades,
and recovery action responses for learned autonomous remediation policies.
"""

import os
import random
import yaml
from typing import Tuple, Dict, Any, List
import numpy as np


class MicroserviceChaosEnv:
    """
    Simulated Gym environment for Aegis recovery RL agent.
    Models inter-service cascades, latency propagation, and remediation dynamics.
    """

    ACTION_TYPES = ["NOOP", "RESTART", "SCALE_UP", "REROUTE_TRAFFIC", "FLUSH_CACHE"]

    def __init__(
        self,
        topology_path: str = "configs/topology.yaml",
        max_steps: int = 25,
    ):
        self.topology_path = topology_path
        self.max_steps = max_steps
        self.current_step = 0
        self.nodes, self.dependencies = self._load_topology()
        self.node_ids = sorted(list(self.nodes.keys()))
        self.num_nodes = len(self.node_ids)

        # Action space: 0 = NOOP, then for each node: 4 remediation types
        # total actions = 1 + (num_nodes * (len(ACTION_TYPES) - 1))
        self.action_space_size = 1 + (self.num_nodes * (len(self.ACTION_TYPES) - 1))

        # State feature dim per node: [status_code, cpu/100, memory/100, latency/2000, error_rate] = 5 dims
        self.feature_dim_per_node = 5
        self.state_dim = self.num_nodes * self.feature_dim_per_node

        self.service_states: Dict[str, Dict[str, Any]] = {}
        self.reset()

    def _load_topology(self) -> Tuple[Dict[str, Any], Dict[str, List[str]]]:
        """Loads nodes and dependency graph from topology.yaml."""
        nodes = {}
        dependencies = {}

        if os.path.exists(self.topology_path):
            try:
                with open(self.topology_path, "r", encoding="utf-8") as f:
                    data = yaml.safe_load(f)
                    for n in data.get("nodes", []):
                        nid = n["id"]
                        nodes[nid] = n
                        dependencies[nid] = n.get("dependencies", [])
            except Exception:
                pass

        if not nodes:
            # Fallback generic 5-node topology
            for nid in ["gateway", "node-a", "node-b", "node-c", "node-d"]:
                nodes[nid] = {"id": nid, "name": nid}
            dependencies = {
                "gateway": ["node-a"],
                "node-a": ["node-b", "node-c"],
                "node-b": [],
                "node-c": ["node-d"],
                "node-d": [],
            }

        return nodes, dependencies

    def reset(self, fault_node: str = None) -> np.ndarray:
        """Resets the environment and injects an initial chaos anomaly."""
        self.current_step = 0
        self.service_states = {}

        for nid in self.node_ids:
            self.service_states[nid] = {
                "status": "healthy",
                "cpu": 20.0 + random.uniform(-5.0, 5.0),
                "memory": 35.0 + random.uniform(-5.0, 5.0),
                "latency": 15.0 + random.uniform(-3.0, 3.0),
                "error_rate": 0.001,
                "replicas": 2,
            }

        # Inject initial fault
        target = fault_node if fault_node in self.service_states else random.choice(self.node_ids)
        fault_type = random.choice(["cpu", "memory", "latency"])
        if fault_type == "cpu":
            self.service_states[target]["status"] = "degraded"
            self.service_states[target]["cpu"] = 92.0
            self.service_states[target]["latency"] = 450.0
        elif fault_type == "memory":
            self.service_states[target]["status"] = "critical"
            self.service_states[target]["memory"] = 96.0
            self.service_states[target]["error_rate"] = 0.25
        else:
            self.service_states[target]["status"] = "critical"
            self.service_states[target]["latency"] = 1400.0
            self.service_states[target]["error_rate"] = 0.45

        # Cascade once
        self._propagate_cascades()

        return self.get_state_vector()

    def decode_action(self, action_idx: int) -> Tuple[str, str]:
        """Decodes discrete action index to (action_type, target_node)."""
        if action_idx == 0:
            return "NOOP", "none"

        offset = action_idx - 1
        num_remediations = len(self.ACTION_TYPES) - 1
        node_idx = offset // num_remediations
        rem_idx = offset % num_remediations

        node_id = self.node_ids[min(node_idx, self.num_nodes - 1)]
        action_type = self.ACTION_TYPES[1 + rem_idx]
        return action_type, node_id

    def step(self, action: int) -> Tuple[np.ndarray, float, bool, Dict[str, Any]]:
        """
        Applies remediation action, simulates cascade dynamics, and returns
        (next_state, reward, done, info).
        """
        self.current_step += 1
        action_type, target_node = self.decode_action(action)
        action_cost = -0.5 if action_type != "NOOP" else 0.0

        # Apply remediation
        if action_type == "RESTART" and target_node in self.service_states:
            svc = self.service_states[target_node]
            svc["cpu"] = 20.0
            svc["memory"] = 35.0
            svc["latency"] = 15.0
            svc["error_rate"] = 0.001
            svc["status"] = "healthy"
        elif action_type == "SCALE_UP" and target_node in self.service_states:
            svc = self.service_states[target_node]
            svc["cpu"] = max(20.0, svc["cpu"] - 50.0)
            svc["latency"] = max(15.0, svc["latency"] * 0.5)
            svc["replicas"] += 2
            if svc["cpu"] < 60.0 and svc["error_rate"] < 0.05:
                svc["status"] = "healthy"
        elif action_type == "FLUSH_CACHE" and target_node in self.service_states:
            svc = self.service_states[target_node]
            svc["memory"] = 25.0
            svc["latency"] = max(15.0, svc["latency"] * 0.4)
            svc["status"] = "healthy"
        elif action_type == "REROUTE_TRAFFIC" and target_node in self.service_states:
            svc = self.service_states[target_node]
            svc["latency"] = 25.0
            svc["error_rate"] = 0.001
            svc["status"] = "healthy"

        # Cascade propagation to upstream callers
        self._propagate_cascades()

        # Calculate reward
        reward = action_cost
        num_healthy = 0
        num_critical = 0

        for nid, svc in self.service_states.items():
            if svc["status"] == "healthy":
                num_healthy += 1
                reward += 2.0
            elif svc["status"] == "degraded":
                reward -= 1.5
            elif svc["status"] == "critical":
                num_critical += 1
                reward -= 4.0

        all_healthy = (num_healthy == self.num_nodes)
        if all_healthy:
            reward += 15.0  # Big bonus for complete cluster recovery

        done = all_healthy or (self.current_step >= self.max_steps)

        info = {
            "step": self.current_step,
            "action": action_type,
            "target": target_node,
            "all_healthy": all_healthy,
            "num_healthy": num_healthy,
            "num_critical": num_critical,
        }

        return self.get_state_vector(), reward, done, info

    def _propagate_cascades(self):
        """Simulates cascade degradation along upstream caller dependencies."""
        for caller, deps in self.dependencies.items():
            if caller not in self.service_states:
                continue
            caller_svc = self.service_states[caller]

            for dep in deps:
                if dep in self.service_states:
                    dep_svc = self.service_states[dep]
                    if dep_svc["status"] == "critical":
                        # Propagate latency and errors upstream
                        caller_svc["latency"] = min(2000.0, caller_svc["latency"] + dep_svc["latency"] * 0.3)
                        caller_svc["error_rate"] = min(1.0, caller_svc["error_rate"] + dep_svc["error_rate"] * 0.4)
                        if caller_svc["error_rate"] > 0.3 or caller_svc["latency"] > 800.0:
                            caller_svc["status"] = "critical"
                        else:
                            caller_svc["status"] = "degraded"

    def get_state_vector(self) -> np.ndarray:
        """Returns flattened normalized numpy state vector."""
        vec = []
        for nid in self.node_ids:
            svc = self.service_states[nid]
            status_val = 0.0 if svc["status"] == "healthy" else (0.5 if svc["status"] == "degraded" else 1.0)
            cpu_norm = min(1.0, svc["cpu"] / 100.0)
            mem_norm = min(1.0, svc["memory"] / 100.0)
            lat_norm = min(1.0, svc["latency"] / 1500.0)
            err_norm = min(1.0, svc["error_rate"])
            vec.extend([status_val, cpu_norm, mem_norm, lat_norm, err_norm])
        return np.array(vec, dtype=np.float32)
