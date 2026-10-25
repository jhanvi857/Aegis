"""
Live Kafka Episode Collector (Phase 5 - Scoped Live Telemetry Validation Bridge)
Captures real-time microservice telemetry from live Kafka streams during physical chaos runs,
processes them through the Phase 2 GraphPreprocessingService, and serializes labeled
spatio-temporal tensors for held-out empirical model validation.
"""

import os
import sys
import time
import json
import logging
import argparse
import urllib.parse
import urllib.request
from typing import Dict, Any, List, Optional
import numpy as np
import torch

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../python-ml")))

from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer

logger = logging.getLogger(__name__)


class LiveKafkaEpisodeCollector:
    """
    Subscribes to live Kafka telemetry topics ('aegis-telemetry') and records physical chaos
    episodes into standard PyTorch tensor representations for empirical validation.
    
    Acts as the bridge between Go distributed playground chaos runs and the Python ML pipeline,
    allowing pre-trained models to be evaluated against genuine physical cluster dynamics.
    """

    def __init__(
        self,
        bootstrap_servers: str = "localhost:9092",
        topic: str = "aegis-telemetry",
        topology_path: str = "configs/topology.yaml",
        output_dir: str = "dataset/live_episodes",
        sequence_length: int = 10,
        feature_dim: int = 8,
    ):
        self.bootstrap_servers = bootstrap_servers
        self.topic = topic
        self.topology_path = topology_path
        self.output_dir = output_dir
        self.sequence_length = sequence_length
        self.feature_dim = feature_dim

        os.makedirs(self.output_dir, exist_ok=True)

        self.service = GraphPreprocessingService(
            topology_path=topology_path,
            structural_ttl_sec=0.01,
        )
        self.consumer = TelemetryConsumer(
            bootstrap_servers=bootstrap_servers,
            topic=topic,
            group_id="aegis-live-dataset-collector",
        )

        initial_rep = self.service.get_latest_representation()
        self.node_ids = sorted(list(initial_rep["feature_matrix"].keys()))
        self.node_to_idx = {nid: i for i, nid in enumerate(self.node_ids)}
        self.num_nodes = len(self.node_ids)

    def record_episode(
        self,
        fault_node: Optional[str] = None,
        fault_type: Optional[str] = None,
        observation_window_sec: float = 15.0,
        poll_interval_sec: float = 1.0,
    ) -> Dict[str, Any]:
        """
        Records a single live episode over an observation window.
        Collects live telemetry batches, computes dynamic graph representations,
        and constructs feature tensors.
        """
        is_failure = fault_node is not None and fault_node in self.node_to_idx
        node_features_seq = []
        adj_seq = []

        logger.info(
            f"Recording live episode: target={fault_node or 'nominal'}, "
            f"type={fault_type or 'nominal'}, window={observation_window_sec}s"
        )

        step_count = 0

        # Connect consumer if not already running
        if not self.consumer.is_running and not self.consumer.start():
            raise RuntimeError(
                f"Kafka telemetry is unavailable at {self.bootstrap_servers}; "
                "refusing to generate synthetic fallback data"
            )

        batch_stream = self.consumer.stream_batches()

        while step_count < self.sequence_length:
            try:
                batch = next(batch_stream)
            except StopIteration as exc:
                raise RuntimeError("Kafka telemetry stream ended before the capture completed") from exc
            except Exception as exc:
                raise RuntimeError("Kafka telemetry stream failed during the capture") from exc

            rep = self.service.process_telemetry_batch(batch)

            # Node feature matrix: (N, feature_dim)
            step_feats = np.zeros((self.num_nodes, self.feature_dim), dtype=np.float32)
            for nid, feat in rep["feature_matrix"].items():
                if nid in self.node_to_idx:
                    step_feats[self.node_to_idx[nid]] = np.array(feat, dtype=np.float32)
            node_features_seq.append(step_feats)

            # Dynamic adjacency: (N, N)
            step_adj = np.zeros((self.num_nodes, self.num_nodes), dtype=np.float32)
            for edge in rep["snapshot"]["edges"]:
                src, tgt = edge["source"], edge["target"]
                if src in self.node_to_idx and tgt in self.node_to_idx:
                    lat_norm = min(1.0, edge.get("latency_ms", 0.0) / 500.0)
                    err_norm = min(1.0, edge.get("error_rate", 0.0) / 0.5)
                    weight = 1.0 + lat_norm + err_norm
                    step_adj[self.node_to_idx[src], self.node_to_idx[tgt]] = weight
            adj_seq.append(step_adj)

            step_count += 1
            time.sleep(min(0.05, poll_interval_sec / self.sequence_length))

        # Determine ground truth labels
        failure_label = 1.0 if is_failure else 0.0
        node_failures = np.zeros(self.num_nodes, dtype=np.float32)
        propagation_mask = np.zeros(self.num_nodes, dtype=np.float32)

        if is_failure:
            rc_idx = self.node_to_idx[fault_node]
            root_cause_label = rc_idx
            node_failures[rc_idx] = 1.0

            latest_rep = self.service.get_latest_representation()
            reachability = latest_rep.get("reachability", {})
            downstream = reachability.get(fault_node, [])
            for d in downstream:
                if d in self.node_to_idx:
                    idx = self.node_to_idx[d]
                    propagation_mask[idx] = 1.0
                    node_failures[idx] = 1.0
            time_to_failure = observation_window_sec
        else:
            root_cause_label = self.num_nodes  # Nominal class
            time_to_failure = 600.0

        return {
            "node_features": np.stack(node_features_seq, axis=0),  # (T, N, feature_dim)
            "adjacency": np.stack(adj_seq, axis=0),                # (T, N, N)
            "failure_label": np.float32(failure_label),
            "node_failures": node_failures,
            "root_cause": np.int64(root_cause_label),
            "propagation_mask": propagation_mask,
            "time_to_failure": np.float32(time_to_failure),
            "fault_node": fault_node or "none",
            "fault_type": fault_type or "nominal",
            "recorded_at": time.time(),
        }

    @staticmethod
    def _prometheus_query(prometheus_url: str, query: str) -> List[Dict[str, Any]]:
        encoded = urllib.parse.urlencode({"query": query})
        with urllib.request.urlopen(
            f"{prometheus_url.rstrip('/')}/api/v1/query?{encoded}",
            timeout=10,
        ) as response:
            payload = json.loads(response.read().decode("utf-8"))
        if payload.get("status") != "success":
            raise RuntimeError(f"Prometheus query failed: {payload}")
        return payload.get("data", {}).get("result", [])

    def record_prometheus_episode(
        self,
        prometheus_url: str,
        observation_window_sec: float = 60.0,
        poll_interval_sec: float = 5.0,
        fault_node: Optional[str] = None,
        fault_type: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Capture only observed Prometheus samples before an externally injected fault."""
        queries = {
            "http_requests_in_flight": "http_requests_in_flight",
            "db_pool_connections_wait_count": "db_pool_connections_wait_count",
            "process_resident_memory_bytes": "process_resident_memory_bytes",
            "go_goroutines": "go_goroutines",
            "worker_pool_utilization_ratio": "worker_pool_utilization_ratio",
            "consumer_lag": "consumer_lag",
            "queue_depth": "queue_depth",
        }
        samples: List[Dict[str, Any]] = []
        deadline = time.time() + observation_window_sec
        while time.time() < deadline:
            timestamp_ns = int(time.time() * 1e9)
            for metric_name, query in queries.items():
                for result in self._prometheus_query(prometheus_url, query):
                    labels = result.get("metric", {})
                    service_id = labels.get("service") or labels.get("job")
                    value = result.get("value")
                    if not service_id or not value:
                        continue
                    samples.append(
                        {
                            "timestamp_unix_nano": timestamp_ns,
                            "service_id": service_id,
                            "metric_name": metric_name,
                            "value": float(value[1]),
                        }
                    )
            time.sleep(poll_interval_sec)

        if not samples:
            raise RuntimeError("Prometheus returned no usable service metrics during the capture")

        return {
            "metrics": samples,
            "fault_node": fault_node or "none",
            "fault_type": fault_type or "nominal",
            "observation_window_sec": observation_window_sec,
            "pre_injection_only": True,
            "captured_at": time.time(),
        }

    def collect_validation_dataset(
        self,
        episodes_per_fault: int = 2,
        output_filename: str = "live_validation_tensors.pt",
    ) -> str:
        """
        Gathers a targeted sanity-check dataset across all fault types
        to validate that models pre-trained on synthetic simulations generalize to live telemetry.
        """
        fault_types = [
            "latency",
            "cpu_stress",
            "kill_service",
            "memory_leak",
            "packet_loss",
            "thread_exhaustion",
            "db_lock",
            "slow_query",
            "mq_lag",
            "cache_down",
        ]

        all_node_features = []
        all_adj = []
        all_failure_labels = []
        all_node_failures = []
        all_root_causes = []
        all_prop_masks = []
        all_ttf = []
        metadata = []

        # 1. Nominal episodes
        for i in range(episodes_per_fault):
            ep = self.record_episode(fault_node=None, fault_type=None)
            all_node_features.append(ep["node_features"])
            all_adj.append(ep["adjacency"])
            all_failure_labels.append(ep["failure_label"])
            all_node_failures.append(ep["node_failures"])
            all_root_causes.append(ep["root_cause"])
            all_prop_masks.append(ep["propagation_mask"])
            all_ttf.append(ep["time_to_failure"])
            metadata.append({"id": f"live-nom-{i}", "type": "nominal", "node": "none"})

        # 2. Chaos episodes across target nodes and faults
        for ftype in fault_types:
            for i in range(episodes_per_fault):
                target_node = self.node_ids[i % len(self.node_ids)]
                ep = self.record_episode(fault_node=target_node, fault_type=ftype)
                all_node_features.append(ep["node_features"])
                all_adj.append(ep["adjacency"])
                all_failure_labels.append(ep["failure_label"])
                all_node_failures.append(ep["node_failures"])
                all_root_causes.append(ep["root_cause"])
                all_prop_masks.append(ep["propagation_mask"])
                all_ttf.append(ep["time_to_failure"])
                metadata.append({"id": f"live-{ftype}-{i}", "type": ftype, "node": target_node})

        out_path = os.path.join(self.output_dir, output_filename)
        torch.save(
            {
                "node_features": torch.from_numpy(np.stack(all_node_features, axis=0)),
                "adjacency_matrices": torch.from_numpy(np.stack(all_adj, axis=0)),
                "failure_labels": torch.from_numpy(np.array(all_failure_labels, dtype=np.float32)),
                "node_failures": torch.from_numpy(np.stack(all_node_failures, axis=0)),
                "root_causes": torch.from_numpy(np.array(all_root_causes, dtype=np.int64)),
                "propagation_masks": torch.from_numpy(np.stack(all_prop_masks, axis=0)),
                "time_to_failure": torch.from_numpy(np.array(all_ttf, dtype=np.float32)),
                "node_ids": self.node_ids,
                "metadata": metadata,
            },
            out_path,
        )
        logger.info(f"Saved {len(all_failure_labels)} live validation episodes to {out_path}")
        return out_path


def main() -> None:
    parser = argparse.ArgumentParser(description="Capture real telemetry without synthetic fallback")
    parser.add_argument("--source", choices=("prometheus", "kafka"), default="prometheus")
    parser.add_argument("--prometheus-url", default="http://localhost:9091")
    parser.add_argument("--kafka-bootstrap", default="localhost:9092")
    parser.add_argument("--topology", default="configs/topology.yaml")
    parser.add_argument("--output", required=True)
    parser.add_argument("--duration-sec", type=float, default=60.0)
    parser.add_argument("--interval-sec", type=float, default=5.0)
    parser.add_argument("--fault-node")
    parser.add_argument("--fault-type")
    args = parser.parse_args()

    logging.basicConfig(level=logging.INFO)
    collector = LiveKafkaEpisodeCollector(
        bootstrap_servers=args.kafka_bootstrap,
        topology_path=args.topology,
    )
    if args.source == "prometheus":
        episode = collector.record_prometheus_episode(
            prometheus_url=args.prometheus_url,
            observation_window_sec=args.duration_sec,
            poll_interval_sec=args.interval_sec,
            fault_node=args.fault_node,
            fault_type=args.fault_type,
        )
    else:
        episode = collector.record_episode(
            fault_node=args.fault_node,
            fault_type=args.fault_type,
            observation_window_sec=args.duration_sec,
        )

    output_path = os.path.abspath(args.output)
    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as output_file:
        json.dump(episode, output_file, indent=2)
    logger.info("Saved observed telemetry episode to %s", output_path)


if __name__ == "__main__":
    main()
