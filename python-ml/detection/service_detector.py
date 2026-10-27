"""Shared, explainable anomaly scoring for service-level telemetry windows."""

from __future__ import annotations

from collections import defaultdict, deque
from dataclasses import dataclass
from datetime import datetime, timezone
import math
import statistics
from pathlib import Path
from typing import Any, Deque, Dict, Iterable, Mapping, Optional

import yaml


@dataclass(frozen=True)
class SignalRule:
    threshold: float
    direction: str = "high"
    unit: str = ""


DEFAULT_RULES: Dict[str, SignalRule] = {
    "cpu_usage": SignalRule(85.0, unit="percent"),
    "cpu_usage_percent": SignalRule(85.0, unit="percent"),
    "memory_usage": SignalRule(90.0, unit="percent"),
    "memory_usage_percent": SignalRule(90.0, unit="percent"),
    "p95_latency_ms": SignalRule(300.0, unit="ms"),
    "p99_latency_ms": SignalRule(1000.0, unit="ms"),
    "latency_ms": SignalRule(300.0, unit="ms"),
    "error_rate": SignalRule(0.05, unit="ratio"),
    "queue_depth": SignalRule(10.0, unit="items"),
    "db_connection_wait_ms": SignalRule(200.0, unit="ms"),
    "cache_miss_rate": SignalRule(0.70, unit="ratio"),
    "packet_loss_rate": SignalRule(0.15, unit="ratio"),
}


