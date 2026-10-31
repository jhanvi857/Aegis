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
from serving.incident_models import IncidentModels


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
        self.incident_models, self.incident_models_load_error = IncidentModels.load_configured()

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

        engine_name = "baseline"
        model_version = "service-baseline-v1"
        fallback_reason = self.incident_models_load_error or ""
        calibrated_confidence = 0.0

        if self.incident_models is not None:
            try:
                ml_predictions, coverage = self.incident_models.predict(metrics_by_service)
                if not ml_predictions:
                    fallback_reason = "insufficient service metrics for trained model inference"
                else:
                    scored_services = sorted(
                        ml_predictions.items(),
                        key=lambda item: (-item[1]["root_score"], item[0]),
                    )
                    qualifying = [item for item in scored_services if item[1]["root_score"] >= item[1]["root_threshold"]]
                    if not qualifying and anomalies and scored_services:
                        qualifying = scored_services[:1]

                    ml_root_causes = []
                    for service_id, prediction in qualifying:
                        measured = metrics_by_service.get(service_id, {})
                        ml_evidence = [{"metric": "ml_root_cause_score", "value": round(prediction["root_score"], 4),
                                        "threshold": round(prediction["root_threshold"], 4), "unit": "uncalibrated score",
                                        "source": prediction["root_model"]}]
                        ml_evidence.extend({"metric": metric, "value": value, "threshold": 0.0, "unit": "observed"}
                                           for metric, value in sorted(measured.items()))
                        ml_root_causes.append({
                            "node_id": service_id,
                            "probability": 0.0,
                            "ranking_score": round(prediction["root_score"], 4),
                            "fault_family": prediction["fault_family"],
                            "evidence": ml_evidence,
                            "contributing_metrics": [entry["metric"] for entry in ml_evidence],
                            "explanation": (
                                f"{service_id} ranks as an incident root-service candidate under the trained "
                                f"{prediction['root_model']} model (score {prediction['root_score']:.3f}); "
                                f"the trained {prediction['fault_model']} model predicts {prediction['raw_fault']} "
                                f"(score {prediction['fault_score']:.3f}). This is not calibrated causal confidence."
                            ),
                            "uncertainty": "supervised score trained on RCAEval incident labels; not calibrated and not proof of causality",
                            "trace_refs": [],
                            "log_refs": [],
                            "observed_metrics": measured,
                            "ml_fault_score": round(prediction["fault_score"], 4),
                        })

                    if ml_root_causes:
                        root_causes = ml_root_causes
                        primary = ml_root_causes[0]
                        incidents = [{"incident_id": "ml-ranked-incident", "affected_services": [primary["node_id"]],
                                      "method": "supervised_root_service_model", "uncertainty": primary["uncertainty"]}]
                        propagation = self._blast_radius(graph, [primary["node_id"]])
                        if generate_plan:
                            plan = self.plan_builder.build_plan(
                                graph=graph,
                                root_causes=[primary],
                                propagation_set=[primary["node_id"]],
                                telemetry_by_node=metrics_by_service,
                            )
                        failure_predictions = [
                            {"node_id": s_id, "failure_probability": float(pred["root_score"]),
                             "confidence": 0.0, "estimated_time_to_failure_sec": 0}
                            for s_id, pred in sorted(ml_predictions.items())
                        ]
                        engine_name = f"trained:{self.incident_models.model_type}"
                        model_version = (
                            f"root-{self.incident_models.model_names.get('root_cause', 'unknown')}-"
                            f"fault-{self.incident_models.model_names.get('fault_family', 'unknown')}-v1"
                        )
                        fallback_reason = ""
            except Exception as exc:
                fallback_reason = f"inference fallback: {exc}"
                engine_name = "baseline"

        return {
            "failure_predictions": failure_predictions,
            "root_causes": root_causes,
            "propagation_paths": propagation,
            "recovery_plan": plan,
            "timestamp_unix_nano": int(now.timestamp() * 1e9),
            "engine": engine_name,
            "fallback_reason": fallback_reason,
            "model_version": model_version,
            "calibrated_confidence": calibrated_confidence,
            "detections": detections,
            "incidents": incidents,
            "rca_uncertain": bool(root_causes),
        }

    @staticmethod
    def _valid_metrics(metrics: Dict[str, Any]) -> Dict[str, float]:
        cleaned = {}
        for key, value in metrics.items():
            try:
                numeric = float(value)
            except (TypeError, ValueError):
                continue
            import math
            if math.isfinite(numeric):
                cleaned[key] = numeric
        return cleaned

    @staticmethod
    def _correlate_incidents(
        anomalies: List[Dict[str, Any]],
        graph: nx.DiGraph,
        traces: Optional[List[Dict[str, Any]]] = None,
        logs: Optional[List[Dict[str, Any]]] = None,
    ) -> List[Dict[str, Any]]:
        del traces, logs
        if not anomalies:
            return []
        affected = {item["service_id"] for item in anomalies}
        subgraph = graph.subgraph(affected).to_undirected()
        components = list(nx.connected_components(subgraph))
        grouped = []
        covered = set()
        for comp in components:
            members = sorted(comp)
            covered.update(members)
            digest = hashlib.sha256("::".join(members).encode("utf-8")).hexdigest()[:12]
            grouped.append({"incident_id": f"inc-{digest}", "affected_services": members})
        for item in anomalies:
            sid = item["service_id"]
            if sid not in covered:
                digest = hashlib.sha256(sid.encode("utf-8")).hexdigest()[:12]
                grouped.append({"incident_id": f"inc-{digest}", "affected_services": [sid]})
                covered.add(sid)
        return grouped

    def _rank_candidates(
        self,
        candidates: List[Dict[str, Any]],
        telemetry: Dict[str, Dict[str, float]],
        graph: nx.DiGraph,
        target_node_id: Optional[str] = None,
        traces: Optional[List[Dict[str, Any]]] = None,
        logs: Optional[List[Dict[str, Any]]] = None,
    ) -> List[Dict[str, Any]]:
        ranked = []
        total_nodes = max(graph.number_of_nodes(), 1)
        for item in candidates:
            service_id = item["service_id"]
            obs = telemetry.get(service_id, {})
            diagnosis = diagnose_fault(service_id, obs)
            reachable = nx.descendants(graph, service_id) if service_id in graph else set()
            topological_depth = len(reachable) / total_nodes
            temporal_lead = 1.0 if any(e.get("source") == "derived_from_trace_status" for e in item["evidence"]) else 0.5
            target_boost = 1.0 if target_node_id and service_id == target_node_id else 0.0
            score = 0.4 * item["score"] + 0.3 * topological_depth + 0.2 * temporal_lead + 0.1 * target_boost
            service_traces = [
                {"trace_id": str(t.get("trace_id", "")), "span_id": str(t.get("span_id", "")),
                 "service_id": service_id, "timestamp_unix_nano": int(t.get("timestamp_unix_nano", 0) or 0),
                 "latency_ms": float(t.get("duration_ms", 0) or 0), "status_code": int(t.get("status_code", 0) or 0)}
                for t in (traces or [])
                if str(t.get("service_id", "")) == service_id
            ][:5]
            service_logs = [
                {"log_id": str(l.get("log_id", "")), "service_id": service_id,
                 "timestamp_unix_nano": int(l.get("timestamp_unix_nano", 0) or 0),
                 "severity": str(l.get("severity", "INFO")), "message": str(l.get("message", ""))}
                for l in (logs or [])
                if str(l.get("service_id", "")) == service_id
            ][:5]
            ranked.append({
                "node_id": service_id,
                "probability": 0.0,
                "ranking_score": round(score, 4),
                "fault_family": diagnosis["fault_family"],
                "evidence": item["evidence"],
                "contributing_metrics": [e["metric"] for e in item["evidence"]],
                "explanation": (
                    f"{service_id} flagged with {len(item['evidence'])} metric breaches; "
                    f"ranked by topology, metric severity, and trace onset. Statistical ranking only; "
                    f"does not establish causality."
                ),
                "uncertainty": "inconclusive: multiple upstream and downstream nodes may share correlated symptoms",
                "trace_refs": service_traces,
                "log_refs": service_logs,
                "observed_metrics": obs,
                "diagnosis_confidence": diagnosis["confidence"],
            })
        ranked.sort(key=lambda r: (-r["ranking_score"], r["node_id"]))
        return ranked

    @staticmethod
    def _blast_radius(graph: nx.DiGraph, root_nodes: List[str]) -> List[Dict[str, Any]]:
        paths = []
        for root in root_nodes:
            if root not in graph:
                continue
            for target in nx.descendants(graph, root):
                try:
                    for path in nx.all_simple_paths(graph, root, target, cutoff=4):
                        paths.append({"path": path, "path_nodes": path, "probability": 0.0, "potential_impact": True, "propagation_risk": 0})
                except nx.NetworkXError:
                    continue
        paths.sort(key=lambda item: (len(item["path"]), item["path"]))
        return paths
