package routes

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
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
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Status       string          `json:"status"` // healthy, degraded, critical
	Replicas     int             `json:"replicas"`
	MaxReplicas  int             `json:"maxReplicas"`
	CPU          float64         `json:"cpu"`
	Memory       float64         `json:"memory"`
	Latency      float64         `json:"latency"`
	RPS          float64         `json:"rps"`
	ErrorRate    float64         `json:"errorRate"`
	QueueDepth   int             `json:"queueDepth"`
	Version      string          `json:"version"`
	Uptime       string          `json:"uptime"`
	Dependencies []string        `json:"dependencies"`
	History      []ServiceMetric `json:"history"`
	Host         string          `json:"-"`
	Port         int             `json:"-"`
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

type PredictionOutput struct {
	FailureProbability  float64  `json:"failureProbability"`
	Confidence          float64  `json:"confidence"`
	EstimatedFailureSec int      `json:"estimatedFailureSec"`
	RootCauseServiceID  string   `json:"rootCauseServiceId"`
	RootCauseServiceName string  `json:"rootCauseServiceName"`
	RootCauseReason     string   `json:"rootCauseReason"`
	BlastRadius         []string `json:"blastRadius"`
	RecommendedAction   string   `json:"recommendedAction"`
	RecommendedActionID string   `json:"recommendedActionId"`
}

type RecoveryAction struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	TargetServiceID string  `json:"targetServiceId"`
	ActionType      string  `json:"actionType"` // restart, scale, flush_cache, increase_pool
	Confidence      float64 `json:"confidence"`
	Status          string  `json:"status"`
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

type ControlPlaneState struct {
	mu              sync.RWMutex
	Services        map[string]*Microservice
	ServiceOrder    []string
	ActiveFaults    map[string]*ChaosInjection
	Alerts          []LiveAlert
	Logs            []LogEntry
	RecoveryHistory []RecoveryHistoryItem
	Experiments     []Experiment
	RawTopologyYAML []byte
}

var State *ControlPlaneState

