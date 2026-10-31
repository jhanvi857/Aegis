"""
Fault Family Diagnosis Engine (Phase 4 - Decision Pipeline)
Classifies the dominant fault family from per-node telemetry metrics, producing
evidence-based explanations that cite actual observed values and thresholds.

Fault families are domain-agnostic:
  - database: db_connection_wait_ms elevated, indicating lock contention or pool exhaustion
  - cache: cache_miss_rate collapse, indicating corrupted or evicted cache
  - queue: queue_depth backlog, indicating consumer starvation
  - memory_leak: sustained memory growth slope toward allocation limit
  - cpu_saturation: compute exhaustion causing backlog
  - network_latency: latency+error with none of the specialized indicators above
  - crash: cpu near zero + error near 1.0, indicating process death

Returns: {fault_family, confidence, evidence, explanation}
"""

import logging
from typing import Dict, Any, List, Optional

logger = logging.getLogger(__name__)


# Thresholds for fault classification — these mirror the rulebook thresholds
# but are used for *classification* not action selection.
THRESHOLDS = {
    "db_connection_wait_ms": 500.0,
    "cache_miss_rate": 0.70,
    "queue_depth": 10,
    "memory_usage_percent": 90.0,
    "memory_usage": 90.0,
    "cpu_usage_percent": 85.0,
    "cpu_usage": 85.0,
    "error_rate": 0.50,
    "p95_latency_ms": 300.0,
    "latency_p99_ms": 1000.0,
    "packet_loss_rate": 0.15,
    "active_threads": 80.0,
}


def _make_evidence(metric: str, value: float, threshold: float) -> Dict[str, Any]:
    """Creates a single evidence item."""
    return {
        "metric": metric,
        "value": round(value, 4),
        "threshold": round(threshold, 4),
    }


