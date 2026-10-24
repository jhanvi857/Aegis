package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
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
}

type Microservice struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Status        string          `json:"status"` // healthy, degraded, critical
	Replicas      int             `json:"replicas"`
	MaxReplicas   int             `json:"maxReplicas"`
	CPU           float64         `json:"cpu"`
	Memory        float64         `json:"memory"`
	Latency       float64         `json:"latency"`
	RPS           float64         `json:"rps"`
	ErrorRate     float64         `json:"errorRate"`
	QueueDepth    int             `json:"queueDepth"`
	DBWait        float64         `json:"dbConnectionWaitMs"`
	CacheMiss     float64         `json:"cacheMissRate"`
	PacketLoss    float64         `json:"packetLossRate"`
	ActiveThreads int             `json:"activeThreads"`
	Version       string          `json:"version"`
	Uptime        string          `json:"uptime"`
	Dependencies  []string        `json:"dependencies"`
	History       []ServiceMetric `json:"history"`
	Host          string          `json:"-"`
	Port          int             `json:"-"`
}

type ChaosInjection struct {
	ID               string `json:"id"`
	FaultType        string `json:"faultType"`
	Title            string `json:"title"`
	TargetServiceID  string `json:"targetServiceId"`
	TargetServiceName string `json:"targetServiceName"`
	Severity         string `json:"severity"`
	DurationSeconds  int    `json:"durationSeconds"`
	RemainingSeconds int    `json:"remainingSeconds"`
	Status           string `json:"status"`
	StartedAt        string `json:"startedAt"`
}

type AffectedNodeInfo struct {
	NodeID      string  `json:"nodeId"`
	HopDistance int32   `json:"hopDistance"`
	ETASeconds  int64   `json:"etaSeconds"`
	RiskScore   float64 `json:"riskScore"`
}

type PredictionOutput struct {
	FailureProbability   float64            `json:"failureProbability"`
	Confidence           float64            `json:"confidence"`
	EstimatedFailureSec  int                `json:"estimatedFailureSec"`
	RootCauseServiceID   string             `json:"rootCauseServiceId"`
	RootCauseServiceName string             `json:"rootCauseServiceName"`
	RootCauseReason      string             `json:"rootCauseReason"`
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
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	TargetServiceID  string  `json:"targetServiceId"`
	ActionType       string  `json:"actionType"` // restart, scale, flush_cache, increase_pool, reroute_traffic
	Confidence       float64 `json:"confidence"`
	Status           string  `json:"status"`
	RiskLevel        string  `json:"riskLevel,omitempty"`
	RequiresApproval bool    `json:"requiresApproval,omitempty"`
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
	RecoveryHistory       []RecoveryHistoryItem
	Experiments           []Experiment
	RawTopologyYAML       []byte
	LatestRecommendations []RecoveryAction
	ApprovedPlans         map[string]bool
	ApprovedRecords       map[string]ApprovalRecord
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
		RecoveryHistory:       make([]RecoveryHistoryItem, 0),
		Experiments:           make([]Experiment, 0),
		LatestRecommendations: make([]RecoveryAction, 0),
		ApprovedPlans:         make(map[string]bool),
		ApprovedRecords:       make(map[string]ApprovalRecord),
	}
}

var (
	State           *ControlPlaneState = newControlPlaneState()
	stateTickerOnce sync.Once
)