class ServiceDetector:
    """Scores multiple services with one detector and independent metric histories.

    Static thresholds work on the first observation. Rolling robust deviations are
    used only after ``min_baseline_samples`` and never replace missing measurements.
    """

    def __init__(
        self,
        rules: Optional[Mapping[str, SignalRule]] = None,
        min_baseline_samples: int = 8,
        history_limit: int = 120,
        robust_z_threshold: float = 4.0,
        service_rules: Optional[Mapping[str, Mapping[str, SignalRule]]] = None,
    ) -> None:
        if min_baseline_samples < 2 or history_limit < min_baseline_samples:
            raise ValueError("history_limit must be >= min_baseline_samples >= 2")
        if robust_z_threshold <= 0:
            raise ValueError("robust_z_threshold must be positive")
        self.rules = dict(rules or DEFAULT_RULES)
        self.min_baseline_samples = min_baseline_samples
        self.history_limit = history_limit
        self.robust_z_threshold = robust_z_threshold
        self.service_rules = {str(service): dict(rules) for service, rules in (service_rules or {}).items()}
        self._history: Dict[tuple[str, str], Deque[tuple[datetime, float]]] = defaultdict(
            lambda: deque(maxlen=self.history_limit)
        )
        self._last_result: Dict[str, Dict[str, Any]] = {}
        self._anomaly_onset: Dict[str, datetime] = {}

    @classmethod
    def from_yaml(cls, path: str | Path) -> "ServiceDetector":
        """Build a detector from explicit signal thresholds and baseline settings."""
        with Path(path).open(encoding="utf-8") as stream:
            config = yaml.safe_load(stream) or {}
        if not isinstance(config, Mapping):
            raise ValueError("detector configuration must be a mapping")
        raw_rules = config.get("signals", {})
        if not isinstance(raw_rules, Mapping):
            raise ValueError("signals must be a mapping")
        rules = dict(DEFAULT_RULES)
        for raw_name, raw_rule in raw_rules.items():
            if not isinstance(raw_rule, Mapping) or "threshold" not in raw_rule:
                raise ValueError(f"signal {raw_name!r} requires a threshold")
            direction = str(raw_rule.get("direction", "high")).lower()
            threshold = float(raw_rule["threshold"])
            if direction not in {"high", "low"} or not math.isfinite(threshold):
                raise ValueError(f"signal {raw_name!r} has an invalid threshold or direction")
            rules[cls._canonical_name(str(raw_name))] = SignalRule(threshold, direction, str(raw_rule.get("unit", "")))
        service_rules: Dict[str, Dict[str, SignalRule]] = {}
        raw_services = config.get("services", {})
        if not isinstance(raw_services, Mapping):
            raise ValueError("services must be a mapping from service.name to SLO overrides")
        for service_id, service_config in raw_services.items():
            if not isinstance(service_config, Mapping) or not isinstance(service_config.get("signals", {}), Mapping):
                raise ValueError(f"service {service_id!r} must contain a signals mapping")
            overrides: Dict[str, SignalRule] = {}
            for raw_name, raw_rule in service_config.get("signals", {}).items():
                if not isinstance(raw_rule, Mapping) or "threshold" not in raw_rule:
                    raise ValueError(f"service {service_id!r} signal {raw_name!r} requires a threshold")
                direction = str(raw_rule.get("direction", "high")).lower()
                threshold = float(raw_rule["threshold"])
                if direction not in {"high", "low"} or not math.isfinite(threshold):
                    raise ValueError(f"service {service_id!r} signal {raw_name!r} has an invalid threshold or direction")
                overrides[cls._canonical_name(str(raw_name))] = SignalRule(threshold, direction, str(raw_rule.get("unit", "")))
            service_rules[str(service_id)] = overrides
        return cls(
            rules=rules,
            min_baseline_samples=int(config.get("min_baseline_samples", 8)),
            history_limit=int(config.get("history_limit", 120)),
            robust_z_threshold=float(config.get("robust_z_threshold", 4.0)),
            service_rules=service_rules,
        )

    def observe(
        self,
        service_id: str,
        values: Mapping[str, Any],
        observed_at: Optional[datetime] = None,
        window_start: Optional[datetime] = None,
        window_seconds: Optional[float] = None,
    ) -> Dict[str, Any]:
        if not service_id or not service_id.strip():
            raise ValueError("service_id is required")
        end = observed_at or datetime.now(timezone.utc)
        if end.tzinfo is None:
            raise ValueError("observed_at must be timezone-aware")

        rules = {**self.rules, **self.service_rules.get(service_id, {})}
        evidence = []
        valid_signal_observed = False
        for raw_name, raw_value in values.items():
            name = self._canonical_name(raw_name)
            rule = rules.get(name)
            if rule is None:
                continue
            try:
                value = float(raw_value)
            except (TypeError, ValueError):
                continue
            if not math.isfinite(value):
                continue
            valid_signal_observed = True

            history = self._history[(service_id, name)]
            baseline = list(history)
            threshold_exceeded = value >= rule.threshold if rule.direction == "high" else value <= rule.threshold
            threshold_severity = max(0.0, (value - rule.threshold) / max(abs(rule.threshold), 1e-9)) if rule.direction == "high" else max(0.0, (rule.threshold - value) / max(abs(rule.threshold), 1e-9))
            robust_z = None
            if len(baseline) >= self.min_baseline_samples:
                center = statistics.median(v for _, v in baseline)
                mad = statistics.median(abs(v - center) for _, v in baseline)
                scale = max(1.4826 * mad, abs(center) * 0.01, 1e-9)
                robust_z = ((value - center) if rule.direction == "high" else (center - value)) / scale

            rolling_exceeded = robust_z is not None and robust_z >= self.robust_z_threshold
            method = "threshold+rolling_mad" if robust_z is not None else "threshold"
            if threshold_exceeded or rolling_exceeded:
                score = max(threshold_severity, (robust_z or 0.0) / self.robust_z_threshold)
                evidence.append({
                    "metric": name,
                    "value": value,
                    "threshold": rule.threshold,
                    "unit": rule.unit,
                    "baseline_samples": len(baseline),
                    "threshold_score": min(1.0, threshold_severity),
                    "robust_z": round(robust_z, 3) if robust_z is not None else None,
                    "method": method,
                })
            # Do not teach the rolling baseline an observation already flagged
            # by its static SLO or its prior rolling distribution.
            if not threshold_exceeded and not rolling_exceeded:
                history.append((end, value))

        score = max((float(item.get("threshold_score", 0.0)) for item in evidence), default=0.0)
        # Account for low-is-bad rules and robust-baseline-only anomalies.
        if evidence:
            for item in evidence:
                z = item.get("robust_z")
                if z is not None:
                    score = max(score, min(1.0, z / (2 * self.robust_z_threshold)))
        severity = "critical" if score >= 1.0 else "warning" if evidence else "nominal"
        if evidence:
            self._anomaly_onset.setdefault(service_id, end)
        else:
            self._anomaly_onset.pop(service_id, None)
        # A threshold-breaching first sample is still a scored observation even
        # though it is correctly excluded from the rolling baseline history.
        known = valid_signal_observed
        start = window_start
        if start is None and window_seconds is not None:
            from datetime import timedelta
            start = end - timedelta(seconds=window_seconds)
        result = {
            "service_id": service_id,
            "observed_at": end.isoformat(),
            "window_start": start.isoformat() if isinstance(start, datetime) else None,
            "window_end": end.isoformat(),
            "window_seconds": window_seconds,
            "anomaly_onset": self._anomaly_onset.get(service_id).isoformat() if service_id in self._anomaly_onset else None,
            "score": round(score, 4),
            "severity": severity,
            "status": "scored" if known else "insufficient_data",
            "method": sorted({item["method"] for item in evidence}) or ["threshold"],
            "evidence": evidence,
            "signals_observed": sorted(k for k in values if self._canonical_name(k) in rules),
            "signals_missing": sorted(set(rules) - {self._canonical_name(k) for k in values}),
        }
        self._last_result[service_id] = result
        return result

    @staticmethod
    def _canonical_name(name: str) -> str:
        normalized = name.strip().lower()
        aliases = {
            "cpu": "cpu_usage", "cpu_usage_percent": "cpu_usage",
            "memory": "memory_usage", "memory_usage_percent": "memory_usage",
            "latency": "latency_ms", "p95_latency": "p95_latency_ms",
            "p99_latency": "p99_latency_ms", "db_wait": "db_connection_wait_ms",
            "cache_miss": "cache_miss_rate", "packet_loss": "packet_loss_rate",
        }
        return aliases.get(normalized, normalized)


def group_metric_points(metrics: Iterable[Mapping[str, Any]]) -> Dict[str, Dict[str, float]]:
    """Convert telemetry points into per-service observations, skipping bad values."""
    grouped: Dict[str, Dict[str, float]] = defaultdict(dict)
    def timestamp(metric: Mapping[str, Any]) -> int:
        try:
            return int(metric.get("timestamp_unix_nano") or metric.get("timestampUnixNano") or 0)
        except (TypeError, ValueError, OverflowError):
            return 0

    # Stable sorting ensures the newest sample wins when a batch contains
    # duplicate service/signal points. Untimestamped points sort before timed
    # points and remain usable when they are the only available observation.
    for metric in sorted(metrics, key=timestamp):
        service = metric.get("service_id") or metric.get("serviceId")
        name = metric.get("metric_name") or metric.get("metricName")
        if not service or not name:
            continue
        try:
            value = float(metric["value"])
        except (KeyError, TypeError, ValueError):
            continue
        if math.isfinite(value):
            grouped[str(service)][str(name)] = value
    return dict(grouped)
