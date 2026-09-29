package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
)

type ChaosInjectRequest struct {
	FaultType       string            `json:"faultType"`
	TargetServiceID string            `json:"targetServiceId"`
	DurationSec     int               `json:"durationSec"`
	Parameters      map[string]string `json:"parameters,omitempty"`
}

func getChaosEngineURL() string {
	url := os.Getenv("CHAOS_ENGINE_URL")
	if url == "" {
		return "http://localhost:8091"
	}
	return url
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

	// Guard against concurrent faults on the same target node (Requirement #6 & #9)
	for _, f := range State.ActiveFaults {
		if f.TargetServiceID == req.TargetServiceID {
			http.Error(w, fmt.Sprintf(`{"error":"A chaos fault ('%s') is already active on service '%s'. Only one fault per node is permitted at a time."}`, f.FaultType, req.TargetServiceID), http.StatusConflict)
			return
		}
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
		ID:                faultID,
		FaultType:         req.FaultType,
		Title:             title,
		TargetServiceID:   svc.ID,
		TargetServiceName: svc.Name,
		Severity:          "critical",
		DurationSeconds:   req.DurationSec,
		RemainingSeconds:  req.DurationSec,
		Status:            "running",
		StartedAt:         nowTime,
	}

	State.ActiveFaults[faultID] = injection

	// Forward physical fault injection to Chaos Engine
	go func(target, fType string, dur int, params map[string]string) {
		client := &http.Client{Timeout: 3 * time.Second}
		payload, _ := json.Marshal(map[string]interface{}{
			"fault_type":   fType,
			"target_node":  target,
			"duration_sec": dur,
			"parameters":   params,
		})

		chaosURL := fmt.Sprintf("%s/inject", getChaosEngineURL())
		resp, err := client.Post(chaosURL, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			log.Printf("[api-gateway] Warning: could not forward fault to chaos-engine at %s: %v", chaosURL, err)
			return
		}
		_ = resp.Body.Close()
		log.Printf("[api-gateway] Successfully forwarded %s on %s to chaos-engine", fType, target)
	}(req.TargetServiceID, req.FaultType, req.DurationSec, req.Parameters)

	// Note: We do NOT write fabricated metrics here in gateway memory.
	// Telemetry flows genuinely back from the physical microservice into gateway state.

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

	// Forward stop request to Chaos Engine to physically revert fault
	go func(target, fid string) {
		client := &http.Client{Timeout: 3 * time.Second}
		payload, _ := json.Marshal(map[string]interface{}{
			"target_node": target,
			"fault_id":    fid,
		})

		stopURL := fmt.Sprintf("%s/stop", getChaosEngineURL())
		resp, err := client.Post(stopURL, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			log.Printf("[api-gateway] Warning: could not forward stop to chaos-engine at %s: %v", stopURL, err)
			return
		}
		_ = resp.Body.Close()
		log.Printf("[api-gateway] Successfully forwarded stop for %s to chaos-engine", target)
	}(fault.TargetServiceID, id)

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
