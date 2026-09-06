package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type ChaosInjectRequest struct {
	FaultType       string `json:"faultType"`
	TargetServiceID string `json:"targetServiceId"`
	DurationSec     int    `json:"durationSec"`
}

func ChaosInjectHandler(w http.ResponseWriter, r *http.Request) {
	var req ChaosInjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if req.DurationSec <= 0 {
		req.DurationSec = 60
	}

	State.mu.Lock()
	defer State.mu.Unlock()

	svc, exists := State.Services[req.TargetServiceID]
	if !exists {
		http.Error(w, fmt.Sprintf(`{"error":"Target service '%s' not found"}`, req.TargetServiceID), http.StatusNotFound)
		return
	}

	faultTitles := map[string]string{
		"latency":           "Network Latency Injection",
		"cpu_stress":        "CPU Core Exhaustion",
		"memory_leak":       "Heap Memory Leak",
		"kill_service":      "Abrupt Process Termination",
		"packet_loss":       "Network Packet Drop",
		"mq_lag":            "Queue Consumption Backpressure",
		"cache_down":        "Cache Outage",
		"db_lock":           "Database Lock Contention",
		"slow_query":        "Unindexed Slow DB Query",
		"thread_exhaustion": "Worker Thread Pool Starvation",
	}

	title := faultTitles[req.FaultType]
	if title == "" {
		title = fmt.Sprintf("Fault: %s", req.FaultType)
	}

	now := time.Now()
	faultID := fmt.Sprintf("fault-%d", now.UnixNano())
	nowTime := now.Format("15:04:05")

	injection := &ChaosInjection{
		ID:               faultID,
		FaultType:        req.FaultType,
		Title:            title,
		TargetServiceID:  svc.ID,
		TargetServiceName: svc.Name,
		Severity:         "critical",
		DurationSeconds:  req.DurationSec,
		RemainingSeconds: req.DurationSec,
		Status:           "running",
		StartedAt:        nowTime,
	}

	State.ActiveFaults[faultID] = injection

	// Degrade target service metrics immediately
	svc.Status = "critical"
	switch req.FaultType {
	case "cpu_stress":
		svc.CPU = 96.5
		svc.Latency = 280.0
	case "latency":
		svc.Latency = 950.0
		svc.ErrorRate = 0.12
		svc.Status = "degraded"
	case "kill_service":
		svc.CPU = 0.0
		svc.Latency = 5000.0
		svc.ErrorRate = 1.0
	case "memory_leak":
		svc.Memory = 94.0
		svc.Latency = 350.0
	default:
		svc.Latency = 600.0
		svc.ErrorRate = 0.08
		svc.Status = "degraded"
	}

	// Record event log
	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", now.UnixNano()),
		Timestamp:   nowTime,
		ServiceID:   svc.ID,
		ServiceName: svc.Name,
		Level:       "ERROR",
		Message:     fmt.Sprintf("CHAOS ENGINE INJECTED: %s on %s (%ds duration)", title, svc.Name, req.DurationSec),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(injection)
}

func ChaosStopHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	State.mu.Lock()
	defer State.mu.Unlock()

	fault, exists := State.ActiveFaults[id]
	if !exists {
		// Attempt finding by target service ID if not by fault ID
		for fid, f := range State.ActiveFaults {
			if f.TargetServiceID == id {
				fault = f
				id = fid
				exists = true
				break
			}
		}
	}

	if !exists {
		http.Error(w, `{"error":"Active fault not found"}`, http.StatusNotFound)
		return
	}

	delete(State.ActiveFaults, id)

	if svc, ok := State.Services[fault.TargetServiceID]; ok {
		svc.Status = "healthy"
		svc.CPU = 22.0
		svc.Latency = 18.0
		svc.ErrorRate = 0.001
	}

	nowTime := time.Now().Format("15:04:05")
	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", time.Now().UnixNano()),
		Timestamp:   nowTime,
		ServiceID:   fault.TargetServiceID,
		ServiceName: fault.TargetServiceName,
		Level:       "INFO",
		Message:     fmt.Sprintf("CHAOS ENGINE CLEARED: Fault %s on %s terminated.", fault.Title, fault.TargetServiceName),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"faultId": id,
	})
}

func ChaosActiveHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	faults := make([]*ChaosInjection, 0, len(State.ActiveFaults))
	for _, f := range State.ActiveFaults {
		faults = append(faults, f)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(faults)
}

// Backwards-compatible legacy handler
func ChaosHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ChaosInjectHandler(w, r)
		return
	}
	ChaosActiveHandler(w, r)
}
