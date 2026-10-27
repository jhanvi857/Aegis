package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type ServiceMetric struct {
	Timestamp  string  `json:"timestamp"`
	CPU        float64 `json:"cpu"`
	Memory     float64 `json:"memory"`
	Latency    float64 `json:"latency"`
	RPS        float64 `json:"rps"`
	ErrorRate  float64 `json:"errorRate"`
	QueueDepth int     `json:"queueDepth"`
	ObservedSignals []string `json:"observedSignals,omitempty"`
}

type Microservice struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Type           string          `json:"type"`
	Status         string          `json:"status"` // healthy, degraded, critical
	Replicas       int             `json:"replicas"`
	MaxReplicas    int             `json:"maxReplicas"`
	CPU            float64         `json:"cpu"`
	Memory         float64         `json:"memory"`
	Latency        float64         `json:"latency"`
	RPS            float64         `json:"rps"`
	ErrorRate      float64         `json:"errorRate"`
	QueueDepth     int             `json:"queueDepth"`
	DBWait         float64         `json:"dbConnectionWaitMs"`
	CacheMiss      float64         `json:"cacheMissRate"`
	PacketLoss     float64         `json:"packetLossRate"`
	ActiveThreads  int             `json:"activeThreads"`
	Version        string          `json:"version"`
	Uptime         string          `json:"uptime"`
	Dependencies   []string        `json:"dependencies"`
	Namespace      string          `json:"namespace,omitempty"`
	ClusterID      string          `json:"clusterId,omitempty"`
	ResourceAttributes map[string]string `json:"resourceAttributes,omitempty"`
	History        []ServiceMetric `json:"history"`
	Host           string          `json:"-"`
	Port           int             `json:"-"`
	LastIngestTime time.Time       `json:"-"`
	Source         string          `json:"source,omitempty"`
	LastSeen       time.Time       `json:"lastSeen,omitempty"`
	DataStatus     string          `json:"dataStatus,omitempty"` // fresh, stale, unknown
	ObservedSignals []string       `json:"observedSignals,omitempty"`
	SignalLastSeen map[string]time.Time `json:"signalLastSeen,omitempty"`
	BreachCount    int             `json:"breachCount,omitempty"`
}

type ChaosInjection struct {
	ID                string `json:"id"`
	FaultType         string `json:"faultType"`
	Title             string `json:"title"`
	TargetServiceID   string `json:"targetServiceId"`
	TargetServiceName string `json:"targetServiceName"`
	Severity          string `json:"severity"`
	DurationSeconds   int    `json:"durationSeconds"`
	RemainingSeconds  int    `json:"remainingSeconds"`
	Status            string `json:"status"`
	StartedAt         string `json:"startedAt"`
}

type AffectedNodeInfo struct {
	NodeID      string  `json:"nodeId"`
	HopDistance int32   `json:"hopDistance"`
	ETASeconds  int64   `json:"etaSeconds"`
	RiskScore   float64 `json:"riskScore"`
}

type ActionEvidence struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Unit      string  `json:"unit,omitempty"`
}

type RootCauseCandidate struct {
	ServiceID string `json:"serviceId"`
	FaultFamily string `json:"faultFamily"`
	RankingScore float64 `json:"rankingScore"`
	Uncertainty string `json:"uncertainty"`
	Explanation string `json:"explanation"`
	AnomalyOnset string `json:"anomalyOnset,omitempty"`
	Evidence []ActionEvidence `json:"evidence,omitempty"`
	TraceRefs []string `json:"traceRefs,omitempty"`
	LogRefs []string `json:"logRefs,omitempty"`
}

