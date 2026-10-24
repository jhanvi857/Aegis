package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	graphpb "github.com/aegis/go-services/shared/pb/graph"
	predictpb "github.com/aegis/go-services/shared/pb/predict"
	telemetrypb "github.com/aegis/go-services/shared/pb/telemetry"
)

func GetLatestPredictionHandler(w http.ResponseWriter, r *http.Request) {
	pred := ComputePredictionOutput()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pred)
}

func ComputePredictionOutput() PredictionOutput {
	State.mu.RLock()
	defer State.mu.RUnlock()

	// 1. Determine primary target / root cause candidate
	var targetServiceID string
	// Deterministically pick active fault by startedAt
	var latestFault *ChaosInjection
	for _, f := range State.ActiveFaults {
		if latestFault == nil || f.StartedAt > latestFault.StartedAt {
			latestFault = f
		}
	}
	if latestFault != nil {
		targetServiceID = latestFault.TargetServiceID
	}

	if targetServiceID == "" {
		for _, id := range State.ServiceOrder {
			svc := State.Services[id]
			if svc.Status == "critical" || svc.Status == "degraded" {
				targetServiceID = id
				break
			}
		}
	}

	// 2. Build graph snapshot and node features for TGNN inference
	serviceIDs := make([]string, 0, len(State.Services))
	seenIDs := make(map[string]bool)
	for _, id := range State.ServiceOrder {
		if _, exists := State.Services[id]; exists && !seenIDs[id] {
			serviceIDs = append(serviceIDs, id)
			seenIDs[id] = true
		}
	}
	for id := range State.Services {
		if !seenIDs[id] {
			serviceIDs = append(serviceIDs, id)
			seenIDs[id] = true
		}
	}

	totalNodes := float64(len(serviceIDs))
	if totalNodes == 0 {
		totalNodes = 1.0
	}

	inDegrees := make(map[string]float64)
	outDegrees := make(map[string]float64)
	for _, svc := range State.Services {
		outDegrees[svc.ID] = float64(len(svc.Dependencies))
		for _, dep := range svc.Dependencies {
			inDegrees[dep]++
		}
	}

	centralityScores := make(map[string]float64)
	for _, id := range serviceIDs {
		centralityScores[id] = (inDegrees[id] + outDegrees[id]) / totalNodes
	}

	snapshotNodes := make([]*graphpb.Node, 0, len(serviceIDs))
	snapshotEdges := make([]*graphpb.Edge, 0)
	metricPoints := make([]*telemetrypb.MetricPoint, 0)

	nowNs := time.Now().UnixNano()

	for _, id := range serviceIDs {
		svc := State.Services[id]
		nodeMetrics := map[string]float64{
			"cpu":                   svc.CPU,
			"memory":                svc.Memory,
			"latency_ms":            svc.Latency,
			"error_rate":            svc.ErrorRate,
			"queue_depth":           float64(svc.QueueDepth),
			"db_connection_wait_ms": svc.DBWait,
			"cache_miss_rate":       svc.CacheMiss,
			"packet_loss_rate":      svc.PacketLoss,
			"active_threads":        float64(svc.ActiveThreads),
		}

		snapshotNodes = append(snapshotNodes, &graphpb.Node{
			Id:      svc.ID,
			Name:    svc.Name,
			Type:    svc.Type,
			Status:  svc.Status,
			Metrics: nodeMetrics,
		})

		// Add telemetry metric points
		metricPoints = append(metricPoints,
			&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: svc.ID, MetricName: "cpu_usage", Value: svc.CPU},
			&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: svc.ID, MetricName: "memory_usage", Value: svc.Memory},
			&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: svc.ID, MetricName: "p95_latency_ms", Value: svc.Latency},
			&telemetrypb.MetricPoint{TimestampUnixNano: nowNs, ServiceId: svc.ID, MetricName: "error_rate", Value: svc.ErrorRate},
		)

		if svc.DBWait > 0 {
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: nowNs,
				ServiceId:         svc.ID,
				MetricName:        "db_connection_wait_ms",
				Value:             svc.DBWait,
			})
		}
		if svc.CacheMiss > 0 {
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: nowNs,
				ServiceId:         svc.ID,
				MetricName:        "cache_miss_rate",
				Value:             svc.CacheMiss,
			})
		}
		if svc.QueueDepth > 0 {
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: nowNs,
				ServiceId:         svc.ID,
				MetricName:        "queue_depth",
				Value:             float64(svc.QueueDepth),
			})
		}
		if svc.PacketLoss > 0 {
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: nowNs,
				ServiceId:         svc.ID,
				MetricName:        "packet_loss_rate",
				Value:             svc.PacketLoss,
			})
		}
		if svc.ActiveThreads > 0 {
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: nowNs,
				ServiceId:         svc.ID,
				MetricName:        "active_threads",
				Value:             float64(svc.ActiveThreads),
			})
		}

		for _, dep := range svc.Dependencies {
			snapshotEdges = append(snapshotEdges, &graphpb.Edge{
				Source:       svc.ID,
				Target:       dep,
				Relationship: "calls",
				LatencyMs:    svc.Latency,
				ErrorRate:    svc.ErrorRate,
				Throughput:   svc.RPS,
			})
		}
	}

	predictReq := &predictpb.PredictRequest{
		Graph: &graphpb.GraphRepresentation{
			Snapshot: &graphpb.GraphSnapshot{
				TimestampUnixNano: nowNs,
				Nodes:             snapshotNodes,
				Edges:             snapshotEdges,
			},
			CentralityScores: centralityScores,
		},
		RecentTelemetry: &telemetrypb.TelemetryBatch{
			Metrics: metricPoints,
		},
		TargetNodeId: targetServiceID,
	}

	// 3. Execute gRPC inference call against TGNN checkpoint
	var pred PredictionOutput
	var grpcErr error

	if GlobalGRPCClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()

		var resp *predictpb.PredictResponse
		resp, grpcErr = GlobalGRPCClient.Predict(ctx, predictReq)
		if grpcErr == nil && resp != nil {
			// Find highest failure probability
			maxFailProb := 0.0
			estimatedTTF := 600
			topConf := 0.95

			for _, fp := range resp.FailurePredictions {
				p := fp.FailureProbability
				if p > maxFailProb {
					maxFailProb = p
					topConf = fp.Confidence
					estimatedTTF = int(fp.EstimatedTimeToFailureSec)
				}
				if targetServiceID != "" && fp.NodeId == targetServiceID {
					if fp.FailureProbability > 0.3 {
						maxFailProb = math.Max(maxFailProb, fp.FailureProbability)
						estimatedTTF = int(fp.EstimatedTimeToFailureSec)
					}
				}
			}

			// Format failure probability to 0-100 scale
			if maxFailProb <= 1.0 {
				maxFailProb = math.Round(maxFailProb*1000) / 10.0
			} else {
				maxFailProb = math.Round(maxFailProb*10) / 10.0
			}

			if topConf <= 1.0 {
				topConf = math.Round(topConf*1000) / 10.0
			}

			var rootCauseID string
			var rootCauseName string
			var rootCauseReason string
			var faultFamily string
			var recAction string
			var recID string
			var recommendedActions []RecoveryAction

			if len(resp.RootCauses) > 0 {
				topRC := resp.RootCauses[0]
				rootCauseID = topRC.NodeId
				if s, ok := State.Services[rootCauseID]; ok {
					rootCauseName = s.Name
				} else {
					rootCauseName = rootCauseID
				}
				rootCauseReason = topRC.Explanation
				faultFamily = topRC.FaultFamily
			}

			// Forward action recommendations directly from Python decision engine
			if len(resp.RecommendedActions) > 0 {
				for _, act := range resp.RecommendedActions {
					recommendedActions = append(recommendedActions, RecoveryAction{
						ID:               act.ActionId,
						Title:            act.Title,
						Description:      act.Description,
						TargetServiceID:  act.TargetNodeId,
						ActionType:       strings.ToLower(act.ActionType),
						Confidence:       act.Confidence * 100.0,
						Status:           "idle",
						RiskLevel:        act.RiskLevel,
						RequiresApproval: act.RequiresApproval,
					})
				}
				primary := resp.RecommendedActions[0]
				recAction = primary.Title
				recID = primary.ActionId
			} else {
				recAction = "Cluster nominal; maintain continuous observation"
				recID = "act-none"
			}

			// Gather blast radius nodes from propagation paths
			blastSet := make(map[string]bool)
			for _, p := range resp.PropagationPaths {
				for _, pn := range p.PathNodes {
					if pn != rootCauseID {
						blastSet[pn] = true
					}
				}
			}
			blastRadius := make([]string, 0, len(blastSet))
			for b := range blastSet {
				blastRadius = append(blastRadius, b)
			}

			// Extract affected nodes with per-hop ETA
			var affectedNodes []AffectedNodeInfo
			for _, an := range resp.AffectedNodes {
				affectedNodes = append(affectedNodes, AffectedNodeInfo{
					NodeID:      an.NodeId,
					HopDistance: an.HopDistance,
					ETASeconds:  an.EtaSeconds,
					RiskScore:   an.RiskScore,
				})
			}

			// Determine if cluster is operating nominally
			hasActiveFault := len(State.ActiveFaults) > 0
			for _, svc := range State.Services {
				if svc.Status == "critical" || svc.Status == "degraded" {
					hasActiveFault = true
					break
				}
			}

			isNominal := faultFamily == "nominal" || faultFamily == "" || rootCauseID == "" || maxFailProb < 30.0

			if isNominal && !hasActiveFault {
				rootCauseID = ""
				rootCauseName = "None"
				rootCauseReason = "Cluster operating within nominal spatiotemporal bounds"
				recAction = "Cluster nominal; maintain continuous observation"
				recID = "act-none"
				blastRadius = []string{}
				affectedNodes = []AffectedNodeInfo{}
				recommendedActions = []RecoveryAction{}
				estimatedTTF = 0
				faultFamily = "nominal"
			}

			State.LatestRecommendations = recommendedActions

			engine := resp.Engine
			if engine == "" {
				engine = "dense_tgnn"
			}
			modelVersion := resp.ModelVersion
			if modelVersion == "" {
				modelVersion = "1.0.0-dense"
			}
			calibratedConf := resp.CalibratedConfidence
			if calibratedConf == 0 {
				calibratedConf = topConf / 100.0
			}

			pred = PredictionOutput{
				FailureProbability:   maxFailProb,
				Confidence:           topConf,
				EstimatedFailureSec:  estimatedTTF,
				RootCauseServiceID:   rootCauseID,
				RootCauseServiceName: rootCauseName,
				RootCauseReason:      rootCauseReason,
				FaultFamily:          faultFamily,
				BlastRadius:          blastRadius,
				AffectedNodes:        affectedNodes,
				RecommendedAction:    recAction,
				RecommendedActionID:  recID,
				RecommendedActions:   recommendedActions,
				Engine:               engine,
				ModelVersion:         modelVersion,
				CalibratedConfidence: calibratedConf,
			}
		}
	}

	// 4. Graceful heuristic fallback if gRPC was unconfigured or timed out
	if grpcErr != nil || GlobalGRPCClient == nil {
		if grpcErr != nil {
			log.Printf("[predict] Notice: Python gRPC call failed (%v); generating topological heuristic prediction", grpcErr)
		}

		var rootCauseSvc *Microservice
		if targetServiceID != "" {
			rootCauseSvc = State.Services[targetServiceID]
		}

		if rootCauseSvc != nil {
			blastSet := make(map[string]bool)
			queue := append([]string{}, rootCauseSvc.Dependencies...)
			for len(queue) > 0 {
				curr := queue[0]
				queue = queue[1:]
				if !blastSet[curr] {
					blastSet[curr] = true
					if s, ok := State.Services[curr]; ok {
						queue = append(queue, s.Dependencies...)
					}
				}
			}
			blastRadius := make([]string, 0, len(blastSet))
			for n := range blastSet {
				blastRadius = append(blastRadius, n)
			}

			prob := 78.5
			if rootCauseSvc.Status == "critical" {
				prob = 92.4
			} else if rootCauseSvc.Status == "degraded" {
				prob = 68.0
			}

			reason := fmt.Sprintf("Observed anomalous telemetry signature on %s (Latency: %.0fms, CPU: %.1f%%)", rootCauseSvc.Name, rootCauseSvc.Latency, rootCauseSvc.CPU)
			recAction := fmt.Sprintf("Execute rolling container restart on %s", rootCauseSvc.Name)
			recID := "act-restart"
			if rootCauseSvc.DBWait >= 500.0 || rootCauseSvc.Type == "database" || rootCauseSvc.Type == "persistence" {
				recAction = fmt.Sprintf("Expand connection pool on %s (+10 connections)", rootCauseSvc.Name)
				recID = "act-increase_pool"
			} else if rootCauseSvc.CacheMiss >= 0.70 || rootCauseSvc.Type == "cache" {
				recAction = fmt.Sprintf("Flush cache on %s", rootCauseSvc.Name)
				recID = "act-flush_cache"
			} else if rootCauseSvc.QueueDepth >= 10 {
				recAction = fmt.Sprintf("Scale consumers for %s queue (Replicas +2)", rootCauseSvc.Name)
				recID = "act-scale"
			} else if rootCauseSvc.CPU > 80.0 {
				recAction = fmt.Sprintf("Scale %s replicas from %d to %d", rootCauseSvc.Name, rootCauseSvc.Replicas, rootCauseSvc.Replicas+2)
				recID = "act-scale"
			} else if rootCauseSvc.Latency > 500.0 {
				recAction = fmt.Sprintf("Reroute ingress traffic around %s", rootCauseSvc.Name)
				recID = "act-reroute"
			}

			ttf := 15
			if rootCauseSvc.Latency > 0 {
				ttf = int(math.Max(5, math.Min(90, 60.0-(rootCauseSvc.Latency/20.0))))
			}

			actType := "restart"
			riskLvl := "LOW"
			reqAppr := false
			if rootCauseSvc.Latency > 500.0 || rootCauseSvc.ErrorRate >= 0.40 {
				actType = "reroute_traffic"
				riskLvl = "HIGH"
				reqAppr = true
			} else if rootCauseSvc.CPU > 80.0 {
				actType = "scale"
				riskLvl = "MEDIUM"
				reqAppr = true
			}

			recs := []RecoveryAction{
				{
					ID:               recID,
					Title:            recAction,
					Description:      reason,
					TargetServiceID:  rootCauseSvc.ID,
					ActionType:       actType,
					Confidence:       95.8,
					Status:           "idle",
					RiskLevel:        riskLvl,
					RequiresApproval: reqAppr,
				},
			}
			State.LatestRecommendations = recs

			pred = PredictionOutput{
				FailureProbability:   prob,
				Confidence:           95.8,
				EstimatedFailureSec:  ttf,
				RootCauseServiceID:   rootCauseSvc.ID,
				RootCauseServiceName: rootCauseSvc.Name,
				RootCauseReason:      reason,
				BlastRadius:          blastRadius,
				RecommendedAction:    recAction,
				RecommendedActionID:  recID,
				RecommendedActions:   recs,
				Engine:               "heuristic_fallback",
				ModelVersion:         "heuristic",
				CalibratedConfidence: 0.85,
			}
		} else {
			pred = PredictionOutput{
				FailureProbability:   4.2,
				Confidence:           98.4,
				EstimatedFailureSec:  0,
				RootCauseServiceID:   "",
				RootCauseServiceName: "None",
				RootCauseReason:      "Cluster operating within normal nominal latency boundaries",
				BlastRadius:          []string{},
				RecommendedAction:    "Cluster nominal; maintain continuous observation",
				RecommendedActionID:  "act-none",
				Engine:               "heuristic_fallback",
				ModelVersion:         "heuristic",
				CalibratedConfidence: 0.98,
			}
		}
	}

	latestVer := 1
	if store := GetAuthStore(); store != nil {
		if v, err := store.GetLatestTopologyVersion(context.Background()); err == nil && v != nil && v.Version > 0 {
			latestVer = v.Version
		}
	}
	pred.TopologyVersion = latestVer

	return pred
}

func GetPredictionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	GetLatestPredictionHandler(w, r)
}

func PredictHandler(w http.ResponseWriter, r *http.Request) {
	GetLatestPredictionHandler(w, r)
}