func InitState() {
	demoMode := strings.EqualFold(os.Getenv("DEMO_MODE"), "true") || strings.EqualFold(os.Getenv("AEGIS_DEMO_MODE"), "true")
	autoRecovery := true
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
	State.RecoveryHistory = make([]RecoveryHistoryItem, 0)
	State.Experiments = make([]Experiment, 0)
	State.LatestRecommendations = make([]RecoveryAction, 0)
	State.ApprovedPlans = make(map[string]bool)
	State.ApprovedRecords = make(map[string]ApprovalRecord)
	State.DemoMode = demoMode
	State.AutonomousRecovery = autoRecovery
	State.mu.Unlock()

	topoPath := os.Getenv("TOPOLOGY_PATH")
	if topoPath == "" {
		candidates := []string{
			"configs/topology.yaml",
			"../configs/topology.yaml",
			"../../configs/topology.yaml",
			"/etc/aegis/topology.yaml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				topoPath = c
				break
			}
		}
	}

	_ = LoadTopology(topoPath)

	// Start background maintenance loop once
	stateTickerOnce.Do(func() {
		// Do not start background maintenance ticker during automated test runs
		if flag.Lookup("test.v") != nil || os.Getenv("GO_TEST") == "1" {
			return
		}
		go State.runTicker()
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

	nowTime := time.Now().Format("15:04:05")
	for _, n := range topo.Nodes {
		baseCPU := 25.0
		baseMem := 40.0
		baseLat := 18.0
		baseRPS := 250.0
		if State.DemoMode {
			baseCPU += rand.Float64() * 15.0
			baseMem += rand.Float64() * 15.0
			baseLat += rand.Float64() * 15.0
			baseRPS += rand.Float64() * 200.0
		}

		history := make([]ServiceMetric, 20)
		for i := 0; i < 20; i++ {
			t := time.Now().Add(-time.Duration(20-i) * 2 * time.Second).Format("15:04:05")
			cpuVal := math.Round(baseCPU)
			memVal := math.Round(baseMem)
			latVal := math.Round(baseLat)
			rpsVal := math.Round(baseRPS)
			if State.DemoMode {
				cpuVal = math.Round(baseCPU + rand.Float64()*4 - 2)
				memVal = math.Round(baseMem + rand.Float64()*2 - 1)
				latVal = math.Round(baseLat + rand.Float64()*4 - 2)
				rpsVal = math.Round(baseRPS + rand.Float64()*20 - 10)
			}
			history[i] = ServiceMetric{
				Timestamp:  t,
				CPU:        cpuVal,
				Memory:     memVal,
				Latency:    latVal,
				RPS:        rpsVal,
				ErrorRate:  0.001,
				QueueDepth: 1,
			}
		}

		svcType := n.Type
		if svcType == "" {
			svcType = "service"
		}

		svc := &Microservice{
			ID:           n.ID,
			Name:         n.Name,
			Type:         svcType,
			Status:       "healthy",
			Replicas:     3,
			MaxReplicas:  10,
			CPU:          math.Round(baseCPU),
			Memory:       math.Round(baseMem),
			Latency:      math.Round(baseLat),
			RPS:          math.Round(baseRPS),
			ErrorRate:    0.001,
			QueueDepth:   1,
			Version:      "v1.0.0",
			Uptime:       "14d 6h",
			Dependencies: n.Dependencies,
			History:      history,
			Host:         n.Host,
			Port:         n.Port,
		}
		State.Services[n.ID] = svc
		State.ServiceOrder = append(State.ServiceOrder, n.ID)
	}

	// Initialize standard experiments including leaf-node autonomous cases
	if len(State.ServiceOrder) > 0 {
		firstWorker := "node-a"
		if len(State.ServiceOrder) > 1 {
			firstWorker = State.ServiceOrder[1]
		}
		secondWorker := "node-c"
		if len(State.ServiceOrder) > 3 {
			secondWorker = State.ServiceOrder[3]
		}
		leafWorkerD := "node-d"
		if _, ok := State.Services[leafWorkerD]; !ok {
			leafWorkerD = State.ServiceOrder[len(State.ServiceOrder)-1]
		}
		leafWorkerB := "node-b"
		if _, ok := State.Services[leafWorkerB]; !ok {
			leafWorkerB = State.ServiceOrder[0]
		}

		State.Experiments = []Experiment{
			{
				ID:                 "exp-1",
				Name:               "Upstream CPU Core Saturation",
				Description:        "Tests failure prediction & scale actions under CPU stress",
				FaultType:          "cpu_stress",
				TargetService:      firstWorker,
				PredictionAccuracy: 96.4,
				DetectionTimeSec:   2,
				MttrSec:            14,
				Status:             "idle",
			},
			{
				ID:                 "exp-2",
				Name:               "Downstream Pipeline Network Latency Spike",
				Description:        "Validates cascade propagation analysis and traffic rerouting",
				FaultType:          "latency",
				TargetService:      secondWorker,
				PredictionAccuracy: 94.2,
				DetectionTimeSec:   3,
				MttrSec:            18,
				Status:             "idle",
			},
			{
				ID:                 "exp-3",
				Name:               "Service Abrupt Crash & Recovery",
				Description:        "Simulates SIGKILL on worker and tests automated rolling restart",
				FaultType:          "kill_service",
				TargetService:      firstWorker,
				PredictionAccuracy: 98.1,
				DetectionTimeSec:   1,
				MttrSec:            12,
				Status:             "idle",
			},
			{
				ID:                 "exp-4",
				Name:               "Leaf-Node Database Contention (Autonomous)",
				Description:        "Tests leaf node with blast radius 0; executes autonomously without approval gate (Risk < 0.35)",
				FaultType:          "db_lock",
				TargetService:      leafWorkerD,
				PredictionAccuracy: 97.5,
				DetectionTimeSec:   2,
				MttrSec:            8,
				Status:             "idle",
			},
			{
				ID:                 "exp-5",
				Name:               "Leaf-Node Cache Degradation (Autonomous)",
				Description:        "Tests autonomous cache flush on edge service without operator intervention",
				FaultType:          "cache_down",
				TargetService:      leafWorkerB,
				PredictionAccuracy: 98.0,
				DetectionTimeSec:   1,
				MttrSec:            6,
				Status:             "idle",
			},
		}
	}

	log.Printf("[api-gateway] Initialized %d services from %s", len(State.Services), topoPath)
	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
		Timestamp:   nowTime,
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
				if activeFault != nil {
					switch activeFault.FaultType {
					case "cpu_stress":
						svc.CPU = 96.5
						svc.Latency = 240.0
						svc.Status = "critical"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "latency":
						svc.Latency = 950.0
						svc.ErrorRate = 0.08
						svc.Status = "degraded"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "kill_service":
						svc.CPU = 0.0
						svc.Latency = 5000.0
						svc.ErrorRate = 1.0
						svc.Status = "critical"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "memory_leak":
						svc.Memory = 94.0
						svc.Latency = 280.0
						svc.Status = "critical"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "db_lock":
						svc.DBWait = 950.0
						svc.Latency = 1200.0
						svc.ErrorRate = 0.15
						svc.Status = "critical"
						svc.CacheMiss = 0.02
					case "slow_query":
						svc.DBWait = 1200.0
						svc.Latency = 1400.0
						svc.ErrorRate = 0.08
						svc.Status = "degraded"
						svc.CacheMiss = 0.02
					case "cache_down":
						svc.CacheMiss = 0.95
						svc.Latency = 450.0
						svc.ErrorRate = 0.04
						svc.Status = "degraded"
						svc.DBWait = 0.0
					case "mq_lag":
						svc.QueueDepth = 850
						svc.Latency = 600.0
						svc.ErrorRate = 0.05
						svc.Status = "degraded"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "thread_exhaustion":
						svc.Latency = 800.0
						svc.ErrorRate = 0.35
						svc.ActiveThreads = 95
						svc.Status = "critical"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					case "packet_loss":
						svc.Latency = 250.0
						svc.ErrorRate = 0.25
						svc.PacketLoss = 0.25
						svc.Status = "critical"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					default:
						svc.Latency = 550.0
						svc.ErrorRate = 0.05
						svc.Status = "degraded"
						svc.DBWait = 0.0
						svc.CacheMiss = 0.02
					}
				} else {
					if s.DemoMode {
						svc.CPU = math.Round(math.Max(10.0, math.Min(50.0, svc.CPU+rand.Float64()*2.0-1.0))*10) / 10
						svc.Memory = math.Round(math.Max(20.0, math.Min(60.0, svc.Memory+rand.Float64()*2.0-1.0))*10) / 10
						svc.Latency = math.Round(math.Max(8.0, math.Min(35.0, svc.Latency+rand.Float64()*2.0-1.0))*10) / 10
						svc.RPS = math.Round(math.Max(50.0, math.Min(500.0, svc.RPS+rand.Float64()*10.0-5.0))*10) / 10
					}
					svc.ErrorRate = 0.001
					svc.Status = "healthy"
					svc.DBWait = 0.0
					svc.CacheMiss = 0.02
					svc.QueueDepth = 0
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
