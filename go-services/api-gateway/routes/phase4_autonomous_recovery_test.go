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

// Helper to set up test environment for Phase 4 tests
func setupPhase4TestRouter(t *testing.T) (*storage.MemoryStore, http.Handler, string, string, string) {
	t.Helper()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	InitState()
	Safety.Reset()

	ctx := context.Background()

	saltAdmin, _ := storage.GenerateSalt()
	hashAdmin, _ := storage.HashPassword("AdminPass#2026", saltAdmin)
	adminUser := &storage.User{
		ID:           "usr-admin",
		Name:         "Admin Alice",
		Email:        "admin@aegis.local",
		PasswordHash: hashAdmin,
		Salt:         saltAdmin,
		Role:         storage.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(ctx, adminUser)
	adminToken := "tok-phase4-admin"
	_ = store.CreateSession(ctx, &storage.Session{
		ID:        adminToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	saltOp, _ := storage.GenerateSalt()
	hashOp, _ := storage.HashPassword("OperatorPass#2026", saltOp)
	opUser := &storage.User{
		ID:           "usr-operator",
		Name:         "Operator Bob",
		Email:        "op@aegis.local",
		PasswordHash: hashOp,
		Salt:         saltOp,
		Role:         storage.RoleOperator,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(ctx, opUser)
	opToken := "tok-phase4-operator"
	_ = store.CreateSession(ctx, &storage.Session{
		ID:        opToken,
		UserID:    opUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	saltView, _ := storage.GenerateSalt()
	hashView, _ := storage.HashPassword("ViewerPass#2026", saltView)
	viewUser := &storage.User{
		ID:           "usr-viewer",
		Name:         "Viewer Charlie",
		Email:        "viewer@aegis.local",
		PasswordHash: hashView,
		Salt:         saltView,
		Role:         storage.RoleViewer,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(ctx, viewUser)
	viewToken := "tok-phase4-viewer"
	_ = store.CreateSession(ctx, &storage.Session{
		ID:        viewToken,
		UserID:    viewUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	router := SetupRouter(store)
	return store, router, adminToken, opToken, viewToken
}

func TestPhase4_AutonomousSelfHealing_FullLoop(t *testing.T) {
	setupPhase4TestRouter(t)

	// Ensure target service node-b exists and simulate degraded state from cache_down
	State.mu.Lock()
	if _, ok := State.Services["node-b"]; !ok {
		State.Services["node-b"] = &Microservice{
			ID:           "node-b",
			Name:         "Cache Leaf Node",
			Type:         "cache",
			Status:       "degraded",
			Latency:      450.0,
			CacheMiss:    0.95,
			Dependencies: []string{},
		}
		State.ServiceOrder = append(State.ServiceOrder, "node-b")
	} else {
		State.Services["node-b"].Status = "degraded"
		State.Services["node-b"].Latency = 450.0
		State.Services["node-b"].CacheMiss = 0.95
	}

	// Register an active chaos fault
	State.ActiveFaults["fault-cache-1"] = &ChaosInjection{
		ID:                "fault-cache-1",
		FaultType:         "cache_down",
		Title:             "Cache Outage",
		TargetServiceID:   "node-b",
		TargetServiceName: "Cache Leaf Node",
		Severity:          "warning",
		DurationSeconds:   30,
		RemainingSeconds:  30,
		Status:            "running",
	}
	State.mu.Unlock()

	// A recommendation without an owner-configured executor binding must fail closed.
	exec, err := ExecuteAutonomousRecovery("node-b", "flush_cache", "act-cache-b-1")
	if err == nil { t.Fatalf("expected an unsupported, unbound action to be rejected; got %+v", exec) }

	// Verify target service active fault remains active (not falsely cleared by recovery)
	State.mu.RLock()
	svcB := State.Services["node-b"]
	activeFaultsCount := len(State.ActiveFaults)
	State.mu.RUnlock()

	if activeFaultsCount != 1 {
		t.Fatalf("Expected active faults to remain active (not auto-cleared), got %d", activeFaultsCount)
	}
	if svcB.Status == "healthy" {
		t.Fatalf("Expected node-b status to remain degraded while fault is ongoing")
	}

	if exec != nil { t.Fatalf("rejected action unexpectedly returned an execution record: %+v", exec) }
}

func TestPhase4_AutonomousSelfHealing_ApprovalGate_HoldHighRisk(t *testing.T) {
	_, _, _, _, _ = setupPhase4TestRouter(t)

	// Ingress gateway is high-criticality tier-1; rerouting traffic is high-risk
	State.mu.Lock()
	if _, ok := State.Services["gateway"]; !ok {
		State.Services["gateway"] = &Microservice{
			ID:           "gateway",
			Name:         "API Ingress Gateway",
			Type:         "gateway",
			Status:       "critical",
			Latency:      950.0,
			Dependencies: []string{"node-a"},
		}
		State.ServiceOrder = append(State.ServiceOrder, "gateway")
	} else {
		State.Services["gateway"].Status = "critical"
	}
	State.mu.Unlock()

	// Attempt autonomous recovery on high-risk action
	exec, err := ExecuteAutonomousRecovery("gateway", "reroute_traffic", "act-gw-1")
	if err == nil {
		t.Fatalf("Expected ExecuteAutonomousRecovery to fail on high-risk action, but it succeeded: %+v", exec)
	}

	// Verify gateway was NOT modified and the missing binding is fail-closed.
	State.mu.RLock()
	gw := State.Services["gateway"]
	State.mu.RUnlock()

	if gw.Status != "critical" {
		t.Fatalf("Expected gateway to remain critical, got %s", gw.Status)
	}
}

func TestPhase4_AutonomousRecoveryStatusAndToggle(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase4TestRouter(t)

	// 1. Viewer can read autonomous status
	req := httptest.NewRequest("GET", "/api/recovery/autonomous", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for Viewer reading autonomous status, got %d", w.Code)
	}
	var statusResp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&statusResp)
	if statusResp["policy"] != "two-tier-risk-approval" {
		t.Fatalf("Unexpected policy in response: %+v", statusResp)
	}

	// 2. Viewer cannot toggle autonomous status (403 Forbidden)
	toggleBody, _ := json.Marshal(map[string]bool{"enabled": false})
	req = httptest.NewRequest("POST", "/api/recovery/autonomous", bytes.NewBuffer(toggleBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for Viewer toggling autonomous status, got %d", w.Code)
	}

	// 3. Operator CAN toggle autonomous status (200 OK)
	req = httptest.NewRequest("POST", "/api/recovery/autonomous", bytes.NewBuffer(toggleBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for Operator toggling autonomous status, got %d", w.Code)
	}

	State.mu.RLock()
	currentAuto := State.AutonomousRecovery
	State.mu.RUnlock()
	if currentAuto != false {
		t.Fatalf("Expected State.AutonomousRecovery to be false, got true")
	}
}
