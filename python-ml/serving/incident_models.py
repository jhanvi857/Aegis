"""Loader and feature adapter for offline-trained incident classifiers."""
from __future__ import annotations

import os
from pathlib import Path
from typing import Any, Mapping, Optional, Tuple

import joblib
import numpy as np
import yaml

DEFAULT_FEATURES = ["cpu_mean", "cpu_max", "latency_ms_mean", "latency_ms_max", "error_rate_mean", "error_rate_max"]
CANONICAL_SIGNALS = ("cpu", "latency", "error_rate")


class InsufficientCoverageError(Exception):
    """Raised when incoming telemetry feature coverage is below the required threshold."""
    pass


class IncidentModels:
    def __init__(self, artifact: Mapping[str, Any], mapping_path: Optional[Path] = None) -> None:
        if artifact.get("schema_version") != 1:
            raise ValueError("unsupported incident model artifact schema")
        self.artifact = artifact
        self.model_type = str(artifact.get("model_type", "RandomForest"))
        self.root_model = artifact["root_model"]
        self.fault_model = artifact["fault_model"]
        self.features = list(artifact.get("features", DEFAULT_FEATURES))
        self.min_feature_coverage = float(artifact.get("min_feature_coverage", 0.5))
        self.root_threshold = float(artifact.get("root_threshold", 0.5))
        self.fault_classes = [str(value) for value in artifact.get("fault_classes", [])]
        self.fault_map = {str(k): str(v) for k, v in artifact.get("fault_map", {}).items()}
        self.model_names = dict(artifact.get("models", {}))
        self.metric_mapping = self._load_metric_mapping(mapping_path)

    @staticmethod
    def _load_metric_mapping(mapping_path: Optional[Path] = None) -> dict[str, str]:
        if mapping_path is None:
            env_path = os.environ.get("AEGIS_METRIC_MAPPING", "").strip()
            if env_path:
                mapping_path = Path(env_path)
            else:
                mapping_path = Path(__file__).resolve().parents[2] / "configs" / "metric_mapping.yaml"
        if mapping_path and mapping_path.is_file():
            try:
                with mapping_path.open(encoding="utf-8") as f:
                    data = yaml.safe_load(f) or {}
                    return {str(k).lower(): str(v).lower() for k, v in data.get("metrics", {}).items()}
            except Exception:
                pass
        return {}

    @classmethod
    def load_configured(cls, configured_path: Optional[str] = None) -> Tuple[Optional["IncidentModels"], Optional[str]]:
        path_str = configured_path or os.environ.get("AEGIS_MODEL_PATH", "").strip() or os.environ.get("AEGIS_ML_MODEL_PATH", "").strip()
        path = Path(path_str) if path_str else Path(__file__).resolve().parents[1] / "artifacts" / "incident_models.joblib"
        if not path.is_file():
            return None, f"artifact file not found at {path}"
        try:
            artifact = joblib.load(path)
            return cls(artifact), None
        except Exception as exc:
            return None, f"corrupt or invalid artifact at {path}: {exc}"

    def canonicalize_metrics(self, raw_metrics: Mapping[str, Any]) -> Tuple[dict[str, float], float]:
        values: dict[str, float] = {}
        for raw_name, raw_val in raw_metrics.items():
            try:
                val = float(raw_val)
            except (TypeError, ValueError):
                continue
            if not np.isfinite(val):
                continue

            lower_name = str(raw_name).lower()
            mapped = self.metric_mapping.get(lower_name, lower_name)

            if mapped in {"cpu", "cpu_usage", "cpu_utilization"}:
                values["cpu"] = val
            elif mapped in {"latency", "latency_ms", "p95_latency_ms", "p99_latency_ms"}:
                values["latency_ms"] = val
            elif mapped in {"error", "error_rate"}:
                if val > 1.0:
                    val /= 100.0
                values["error_rate"] = val

        observed_count = sum(1 for s in CANONICAL_SIGNALS if (s in values or f"{s}_ms" in values))
        coverage = observed_count / float(len(CANONICAL_SIGNALS))
        return values, coverage

    def predict(self, metrics_by_service: Mapping[str, Mapping[str, Any]]) -> Tuple[dict[str, dict[str, Any]], float]:
        services = []
        rows = []
        coverages = []

        for service, metrics in metrics_by_service.items():
            if not metrics:
                continue
            values, cov = self.canonicalize_metrics(metrics)
            coverages.append(cov)
            if not values:
                continue

            row: dict[str, float] = {
                "cpu_mean": values.get("cpu", np.nan),
                "cpu_max": values.get("cpu", np.nan),
                "latency_ms_mean": values.get("latency_ms", np.nan),
                "latency_ms_max": values.get("latency_ms", np.nan),
                "error_rate_mean": values.get("error_rate", np.nan),
                "error_rate_max": values.get("error_rate", np.nan),
            }
            services.append(str(service))
            rows.append([row.get(name, np.nan) for name in self.features])

        max_cov = max(coverages) if coverages else 0.0
        if max_cov < self.min_feature_coverage:
            raise InsufficientCoverageError(
                f"feature coverage {max_cov:.2f} < minimum required {self.min_feature_coverage:.2f}"
            )

        if not rows:
            return {}, max_cov

        matrix = np.asarray(rows, dtype=float)
        classes = list(self.root_model.classes_)
        root_index = classes.index(1) if 1 in classes else (classes.index(True) if True in classes else int(np.argmax(classes)))
        root_scores = self.root_model.predict_proba(matrix)[:, root_index]
        fault_scores = self.fault_model.predict_proba(matrix)

        output = {}
        for index, service in enumerate(services):
            class_index = int(np.argmax(fault_scores[index]))
            raw_fault = self.fault_classes[class_index] if class_index < len(self.fault_classes) else "unknown"
            output[service] = {
                "root_score": float(root_scores[index]),
                "root_threshold": self.root_threshold,
                "raw_fault": raw_fault,
                "fault_family": self.fault_map.get(raw_fault, "unknown"),
                "fault_score": float(fault_scores[index, class_index]),
                "features": self.canonicalize_metrics(metrics_by_service[service])[0],
                "root_model": self.model_names.get("root_cause", self.model_type),
                "fault_model": self.model_names.get("fault_family", self.model_type),
            }
        return output, max_cov
