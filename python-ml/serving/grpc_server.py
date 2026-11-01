"""
gRPC server providing baseline detection and evidence-based RCA to the Go control plane.
"""

import os
import sys
import time
import logging
from datetime import datetime, timezone
from concurrent import futures
from typing import Dict, Any, Optional, List

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

import networkx as nx

logger = logging.getLogger(__name__)


from serving.baseline_engine import PredictionEngine as PredictionEngine


try:
    import grpc
    from shared.pb import predict_pb2, predict_pb2_grpc, recovery_pb2, recovery_pb2_grpc

    class PredictionServiceServicer(predict_pb2_grpc.PredictionServiceServicer):
        def __init__(self, engine: PredictionEngine):
            self.engine = engine

        def Predict(self, request, context):
            try:
                # 1. Extract nodes and edges from request.graph
                node_ids = []
                snapshot_nodes = []
                snapshot_edges = []

                if request.HasField("graph") and request.graph.snapshot:
                    for n in request.graph.snapshot.nodes:
                        node_ids.append(n.id)
                        snapshot_nodes.append({
                            "id": n.id,
                            "name": n.name,
                            "type": n.type,
                            "status": n.status,
                            "metrics": dict(n.metrics),
                        })
                    for e in request.graph.snapshot.edges:
                        snapshot_edges.append({
                            "source": e.source,
                            "target": e.target,
                            "latency_ms": e.latency_ms,
                            "error_rate": e.error_rate,
                            "throughput": e.throughput,
                        })

                if not node_ids:
                    resp = predict_pb2.PredictResponse()
                    resp.timestamp_unix_nano = int(time.time() * 1e9)
                    resp.engine = "statistical_baseline"
                    resp.model_version = "service-baseline-v1"
                    resp.calibrated_confidence = 0.0
                    return resp

                # Do not synthesize feature values for missing telemetry.
                graph_rep = {
                    "node_ids": node_ids,
                    "snapshot": {
                        "nodes": snapshot_nodes,
                        "edges": snapshot_edges,
                    },
                }

                # 3. Telemetry batch
                telem_batch = {"metrics": [], "logs": [], "traces": []}
                recent_telem = request.telemetry if request.HasField("telemetry") else None
                if recent_telem:
                    for m in recent_telem.metrics:
                        telem_batch["metrics"].append({
                            "service_id": m.service_id,
                            "metric_name": m.metric_name,
                            "value": m.value,
                            "timestamp_unix_nano": m.timestamp_unix_nano,
                            "unit": m.unit,
                        })
                    for entry in recent_telem.logs:
                        telem_batch["logs"].append({
                            "service_id": entry.service_id,
                            "level": entry.level,
                            "message": entry.message,
                            "trace_id": entry.trace_id,
                            "timestamp_unix_nano": entry.timestamp_unix_nano,
                            "attributes": dict(entry.attributes),
                        })
                    for span in recent_telem.traces:
                        telem_batch["traces"].append({
                            "trace_id": span.trace_id,
                            "span_id": span.span_id,
                            "parent_span_id": span.parent_span_id,
                            "service_id": span.service_id,
                            "operation_name": span.operation_name,
                            "start_time_unix_nano": span.start_time_unix_nano,
                            "end_time_unix_nano": span.end_time_unix_nano,
                            "status_code": span.status_code,
                            "attributes": dict(span.attributes),
                        })

                # 4. Predict
                target_node = request.target_node_id or None
                result = self.engine.predict(
                    graph_representation=graph_rep,
                    telemetry_batch=telem_batch,
                    target_node_id=target_node,
                    generate_plan=True,
                )

                # 5. Build response
                resp = predict_pb2.PredictResponse()
                for fp in result.get("failure_predictions", []):
                    resp.failure_predictions.append(
                        predict_pb2.FailurePrediction(
                            node_id=str(fp.get("node_id", "")),
                            failure_probability=float(fp.get("failure_probability", 0.0)),
                            confidence=float(fp.get("confidence", 0.0)),
                            estimated_time_to_failure_sec=int(fp.get("estimated_time_to_failure_sec", 0)),
                        )
                    )
                for rc in result.get("root_causes", []):
                    if rc.get("fault_family") == "nominal" or not rc.get("node_id"):
                        continue
                    pb_trace_refs = []
                    for tr in rc.get("trace_refs", []):
                        if isinstance(tr, str):
                            pb_trace_refs.append(predict_pb2.TraceRef(trace_id=tr))
                        elif isinstance(tr, dict):
                            pb_trace_refs.append(predict_pb2.TraceRef(
                                trace_id=str(tr.get("trace_id", "")),
                                span_id=str(tr.get("span_id", "")),
                                service_id=str(tr.get("service_id", "")),
                                timestamp_unix_nano=int(tr.get("timestamp_unix_nano", 0)),
                                latency_ms=float(tr.get("latency_ms", 0.0)),
                                status_code=int(tr.get("status_code", 0)),
                            ))
                    pb_log_refs = []
                    for lr in rc.get("log_refs", []):
                        if isinstance(lr, str):
                            pb_log_refs.append(predict_pb2.LogRef(log_id=lr))
                        elif isinstance(lr, dict):
                            pb_log_refs.append(predict_pb2.LogRef(
                                log_id=str(lr.get("log_id", "")),
                                service_id=str(lr.get("service_id", "")),
                                timestamp_unix_nano=int(lr.get("timestamp_unix_nano", 0)),
                                severity=str(lr.get("severity", "")),
                                message=str(lr.get("message", "")),
                            ))

                    rc_pb = predict_pb2.RootCause(
                        node_id=str(rc.get("node_id", "")),
                        probability=float(rc.get("probability", 0.0)),
                        explanation=str(rc.get("explanation", "")),
                        contributing_metrics=list(rc.get("contributing_metrics", [])),
                        fault_family=str(rc.get("fault_family", "nominal")),
                        uncertainty=str(rc.get("uncertainty", "RCA confidence is uncalibrated.")),
                        ranking_score=float(rc.get("ranking_score", 0.0)),
                        trace_refs=pb_trace_refs,
                        log_refs=pb_log_refs,
                    )
                    for ev in rc.get("evidence", []):
                        rc_pb.evidence.append(
                            predict_pb2.MetricEvidence(
                                metric=str(ev.get("metric", "")),
                                value=float(ev.get("value", 0.0)),
                                threshold=float(ev.get("threshold", 0.0)),
                                unit=str(ev.get("unit", "")),
                                source=str(ev.get("source", "")),
                                sample_count=int(ev.get("sample_count", 1)),
                            )
                        )
                    resp.root_causes.append(rc_pb)

                for pp in result.get("propagation_paths", []):
                    path_list = pp.get("path") or pp.get("path_nodes") or []
                    prob = float(pp.get("probability") or pp.get("propagation_risk") or 0.0)
                    resp.propagation_paths.append(
                        predict_pb2.PropagationPath(
                            path=[str(n) for n in path_list],
                            probability=prob,
                        )
                    )

                # Build edge latency lookup for dynamic propagation ETA calculation
                edge_latencies = {}
                for e in snapshot_edges:
                    src = e.get("source")
                    tgt = e.get("target")
                    if src and tgt:
                        edge_latencies[(src, tgt)] = float(e.get("latency_ms", 0.0))

                # 5b. Compute affected_nodes
                visited_nodes = {}
                for pp in result.get("propagation_paths", []):
                    nodes_list = pp.get("path_nodes") or pp.get("path") or []
                    for hop, nid in enumerate(nodes_list):
                        if nid not in visited_nodes or hop < visited_nodes[nid]:
                            visited_nodes[nid] = hop

                for nid, hop in visited_nodes.items():
                    resp.affected_nodes.append(
                        predict_pb2.AffectedNode(
                            node_id=nid,
                            status="at_risk" if hop > 0 else "impacted",
                            reason=f"hop distance {hop}",
                        )
                    )

                # 5c. Populate recommended_actions from RecoveryPlan
                plan = result.get("recovery_plan") or {}
                has_anom = any(rc.get("fault_family") != "nominal" for rc in result.get("root_causes", []))
                if plan and has_anom:
                    risk_info = plan.get("risk", {})
                    for act in plan.get("actions", []):
                        act_type = act.get("action_type", "RESTART")
                        target = act.get("target_node_id", "")
                        rec_risk_level = act.get("risk_level", risk_info.get("risk_level", "LOW"))
                        req_approval = (rec_risk_level.upper() in ["MEDIUM", "HIGH"])
                        resp.recommended_actions.append(
                            predict_pb2.ActionRecommendation(
                                action_id=str(act.get("action_id", "")),
                                action_type=str(act_type),
                                target_node_id=str(target),
                                title=f"{act_type} on {target}",
                                description=f"Autonomous remediation: execute {act_type} on {target}",
                                risk_level=str(rec_risk_level),
                                requires_approval=bool(req_approval),
                                blast_radius_services=[str(s) for s in act.get("blast_radius", [])],
                                parameters={str(k): str(v) for k, v in act.get("parameters", {}).items()},
                            )
                        )

                resp.timestamp_unix_nano = int(result.get("timestamp_unix_nano", int(time.time() * 1e9)))
                resp.engine = str(result.get("engine", "statistical_baseline"))
                resp.model_version = str(result.get("model_version", "service-baseline-v1"))
                resp.calibrated_confidence = float(result.get("calibrated_confidence", 0.0))
                resp.fallback_reason = str(result.get("fallback_reason", ""))
                return resp
            except Exception as e:
                logger.error(f"[grpc-server] Error handling Predict: {e}", exc_info=True)
                context.set_code(grpc.StatusCode.INTERNAL)
                context.set_details(str(e))
                return predict_pb2.PredictResponse()

    class RecoveryServiceServicer(recovery_pb2_grpc.RecoveryServiceServicer):
        def __init__(self, engine: PredictionEngine):
            self.engine = engine

        def GeneratePlan(self, request, context):
            try:
                # Map PredictResponse into RecoveryPlan
                root_causes = []
                for rc in request.root_causes:
                    root_causes.append({
                        "node_id": rc.node_id,
                        "probability": rc.probability,
                        "explanation": rc.explanation,
                    })

                all_prop = []
                for p in request.propagation_paths:
                    p_nodes = list(p.path)
                    all_prop.extend(p_nodes)

                g = nx.DiGraph()
                for rc in root_causes:
                    g.add_node(rc["node_id"])
                for p in request.propagation_paths:
                    p_nodes = list(p.path)
                    for i in range(len(p_nodes) - 1):
                        g.add_edge(p_nodes[i], p_nodes[i + 1])

                plan_data = self.engine.plan_builder.build_plan(
                    graph=g,
                    root_causes=root_causes,
                    propagation_set=list(set(all_prop)),
                )

                resp = recovery_pb2.RecoveryPlan()
                resp.plan_id = plan_data.get("plan_id", "plan-1")
                resp.created_at_unix_nano = plan_data.get("created_at_unix_nano", int(time.time() * 1e9))

                risk_info = plan_data.get("risk", {})
                resp.risk.risk_level = risk_info.get("risk_level", "LOW")
                resp.risk.risk_score = float(risk_info.get("risk_score", 0.2))
                resp.risk.requires_approval = bool(risk_info.get("requires_approval", False))
                resp.risk.justification = str(risk_info.get("justification", ""))

                action_type_map = {
                    "RESTART": recovery_pb2.ActionType.RESTART,
                    "SCALE_UP": recovery_pb2.ActionType.SCALE_UP,
                    "SCALE_DOWN": recovery_pb2.ActionType.SCALE_DOWN,
                    "REROUTE_TRAFFIC": recovery_pb2.ActionType.REROUTE_TRAFFIC,
                    "FLUSH_CACHE": recovery_pb2.ActionType.FLUSH_CACHE,
                    "INCREASE_POOL": recovery_pb2.ActionType.INCREASE_POOL,
                    "ISOLATE": recovery_pb2.ActionType.ISOLATE,
                }

                for act in plan_data.get("actions", []):
                    act_type_str = act.get("action_type", "RESTART").upper()
                    act_enum = action_type_map.get(act_type_str, recovery_pb2.ActionType.RESTART)
                    rec_act = recovery_pb2.RecoveryAction(
                        action_id=str(act.get("action_id", "")),
                        action_type=act_enum,
                        target_node_id=str(act.get("target_node_id", "")),
                        execution_order=int(act.get("execution_order", 1)),
                    )
                    for k, v in act.get("parameters", {}).items():
                        rec_act.parameters[k] = str(v)
                    resp.actions.append(rec_act)

                return resp
            except Exception as e:
                logger.error(f"[grpc-server] Error handling GeneratePlan: {e}", exc_info=True)
                context.set_code(grpc.StatusCode.INTERNAL)
                context.set_details(str(e))
                return recovery_pb2.RecoveryPlan()

        def ExecutePlan(self, request, context):
            context.set_code(grpc.StatusCode.UNIMPLEMENTED)
            context.set_details(
                "Python serves recommendations only; actions must pass through the authorized Go recovery engine"
            )
            return recovery_pb2.RecoveryResult(
                plan_id=request.plan_id,
                success=False,
                executed_action_ids=[],
                details="Not executed",
                completed_at_unix_nano=int(time.time() * 1e9),
            )

