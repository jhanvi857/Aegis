package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegis/go-services/api-gateway/storage"
)

// Test (1): dispatch SCALE_UP while a CPU fault is active -> fault still in State.ActiveFaults
func TestRecovery_DispatchScaleUpWithoutOwnerPolicyIsRejected(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	State.mu.Lock()
	State.ActiveFaults["fault-cpu-ongoing"] = &ChaosInjection{ID: "fault-cpu-ongoing", FaultType: "cpu_stress", TargetServiceID: "node-a", Status: "running"}
	State.ApprovedPlans["rec-scale-up-node-a"] = true
	State.mu.Unlock()
	payload, _ := json.Marshal(map[string]interface{}{"actionId": "rec-scale-up-node-a", "targetServiceId": "node-a", "actionType": "scale_up"})
	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("missing owner policy must fail closed with 409, got %d", rr.Code)
	}
	State.mu.RLock()
	defer State.mu.RUnlock()
	if _, ok := State.ActiveFaults["fault-cpu-ongoing"]; !ok {
		t.Fatal("rejected action changed active fault state")
	}
	if State.Services["node-a"].Replicas != 2 {
		t.Fatal("rejected action changed replica count")
	}
}

func TestRecovery_FaultOngoingWithoutOwnerPolicyDoesNotDispatch(t *testing.T) {
	setupTestState()
	SetAuthStore(storage.NewMemoryStore())
	State.mu.Lock()
	State.ActiveFaults["fault-cpu-unmitigated"] = &ChaosInjection{ID: "fault-cpu-unmitigated", FaultType: "cpu_stress", TargetServiceID: "node-a", Status: "running"}
	State.mu.Unlock()
	payload, _ := json.Marshal(map[string]interface{}{"actionId": "rec-scale-up-node-a-ineffective", "targetServiceId": "node-a", "actionType": "scale_up"})
	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("missing owner policy must fail closed with 409, got %d", rr.Code)
	}
	WaitForRecoveryVerifications()
}

func TestRecovery_FaultExpiryDoesNotReplaceOwnerPolicy(t *testing.T) {
	setupTestState()
	SetAuthStore(storage.NewMemoryStore())
	State.mu.Lock()
	State.ActiveFaults["fault-short-lived"] = &ChaosInjection{ID: "fault-short-lived", FaultType: "cpu_stress", TargetServiceID: "node-a", Status: "running"}
	State.mu.Unlock()
	payload, _ := json.Marshal(map[string]interface{}{"actionId": "rec-restart-node-a-natural-expire", "targetServiceId": "node-a", "actionType": "restart"})
	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("missing owner policy must fail closed with 409, got %d", rr.Code)
	}
	State.mu.RLock()
	defer State.mu.RUnlock()
	if _, ok := State.ActiveFaults["fault-short-lived"]; !ok {
		t.Fatal("rejected action changed active fault state")
	}
}

func TestRecovery_DurationIsNotRecordedWithoutOwnerPolicy(t *testing.T) {
	setupTestState()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	payload, _ := json.Marshal(map[string]interface{}{"actionId": "rec-restart-node-a-duration-test", "targetServiceId": "node-a", "actionType": "restart"})
	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()
	ExecuteRecoveryHandler(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("missing owner policy must fail closed with 409, got %d", rr.Code)
	}
	executions, err := store.ListRecoveryExecutions(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 0 {
		t.Fatalf("policy rejection created %d recovery executions", len(executions))
	}
}
