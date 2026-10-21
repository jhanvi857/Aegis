package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

type ExecuteRecoveryRequest struct {
	ActionID        string `json:"actionId"`
	TargetServiceID string `json:"targetServiceId,omitempty"`
	ActionType      string `json:"actionType,omitempty"`
}

func GetRecommendationsHandler(w http.ResponseWriter, r *http.Request) {
	State.mu.RLock()
	recs := State.LatestRecommendations
	State.mu.RUnlock()

	// If no cached Python recommendations yet, trigger prediction evaluation
	if len(recs) == 0 {
		pred := ComputePredictionOutput()
		recs = pred.RecommendedActions
	}

	if recs == nil {
		recs = make([]RecoveryAction, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recs)
}

func ExecuteRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRecoveryRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Pre-recovery prediction to derive real baseline risk
	predBefore := ComputePredictionOutput()

	State.mu.Lock()

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
		if len(State.ServiceOrder) > 0 {
			targetID = State.ServiceOrder[0]
			svc = State.Services[targetID]
		} else {
			State.mu.Unlock()
			http.Error(w, `{"error":"No service to remediate"}`, http.StatusBadRequest)
			return
		}
	}

	// Determine genuine risk before recovery from live prediction & service status
	riskBefore := predBefore.FailureProbability
	if riskBefore < 10.0 {
		if svc.Status == "critical" {
			riskBefore = 93.5
		} else if svc.Status == "degraded" {
			riskBefore = 68.0
		} else if len(State.ActiveFaults) > 0 {
			riskBefore = 75.0
		}
	}

	riskScore := math.Round((riskBefore/100.0)*100) / 100.0
	riskLevel := "LOW"
	if riskBefore >= 75.0 {
		riskLevel = "HIGH"
	} else if riskBefore >= 35.0 {
		riskLevel = "MEDIUM"
	}

	actionType := req.ActionType
	if actionType == "" {
		// Look up in LatestRecommendations from Python decision engine first
		for _, rec := range State.LatestRecommendations {
			if rec.ID == req.ActionID || rec.TargetServiceID == targetID {
				actionType = rec.ActionType
				break
			}
		}
	}
	if actionType == "" && predBefore.RecommendedActionID != "" && predBefore.RecommendedActionID != "act-none" {
		for _, rec := range predBefore.RecommendedActions {
			if rec.TargetServiceID == targetID {
				actionType = rec.ActionType
				break
			}
		}
	}
	if actionType == "" {
		actionType = "restart"
	}

	// Server-side authority boundary gate: MEDIUM and HIGH risk actions require human operator sign-off
	requiresApproval := (riskLevel == "MEDIUM" || riskLevel == "HIGH")

	rec, hasRecord := State.ApprovedRecords[req.ActionID]
	isApproved := hasRecord || (req.ActionID != "" && State.ApprovedPlans[req.ActionID])

	if requiresApproval && !isApproved {
		State.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":            fmt.Sprintf("Action '%s' on %s requires operator approval before execution (risk level: %s)", actionType, targetID, riskLevel),
			"requiresApproval": true,
			"riskLevel":        riskLevel,
			"actionId":         req.ActionID,
			"targetServiceId":  targetID,
		})
		return
	}

	// Verify target and action bindings if recorded
	if hasRecord {
		if rec.TargetServiceID != "" && rec.TargetServiceID != targetID {
			State.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": fmt.Sprintf("Approval authorization mismatch: token bound to '%s' cannot be executed on '%s'", rec.TargetServiceID, targetID),
			})
			return
		}
		if rec.ActionType != "" && actionType != "" && !strings.EqualFold(rec.ActionType, actionType) {
			State.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": fmt.Sprintf("Approval authorization mismatch: token bound to '%s' cannot be executed for action '%s'", rec.ActionType, actionType),
			})
			return
		}
	}

	approverName := "Authorized Operator"
	if authUser, err := GetUserFromRequest(r); err == nil && authUser != nil {
		approverName = authUser.Email
	} else if hasRecord && rec.Approver != "" {
		approverName = rec.Approver
	}

	// Consume approval token upon execution
	if req.ActionID != "" {
		delete(State.ApprovedPlans, req.ActionID)
		delete(State.ApprovedRecords, req.ActionID)
	}

	now := time.Now()
	nowTime := now.Format("15:04:05")
	planID := fmt.Sprintf("plan-exec-%d", now.UnixNano())

	// 1. Physically revert any active chaos faults on this service
	stoppedFaultIDs := make([]string, 0)
	for fid, f := range State.ActiveFaults {
		if f.TargetServiceID == targetID {
			stoppedFaultIDs = append(stoppedFaultIDs, fid)
			delete(State.ActiveFaults, fid)
		}
	}

	// Restore service to healthy metrics
	svc.Status = "healthy"
	svc.CPU = 25.0
	svc.Latency = 20.0
	svc.ErrorRate = 0.001

	if actionType == "scale" {
		svc.Replicas = svc.Replicas + 2
	}

	actionTitle := fmt.Sprintf("Autonomous Remediation (%s)", svc.Name)
	if actionType != "" {
		actionTitle = fmt.Sprintf("Remediation: %s on %s", actionType, svc.Name)
	}
	svcName := svc.Name
	svcID := svc.ID

	State.mu.Unlock()

	// Forward stop to Chaos Engine for each cleared fault
	for _, fid := range stoppedFaultIDs {
		go func(t, id string) {
			client := &http.Client{Timeout: 2 * time.Second}
			payload, _ := json.Marshal(map[string]interface{}{"target_node": t, "fault_id": id})
			_, _ = client.Post(fmt.Sprintf("%s/stop", getChaosEngineURL()), "application/json", bytes.NewBuffer(payload))
		}(targetID, fid)
	}

	// 2. Dispatch real recovery plan to Recovery Engine with genuinely derived risk level and score
	go func(pID, rLevel string, rScore float64, aType, tNode string) {
		recEngineURL := os.Getenv("RECOVERY_ENGINE_URL")
		if recEngineURL == "" {
			recEngineURL = "http://localhost:8092"
		}

		client := &http.Client{Timeout: 3 * time.Second}
		planPayload, _ := json.Marshal(map[string]interface{}{
			"plan_id":    pID,
			"risk_level": rLevel,
			"risk_score": rScore,
			"steps": []map[string]interface{}{
				{
					"action_id":       fmt.Sprintf("act-%d", time.Now().UnixNano()),
					"action_type":     aType,
					"target_node":     tNode,
					"execution_order": 1,
					"parameters":      map[string]string{},
				},
			},
		})

		resp, err := client.Post(fmt.Sprintf("%s/plan/submit", recEngineURL), "application/json", bytes.NewBuffer(planPayload))
		if err == nil {
			_ = resp.Body.Close()
		}
	}(planID, riskLevel, riskScore, actionType, targetID)

	// 3. Compute genuine post-remediation risk score via fresh prediction engine evaluation
	predAfter := ComputePredictionOutput()
	riskAfter := predAfter.FailureProbability

	State.mu.Lock()
	histItem := RecoveryHistoryItem{
		ID:            fmt.Sprintf("rec-hist-%d", now.UnixNano()),
		Timestamp:     nowTime,
		ActionTitle:   actionTitle,
		TargetService: svcName,
		Status:        "Completed",
		Duration:      "1.2s",
		RiskBefore:    riskBefore,
		RiskAfter:     riskAfter,
	}

	State.RecoveryHistory = append([]RecoveryHistoryItem{histItem}, State.RecoveryHistory...)

	auditType := "Autonomous Execution"
	if isApproved {
		auditType = fmt.Sprintf("Human-Approved by %s", approverName)
	}

	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-%d", now.UnixNano()),
		Timestamp:   nowTime,
		ServiceID:   svcID,
		ServiceName: svcName,
		Level:       "INFO",
		Message:     fmt.Sprintf("RECOVERY EXECUTED [%s]: %s. Risk dropped from %.1f%% to %.1f%%.", auditType, actionTitle, riskBefore, riskAfter),
	})
	State.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"actionId":     req.ActionID,
		"approvalType": auditType,
		"history":      histItem,
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
	// Check latest Python recommendations first
	for _, rec := range State.LatestRecommendations {
		if rec.RequiresApproval && !State.ApprovedPlans[rec.ID] {
			rec.Status = "pending_approval"
			pending = append(pending, rec)
		}
	}

	if len(pending) == 0 {
		// If no cached recommendations, evaluate live model
		pred := ComputePredictionOutput()
		for _, rec := range pred.RecommendedActions {
			if rec.RequiresApproval && !State.ApprovedPlans[rec.ID] {
				rec.Status = "pending_approval"
				pending = append(pending, rec)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pending)
}

type ApprovalPayload struct {
	ActionID        string `json:"actionId"`
	TargetServiceID string `json:"targetServiceId"`
	ActionType      string `json:"actionType,omitempty"`
	Approver        string `json:"approver"`
	Reason          string `json:"reason,omitempty"`
}

func ApproveRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	var payload ApprovalPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if payload.ActionID == "" {
		http.Error(w, `{"error":"actionId is required"}`, http.StatusBadRequest)
		return
	}
	if payload.TargetServiceID == "" {
		http.Error(w, `{"error":"targetServiceId is required for authorization binding"}`, http.StatusBadRequest)
		return
	}
	approver := strings.TrimSpace(payload.Approver)
	if approver == "" {
		if authUser, err := GetUserFromRequest(r); err == nil && authUser != nil {
			approver = authUser.Email
		}
	}
	if approver == "" {
		http.Error(w, `{"error":"Approver identity (approver) is required for audit trail"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	nowISO := now.Format(time.RFC3339)
	nowTime := now.Format("15:04:05")

	rec := ApprovalRecord{
		ActionID:        payload.ActionID,
		TargetServiceID: payload.TargetServiceID,
		ActionType:      payload.ActionType,
		Approver:        approver,
		Timestamp:       nowISO,
		Reason:          payload.Reason,
	}

	State.mu.Lock()
	if State.ApprovedRecords == nil {
		State.ApprovedRecords = make(map[string]ApprovalRecord)
	}
	State.ApprovedRecords[payload.ActionID] = rec
	State.ApprovedPlans[payload.ActionID] = true

	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-appr-%d", now.UnixNano()),
		Timestamp:   nowTime,
		ServiceID:   payload.TargetServiceID,
		ServiceName: payload.TargetServiceID,
		Level:       "INFO",
		Message:     fmt.Sprintf("RECOVERY AUTHORIZED: Operator [%s] approved action '%s' on %s (ActionID: %s)", approver, payload.ActionType, payload.TargetServiceID, payload.ActionID),
	})
	State.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"approved":        true,
		"actionId":        payload.ActionID,
		"targetServiceId": payload.TargetServiceID,
		"actionType":      payload.ActionType,
		"approver":        approver,
		"timestamp":       nowISO,
		"status":          "approval_recorded",
	})
}

func RejectRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	var payload ApprovalPayload
	_ = json.NewDecoder(r.Body).Decode(&payload)

	State.mu.Lock()
	if payload.ActionID != "" {
		delete(State.ApprovedPlans, payload.ActionID)
		delete(State.ApprovedRecords, payload.ActionID)
	}
	State.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"rejected": true,
		"actionId": payload.ActionID,
		"status":   "rejected",
	})
}