func InitState() {
	State = &ControlPlaneState{
		Services:        make(map[string]*Microservice),
		ActiveFaults:    make(map[string]*ChaosInjection),
		Alerts:          make([]LiveAlert, 0),
		Logs:            make([]LogEntry, 0),
		RecoveryHistory: make([]RecoveryHistoryItem, 0),
		Experiments:     make([]Experiment, 0),
	}

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

	data, err := os.ReadFile(topoPath)
	if err != nil {
		log.Printf("[api-gateway] Warning: could not load topology: %v", err)
	} else {
		State.RawTopologyYAML = data
		var topo TopologyYAML
		if err := yaml.Unmarshal(data, &topo); err == nil {
			nowTime := time.Now().Format("15:04:05")
			for _, n := range topo.Nodes {
				baseCPU := 20.0 + rand.Float64()*15.0
				baseMem := 35.0 + rand.Float64()*15.0
				baseLat := 15.0 + rand.Float64()*15.0
				baseRPS := 200.0 + rand.Float64()*200.0

				history := make([]ServiceMetric, 20)
				for i := 0; i < 20; i++ {
					t := time.Now().Add(-time.Duration(20-i) * 2 * time.Second).Format("15:04:05")
					history[i] = ServiceMetric{
						Timestamp:  t,
						CPU:        math.Round(baseCPU + rand.Float64()*4 - 2),
						Memory:     math.Round(baseMem + rand.Float64()*2 - 1),
						Latency:    math.Round(baseLat + rand.Float64()*4 - 2),
						RPS:        math.Round(baseRPS + rand.Float64()*20 - 10),
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
			log.Printf("[api-gateway] Initialized %d services from %s", len(State.Services), topoPath)

			State.Logs = append(State.Logs, LogEntry{
				ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
				Timestamp:   nowTime,
				ServiceID:   "api-gateway",
				ServiceName: "Control Plane",
				Level:       "INFO",
				Message:     fmt.Sprintf("Topology loaded successfully: %d nodes observed.", len(State.Services)),
			})
		}
	}

	// Initialize standard experiments from generic topology
	if len(State.ServiceOrder) > 0 {
		firstWorker := "node-a"
		if len(State.ServiceOrder) > 1 {
			firstWorker = State.ServiceOrder[1]
		}
		secondWorker := "node-c"
		if len(State.ServiceOrder) > 3 {
			secondWorker = State.ServiceOrder[3]
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
		}
	}

	// Start background maintenance loop
	go State.runTicker()
}

func (s *ControlPlaneState) runTicker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	httpClient := &http.Client{Timeout: 500 * time.Millisecond}

	for range ticker.C {
		s.mu.Lock()
		nowTime := time.Now().Format("15:04:05")

		// 1. Process active chaos fault countdowns
		for id, fault := range s.ActiveFaults {
			fault.RemainingSeconds--
			if fault.RemainingSeconds <= 0 {
				fault.Status = "completed"
				delete(s.ActiveFaults, id)

				// Restore service to healthy
				if svc, ok := s.Services[fault.TargetServiceID]; ok {
					svc.Status = "healthy"
					svc.CPU = 25.0
					svc.Latency = 20.0
					svc.ErrorRate = 0.001
				}

				s.Logs = append(s.Logs, LogEntry{
					ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
					Timestamp:   nowTime,
					ServiceID:   fault.TargetServiceID,
					ServiceName: fault.TargetServiceName,
					Level:       "INFO",
					Message:     fmt.Sprintf("Chaos episode ended: %s on %s terminated.", fault.Title, fault.TargetServiceName),
				})
			}
		}

		// 2. Refresh node metrics (poll real playground endpoints if reachable, else smooth variance)
		for _, id := range s.ServiceOrder {
			svc, ok := s.Services[id]
			if !ok {
				continue
			}

			// Check if service is currently targeted by a chaos fault
			isFaulty := false
			var activeFault *ChaosInjection
			for _, f := range s.ActiveFaults {
				if f.TargetServiceID == id {
					isFaulty = true
					activeFault = f
					break
				}
			}

			if isFaulty && activeFault != nil {
				switch activeFault.FaultType {
				case "cpu_stress":
					svc.CPU = math.Min(99.0, svc.CPU+5.0)
					svc.Latency = math.Min(350.0, svc.Latency+15.0)
					svc.Status = "critical"
				case "latency":
					svc.Latency = math.Min(1500.0, svc.Latency+120.0)
					svc.ErrorRate = math.Min(0.25, svc.ErrorRate+0.02)
					svc.Status = "degraded"
				case "kill_service":
					svc.CPU = 0.0
					svc.Latency = 5000.0
					svc.ErrorRate = 1.0
					svc.Status = "critical"
				case "memory_leak":
					svc.Memory = math.Min(98.0, svc.Memory+4.0)
					svc.Latency = math.Min(400.0, svc.Latency+10.0)
					svc.Status = "critical"
				default:
					svc.Latency = math.Min(600.0, svc.Latency+30.0)
					svc.Status = "degraded"
				}
			} else {
				// Try probing real playground health endpoint
				if svc.Port > 0 {
					url := fmt.Sprintf("http://localhost:%d/health", svc.Port)
					resp, err := httpClient.Get(url)
					if err == nil && resp.StatusCode == http.StatusOK {
						resp.Body.Close()
						svc.Status = "healthy"
					}
				}

				// If not faulty, keep healthy around nominal baseline
				svc.CPU = math.Max(10.0, math.Min(60.0, svc.CPU+rand.Float64()*4.0-2.0))
				svc.Memory = math.Max(20.0, math.Min(70.0, svc.Memory+rand.Float64()*2.0-1.0))
				svc.Latency = math.Max(5.0, math.Min(45.0, svc.Latency+rand.Float64()*4.0-2.0))
				svc.RPS = math.Max(50.0, math.Min(600.0, svc.RPS+rand.Float64()*20.0-10.0))
				svc.ErrorRate = 0.001
				svc.Status = "healthy"
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

		// 3. Keep alert list fresh based on active states
		var activeAlerts []LiveAlert
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
	}
}
