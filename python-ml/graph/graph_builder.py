"""
Builds and dynamically updates system dependency graph from topology.yaml and live telemetry.
Annotates nodes and edges with real-time health, latency, CPU, and error rate metrics.
"""

from typing import Dict, Any, List, Optional
import time
import os
import yaml
import networkx as nx
import logging

logger = logging.getLogger(__name__)


class GraphBuilder:
    """
    Constructs and maintains the authoritative live system dependency graph.
    Ingests static topology configurations and continuous telemetry metric streams.
    """

    def __init__(self, topology_path: str = "configs/topology.yaml"):
        self.topology_path = topology_path
        self.graph = nx.DiGraph()
        self.last_updated_ns = int(time.time() * 1e9)
        self.system_name = "aegis-mesh"
        self.load_topology()

    def load_topology(self, path: Optional[str] = None) -> nx.DiGraph:
        """
        Loads static topology from YAML configuration into directed graph.
        Initializes default metric profiles for each node and dependency edge.
        """
        if path:
            self.topology_path = path

        if not os.path.exists(self.topology_path):
            logger.warning(f"Topology file not found at {self.topology_path}, initializing empty graph")
            return self.graph

        try:
            with open(self.topology_path, "r") as f:
                data = yaml.safe_load(f)

            self.system_name = data.get("system_name", "aegis-mesh")
            self.graph.clear()
            nodes_config = data.get("nodes", [])

            now_ns = int(time.time() * 1e9)
            self.last_updated_ns = now_ns

            for node in nodes_config:
                node_id = node["id"]
                self.graph.add_node(
                    node_id,
                    id=node_id,
                    name=node.get("name", node_id),
                    type=node.get("type", "service"),
                    host=node.get("host", node_id),
                    port=node.get("port", 8080),
                    health_endpoint=node.get("health_endpoint", "/health"),
                    metrics_endpoint=node.get("metrics_endpoint", "/metrics"),
                    status="healthy",
                    metadata=node.get("metadata", {}),
                    metrics={
                        "cpu_usage": 20.0,
                        "memory_usage": 40.0,
                        "p95_latency_ms": 25.0,
                        "error_rate": 0.001,
                        "rps": 150.0,
                    },
                    last_updated_ns=now_ns,
                )

            for node in nodes_config:
                source_id = node["id"]
                for dep_id in node.get("dependencies", []):
                    self.graph.add_edge(
                        source_id,
                        dep_id,
                        relationship="calls",
                        latency_ms=25.0,
                        error_rate=0.001,
                        throughput=150.0,
                        last_updated_ns=now_ns,
                    )

            logger.info(
                f"Successfully loaded topology '{self.system_name}': "
                f"{self.graph.number_of_nodes()} nodes, {self.graph.number_of_edges()} edges"
            )
            return self.graph

        except Exception as e:
            logger.error(f"Failed to load topology from {self.topology_path}: {e}")
            return self.graph

    def update_with_telemetry(self, telemetry_data: Dict[str, Any]) -> int:
        """
        Annotates graph nodes and caller-callee edges with real-time metrics.
        Computes dynamic node health states (healthy, degraded, critical).
        Returns number of updated nodes.
        """
        now_ns = telemetry_data.get("timestamp_unix_nano", int(time.time() * 1e9))
        self.last_updated_ns = now_ns
        updated_nodes = set()

        metrics_list = telemetry_data.get("metrics", [])
        logs_list = telemetry_data.get("logs", [])

        # Group metrics by service_id
        node_metric_updates: Dict[str, Dict[str, float]] = {}
        for m in metrics_list:
            svc_id = m.get("service_id")
            name = m.get("metric_name")
            val = float(m.get("value", 0.0))
            if svc_id and name:
                if svc_id not in node_metric_updates:
                    node_metric_updates[svc_id] = {}
                node_metric_updates[svc_id][name] = val

        # Apply metric updates to existing or dynamically discovered nodes
        for node_id, metrics in node_metric_updates.items():
            if node_id not in self.graph:
                self.graph.add_node(
                    node_id,
                    id=node_id,
                    name=node_id,
                    type="service",
                    status="healthy",
                    metadata={},
                    metrics={},
                    last_updated_ns=now_ns,
                )

            current_metrics = self.graph.nodes[node_id].get("metrics", {})
            current_metrics.update(metrics)
            self.graph.nodes[node_id]["metrics"] = current_metrics
            self.graph.nodes[node_id]["last_updated_ns"] = now_ns

            # Dynamic Health Assessment
            cpu = current_metrics.get("cpu_usage", 0.0)
            mem = current_metrics.get("memory_usage", 0.0)
            lat = current_metrics.get("p95_latency_ms", 0.0)
            err = current_metrics.get("error_rate", 0.0)

            if err >= 0.20 or lat >= 3000.0:
                status = "critical"
            elif cpu >= 85.0 or mem >= 85.0 or err >= 0.05 or lat >= 500.0:
                status = "degraded"
            else:
                status = "healthy"

            self.graph.nodes[node_id]["status"] = status
            updated_nodes.add(node_id)

            # Propagate updated latency and error rate to incoming edges
            for caller in self.graph.predecessors(node_id):
                if self.graph.has_edge(caller, node_id):
                    self.graph.edges[caller, node_id]["latency_ms"] = lat
                    self.graph.edges[caller, node_id]["error_rate"] = err
                    if "rps" in current_metrics:
                        self.graph.edges[caller, node_id]["throughput"] = current_metrics["rps"]
                    self.graph.edges[caller, node_id]["last_updated_ns"] = now_ns

        # Scan error logs for acute node degradation
        for log in logs_list:
            svc_id = log.get("service_id")
            lvl = log.get("level", "").upper()
            if svc_id in self.graph and lvl in ("ERROR", "FATAL"):
                if self.graph.nodes[svc_id]["status"] == "healthy":
                    self.graph.nodes[svc_id]["status"] = "degraded"
                updated_nodes.add(svc_id)

        return len(updated_nodes)

    def get_snapshot_dict(self) -> Dict[str, Any]:
        """
        Returns structured snapshot dictionary conforming to proto/graph.proto:GraphSnapshot.
        """
        nodes_list = []
        for node_id, data in self.graph.nodes(data=True):
            nodes_list.append({
                "id": node_id,
                "name": data.get("name", node_id),
                "type": data.get("type", "service"),
                "status": data.get("status", "healthy"),
                "metadata": {str(k): str(v) for k, v in data.get("metadata", {}).items()},
                "metrics": {str(k): float(v) for k, v in data.get("metrics", {}).items()},
            })

        edges_list = []
        for src, dst, data in self.graph.edges(data=True):
            edges_list.append({
                "source": src,
                "target": dst,
                "relationship": data.get("relationship", "calls"),
                "latency_ms": float(data.get("latency_ms", 0.0)),
                "error_rate": float(data.get("error_rate", 0.0)),
                "throughput": float(data.get("throughput", 0.0)),
            })

        return {
            "timestamp_unix_nano": self.last_updated_ns,
            "nodes": nodes_list,
            "edges": edges_list,
        }

    def export_topology(self) -> Dict[str, Any]:
        """
        Exports clean topology metadata payload for dashboard and REST endpoints.
        """
        return {
            "system_name": self.system_name,
            "num_nodes": self.graph.number_of_nodes(),
            "num_edges": self.graph.number_of_edges(),
            "snapshot": self.get_snapshot_dict(),
        }
