"""
TGNN Training Pipeline (Phase 3)
Trains the multi-task Spatio-Temporal Graph Neural Network on processed chaos episodes.
Optimizes failure prediction, root cause classification, and propagation forecasting.
Saves model checkpoints to python-ml/models/checkpoints/tgnn_best.pt.
"""

import os
import sys
import json
import logging
from typing import Dict, Any, Tuple
import torch
import torch.nn as nn
from torch.utils.data import DataLoader

# Ensure python-ml in sys.path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from models.tgnn.model import TGNNModel
from dataset.generator import DatasetGenerator, ChaosEpisodeDataset

logger = logging.getLogger(__name__)


def train(
    epochs: int = 25,
    batch_size: int = 32,
    lr: float = 0.003,
    checkpoint_dir: str = "python-ml/models/checkpoints",
) -> Dict[str, Any]:
    """Trains the TGNN model and saves the best performing weights."""
    os.makedirs(checkpoint_dir, exist_ok=True)
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device} for TGNN training")

    # Load dataset splits
    gen = DatasetGenerator()
    train_dataset = gen.load_split_dataset(split="train")
    val_dataset = gen.load_split_dataset(split="val")

    train_loader = DataLoader(train_dataset, batch_size=batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=batch_size, shuffle=False)

    num_nodes = train_dataset.num_nodes
    in_dim = train_dataset.node_features.shape[-1]

    model = TGNNModel(
        in_dim=in_dim,
        hidden_dim=64,
        num_nodes=num_nodes,
        num_spatial_layers=2,
        dropout=0.1,
    ).to(device)

    optimizer = torch.optim.AdamW(model.parameters(), lr=lr, weight_decay=1e-4)
    scheduler = torch.optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=epochs)

    # Loss criteria
    bce_loss = nn.BCELoss()
    ce_loss = nn.CrossEntropyLoss()
    huber_loss = nn.SmoothL1Loss()

    best_val_loss = float("inf")
    best_checkpoint_path = os.path.join(checkpoint_dir, "tgnn_best.pt")

    history = {
        "train_loss": [],
        "val_loss": [],
        "val_rc_acc": [],
        "val_fail_acc": [],
    }

    logger.info(f"Starting TGNN training for {epochs} epochs (Train: {len(train_dataset)}, Val: {len(val_dataset)})...")

    for epoch in range(1, epochs + 1):
        model.train()
        total_train_loss = 0.0

        for batch in train_loader:
            x = batch["node_features"].to(device)
            adj = batch["adjacency"].to(device)
            y_fail = batch["failure_label"].to(device)
            y_node_fail = batch["node_failures"].to(device)
            y_rc = batch["root_cause"].to(device)
            y_prop = batch["propagation_mask"].to(device)
            y_ttf = batch["time_to_failure"].to(device)

            optimizer.zero_grad()

            out = model(x, adj)

            # Multi-task loss formulation:
            # 1. Cluster failure risk
            loss_cluster_fail = bce_loss(out["cluster_failure_prob"], y_fail)
            # 2. Node failure probabilities
            loss_node_fail = bce_loss(out["node_failure_probs"], y_node_fail)
            # 3. Root cause classification
            loss_rc = ce_loss(out["root_cause_logits"], y_rc)
            # 4. Propagation mask prediction
            loss_prop = bce_loss(out["propagation_probs"], y_prop)
            # 5. Time to failure regression (normalized by 600s)
            loss_ttf = huber_loss(out["time_to_failure"] / 600.0, y_ttf / 600.0)

            loss = (
                1.5 * loss_cluster_fail
                + 1.0 * loss_node_fail
                + 2.0 * loss_rc
                + 2.0 * loss_prop
                + 0.5 * loss_ttf
            )

            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), max_norm=1.0)
            optimizer.step()

            total_train_loss += loss.item() * len(y_fail)

        scheduler.step()
        train_loss = total_train_loss / len(train_dataset)

        # Validation loop
        model.eval()
        total_val_loss = 0.0
        val_rc_correct = 0
        val_fail_correct = 0
        total_val = len(val_dataset)

        with torch.no_grad():
            for batch in val_loader:
                x = batch["node_features"].to(device)
                adj = batch["adjacency"].to(device)
                y_fail = batch["failure_label"].to(device)
                y_node_fail = batch["node_failures"].to(device)
                y_rc = batch["root_cause"].to(device)
                y_prop = batch["propagation_mask"].to(device)
                y_ttf = batch["time_to_failure"].to(device)

                out = model(x, adj)

                l_cluster = bce_loss(out["cluster_failure_prob"], y_fail)
                l_node = bce_loss(out["node_failure_probs"], y_node_fail)
                l_rc = ce_loss(out["root_cause_logits"], y_rc)
                l_prop = bce_loss(out["propagation_probs"], y_prop)
                l_ttf = huber_loss(out["time_to_failure"] / 600.0, y_ttf / 600.0)

                v_loss = 1.5 * l_cluster + 1.0 * l_node + 2.0 * l_rc + 1.0 * l_prop + 0.5 * l_ttf
                total_val_loss += v_loss.item() * len(y_fail)

                # Metrics
                preds_rc = out["root_cause_probs"].argmax(dim=-1)
                val_rc_correct += (preds_rc == y_rc).sum().item()

                pred_fail = (out["cluster_failure_prob"] >= 0.5).float()
                val_fail_correct += (pred_fail == y_fail).sum().item()

        val_loss = total_val_loss / total_val
        val_rc_acc = val_rc_correct / total_val
        val_fail_acc = val_fail_correct / total_val

        history["train_loss"].append(round(train_loss, 4))
        history["val_loss"].append(round(val_loss, 4))
        history["val_rc_acc"].append(round(val_rc_acc, 4))
        history["val_fail_acc"].append(round(val_fail_acc, 4))

        if val_loss < best_val_loss:
            best_val_loss = val_loss
            torch.save(
                {
                    "epoch": epoch,
                    "model_state_dict": model.state_dict(),
                    "optimizer_state_dict": optimizer.state_dict(),
                    "val_loss": val_loss,
                    "val_rc_acc": val_rc_acc,
                    "val_fail_acc": val_fail_acc,
                    "node_ids": train_dataset.node_ids,
                    "in_dim": in_dim,
                    "hidden_dim": 64,
                },
                best_checkpoint_path,
            )

        if epoch % 5 == 0 or epoch == epochs:
            logger.info(
                f"Epoch {epoch:02d}/{epochs:02d} | "
                f"Train Loss: {train_loss:.4f} | "
                f"Val Loss: {val_loss:.4f} | "
                f"Val Fail Acc: {val_fail_acc * 100:.1f}% | "
                f"Val RC Acc: {val_rc_acc * 100:.1f}%"
            )

    logger.info(f"TGNN training complete. Best model checkpoint saved to: {best_checkpoint_path}")
    return {
        "best_checkpoint": best_checkpoint_path,
        "best_val_loss": best_val_loss,
        "final_val_rc_acc": history["val_rc_acc"][-1],
        "final_val_fail_acc": history["val_fail_acc"][-1],
        "history": history,
    }


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    train(epochs=20)
