package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aegis/go-services/api-gateway/storage"
)

func TestServiceBuilder_Phase3_RecoveryGatesAndTopologyVersion(t *testing.T) {
	_, router, adminToken, _, _ := setupPhase3TestRouter(t)

	// 1. Create an observe-only service (no recovery binding, observe mode)
	obsSvc := storage.TopologyService{
		ID:             "obs-svc-1",
		Name:           "Observe Only Service",
		MonitoringMode: "observe",
		Namespace:      "default",
	}
	_ = GetAuthStore().UpsertService(context.Background(), &obsSvc)
	State.mu.Lock()
	State.Services["obs-svc-1"] = &Microservice{ID: "obs-svc-1", Name: "Observe Only Service", Status: "degraded", Latency: 250.0}
	State.mu.Unlock()

	// Attempt recovery on observe-only service -> Must be rejected with observe-only error
	body, _ := json.Marshal(map[string]interface{}{
		"targetServiceId": "obs-svc-1",
		"actionType":      "restart",
	})
	req := httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for observe-only service, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "service monitoring mode does not permit recovery execution") {
		t.Fatalf("Expected explicit observe-mode rejection, got %q", w.Body.String())
	}

	// 2. Create service in recommend mode but with NO recovery binding -> Must be rejected
	noBindingSvc := storage.TopologyService{
		ID:             "no-binding-svc",
		Name:           "No Binding Service",
		MonitoringMode: "recommend",
		Namespace:      "default",
	}
	_ = GetAuthStore().UpsertService(context.Background(), &noBindingSvc)
	State.mu.Lock()
	State.Services["no-binding-svc"] = &Microservice{ID: "no-binding-svc", Name: "No Binding Service", Status: "degraded", Latency: 250.0}
	State.mu.Unlock()

	body, _ = json.Marshal(map[string]interface{}{
		"targetServiceId": "no-binding-svc",
		"actionType":      "restart",
	})
	req = httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for service without binding, got %d: %s", w.Code, w.Body.String())
	}

	// 3. An unsupported runtime must be rejected before evaluating the action.
	restrictedSvc := storage.TopologyService{
		ID:             "scale-only-svc",
		Name:           "Scale Only Service",
		MonitoringMode: "recommend",
		Namespace:      "default",
		RecoveryBinding: &storage.RecoveryBinding{
			Runtime:        "kubernetes",
			Target:         "default/scale-only-svc",
			AllowedActions: []string{"scale"},
		},
	}
	_ = GetAuthStore().UpsertService(context.Background(), &restrictedSvc)
	State.mu.Lock()
	State.Services["scale-only-svc"] = &Microservice{ID: "scale-only-svc", Name: "Scale Only Service", Status: "degraded", Latency: 250.0}
	State.mu.Unlock()

	body, _ = json.Marshal(map[string]interface{}{
		"targetServiceId": "scale-only-svc",
		"actionType":      "restart",
	})
	req = httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("Expected 501 Not Implemented for unsupported runtime, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Dry-run does not bypass an unsupported recovery runtime.
	body, _ = json.Marshal(map[string]interface{}{
		"targetServiceId": "scale-only-svc",
		"actionType":      "scale",
		"dryRun":          true,
	})
	req = httptest.NewRequest("POST", "/api/recovery/execute", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("Expected 501 Not Implemented for unsupported runtime, got %d: %s", w.Code, w.Body.String())
	}

	// 5. Verify Prediction Output includes TopologyVersion
	pred := ComputePredictionOutput()
	if pred.TopologyVersion <= 0 {
		t.Fatalf("Expected TopologyVersion > 0, got %d", pred.TopologyVersion)
	}
}
