package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
)

// setupPhase2TestEnvironment sets up fresh mock state and in-memory storage for Phase 2 testing
func setupPhase2TestEnvironment() storage.Store {
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	InitState()
	Safety.Reset()

	// Configure mock topology with clear dependency chain: gateway -> node-a -> node-b -> node-c
	State.Services = map[string]*Microservice{
		"gateway": {
			ID:           "gateway",
			Name:         "API Gateway",
			Type:         "gateway",
			Status:       "healthy",
			CPU:          20.0,
			Latency:      15.0,
			ErrorRate:    0.001,
			Replicas:     2,
			Dependencies: []string{"node-a"},
		},
		"node-a": {
			ID:           "node-a",
			Name:         "Node A",
			Type:         "service",
			Status:       "critical",
			CPU:          92.0,
			Latency:      650.0,
			ErrorRate:    0.35,
			Replicas:     2,
			Dependencies: []string{"node-b"},
		},
		"node-b": {
			ID:           "node-b",
			Name:         "Node B",
			Type:         "service",
			Status:       "degraded",
			CPU:          55.0,
			Latency:      120.0,
			ErrorRate:    0.05,
			Replicas:     2,
			Dependencies: []string{"node-c"},
		},
		"node-c": {
			ID:           "node-c",
			Name:         "Database C",
			Type:         "database",
			Status:       "healthy",
			CPU:          25.0,
			Latency:      10.0,
			ErrorRate:    0.00,
			Replicas:     1,
			Dependencies: []string{},
		},
	}
	State.ServiceOrder = []string{"gateway", "node-a", "node-b", "node-c"}
	return store
}

func TestPhase2_PolicyRiskCalculationFormula(t *testing.T) {
	setupPhase2TestEnvironment()

	// 1. RESTART on regular tier-2 node (node-b) with blast radius of 2 (node-a and gateway depend on node-b)
	assessmentRestart := AssessPolicy("restart", "node-b")
	if assessmentRestart.ActionType != "RESTART" {
		t.Errorf("expected action RESTART, got %s", assessmentRestart.ActionType)
	}
	if assessmentRestart.TargetTier != "tier-2" {
		t.Errorf("expected tier-2 for node-b, got %s", assessmentRestart.TargetTier)
	}
	// BaseSeverity=0.20 + radius penalty(2*0.08=0.16) + criticality(0.40*0.15=0.06) = 0.42
	if assessmentRestart.RiskScore != 0.42 {
		t.Errorf("expected restart risk score 0.42, got %.2f", assessmentRestart.RiskScore)
	}
	if assessmentRestart.RiskLevel != "MEDIUM" {
		t.Errorf("expected MEDIUM risk level for restart on node with blast radius 2, got %s", assessmentRestart.RiskLevel)
	}

	// 2. ISOLATE is always HIGH risk regardless of target
	assessmentIsolate := AssessPolicy("isolate", "node-b")
	if assessmentIsolate.RiskLevel != "HIGH" {
		t.Errorf("expected ISOLATE to be HIGH risk, got %s", assessmentIsolate.RiskLevel)
	}
	if !assessmentIsolate.RequiresApproval {
		t.Errorf("expected ISOLATE to strictly require approval")
	}

	// 3. Blast radius check for node-c (leaf dependency):
	// node-b depends on node-c, node-a depends on node-b, gateway depends on node-a
	blastRadiusC := ComputeBlastRadius("node-c")
	if len(blastRadiusC) < 3 {
		t.Errorf("expected downstream blast radius of node-c to contain at least 3 nodes, got %d (%v)",
			len(blastRadiusC), blastRadiusC)
	}

	// 4. Criticality bonus for gateway (tier-1-ingress)
	assessmentGateway := AssessPolicy("restart", "gateway")
	if assessmentGateway.TargetTier != "tier-1-ingress" {
		t.Errorf("expected gateway tier-1-ingress, got %s", assessmentGateway.TargetTier)
	}
	if assessmentGateway.TargetCriticality < 0.80 {
		t.Errorf("expected high criticality >= 0.80 for gateway, got %.2f", assessmentGateway.TargetCriticality)
	}
}

