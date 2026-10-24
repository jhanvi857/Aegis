"""
gRPC / RPC Server serving PredictionService to Go control plane (Phase 3)
Integrates FailurePredictor, RootCauseAnalyzer, and PropagationPredictor
into a unified low-latency prediction endpoint conforming to proto/predict.proto.
"""

import os
import sys
import time
import logging
from concurrent import futures
from typing import Dict, Any, Optional

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from tasks.failure_prediction import FailurePredictor
from tasks.root_cause import RootCauseAnalyzer
from tasks.propagation_prediction import PropagationPredictor
from recovery_planner.plan_builder import RecoveryPlanBuilder
import networkx as nx

logger = logging.getLogger(__name__)


class PredictionEngine:
    """
    Unified ML inference orchestrator for Phase 3 and Phase 4.
    Supports dynamic N topologies via two-stage pipeline (sparse TGNN)
    for graphs > AEGIS_SPARSE_THRESHOLD (default 20), and dense TGNN for small graphs.
    """

    def __init__(self, checkpoint_path: str = "python-ml/models/checkpoints/tgnn_best.pt"):
        self.sparse_threshold = int(os.getenv("AEGIS_SPARSE_THRESHOLD", "20"))
        self.failure_predictor = FailurePredictor(model_checkpoint=checkpoint_path)
        self.root_cause_analyzer = RootCauseAnalyzer(model_checkpoint=checkpoint_path)
        self.propagation_predictor = PropagationPredictor(model_checkpoint=checkpoint_path)
        self.plan_builder = RecoveryPlanBuilder()

        # Load Two-Stage Sparse-TGNN Pipeline
        from models.two_stage_pipeline import TwoStageInferencePipeline
        self.two_stage_pipeline = TwoStageInferencePipeline(
            node_in_features=8,
            hidden_dim=32,
            k_hops=2,
            anomaly_threshold=0.35,
        )

        # Phase 6: Optional RL Agent policy network
        self.rl_policy = None
        try:
            from rl_agent.policy import RecoveryPolicyNetwork
            rl_ckpt = "python-ml/models/checkpoints/rl_policy_best.pt"
            if os.path.exists(rl_ckpt):
                self.rl_policy = RecoveryPolicyNetwork(state_dim=25, action_dim=21)
                self.rl_policy.load(rl_ckpt)
        except Exception:
            self.rl_policy = None

    def predict_rl_action(self, state_vector: Any, epsilon: float = 0.0) -> int:
        """Selects optimal remediation action from trained RL policy network (Phase 6)."""
        if self.rl_policy is not None:
            return self.rl_policy.select_action(state_vector, epsilon=epsilon)
        return 0

    def predict(
        self,
        graph_representation: Dict[str, Any],
        telemetry_batch: Optional[Dict[str, Any]] = None,
        target_node_id: Optional[str] = None,
        generate_plan: bool = True,
    ) -> Dict[str, Any]:
        """
        Executes unified TGNN inference:
        - For graphs with > AEGIS_SPARSE_THRESHOLD nodes: Two-Stage Pipeline (Stage 1 O(N) anomaly filter + Stage 2 Sparse TGNN on k-hop neighborhood).
        - For small graphs: Dense TGNN.
        """
        now_ns = int(time.time() * 1e9)
        node_ids = graph_representation.get("node_ids", [])
        if not node_ids:
            snapshot_nodes = graph_representation.get("snapshot", {}).get("nodes", [])
            node_ids = [n.get("id") for n in snapshot_nodes if n.get("id")]

        use_sparse = len(node_ids) > self.sparse_threshold

        if use_sparse:
            import torch
            # 1. Build adjacency maps
            adj_out: Dict[str, List[str]] = {nid: [] for nid in node_ids}
            adj_in: Dict[str, List[str]] = {nid: [] for nid in node_ids}
            snapshot_edges = graph_representation.get("snapshot", {}).get("edges", [])
            for e in snapshot_edges:
                src, tgt = e.get("source"), e.get("target")
                if src in adj_out and tgt in adj_out:
                    adj_out[src].append(tgt)
                if tgt in adj_in and src in adj_in:
                    adj_in[tgt].append(src)

            # 2. Build latest_metrics
            latest_metrics: Dict[str, Dict[str, float]] = {}
            snapshot_nodes = graph_representation.get("snapshot", {}).get("nodes", [])
            for snode in snapshot_nodes:
                nid = snode.get("id")
                if nid:
                    m = snode.get("metrics", {})
                    latest_metrics[nid] = {
                        "cpu": float(m.get("cpu", 20.0)),
                        "memory": float(m.get("memory", 30.0)),
                        "latency": float(m.get("latency_ms", m.get("latency", 20.0))),
                        "error_rate": float(m.get("error_rate", 0.001)),
                    }

            if telemetry_batch:
                for metric in telemetry_batch.get("metrics", []):
                    svc = metric.get("service_id")
                    name = metric.get("metric_name")
                    val = metric.get("value")
                    if svc in latest_metrics and name and val is not None:
                        if name in ["cpu", "cpu_usage", "cpu_usage_percent"]:
                            latest_metrics[svc]["cpu"] = float(val)
                        elif name in ["memory", "memory_usage", "memory_usage_percent"]:
                            latest_metrics[svc]["memory"] = float(val)
                        elif name in ["latency", "latency_ms", "p95_latency_ms", "p99_latency_ms"]:
                            latest_metrics[svc]["latency"] = float(val)
                        elif name in ["error_rate", "errorRate"]:
                            latest_metrics[svc]["error_rate"] = float(val)

            # 3. Build node_features_seq
            feature_matrix = graph_representation.get("feature_matrix", {})
            feature_list = []
            for nid in node_ids:
                if nid in feature_matrix:
                    feature_list.append(feature_matrix[nid])
                else:
                    lm = latest_metrics.get(nid, {})
                    feature_list.append([
                        lm.get("cpu", 20.0) / 100.0,
                        lm.get("memory", 30.0) / 100.0,
                        lm.get("latency", 20.0) / 1000.0,
                        lm.get("error_rate", 0.001),
                        float(len(adj_in.get(nid, []))),
                        float(len(adj_out.get(nid, []))),
                        0.5,
                        0.1,
                    ])
            feats_tensor = torch.tensor(feature_list, dtype=torch.float32).unsqueeze(0).repeat(5, 1, 1)

            # 4. Predict with TwoStageInferencePipeline
            two_stage_res = self.two_stage_pipeline.predict(
                service_ids=node_ids,
                node_features_seq=feats_tensor,
                adj_out=adj_out,
                adj_in=adj_in,
                latest_metrics=latest_metrics,
            )

            fail_prob = float(two_stage_res["failure_probability"])
            confidence = float(two_stage_res["confidence"])
            is_anomalous = bool(two_stage_res["is_anomalous"])
            root_cause_node = two_stage_res["root_cause"]
            root_cause_scores = two_stage_res.get("root_cause_scores", {})

            failure_preds = []
            for nid in node_ids:
                rc_score = root_cause_scores.get(nid, 0.0)
                node_fail_p = fail_prob if (nid == root_cause_node or rc_score > 0.3) else (fail_prob * 0.15 if is_anomalous else 0.05)
                ttf = 30 if node_fail_p > 0.5 else 600
                failure_preds.append({
                    "node_id": nid,
                    "failure_probability": float(node_fail_p),
                    "confidence": confidence,
                    "estimated_time_to_failure_sec": ttf,
                })

            root_causes = []
            if is_anomalous and root_cause_node:
                rc_m = latest_metrics.get(root_cause_node, {})
                root_causes.append({
                    "node_id": root_cause_node,
                    "probability": float(root_cause_scores.get(root_cause_node, 0.95)),
                    "explanation": f"Sparse TGNN localized anomaly on {root_cause_node} (CPU: {rc_m.get('cpu', 0)}%, Latency: {rc_m.get('latency', 0)}ms, ErrorRate: {rc_m.get('error_rate', 0)})",
                    "contributing_metrics": [k for k, v in rc_m.items() if (k != "error_rate" and v > 50) or (k == "error_rate" and v > 0.05)],
                    "fault_family": "cpu_stress" if rc_m.get("cpu", 0) > 80 else ("latency" if rc_m.get("latency", 0) > 200 else "degraded"),
                    "evidence": [
                        {"metric": "cpu", "value": float(rc_m.get("cpu", 0.0)), "threshold": 75.0, "unit": "%"},
                        {"metric": "latency", "value": float(rc_m.get("latency", 0.0)), "threshold": 250.0, "unit": "ms"},
                    ],
                })

            propagation_paths = []
            for pe in two_stage_res.get("propagation_edges", []):
                propagation_paths.append({
                    "path_nodes": [pe["source"], pe["target"]],
                    "propagation_risk": float(pe["probability"]),
                })

            plan = None
            if generate_plan and root_causes:
                g = nx.DiGraph()
                for nid in node_ids:
                    g.add_node(nid, telemetry=latest_metrics.get(nid, {}), metrics=latest_metrics.get(nid, {}))
                for src, tgts in adj_out.items():
                    for tgt in tgts:
                        g.add_edge(src, tgt)

                all_prop_nodes = []
                for p in propagation_paths:
                    all_prop_nodes.extend(p.get("path_nodes", []))

                plan = self.plan_builder.build_plan(
                    graph=g,
                    root_causes=root_causes,
                    propagation_set=list(set(all_prop_nodes)),
                    telemetry_by_node=latest_metrics,
                )

            return {
                "failure_predictions": failure_preds,
                "root_causes": root_causes,
                "propagation_paths": propagation_paths,
                "recovery_plan": plan,
                "timestamp_unix_nano": now_ns,
                "engine": "sparse_two_stage",
                "model_version": self.two_stage_pipeline.version,
                "calibrated_confidence": confidence,
            }

        # 1. Failure predictions
        failure_preds = self.failure_predictor.predict(
            graph_representation=graph_representation,
            telemetry_batch=telemetry_batch,
        )

        # 2. Root causes
        root_causes = self.root_cause_analyzer.analyze(
            graph_representation=graph_representation,
            telemetry_batch=telemetry_batch,
            top_k=3,
        )

        # 3. Propagation paths for primary root cause
        primary_rc = root_causes[0]["node_id"] if root_causes else (target_node_id or "node-a")
        propagation_paths = self.propagation_predictor.predict_paths(
            graph_representation=graph_representation,
            root_cause_node=primary_rc,
        )

        # 4. Optional recovery plan generation
        plan = None
        if generate_plan:
            telemetry_by_node: Dict[str, Dict[str, float]] = {}

            snapshot_nodes = graph_representation.get("snapshot", {}).get("nodes", [])
            for snode in snapshot_nodes:
                nid = snode.get("id")
                if nid and "metrics" in snode:
                    telemetry_by_node[nid] = {str(k): float(v) for k, v in snode["metrics"].items()}

            if telemetry_batch:
                for metric in telemetry_batch.get("metrics", []):
                    svc = metric.get("service_id")
                    name = metric.get("metric_name")
                    val = metric.get("value")
                    if svc and name and val is not None:
                        if svc not in telemetry_by_node:
                            telemetry_by_node[svc] = {}
                        telemetry_by_node[svc][name] = float(val)
                        if name == "cpu_usage":
                            telemetry_by_node[svc]["cpu_usage_percent"] = float(val)
                        elif name == "memory_usage":
                            telemetry_by_node[svc]["memory_usage_percent"] = float(val)
                        elif name in ["p95_latency_ms", "p99_latency_ms"]:
                            telemetry_by_node[svc]["latency_p99_ms"] = float(val)
                            telemetry_by_node[svc]["p99_latency_ms"] = float(val)

            g = nx.DiGraph()
            node_ids_g = graph_representation.get("node_ids", [])
            for nid in node_ids_g:
                node_telem = telemetry_by_node.get(nid, {})
                g.add_node(nid, telemetry=node_telem, metrics=node_telem)

            edge_sources = graph_representation.get("edge_sources", [])
            edge_targets = graph_representation.get("edge_targets", [])
            for src, tgt in zip(edge_sources, edge_targets):
                g.add_edge(src, tgt)

            all_prop_nodes = []
            for p in propagation_paths:
                all_prop_nodes.extend(p.get("path_nodes", []))

            plan = self.plan_builder.build_plan(
                graph=g,
                root_causes=root_causes,
                propagation_set=list(set(all_prop_nodes)),
                telemetry_by_node=telemetry_by_node,
            )

        calibrated_conf = 0.95
        if failure_preds:
            calibrated_conf = float(failure_preds[0].get("confidence", 0.95))

        return {
            "failure_predictions": failure_preds,
            "root_causes": root_causes,
            "propagation_paths": propagation_paths,
            "recovery_plan": plan,
            "timestamp_unix_nano": now_ns,
            "engine": "dense_tgnn",
            "model_version": "1.0.0-dense",
            "calibrated_confidence": calibrated_conf,
        }


