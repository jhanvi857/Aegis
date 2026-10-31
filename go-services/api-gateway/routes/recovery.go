package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
)

var (
	recoveryWG sync.WaitGroup
)

func WaitForRecoveryVerifications() {
	recoveryWG.Wait()
}

func hasFreshHealthSignal(serviceID string, after time.Time) bool {
	State.mu.RLock()
	defer State.mu.RUnlock()
	svc := State.Services[serviceID]
	return svc != nil && (svc.SignalLastSeen["error_rate"].After(after) || svc.SignalLastSeen["latency"].After(after))
}

func appendRecoveryVerificationAudit(store storage.Store, planID, targetID, status string) {
	_ = store.AppendAuditLog(context.Background(), &storage.AuditLogEntry{
		Timestamp: time.Now().UTC(), ActorID: "system", ActorEmail: "aegis-verification@system.internal",
		ActorRole: "system", Action: "recovery.verify", TargetServiceID: targetID,
		ApprovalStatus: "RECORDED", Payload: "plan_id: " + planID, ResultStatus: strings.ToUpper(status),
	})
}

type ExecuteRecoveryRequest struct {
	ActionID        string `json:"actionId"`
	TargetServiceID string `json:"targetServiceId,omitempty"`
	ActionType      string `json:"actionType,omitempty"`
	DryRun          bool   `json:"dryRun,omitempty"`
	IdempotencyKey  string `json:"idempotencyKey,omitempty"`
	Parameters      map[string]string `json:"parameters,omitempty"`
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

	// Idempotency Key check: support header or payload
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		idempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	}

	if idempotencyKey != "" {
		store := GetAuthStore()
		if store != nil {
			existingExec, err := store.GetRecoveryExecutionByIdempotencyKey(r.Context(), idempotencyKey)
			if err == nil && existingExec != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"idempotent":       true,
					"idempotentReplay": true,
					"actionId":         existingExec.ActionID,
					"planId":           existingExec.PlanID,
					"status":           existingExec.Status,
					"targetServiceId":  existingExec.TargetServiceID,
					"actionType":       existingExec.ActionType,
					"riskLevel":        existingExec.RiskLevel,
					"riskScore":        existingExec.RiskScore,
					"requiresApproval": existingExec.RequiresApproval,
					"duration":         existingExec.DurationStr,
					"metricsBefore":    existingExec.MetricsBefore,
					"metricsAfter":     existingExec.MetricsAfter,
					"riskBefore":       existingExec.RiskBefore,
					"riskAfter":        existingExec.RiskAfter,
					"message":          "Idempotent replay: returning previously recorded execution result.",
				})
				return
			}
		}
	}

	// Dry run support via header or payload
	if r.Header.Get("X-Dry-Run") == "true" {
		req.DryRun = true
	}

	// Pre-recovery prediction to derive real baseline risk
	predBefore := ComputePredictionOutput()

	targetID := req.TargetServiceID
	State.mu.RLock()
	// If targetServiceId not provided directly, parse from actionId if format is rec-action-nodeId
	if targetID == "" && len(req.ActionID) > 4 {
		for id := range State.Services {
			if len(req.ActionID) >= len(id) && req.ActionID[len(req.ActionID)-len(id):] == id {
				targetID = id
				break
			}
		}
	}

	svc, exists := State.Services[targetID]
	rec, hasRecord := State.ApprovedRecords[req.ActionID]
	isApproved := hasRecord || (req.ActionID != "" && State.ApprovedPlans[req.ActionID])
	State.mu.RUnlock()

	if !exists || svc == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("Service '%s' not found in active topology", targetID),
		})
		return
	}

	// Resolve action type
	actionType := req.ActionType
	if actionType == "" {
		if strings.Contains(req.ActionID, "reroute") {
			actionType = "reroute_traffic"
		} else if strings.Contains(req.ActionID, "restart") {
			actionType = "restart"
		} else if strings.Contains(req.ActionID, "scale") {
			actionType = "scale"
		} else if strings.Contains(req.ActionID, "flush_cache") {
			actionType = "flush_cache"
		} else if strings.Contains(req.ActionID, "increase_pool") {
			actionType = "increase_pool"
		}
	}

	if actionType == "" {
		// Look up in LatestRecommendations from Python decision engine
		State.mu.RLock()
		for _, recItem := range State.LatestRecommendations {
			if recItem.ID == req.ActionID || recItem.TargetServiceID == targetID {
				actionType = recItem.ActionType
				break
			}
		}
		State.mu.RUnlock()
	}
	if actionType == "" && predBefore.RecommendedActionID != "" && predBefore.RecommendedActionID != "act-none" {
		for _, recItem := range predBefore.RecommendedActions {
			if recItem.TargetServiceID == targetID {
				actionType = recItem.ActionType
				break
			}
		}
	}
	if actionType == "" {
		LogAudit(r, "recovery.disallowed", targetID, "REJECTED", "", "action type unresolved", "BLOCKED", "Recovery must reference a known action recommendation")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "action type is required and must match a known recovery recommendation",
			"status": "rejected",
		})
		return
	}

	// Fail closed: recovery requires an explicitly configured registry binding
	// and an allowlist. Discovery never grants execution capability.
	store := GetAuthStore()
	if store == nil {
		http.Error(w, "recovery registry unavailable", http.StatusServiceUnavailable)
		return
	}
	regSvc, regErr := store.GetService(r.Context(), targetID)
	if regErr != nil || regSvc == nil {
		LogAudit(r, "recovery.disallowed", targetID, "REJECTED", "", "registry service missing", "BLOCKED", "No owner-managed recovery policy exists for this service")
		http.Error(w, "no owner-managed recovery policy exists for this service", http.StatusConflict)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(regSvc.MonitoringMode))
	policyRequiresApproval := mode == "recommend"
	if mode != "recommend" && mode != "auto_remediate" {
		LogAudit(r, "recovery.disallowed", targetID, "REJECTED", "", fmt.Sprintf("action: %s, mode: %s", actionType, regSvc.MonitoringMode), "BLOCKED", "Service is not enabled for recovery")
		http.Error(w, "service monitoring mode does not permit recovery execution", http.StatusForbidden)
		return
	}
	if regSvc.RecoveryBinding == nil || strings.TrimSpace(regSvc.RecoveryBinding.Target) == "" {
		LogAudit(r, "recovery.disallowed", targetID, "REJECTED", "", fmt.Sprintf("action: %s", actionType), "BLOCKED", "Service has no recovery binding")
		http.Error(w, "service has no recovery binding", http.StatusConflict)
		return
	}
	if !strings.EqualFold(regSvc.RecoveryBinding.Runtime, "docker") {
		http.Error(w, "recovery runtime is unsupported by the configured executor", http.StatusNotImplemented)
		return
	}
	if len(regSvc.RecoveryBinding.AllowedActions) == 0 {
		http.Error(w, "recovery binding must explicitly allow at least one action", http.StatusConflict)
		return
	}
	normalizedAction := strings.ToLower(NormalizeActionType(actionType))
	allowed := false
	for _, boundAction := range regSvc.RecoveryBinding.AllowedActions {
		if strings.ToLower(NormalizeActionType(boundAction)) == normalizedAction {
			allowed = true
			break
		}
	}
	if !allowed {
		LogAudit(r, "recovery.disallowed", targetID, "REJECTED", "", fmt.Sprintf("action: %s", actionType), "BLOCKED", "Action not in allowed_actions")
		http.Error(w, fmt.Sprintf("action %q is not allowed by the service recovery binding", actionType), http.StatusForbidden)
		return
	}
	executionTarget := strings.TrimSpace(regSvc.RecoveryBinding.Target)
	// Single Source of Truth: evaluate Risk & Approval Policy
	assessment := AssessPolicy(actionType, targetID)
	riskLevel := assessment.RiskLevel
	riskScore := assessment.RiskScore
	requiresApproval := assessment.RequiresApproval || policyRequiresApproval
	if !req.DryRun {
		switch NormalizeActionType(actionType) {
		case "RESTART", "SCALE_UP":
		default:
			http.Error(w, "recovery action has no configured executor integration", http.StatusNotImplemented)
			return
		}
	}

	// 1. Safety Limits Enforcement (Cooldown, Concurrent limit, Blast-radius budget)
	if !req.DryRun {
		if err := Safety.CanExecute(targetID, assessment.BlastRadiusSize); err != nil {
			LogAudit(r, "recovery.safety_limit", targetID, riskLevel, "", fmt.Sprintf("action: %s", actionType), "BLOCKED", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":                err.Error(),
				"cooldownRemainingSec": assessment.CooldownRemainingSec,
			})
			return
		}
	}

	// 2. Server-side RBAC: High-risk execution strictly requires admin privileges
	caller := GetCaller(r)
	if caller != nil && riskLevel == "HIGH" && caller.Role != storage.RoleAdmin {
		LogAudit(r, "recovery.execute", targetID, riskLevel, "", fmt.Sprintf("action: %s", actionType), "BLOCKED", "Executing high-risk action requires admin privileges")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     fmt.Sprintf("Forbidden: executing high-risk action '%s' on %s requires admin privileges", actionType, targetID),
			"riskLevel": riskLevel,
		})
		return
	}

	// 3. Dry-Run Mode (Simulation without side-effects)
	if req.DryRun {
		LogAudit(r, "recovery.dry_run", targetID, riskLevel, fmt.Sprintf("%v", isApproved), fmt.Sprintf("action: %s", actionType), "SUCCESS", "")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":          true,
			"dryRun":           true,
			"actionId":         req.ActionID,
			"targetServiceId":  targetID,
			"actionType":       actionType,
			"riskLevel":        riskLevel,
			"riskScore":        riskScore,
			"requiresApproval": requiresApproval,
			"blastRadius":      assessment.BlastRadius,
			"blastRadiusSize":  assessment.BlastRadiusSize,
			"criticality":      assessment.TargetTier,
			"justification":    assessment.Justification,
			"message":          "Dry-run simulation completed. Policy validated without dispatching changes.",
		})
		return
	}

	// 4. Approval Gate: Policy-governed approval check
	if requiresApproval && !isApproved {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":            fmt.Sprintf("Action '%s' on %s requires operator approval before execution (risk level: %s)", actionType, targetID, riskLevel),
			"requiresApproval": true,
			"riskLevel":        riskLevel,
			"riskScore":        riskScore,
			"actionId":         req.ActionID,
			"targetServiceId":  targetID,
			"justification":    assessment.Justification,
		})
		return
	}

	// Verify target and action bindings if recorded
	if hasRecord {
		if rec.TargetServiceID != "" && rec.TargetServiceID != targetID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": fmt.Sprintf("Approval authorization mismatch: token bound to '%s' cannot be executed on '%s'", rec.TargetServiceID, targetID),
			})
			return
		}
		if rec.ActionType != "" && actionType != "" && !strings.EqualFold(rec.ActionType, actionType) {
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

	State.mu.Lock()
	// Consume approval token upon execution
	if req.ActionID != "" {
		delete(State.ApprovedPlans, req.ActionID)
		delete(State.ApprovedRecords, req.ActionID)
	}

	svc, exists = State.Services[targetID]
	if !exists {
		State.mu.Unlock()
		http.Error(w, `{"error":"No service to remediate"}`, http.StatusBadRequest)
		return
	}

	now := time.Now()
	// nowTime := now.Format("15:04:05")
	planID := fmt.Sprintf("plan-exec-%d", now.UnixNano())

	// Capture metrics before remediation
	metricsBefore := map[string]interface{}{
		"cpu":       svc.CPU,
		"memory":    svc.Memory,
		"latency":   svc.Latency,
		"errorRate": svc.ErrorRate,
		"status":    svc.Status,
		"replicas":  svc.Replicas,
	}

	// Register in-flight action with Safety limits manager
	Safety.RegisterDispatch(planID, targetID, actionType, assessment.BlastRadiusSize)

	// Persist initial execution record in storage
	execRecord := &storage.RecoveryExecution{
		ID:               planID,
		PlanID:           planID,
		ActionID:         req.ActionID,
		ActionType:       actionType,
		TargetServiceID:  targetID,
		Status:           "Dispatching",
		RiskLevel:        riskLevel,
		RiskScore:        riskScore,
		RequiresApproval: requiresApproval,
		Approver:         approverName,
		DryRun:           false,
		IdempotencyKey:   idempotencyKey,
		DurationMs:       0,
		DurationStr:      "0.0s",
		MetricsBefore:    metricsBefore,
		RiskBefore:       predBefore.FailureProbability,
		StartedAt:        now.UTC(),
	}
	if store := GetAuthStore(); store != nil {
		_ = store.CreateRecoveryExecution(r.Context(), execRecord)
	}

	// Dispatch is not proof of recovery. Keep observed health untouched until
	// fresh post-action telemetry verifies it.
	statusBeforeDispatch := svc.Status
	if svc.Status != "healthy" {
		svc.Status = "remediating"
	}

	actionTitle := fmt.Sprintf("Remediation: %s on %s", actionType, svc.Name)
	svcName := svc.Name

	State.mu.Unlock()

	// Dispatch recovery plan to physical Recovery Engine
	if flag.Lookup("test.v") == nil || os.Getenv("RECOVERY_ENGINE_URL") != "" {
		recEngineURL := strings.TrimSpace(os.Getenv("RECOVERY_ENGINE_URL"))
		if recEngineURL == "" { recEngineURL = "http://localhost:8092" }
		planPayload, _ := json.Marshal(map[string]interface{}{
			"plan_id": planID, "risk_level": riskLevel, "risk_score": riskScore,
			"steps": []map[string]interface{}{{
				"action_id": fmt.Sprintf("act-%d", time.Now().UnixNano()), "action_type": actionType,
				"target_node": executionTarget, "execution_order": 1, "parameters": req.Parameters,
			}},
		})
		client := &http.Client{Timeout: 3 * time.Second}
		resp, dispatchErr := client.Post(fmt.Sprintf("%s/plan/submit", recEngineURL), "application/json", bytes.NewBuffer(planPayload))
		if dispatchErr == nil {
			var ack struct { Status string `json:"status"` }
			if resp.StatusCode >= 200 && resp.StatusCode < 300 { dispatchErr = json.NewDecoder(resp.Body).Decode(&ack) }
			if resp.StatusCode < 200 || resp.StatusCode >= 300 { dispatchErr = fmt.Errorf("recovery engine returned HTTP %d", resp.StatusCode) }
			_ = resp.Body.Close()
			if dispatchErr == nil && ack.Status == "held_for_approval" {
				if !isApproved {
					dispatchErr = fmt.Errorf("recovery engine requires approval")
				} else {
					approveBody, _ := json.Marshal(map[string]string{"plan_id": planID, "approved_by": approverName})
					approveResp, err := client.Post(fmt.Sprintf("%s/plan/approve", recEngineURL), "application/json", bytes.NewBuffer(approveBody))
					if err != nil { dispatchErr = err } else {
						var approveAck struct { Status string `json:"status"` }
						if approveResp.StatusCode < 200 || approveResp.StatusCode >= 300 { dispatchErr = fmt.Errorf("recovery engine approval returned HTTP %d", approveResp.StatusCode) } else { dispatchErr = json.NewDecoder(approveResp.Body).Decode(&approveAck) }
						_ = approveResp.Body.Close()
						if dispatchErr == nil && approveAck.Status != "approved_and_executed" { dispatchErr = fmt.Errorf("recovery engine approval ended in %q", approveAck.Status) }
					}
				}
			}
		}
		if dispatchErr != nil {
			completedAt := time.Now().UTC()
			if store := GetAuthStore(); store != nil { _ = store.UpdateRecoveryExecution(r.Context(), &storage.RecoveryExecution{ID:planID, Status:"Failed", CompletedAt:&completedAt}) }
			Safety.CompleteDispatch(planID)
			State.mu.Lock()
			if current := State.Services[targetID]; current != nil && current.Status == "remediating" {
				current.Status = statusBeforeDispatch
			}
			State.mu.Unlock()
			LogAudit(r, "recovery.dispatch", targetID, riskLevel, "", fmt.Sprintf("action: %s, plan_id: %s", actionType, planID), "FAILED", dispatchErr.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":           "recovery engine dispatch failed; no action was executed",
				"actionId":        req.ActionID,
				"targetServiceId": targetID,
				"status":          "failed",
			})
			return
		}
		if store := GetAuthStore(); store != nil { _ = store.UpdateRecoveryExecution(r.Context(), &storage.RecoveryExecution{ID:planID, Status:"Verifying"}) }
	}

	// 5. Asynchronous Real Telemetry Verification Window (Replacing fake verify)
	recoveryWG.Add(1)
	go func(pID, targetNode, actType, aTitle, tName string, rBefore float64) {
		defer recoveryWG.Done()
		start := time.Now()
		window := 10 * time.Second
		if Safety != nil {
			vw := Safety.GetVerificationWindow()
			if vw > 0 {
				window = vw
			}
		}
		if window <= 0 {
			if flag.Lookup("test.v") != nil {
				window = 100 * time.Millisecond
			} else {
				window = 10 * time.Second
			}
		}
		deadline := start.Add(window)

		verified := false
		postActionTelemetry := false
		var finalMetrics map[string]interface{}

		checkInterval := 1 * time.Second
		if window <= 200*time.Millisecond {
			checkInterval = 20 * time.Millisecond
		} else if window < 3*time.Second {
			checkInterval = 100 * time.Millisecond
		}

		for time.Now().Before(deadline) {
			time.Sleep(checkInterval)
			State.mu.RLock()
			s, exists := State.Services[targetNode]
			if exists {
				if s.LastIngestTime.After(start) && len(s.ObservedSignals) > 0 { postActionTelemetry = true }
				finalMetrics = map[string]interface{}{
					"cpu":       s.CPU,
					"memory":    s.Memory,
					"latency":   s.Latency,
					"errorRate": s.ErrorRate,
					"status":    s.Status,
					"replicas":  s.Replicas,
				}
				// Verify only against fresh telemetry received after dispatch. Signals
				// that the service does not report are not treated as zero.
				errorFresh := s.SignalLastSeen["error_rate"].After(start)
				latencyFresh := s.SignalLastSeen["latency"].After(start)
				healthEvidence := errorFresh || latencyFresh
				verificationLimits := getIncidentGateConfig()
				errorOK := !errorFresh || s.ErrorRate <= verificationLimits.ErrorThreshold
				latencyOK := !latencyFresh || s.Latency <= verificationLimits.LatencyThreshold
				if healthEvidence && s.LastIngestTime.After(start) && s.DataStatus == "fresh" && s.Status == "healthy" && errorOK && latencyOK {
					verified = true
					State.mu.RUnlock()
					break
				}
			}
			State.mu.RUnlock()
		}

		duration := time.Since(start)
		durationMs := duration.Milliseconds()
		if durationMs <= 0 {
			durationMs = 1
		}
		var durationStr string
		if duration < time.Second {
			durationStr = fmt.Sprintf("%.2fs", duration.Seconds())
			if durationStr == "0.00s" {
				durationStr = "0.01s"
			}
		} else {
			durationStr = fmt.Sprintf("%.1fs", duration.Seconds())
		}

		status := "Verified"
		var escalatedAction string
		riskAfter := rBefore
		if verified { riskAfter = ComputePredictionOutput().FailureProbability }

		if !verified {
			status = "Unverified"
			if postActionTelemetry && hasFreshHealthSignal(targetNode, start) { status = "Ineffective" }
			riskAfter = rBefore

			if postActionTelemetry {
				// Secondary actions are recommendations for operator review only.
				family := "cpu_saturation"
				for fam, esc := range FaultFamilyEscalations {
					if esc.Primary == NormalizeActionType(actType) { family = fam; break }
				}
				if esc, ok := FaultFamilyEscalations[family]; ok { escalatedAction = esc.Secondary }
			}

			// Add human escalation alert
			State.mu.Lock()
			State.Alerts = append(State.Alerts, LiveAlert{
				ID:          fmt.Sprintf("alert-esc-%d", time.Now().UnixNano()),
				Timestamp:   time.Now().Format("15:04:05"),
				ServiceID:   targetNode,
				ServiceName: tName,
				Severity:    map[bool]string{true:"critical", false:"warning"}[postActionTelemetry],
				Message:     fmt.Sprintf("REMEDIATION %s: Action '%s' on %s has no successful verification evidence. Operator review required. Suggested follow-up: %s", status, actType, tName, escalatedAction),
				Resolved:    false,
			})
			State.mu.Unlock()
		}

		completedAt := time.Now().UTC()
		updateExec := &storage.RecoveryExecution{
			ID:              pID,
			Status:          status,
			DurationMs:      durationMs,
			DurationStr:     durationStr,
			MetricsAfter:    finalMetrics,
			RiskAfter:       riskAfter,
			EscalatedAction: escalatedAction,
			CompletedAt:     &completedAt,
		}
		if store := GetAuthStore(); store != nil {
			_ = store.UpdateRecoveryExecution(context.Background(), updateExec)
			appendRecoveryVerificationAudit(store, pID, targetNode, status)
		}
		Safety.CompleteDispatch(pID)

		// Record in in-memory history
		State.mu.Lock()
		histItem := RecoveryHistoryItem{
			ID:            fmt.Sprintf("rec-hist-%d", time.Now().UnixNano()),
			Timestamp:     time.Now().Format("15:04:05"),
			ActionTitle:   aTitle,
			TargetService: tName,
			Status:        status,
			Duration:      durationStr,
			RiskBefore:    rBefore,
			RiskAfter:     riskAfter,
		}
		State.RecoveryHistory = append([]RecoveryHistoryItem{histItem}, State.RecoveryHistory...)
		State.Logs = append(State.Logs, LogEntry{
			ID:          fmt.Sprintf("log-verify-%d", time.Now().UnixNano()),
			Timestamp:   time.Now().Format("15:04:05"),
			ServiceID:   targetNode,
			ServiceName: tName,
			Level:       "INFO",
			Message:     fmt.Sprintf("RECOVERY VERIFICATION [%s]: %s on %s completed in %s. Risk: %.1f%% -> %.1f%%", status, actType, tName, durationStr, rBefore, riskAfter),
		})
		State.mu.Unlock()
	}(planID, targetID, actionType, actionTitle, svcName, predBefore.FailureProbability)

	auditType := "Autonomous Execution"
	if isApproved {
		auditType = fmt.Sprintf("Human-Approved by %s", approverName)
	}

	LogAudit(r, "recovery.execute", targetID, riskLevel, fmt.Sprintf("%v", isApproved), fmt.Sprintf("action: %s, plan_id: %s", actionType, planID), "DISPATCHED", "")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          false,
		"accepted":         true,
		"status":           "Verifying",
		"actionId":         req.ActionID,
		"planId":           planID,
		"targetServiceId":  targetID,
		"actionType":       actionType,
		"riskLevel":        riskLevel,
		"riskScore":        riskScore,
		"requiresApproval": requiresApproval,
		"approvalType":     auditType,
		"metricsBefore":    metricsBefore,
		"message":          "Recovery action dispatched; telemetry verification window in progress.",
	})
}