func TestPhase2_PolicyCheckHandler_Endpoint(t *testing.T) {
	setupPhase2TestEnvironment()

	payload, _ := json.Marshal(map[string]string{
		"actionType":      "scale_up",
		"targetServiceId": "node-a",
	})

	req := httptest.NewRequest("POST", "/api/recovery/policy-check", bytes.NewBuffer(payload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	PolicyCheckHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from policy check handler, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var assessment PolicyAssessment
	if err := json.Unmarshal(rr.Body.Bytes(), &assessment); err != nil {
		t.Fatalf("failed to decode policy assessment: %v", err)
	}

	if assessment.ActionType != "SCALE_UP" {
		t.Errorf("expected action SCALE_UP, got %s", assessment.ActionType)
	}
	if assessment.TargetServiceID != "node-a" {
		t.Errorf("expected targetServiceId node-a, got %s", assessment.TargetServiceID)
	}
	if assessment.RiskScore <= 0 {
		t.Errorf("expected non-zero risk score, got %.2f", assessment.RiskScore)
	}
}

func TestPhase2_Safety_ServiceCooldownEnforcement(t *testing.T) {
	setupPhase2TestEnvironment()
	Safety.ServiceCooldown = 30 * time.Second

	// Pre-approve action on node-a
	actionID := "rec-scale-node-a"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		ActionType:      "scale",
		Approver:        "admin@enterprise.io",
	}

	// First execution should succeed
	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "scale",
	})
	req1 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr1 := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("first execution should succeed (200), got %d. Body: %s", rr1.Code, rr1.Body.String())
	}

	// Immediate second execution on node-a should trigger cooldown (429 Too Many Requests)
	actionID2 := "rec-restart-node-a"
	State.ApprovedPlans[actionID2] = true
	State.ApprovedRecords[actionID2] = ApprovalRecord{
		ActionID:        actionID2,
		TargetServiceID: "node-a",
		ActionType:      "restart",
		Approver:        "admin@enterprise.io",
	}

	payload2, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID2,
		"targetServiceId": "node-a",
		"actionType":      "restart",
	})
	req2 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload2))
	rr2 := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests due to cooldown, got %d. Body: %s", rr2.Code, rr2.Body.String())
	}

	var errResp map[string]interface{}
	_ = json.Unmarshal(rr2.Body.Bytes(), &errResp)
	rem, exists := errResp["cooldownRemainingSec"]
	if !exists || rem.(float64) <= 0 {
		t.Errorf("expected positive cooldownRemainingSec in error response, got %v", rem)
	}
}

func TestPhase2_Safety_MaxConcurrentInFlightActions(t *testing.T) {
	setupPhase2TestEnvironment()
	Safety.MaxConcurrent = 2

	// Register 2 in-flight actions to hit the max concurrent limit
	Safety.RegisterDispatch("plan-1", "node-b", "restart", 1)
	Safety.RegisterDispatch("plan-2", "node-c", "restart", 1)

	// Attempting a 3rd action on node-a must be blocked with 429
	err := Safety.CanExecute("node-a", 1)
	if err == nil {
		t.Fatalf("expected error exceeding max concurrent in-flight limit, got nil")
	}

	// Completing one action allows the next to proceed
	Safety.CompleteDispatch("plan-1")
	errAfter := Safety.CanExecute("node-a", 1)
	if errAfter != nil {
		t.Fatalf("expected execution allowed after completing in-flight action, got error: %v", errAfter)
	}
}

