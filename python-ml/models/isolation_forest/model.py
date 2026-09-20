"""
Isolation Forest Baseline (Phase 3)
Topology-blind tabular anomaly detection baseline using scikit-learn.
Flattens microservice metrics into an unstructured feature vector,
serving as the non-graph, non-temporal comparative baseline.
"""

from typing import Dict, Any, List, Optional
import numpy as np
from sklearn.ensemble import IsolationForest


class IsolationForestBaseline:
    """
    Evaluates failure anomalies by fitting isolation trees on flattened metrics.
    Lacks spatial topology and message passing awareness.
    """

    def __init__(self, contamination: float = 0.15, n_estimators: int = 100, random_state: int = 42):
        self.model = IsolationForest(
            contamination=contamination,
            n_estimators=n_estimators,
            random_state=random_state,
            n_jobs=-1,
        )
        self.is_fitted = False
        self.num_nodes = 5
        self.feature_dim = 8

    def _flatten_features(self, node_features: np.ndarray) -> np.ndarray:
        """
        node_features can be:
        (B, T, N, D) -> average over T to get (B, N * D)
        or (B, N, D) -> (B, N * D)
        """
        if node_features.ndim == 4:
            # (B, T, N, D) -> mean over time window
            mean_over_time = np.mean(node_features, axis=1)  # (B, N, D)
            return mean_over_time.reshape(mean_over_time.shape[0], -1)
        elif node_features.ndim == 3:
            return node_features.reshape(node_features.shape[0], -1)
        else:
            raise ValueError(f"Unexpected shape for node_features: {node_features.shape}")

    def fit(self, node_features: np.ndarray):
        """Fits the Isolation Forest on flattened metric representations."""
        X = self._flatten_features(node_features)
        self.model.fit(X)
        self.is_fitted = True

    def predict_anomaly(self, node_features: np.ndarray) -> Dict[str, np.ndarray]:
        """
        Returns:
          failure_probs: (B,) failure probability derived from decision function
          root_cause_scores: (B, N) ranking nodes based on individual metric deviation
        """
        if not self.is_fitted:
            raise RuntimeError("IsolationForest baseline must be fitted before predict_anomaly")

        X = self._flatten_features(node_features)
        raw_scores = self.model.decision_function(X)  # Lower is more anomalous
        # Normalize into [0, 1] failure probability
        failure_probs = 1.0 / (1.0 + np.exp(raw_scores * 8.0))

        # Approximate node root causes by examining node feature magnitude deviation
        if node_features.ndim == 4:
            latest_features = node_features[:, -1, :, :]  # (B, N, D)
        else:
            latest_features = node_features  # (B, N, D)

        # Deviation from nominal median (heuristic for non-graph baseline)
        node_deviations = np.linalg.norm(latest_features, axis=-1)  # (B, N)
        # Normalize to probability distribution over nodes
        sums = node_deviations.sum(axis=-1, keepdims=True)
        sums[sums == 0] = 1e-6
        root_cause_probs = node_deviations / sums

        return {
            "failure_probs": failure_probs,
            "root_cause_probs": root_cause_probs,
            "raw_scores": raw_scores,
        }
