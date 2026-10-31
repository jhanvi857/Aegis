"""Data loading and feature construction shared by the notebook suite."""

from pathlib import Path
import json
import sys

import numpy as np

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
if str(ROOT / "python-ml") not in sys.path:
    sys.path.insert(0, str(ROOT / "python-ml"))

FEATURE_NAMES = ["cpu", "memory", "latency", "error", "in_degree", "out_degree", "criticality", "queue"]


def load_dataset(root: Path = ROOT) -> dict:
    splits = {}
    for name in ("train", "val", "test"):
        p = root / f"dataset/splits/{name}_idx.json"
        if p.exists():
            splits[name] = json.loads(p.read_text())
    return {"splits": splits}