func TestPhase2_Safety_GlobalBlastRadiusBudget(t *testing.T) {
	setupPhase2TestEnvironment()
	Safety.BlastRadiusBudget = 5

	// Action with blast radius 4 is currently running
	Safety.RegisterDispatch("plan-big", "node-c", "reroute_traffic", 4)

	// New action with blast radius 2 would total 6 > budget 5 -> blocked
	err := Safety.CanExecute("node-b", 2)
	if err == nil {
		t.Fatalf("expected error exceeding blast radius budget (4 + 2 > 5), got nil")
	}

	// Action with blast radius 1 (total 5 <= budget 5) is allowed
	errSmall := Safety.CanExecute("node-b", 1)
	if errSmall != nil {
		t.Fatalf("expected blast radius 1 to fit in budget, got error: %v", errSmall)
	}
}

func TestPhase2_Recovery_IdempotencyKeyReplay(t *testing.T) {
	store := setupPhase2TestEnvironment()
	idempotencyKey := "idemp-phase2-test-key-999"

	// Pre-approve action on node-b
	actionID := "rec-restart-node-b"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-b",
		ActionType:      "restart",
		Approver:        "admin@enterprise.io",
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-b",
		"actionType":      "restart",
	})

	// 1. Initial request with Idempotency-Key
	req1 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	req1.Header.Set("Idempotency-Key", idempotencyKey)
	rr1 := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on first dispatch, got %d. Body: %s", rr1.Code, rr1.Body.String())
	}

	var resp1 map[string]interface{}
	_ = json.Unmarshal(rr1.Body.Bytes(), &resp1)
	planID := resp1["planId"].(string)

	// Verify recorded in storage
	stored, err := store.GetRecoveryExecutionByIdempotencyKey(context.Background(), idempotencyKey)
	if err != nil || stored == nil {
		t.Fatalf("expected execution stored with idempotency key, err: %v", err)
	}
	if stored.ID != planID {
		t.Errorf("stored plan ID mismatch: %s vs %s", stored.ID, planID)
	}

	// 2. Replay request with same Idempotency-Key
	req2 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	req2.Header.Set("Idempotency-Key", idempotencyKey)
	rr2 := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on idempotent replay, got %d. Body: %s", rr2.Code, rr2.Body.String())
	}

	var resp2 map[string]interface{}
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp2)

	if resp2["idempotentReplay"] != true {
		t.Errorf("expected idempotentReplay to be true, got %v", resp2["idempotentReplay"])
	}
	if resp2["planId"] != planID {
		t.Errorf("replayed planId mismatch: got %v, want %s", resp2["planId"], planID)
	}
}

func TestPhase2_Recovery_DryRunSimulation(t *testing.T) {
	setupPhase2TestEnvironment()
	initialReplicas := State.Services["node-a"].Replicas

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "rec-scale-node-a",
		"targetServiceId": "node-a",
		"actionType":      "scale",
		"dryRun":          true,
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for dry-run simulation, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["dryRun"] != true {
		t.Errorf("expected dryRun: true in response, got %v", resp["dryRun"])
	}
	// Service replicas must remain completely unchanged
	if State.Services["node-a"].Replicas != initialReplicas {
		t.Errorf("dry-run must not mutate service replicas: before=%d, after=%d",
			initialReplicas, State.Services["node-a"].Replicas)
	}
	// Cooldown must not be triggered by dry-run
	if Safety.GetCooldownRemainingSec("node-a") > 0 {
		t.Errorf("dry-run must not activate cooldown on target service")
	}
}

