package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	graphpb "github.com/aegis/go-services/shared/pb/graph"
	predictpb "github.com/aegis/go-services/shared/pb/predict"
	telemetrypb "github.com/aegis/go-services/shared/pb/telemetry"
)

type IncidentGateConfig struct {
	MinProb                float64
	MinWindows             int
	ScrapeIntervalSec      int
	CPUThreshold           float64
	LatencyThreshold       float64
	ErrorThreshold         float64
	DBWaitThreshold        float64
	CacheMissThreshold     float64
	QueueDepthThreshold    int
	PacketLossThreshold    float64
	ActiveThreadsThreshold int
}

func getIncidentGateConfig() IncidentGateConfig {
	return IncidentGateConfig{
		MinProb:                getEnvFloat("AEGIS_INCIDENT_MIN_PROB", 50.0),
		MinWindows:             getEnvInt("AEGIS_INCIDENT_MIN_WINDOWS", 2),
		ScrapeIntervalSec:      getEnvInt("AEGIS_SCRAPE_INTERVAL_SEC", getEnvInt("AEGIS_SCRAPE_INTERVAL", 2)),
		CPUThreshold:           getEnvFloat("AEGIS_CPU_BREACH_THRESHOLD", 85.0),
		LatencyThreshold:       getEnvFloat("AEGIS_LATENCY_BREACH_THRESHOLD", 300.0),
		ErrorThreshold:         getEnvFloat("AEGIS_ERROR_BREACH_THRESHOLD", 0.10),
		DBWaitThreshold:        getEnvFloat("AEGIS_DB_WAIT_BREACH_THRESHOLD", 200.0),
		CacheMissThreshold:     getEnvFloat("AEGIS_CACHE_MISS_BREACH_THRESHOLD", 0.70),
		QueueDepthThreshold:    getEnvInt("AEGIS_QUEUE_DEPTH_BREACH_THRESHOLD", 10),
		PacketLossThreshold:    getEnvFloat("AEGIS_PACKET_LOSS_BREACH_THRESHOLD", 0.15),
		ActiveThreadsThreshold: getEnvInt("AEGIS_THREADS_BREACH_THRESHOLD", 80),
	}
}

func getEnvFloat(key string, def float64) float64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return def
}

func getEnvInt(key string, def int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return def
}

func isMetricBreached(svc *Microservice, cfg IncidentGateConfig) bool {
	if svc == nil {
		return false
	}
	return svc.Status == "offline" || svc.Status == "failed" ||
		svc.CPU >= cfg.CPUThreshold ||
		svc.Latency >= cfg.LatencyThreshold ||
		svc.ErrorRate >= cfg.ErrorThreshold ||
		svc.DBWait >= cfg.DBWaitThreshold ||
		svc.CacheMiss >= cfg.CacheMissThreshold ||
		svc.QueueDepth >= cfg.QueueDepthThreshold ||
		svc.PacketLoss >= cfg.PacketLossThreshold ||
		svc.ActiveThreads >= cfg.ActiveThreadsThreshold ||
		svc.Status == "critical"
}

