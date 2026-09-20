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

    # Loss criteria & Unified Loss Weights (Identical across train & validation)
    LOSS_WEIGHTS = {
        "cluster": 1.5,
        "node": 1.0,
        "rc": 2.0,
        "prop": 2.0,
        "ttf": 0.5,
    }

    # Class-imbalance compensation: 1-2 positive nodes per 5-node graph -> pos_weight ~ 4.0
    POS_WEIGHT = 4.0

    def weighted_bce_loss(pred: torch.Tensor, target: torch.Tensor, pos_weight: float = POS_WEIGHT, eps: float = 1e-7) -> torch.Tensor:
        """Binary cross entropy with positive class weighting to counteract microservice failure sparsity."""
        pred = torch.clamp(pred, min=eps, max=1.0 - eps)
        loss = -(pos_weight * target * torch.log(pred) + (1.0 - target) * torch.log(1.0 - pred))
        return loss.mean()

    bce_loss = nn.BCELoss()
    ce_loss = nn.CrossEntropyLoss()
    huber_loss = nn.SmoothL1Loss()

    # Mixed Precision (AMP) setup
    use_amp = device.type == "cuda"
    scaler = torch.amp.GradScaler("cuda", enabled=use_amp)

    best_val_loss = float("inf")
    best_checkpoint_path = os.path.join(checkpoint_dir, "tgnn_best.pt")
    patience = 10
    patience_counter = 0

    history = {
        "train_loss": [],
        "val_loss": [],
        "val_rc_acc": [],
        "val_fail_acc": [],
    }

    logger.info(
        f"Starting TGNN training for {epochs} epochs | "
        f"Train: {len(train_dataset)}, Val: {len(val_dataset)} | "
        f"Model Parameters: {model.get_param_count():,} | "
        f"AMP Enabled: {use_amp} | Patience: {patience}"
    )

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

            with torch.amp.autocast("cuda", enabled=use_amp):
                out = model(x, adj)

                # Multi-task loss formulation:
                loss_cluster_fail = bce_loss(out["cluster_failure_prob"], y_fail)
                loss_node_fail = weighted_bce_loss(out["node_failure_probs"], y_node_fail)
                loss_rc = ce_loss(out["root_cause_logits"], y_rc)
                loss_prop = weighted_bce_loss(out["propagation_probs"], y_prop)
                loss_ttf = huber_loss(out["time_to_failure"] / 600.0, y_ttf / 600.0)

                loss = (
                    LOSS_WEIGHTS["cluster"] * loss_cluster_fail
                    + LOSS_WEIGHTS["node"] * loss_node_fail
                    + LOSS_WEIGHTS["rc"] * loss_rc
                    + LOSS_WEIGHTS["prop"] * loss_prop
                    + LOSS_WEIGHTS["ttf"] * loss_ttf
                )

            if use_amp:
                scaler.scale(loss).backward()
                scaler.unscale_(optimizer)
                torch.nn.utils.clip_grad_norm_(model.parameters(), max_norm=1.0)
                scaler.step(optimizer)
                scaler.update()
            else:
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

                with torch.amp.autocast("cuda", enabled=use_amp):
                    out = model(x, adj)

                    l_cluster = bce_loss(out["cluster_failure_prob"], y_fail)
                    l_node = weighted_bce_loss(out["node_failure_probs"], y_node_fail)
                    l_rc = ce_loss(out["root_cause_logits"], y_rc)
                    l_prop = weighted_bce_loss(out["propagation_probs"], y_prop)
                    l_ttf = huber_loss(out["time_to_failure"] / 600.0, y_ttf / 600.0)

                    # Unified validation loss objective matching training weights exactly
                    v_loss = (
                        LOSS_WEIGHTS["cluster"] * l_cluster
                        + LOSS_WEIGHTS["node"] * l_node
                        + LOSS_WEIGHTS["rc"] * l_rc
                        + LOSS_WEIGHTS["prop"] * l_prop
                        + LOSS_WEIGHTS["ttf"] * l_ttf
                    )

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

        if val_loss < best_val_loss - 1e-4:
            best_val_loss = val_loss
            patience_counter = 0
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
                    "param_count": model.get_param_count(),
                },
                best_checkpoint_path,
            )
        else:
            patience_counter += 1

        if epoch % 5 == 0 or epoch == epochs:
            logger.info(
                f"Epoch {epoch:02d}/{epochs:02d} | "
                f"Train Loss: {train_loss:.4f} | "
                f"Val Loss: {val_loss:.4f} | "
                f"Val Fail Acc: {val_fail_acc * 100:.1f}% | "
                f"Val RC Acc: {val_rc_acc * 100:.1f}%"
            )

        if patience_counter >= patience:
            logger.info(f"Early stopping triggered at epoch {epoch} (no validation improvement for {patience} epochs).")
            break

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