func TestPhase2_Recovery_RollbackAction(t *testing.T) {
	store := setupPhase2TestEnvironment()

	// 1. Manually insert a completed scale-up execution in storage
	planID := "plan-rollback-test-1"
	now := time.Now().UTC()
	execRecord := &storage.RecoveryExecution{
		ID:               planID,
		PlanID:           planID,
		ActionID:         "act-scale-node-a",
		ActionType:       "SCALE_UP",
		TargetServiceID:  "node-a",
		Status:           "Verified",
		RiskLevel:        "MEDIUM",
		RiskScore:        0.55,
		RequiresApproval: true,
		Approver:         "admin@enterprise.io",
		StartedAt:        now.Add(-2 * time.Minute),
		CompletedAt:      &now,
		DurationMs:       1500,
		DurationStr:      "1.5s",
	}
	_ = store.CreateRecoveryExecution(context.Background(), execRecord)

	// Simulate current scale state (replicas scaled from 2 to 4)
	State.Services["node-a"].Replicas = 4

	// 2. Dispatch rollback via router
	router := mux.NewRouter()
	router.HandleFunc("/api/recovery/rollback/{id}", RollbackRecoveryHandler).Methods("POST")

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/recovery/rollback/%s", planID), nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from rollback handler, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "RolledBack" {
		t.Errorf("expected status 'RolledBack', got '%v'", resp["status"])
	}

	// Service replicas should be reduced back
	if State.Services["node-a"].Replicas != 2 {
		t.Errorf("expected replicas reduced back to 2, got %d", State.Services["node-a"].Replicas)
	}

	// Storage record updated to RolledBack
	updated, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || updated.Status != "RolledBack" {
		t.Errorf("expected stored execution status RolledBack, got %s, err: %v", updated.Status, err)
	}
}

func TestPhase2_Recovery_VerificationWindowAndEscalation(t *testing.T) {
	store := setupPhase2TestEnvironment()
	Safety.VerificationWindow = 1100 * time.Millisecond

	// 1. Success case: execute restart, service metrics normalize, status becomes Verified
	actionID := "rec-restart-gateway"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "gateway",
		ActionType:      "restart",
		Approver:        "admin@enterprise.io",
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "gateway",
		"actionType":      "restart",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from execute handler, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	planID := resp["planId"].(string)

	// Wait for asynchronous verification window to complete
	time.Sleep(1500 * time.Millisecond)

	execSuccess, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || execSuccess == nil {
		t.Fatalf("failed to retrieve execution record: %v", err)
	}
	if execSuccess.Status != "Verified" {
		t.Errorf("expected status 'Verified' for recovered service, got '%s'", execSuccess.Status)
	}
	if execSuccess.DurationMs <= 0 {
		t.Errorf("expected positive real durationMs, got %d", execSuccess.DurationMs)
	}

	// 2. Ineffective case: if metrics fail to normalize, transitions to Ineffective with escalation
	Safety.Reset()
	Safety.VerificationWindow = 1100 * time.Millisecond
	actionID2 := "rec-scale-node-b"
	State.ApprovedPlans[actionID2] = true
	State.ApprovedRecords[actionID2] = ApprovalRecord{
		ActionID:        actionID2,
		TargetServiceID: "node-b",
		ActionType:      "scale",
		Approver:        "admin@enterprise.io",
	}

	payload2, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID2,
		"targetServiceId": "node-b",
		"actionType":      "scale",
	})

	req2 := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload2))
	rr2 := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr2.Code)
	}

	var resp2 map[string]interface{}
	_ = json.Unmarshal(rr2.Body.Bytes(), &resp2)
	planID2 := resp2["planId"].(string)

	// Keep node-b degraded immediately so verification fails
	go func() {
		for i := 0; i < 15; i++ {
			State.mu.Lock()
			if s, ok := State.Services["node-b"]; ok {
				s.Status = "critical"
				s.ErrorRate = 0.50
				s.Latency = 999.0
			}
			State.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	time.Sleep(1600 * time.Millisecond)

	execFailed, err := store.GetRecoveryExecution(context.Background(), planID2)
	if err != nil || execFailed == nil {
		t.Fatalf("failed to retrieve failed execution record: %v", err)
	}
	if execFailed.Status != "Ineffective" {
		t.Errorf("expected status 'Ineffective' for unrecovered service, got '%s'", execFailed.Status)
	}

	// Verify an escalation alert was generated
	State.mu.RLock()
	foundEscalation := false
	for _, a := range State.Alerts {
		if a.ServiceID == "node-b" && strings.EqualFold(a.Severity, "critical") {
			foundEscalation = true
			break
		}
	}
	State.mu.RUnlock()

	if !foundEscalation {
		t.Errorf("expected critical escalation alert emitted for ineffective remediation on node-b")
	}
}