def diagnose_fault(
    node_id: str,
    metrics: Dict[str, float],
) -> Dict[str, Any]:
    """
    Classifies the dominant fault family for a single node from its telemetry metrics.

    Args:
        node_id: ID of the node being diagnosed.
        metrics: Dictionary of metric_name -> observed_value for this node.

    Returns:
        Dictionary with keys:
          - fault_family: str (one of the families above, or "nominal")
          - confidence: float (0.0 - 1.0)
          - evidence: list of {metric, value, threshold}
          - explanation: str citing actual metric values
    """
    if not metrics:
        return {
            "fault_family": "nominal",
            "confidence": 0.0,
            "evidence": [],
            "explanation": f"No telemetry data available for {node_id}.",
        }

    evidence: List[Dict[str, Any]] = []

    # Normalize metric name variants
    cpu = _get_metric(metrics, ["cpu_usage", "cpu_usage_percent", "cpu"])
    memory = _get_metric(metrics, ["memory_usage", "memory_usage_percent", "memory"])
    latency = _get_metric(metrics, ["p95_latency_ms", "latency_p99_ms", "p99_latency_ms", "latency_ms", "latency"])
    error_rate = _get_metric(metrics, ["error_rate"])
    db_wait = _get_metric(metrics, ["db_connection_wait_ms"])
    cache_miss = _get_metric(metrics, ["cache_miss_rate"])
    queue_depth = _get_metric(metrics, ["queue_depth"])
    active_threads = _get_metric(metrics, ["active_threads", "thread_count", "thread_utilization", "threads"])
    packet_loss = _get_metric(metrics, ["packet_loss_rate", "packet_loss", "dropped_packets_rate", "packet_loss_pct"])

    # --- Classification rules (ordered by specificity, most specific first) ---

    # 1. Crash detection: CPU near zero AND error near 1.0
    if cpu is not None and error_rate is not None:
        if cpu < 5.0 and error_rate > 0.80:
            evidence.append(_make_evidence("cpu_usage", cpu, 5.0))
            evidence.append(_make_evidence("error_rate", error_rate, 0.80))
            return {
                "fault_family": "crash",
                "confidence": min(1.0, 0.7 + (1.0 - cpu / 5.0) * 0.15 + error_rate * 0.15),
                "evidence": evidence,
                "explanation": (
                    f"Service {node_id} appears crashed: CPU at {cpu:.1f}% (near zero) "
                    f"with error rate {error_rate:.2%}, indicating process death or SIGKILL."
                ),
            }

    # 2. Database contention: db_connection_wait_ms elevated
    if db_wait is not None and db_wait >= THRESHOLDS["db_connection_wait_ms"]:
        evidence.append(_make_evidence("db_connection_wait_ms", db_wait, THRESHOLDS["db_connection_wait_ms"]))
        if error_rate is not None and error_rate > 0.02:
            evidence.append(_make_evidence("error_rate", error_rate, 0.02))
        conf = min(1.0, 0.6 + (db_wait - THRESHOLDS["db_connection_wait_ms"]) / 1000.0)
        return {
            "fault_family": "database",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Database contention on {node_id}: db_connection_wait_ms={db_wait:.1f}ms "
                f"(threshold {THRESHOLDS['db_connection_wait_ms']:.0f}ms)"
                + (f", error_rate={error_rate:.2%}" if error_rate is not None and error_rate > 0.02 else "")
                + ". Indicates lock contention or connection pool exhaustion."
            ),
        }

    # 3. Cache failure: cache_miss_rate elevated
    if cache_miss is not None and cache_miss >= THRESHOLDS["cache_miss_rate"]:
        evidence.append(_make_evidence("cache_miss_rate", cache_miss, THRESHOLDS["cache_miss_rate"]))
        conf = min(1.0, 0.6 + (cache_miss - THRESHOLDS["cache_miss_rate"]) / 0.3)
        return {
            "fault_family": "cache",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Cache failure on {node_id}: cache_miss_rate={cache_miss:.2%} "
                f"(threshold {THRESHOLDS['cache_miss_rate']:.0%}). "
                f"Indicates cache corruption or eviction storm."
            ),
        }

    # 4. Queue backlog: queue_depth elevated
    if queue_depth is not None and queue_depth >= THRESHOLDS["queue_depth"]:
        evidence.append(_make_evidence("queue_depth", float(queue_depth), float(THRESHOLDS["queue_depth"])))
        conf = min(1.0, 0.6 + (queue_depth - THRESHOLDS["queue_depth"]) / 20.0)
        return {
            "fault_family": "queue",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Queue backlog on {node_id}: queue_depth={int(queue_depth)} "
                f"(threshold {THRESHOLDS['queue_depth']}). "
                f"Indicates consumer starvation or upstream overproduction."
            ),
        }

    # 5. Memory leak: memory elevated
    if memory is not None and memory >= THRESHOLDS["memory_usage_percent"]:
        evidence.append(_make_evidence("memory_usage", memory, THRESHOLDS["memory_usage_percent"]))
        conf = min(1.0, 0.6 + (memory - THRESHOLDS["memory_usage_percent"]) / 10.0)
        return {
            "fault_family": "memory_leak",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Memory pressure on {node_id}: memory_usage={memory:.1f}% "
                f"(threshold {THRESHOLDS['memory_usage_percent']:.0f}%). "
                f"Indicates potential memory leak approaching allocation limit."
            ),
        }

    # 6. CPU saturation: CPU elevated
    if cpu is not None and cpu >= THRESHOLDS["cpu_usage_percent"]:
        evidence.append(_make_evidence("cpu_usage", cpu, THRESHOLDS["cpu_usage_percent"]))
        conf = min(1.0, 0.6 + (cpu - THRESHOLDS["cpu_usage_percent"]) / 15.0)
        return {
            "fault_family": "cpu_saturation",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"CPU saturation on {node_id}: cpu_usage={cpu:.1f}% "
                f"(threshold {THRESHOLDS['cpu_usage_percent']:.0f}%). "
                f"Indicates compute exhaustion causing request queuing."
            ),
        }

    # 7. Thread pool exhaustion: active worker threads saturated
    if active_threads is not None and active_threads >= THRESHOLDS["active_threads"]:
        evidence.append(_make_evidence("active_threads", active_threads, THRESHOLDS["active_threads"]))
        if error_rate is not None and error_rate > 0.02:
            evidence.append(_make_evidence("error_rate", error_rate, 0.02))
        conf = min(1.0, 0.6 + (active_threads - THRESHOLDS["active_threads"]) / 40.0)
        return {
            "fault_family": "thread_exhaustion",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Thread pool exhaustion on {node_id}: active_threads={active_threads:.0f} "
                f"(threshold {THRESHOLDS['active_threads']:.0f}). "
                f"Indicates worker thread pool starvation rejecting concurrent requests."
            ),
        }

    # 8. Network packet loss: packet_loss_rate elevated
    if packet_loss is not None and packet_loss >= THRESHOLDS["packet_loss_rate"]:
        evidence.append(_make_evidence("packet_loss_rate", packet_loss, THRESHOLDS["packet_loss_rate"]))
        if error_rate is not None and error_rate > 0.02:
            evidence.append(_make_evidence("error_rate", error_rate, 0.02))
        conf = min(1.0, 0.6 + (packet_loss - THRESHOLDS["packet_loss_rate"]) / 0.5)
        return {
            "fault_family": "packet_loss",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Network packet loss anomaly on {node_id}: packet_loss_rate={packet_loss:.2%} "
                f"(threshold {THRESHOLDS['packet_loss_rate']:.0%}). "
                f"Indicates dropped transmission packets causing TCP retransmissions."
            ),
        }

    # 9. Network/latency: latency or error elevated without any of the specific indicators above
    latency_threshold = THRESHOLDS["p95_latency_ms"]
    if latency is not None and latency >= latency_threshold:
        evidence.append(_make_evidence("p95_latency_ms", latency, latency_threshold))
        if error_rate is not None and error_rate > 0.02:
            evidence.append(_make_evidence("error_rate", error_rate, 0.02))
        conf = min(1.0, 0.5 + (latency - latency_threshold) / 1000.0)
        return {
            "fault_family": "network_latency",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"Network latency anomaly on {node_id}: p95_latency={latency:.1f}ms "
                f"(threshold {latency_threshold:.0f}ms)"
                + (f", error_rate={error_rate:.2%}" if error_rate is not None and error_rate > 0.02 else "")
                + ". No database, cache, or queue indicators elevated — classified as network/latency."
            ),
        }

    # 10. High error rate alone (not crash, not any specific family)
    if error_rate is not None and error_rate >= THRESHOLDS["error_rate"]:
        evidence.append(_make_evidence("error_rate", error_rate, THRESHOLDS["error_rate"]))
        conf = min(1.0, 0.5 + error_rate * 0.3)
        return {
            "fault_family": "crash",
            "confidence": round(conf, 4),
            "evidence": evidence,
            "explanation": (
                f"High error rate on {node_id}: error_rate={error_rate:.2%} "
                f"(threshold {THRESHOLDS['error_rate']:.0%}). "
                f"Service may be partially crashed or failing."
            ),
        }

    # 9. Nominal — no indicators breached
    return {
        "fault_family": "nominal",
        "confidence": 0.95,
        "evidence": [],
        "explanation": f"Service {node_id} operating within normal thresholds.",
    }