except ImportError as e:
    logger.warning(f"[grpc-server] Could not import protobuf stubs: {e}")
    PredictionServiceServicer = None
    RecoveryServiceServicer = None


def serve(port: int = 50051):
    """Starts PredictionService gRPC server or fallback HTTP/socket listener."""
    logger.info(f"[grpc-server] Aegis ML Inference Server initializing on port {port}...")
    engine = PredictionEngine()
    logger.info("[grpc-server] Shared statistical detection and graph RCA engine ready.")

    # Graceful gRPC initialization
    try:
        import grpc
        if PredictionServiceServicer is not None:
            server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
            predict_pb2_grpc.add_PredictionServiceServicer_to_server(PredictionServiceServicer(engine), server)
            recovery_pb2_grpc.add_RecoveryServiceServicer_to_server(RecoveryServiceServicer(engine), server)
            server.add_insecure_port(f"[::]:{port}")
            server.start()
            logger.info(f"[grpc-server] gRPC server listening on port {port} with PredictionService and RecoveryService registered")
            server.wait_for_termination()
        else:
            logger.warning(f"[grpc-server] Protobuf gRPC stubs not compiled; server in standby mode on port {port}.")
            while True:
                time.sleep(3600)
    except ImportError:
        logger.info(f"[grpc-server] grpc package not found; engine operating in direct in-memory Python binding mode.")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    port_env = os.getenv("AEGIS_GRPC_PORT", os.getenv("PORT", "50051"))
    serve(port=int(port_env))