var CanonicalFaultFamilyActions = map[string][]struct {
	ActionType     string
	RiskLevel      string
	ReqApproval    bool
	ExpectedEffect string
	Reason         string
}{
	"database": {
		{"increase_pool", "LOW", false, "Reduce db_connection_wait_ms below threshold", "Expand connection pool to relieve lock contention"},
		{"kill_blocking_query", "MEDIUM", true, "Terminate blocking query holding row locks", "Cancel blocking transactions"},
	},
	"db_lock": {
		{"increase_pool", "LOW", false, "Reduce db_connection_wait_ms below threshold", "Expand connection pool to relieve lock contention"},
		{"kill_blocking_query", "MEDIUM", true, "Terminate blocking query holding row locks", "Cancel blocking transactions"},
	},
	"db_contention": {
		{"increase_pool", "LOW", false, "Reduce db_connection_wait_ms below threshold", "Expand connection pool to relieve lock contention"},
		{"kill_blocking_query", "MEDIUM", true, "Terminate blocking query holding row locks", "Cancel blocking transactions"},
	},
	"slow_query": {
		{"increase_pool", "LOW", false, "Expand pool to absorb query latency", "Expand connection pool"},
		{"kill_blocking_query", "MEDIUM", true, "Cancel long-running slow query execution", "Terminate slow queries"},
	},
	"cache": {
		{"flush_cache", "MEDIUM", true, "Purge corrupted or stale cache keys", "Purge cache"},
		{"failover", "MEDIUM", true, "Failover to replica cache cluster", "Failover cache cluster"},
	},
	"cache_down": {
		{"flush_cache", "MEDIUM", true, "Purge corrupted or stale cache keys", "Purge cache"},
		{"failover", "MEDIUM", true, "Failover to replica cache cluster", "Failover cache cluster"},
	},
	"cpu_stress": {
		{"scale", "LOW", false, "Scale replicas to distribute CPU load below 80%", "Scale replicas"},
		{"restart", "MEDIUM", true, "Restart container to terminate runaway CPU loop", "Restart CPU-bound container"},
	},
	"cpu_saturation": {
		{"scale", "LOW", false, "Scale replicas to distribute CPU load below 80%", "Scale replicas"},
		{"restart", "MEDIUM", true, "Restart container to terminate runaway CPU loop", "Restart CPU-bound container"},
	},
	"latency": {
		{"reroute_traffic", "MEDIUM", true, "Divert transit traffic away from high-latency path", "Reroute ingress traffic"},
		{"circuit_break", "HIGH", true, "Trip circuit breaker to isolate degraded link", "Open circuit breaker"},
	},
	"network_latency": {
		{"reroute_traffic", "MEDIUM", true, "Divert transit traffic away from high-latency path", "Reroute ingress traffic"},
		{"circuit_break", "HIGH", true, "Trip circuit breaker to isolate degraded link", "Open circuit breaker"},
	},
	"packet_loss": {
		{"reroute_traffic", "MEDIUM", true, "Divert traffic away from packet loss interface", "Reroute traffic around link"},
		{"circuit_break", "HIGH", true, "Trip circuit breaker to isolate packet-dropping link", "Isolate degraded link"},
	},
	"crash": {
		{"restart", "HIGH", true, "Restart crashed service process to restore availability", "Rolling container restart"},
		{"scale", "MEDIUM", true, "Add replicas to ensure capacity during recovery", "Scale replicas for redundancy"},
	},
	"kill_service": {
		{"restart", "HIGH", true, "Restart crashed service process to restore availability", "Rolling container restart"},
		{"scale", "MEDIUM", true, "Add replicas to ensure capacity during recovery", "Scale replicas for redundancy"},
	},
	"memory_leak": {
		{"restart", "HIGH", true, "Restart container to release leaked heap memory", "Restart leaking container"},
		{"scale", "MEDIUM", true, "Distribute memory pressure across replicas", "Scale replicas"},
	},
	"queue": {
		{"scale", "LOW", false, "Scale consumer replicas to drain message backlog", "Scale consumers"},
		{"drain_queue", "MEDIUM", true, "Route messages to dead letter queue", "Drain queue backlog"},
	},
	"mq_lag": {
		{"scale", "LOW", false, "Scale consumer replicas to drain message backlog", "Scale consumers"},
		{"drain_queue", "MEDIUM", true, "Route messages to dead letter queue", "Drain queue backlog"},
	},
	"thread_exhaustion": {
		{"scale", "LOW", false, "Scale service replicas to distribute thread pool pressure", "Scale replicas"},
		{"circuit_break", "HIGH", true, "Shed incoming traffic to relieve thread pool", "Shed load"},
	},
}

func GetLatestPredictionHandler(w http.ResponseWriter, r *http.Request) {
	pred := ComputePredictionOutput()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pred)
}

func DecisionTraceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	State.mu.RLock()
	trace := State.LatestDecisionTrace
	State.mu.RUnlock()
	if trace == nil {
		pred := ComputePredictionOutput()
		_ = pred
		State.mu.RLock()
		trace = State.LatestDecisionTrace
		State.mu.RUnlock()
	}
	_ = json.NewEncoder(w).Encode(trace)
}

