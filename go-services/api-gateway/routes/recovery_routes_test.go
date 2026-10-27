package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegis/go-services/api-gateway/storage"
)

func setupTestState() {
	InitState()
	Safety.Reset()
	SetAuthStore(storage.NewMemoryStore())
	State.mu.Lock()
	defer State.mu.Unlock()
	State.Services["node-a"] = &Microservice{
		ID:           "node-a",
		Name:         "Node A",
		Type:         "service",
		Status:       "critical",
		CPU:          95.0,
		Latency:      850.0,
		ErrorRate:    0.45,
		Replicas:     2,
		Dependencies: []string{"node-b"},
	}
	State.Services["node-b"] = &Microservice{
		ID:           "node-b",
		Name:         "Node B",
		Type:         "database",
		Status:       "degraded",
		CPU:          30.0,
		Latency:      45.0,
		ErrorRate:    0.05,
		Replicas:     1,
		Dependencies: []string{},
	}
	State.ServiceOrder = []string{"node-a", "node-b"}
}

func TestGetRecommendations_PolicyAndDifferentiation(t *testing.T) {
	setupTestState()

	req := httptest.NewRequest("GET", "/api/recovery/recommendations", nil)
	rr := httptest.NewRecorder()

	GetRecommendationsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var recs []RecoveryAction
	if err := json.Unmarshal(rr.Body.Bytes(), &recs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(recs) != 0 {
		t.Fatalf("services without fresh telemetry and owner recovery policy must not receive recommendations, got %d", len(recs))
	}
}

func TestExecuteRecovery_ApprovalGateEnforcement(t *testing.T) {
	setupTestState()

	// 1. Attempt executing HIGH risk action without approval -> should be blocked with 403
	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "rec-reroute_traffic-node-a",
		"targetServiceId": "node-a",
		"actionType":      "reroute_traffic",
		"approved":        false,
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("missing owner policy must fail closed with 409 before approval, got %d", rr.Code)
	}

	// 2. Grant approval via /recovery/approve with approver identity
	approvePayload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "rec-reroute_traffic-node-a",
		"targetServiceId": "node-a",
		"actionType":      "reroute_traffic",
		"approver":        "secops-lead@aegis",
	})
	appReq := httptest.NewRequest("POST", "/api/recovery/approve", bytes.NewBuffer(approvePayload))
	appRr := httptest.NewRecorder()

	ApproveRecoveryHandler(appRr, appReq)
	if appRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from approve handler, got %d. Body: %s", appRr.Code, appRr.Body.String())
	}

	// An approval record alone must not replace the owner-managed policy.
	execReq := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	execRr := httptest.NewRecorder()

	ExecuteRecoveryHandler(execRr, execReq)
	if execRr.Code != http.StatusConflict {
		t.Fatalf("approval must not bypass missing owner policy; expected 409, got %d", execRr.Code)
	}
}

func TestRejectRecovery_RevokesApproval(t *testing.T) {
	setupTestState()

	actionID := "rec-scale-node-a"
	State.mu.Lock()
	State.ApprovedPlans[actionID] = true
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-a",
		Approver:        "secops@aegis",
	}
	State.mu.Unlock()

	rejectPayload, _ := json.Marshal(map[string]interface{}{
		"actionId": actionID,
	})
	rejReq := httptest.NewRequest("POST", "/api/recovery/reject", bytes.NewBuffer(rejectPayload))
	rejRr := httptest.NewRecorder()

	RejectRecoveryHandler(rejRr, rejReq)

	if rejRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from reject handler, got %d", rejRr.Code)
	}

	State.mu.RLock()
	hasPlan := State.ApprovedPlans[actionID]
	recIDLen := len(State.ApprovedRecords[actionID].ActionID)
	State.mu.RUnlock()

	if hasPlan || recIDLen > 0 {
		t.Errorf("expected approval to be removed after rejection")
	}
}

func TestExecuteRecovery_CannotBypassWithClientFlags(t *testing.T) {
	setupTestState()

	// Client sends approved: true in request body, but server has not approved
	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "rec-reroute_traffic-node-a",
		"targetServiceId": "node-a",
		"actionType":      "reroute_traffic",
		"approved":        true,
		"force":           true,
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("client flags must not bypass missing owner policy; expected 409, got %d", rr.Code)
	}
}

func TestApproveRecovery_RequiresApproverIdentity(t *testing.T) {
	setupTestState()

	// Anonymous / empty approver must be rejected
	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        "rec-scale-node-a",
		"targetServiceId": "node-a",
		"approver":        "",
	})

	req := httptest.NewRequest("POST", "/api/recovery/approve", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	ApproveRecoveryHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing approver identity, got %d", rr.Code)
	}
}

func TestExecuteRecovery_TargetMismatchBlocked(t *testing.T) {
	setupTestState()

	actionID := "rec-scale-node-b"
	// Approved specifically for node-b
	State.mu.Lock()
	State.ApprovedRecords[actionID] = ApprovalRecord{
		ActionID:        actionID,
		TargetServiceID: "node-b",
		ActionType:      "scale",
		Approver:        "admin@aegis",
	}
	State.mu.Unlock()

	// Client attempts to execute this token against node-a
	payload, _ := json.Marshal(map[string]interface{}{
		"actionId":        actionID,
		"targetServiceId": "node-a",
		"actionType":      "scale",
	})

	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(payload))
	rr := httptest.NewRecorder()

	ExecuteRecoveryHandler(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("approval record must not bypass missing owner policy; expected 409, got %d", rr.Code)
	}
}
