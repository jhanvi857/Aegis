"""
Dataset Generator (Phase 3)
Converts simulated chaos episodes and live telemetry into labeled spatio-temporal
graph snapshots and feature tensors for TGNN and baseline models.
"""

import os
import sys
import json
import random
import logging
from typing import Dict, Any, List, Tuple, Optional
import numpy as np
import torch
from torch.utils.data import Dataset

# Ensure python-ml is in path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../python-ml")))

from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer

logger = logging.getLogger(__name__)


class ChaosEpisodeDataset(Dataset):
    """PyTorch Dataset wrapping processed spatio-temporal graph episodes."""

    def __init__(
        self,
        node_features: torch.Tensor,     # (num_episodes, T, num_nodes, feature_dim)
        adjacency_matrices: torch.Tensor, # (num_episodes, T, num_nodes, num_nodes)
        failure_labels: torch.Tensor,     # (num_episodes,) binary
        node_failures: torch.Tensor,      # (num_episodes, num_nodes) binary
        root_causes: torch.Tensor,        # (num_episodes,) long index (0..num_nodes, with num_nodes = nominal)
        propagation_masks: torch.Tensor,  # (num_episodes, num_nodes) binary
        time_to_failure: torch.Tensor,    # (num_episodes,) float seconds
        node_ids: List[str],
    ):
        self.node_features = node_features
        self.adjacency_matrices = adjacency_matrices
        self.failure_labels = failure_labels
        self.node_failures = node_failures
        self.root_causes = root_causes
        self.propagation_masks = propagation_masks
        self.time_to_failure = time_to_failure
        self.node_ids = node_ids
        self.num_nodes = len(node_ids)

    def __len__(self) -> int:
        return len(self.failure_labels)

    def __getitem__(self, idx: int) -> Dict[str, torch.Tensor]:
        return {
            "node_features": self.node_features[idx],
            "adjacency": self.adjacency_matrices[idx],
            "failure_label": self.failure_labels[idx],
            "node_failures": self.node_failures[idx],
            "root_cause": self.root_causes[idx],
            "propagation_mask": self.propagation_masks[idx],
            "time_to_failure": self.time_to_failure[idx],
        }


