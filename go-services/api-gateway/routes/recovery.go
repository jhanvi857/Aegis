package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ExecuteRecoveryRequest struct {
	ActionID        string `json:"actionId"`
	TargetServiceID string `json:"targetServiceId,omitempty"`
	ActionType      string `json:"actionType,omitempty"`
}

func GetRecommendationsHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	recommendations := make([]RecoveryAction, 0)

	for _, id := range State.ServiceOrder {
		svc := State.Services[id]
		if svc.Status != "healthy" {
			actionType := "restart"
			title := fmt.Sprintf("Restart %s Container", svc.Name)
			desc := fmt.Sprintf("Execute rolling container restart on %s to flush degraded memory and connections.", svc.Name)
			riskLevel := "LOW"
			requiresApproval := false

			if svc.Latency > 800.0 || svc.ErrorRate >= 0.40 {
				actionType = "reroute_traffic"
				title = fmt.Sprintf("Divert Ingress Traffic from %s", svc.Name)
				desc = fmt.Sprintf("Critical latency/error cascade on %s. Temporarily reroute ingress traffic to healthy paths.", svc.Name)
				riskLevel = "HIGH"
				requiresApproval = true
			} else if svc.CPU > 80.0 {
				actionType = "scale"
				title = fmt.Sprintf("Scale %s Horizontal Pod Replicas", svc.Name)
				desc = fmt.Sprintf("Increase %s replicas from %d to %d to distribute compute load.", svc.Name, svc.Replicas, svc.Replicas+2)
				riskLevel = "MEDIUM"
				requiresApproval = false
			} else if svc.Type == "cache" {
				actionType = "flush_cache"
				title = fmt.Sprintf("Flush Corrupted Cache on %s", svc.Name)
				desc = fmt.Sprintf("Evict keys and reset memory allocation on %s.", svc.Name)
				riskLevel = "LOW"
				requiresApproval = false
			} else if svc.Type == "persistence" || svc.Type == "database" {
				actionType = "increase_pool"
				title = fmt.Sprintf("Expand Connection Pool on %s", svc.Name)
				desc = fmt.Sprintf("Increase database/connection pool capacity on %s.", svc.Name)
				riskLevel = "MEDIUM"
				requiresApproval = false
			}

			recommendations = append(recommendations, RecoveryAction{
				ID:               fmt.Sprintf("rec-%s-%s", actionType, svc.ID),
				Title:            title,
				Description:      desc,
				TargetServiceID:  svc.ID,
				ActionType:       actionType,
				Confidence:       95.4,
				Status:           "idle",
				RiskLevel:        riskLevel,
				RequiresApproval: requiresApproval,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recommendations)
}

func ExecuteRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRecoveryRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	State.mu.Lock()
	defer State.mu.Unlock()

	targetID := req.TargetServiceID
	// If targetServiceId not provided directly, parse from actionId if format is rec-action-nodeId
	if targetID == "" && len(req.ActionID) > 4 {
		for id := range State.Services {
			if len(req.ActionID) >= len(id) && req.ActionID[len(req.ActionID)-len(id):] == id {
				targetID = id
				break
			}
		}
	}

	// Fallback to first non-healthy node if target still not found
	if targetID == "" {
		for _, id := range State.ServiceOrder {
			if State.Services[id].Status != "healthy" {
				targetID = id
				break
			}
		}
	}

	svc, exists := State.Services[targetID]
	if !exists {
		// If all healthy, pick first node
		if len(State.ServiceOrder) > 0 {
			targetID = State.ServiceOrder[0]
			svc = State.Services[targetID]
		} else {
			http.Error(w, `{"error":"No service to remediate"}`, http.StatusBadRequest)
			return
		}
	}

	riskBefore := 85.0
	if svc.Status == "critical" {
		riskBefore = 92.0
	} else if svc.Status == "degraded" {
		riskBefore = 65.0
	}

	// Remediate service to healthy
	svc.Status = "healthy"
	svc.CPU = 20.0
	svc.Latency = 16.0
	svc.ErrorRate = 0.001

	if req.ActionType == "scale" {
		svc.Replicas = svc.Replicas + 2
	}

	// Clear any active chaos faults on this service
	for fid, f := range State.ActiveFaults {
		if f.TargetServiceID == targetID {
			delete(State.ActiveFaults, fid)
		}
	}

	now := time.Now()
	nowTime := now.Format("15:04:05")
	actionTitle := fmt.Sprintf("Autonomous Remediation (%s)", svc.Name)
	if req.ActionType != "" {
		actionTitle = fmt.Sprintf("Remediation: %s on %s", req.ActionType, svc.Name)
	}

	histItem := RecoveryHistoryItem{
		ID:            fmt.Sprintf("rec-hist-%d", now.UnixNano()),
		Timestamp:     nowTime,
		ActionTitle:   actionTitle,
		TargetService: svc.Name,
		Status:        "Completed",
		Duration:      "1.4s",
		RiskBefore:    riskBefore,
		RiskAfter:     12.0,
	}

	State.RecoveryHistory = append([]RecoveryHistoryItem{histItem}, State.RecoveryHistory...)

	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", now.UnixNano()),
		Timestamp:   nowTime,
		ServiceID:   svc.ID,
		ServiceName: svc.Name,
		Level:       "INFO",
		Message:     fmt.Sprintf("RECOVERY EXECUTED: %s. Risk dropped from %.0f%% to 12%%.", actionTitle, riskBefore),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"actionId": req.ActionID,
		"history":  histItem,
	})
}

func GetRecoveryHistoryHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	history := State.RecoveryHistory
	if history == nil {
		history = make([]RecoveryHistoryItem, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(history)
}

// Backwards-compatible legacy handler
func RecoveryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ExecuteRecoveryHandler(w, r)
		return
	}
	GetRecommendationsHandler(w, r)
}

func GetPendingRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	defer State.mu.RUnlock()

	pending := make([]RecoveryAction, 0)
	for _, id := range State.ServiceOrder {
		svc := State.Services[id]
		if svc.Status != "healthy" && (svc.Latency > 800.0 || svc.ErrorRate >= 0.40) {
			pending = append(pending, RecoveryAction{
				ID:               fmt.Sprintf("rec-reroute_traffic-%s", svc.ID),
				Title:            fmt.Sprintf("Divert Ingress Traffic from %s", svc.Name),
				Description:      fmt.Sprintf("Critical cascade on %s requires operator approval.", svc.Name),
				TargetServiceID:  svc.ID,
				ActionType:       "reroute_traffic",
				Confidence:       94.2,
				Status:           "pending_approval",
				RiskLevel:        "HIGH",
				RequiresApproval: true,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pending)
}

func ApproveRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	// Re-route to execute handler to remediate
	ExecuteRecoveryHandler(w, r)
}
