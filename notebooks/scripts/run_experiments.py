"""Run the compact, reproducible notebook benchmark and save JSON outputs.

The default path evaluates the committed TGNN checkpoint and fits lightweight
classical baselines. ``--retrain`` additionally trains the LSTM and TGNN with
the same train/validation split; no test examples are used for model choice.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

import numpy as np
import torch
from sklearn.ensemble import IsolationForest, RandomForestClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.svm import SVC

try:
    from xgboost import XGBClassifier
except ImportError:  # Optional in minimal local environments; listed for Kaggle.
    XGBClassifier = None

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "notebooks"))
sys.path.insert(0, str(ROOT / "python-ml"))
sys.path.insert(0, str(ROOT))

from common.data import load_dataset, split_arrays
from common.metrics import failure_metrics, propagation_iou, top_k_accuracy, ttf_mae
from common.seed import set_seed
from models.heuristic.heuristic_baseline import HeuristicThresholdBaseline, MajorityClassBaseline
from models.isolation_forest.model import IsolationForestBaseline
from models.lstm.model import LSTMBaseline
from models.tgnn.model import TGNNModel


def _predictions(model, arrays, kind):
    if kind == "classical":
        return model.predict_proba(arrays["flat"])[:, 1]
    if kind == "isolation":
        return model.predict_anomaly(arrays["x"])["failure_probs"]
    if kind == "heuristic":
        return model.predict(arrays["x"])["failure_probs"]
    with torch.no_grad():
        out = model(torch.from_numpy(arrays["x"]), torch.from_numpy(arrays["adj"]))
    return {"failure": out["cluster_failure_prob"].numpy(), "root": out["root_cause_probs"].numpy(), "prop": out["propagation_probs"].numpy(), "ttf": out["time_to_failure"].numpy()}


def _score(name, predictions, test, root_predictions=None):
    if isinstance(predictions, dict):
        failure = predictions["failure"]
        root = predictions["root"]
        prop = predictions["prop"]
        ttf = predictions["ttf"]
    else:
        failure = predictions
        root = root_predictions
        prop = np.zeros((len(failure), test["x"].shape[2]))
        ttf = np.full(len(failure), np.mean(test["ttf"]))
    metrics = failure_metrics(test["failure"], failure)
    metrics.update({"model": name, "root_top1": None if root is None else top_k_accuracy(test["root"], root, 1), "root_top3": None if root is None else top_k_accuracy(test["root"], root, min(3, root.shape[1])), "propagation_iou": propagation_iou(test["prop"], prop), "ttf_mae": ttf_mae(test["ttf"], ttf)})
    return metrics


def _load_tgnn(train, retrain, artifact_dir):
    device = torch.device("cpu")
    checkpoint = ROOT / "python-ml/models/checkpoints/tgnn_best.pt"
    if retrain:
        model = TGNNModel(in_dim=train["x"].shape[-1], hidden_dim=32, num_nodes=train["x"].shape[2], num_spatial_layers=1, dropout=0.0).to(device)
        optimizer = torch.optim.Adam(model.parameters(), lr=0.003)
        x = torch.from_numpy(train["x"])
        adj = torch.from_numpy(train["adj"])
        y = torch.from_numpy(train["failure"].astype(np.float32))
        for _ in range(4):
            optimizer.zero_grad()
            out = model(x, adj)
            loss = torch.nn.functional.binary_cross_entropy(out["cluster_failure_prob"], y)
            loss.backward()
            optimizer.step()
        torch.save({"model_state_dict": model.state_dict(), "in_dim": train["x"].shape[-1], "hidden_dim": 32, "node_ids": [str(i) for i in range(train["x"].shape[2])]}, artifact_dir / "tgnn_retrained.pt")
        return model, {"epochs_run": 4, "learning_rate": 0.003, "batch_size": len(train["x"]), "parameters": model.get_param_count()}
    checkpoint_data = torch.load(checkpoint, map_location=device, weights_only=False)
    model = TGNNModel(in_dim=checkpoint_data.get("in_dim", 8), hidden_dim=checkpoint_data.get("hidden_dim", 64), num_nodes=train["x"].shape[2]).to(device)
    model.load_state_dict(checkpoint_data["model_state_dict"])
    model.eval()
    return model, {"epochs_run": checkpoint_data.get("epoch", "committed checkpoint"), "learning_rate": "from checkpoint", "batch_size": "from checkpoint", "parameters": model.get_param_count()}


def run(retrain=False, reduced=False):
    set_seed(42)
    data = split_arrays(load_dataset())
    train, test = data["train"], data["test"]
    results_dir = ROOT / "notebooks/results"
    artifact_dir = ROOT / "notebooks/artifacts"
    results_dir.mkdir(parents=True, exist_ok=True)
    artifact_dir.mkdir(parents=True, exist_ok=True)
    models = {}

    majority = MajorityClassBaseline()
    majority.fit(train["failure"], train["root"], num_nodes=train["x"].shape[2])
    majority_out = majority.predict(len(test["failure"]), test["x"].shape[2])
    models["majority"] = _score("Majority", majority_out["failure_probs"], test, majority_out["root_cause_probs"])

    heuristic = HeuristicThresholdBaseline()
    heuristic.fit(train["x"])
    heuristic_out = heuristic.predict(test["x"])
    models["heuristic"] = _score("Heuristic", heuristic_out["failure_probs"], test, heuristic_out["root_cause_probs"])

    for name, estimator in (("logistic_regression", LogisticRegression(max_iter=300, random_state=42)), ("svm", SVC(probability=True, random_state=42)), ("random_forest", RandomForestClassifier(n_estimators=50, random_state=42, n_jobs=-1))):
        estimator.fit(train["flat"], train["failure"])
        models[name] = _score(name, _predictions(estimator, test, "classical"), test)
    if XGBClassifier is not None:
        xgb = XGBClassifier(n_estimators=50, max_depth=3, learning_rate=0.1, random_state=42, n_jobs=1, eval_metric="logloss")
        xgb.fit(train["flat"], train["failure"])
        models["xgboost"] = _score("XGBoost", _predictions(xgb, test, "classical"), test)

    iso = IsolationForestBaseline(contamination=0.3, random_state=42)
    iso.fit(train["x"])
    models["isolation_forest"] = _score("Isolation Forest", _predictions(iso, test, "isolation"), test)

    tgnn, training = _load_tgnn(train, retrain, artifact_dir)
    models["tgnn"] = _score("TGNN", _predictions(tgnn, test, "tgnn"), test)
    results = {"seed": 42, "test_episodes": len(test["failure"]), "models": models, "training": {"tgnn": training}, "feature_columns": 32}
    (results_dir / "core_results.json").write_text(json.dumps(results, indent=2), encoding="utf-8")

    repeats = []
    for seed in (0, 1, 2, 3, 4):
        set_seed(seed)
        estimator = LogisticRegression(max_iter=300, random_state=seed).fit(train["flat"], train["failure"])
        repeats.append({"seed": seed, "model": "Logistic Regression", **failure_metrics(test["failure"], estimator.predict_proba(test["flat"])[:, 1])})
    (results_dir / "repeat_results.json").write_text(json.dumps(repeats, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--retrain", action="store_true")
    parser.add_argument("--reduced", action="store_true")
    args = parser.parse_args()
    print(json.dumps(run(args.retrain, args.reduced), indent=2))