func ComputePredictionOutput() PredictionOutput {
	State.mu.Lock()
	defer State.mu.Unlock()

	cfg := getIncidentGateConfig()
	staleThreshold := time.Duration(2*cfg.ScrapeIntervalSec) * time.Second

	// 1. Data Quality Gate: evaluate freshness and drop stale data (>2x scrape interval)
	// Services with no fresh data get status "unknown" and are excluded from inference.
	freshServiceIDs := make([]string, 0, len(State.Services))
	dataFresh := false
	hasAnyFreshData := false

	for id, svc := range State.Services {
		if svc.LastIngestTime.IsZero() {
			// Registration and process reachability are not telemetry evidence.
			// Do not score services until at least one timestamped signal arrives.
			svc.Status = "unknown"
			svc.DataStatus = "unknown"
		} else if time.Since(svc.LastIngestTime) <= staleThreshold {
			freshServiceIDs = append(freshServiceIDs, id)
			hasAnyFreshData = true
			svc.DataStatus = "fresh"
		} else {
			// Stale data dropped from inference
			svc.Status = "unknown"
			svc.DataStatus = "unknown"
		}
	}
	dataFresh = hasAnyFreshData

	// 2. Derive Candidate Target strictly from anomalous telemetry (NO simulator leakage)
	var targetServiceID string
	highestDev := 0.0

	for _, id := range freshServiceIDs {
		svc := State.Services[id]
		if svc.Status == "unknown" {
			continue
		}
		// Compute deviation score from normalized telemetry metrics
		dev := 0.0
		if svc.Status == "offline" || svc.Status == "failed" {
			dev = math.Max(dev, cfg.MinProb/8.0)
		}
		if svc.DBWait > 0 {
			dev += (svc.DBWait / 200.0) * 4.0
		}
		if svc.CacheMiss > 0.5 {
			dev += (svc.CacheMiss / 0.5) * 3.0
		}
		if svc.CPU > cfg.CPUThreshold {
			dev += ((svc.CPU - cfg.CPUThreshold) / 10.0) * 2.0
		}
		if svc.Latency > cfg.LatencyThreshold {
			dev += ((svc.Latency - cfg.LatencyThreshold) / 200.0) * 3.0
		}
		if svc.ErrorRate > cfg.ErrorThreshold {
			dev += (svc.ErrorRate / cfg.ErrorThreshold) * 4.0
		}
		if svc.PacketLoss > cfg.PacketLossThreshold {
			dev += (svc.PacketLoss / cfg.PacketLossThreshold) * 3.0
		}
		if svc.QueueDepth > cfg.QueueDepthThreshold {
			dev += (float64(svc.QueueDepth) / float64(cfg.QueueDepthThreshold)) * 2.0
		}
		if svc.ActiveThreads > cfg.ActiveThreadsThreshold {
			dev += (float64(svc.ActiveThreads) / float64(cfg.ActiveThreadsThreshold)) * 2.0
		}

		if dev > highestDev {
			highestDev = dev
			targetServiceID = id
		}
	}

	// 3. Track sustained breach windows per node for the Incident Gate
	anyNodeBreached := false
	hasSustainedBreach := false

	if State.NodeBreachCounts == nil {
		State.NodeBreachCounts = make(map[string]int)
	}

	for _, id := range freshServiceIDs {
		svc := State.Services[id]
		if isMetricBreached(svc, cfg) {
			State.NodeBreachCounts[id]++
			anyNodeBreached = true
			if State.NodeBreachCounts[id] >= cfg.MinWindows {
				hasSustainedBreach = true
			}
		} else {
			State.NodeBreachCounts[id] = 0
		}
	}

	// 4. Build graph snapshot and observed telemetry for baseline analysis
	serviceIDs := make([]string, 0, len(freshServiceIDs))
	seenIDs := make(map[string]bool)
	for _, id := range State.ServiceOrder {
		if _, exists := State.Services[id]; exists && State.Services[id].Status != "unknown" && !seenIDs[id] {
			serviceIDs = append(serviceIDs, id)
			seenIDs[id] = true
		}
	}
	for _, id := range freshServiceIDs {
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
	for _, id := range serviceIDs {
		svc := State.Services[id]
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
	logRecords := make([]*telemetrypb.LogEntry, 0)
	traceSpans := make([]*telemetrypb.TraceSpan, 0)

	nowNs := time.Now().UnixNano()

	for _, id := range serviceIDs {
		svc := State.Services[id]
		allNodeMetrics := map[string]float64{
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
		nodeMetrics := make(map[string]float64)
		for name, value := range allNodeMetrics {
			signal := name
			if signal == "latency_ms" {
				signal = "latency"
			}
			if signal == "db_connection_wait_ms" {
				signal = "db_wait"
			}
			lastSeen, observed := svc.SignalLastSeen[signal]
			if !observed || time.Since(lastSeen) > staleThreshold {
				continue
			}
			// Explicitly observed zero values are valid telemetry.
			nodeMetrics[name] = value
			metricPoints = append(metricPoints, &telemetrypb.MetricPoint{
				TimestampUnixNano: lastSeen.UnixNano(),
				ServiceId:         svc.ID,
				MetricName:        name,
				Value:             value,
				Unit:              metricUnit(name),
				Labels:            svc.ResourceAttributes,
			})
		}

		metadata := make(map[string]string, len(svc.ResourceAttributes)+2)
		if svc.Namespace != "" {
			metadata["service.namespace"] = svc.Namespace
		}
		if svc.ClusterID != "" {
			metadata["k8s.cluster.name"] = svc.ClusterID
		}
		for key, value := range svc.ResourceAttributes {
			metadata[key] = value
		}
		snapshotNodes = append(snapshotNodes, &graphpb.Node{
			Id:       svc.ID,
			Name:     svc.Name,
			Type:     svc.Type,
			Status:   svc.Status,
			Metadata: metadata,
			Metrics:  nodeMetrics,
		})

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

	// Keep trace/log evidence bounded to a recent window and forward the actual
	// records to the RCA stage. No relationship is inferred from log text alone.
	cutoffNs := nowNs - int64(5*time.Minute)
	for _, entry := range State.Logs {
		if entry.ServiceID == "" {
			continue
		}
		timestampNs := nowNs
		if parsed, err := time.Parse(time.RFC3339Nano, entry.Timestamp); err == nil {
			timestampNs = parsed.UnixNano()
		}
		if timestampNs < cutoffNs {
			continue
		}
		logRecords = append(logRecords, &telemetrypb.LogEntry{
			TimestampUnixNano: timestampNs, ServiceId: entry.ServiceID, Level: entry.Level,
			Message: entry.Message, TraceId: entry.TraceID, Attributes: entry.ResourceAttributes,
		})
	}
	traceStart := 0
	if len(State.Traces) > 2000 {
		traceStart = len(State.Traces) - 2000
	}
	for _, span := range State.Traces[traceStart:] {
		if span.ServiceID == "" || span.EndTimeUnixNano < cutoffNs {
			continue
		}
		attributes := make(map[string]string, len(span.Attributes)+len(span.ResourceAttributes))
		for key, value := range span.ResourceAttributes {
			attributes[key] = value
		}
		for key, value := range span.Attributes {
			attributes[key] = fmt.Sprint(value)
		}
		traceSpans = append(traceSpans, &telemetrypb.TraceSpan{
			TraceId: span.TraceID, SpanId: span.SpanID, ParentSpanId: span.ParentSpanID,
			ServiceId: span.ServiceID, OperationName: span.OperationName,
			StartTimeUnixNano: span.StartTimeUnixNano, EndTimeUnixNano: span.EndTimeUnixNano,
			StatusCode: int32(span.StatusCode), Attributes: attributes,
		})
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
			Logs:    logRecords,
			Traces:  traceSpans,
		},
		TargetNodeId: targetServiceID,
	}

	// 5. Request service-level baseline analysis; keep local policy evidence if unavailable
	var pred PredictionOutput
	var grpcErr error
	var chosenRootCauseID string
	var chosenRootCauseReason string
	var chosenFaultFamily string
	rootCauseCandidates := make([]RootCauseCandidate, 0)
	var chosenActions []RecoveryAction
	rawProb := 0.0
	rawConf := 0.0
	var perNodeScores = make(map[string]float64)
	engineName := "evidence_only_advisory"
	fallbackReason := ""
	modelVersion := "evidence-only"
	calibratedConf := 0.0
	confLabel := "uncalibrated statistical ranking"

	if GlobalGRPCClient != nil && len(serviceIDs) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()

		var resp *predictpb.PredictResponse
		resp, grpcErr = GlobalGRPCClient.Predict(ctx, predictReq)
		if grpcErr == nil && resp != nil {
			engineName = resp.Engine
			if engineName == "" {
				engineName = "baseline"
			}
			fallbackReason = resp.FallbackReason
			modelVersion = resp.ModelVersion
			if modelVersion == "" {
				modelVersion = "service-baseline-v1"
			}
			confLabel = "uncalibrated statistical ranking"

			for _, fp := range resp.FailurePredictions {
				perNodeScores[fp.NodeId] = fp.FailureProbability
				if fp.FailureProbability > rawProb {
					rawProb = fp.FailureProbability
					rawConf = fp.Confidence
				}
			}

			if len(resp.RootCauses) > 0 {
				topRC := resp.RootCauses[0]
				chosenRootCauseID = topRC.NodeId
				chosenFaultFamily = topRC.FaultFamily
				chosenRootCauseReason = topRC.Explanation
			}
			for _, candidate := range resp.RootCauses {
				item := RootCauseCandidate{
					ServiceID: candidate.NodeId, FaultFamily: candidate.FaultFamily,
					RankingScore: candidate.RankingScore, Uncertainty: candidate.Uncertainty,
					Explanation: candidate.Explanation, TraceRefs: append([]string(nil), candidate.TraceRefs...),
					LogRefs: append([]string(nil), candidate.LogRefs...),
				}
				if candidate.AnomalyOnsetUnixNano > 0 {
					item.AnomalyOnset = time.Unix(0, candidate.AnomalyOnsetUnixNano).UTC().Format(time.RFC3339Nano)
				}
				for _, evidence := range candidate.Evidence {
					item.Evidence = append(item.Evidence, ActionEvidence{Metric: evidence.Metric, Value: evidence.ObservedValue, Threshold: evidence.ThresholdValue, Unit: evidence.Unit})
				}
				rootCauseCandidates = append(rootCauseCandidates, item)
			}

			// Forward action recommendations directly from Python decision pipeline
			for _, act := range resp.RecommendedActions {
				chosenActions = append(chosenActions, RecoveryAction{
					ID:               act.ActionId,
					Title:            act.Title,
					Description:      act.Description,
					TargetServiceID:  act.TargetNodeId,
					Target:           act.TargetNodeId,
					ActionType:       strings.ToLower(act.ActionType),
					Confidence:       act.Confidence * 100.0,
					Status:           "idle",
					RiskLevel:        act.RiskLevel,
					Risk:             act.RiskLevel,
					RequiresApproval: act.RequiresApproval,
					FaultFamily:      chosenFaultFamily,
					WhyThisTarget:    fmt.Sprintf("Diagnosed root cause %s exhibiting %s fault signature", act.TargetNodeId, chosenFaultFamily),
					ExpectedEffect:   fmt.Sprintf("Relieve %s symptoms and restore SLA", chosenFaultFamily),
				})
			}
			calibratedConf = resp.CalibratedConfidence
		}
	}

	// 6. Graceful evidence-based fallback if gRPC was unconfigured or timed out
	if grpcErr != nil || GlobalGRPCClient == nil || len(chosenActions) == 0 {
		if grpcErr != nil {
			log.Printf("[predict] Notice: Python gRPC unavailable (%v); running topological evidence-based decision engine", grpcErr)
			if fallbackReason == "" {
				fallbackReason = fmt.Sprintf("Python gRPC unavailable: %v", grpcErr)
			}
			engineName = "baseline"
		}

		if targetServiceID != "" && State.Services[targetServiceID] != nil {
			rcSvc := State.Services[targetServiceID]
			chosenRootCauseID = rcSvc.ID

			// Classify fault family strictly from evidence metrics (crash process death evaluated first)
			if rcSvc.Status == "offline" || rcSvc.Status == "failed" || (rcSvc.ErrorRate >= 0.80 && rcSvc.CPU <= 5.0) {
				chosenFaultFamily = "crash"
			} else if rcSvc.DBWait >= cfg.DBWaitThreshold {
				chosenFaultFamily = "db_contention"
			} else if rcSvc.CacheMiss >= cfg.CacheMissThreshold {
				chosenFaultFamily = "cache_down"
			} else if rcSvc.CPU >= cfg.CPUThreshold {
				chosenFaultFamily = "cpu_stress"
			} else if rcSvc.QueueDepth >= cfg.QueueDepthThreshold {
				chosenFaultFamily = "mq_lag"
			} else if rcSvc.PacketLoss >= cfg.PacketLossThreshold {
				chosenFaultFamily = "packet_loss"
			} else if rcSvc.Latency >= cfg.LatencyThreshold {
				chosenFaultFamily = "latency"
			} else {
				chosenFaultFamily = "nominal"
			}

			if chosenFaultFamily != "nominal" {
				chosenRootCauseReason = fmt.Sprintf("%s is an anomalous-service candidate from observed %s signals; causal attribution is inconclusive without correlated temporal or trace evidence.", rcSvc.ID, chosenFaultFamily)
				rootCauseCandidates = append(rootCauseCandidates, RootCauseCandidate{ServiceID: rcSvc.ID, FaultFamily: chosenFaultFamily, Uncertainty: "evidence-only fallback; causal attribution is inconclusive", Explanation: chosenRootCauseReason})
				rawProb = math.Min(95.0, 50.0+(highestDev*8.0))
				rawConf = 0.0 // uncalibrated / evidence mode
				confLabel = "model unavailable, evidence-only mode"

				// Populate ranked 2-3 actions from CanonicalFaultFamilyActions
				if acts, ok := CanonicalFaultFamilyActions[chosenFaultFamily]; ok && len(acts) > 0 {
					evidenceList := make([]ActionEvidence, 0)
					if rcSvc.DBWait > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "db_connection_wait_ms", Value: rcSvc.DBWait, Threshold: cfg.DBWaitThreshold, Unit: "ms"})
					}
					if rcSvc.CacheMiss > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "cache_miss_rate", Value: rcSvc.CacheMiss, Threshold: cfg.CacheMissThreshold, Unit: "ratio"})
					}
					if rcSvc.CPU > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "cpu_usage", Value: rcSvc.CPU, Threshold: cfg.CPUThreshold, Unit: "%"})
					}
					if rcSvc.Latency > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "p95_latency_ms", Value: rcSvc.Latency, Threshold: cfg.LatencyThreshold, Unit: "ms"})
					}
					if rcSvc.ErrorRate > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "error_rate", Value: rcSvc.ErrorRate, Threshold: cfg.ErrorThreshold, Unit: "ratio"})
					}
					if rcSvc.PacketLoss > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "packet_loss_rate", Value: rcSvc.PacketLoss, Threshold: cfg.PacketLossThreshold, Unit: "ratio"})
					}
					if rcSvc.QueueDepth > 0 {
						evidenceList = append(evidenceList, ActionEvidence{Metric: "queue_depth", Value: float64(rcSvc.QueueDepth), Threshold: float64(cfg.QueueDepthThreshold), Unit: "messages"})
					}

					for idx, actDef := range acts {
						actID := fmt.Sprintf("act-%s-%s-%d", actDef.ActionType, rcSvc.ID, idx+1)
						title := fmt.Sprintf("%s on %s", strings.ToUpper(actDef.ActionType), rcSvc.Name)
						desc := fmt.Sprintf("%s. Expected effect: %s", actDef.Reason, actDef.ExpectedEffect)
						chosenActions = append(chosenActions, RecoveryAction{
							ID:               actID,
							Title:            title,
							Description:      desc,
							TargetServiceID:  rcSvc.ID,
							Target:           rcSvc.ID,
							ActionType:       actDef.ActionType,
							Confidence:       0.0,
							Status:           "idle",
							RiskLevel:        actDef.RiskLevel,
							Risk:             actDef.RiskLevel,
							RequiresApproval: actDef.ReqApproval,
							FaultFamily:      chosenFaultFamily,
							Evidence:         evidenceList,
							WhyThisTarget:    fmt.Sprintf("Diagnosed root cause %s exhibiting %s fault signature", rcSvc.ID, chosenFaultFamily),
							ExpectedEffect:   actDef.ExpectedEffect,
						})
					}
				}
			}
		}
	}

	// 7. INVARIANT CHECK: action.target MUST be in {rootCause} U {its direct deps/callers}
	invariantPassed := true
	if chosenRootCauseID != "" {
		allowedTargets := make(map[string]bool)
		allowedTargets[chosenRootCauseID] = true
		if rcSvc, ok := State.Services[chosenRootCauseID]; ok {
			for _, dep := range rcSvc.Dependencies {
				allowedTargets[dep] = true
			}
		}
		for id, s := range State.Services {
			for _, dep := range s.Dependencies {
				if dep == chosenRootCauseID {
					allowedTargets[id] = true
				}
			}
		}

		validActions := make([]RecoveryAction, 0)
		for _, act := range chosenActions {
			if !allowedTargets[act.TargetServiceID] {
				log.Printf("[predict] Invariant violation REJECTED: action target '%s' is not in {rootCause %s} U {direct deps/callers}", act.TargetServiceID, chosenRootCauseID)
				invariantPassed = false
				continue
			}
			validActions = append(validActions, act)
		}
		chosenActions = validActions
	}

	// Scale probability to 0-100 range
	failProbScaled := rawProb
	if failProbScaled <= 1.0 && failProbScaled > 0 {
		failProbScaled = math.Round(failProbScaled*1000.0) / 10.0
	} else {
		failProbScaled = math.Round(failProbScaled*10.0) / 10.0
	}

	// 8. INCIDENT GATE: incident ONLY IF (prob >= INCIDENT_MIN_PROB) AND (sustained metric breach for N windows) AND data fresh
	isIncident := (failProbScaled >= cfg.MinProb) && hasSustainedBreach && dataFresh
	var clusterState string

	if isIncident {
		clusterState = "incident"
	} else if (failProbScaled >= 30.0 || anyNodeBreached) && dataFresh {
		clusterState = "watch"
	} else {
		clusterState = "nominal"
	}

	// Build blast radius deterministically rooted at chosenRootCauseID
	blastRadius := make([]string, 0)
	affectedNodes := make([]AffectedNodeInfo, 0)

	if clusterState != "nominal" && chosenRootCauseID != "" {
		blastRadius = append(blastRadius, chosenRootCauseID)
		seenBlast := map[string]bool{chosenRootCauseID: true}

		if rcSvc, ok := State.Services[chosenRootCauseID]; ok {
			for _, dep := range rcSvc.Dependencies {
				if !seenBlast[dep] {
					seenBlast[dep] = true
					blastRadius = append(blastRadius, dep)
					affectedNodes = append(affectedNodes, AffectedNodeInfo{
						NodeID:      dep,
						HopDistance: 1,
						ETASeconds:  15,
						RiskScore:   0.65,
					})
				}
			}
		}
	}

	// 9. Construct unified PredictionOutput (Single Source of Truth)
	if clusterState == "nominal" {
		pred = PredictionOutput{
			FailureProbability:   0.0,
			Confidence:           0.0,
			ConfidenceLabel:      "n/a",
			Incident:             false,
			State:                "nominal",
			DataFresh:            dataFresh,
			EstimatedFailureSec:  0,
			RootCauseServiceID:   "",
			RootCauseServiceName: "None",
			RootCauseReason:      "Cluster operating within nominal bounds; no sustained metric breach",
			FaultFamily:          "nominal",
			BlastRadius:          []string{},
			AffectedNodes:        []AffectedNodeInfo{},
			RecommendedAction:    "Cluster nominal; maintain continuous observation",
			RecommendedActionID:  "act-none",
			RecommendedActions:   []RecoveryAction{},
			Engine:               engineName,
			FallbackReason:       fallbackReason,
			ModelVersion:         modelVersion,
			CalibratedConfidence: 0.0,
		}
		State.LatestRecommendations = []RecoveryAction{}
	} else {
		var rootCauseName string
		if s, ok := State.Services[chosenRootCauseID]; ok {
			rootCauseName = s.Name
		} else {
			rootCauseName = chosenRootCauseID
		}

		recActionTitle := "Cluster nominal; maintain continuous observation"
		recActionID := "act-none"
		if len(chosenActions) > 0 {
			recActionTitle = chosenActions[0].Title
			recActionID = chosenActions[0].ID
		}

		pred = PredictionOutput{
			FailureProbability:   failProbScaled,
			Confidence:           rawConf,
			ConfidenceLabel:      confLabel,
			Incident:             isIncident,
			State:                clusterState,
			DataFresh:            dataFresh,
			EstimatedFailureSec:  15,
			RootCauseServiceID:   chosenRootCauseID,
			RootCauseServiceName: rootCauseName,
			RootCauseReason:      chosenRootCauseReason,
			RootCauseCandidates:  rootCauseCandidates,
			FaultFamily:          chosenFaultFamily,
			BlastRadius:          blastRadius,
			AffectedNodes:        affectedNodes,
			RecommendedAction:    recActionTitle,
			RecommendedActionID:  recActionID,
			RecommendedActions:   chosenActions,
			Engine:               engineName,
			FallbackReason:       fallbackReason,
			ModelVersion:         modelVersion,
			CalibratedConfidence: calibratedConf,
		}
		State.LatestRecommendations = chosenActions
	}

	// 10. Record Provenance & Decision Trace for /debug/decision-trace
	var chosenActMap map[string]interface{}
	if len(pred.RecommendedActions) > 0 {
		topAct := pred.RecommendedActions[0]
		chosenActMap = map[string]interface{}{
			"action_id":        topAct.ID,
			"action_type":      topAct.ActionType,
			"target":           topAct.TargetServiceID,
			"risk":             topAct.RiskLevel,
			"requiresApproval": topAct.RequiresApproval,
			"why_this_target":  topAct.WhyThisTarget,
			"expected_effect":  topAct.ExpectedEffect,
		}
	}

	candidateMaps := make([]map[string]interface{}, 0, len(pred.RecommendedActions))
	for _, act := range pred.RecommendedActions {
		candidateMaps = append(candidateMaps, map[string]interface{}{
			"action_id":        act.ID,
			"action_type":      act.ActionType,
			"target":           act.TargetServiceID,
			"risk":             act.RiskLevel,
			"requiresApproval": act.RequiresApproval,
		})
	}

	trace := &DecisionTrace{
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		ModelVersion:       pred.ModelVersion,
		FeatureVectorUsed:  map[string]float64{"highest_dev": highestDev, "failure_prob": pred.FailureProbability},
		PerNodeScores:      perNodeScores,
		ClassifierOutput:   map[string]interface{}{"family": pred.FaultFamily, "score": pred.FailureProbability},
		CandidateActions:   candidateMaps,
		ChosenAction:       chosenActMap,
		InvariantPassed:    invariantPassed,
		IncidentGatePassed: isIncident,
		State:              pred.State,
		DataFresh:          pred.DataFresh,
	}
	State.LatestDecisionTrace = trace

	latestVer := 1
	if store := GetAuthStore(); store != nil {
		if v, err := store.GetLatestTopologyVersion(context.Background()); err == nil && v != nil && v.Version > 0 {
			latestVer = v.Version
		}
	}
	pred.TopologyVersion = latestVer

	return pred
}

func metricUnit(name string) string {
	switch name {
	case "cpu", "memory":
		return "percent"
	case "latency_ms", "db_connection_wait_ms":
		return "ms"
	case "error_rate", "cache_miss_rate", "packet_loss_rate":
		return "ratio"
	case "rps":
		return "requests_per_second"
	case "queue_depth", "active_threads":
		return "count"
	default:
		return ""
	}
}
func GetPredictionHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	GetLatestPredictionHandler(w, r)
}

func PredictHandler(w http.ResponseWriter, r *http.Request) {
	GetLatestPredictionHandler(w, r)
}
