"""Data loading and feature construction shared by the notebook suite."""

from pathlib import Path
import json
import sys

import numpy as np
import torch

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
if str(ROOT / "python-ml") not in sys.path:
    sys.path.insert(0, str(ROOT / "python-ml"))

FEATURE_NAMES = ["cpu", "memory", "latency", "error", "in_degree", "out_degree", "criticality", "queue"]


def load_dataset(root: Path = ROOT) -> dict:
    tensors = torch.load(root / "dataset/processed/tensors.pt", map_location="cpu", weights_only=False)
    splits = {name: json.loads((root / f"dataset/splits/{name}_idx.json").read_text()) for name in ("train", "val", "test")}
    return {"tensors": tensors, "splits": splits}


def episode_flat_features(features: np.ndarray, adjacency: np.ndarray) -> np.ndarray:
    mean = features.mean(axis=1).reshape(features.shape[0], -1)
    maximum = features.max(axis=1).reshape(features.shape[0], -1)
    slope = (features[:, -1] - features[:, 0]).reshape(features.shape[0], -1)
    node_mean = features.mean(axis=1)
    neighbor = np.einsum("bij,bjk->bik", (adjacency.mean(axis=1) > 0).astype(np.float32), node_mean)
    return np.concatenate([mean, maximum, slope, neighbor.reshape(features.shape[0], -1)], axis=1)


def split_arrays(data: dict) -> dict:
    tensors, splits = data["tensors"], data["splits"]
    x = tensors["node_features"].numpy()
    adj = tensors["adjacency_matrices"].numpy()
    flat = episode_flat_features(x, adj)
    return {name: {"x": x[idx], "adj": adj[idx], "flat": flat[idx], "failure": tensors["failure_labels"].numpy()[idx].astype(int), "root": tensors["root_causes"].numpy()[idx], "prop": tensors["propagation_masks"].numpy()[idx].astype(int), "ttf": tensors["time_to_failure"].numpy()[idx]} for name, idx in splits.items()}