try:
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

                if request.graph and request.graph.snapshot:
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
                    node_ids = list(self.engine.failure_predictor.node_ids) or ["gateway", "node-a", "node-b", "node-c", "node-d"]

                # 2. Build feature matrix [cpu, mem, lat, err, in_deg, out_deg, centrality, queue_depth]
                in_degrees = {nid: 0 for nid in node_ids}
                out_degrees = {nid: 0 for nid in node_ids}
                for e in snapshot_edges:
                    src, tgt = e.get("source"), e.get("target")
                    if src in out_degrees:
                        out_degrees[src] += 1
                    if tgt in in_degrees:
                        in_degrees[tgt] += 1

                centrality_scores = dict(request.graph.centrality_scores) if (request.graph and request.graph.centrality_scores) else {}
                total_nodes = max(1, len(node_ids))

                feature_matrix = {}
                for snode in snapshot_nodes:
                    nid = snode["id"]
                    m = snode.get("metrics", {})
                    cpu = m.get("cpu", 20.0) / 100.0
                    mem = m.get("memory", 30.0) / 100.0
                    lat = m.get("latency_ms", m.get("latency", 20.0)) / 1000.0
                    err = m.get("error_rate", 0.001)
                    in_d = float(in_degrees.get(nid, 0))
                    out_d = float(out_degrees.get(nid, 0))
                    cent = float(centrality_scores.get(nid, (in_d + out_d) / total_nodes))
                    q_depth = m.get("queue_depth", 1.0) / 10.0
                    feature_matrix[nid] = [cpu, mem, lat, err, in_d, out_d, cent, q_depth]

                for nid in node_ids:
                    if nid not in feature_matrix:
                        feature_matrix[nid] = [0.2, 0.3, 0.02, 0.001, 1.0, 1.0, 0.5, 0.1]

                edge_sources = [e["source"] for e in snapshot_edges]
                edge_targets = [e["target"] for e in snapshot_edges]

                graph_rep = {
                    "node_ids": node_ids,
                    "feature_matrix": feature_matrix,
                    "snapshot": {
                        "nodes": snapshot_nodes,
                        "edges": snapshot_edges,
                    },
                    "edge_sources": edge_sources,
                    "edge_targets": edge_targets,
                    "centrality_scores": centrality_scores,
                }

                # 3. Telemetry batch
                telem_batch = {"metrics": [], "logs": [], "traces": []}
                if request.recent_telemetry:
                    for m in request.recent_telemetry.metrics:
                        telem_batch["metrics"].append({
                            "service_id": m.service_id,
                            "metric_name": m.metric_name,
                            "value": m.value,
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
                            confidence=float(fp.get("confidence", 0.95)),
                            estimated_time_to_failure_sec=int(fp.get("estimated_time_to_failure_sec", 600)),
                        )
                    )
                for rc in result.get("root_causes", []):
                    rc_pb = predict_pb2.RootCause(
                        node_id=str(rc.get("node_id", "")),
                        probability=float(rc.get("probability", 0.0)),
                        explanation=str(rc.get("explanation", "")),
                        contributing_metrics=list(rc.get("contributing_metrics", [])),
                        fault_family=str(rc.get("fault_family", "nominal")),
                    )
                    for ev in rc.get("evidence", []):
                        rc_pb.evidence.append(
                            predict_pb2.Evidence(
                                metric=str(ev.get("metric", "")),
                                observed_value=float(ev.get("value", 0.0)),
                                threshold_value=float(ev.get("threshold", 0.0)),
                                unit=str(ev.get("unit", "")),
                            )
                        )
                    resp.root_causes.append(rc_pb)

                for pp in result.get("propagation_paths", []):
                    resp.propagation_paths.append(
                        predict_pb2.PropagationPath(
                            path_nodes=list(pp.get("path_nodes", [])),
                            propagation_risk=float(pp.get("propagation_risk", 0.0)),
                        )
                    )

                # Build edge latency lookup for dynamic propagation ETA calculation
                edge_latencies = {}
                for e in snapshot_edges:
                    src = e.get("source")
                    tgt = e.get("target")
                    if src and tgt:
                        edge_latencies[(src, tgt)] = float(e.get("latency_ms", 25.0))

                # 5b. Compute affected_nodes with hop distance and dynamic cumulative ETA
                visited_nodes = {}
                node_etas = {}
                for pp in result.get("propagation_paths", []):
                    nodes_list = pp.get("path_nodes", [])
                    cum_latency = 0.0
                    for hop, nid in enumerate(nodes_list):
                        if hop > 0:
                            prev_nid = nodes_list[hop - 1]
                            edge_lat = edge_latencies.get((prev_nid, nid), 25.0)
                            cum_latency += edge_lat

                        eta = 0 if hop == 0 else max(1, int(round(cum_latency / 50.0)))

                        if nid not in visited_nodes or hop < visited_nodes[nid]:
                            visited_nodes[nid] = hop
                            node_etas[nid] = eta

                for nid, hop in visited_nodes.items():
                    resp.affected_nodes.append(
                        predict_pb2.AffectedNode(
                            node_id=nid,
                            hop_distance=hop,
                            eta_seconds=node_etas.get(nid, hop * 5),
                            risk_score=max(0.2, 0.95 - (hop * 0.2)),
                        )
                    )

                # 5c. Populate recommended_actions from RecoveryPlan
                plan = result.get("recovery_plan") or {}
                if plan:
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
                                confidence=0.95,
                                parameters={str(k): str(v) for k, v in act.get("parameters", {}).items()},
                            )
                        )

                resp.timestamp_unix_nano = int(result.get("timestamp_unix_nano", int(time.time() * 1e9)))
                resp.engine = str(result.get("engine", "dense_tgnn"))
                resp.model_version = str(result.get("model_version", "1.0.0-dense"))
                resp.calibrated_confidence = float(result.get("calibrated_confidence", 0.95))
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
                    all_prop.extend(p.path_nodes)

                g = nx.DiGraph()
                for rc in root_causes:
                    g.add_node(rc["node_id"])
                for p in request.propagation_paths:
                    for i in range(len(p.path_nodes) - 1):
                        g.add_edge(p.path_nodes[i], p.path_nodes[i + 1])

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
            return recovery_pb2.RecoveryResult(
                plan_id=request.plan_id,
                success=True,
                executed_action_ids=[a.action_id for a in request.actions],
                details="Executed via decision engine plan runner",
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
    logger.info("[grpc-server] TGNN Engine loaded and ready for inference requests.")

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
    port_env = os.getenv("PORT", "50051")
    serve(port=int(port_env))
