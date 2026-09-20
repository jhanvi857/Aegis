"""
Hard-Case Stress Testing Benchmark
Evaluates TGNN and baselines under boundary degradation scenarios from the Error Taxonomy:
1. Low-SNR Precursor Ambiguity (Heavy nominal noise overlapping with precursor signals)
2. Cyclic SCC Feedback Loops (Mutual retry backpressure in a cycle)
3. Multi-Point Concurrent Faults (Dual independent faults across tiers)
"""

import os
import sys
import random
import logging
from typing import Dict, Any, List, Tuple
import numpy as np
import torch
import torch.nn.functional as F

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from models.tgnn.model import TGNNModel
from models.lstm.model import LSTMBaseline
from graph.service import GraphPreprocessingService
from ingestion.kafka_consumer import TelemetryConsumer

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def run_hard_case_eval(num_episodes_per_case: int = 50, seed: int = 42) -> Dict[str, Any]:
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)

    device = torch.device("cpu")
    ckpt_path = "python-ml/models/checkpoints/tgnn_best.pt"
    if not os.path.exists(ckpt_path):
        raise FileNotFoundError(f"Missing {ckpt_path}")

    ckpt = torch.load(ckpt_path, map_location=device, weights_only=False)
    in_dim = ckpt.get("in_dim", 8)
    hidden_dim = ckpt.get("hidden_dim", 64)

    tgnn = TGNNModel(in_dim=in_dim, hidden_dim=hidden_dim, num_nodes=5).to(device)
    tgnn.load_state_dict(ckpt["model_state_dict"])
    tgnn.eval()

    lstm = LSTMBaseline(num_nodes=5, feature_dim=in_dim, hidden_dim=64).to(device)
    lstm.eval()

    service = GraphPreprocessingService(topology_path="configs/topology.yaml")
    consumer = TelemetryConsumer()
    node_ids = sorted(list(service.get_latest_representation()["feature_matrix"].keys()))
    node_to_idx = {nid: i for i, nid in enumerate(node_ids)}
    num_nodes = len(node_ids)

    results = {}

    # =========================================================================
    # Stress Case 1: Low-SNR / Heavy Noise (Precursor Ambiguity)
    # =========================================================================
    logger.info("Running Stress Case 1: Low-SNR Precursor Ambiguity (High Noise)...")
    c1_tgnn_rc_hits = 0
    c1_tgnn_rc_top3_hits = 0
    c1_tgnn_fail_hits = 0
    c1_lstm_rc_hits = 0
    c1_lstm_fail_hits = 0

    for _ in range(num_episodes_per_case):
        target_node = random.choice(node_ids)
        target_idx = node_to_idx[target_node]
        fault_type = random.choice(["cpu_stress", "latency", "memory_leak", "kill_service"])

        feats_seq = []
        adj_seq = []

        for step in range(10):
            # Weak precursor ramp: max 0.25 (barely above noise)
            ramp = (step + 1) / 10.0
            weak_s = 0.05 + 0.20 * ramp

            batch = consumer.generate_simulated_batch(
                fault_node=target_node,
                fault_type=fault_type,
                precursor_severity=weak_s,
            )

            # Inject high ambient variance across ALL nodes (mimicking production traffic surges)
            for m in batch.get("metrics", []):
                noise = random.gauss(0, 8.0)
                if m["metric_name"] == "cpu_usage":
                    m["value"] = max(10.0, min(80.0, m["value"] + noise))
                elif m["metric_name"] == "p95_latency_ms":
                    m["value"] = max(10.0, min(85.0, m["value"] + abs(noise) * 2))

            rep = service.process_telemetry_batch(batch)
            step_feats = np.zeros((num_nodes, in_dim), dtype=np.float32)
            for nid, feat in rep["feature_matrix"].items():
                if nid in node_to_idx:
                    step_feats[node_to_idx[nid]] = np.array(feat, dtype=np.float32)
            feats_seq.append(step_feats)

            step_adj = np.zeros((num_nodes, num_nodes), dtype=np.float32)
            for edge in rep["snapshot"]["edges"]:
                src, tgt = edge["source"], edge["target"]
                if src in node_to_idx and tgt in node_to_idx:
                    step_adj[node_to_idx[src], node_to_idx[tgt]] = 1.0
            adj_seq.append(step_adj)

        x_t = torch.from_numpy(np.stack(feats_seq, axis=0)).unsqueeze(0).to(device)
        adj_t = torch.from_numpy(np.stack(adj_seq, axis=0)).unsqueeze(0).to(device)

        with torch.no_grad():
            out = tgnn(x_t, adj_t)
            rc_probs = out["root_cause_probs"][0].cpu().numpy()
            fail_prob = out["cluster_failure_prob"][0].item()

            lstm_out = lstm(x_t)
            lstm_rc = np.argmax(lstm_out["root_cause_probs"][0].cpu().numpy())
            lstm_fail = lstm_out["cluster_failure_prob"][0].item()

        pred_rc = np.argmax(rc_probs)
        top3_rc = np.argsort(rc_probs)[-3:]

        if pred_rc == target_idx:
            c1_tgnn_rc_hits += 1
        if target_idx in top3_rc:
            c1_tgnn_rc_top3_hits += 1
        if fail_prob >= 0.5:
            c1_tgnn_fail_hits += 1

        if lstm_rc == target_idx:
            c1_lstm_rc_hits += 1
        if lstm_fail >= 0.5:
            c1_lstm_fail_hits += 1

    results["case1_low_snr"] = {
        "description": "Low-SNR Precursor Ambiguity (High Gaussian Noise)",
        "tgnn_rc_top1_acc": c1_tgnn_rc_hits / num_episodes_per_case,
        "tgnn_rc_top3_acc": c1_tgnn_rc_top3_hits / num_episodes_per_case,
        "tgnn_fail_recall": c1_tgnn_fail_hits / num_episodes_per_case,
        "lstm_rc_top1_acc": c1_lstm_rc_hits / num_episodes_per_case,
        "lstm_fail_recall": c1_lstm_fail_hits / num_episodes_per_case,
    }

    # =========================================================================
    # Stress Case 2: Cyclic SCC Feedback Loop (Mutual Degradation)
    # =========================================================================
    logger.info("Running Stress Case 2: Cyclic SCC Feedback Loop...")
    c2_tgnn_rc_hits = 0
    c2_tgnn_rc_top3_hits = 0
    c2_lstm_rc_hits = 0

    # In our topology: node-b and node-c interact closely.
    # Simulate mutual retry ping-pong where node-b is the initiator but node-c echoes backpressure
    for _ in range(num_episodes_per_case):
        target_node = "node-b"
        echo_node = "node-c"
        target_idx = node_to_idx[target_node]
        echo_idx = node_to_idx[echo_node]

        feats_seq = []
        adj_seq = []

        for step in range(10):
            ramp = (step + 1) / 10.0
            b_sev = 0.10 + 0.45 * ramp
            # Echo node also degrades heavily in retry loop (0.40 * ramp)
            c_sev = 0.08 + 0.40 * ramp

            batch = consumer.generate_simulated_batch(
                fault_node=target_node,
                fault_type="latency",
                precursor_severity=b_sev,
            )

            # Add cyclic feedback to echo_node
            for m in batch.get("metrics", []):
                if m["service_id"] == echo_node:
                    if m["metric_name"] == "p95_latency_ms":
                        m["value"] += 70.0 * c_sev
                    elif m["metric_name"] == "queue_depth":
                        m["value"] += int(10 * c_sev)

            rep = service.process_telemetry_batch(batch)
            step_feats = np.zeros((num_nodes, in_dim), dtype=np.float32)
            for nid, feat in rep["feature_matrix"].items():
                if nid in node_to_idx:
                    step_feats[node_to_idx[nid]] = np.array(feat, dtype=np.float32)
            feats_seq.append(step_feats)

            step_adj = np.zeros((num_nodes, num_nodes), dtype=np.float32)
            for edge in rep["snapshot"]["edges"]:
                src, tgt = edge["source"], edge["target"]
                if src in node_to_idx and tgt in node_to_idx:
                    step_adj[node_to_idx[src], node_to_idx[tgt]] = 1.0
            adj_seq.append(step_adj)

        x_t = torch.from_numpy(np.stack(feats_seq, axis=0)).unsqueeze(0).to(device)
        adj_t = torch.from_numpy(np.stack(adj_seq, axis=0)).unsqueeze(0).to(device)

        with torch.no_grad():
            out = tgnn(x_t, adj_t)
            rc_probs = out["root_cause_probs"][0].cpu().numpy()
            lstm_out = lstm(x_t)
            lstm_rc = np.argmax(lstm_out["root_cause_probs"][0].cpu().numpy())

        pred_rc = np.argmax(rc_probs)
        top3_rc = np.argsort(rc_probs)[-3:]

        if pred_rc == target_idx:
            c2_tgnn_rc_hits += 1
        if target_idx in top3_rc:
            c2_tgnn_rc_top3_hits += 1
        if lstm_rc == target_idx:
            c2_lstm_rc_hits += 1

    results["case2_cyclic_scc"] = {
        "description": "Cyclic SCC Feedback Loop (Mutual Retries / Ping-Pong)",
        "tgnn_rc_top1_acc": c2_tgnn_rc_hits / num_episodes_per_case,
        "tgnn_rc_top3_acc": c2_tgnn_rc_top3_hits / num_episodes_per_case,
        "lstm_rc_top1_acc": c2_lstm_rc_hits / num_episodes_per_case,
    }

    # =========================================================================
    # Stress Case 3: Concurrent Multi-Point Faults (Dual Failures)
    # =========================================================================
    logger.info("Running Stress Case 3: Multi-Point Concurrent Faults (Dual Failures)...")
    c3_tgnn_found_either = 0
    c3_tgnn_found_both_top3 = 0
    c3_lstm_found_either = 0

    for _ in range(num_episodes_per_case):
        # Pick two distinct services: e.g. gateway and node-d
        node1, node2 = random.sample(node_ids, 2)
        idx1, idx2 = node_to_idx[node1], node_to_idx[node2]

        feats_seq = []
        adj_seq = []

        for step in range(10):
            ramp = (step + 1) / 10.0
            s1 = 0.10 + 0.50 * ramp
            s2 = 0.12 + 0.48 * ramp

            batch1 = consumer.generate_simulated_batch(
                fault_node=node1,
                fault_type="cpu_stress",
                precursor_severity=s1,
            )
            batch2 = consumer.generate_simulated_batch(
                fault_node=node2,
                fault_type="memory_leak",
                precursor_severity=s2,
            )

            # Merge metrics: replace node2's metrics in batch1 with batch2's
            merged_metrics = [
                m for m in batch1["metrics"] if m["service_id"] != node2
            ] + [
                m for m in batch2["metrics"] if m["service_id"] == node2
            ]
            batch1["metrics"] = merged_metrics

            rep = service.process_telemetry_batch(batch1)
            step_feats = np.zeros((num_nodes, in_dim), dtype=np.float32)
            for nid, feat in rep["feature_matrix"].items():
                if nid in node_to_idx:
                    step_feats[node_to_idx[nid]] = np.array(feat, dtype=np.float32)
            feats_seq.append(step_feats)

            step_adj = np.zeros((num_nodes, num_nodes), dtype=np.float32)
            for edge in rep["snapshot"]["edges"]:
                src, tgt = edge["source"], edge["target"]
                if src in node_to_idx and tgt in node_to_idx:
                    step_adj[node_to_idx[src], node_to_idx[tgt]] = 1.0
            adj_seq.append(step_adj)

        x_t = torch.from_numpy(np.stack(feats_seq, axis=0)).unsqueeze(0).to(device)
        adj_t = torch.from_numpy(np.stack(adj_seq, axis=0)).unsqueeze(0).to(device)

        with torch.no_grad():
            out = tgnn(x_t, adj_t)
            rc_probs = out["root_cause_probs"][0].cpu().numpy()
            lstm_out = lstm(x_t)
            lstm_rc = np.argmax(lstm_out["root_cause_probs"][0].cpu().numpy())

        pred_rc = np.argmax(rc_probs)
        top3_rc = np.argsort(rc_probs)[-3:]

        if pred_rc in (idx1, idx2):
            c3_tgnn_found_either += 1
        if idx1 in top3_rc and idx2 in top3_rc:
            c3_tgnn_found_both_top3 += 1
        if lstm_rc in (idx1, idx2):
            c3_lstm_found_either += 1

    results["case3_concurrent_dual_faults"] = {
        "description": "Multi-Point Concurrent Faults (Dual Independent Failures)",
        "tgnn_rc_top1_captured_either": c3_tgnn_found_either / num_episodes_per_case,
        "tgnn_rc_top3_captured_both": c3_tgnn_found_both_top3 / num_episodes_per_case,
        "lstm_rc_captured_either": c3_lstm_found_either / num_episodes_per_case,
    }

    logger.info("=== Hard-Case Stress Testing Benchmark Results ===")
    for case, data in results.items():
        logger.info(f"\n{data['description']}:")
        for k, v in data.items():
            if k != "description":
                logger.info(f"   {k}: {v*100:.1f}%")

    return results


if __name__ == "__main__":
    run_hard_case_eval()
