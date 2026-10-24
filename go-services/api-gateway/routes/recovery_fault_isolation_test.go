package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

// Test (1): dispatch SCALE_UP while a CPU fault is active -> fault still in State.ActiveFaults
func TestRecovery_DispatchScaleUp_FaultRemainsInActiveFaults(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)

	State.mu.Lock()
	State.Services["node-a"] = &Microservice{
		ID:        "node-a",
		Name:      "Node A",
		Type:      "service",
		Status:    "critical",
		CPU:       98.5,
		Latency:   250.0,
		ErrorRate: 0.08,
		Replicas:  2,
	}
	cpuFault := &ChaosInjection{
		ID:                "fault-cpu-ongoing",
		FaultType:         "cpu_stress",
		Title:             "CPU Saturation Attack",
		TargetServiceID:   "node-a",
		TargetServiceName: "Node A",
		Severity:          "critical",
		DurationSeconds:   60,
		RemainingSeconds:  60,
		Status:            "running",
	}
	State.ActiveFaults["fault-cpu-ongoing"] = cpuFault

	actionID := "rec-scale-up-node-a"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		ActionType:      "scale_up",
		Approver:        "sre-lead@aegis",
	}
	State.mu.Unlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "scale_up",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from execute handler, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	State.mu.RLock()
	fault, exists := State.ActiveFaults["fault-cpu-ongoing"]
	replicas := State.Services["node-a"].Replicas
	State.mu.RUnlock()

	if !exists || fault == nil {
		t.Fatalf("Expected fault-cpu-ongoing to remain active in State.ActiveFaults after recovery dispatch")
	}
	if replicas != 4 {
		t.Fatalf("Expected replicas to increase to 4 after scale_up, got %d", replicas)
	}
	WaitForRecoveryVerifications()
}

// Test (2): fault ongoing + no metric improvement -> Ineffective -> escalation to secondary action
func TestRecovery_FaultOngoing_NoMetricImprovement_IneffectiveAndEscalation(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	Safety.SetVerificationWindow(80 * time.Millisecond)

	State.mu.Lock()
	State.Services["node-a"] = &Microservice{
		ID:        "node-a",
		Name:      "Node A",
		Type:      "service",
		Status:    "critical",
		CPU:       98.5,
		Latency:   450.0,
		ErrorRate: 0.25,
		Replicas:  2,
	}
	State.ActiveFaults["fault-cpu-unmitigated"] = &ChaosInjection{
		ID:                "fault-cpu-unmitigated",
		FaultType:         "cpu_stress",
		Title:             "Severe CPU Overload",
		TargetServiceID:   "node-a",
		TargetServiceName: "Node A",
		Severity:          "critical",
		DurationSeconds:   120,
		RemainingSeconds:  120,
		Status:            "running",
	}

	actionID := "rec-scale-up-node-a-ineffective"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		ActionType:      "scale_up",
		Approver:        "sre@aegis",
	}
	State.mu.Unlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "scale_up",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	planID := resp["planId"].(string)

	WaitForRecoveryVerifications()

	exec, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || exec == nil {
		t.Fatalf("Failed to fetch execution record: %v", err)
	}

	if exec.Status != "Ineffective" {
		t.Fatalf("Expected status 'Ineffective' due to ongoing unmitigated fault, got '%s'", exec.Status)
	}
	if exec.EscalatedAction == "" {
		t.Fatalf("Expected EscalatedAction to be populated on Ineffective verification")
	}

	State.mu.RLock()
	var foundEscalationAlert bool
	for _, alert := range State.Alerts {
		if alert.ServiceID == "node-a" && alert.Severity == "critical" {
			foundEscalationAlert = true
			break
		}
	}
	State.mu.RUnlock()

	if !foundEscalationAlert {
		t.Fatalf("Expected critical escalation alert logged in State.Alerts")
	}
}

// Test (3): fault expires naturally -> verification can pass
func TestRecovery_FaultExpiresNaturally_VerificationPasses(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	Safety.SetVerificationWindow(200 * time.Millisecond)

	State.mu.Lock()
	State.Services["node-a"] = &Microservice{
		ID:        "node-a",
		Name:      "Node A",
		Type:      "service",
		Status:    "critical",
		CPU:       95.0,
		Latency:   300.0,
		ErrorRate: 0.10,
		Replicas:  2,
	}
	faultID := "fault-short-lived"
	State.ActiveFaults[faultID] = &ChaosInjection{
		ID:                faultID,
		FaultType:         "cpu_stress",
		Title:             "Short Pulse CPU Fault",
		TargetServiceID:   "node-a",
		TargetServiceName: "Node A",
		Severity:          "warning",
		DurationSeconds:   1,
		RemainingSeconds:  1,
		Status:            "running",
	}

	actionID := "rec-restart-node-a-natural-expire"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		ActionType:      "restart",
		Approver:        "operator@aegis",
	}
	State.mu.Unlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "restart",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	planID := resp["planId"].(string)

	// Simulate fault natural expiration during the verification window
	go func() {
		time.Sleep(30 * time.Millisecond)
		State.mu.Lock()
		delete(State.ActiveFaults, faultID)
		if svc, ok := State.Services["node-a"]; ok {
			svc.Status = "healthy"
			svc.CPU = 20.0
			svc.Latency = 15.0
			svc.ErrorRate = 0.001
		}
		State.mu.Unlock()
	}()

	WaitForRecoveryVerifications()

	exec, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || exec == nil {
		t.Fatalf("Failed to retrieve execution record: %v", err)
	}

	if exec.Status != "Verified" {
		t.Fatalf("Expected status 'Verified' after fault naturally expired, got '%s'", exec.Status)
	}
}

// Test (4): Set DurationMs/DurationStr from the real verification window when finalized
func TestRecovery_DurationMsAndDurationStr_Finalized(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	Safety.SetVerificationWindow(120 * time.Millisecond)

	State.mu.Lock()
	State.Services["node-a"] = &Microservice{
		ID:        "node-a",
		Name:      "Node A",
		Type:      "service",
		Status:    "healthy",
		CPU:       20.0,
		Latency:   15.0,
		ErrorRate: 0.001,
		Replicas:  2,
	}

	actionID := "rec-restart-node-a-duration-test"
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		ActionType:      "restart",
		Approver:        "operator@aegis",
	}
	State.mu.Unlock()

	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "restart",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	planID := resp["planId"].(string)

	// Immediate check: status is Verifying, duration is 0
	initExec, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || initExec == nil {
		t.Fatalf("Failed to fetch initial execution record: %v", err)
	}
	if initExec.Status != "Verifying" {
		t.Fatalf("Expected initial status 'Verifying', got '%s'", initExec.Status)
	}

	WaitForRecoveryVerifications()

	finalExec, err := store.GetRecoveryExecution(context.Background(), planID)
	if err != nil || finalExec == nil {
		t.Fatalf("Failed to fetch finalized execution record: %v", err)
	}

	if finalExec.Status != "Verified" {
		t.Fatalf("Expected final status 'Verified', got '%s'", finalExec.Status)
	}
	if finalExec.DurationMs <= 0 {
		t.Fatalf("Expected positive DurationMs after finalization, got %d", finalExec.DurationMs)
	}
	if finalExec.DurationStr == "" || finalExec.DurationStr == "0.0s" {
		t.Fatalf("Expected non-zero DurationStr after finalization, got '%s'", finalExec.DurationStr)
	}
}