type PredictionOutput struct {
	FailureProbability   float64            `json:"failureProbability"`
	Confidence           float64            `json:"confidence"`
	ConfidenceLabel      string             `json:"confidenceLabel,omitempty"`
	Incident             bool               `json:"incident"`
	State                string             `json:"state"` // nominal, watch, incident
	DataFresh            bool               `json:"dataFresh"`
	EstimatedFailureSec  int                `json:"estimatedFailureSec"`
	RootCauseServiceID   string             `json:"rootCauseServiceId"`
	RootCauseServiceName string             `json:"rootCauseServiceName"`
	RootCauseReason      string             `json:"rootCauseReason"`
	RootCauseCandidates  []RootCauseCandidate `json:"rootCauseCandidates,omitempty"`
	FaultFamily          string             `json:"faultFamily,omitempty"`
	BlastRadius          []string           `json:"blastRadius"`
	AffectedNodes        []AffectedNodeInfo `json:"affectedNodes,omitempty"`
	RecommendedAction    string             `json:"recommendedAction"`
	RecommendedActionID  string             `json:"recommendedActionId"`
	RecommendedActions   []RecoveryAction   `json:"recommendedActions,omitempty"`
	Engine               string             `json:"engine"`
	ModelVersion         string             `json:"modelVersion"`
	CalibratedConfidence float64            `json:"calibratedConfidence"`
	TopologyVersion      int                `json:"topologyVersion,omitempty"`
}

type RecoveryAction struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	TargetServiceID  string           `json:"targetServiceId"` // target
	Target           string           `json:"target,omitempty"`
	ActionType       string           `json:"actionType"` // restart, scale, flush_cache, increase_pool, reroute_traffic, kill_blocking_query, circuit_break, failover
	Confidence       float64          `json:"confidence"`
	Status           string           `json:"status"`
	RiskLevel        string           `json:"riskLevel,omitempty"`
	Risk             string           `json:"risk,omitempty"`
	RequiresApproval bool             `json:"requiresApproval,omitempty"`
	FaultFamily      string           `json:"fault_family,omitempty"`
	Evidence         []ActionEvidence `json:"evidence,omitempty"`
	ExpectedEffect   string           `json:"expected_effect,omitempty"`
	WhyThisTarget    string           `json:"why_this_target,omitempty"`
}

type DecisionTrace struct {
	Timestamp          string                   `json:"timestamp"`
	ModelVersion       string                   `json:"modelVersion"`
	CheckpointHash     string                   `json:"checkpointHash"`
	FeatureVectorUsed  map[string]float64       `json:"featureVectorUsed"`
	PerNodeScores      map[string]float64       `json:"perNodeScores"`
	ClassifierOutput   map[string]interface{}   `json:"classifierOutput"`
	CandidateActions   []map[string]interface{} `json:"candidateActions"`
	ChosenAction       map[string]interface{}   `json:"chosenAction"`
	InvariantPassed    bool                     `json:"invariantCheckPassed"`
	IncidentGatePassed bool                     `json:"incidentGatePassed"`
	State              string                   `json:"state"`
	DataFresh          bool                     `json:"dataFresh"`
}

type RecoveryHistoryItem struct {
	ID            string  `json:"id"`
	Timestamp     string  `json:"timestamp"`
	ActionTitle   string  `json:"actionTitle"`
	TargetService string  `json:"targetService"`
	Status        string  `json:"status"`
	Duration      string  `json:"duration"`
	RiskBefore    float64 `json:"riskBefore"`
	RiskAfter     float64 `json:"riskAfter"`
}