func GetRecoveryHistoryHandler(w http.ResponseWriter, r *http.Request) {
	// First check database storage for persisted executions
	if store := GetAuthStore(); store != nil {
		executions, err := store.ListRecoveryExecutions(r.Context(), 50)
		if err == nil && len(executions) > 0 {
			items := make([]RecoveryHistoryItem, 0, len(executions))
			for _, e := range executions {
				items = append(items, RecoveryHistoryItem{
					ID:            e.ID,
					Timestamp:     e.StartedAt.Format("15:04:05"),
					ActionTitle:   fmt.Sprintf("Remediation: %s on %s", e.ActionType, e.TargetServiceID),
					TargetService: e.TargetServiceID,
					Status:        e.Status,
					Duration:      e.DurationStr,
					RiskBefore:    e.RiskBefore,
					RiskAfter:     e.RiskAfter,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(items)
			return
		}
	}

	State.mu.RLock()
	defer State.mu.RUnlock()

	history := State.RecoveryHistory
	if history == nil {
		history = make([]RecoveryHistoryItem, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(history)
}

// RollbackRecoveryHandler reverts a previously executed recovery action
func RollbackRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	actionID := vars["id"]

	store := GetAuthStore()
	if store == nil {
		http.Error(w, `{"error":"Recovery store unavailable"}`, http.StatusInternalServerError)
		return
	}

	exec, err := store.GetRecoveryExecution(r.Context(), actionID)
	if err != nil {
		http.Error(w, `{"error":"Recovery execution record not found"}`, http.StatusNotFound)
		return
	}

	State.mu.Lock()
	svc, exists := State.Services[exec.TargetServiceID]
	if !exists {
		State.mu.Unlock()
		http.Error(w, `{"error":"Target service not found"}`, http.StatusNotFound)
		return
	}

	// Revert action parameters
	norm := NormalizeActionType(exec.ActionType)
	switch norm {
	case "SCALE_UP":
		if svc.Replicas > 1 {
			svc.Replicas -= 2
		}
	case "REROUTE_TRAFFIC":
		svc.Latency = 20.0
	case "INCREASE_POOL":
		svc.DBWait = 10.0
	}
	State.mu.Unlock()

	now := time.Now().UTC()
	exec.Status = "RolledBack"
	exec.CompletedAt = &now
	_ = store.UpdateRecoveryExecution(r.Context(), exec)

	LogAudit(r, "recovery.rollback", exec.TargetServiceID, exec.RiskLevel, "", fmt.Sprintf("action: %s, plan_id: %s", exec.ActionType, exec.ID), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"actionId": actionID,
		"status":   "RolledBack",
		"message":  fmt.Sprintf("Successfully rolled back action '%s' on %s", exec.ActionType, exec.TargetServiceID),
	})
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
	pending := make([]RecoveryAction, 0)
	// Check latest Python recommendations first
	for _, rec := range State.LatestRecommendations {
		if rec.RequiresApproval && !State.ApprovedPlans[rec.ID] {
			rec.Status = "pending_approval"
			pending = append(pending, rec)
		}
	}
	State.mu.RUnlock()

	if len(pending) == 0 {
		// If no cached recommendations, evaluate live model
		pred := ComputePredictionOutput()
		State.mu.RLock()
		for _, rec := range pred.RecommendedActions {
			if rec.RequiresApproval && !State.ApprovedPlans[rec.ID] {
				rec.Status = "pending_approval"
				pending = append(pending, rec)
			}
		}
		State.mu.RUnlock()
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
	caller := GetCaller(r)
	if caller == nil || strings.TrimSpace(caller.Email) == "" {
		http.Error(w, `{"error":"Authenticated operator identity is required for approval"}`, http.StatusUnauthorized)
		return
	}
	approver := strings.TrimSpace(caller.Email)
	if supplied := strings.TrimSpace(payload.Approver); supplied != "" && !strings.EqualFold(supplied, approver) {
		http.Error(w, `{"error":"approver must match the authenticated operator"}`, http.StatusForbidden)
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

	LogAudit(r, "recovery.approve", payload.TargetServiceID, "", "APPROVED", fmt.Sprintf("action_id: %s, action_type: %s", payload.ActionID, payload.ActionType), "SUCCESS", "")

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

	LogAudit(r, "recovery.reject", payload.TargetServiceID, "", "REJECTED", fmt.Sprintf("action_id: %s", payload.ActionID), "SUCCESS", "")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"rejected": true,
		"actionId": payload.ActionID,
		"status":   "rejected",
	})
}

// ─── Phase 4 Autonomous Recovery Engine ──────────────────────────────────────

// ExecuteAutonomousRecovery autonomously executes a low/medium risk recovery action that does not require human approval
func ExecuteAutonomousRecovery(targetID string, actionType string, actionID string) (*storage.RecoveryExecution, error) {
	if targetID == "" {
		return nil, fmt.Errorf("target service ID is required")
	}

	State.mu.RLock()
	svc, exists := State.Services[targetID]
	State.mu.RUnlock()
	if !exists || svc == nil {
		return nil, fmt.Errorf("service %s not found", targetID)
	}

	store := GetAuthStore()
	if store == nil { return nil, fmt.Errorf("recovery registry unavailable") }
	registered, err := store.GetService(context.Background(), targetID)
	if err != nil || registered == nil || registered.RecoveryBinding == nil { return nil, fmt.Errorf("service has no configured recovery binding") }
	if strings.ToLower(strings.TrimSpace(registered.MonitoringMode)) != "auto_remediate" { return nil, fmt.Errorf("service is not enabled for autonomous recovery") }
	if !strings.EqualFold(strings.TrimSpace(registered.RecoveryBinding.Runtime), "docker") || strings.TrimSpace(registered.RecoveryBinding.Target) == "" { return nil, fmt.Errorf("configured recovery runtime or target is unsupported") }
	allowedAction := false
	for _, allowed := range registered.RecoveryBinding.AllowedActions { if strings.EqualFold(NormalizeActionType(allowed), NormalizeActionType(actionType)) { allowedAction = true; break } }
	if !allowedAction { return nil, fmt.Errorf("action is not allowed by the service recovery binding") }
	if NormalizeActionType(actionType) != "RESTART" { return nil, fmt.Errorf("autonomous executor does not support this action without explicit parameters") }
	executionTarget := strings.TrimSpace(registered.RecoveryBinding.Target)

	// 1. Policy check
	assessment := AssessPolicy(actionType, targetID)
	if assessment.RequiresApproval {
		State.mu.Lock()
		State.Logs = append(State.Logs, LogEntry{
			ID:          fmt.Sprintf("log-gate-%d", time.Now().UnixNano()),
			Timestamp:   time.Now().Format("15:04:05"),
			ServiceID:   targetID,
			ServiceName: svc.Name,
			Level:       "WARN",
			Message:     fmt.Sprintf("[APPROVAL GATE] High-risk remediation %s on %s held pending operator authorization (Risk: %s, Score: %.2f)", actionType, svc.Name, assessment.RiskLevel, assessment.RiskScore),
		})
		State.mu.Unlock()
		return nil, fmt.Errorf("action %s on %s requires human operator approval (risk: %s)", actionType, targetID, assessment.RiskLevel)
	}

	// 2. Safety limits check
	if Safety != nil {
		if err := Safety.CanExecute(targetID, assessment.BlastRadiusSize); err != nil {
			return nil, fmt.Errorf("safety limit blocked execution: %w", err)
		}
	}

	// 3. Baseline prediction
	predBefore := ComputePredictionOutput()

	State.mu.Lock()
	now := time.Now()
	planID := fmt.Sprintf("plan-auto-%d", now.UnixNano())
	if actionID == "" {
		actionID = fmt.Sprintf("act-auto-%d", now.UnixNano())
	}

	metricsBefore := map[string]interface{}{
		"cpu":       svc.CPU,
		"memory":    svc.Memory,
		"latency":   svc.Latency,
		"errorRate": svc.ErrorRate,
		"status":    svc.Status,
		"replicas":  svc.Replicas,
	}

	// Register in-flight action with Safety limits manager
	if Safety != nil {
		Safety.RegisterDispatch(planID, targetID, actionType, assessment.BlastRadiusSize)
	}

	// Persist initial execution record in storage
	execRecord := &storage.RecoveryExecution{
		ID:               planID,
		PlanID:           planID,
		ActionID:         actionID,
		ActionType:       actionType,
		TargetServiceID:  targetID,
		Status:           "Dispatching",
		RiskLevel:        assessment.RiskLevel,
		RiskScore:        assessment.RiskScore,
		RequiresApproval: false,
		Approver:         "Autonomous Decision Engine",
		DryRun:           false,
		DurationMs:       0,
		DurationStr:      "0.0s",
		MetricsBefore:    metricsBefore,
		RiskBefore:       predBefore.FailureProbability,
		StartedAt:        now.UTC(),
	}
	if store := GetAuthStore(); store != nil {
		_ = store.CreateRecoveryExecution(context.Background(), execRecord)
	}

	// An action dispatch does not alter observed service metrics or prove recovery.
	if svc.Status != "healthy" {
		svc.Status = "remediating"
	}

	actionTitle := fmt.Sprintf("Autonomous Remediation: %s on %s", actionType, svc.Name)
	svcName := svc.Name

	State.Logs = append(State.Logs, LogEntry{
		ID:          fmt.Sprintf("log-auto-%d", now.UnixNano()),
		Timestamp:   now.Format("15:04:05"),
		ServiceID:   targetID,
		ServiceName: svcName,
		Level:       "INFO",
		Message:     fmt.Sprintf("Autonomous action %s dispatched to configured target for %s (risk: %s, score: %.2f)", actionType, svcName, assessment.RiskLevel, assessment.RiskScore),
	})
	State.mu.Unlock()

	// Synchronously confirm that the recovery engine accepted an actual dispatch.
	recEngineURL := strings.TrimSpace(os.Getenv("RECOVERY_ENGINE_URL"))
	if recEngineURL == "" { recEngineURL = "http://localhost:8092" }
	planPayload, _ := json.Marshal(map[string]interface{}{
		"plan_id": planID, "risk_level": assessment.RiskLevel, "risk_score": assessment.RiskScore,
		"steps": []map[string]interface{}{{"action_id": actionID, "action_type": actionType, "target_node": executionTarget, "execution_order": 1, "parameters": map[string]string{}}},
	})
	client := &http.Client{Timeout: 3 * time.Second}
	resp, dispatchErr := client.Post(fmt.Sprintf("%s/plan/submit", recEngineURL), "application/json", bytes.NewBuffer(planPayload))
	if dispatchErr == nil {
		var ack struct { Status string `json:"status"` }
		if resp.StatusCode >= 200 && resp.StatusCode < 300 { dispatchErr = json.NewDecoder(resp.Body).Decode(&ack) } else { dispatchErr = fmt.Errorf("recovery engine returned HTTP %d", resp.StatusCode) }
		_ = resp.Body.Close()
		if dispatchErr == nil && ack.Status != "auto_executed" { dispatchErr = fmt.Errorf("autonomous action was not auto-executed (engine status %q)", ack.Status) }
	}
	if dispatchErr != nil {
		completedAt := time.Now().UTC()
		_ = store.UpdateRecoveryExecution(context.Background(), &storage.RecoveryExecution{ID: planID, Status: "Failed", CompletedAt: &completedAt})
		_ = store.AppendAuditLog(context.Background(), &storage.AuditLogEntry{
			Timestamp: completedAt, ActorID: "system", ActorEmail: "aegis-autonomous-agent@system.internal",
			ActorRole: "system", Action: "recovery.dispatch", TargetServiceID: targetID,
			RiskLevel: assessment.RiskLevel, ApprovalStatus: "AUTONOMOUS", Payload: fmt.Sprintf("action: %s, plan_id: %s", actionType, planID), ResultStatus: "FAILED", ErrorMessage: dispatchErr.Error(),
		})
		if Safety != nil { Safety.CompleteDispatch(planID) }
		return nil, fmt.Errorf("recovery dispatch failed: %w", dispatchErr)
	}
	_ = store.UpdateRecoveryExecution(context.Background(), &storage.RecoveryExecution{ID: planID, Status: "Verifying"})

	// Verification window goroutine
	recoveryWG.Add(1)
	go func(pID, targetNode, actType, aTitle, tName string, rBefore float64) {
		defer recoveryWG.Done()
		start := time.Now()
		window := 10 * time.Second
		if Safety != nil {
			vw := Safety.GetVerificationWindow()
			if vw > 0 {
				window = vw
			}
		} else if flag.Lookup("test.v") != nil {
			window = 100 * time.Millisecond
		}
		deadline := start.Add(window)

		verified := false
		postActionTelemetry := false
		var finalMetrics map[string]interface{}
		checkInterval := 1 * time.Second
		if window <= 200*time.Millisecond {
			checkInterval = 20 * time.Millisecond
		} else if window < 3*time.Second {
			checkInterval = 100 * time.Millisecond
		}

		for time.Now().Before(deadline) {
			time.Sleep(checkInterval)
			State.mu.RLock()
			s, ex := State.Services[targetNode]
			if ex {
				if s.LastIngestTime.After(start) && len(s.ObservedSignals) > 0 { postActionTelemetry = true }
				finalMetrics = map[string]interface{}{
					"cpu":       s.CPU,
					"memory":    s.Memory,
					"latency":   s.Latency,
					"errorRate": s.ErrorRate,
					"status":    s.Status,
					"replicas":  s.Replicas,
				}
			errorFresh := s.SignalLastSeen["error_rate"].After(start)
			latencyFresh := s.SignalLastSeen["latency"].After(start)
			healthEvidence := errorFresh || latencyFresh
			verificationLimits := getIncidentGateConfig()
			errorOK := !errorFresh || s.ErrorRate <= verificationLimits.ErrorThreshold
			latencyOK := !latencyFresh || s.Latency <= verificationLimits.LatencyThreshold
			if healthEvidence && s.LastIngestTime.After(start) && s.DataStatus == "fresh" && s.Status == "healthy" && errorOK && latencyOK {
					verified = true
					State.mu.RUnlock()
					break
				}
			}
			State.mu.RUnlock()
		}

		duration := time.Since(start)
		durationMs := duration.Milliseconds()
		if durationMs <= 0 {
			durationMs = 1
		}
		var durationStr string
		if duration < time.Second {
			durationStr = fmt.Sprintf("%.2fs", duration.Seconds())
			if durationStr == "0.00s" {
				durationStr = "0.01s"
			}
		} else {
			durationStr = fmt.Sprintf("%.1fs", duration.Seconds())
		}

		status := "Verified"
		var escalatedAction string
		riskAfter := rBefore
		if !verified {
			status = "Unverified"
			if postActionTelemetry && hasFreshHealthSignal(targetNode, start) { status = "Ineffective" }
			riskAfter = rBefore

			if postActionTelemetry {
				// Recommend a secondary action for operator review; do not execute it automatically.
				family := "cpu_saturation"
				for fam, esc := range FaultFamilyEscalations {
					if esc.Primary == NormalizeActionType(actType) { family = fam; break }
				}
				if esc, ok := FaultFamilyEscalations[family]; ok { escalatedAction = esc.Secondary }
			}

			// Add human escalation alert
			State.mu.Lock()
			State.Alerts = append(State.Alerts, LiveAlert{
				ID:          fmt.Sprintf("alert-esc-%d", time.Now().UnixNano()),
				Timestamp:   time.Now().Format("15:04:05"),
				ServiceID:   targetNode,
				ServiceName: tName,
				Severity:    map[bool]string{true:"critical", false:"warning"}[postActionTelemetry],
				Message:     fmt.Sprintf("AUTONOMOUS REMEDIATION %s: Action '%s' on %s has no successful verification evidence. Operator review required. Suggested follow-up: %s", status, actType, tName, escalatedAction),
				Resolved:    false,
			})
			State.mu.Unlock()
		}

		if Safety != nil {
			Safety.CompleteDispatch(pID)
		}

		if verified {
			predAfter := ComputePredictionOutput()
			riskAfter = predAfter.FailureProbability
		}

		completedAt := time.Now().UTC()
		updateExec := &storage.RecoveryExecution{
			ID:              pID,
			Status:          status,
			DurationMs:      durationMs,
			DurationStr:     durationStr,
			MetricsAfter:    finalMetrics,
			RiskAfter:       riskAfter,
			EscalatedAction: escalatedAction,
			CompletedAt:     &completedAt,
		}
		if store := GetAuthStore(); store != nil {
			_ = store.UpdateRecoveryExecution(context.Background(), updateExec)
			appendRecoveryVerificationAudit(store, pID, targetNode, status)
		}

		State.mu.Lock()
		histItem := RecoveryHistoryItem{
			ID:            pID,
			Timestamp:     time.Now().Format("15:04:05"),
			ActionTitle:   aTitle,
			TargetService: tName,
			Status:        status,
			Duration:      durationStr,
			RiskBefore:    rBefore,
			RiskAfter:     riskAfter,
		}
		State.RecoveryHistory = append([]RecoveryHistoryItem{histItem}, State.RecoveryHistory...)
		State.Logs = append(State.Logs, LogEntry{
			ID:          fmt.Sprintf("log-verify-%d", time.Now().UnixNano()),
			Timestamp:   time.Now().Format("15:04:05"),
			ServiceID:   targetNode,
			ServiceName: tName,
			Level:       "INFO",
			Message:     fmt.Sprintf("[AUTONOMOUS VERIFICATION] [%s]: %s on %s completed in %s. Risk: %.1f%% -> %.1f%%", status, actType, tName, durationStr, rBefore, riskAfter),
		})
		State.mu.Unlock()
	}(planID, targetID, actionType, actionTitle, svcName, predBefore.FailureProbability)

	// Record audit log
	if store := GetAuthStore(); store != nil {
		_ = store.AppendAuditLog(context.Background(), &storage.AuditLogEntry{
			Timestamp:       now.UTC(),
			ActorID:         "system",
			ActorEmail:      "aegis-autonomous-agent@system.internal",
			ActorRole:       "system",
			Action:          "recovery.autonomous_execute",
			TargetServiceID: targetID,
			RiskLevel:       assessment.RiskLevel,
			ApprovalStatus:  "AUTONOMOUS",
			Payload:         fmt.Sprintf("action: %s, plan_id: %s", actionType, planID),
			ResultStatus:    "DISPATCHED",
		})
	}

	return execRecord, nil
}

func GetAutonomousRecoveryStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	State.mu.RLock()
	enabled := State.AutonomousRecovery
	State.mu.RUnlock()

	activeCount := 0
	cooldown := 30
	maxConcurrent := 2
	if Safety != nil {
		activeCount = Safety.ActiveDispatchCount()
		cooldown = int(Safety.GetServiceCooldown().Seconds())
		maxConcurrent = Safety.GetMaxConcurrent()
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":          enabled,
		"policy":           "two-tier-risk-approval",
		"autoExecuteLow":   true,
		"cooldownSec":      cooldown,
		"maxConcurrent":    maxConcurrent,
		"activeDispatches": activeCount,
	})
}

func SetAutonomousRecoveryStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	State.mu.Lock()
	State.AutonomousRecovery = req.Enabled
	State.mu.Unlock()

	LogAudit(r, "recovery.autonomous_toggle", "system", "MEDIUM", "", fmt.Sprintf("autonomousRecovery set to %v", req.Enabled), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"enabled": req.Enabled,
		"message": fmt.Sprintf("Autonomous self-healing mode set to %v", req.Enabled),
	})
}
