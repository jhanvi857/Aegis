"""
Dataset Audit Suite (Phase 3 Forensic Verification)
1. Audits Root-Cause Node Distribution: Chi-Square goodness-of-fit test against uniform
   sampling to formally prove zero position/index bias.
2. Train/Val/Test Distribution Shift Check: Two-sample Kolmogorov-Smirnov (KS) test and
   Wasserstein distance across all 8 features to verify covariate stability.
"""

import os
import sys
import json
import logging
from typing import Dict, Any, List
import numpy as np
import scipy.stats as stats
import torch

# Add paths
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from dataset.generator import DatasetGenerator

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("audit_dataset")


def audit_root_cause_distribution(tensors_path: str = "dataset/processed/tensors.pt") -> Dict[str, Any]:
    """
    Checks whether root causes are evenly distributed across nodes or biased toward
    specific topological positions (e.g. gateway vs leaf).
    """
    data = torch.load(tensors_path, weights_only=False)
    root_causes = data["root_causes"].numpy()
    node_ids = data["node_ids"]
    num_nodes = len(node_ids)

    # Filter out nominal episodes (index == num_nodes)
    fault_rcs = root_causes[root_causes < num_nodes]
    total_fault_episodes = len(fault_rcs)

    counts = {node_ids[i]: int(np.sum(fault_rcs == i)) for i in range(num_nodes)}
    percentages = {node_ids[i]: float(counts[node_ids[i]] / total_fault_episodes * 100.0) for i in range(num_nodes)}

    # Chi-Square Goodness-of-Fit Test against Uniform Distribution
    observed = [counts[nid] for nid in node_ids]
    expected = [total_fault_episodes / float(num_nodes)] * num_nodes
    chi2_stat, p_value = stats.chisquare(observed, f_exp=expected)

    # Entropy of distribution
    probs = [c / total_fault_episodes for c in observed]
    empirical_entropy = float(stats.entropy(probs, base=2))
    max_entropy = float(np.log2(num_nodes))

    audit_result = {
        "num_nodes": num_nodes,
        "total_chaos_episodes": total_fault_episodes,
        "counts_per_node": counts,
        "percentages_per_node": percentages,
        "expected_count_per_node": float(total_fault_episodes / num_nodes),
        "chi2_statistic": float(chi2_stat),
        "p_value": float(p_value),
        "empirical_entropy_bits": empirical_entropy,
        "max_entropy_bits": max_entropy,
        "entropy_ratio": float(empirical_entropy / max_entropy),
        "is_uniform_and_unbiased": bool(p_value > 0.05),
    }

    logger.info("=== Root-Cause Node Distribution Audit ===")
    logger.info(f"{'Node ID':<15} | {'Count':<8} | {'Share (%)':<10}")
    logger.info("-" * 40)
    for nid in node_ids:
        logger.info(f"{nid:<15} | {counts[nid]:<8} | {percentages[nid]:<10.1f}%")
    logger.info(f"Chi-Square: {chi2_stat:.4f}, p-value: {p_value:.4f} (Uniform if p > 0.05: {p_value > 0.05})")
    logger.info(f"Entropy: {empirical_entropy:.3f} / {max_entropy:.3f} bits ({empirical_entropy/max_entropy*100:.1f}%)")

    return audit_result


def audit_train_val_test_distribution(
    tensors_path: str = "dataset/processed/tensors.pt",
    splits_dir: str = "dataset/splits",
) -> Dict[str, Any]:
    """
    Computes two-sample KS test and Wasserstein distance for each of the 8 features
    across Train vs Val and Train vs Test splits.
    """
    data = torch.load(tensors_path, weights_only=False)
    features = data["node_features"].numpy()  # (N_episodes, T, num_nodes, feature_dim)
    # Average over time and nodes to get episode-level feature distribution
    ep_feats = np.mean(features, axis=(1, 2))  # (N_episodes, feature_dim)

    with open(os.path.join(splits_dir, "train_idx.json"), "r") as f:
        train_idx = json.load(f)
    with open(os.path.join(splits_dir, "val_idx.json"), "r") as f:
        val_idx = json.load(f)
    with open(os.path.join(splits_dir, "test_idx.json"), "r") as f:
        test_idx = json.load(f)

    train_f = ep_feats[train_idx]
    val_f = ep_feats[val_idx]
    test_f = ep_feats[test_idx]

    feature_names = [
        "cpu_utilization",
        "memory_utilization",
        "p95_latency",
        "error_rate",
        "in_degree",
        "out_degree",
        "criticality",
        "queue_depth",
    ]

    feature_audits = {}
    all_train_test_p_values = []

    for dim, name in enumerate(feature_names):
        tr_vals = train_f[:, dim]
        val_vals = val_f[:, dim]
        te_vals = test_f[:, dim]

        # Train vs Val
        ks_val_stat, ks_val_p = stats.ks_2samp(tr_vals, val_vals)
        wass_val = stats.wasserstein_distance(tr_vals, val_vals)

        # Train vs Test
        ks_te_stat, ks_te_p = stats.ks_2samp(tr_vals, te_vals)
        wass_te = stats.wasserstein_distance(tr_vals, te_vals)

        all_train_test_p_values.append(ks_te_p)

        feature_audits[name] = {
            "train_vs_val": {
                "ks_stat": float(ks_val_stat),
                "p_value": float(ks_val_p),
                "wasserstein": float(wass_val),
            },
            "train_vs_test": {
                "ks_stat": float(ks_te_stat),
                "p_value": float(ks_te_p),
                "wasserstein": float(wass_te),
            },
            "no_significant_shift": bool(ks_te_p > 0.05),
        }

    overall_result = {
        "train_size": len(train_idx),
        "val_size": len(val_idx),
        "test_size": len(test_idx),
        "features": feature_audits,
        "min_train_test_p_value": float(min(all_train_test_p_values)),
        "distribution_shift_detected": bool(any(p < 0.01 for p in all_train_test_p_values)),
    }

    logger.info("=== Train/Val/Test Distribution Check (Two-Sample KS Test) ===")
    logger.info(f"{'Feature':<20} | {'Train-Test KS':<14} | {'p-value':<10} | {'Wasserstein':<12} | {'Shift?':<8}")
    logger.info("-" * 75)
    for name in feature_names:
        te_res = feature_audits[name]["train_vs_test"]
        shift_str = "NO" if te_res["p_value"] > 0.05 else "MARGINAL"
        logger.info(
            f"{name:<20} | {te_res['ks_stat']:<14.4f} | {te_res['p_value']:<10.4f} | "
            f"{te_res['wasserstein']:<12.4f} | {shift_str:<8}"
        )

    return overall_result


def run_full_audit(output_path: str = "dataset/processed/audit_statistics.json") -> Dict[str, Any]:
    """Runs root cause and feature split audits and saves consolidated results."""
    rc_audit = audit_root_cause_distribution()
    split_audit = audit_train_val_test_distribution()

    consolidated = {
        "root_cause_distribution_audit": rc_audit,
        "train_val_test_distribution_audit": split_audit,
    }

    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    with open(output_path, "w") as f:
        json.dump(consolidated, f, indent=2)

    logger.info(f"Audit statistics saved to {output_path}")
    return consolidated


if __name__ == "__main__":
    run_full_audit()