type LiveAlert struct {
	ID          string `json:"id"`
	Timestamp   string `json:"timestamp"`
	ServiceID   string `json:"serviceId"`
	ServiceName string `json:"serviceName"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Resolved    bool   `json:"resolved"`
}

type LogEntry struct {
	ID          string `json:"id"`
	Timestamp   string `json:"timestamp"`
	ServiceID   string `json:"serviceId"`
	ServiceName string `json:"serviceName"`
	Level       string `json:"level"`
	Message     string `json:"message"`
	TraceID     string `json:"traceId,omitempty"`
	SpanID      string `json:"spanId,omitempty"`
	ResourceAttributes map[string]string `json:"resourceAttributes,omitempty"`
}

type Experiment struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	FaultType          string  `json:"faultType"`
	TargetService      string  `json:"targetService"`
	PredictionAccuracy float64 `json:"predictionAccuracy"`
	DetectionTimeSec   int     `json:"detectionTimeSec"`
	MttrSec            int     `json:"mttrSec"`
	Status             string  `json:"status"`
}

type TopologyYAML struct {
	Version     string `yaml:"version"`
	SystemName  string `yaml:"system_name"`
	Description string `yaml:"description"`
	Nodes       []struct {
		ID              string            `yaml:"id"`
		Name            string            `yaml:"name"`
		Type            string            `yaml:"type"`
		Host            string            `yaml:"host"`
		Port            int               `yaml:"port"`
		HealthEndpoint  string            `yaml:"health_endpoint"`
		MetricsEndpoint string            `yaml:"metrics_endpoint"`
		Dependencies    []string          `yaml:"dependencies"`
		Metadata        map[string]string `yaml:"metadata"`
	} `yaml:"nodes"`
}

type ApprovalRecord struct {
	ActionID        string `json:"actionId"`
	TargetServiceID string `json:"targetServiceId"`
	ActionType      string `json:"actionType"`
	Approver        string `json:"approver"`
	Timestamp       string `json:"timestamp"`
	Reason          string `json:"reason,omitempty"`
}

type ControlPlaneState struct {
	mu                    sync.RWMutex
	Services              map[string]*Microservice
	ServiceOrder          []string
	ActiveFaults          map[string]*ChaosInjection
	Alerts                []LiveAlert
	Logs                  []LogEntry
	Traces                []IngestSpanItem
	RecoveryHistory       []RecoveryHistoryItem
	Experiments           []Experiment
	RawTopologyYAML       []byte
	LatestRecommendations []RecoveryAction
	ApprovedPlans         map[string]bool
	ApprovedRecords       map[string]ApprovalRecord
	NodeBreachCounts      map[string]int
	LatestDecisionTrace   *DecisionTrace
	DemoMode              bool
	AutonomousRecovery    bool
}

func newControlPlaneState() *ControlPlaneState {
	return &ControlPlaneState{
		Services:              make(map[string]*Microservice),
		ServiceOrder:          make([]string, 0),
		ActiveFaults:          make(map[string]*ChaosInjection),
		Alerts:                make([]LiveAlert, 0),
		Logs:                  make([]LogEntry, 0),
		Traces:                make([]IngestSpanItem, 0),
		RecoveryHistory:       make([]RecoveryHistoryItem, 0),
		Experiments:           make([]Experiment, 0),
		LatestRecommendations: make([]RecoveryAction, 0),
		ApprovedPlans:         make(map[string]bool),
		ApprovedRecords:       make(map[string]ApprovalRecord),
		NodeBreachCounts:      make(map[string]int),
	}
}

func (s *ControlPlaneState) Lock() {
	s.mu.Lock()
}

func (s *ControlPlaneState) Unlock() {
	s.mu.Unlock()
}

func (s *ControlPlaneState) RLock() {
	s.mu.RLock()
}

func (s *ControlPlaneState) RUnlock() {
	s.mu.RUnlock()
}

var (
	State           *ControlPlaneState = newControlPlaneState()
	stateTickerOnce sync.Once
)

func InitState() {
	demoMode := strings.EqualFold(os.Getenv("DEMO_MODE"), "true") || strings.EqualFold(os.Getenv("AEGIS_DEMO_MODE"), "true")
	autoRecovery := false
	if env := os.Getenv("AEGIS_AUTONOMOUS_RECOVERY"); env != "" {
		autoRecovery = strings.EqualFold(env, "true")
	}

	if State == nil {
		State = newControlPlaneState()
	}

	State.mu.Lock()
	State.Services = make(map[string]*Microservice)
	State.ServiceOrder = make([]string, 0)
	State.ActiveFaults = make(map[string]*ChaosInjection)
	State.Alerts = make([]LiveAlert, 0)
	State.Logs = make([]LogEntry, 0)
	State.Traces = make([]IngestSpanItem, 0)
	State.RecoveryHistory = make([]RecoveryHistoryItem, 0)
	State.Experiments = make([]Experiment, 0)
	State.LatestRecommendations = make([]RecoveryAction, 0)
	State.ApprovedPlans = make(map[string]bool)
	State.ApprovedRecords = make(map[string]ApprovalRecord)
	State.DemoMode = demoMode
	State.AutonomousRecovery = autoRecovery
	State.mu.Unlock()

	topoPath := strings.TrimSpace(os.Getenv("TOPOLOGY_PATH"))
	if topoPath != "" {
		if err := LoadTopology(topoPath); err != nil {
			log.Printf("[api-gateway] Warning: configured topology could not be loaded: %v", err)
		}
	} else {
		log.Printf("[api-gateway] No topology configured; waiting for owner-created services or an import")
	}

	// Start background maintenance loop once
	stateTickerOnce.Do(func() {
		// Do not start background maintenance ticker during automated test runs
		if flag.Lookup("test.v") != nil || os.Getenv("GO_TEST") == "1" {
			return
		}
		go State.runTicker()
		InitPrometheusWorker()
	})
}

func LoadTopology(topoPath string) error {
	data, err := os.ReadFile(topoPath)
	if err != nil {
		log.Printf("[api-gateway] Warning: could not load topology: %v", err)
		return err
	}

	var topo TopologyYAML
	if err := yaml.Unmarshal(data, &topo); err != nil {
		return err
	}

	State.mu.Lock()
	defer State.mu.Unlock()

	State.RawTopologyYAML = data
	State.Services = make(map[string]*Microservice)
	State.ServiceOrder = make([]string, 0)

	for _, n := range topo.Nodes {
		svcType := n.Type
		if svcType == "" {
			svcType = "service"
		}

		svc := &Microservice{
			ID:           n.ID,
			Name:         n.Name,
			Type:         svcType,
			Status:       "unknown",
			DataStatus:   "unknown",
			Replicas:     3,
			MaxReplicas:  10,
			History:      make([]ServiceMetric, 0),
			Version:      "v1.0.0",
			Uptime:       "up",
			Dependencies: n.Dependencies,
			Host:         n.Host,
			Port:         n.Port,
		}
		State.Services[n.ID] = svc
		State.ServiceOrder = append(State.ServiceOrder, n.ID)
	}

	log.Printf("[api-gateway] Initialized %d services from %s", len(State.Services), topoPath)
	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
		Timestamp:   time.Now().Format("15:04:05"),
		ServiceID:   "api-gateway",
		ServiceName: "Control Plane",
		Level:       "INFO",
		Message:     fmt.Sprintf("Topology loaded successfully: %d nodes observed.", len(State.Services)),
	})
	return nil
}

func (s *ControlPlaneState) runTicker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	httpClient := &http.Client{Timeout: 500 * time.Millisecond}
	tickCounter := 0

	for range ticker.C {
		tickCounter++
		s.mu.Lock()
		nowTime := time.Now().Format("15:04:05")

		// Periodic synchronization with dynamic service registry (every 5 seconds)
		store := GetAuthStore()
		if store != nil && tickCounter%5 == 0 {
			if dbServices, err := store.ListServices(context.Background(), "", ""); err == nil && len(dbServices) > 0 {
				for _, dbs := range dbServices {
					if _, exists := s.Services[dbs.ID]; !exists {
						s.Services[dbs.ID] = &Microservice{
							ID:          dbs.ID,
							Name:        dbs.Name,
							Type:        dbs.Type,
							Status:      dbs.Status,
							Replicas:    dbs.Replicas,
							MaxReplicas: dbs.MaxReplicas,
							CPU:         dbs.CPU,
							Memory:      dbs.Memory,
							Latency:     dbs.Latency,
							RPS:         dbs.RPS,
							ErrorRate:   dbs.ErrorRate,
							QueueDepth:  dbs.QueueDepth,
							Version:     dbs.Version,
							Uptime:      "up",
						}
						s.ServiceOrder = append(s.ServiceOrder, dbs.ID)
					}
				}
			}

			if dbEdges, err := store.ListEdges(context.Background(), "", ""); err == nil && len(dbEdges) > 0 {
				edgeMap := make(map[string][]string)
				for _, e := range dbEdges {
					if e.State != "rejected" {
						edgeMap[e.SourceServiceID] = append(edgeMap[e.SourceServiceID], e.TargetServiceID)
					}
				}
				for id, svc := range s.Services {
					if deps, ok := edgeMap[id]; ok {
						svc.Dependencies = deps
					} else {
						svc.Dependencies = []string{}
					}
				}
			}
		}

		serviceTTL := 15 * time.Minute
		if raw := os.Getenv("SERVICE_TTL"); raw != "" {
			if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
				serviceTTL = parsed
			}
		}
		for id, svc := range s.Services {
			if !svc.LastSeen.IsZero() && time.Since(svc.LastSeen) > serviceTTL {
				delete(s.Services, id)
				for i, orderedID := range s.ServiceOrder {
					if orderedID == id {
						s.ServiceOrder = append(s.ServiceOrder[:i], s.ServiceOrder[i+1:]...)
						break
					}
				}
			}
		}

		// 1. Process active chaos fault countdowns
		for id, fault := range s.ActiveFaults {
			fault.RemainingSeconds--
			if fault.RemainingSeconds <= 0 {
				fault.Status = "completed"
				targetID := fault.TargetServiceID
				faultID := id
				delete(s.ActiveFaults, id)

				// Restore service to healthy
				if svc, ok := s.Services[targetID]; ok {
					svc.Status = "healthy"
					svc.CPU = 25.0
					svc.Latency = 20.0
					svc.ErrorRate = 0.001
					svc.DBWait = 0.0
					svc.CacheMiss = 0.0
					svc.PacketLoss = 0.0
					svc.QueueDepth = 1
					svc.ActiveThreads = 10
				}

				s.Logs = append(s.Logs, LogEntry{
					ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
					Timestamp:   nowTime,
					ServiceID:   targetID,
					ServiceName: fault.TargetServiceName,
					Level:       "INFO",
					Message:     fmt.Sprintf("Chaos episode ended: %s on %s terminated.", fault.Title, fault.TargetServiceName),
				})

				// Physically halt chaos on node / chaos-engine
				go func(t, fid string) {
					client := &http.Client{Timeout: 2 * time.Second}
					payload, _ := json.Marshal(map[string]interface{}{"target_node": t, "fault_id": fid})
					_, _ = client.Post(fmt.Sprintf("%s/stop", getChaosEngineURL()), "application/json", bytes.NewBuffer(payload))
				}(targetID, faultID)
			}
		}

		// 2. Refresh node metrics (query real microservice health/metrics endpoints)
		for _, id := range s.ServiceOrder {
			svc, ok := s.Services[id]
			if !ok {
				continue
			}

			// Deterministic fault selection: choose the most recently started fault (Requirement #6)
			var activeFault *ChaosInjection
			for _, f := range s.ActiveFaults {
				if f.TargetServiceID == id {
					if activeFault == nil || f.StartedAt > activeFault.StartedAt || (f.StartedAt == activeFault.StartedAt && f.ID > activeFault.ID) {
						activeFault = f
					}
				}
			}

			// Probe genuine playground microservice endpoint
			probed := false
			if svc.Port > 0 {
				var healthResp struct {
					Status             string  `json:"status"`
					CPU                float64 `json:"cpu"`
					Memory             float64 `json:"memory"`
					LatencyMs          float64 `json:"latency_ms"`
					ErrorRate          float64 `json:"error_rate"`
					ActiveFault        string  `json:"active_fault"`
					RemainingSec       int     `json:"remaining_sec"`
					DBConnectionWaitMs float64 `json:"db_connection_wait_ms"`
					CacheMissRate      float64 `json:"cache_miss_rate"`
					QueueDepth         int     `json:"queue_depth"`
					PacketLossRate     float64 `json:"packet_loss_rate"`
					ActiveThreads      int     `json:"active_threads"`
				}

				probeURLs := []string{
					fmt.Sprintf("http://%s:%d/health", svc.Host, svc.Port),
					fmt.Sprintf("http://localhost:%d/health", svc.Port),
				}

				for _, u := range probeURLs {
					resp, err := httpClient.Get(u)
					if err == nil {
						if json.NewDecoder(resp.Body).Decode(&healthResp) == nil {
							probed = true
							if resp.StatusCode == http.StatusServiceUnavailable || healthResp.Status == "critical" || healthResp.Status == "CRITICAL" {
								svc.Status = "critical"
							} else if healthResp.Status == "degraded" || healthResp.Status == "DEGRADED" {
								svc.Status = "degraded"
							} else {
								svc.Status = "healthy"
							}

							if healthResp.CPU > 0 {
								svc.CPU = math.Round(healthResp.CPU*10) / 10
							}
							if healthResp.Memory > 0 {
								svc.Memory = math.Round(healthResp.Memory*10) / 10
							}
							if healthResp.LatencyMs > 0 {
								svc.Latency = math.Round(healthResp.LatencyMs*10) / 10
							}
							svc.ErrorRate = math.Round(healthResp.ErrorRate*1000) / 1000
							svc.DBWait = healthResp.DBConnectionWaitMs
							svc.CacheMiss = healthResp.CacheMissRate
							svc.PacketLoss = healthResp.PacketLossRate
							svc.ActiveThreads = healthResp.ActiveThreads
							if healthResp.QueueDepth > 0 {
								svc.QueueDepth = healthResp.QueueDepth
							}
						}
						_ = resp.Body.Close()
						break
					}
				}
			}

			// Fallback only if physical microservice process is completely unreachable
			if !probed {
				// Only timestamped ingested telemetry keeps an unprobed service
				// current. Registration state alone does not imply health.
				if !svc.LastIngestTime.IsZero() && time.Since(svc.LastIngestTime) <= 4*time.Second {
					svc.DataStatus = "fresh"
				} else {
					svc.Status = "unknown"
					svc.DataStatus = "unknown"
				}
			}

			// Append to history sliding window (keep last 25 points)
			newPoint := ServiceMetric{
				Timestamp:  nowTime,
				CPU:        math.Round(svc.CPU*10) / 10,
				Memory:     math.Round(svc.Memory*10) / 10,
				Latency:    math.Round(svc.Latency),
				RPS:        math.Round(svc.RPS),
				ErrorRate:  math.Round(svc.ErrorRate*1000) / 1000,
				QueueDepth: svc.QueueDepth,
			}

			if len(svc.History) >= 25 {
				svc.History = append(svc.History[1:], newPoint)
			} else {
				svc.History = append(svc.History, newPoint)
			}
		}

		// 3. Keep alert list fresh based on active states while preserving escalation alerts
		var activeAlerts []LiveAlert
		for _, existing := range s.Alerts {
			if strings.HasPrefix(existing.ID, "alert-esc-") && !existing.Resolved {
				activeAlerts = append(activeAlerts, existing)
			}
		}
		for _, svc := range s.Services {
			if svc.Status == "critical" {
				activeAlerts = append(activeAlerts, LiveAlert{
					ID:          fmt.Sprintf("alt-%s", svc.ID),
					Timestamp:   nowTime,
					ServiceID:   svc.ID,
					ServiceName: svc.Name,
					Severity:    "critical",
					Message:     fmt.Sprintf("Critical failure threshold reached on %s (Latency: %.0fms, CPU: %.1f%%)", svc.Name, svc.Latency, svc.CPU),
					Resolved:    false,
				})
			} else if svc.Status == "degraded" {
				activeAlerts = append(activeAlerts, LiveAlert{
					ID:          fmt.Sprintf("alt-%s", svc.ID),
					Timestamp:   nowTime,
					ServiceID:   svc.ID,
					ServiceName: svc.Name,
					Severity:    "warning",
					Message:     fmt.Sprintf("Performance degradation detected on %s (Latency: %.0fms)", svc.Name, svc.Latency),
					Resolved:    false,
				})
			}
		}
		s.Alerts = activeAlerts

		// Trim logs to last 100 entries
		if len(s.Logs) > 100 {
			s.Logs = s.Logs[len(s.Logs)-100:]
		}

		s.mu.Unlock()

		// 4. Autonomous Recovery Loop (Phase 4 closed loop: chaos -> predict -> decide -> order -> recover -> observe)
		if s.AutonomousRecovery && Safety != nil && Safety.ActiveDispatchCount() == 0 {
			s.mu.RLock()
			needsHealing := len(s.ActiveFaults) > 0
			if !needsHealing {
				for _, svc := range s.Services {
					if svc.Status == "critical" || svc.Status == "degraded" {
						needsHealing = true
						break
					}
				}
			}
			s.mu.RUnlock()

			if needsHealing {
				pred := ComputePredictionOutput()
				if len(pred.RecommendedActions) > 0 {
					for _, rec := range pred.RecommendedActions {
						if !rec.RequiresApproval && !Safety.IsInCooldown(rec.TargetServiceID) {
							// Autonomous execution of DAG-ordered safe action
							go func(tID, aType, aID string) {
								_, _ = ExecuteAutonomousRecovery(tID, aType, aID)
							}(rec.TargetServiceID, rec.ActionType, rec.ID)
							break
						}
					}
				}
			}
		}
	}
}
