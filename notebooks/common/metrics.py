"""Metric definitions used by evaluation notebooks."""

import numpy as np
from sklearn.metrics import average_precision_score, balanced_accuracy_score, confusion_matrix, f1_score, matthews_corrcoef, mean_absolute_error


def failure_metrics(y_true, probabilities, threshold=0.5):
    predictions = (np.asarray(probabilities) >= threshold).astype(int)
    return {"f1": float(f1_score(y_true, predictions, zero_division=0)), "pr_auc": float(average_precision_score(y_true, probabilities)), "mcc": float(matthews_corrcoef(y_true, predictions)), "balanced_accuracy": float(balanced_accuracy_score(y_true, predictions)), "confusion_matrix": confusion_matrix(y_true, predictions).tolist()}


def top_k_accuracy(y_true, probabilities, k):
    return float(np.mean([target in np.argsort(row)[-k:] for target, row in zip(y_true, probabilities)]))


def propagation_iou(y_true, probabilities, threshold=0.5):
    scores = []
    for target, prediction in zip(np.asarray(y_true), np.asarray(probabilities) >= threshold):
        union = np.logical_or(target, prediction).sum()
        scores.append(1.0 if union == 0 else float(np.logical_and(target, prediction).sum() / union))
    return float(np.mean(scores))


def ttf_mae(y_true, prediction):
    return float(mean_absolute_error(y_true, prediction))