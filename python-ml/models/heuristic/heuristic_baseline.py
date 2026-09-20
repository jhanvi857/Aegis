"""
Heuristic & Majority-Class Baselines (Phase 3 Extension)
Provides traditional rule-based threshold alerting and naive majority-class baselines
to establish a rigorous 4-tier model hierarchy:
  1. Baseline: Heuristic Threshold Rule & Majority Class
  2. Model 1: Isolation Forest (Tabular Unsupervised Anomaly Detection)
  3. Model 2: LSTM (Temporal-Only Sequence Modeling)
  4. Model 3: TGNN (Spatio-Temporal Graph Neural Network - Ours)
"""

from typing import Dict, Any, Optional
import numpy as np


class HeuristicThresholdBaseline:
    """
    Simulates industry-standard static threshold alerting (e.g., Prometheus / CloudWatch alerts).
    Fires an alert if any telemetry metric breaches predefined operational thresholds
    during the observation window. Root cause is assigned to the service node with the
    largest metric excursion. Lacks graph dependency and cascade awareness.
    """

    def __init__(
        self,
        cpu_threshold: float = 0.60,       # 60% CPU
        lat_threshold: float = 0.15,       # 75ms normalized against 500ms
        err_threshold: float = 0.04,       # 2% error rate normalized against 50%
        queue_threshold: float = 0.12,     # 6 items normalized against 50
    ):
        self.cpu_threshold = cpu_threshold
        self.lat_threshold = lat_threshold
        self.err_threshold = err_threshold
        self.queue_threshold = queue_threshold
        self.feature_means = None
        self.feature_stds = None

    def fit(self, node_features: np.ndarray):
        """
        Fits baseline mean and standard deviation from nominal/training telemetry.
        node_features: (B, T, N, D) or (B, N, D)
        """
        if node_features.ndim == 4:
            flat = node_features.reshape(-1, node_features.shape[-1])
        else:
            flat = node_features.reshape(-1, node_features.shape[-1])

        self.feature_means = np.mean(flat, axis=0)
        self.feature_stds = np.std(flat, axis=0) + 1e-6

    def predict(
        self,
        node_features: np.ndarray,
        node_mask: Optional[np.ndarray] = None,
    ) -> Dict[str, np.ndarray]:
        """
        Evaluates heuristic threshold rules over observation window.
        Input:
          node_features: (B, T, N, D)
          node_mask: (B, N) optional binary mask
        Returns:
          failure_probs: (B,) binary 0.0 or 1.0
          root_cause_probs: (B, N + 1) one-hot distribution (N = nominal)
          propagation_probs: (B, N) all zeros (cannot forecast cascades)
        """
        if node_features.ndim != 4:
            raise ValueError(f"Expected 4D array (B, T, N, D), got {node_features.shape}")

        batch_size, seq_len, num_nodes, feat_dim = node_features.shape
        if node_mask is None:
            node_mask = np.ones((batch_size, num_nodes), dtype=np.float32)

        # Max over observation window T
        max_over_time = np.max(node_features, axis=1)  # (B, N, D)

        cpu = max_over_time[..., 0]
        lat = max_over_time[..., 2]
        err = max_over_time[..., 3]
        queue = max_over_time[..., 7]

        # Anomaly condition per node
        breach_cpu = cpu > self.cpu_threshold
        breach_lat = lat > self.lat_threshold
        breach_err = err > self.err_threshold
        breach_queue = queue > self.queue_threshold

        node_breach = (breach_cpu | breach_lat | breach_err | breach_queue) & (node_mask > 0)  # (B, N)
        cluster_failure = np.any(node_breach, axis=1).astype(np.float32)  # (B,)

        # Severity score = sum of relative deviations
        cpu_dev = np.maximum(0.0, (cpu - self.cpu_threshold) / (1.0 - self.cpu_threshold + 1e-6))
        lat_dev = np.maximum(0.0, (lat - self.lat_threshold) / (1.0 - self.lat_threshold + 1e-6))
        err_dev = np.maximum(0.0, (err - self.err_threshold) / (1.0 - self.err_threshold + 1e-6))
        queue_dev = np.maximum(0.0, (queue - self.queue_threshold) / (1.0 - self.queue_threshold + 1e-6))
        severity = (cpu_dev + lat_dev + err_dev + queue_dev) * node_mask  # (B, N)

        # Root cause distribution over N nodes + 1 nominal class
        root_cause_probs = np.zeros((batch_size, num_nodes + 1), dtype=np.float32)
        for b in range(batch_size):
            if cluster_failure[b] > 0.5:
                rc_node = int(np.argmax(severity[b]))
                root_cause_probs[b, rc_node] = 1.0
            else:
                # Nominal class
                root_cause_probs[b, num_nodes] = 1.0

        return {
            "failure_probs": cluster_failure,
            "root_cause_probs": root_cause_probs,
            "propagation_probs": np.zeros((batch_size, num_nodes), dtype=np.float32),
        }


class MajorityClassBaseline:
    """
    Naive majority-class baseline that predicts the empirical mode of the training distribution.
    Useful for demonstrating lower-bound accuracy under class imbalance.
    """

    def __init__(self):
        self.majority_failure = 1.0
        self.majority_rc = 0
        self.num_nodes = 5

    def fit(self, failure_labels: np.ndarray, root_causes: np.ndarray, num_nodes: int = 5):
        self.num_nodes = num_nodes
        # Mode of failure labels
        counts = np.bincount(failure_labels.astype(int))
        self.majority_failure = float(np.argmax(counts))

        # Mode of root causes
        rc_counts = np.bincount(root_causes.astype(int))
        self.majority_rc = int(np.argmax(rc_counts))

    def predict(self, batch_size: int, num_nodes: Optional[int] = None) -> Dict[str, np.ndarray]:
        n_nodes = num_nodes or self.num_nodes
        fail_probs = np.full((batch_size,), self.majority_failure, dtype=np.float32)

        rc_probs = np.zeros((batch_size, n_nodes + 1), dtype=np.float32)
        target_col = min(self.majority_rc, n_nodes)
        rc_probs[:, target_col] = 1.0

        return {
            "failure_probs": fail_probs,
            "root_cause_probs": rc_probs,
            "propagation_probs": np.zeros((batch_size, n_nodes), dtype=np.float32),
        }