class DatasetGenerator:
    """
    Generates synthetic chaos episodes across diverse fault types,
    extracts sequential GraphRepresentations from Phase 2 graph service,
    and serializes labeled tensors into train/val/test splits.
    """

    FAULT_TYPES = [
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

    def __init__(
        self,
        topology_path: str = "configs/topology.yaml",
        raw_dir: str = "dataset/raw",
        processed_dir: str = "dataset/processed",
        splits_dir: str = "dataset/splits",
        sequence_length: int = 10,
    ):
        self.topology_path = topology_path
        self.raw_dir = raw_dir
        self.processed_dir = processed_dir
        self.splits_dir = splits_dir
        self.sequence_length = sequence_length

        self.service = GraphPreprocessingService(
            topology_path=topology_path,
            structural_ttl_sec=0.01,
        )
        self.consumer = TelemetryConsumer()
        initial_rep = self.service.get_latest_representation()
        self.node_ids = sorted(list(initial_rep["feature_matrix"].keys()))
        self.node_to_idx = {nid: i for i, nid in enumerate(self.node_ids)}
        self.num_nodes = len(self.node_ids)
        self.feature_dim = len(next(iter(initial_rep["feature_matrix"].values())))

    def generate_episode(
        self,
        fault_node: Optional[str] = None,
        fault_type: Optional[str] = None,
        horizon_lead_sec: float = 20.0,
    ) -> Dict[str, Any]:
        """
        Simulates an episode over a strict PRE-INJECTION observation window of T=10 timesteps.
        Catastrophic threshold breach occurs strictly in the FUTURE horizon (t > T_obs).
        
        - If fault_node is present:
          The target node exhibits subtle, stochastic precursor signals during the
          observation window (e.g. rising memory slope, queue buildup, latency jitter).
          Crucially, catastrophic failure has NOT occurred yet during t in [0, T_obs-1].
          Downstream nodes also experience faint early jitter reflecting dependency coupling.
          Labels test whether the model can anticipate the upcoming failure in (T_obs, T_obs + horizon].
        - If fault_node is None (nominal):
          All nodes experience nominal traffic with natural stochastic variance.
        """
        is_failure = fault_node is not None
        node_features_seq = []
        adj_seq = []

        # Reachability for propagation ground truth from latest representation
        latest_rep = self.service.get_latest_representation()
        reachability = latest_rep.get("reachability", {})
        downstream_nodes = reachability.get(fault_node, []) if is_failure else []

        for step in range(self.sequence_length):
            # Precursor severity increases gradually from ~0.10 at step 0 to ~0.60 at step 9
            # NEVER breaches catastrophic thresholds during the observation window!
            if is_failure:
                ramp = (step + 1) / float(self.sequence_length)
                precursor_sev = max(0.05, min(0.65, 0.10 + 0.50 * ramp + random.gauss(0, 0.04)))
            else:
                precursor_sev = 0.0

            batch = self.consumer.generate_simulated_batch(
                fault_node=fault_node,
                fault_type=fault_type,
                precursor_severity=precursor_sev,
            )

            # Downstream nodes experience subtle early jitter (precursor propagation)
            if is_failure and len(downstream_nodes) > 0:
                downstream_jitter = precursor_sev * 0.4
                for metric in batch.get("metrics", []):
                    if metric["service_id"] in downstream_nodes:
                        if metric["metric_name"] == "p95_latency_ms":
                            metric["value"] = metric["value"] + (35.0 * downstream_jitter)
                        elif metric["metric_name"] == "error_rate":
                            metric["value"] = min(0.03, metric["value"] + (0.015 * downstream_jitter))

            rep = self.service.process_telemetry_batch(batch)

            # Node feature matrix: (num_nodes, feature_dim)
            step_feats = np.zeros((self.num_nodes, self.feature_dim), dtype=np.float32)
            for nid, feat in rep["feature_matrix"].items():
                if nid in self.node_to_idx:
                    step_feats[self.node_to_idx[nid]] = np.array(feat, dtype=np.float32)
            node_features_seq.append(step_feats)

            # Dynamic Adjacency matrix: (num_nodes, num_nodes)
            step_adj = np.zeros((self.num_nodes, self.num_nodes), dtype=np.float32)
            for edge in rep["snapshot"]["edges"]:
                src, tgt = edge["source"], edge["target"]
                if src in self.node_to_idx and tgt in self.node_to_idx:
                    lat_norm = min(1.0, edge.get("latency_ms", 0.0) / 500.0)
                    err_norm = min(1.0, edge.get("error_rate", 0.0) / 0.5)
                    weight = 1.0 + lat_norm + err_norm
                    step_adj[self.node_to_idx[src], self.node_to_idx[tgt]] = weight
            adj_seq.append(step_adj)

        # Labels (Evaluating prediction of future horizon!)
        failure_label = 1.0 if is_failure else 0.0

        node_failures = np.zeros(self.num_nodes, dtype=np.float32)
        propagation_mask = np.zeros(self.num_nodes, dtype=np.float32)

        if is_failure:
            rc_idx = self.node_to_idx[fault_node]
            node_failures[rc_idx] = 1.0
            root_cause_label = rc_idx
            time_to_failure = horizon_lead_sec

            for d in downstream_nodes:
                if d in self.node_to_idx:
                    idx = self.node_to_idx[d]
                    propagation_mask[idx] = 1.0
                    node_failures[idx] = 1.0
        else:
            root_cause_label = self.num_nodes  # Nominal class index
            time_to_failure = 600.0

        return {
            "node_features": np.stack(node_features_seq, axis=0),  # (T, num_nodes, feature_dim)
            "adjacency": np.stack(adj_seq, axis=0),                # (T, num_nodes, num_nodes)
            "failure_label": np.float32(failure_label),
            "node_failures": node_failures,
            "root_cause": np.int64(root_cause_label),
            "propagation_mask": propagation_mask,
            "time_to_failure": np.float32(time_to_failure),
            "fault_node": fault_node or "none",
            "fault_type": fault_type or "nominal",
        }

    def generate_dataset(
        self,
        num_episodes: int = 600,
        train_ratio: float = 0.70,
        val_ratio: float = 0.15,
        seed: int = 42,
    ) -> Dict[str, Any]:
        """Generates full balanced dataset of nominal and chaos episodes."""
        random.seed(seed)
        np.random.seed(seed)
        torch.manual_seed(seed)

        os.makedirs(self.raw_dir, exist_ok=True)
        os.makedirs(self.processed_dir, exist_ok=True)
        os.makedirs(self.splits_dir, exist_ok=True)

        logger.info(f"Generating {num_episodes} chaos episodes for topology with nodes: {self.node_ids}...")

        all_node_features = []
        all_adj = []
        all_failure_labels = []
        all_node_failures = []
        all_root_causes = []
        all_prop_masks = []
        all_ttf = []
        metadata = []

        nominal_count = int(num_episodes * 0.3)
        chaos_count = num_episodes - nominal_count

        # 1. Nominal episodes
        for i in range(nominal_count):
            ep = self.generate_episode(fault_node=None, fault_type=None)
            all_node_features.append(ep["node_features"])
            all_adj.append(ep["adjacency"])
            all_failure_labels.append(ep["failure_label"])
            all_node_failures.append(ep["node_failures"])
            all_root_causes.append(ep["root_cause"])
            all_prop_masks.append(ep["propagation_mask"])
            all_ttf.append(ep["time_to_failure"])
            metadata.append({"id": f"ep-nom-{i}", "type": "nominal", "fault_node": "none"})

        # 2. Chaos episodes across all fault types and nodes (strict pre-injection)
        for i in range(chaos_count):
            target_node = random.choice(self.node_ids)
            fault_type = random.choice(self.FAULT_TYPES)
            lead_sec = random.choice([15.0, 20.0, 25.0, 30.0])

            ep = self.generate_episode(
                fault_node=target_node,
                fault_type=fault_type,
                horizon_lead_sec=lead_sec,
            )
            all_node_features.append(ep["node_features"])
            all_adj.append(ep["adjacency"])
            all_failure_labels.append(ep["failure_label"])
            all_node_failures.append(ep["node_failures"])
            all_root_causes.append(ep["root_cause"])
            all_prop_masks.append(ep["propagation_mask"])
            all_ttf.append(ep["time_to_failure"])
            metadata.append({
                "id": f"ep-chaos-{i}",
                "type": fault_type,
                "fault_node": target_node,
                "horizon_lead_sec": lead_sec,
            })

        # Convert to PyTorch tensors
        tensor_data = {
            "node_features": torch.from_numpy(np.stack(all_node_features, axis=0)),
            "adjacency_matrices": torch.from_numpy(np.stack(all_adj, axis=0)),
            "failure_labels": torch.from_numpy(np.array(all_failure_labels, dtype=np.float32)),
            "node_failures": torch.from_numpy(np.stack(all_node_failures, axis=0)),
            "root_causes": torch.from_numpy(np.array(all_root_causes, dtype=np.int64)),
            "propagation_masks": torch.from_numpy(np.stack(all_prop_masks, axis=0)),
            "time_to_failure": torch.from_numpy(np.array(all_ttf, dtype=np.float32)),
            "node_ids": self.node_ids,
        }

        # Shuffle indices
        indices = list(range(num_episodes))
        random.shuffle(indices)

        n_train = int(num_episodes * train_ratio)
        n_val = int(num_episodes * val_ratio)
        train_indices = indices[:n_train]
        val_indices = indices[n_train : n_train + n_val]
        test_indices = indices[n_train + n_val :]

        # Save splits
        with open(os.path.join(self.splits_dir, "train_idx.json"), "w") as f:
            json.dump(train_indices, f)
        with open(os.path.join(self.splits_dir, "val_idx.json"), "w") as f:
            json.dump(val_indices, f)
        with open(os.path.join(self.splits_dir, "test_idx.json"), "w") as f:
            json.dump(test_indices, f)

        # Save processed dataset
        processed_path = os.path.join(self.processed_dir, "tensors.pt")
        torch.save(tensor_data, processed_path)

        # Save raw episode metadata sample
        with open(os.path.join(self.raw_dir, "sample_episodes.json"), "w") as f:
            json.dump(metadata[:50], f, indent=2)

        logger.info(
            f"Dataset generated successfully: {num_episodes} episodes "
            f"(Train: {len(train_indices)}, Val: {len(val_indices)}, Test: {len(test_indices)}) "
            f"saved to {processed_path}"
        )

        return {
            "path": processed_path,
            "num_episodes": num_episodes,
            "train_count": len(train_indices),
            "val_count": len(val_indices),
            "test_count": len(test_indices),
            "node_ids": self.node_ids,
        }

    def load_split_dataset(self, split: str = "train") -> ChaosEpisodeDataset:
        """Loads a ChaosEpisodeDataset instance for a specific split ('train', 'val', 'test')."""
        processed_path = os.path.join(self.processed_dir, "tensors.pt")
        split_path = os.path.join(self.splits_dir, f"{split}_idx.json")

        if not os.path.exists(processed_path) or not os.path.exists(split_path):
            self.generate_dataset(num_episodes=600)

        data = torch.load(processed_path, weights_only=False)
        with open(split_path, "r") as f:
            indices = json.load(f)

        return ChaosEpisodeDataset(
            node_features=data["node_features"][indices],
            adjacency_matrices=data["adjacency_matrices"][indices],
            failure_labels=data["failure_labels"][indices],
            node_failures=data["node_failures"][indices],
            root_causes=data["root_causes"][indices],
            propagation_masks=data["propagation_masks"][indices],
            time_to_failure=data["time_to_failure"][indices],
            node_ids=data["node_ids"],
        )


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    gen = DatasetGenerator()
    gen.generate_dataset(num_episodes=600)