def diagnose_nodes(
    telemetry_by_node: Dict[str, Dict[str, float]],
    target_node_id: Optional[str] = None,
) -> Dict[str, Dict[str, Any]]:
    """
    Diagnoses fault family for each node (or just the target node) in the telemetry map.

    Returns a dict mapping node_id -> diagnosis result.
    """
    results = {}

    if target_node_id and target_node_id in telemetry_by_node:
        results[target_node_id] = diagnose_fault(target_node_id, telemetry_by_node[target_node_id])
    else:
        for node_id, metrics in telemetry_by_node.items():
            results[node_id] = diagnose_fault(node_id, metrics)

    return results


def _get_metric(metrics: Dict[str, float], names: List[str]) -> Optional[float]:
    """Returns the first matching metric value from a list of name variants."""
    for name in names:
        if name in metrics:
            try:
                return float(metrics[name])
            except (ValueError, TypeError):
                continue
    return None


# --- Fault family to action mapping (used by plan_builder & decision engine) ---
# Invariant: Never RESTART for contention or latency families.
# Unknown family -> advisory only, no executable action.

DEFAULT_FAULT_FAMILY_ACTIONS = {
    "database": [
        {"action": "INCREASE_POOL", "risk": "LOW", "reason": "Expand connection pool to relieve lock contention", "expected_effect": "Reduce db_connection_wait_ms below threshold"},
        {"action": "KILL_BLOCKING_QUERY", "risk": "MEDIUM", "reason": "Terminate blocking query holding row locks", "expected_effect": "Release locks and restore transactional throughput"},
    ],
    "db_lock": [
        {"action": "INCREASE_POOL", "risk": "LOW", "reason": "Expand connection pool to relieve lock contention", "expected_effect": "Reduce db_connection_wait_ms below threshold"},
        {"action": "KILL_BLOCKING_QUERY", "risk": "MEDIUM", "reason": "Terminate blocking query holding row locks", "expected_effect": "Release locks and restore transactional throughput"},
    ],
    "db_contention": [
        {"action": "INCREASE_POOL", "risk": "LOW", "reason": "Expand connection pool to relieve lock contention", "expected_effect": "Reduce db_connection_wait_ms below threshold"},
        {"action": "KILL_BLOCKING_QUERY", "risk": "MEDIUM", "reason": "Terminate blocking query holding row locks", "expected_effect": "Release locks and restore transactional throughput"},
    ],
    "slow_query": [
        {"action": "INCREASE_POOL", "risk": "LOW", "reason": "Expand pool to absorb query latency", "expected_effect": "Prevent connection starvation"},
        {"action": "KILL_BLOCKING_QUERY", "risk": "MEDIUM", "reason": "Cancel long-running slow query execution", "expected_effect": "Free compute resources"},
    ],
    "cache": [
        {"action": "FLUSH_CACHE", "risk": "LOW", "reason": "Purge corrupted cache entries", "expected_effect": "Rebuild cache from backing store"},
        {"action": "FAILOVER", "risk": "MEDIUM", "reason": "Failover to replica cache cluster", "expected_effect": "Restore cache availability and reduce miss rate"},
    ],
    "cache_down": [
        {"action": "FLUSH_CACHE", "risk": "LOW", "reason": "Purge corrupted cache entries", "expected_effect": "Rebuild cache from backing store"},
        {"action": "FAILOVER", "risk": "MEDIUM", "reason": "Failover to replica cache cluster", "expected_effect": "Restore cache availability and reduce miss rate"},
    ],
    "queue": [
        {"action": "SCALE_UP", "risk": "LOW", "reason": "Scale consumers to drain backlog", "expected_effect": "Reduce queue_depth to normal levels"},
        {"action": "DRAIN_QUEUE", "risk": "MEDIUM", "reason": "Route messages to secondary consumers or dead-letter queue", "expected_effect": "Clear processing bottleneck"},
    ],
    "mq_lag": [
        {"action": "SCALE_UP", "risk": "LOW", "reason": "Scale consumers to drain backlog", "expected_effect": "Reduce queue_depth to normal levels"},
        {"action": "DRAIN_QUEUE", "risk": "MEDIUM", "reason": "Route messages to secondary consumers or dead-letter queue", "expected_effect": "Clear processing bottleneck"},
    ],
    "memory_leak": [
        {"action": "RESTART", "risk": "HIGH", "reason": "Restart to reclaim leaked heap memory", "expected_effect": "Reset memory usage to baseline"},
        {"action": "SCALE_UP", "risk": "MEDIUM", "reason": "Add replicas to distribute memory pressure", "expected_effect": "Reduce per-instance memory load"},
    ],
    "cpu_saturation": [
        {"action": "SCALE_UP", "risk": "LOW", "reason": "Scale replicas to distribute CPU load", "expected_effect": "Reduce per-instance CPU usage below threshold"},
        {"action": "RESTART", "risk": "MEDIUM", "reason": "Restart container to terminate runaway CPU loop", "expected_effect": "Release compute resources"},
    ],
    "cpu_stress": [
        {"action": "SCALE_UP", "risk": "LOW", "reason": "Scale replicas to distribute CPU load", "expected_effect": "Reduce per-instance CPU usage below threshold"},
        {"action": "RESTART", "risk": "MEDIUM", "reason": "Restart container to terminate runaway CPU loop", "expected_effect": "Release compute resources"},
    ],
    "network_latency": [
        {"action": "REROUTE_TRAFFIC", "risk": "MEDIUM", "reason": "Divert traffic away from high-latency path", "expected_effect": "Restore end-to-end latency SLA"},
        {"action": "CIRCUIT_BREAK", "risk": "HIGH", "reason": "Trip circuit breaker to isolate degraded link", "expected_effect": "Prevent cascade failure to callers"},
    ],
    "latency": [
        {"action": "REROUTE_TRAFFIC", "risk": "MEDIUM", "reason": "Divert traffic away from high-latency path", "expected_effect": "Restore end-to-end latency SLA"},
        {"action": "CIRCUIT_BREAK", "risk": "HIGH", "reason": "Trip circuit breaker to isolate degraded link", "expected_effect": "Prevent cascade failure to callers"},
    ],
    "crash": [
        {"action": "RESTART", "risk": "HIGH", "reason": "Restart crashed service process", "expected_effect": "Restore service availability"},
        {"action": "SCALE_UP", "risk": "MEDIUM", "reason": "Add replicas for redundancy", "expected_effect": "Ensure capacity during recovery"},
    ],
    "kill_service": [
        {"action": "RESTART", "risk": "HIGH", "reason": "Restart crashed service process", "expected_effect": "Restore service availability"},
        {"action": "SCALE_UP", "risk": "MEDIUM", "reason": "Add replicas for redundancy", "expected_effect": "Ensure capacity during recovery"},
    ],
    "packet_loss": [
        {"action": "REROUTE_TRAFFIC", "risk": "MEDIUM", "reason": "Divert traffic away from packet loss degraded network interface", "expected_effect": "Eliminate packet drops on critical path"},
        {"action": "CIRCUIT_BREAK", "risk": "HIGH", "reason": "Trip circuit breaker to isolate packet-dropping link", "expected_effect": "Force traffic through healthy path"},
    ],
    "thread_exhaustion": [
        {"action": "SCALE_UP", "risk": "LOW", "reason": "Scale service replicas to distribute thread pool pressure", "expected_effect": "Reduce active threads per instance below threshold"},
        {"action": "CIRCUIT_BREAK", "risk": "HIGH", "reason": "Shed incoming traffic to relieve thread pool", "expected_effect": "Prevent complete process starvation"},
    ],
    "nominal": [],
}


