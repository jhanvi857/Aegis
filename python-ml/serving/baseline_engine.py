"""Baseline-first prediction, blast-radius, RCA, and recommendation pipeline."""

from collections import deque
from datetime import datetime, timezone
import hashlib
import os
import time
from typing import Any, Dict, List, Mapping, Optional

import networkx as nx

from detection.service_detector import ServiceDetector, group_metric_points
from decision_engine.diagnosis import diagnose_fault
from recovery_planner.plan_builder import RecoveryPlanBuilder


class PredictionEngine:
    """Shared statistical detector followed by evidence-ranked graph RCA.

    The engine emits recommendations only. It does not execute recovery actions.
    Candidate scores are rankings, not calibrated causal probabilities.
    """

    def __init__(self, checkpoint_path: Optional[str] = None) -> None:
        del checkpoint_path  # Kept for backwards-compatible construction only.
        detector_config = os.environ.get("AEGIS_DETECTION_CONFIG", "").strip()
        self.detector = ServiceDetector.from_yaml(detector_config) if detector_config else ServiceDetector()
        self.plan_builder = RecoveryPlanBuilder()

    def predict(
        self,
        graph_representation: Dict[str, Any],
        telemetry_batch: Optional[Dict[str, Any]] = None,
        target_node_id: Optional[str] = None,
        generate_plan: bool = True,
    ) -> Dict[str, Any]:
        now = datetime.now(timezone.utc)
        snapshot = graph_representation.get("snapshot", {})
        nodes = snapshot.get("nodes", [])
        edges = snapshot.get("edges", [])
        service_ids = list(graph_representation.get("node_ids", []))
        if not service_ids:
            service_ids = [str(node["id"]) for node in nodes if node.get("id")]

        metrics_by_service: Dict[str, Dict[str, float]] = {}
        observed_at_by_service: Dict[str, datetime] = {}
        for node in nodes:
            service_id = str(node.get("id", ""))
            if service_id:
                metrics_by_service[service_id] = self._valid_metrics(node.get("metrics", {}))
        if telemetry_batch:
            for service_id, values in group_metric_points(telemetry_batch.get("metrics", [] )).items():
                metrics_by_service.setdefault(service_id, {}).update(values)
                if service_id not in service_ids:
                    service_ids.append(service_id)
            for point in telemetry_batch.get("metrics", []):
                service_id = point.get("service_id") or point.get("serviceId")
                timestamp = point.get("timestamp_unix_nano") or point.get("timestampUnixNano")
                try:
                    observed = datetime.fromtimestamp(int(timestamp) / 1e9, tz=timezone.utc)
                except (TypeError, ValueError, OSError, OverflowError):
                    continue
                if service_id and observed <= now and observed > observed_at_by_service.get(str(service_id), datetime.min.replace(tzinfo=timezone.utc)):
                    observed_at_by_service[str(service_id)] = observed

        # Derive a request error ratio only from a minimally useful trace sample.
        # This remains evidence from observed spans and is never fabricated from
        # a missing metric field.
        trace_counts: Dict[str, List[int]] = {}
        derived_trace_error_rate = set()
        for span in (telemetry_batch or {}).get("traces", []):
            service_id = str(span.get("service_id", ""))
            if not service_id:
                continue
            counts = trace_counts.setdefault(service_id, [0, 0])
            counts[0] += 1
            if int(span.get("status_code", 0) or 0) >= 400:
                counts[1] += 1
        for service_id, (total, failures) in trace_counts.items():
            if total >= 10 and "error_rate" not in metrics_by_service.get(service_id, {}):
                metrics_by_service.setdefault(service_id, {})["error_rate"] = failures / total
                derived_trace_error_rate.add(service_id)
                if service_id not in service_ids:
                    service_ids.append(service_id)

        # If no metrics were supplied for a registered node, emit no score.
        detections = {}
        for service_id in service_ids:
            observed = metrics_by_service.get(service_id, {})
            if observed:
                detections[service_id] = self.detector.observe(
                    service_id, observed, observed_at=observed_at_by_service.get(service_id, now)
                )
                if service_id in derived_trace_error_rate:
                    for evidence in detections[service_id]["evidence"]:
                        if evidence["metric"] == "error_rate":
                            evidence["source"] = "derived_from_trace_status"
                            evidence["sample_count"] = trace_counts[service_id][0]

        anomalies = [d for d in detections.values() if d["evidence"]]
        anomalies.sort(key=lambda item: (-item["score"], item["observed_at"], item["service_id"]))
        graph = nx.DiGraph()
        graph.add_nodes_from(service_ids)
        for edge in edges:
            source, target = edge.get("source"), edge.get("target")
            if source in graph and target in graph:
                graph.add_edge(source, target, **{k: v for k, v in edge.items() if k not in {"source", "target"}})

        traces = (telemetry_batch or {}).get("traces", [])
        logs = (telemetry_batch or {}).get("logs", [])
        incidents = self._correlate_incidents(anomalies, graph, traces, logs)
        root_causes = []
        for incident in incidents:
            incident_candidates = [item for item in anomalies if item["service_id"] in incident["affected_services"]]
            for candidate in self._rank_candidates(
                incident_candidates, metrics_by_service, graph, target_node_id, traces, logs
            ):
                candidate["incident_id"] = incident["incident_id"]
                root_causes.append(candidate)
        propagation = self._blast_radius(graph, [item["node_id"] for item in root_causes])
        failure_predictions = [
            {
                "node_id": service_id,
                "failure_probability": float(detections[service_id]["score"]),
                "confidence": 0.0,
                "estimated_time_to_failure_sec": 0,
            }
            for service_id in service_ids
            if service_id in detections
        ]

        plan = None
        if generate_plan and root_causes:
            # Recommendations apply only to services with observed anomaly
            # evidence. BFS reachability is potential impact and must not
            # become an implicit recovery target list.
            observed_affected = sorted({item["service_id"] for item in anomalies})
            plan = self.plan_builder.build_plan(
                graph=graph,
                root_causes=root_causes,
                propagation_set=observed_affected,
                telemetry_by_node=metrics_by_service,
            )

        return {
            "failure_predictions": failure_predictions,
            "root_causes": root_causes,
            "propagation_paths": propagation,
            "recovery_plan": plan,
            "timestamp_unix_nano": int(now.timestamp() * 1e9),
            "engine": "statistical_baseline",
            "model_version": "service-baseline-v1",
            "calibrated_confidence": 0.0,
            "detections": detections,
            "incidents": incidents,
            # The protobuf probability field has no calibration contract. Keep
            # it at zero; ranking_score and evidence explain candidate order.
            "rca_uncertain": bool(root_causes),
        }

    @staticmethod
    def _valid_metrics(metrics: Mapping[str, Any]) -> Dict[str, float]:
        from math import isfinite

        valid = {}
        for key, value in metrics.items():
            try:
                number = float(value)
            except (TypeError, ValueError):
                continue
            if isfinite(number):
                valid[str(key)] = number
        return valid

    @staticmethod
    def _rank_candidates(
        anomalies: List[Dict[str, Any]],
        metrics: Mapping[str, Mapping[str, float]],
        graph: nx.DiGraph,
        target_node_id: Optional[str],
        traces: Optional[List[Mapping[str, Any]]] = None,
        logs: Optional[List[Mapping[str, Any]]] = None,
    ) -> List[Dict[str, Any]]:
        """Rank abnormal nodes while explicitly avoiding causal certainty claims."""
        if not anomalies:
            return []
        ranked = []
        onset_by_service = {}
        for anomaly in anomalies:
            try:
                onset_by_service[anomaly["service_id"]] = datetime.fromisoformat(anomaly.get("anomaly_onset") or anomaly["observed_at"])
            except (TypeError, ValueError):
                continue
        earliest_onset = min(onset_by_service.values(), default=None)
        for item in anomalies:
            service_id = item["service_id"]
            fault = diagnose_fault(service_id, dict(metrics.get(service_id, {})))
            direct_score = float(item["score"])
            evidence = list(item["evidence"])
            service_traces = [span for span in (traces or []) if span.get("service_id") == service_id]
            error_spans = [span for span in service_traces if int(span.get("status_code", 0) or 0) >= 400]
            error_span_times = [int(span.get("start_time_unix_nano", 0) or 0) for span in error_spans if int(span.get("start_time_unix_nano", 0) or 0) > 0]
            service_logs = [entry for entry in (logs or []) if entry.get("service_id") == service_id]
            error_logs = [entry for entry in service_logs if str(entry.get("level", "")).upper() in {"ERROR", "FATAL", "SEVERE"}]
            score = direct_score + min(0.3, 0.1 * len(error_spans)) + min(0.15, 0.05 * len(error_logs))
            onset = onset_by_service.get(service_id)
            if earliest_onset is not None and onset == earliest_onset and len(onset_by_service) > 1:
                score += 0.05
                evidence.append({"metric": "relative_anomaly_onset_order", "value": 1, "threshold": 1, "unit": "earliest_in_incident"})
            if error_spans:
                evidence.append({"metric": "trace_error_span_count", "value": len(error_spans), "threshold": 1, "unit": "spans"})
                if error_span_times:
                    evidence.append({"metric": "earliest_error_span_unix_nano", "value": min(error_span_times), "threshold": 0, "unit": "unix_ns"})
            if error_logs:
                evidence.append({"metric": "error_log_count", "value": len(error_logs), "threshold": 1, "unit": "records"})
            if target_node_id and service_id == target_node_id:
                evidence.append({"metric": "requested_target", "value": 1, "threshold": 1, "unit": "boolean"})
            ranked.append({
                "node_id": service_id,
                # This normalized value is a ranking share, not a calibrated probability.
                "probability": 0.0,
                "ranking_score": round(score, 4),
                "fault_family": fault.get("fault_family", "degraded"),
                "evidence": evidence,
                "contributing_metrics": [entry["metric"] for entry in evidence],
                "explanation": (
                    f"{service_id} is an abnormal-service candidate based on "
                    f"{', '.join(entry['metric'] for entry in evidence)}. "
                    "Telemetry and trace/log evidence prioritize this candidate; this does not establish causality."
                ),
                "uncertainty": "causal attribution remains inconclusive; topology and correlated telemetry are supporting evidence, not proof",
                "observed_at": item["observed_at"],
                "anomaly_onset": item.get("anomaly_onset"),
                "trace_refs": sorted({str(span.get("trace_id", "")) for span in error_spans if span.get("trace_id")}),
                # OTLP log records do not carry an Aegis database ID through
                # the prediction contract. Preserve a deterministic source
                # reference from their service, event time, and trace ID.
                "log_refs": sorted({
                    str(entry.get("id") or "log:{service}:{timestamp}:{trace}".format(
                        service=service_id,
                        timestamp=entry.get("timestamp_unix_nano") or entry.get("timestamp", "unknown-time"),
                        trace=entry.get("trace_id") or "no-trace",
                    ))
                    for entry in error_logs
                }),
                "score": direct_score,
            })
        return sorted(ranked, key=lambda item: (-item["ranking_score"], item["node_id"]))

    @staticmethod
    def _correlate_incidents(
        anomalies: List[Dict[str, Any]], graph: nx.DiGraph,
        traces: List[Mapping[str, Any]], logs: List[Mapping[str, Any]],
        correlation_window_seconds: float = 60.0,
    ) -> List[Dict[str, Any]]:
        """Group only anomalies with temporal and direct trace/topology evidence."""
        if not anomalies:
            return []
        parent = list(range(len(anomalies)))

        def find(index: int) -> int:
            while parent[index] != index:
                parent[index] = parent[parent[index]]
                index = parent[index]
            return index

        def union(left: int, right: int) -> None:
            root_left, root_right = find(left), find(right)
            if root_left != root_right:
                parent[root_right] = root_left

        by_trace: Dict[str, set[str]] = {}
        for span in traces:
            trace_id, service_id = str(span.get("trace_id", "")), str(span.get("service_id", ""))
            if trace_id and service_id:
                by_trace.setdefault(trace_id, set()).add(service_id)
        shared_trace_pairs = set()
        for members in by_trace.values():
            for left in members:
                for right in members:
                    if left != right:
                        shared_trace_pairs.add((left, right))
        log_services_by_trace: Dict[str, set[str]] = {}
        for entry in logs:
            trace_id, service_id = str(entry.get("trace_id", "")), str(entry.get("service_id", ""))
            if trace_id and service_id:
                log_services_by_trace.setdefault(trace_id, set()).add(service_id)
        for members in log_services_by_trace.values():
            for left in members:
                for right in members:
                    if left != right:
                        shared_trace_pairs.add((left, right))

        for left in range(len(anomalies)):
            for right in range(left + 1, len(anomalies)):
                a, b = anomalies[left], anomalies[right]
                try:
                    onset_a = datetime.fromisoformat(a.get("anomaly_onset") or a["observed_at"])
                    onset_b = datetime.fromisoformat(b.get("anomaly_onset") or b["observed_at"])
                    close_in_time = abs((onset_a - onset_b).total_seconds()) <= correlation_window_seconds
                except (TypeError, ValueError):
                    close_in_time = False
                if not close_in_time:
                    continue
                svc_a, svc_b = a["service_id"], b["service_id"]
                shares_trace = (svc_a, svc_b) in shared_trace_pairs
                directly_connected = graph.has_edge(svc_a, svc_b) or graph.has_edge(svc_b, svc_a)
                if shares_trace or directly_connected:
                    union(left, right)

        grouped: Dict[int, List[Dict[str, Any]]] = {}
        for index, anomaly in enumerate(anomalies):
            grouped.setdefault(find(index), []).append(anomaly)
        incidents = []
        for members in grouped.values():
            services = sorted(item["service_id"] for item in members)
            onset = min(item.get("anomaly_onset") or item["observed_at"] for item in members)
            identity = hashlib.sha1(("|".join(services) + "|" + onset).encode("utf-8")).hexdigest()[:12]
            incidents.append({
                "incident_id": f"inc-{identity}",
                "started_at": onset,
                "affected_services": services,
                "anomaly_evidence": [item for item in members],
                "trace_ids": sorted({str(span.get("trace_id", "")) for span in traces if span.get("service_id") in services and span.get("trace_id")}),
                "log_refs": [str(entry.get("id", "")) for entry in logs if entry.get("service_id") in services and entry.get("id")],
                "correlation_window_seconds": correlation_window_seconds,
            })
        return sorted(incidents, key=lambda incident: (incident["started_at"], incident["incident_id"]))

    @staticmethod
    def _blast_radius(graph: nx.DiGraph, roots: List[str]) -> List[Dict[str, Any]]:
        """BFS over configured A-calls-B edges; results are potential impact only."""
        paths: List[Dict[str, Any]] = []
        for root in roots:
            if root not in graph:
                continue
            queue = deque([(root, [root])])
            visited = {root}
            while queue:
                node, path = queue.popleft()
                if len(path) > 1:
                    paths.append({
                        "path_nodes": path,
                        "propagation_risk": 0.0,
                        "potential_impact": True,
                    })
                for neighbor in sorted(graph.successors(node)):
                    if neighbor not in visited:
                        visited.add(neighbor)
                        queue.append((neighbor, path + [neighbor]))
        return paths
