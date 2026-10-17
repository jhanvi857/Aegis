"""Export the tensor dataset into auditable episode and node CSV tables."""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import numpy as np
import pandas as pd
import torch


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

FEATURE_NAMES = [
    "cpu_utilization",
    "memory_utilization",
    "p95_latency",
    "error_rate",
    "in_degree",
    "out_degree",
    "criticality",
    "queue_depth",
]


def _load_metadata(raw_dir: Path, tensors: dict, splits_dir: Path) -> list[dict]:
    metadata_path = raw_dir / "episodes_metadata.json"
    if not metadata_path.exists():
        from dataset.generator import DatasetGenerator

        DatasetGenerator().generate_dataset(num_episodes=len(tensors["failure_labels"]))
    with metadata_path.open(encoding="utf-8") as handle:
        metadata = json.load(handle)
    split_lookup = {}
    for split in ("train", "val", "test"):
        with (splits_dir / f"{split}_idx.json").open(encoding="utf-8") as handle:
            split_lookup.update({int(i): split for i in json.load(handle)})
    rows = []
    for index, item in enumerate(metadata):
        row = dict(item)
        row["episode_id"] = row.pop("id")
        row["topology"] = "topology.yaml"
        row["fault_type"] = row.pop("type")
        row["failure"] = int(row["fault_type"] != "nominal")
        row["root_cause"] = row.pop("fault_node")
        row["time_to_failure"] = float(row.pop("horizon_lead_sec", 600.0))
        row["split"] = split_lookup[index]
        rows.append(row)
    return rows


def export_csv(
    tensors_path: str = "dataset/processed/tensors.pt",
    raw_dir: str = "dataset/raw",
    splits_dir: str = "dataset/splits",
    output_dir: str = "data",
) -> dict[str, str]:
    """Write node-level, edge-level, episode-level, and flat feature CSVs."""
    tensors = torch.load(tensors_path, map_location="cpu", weights_only=False)
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)
    metadata = _load_metadata(Path(raw_dir), tensors, Path(splits_dir))
    features = tensors["node_features"].numpy()
    nodes = tensors["node_ids"]
    split_by_episode = {row["episode_id"]: row["split"] for row in metadata}

    node_rows = []
    flat_rows = []
    for episode_index, episode_id in enumerate([row["episode_id"] for row in metadata]):
        episode = features[episode_index]
        means = episode.mean(axis=0)
        maxima = episode.max(axis=0)
        slopes = (episode[-1] - episode[0]) / max(1, episode.shape[0] - 1)
        for node_index, node_id in enumerate(nodes):
            row = {"episode_id": episode_id, "node_id": node_id}
            row.update({f"{name}_mean": float(means[node_index, j]) for j, name in enumerate(FEATURE_NAMES)})
            row.update({f"{name}_max": float(maxima[node_index, j]) for j, name in enumerate(FEATURE_NAMES)})
            row.update({f"{name}_slope": float(slopes[node_index, j]) for j, name in enumerate(FEATURE_NAMES)})
            flat_rows.append(row)
            for timestep in range(episode.shape[0]):
                time_row = {"episode_id": episode_id, "t": timestep, "node_id": node_id}
                time_row.update({name: float(episode[timestep, node_index, j]) for j, name in enumerate(FEATURE_NAMES)})
                time_row["is_root_cause"] = int(metadata[episode_index]["root_cause"] == node_id)
                time_row["in_blast_radius"] = int(time_row["is_root_cause"] or (metadata[episode_index]["root_cause"] != "none" and node_index in np.flatnonzero(tensors["propagation_masks"][episode_index].numpy())))
                node_rows.append(time_row)

    flat = pd.DataFrame(flat_rows)
    for name in FEATURE_NAMES:
        for statistic in ("mean", "max", "slope"):
            column = f"{name}_{statistic}"
            flat[f"neighbor_{column}"] = flat.groupby("episode_id")[column].transform("sum") - flat[column]
    pd.DataFrame(node_rows).to_csv(output / "node_timeseries.csv", index=False)
    pd.DataFrame([{**row, "episode_id": row["episode_id"]} for row in metadata]).to_csv(output / "episodes.csv", index=False)

    edge_rows = []
    topology_file = ROOT / "configs" / "topology.yaml"
    import yaml

    with topology_file.open(encoding="utf-8") as handle:
        topology = yaml.safe_load(handle)
    for node in topology.get("nodes", []):
        for destination in node.get("dependencies", []):
            edge_rows.append({"topology": topology_file.name, "src": node["id"], "dst": destination, "weight": 1.0})
    pd.DataFrame(edge_rows).to_csv(output / "edges.csv", index=False)
    flat.to_csv(output / "episodes_flat.csv", index=False)
    return {name: str(output / name) for name in ("node_timeseries.csv", "edges.csv", "episodes.csv", "episodes_flat.csv")}


if __name__ == "__main__":
    print(json.dumps(export_csv(), indent=2))