def _load_actions_from_rulebook() -> Dict[str, List[Dict[str, Any]]]:
    """Dynamically loads fault_family_actions from rulebook.yaml if available."""
    try:
        import yaml
        base_dir = os.path.dirname(os.path.abspath(__file__))
        path = os.path.join(base_dir, "rulebook.yaml")
        if os.path.exists(path):
            with open(path, "r", encoding="utf-8") as f:
                data = yaml.safe_load(f)
                if isinstance(data, dict) and "fault_family_actions" in data:
                    res: Dict[str, List[Dict[str, Any]]] = {}
                    for fam, acts in data["fault_family_actions"].items():
                        if isinstance(acts, list):
                            res[fam] = acts
                    if res:
                        return res
    except Exception:
        pass
    return DEFAULT_FAULT_FAMILY_ACTIONS


FAULT_FAMILY_ACTIONS = _load_actions_from_rulebook()


def get_candidate_actions(fault_family: str, top_k: int = 3) -> List[Dict[str, Any]]:
    """
    Returns the top-k ranked candidate recovery actions for a given fault family.
    Each candidate includes action type, risk level, reason, and expected effect.
    Returns empty list for nominal or unmapped states (advisory only).
    """
    if not fault_family or fault_family == "nominal":
        return []
    actions_map = _load_actions_from_rulebook()
    candidates = actions_map.get(fault_family, [])
    return candidates[:top_k]

