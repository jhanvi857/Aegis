package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

// Helper to create an isolated test router with authenticated test tokens
func setupPhase3TestRouter(t *testing.T) (*storage.MemoryStore, http.Handler, string, string, string) {
	t.Helper()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	InitState()

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
	adminToken := "tok-phase3-admin"
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
	opToken := "tok-phase3-operator"
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
	viewToken := "tok-phase3-viewer"
	_ = store.CreateSession(ctx, &storage.Session{
		ID:        viewToken,
		UserID:    viewUser.ID,
		ExpiresAt: time.Now().Add(2 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	router := SetupRouter(store)
	return store, router, adminToken, opToken, viewToken
}

func TestPhase3_DynamicServiceRegistryCRUD(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase3TestRouter(t)

	// 1. Viewer cannot create a service (403)
	svcPayload := map[string]interface{}{
		"id":           "order-service",
		"name":         "Order Processing Service",
		"namespace":    "production",
		"clusterId":    "k8s-us-east-1",
		"tier":         "tier-1",
		"owner":        "checkout-team@corp.internal",
		"capabilities": "restart,scale,reroute",
		"replicas":     3,
		"maxReplicas":  12,
	}
	body, _ := json.Marshal(svcPayload)

	req := httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for Viewer creating service, got %d", w.Code)
	}

	// 2. Operator CAN create a service (200/201)
	req = httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("Expected 200/201 for Operator creating service, got %d: %s", w.Code, w.Body.String())
	}

	// 3. Viewer CAN list services
	req = httptest.NewRequest("GET", "/api/registry/services?namespace=production", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for Viewer listing services, got %d", w.Code)
	}
	var svcList []storage.TopologyService
	if err := json.NewDecoder(w.Body).Decode(&svcList); err != nil || len(svcList) != 1 {
		t.Fatalf("Expected 1 service in list, got %d (err: %v)", len(svcList), err)
	}
	if svcList[0].Tier != "tier-1" || svcList[0].Owner != "checkout-team@corp.internal" {
		t.Fatalf("Unexpected service attributes: %+v", svcList[0])
	}

	// 4. Operator CAN update service
	updatePayload := map[string]interface{}{
		"name":         "Order Processing Service v2",
		"tier":         "tier-0",
		"capabilities": "restart,scale,flush_cache",
	}
	upBody, _ := json.Marshal(updatePayload)
	req = httptest.NewRequest("PUT", "/api/registry/services/order-service", bytes.NewBuffer(upBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK updating service, got %d: %s", w.Code, w.Body.String())
	}

	// 5. Viewer CANNOT delete service (Requires Operator/Admin)
	req = httptest.NewRequest("DELETE", "/api/registry/services/order-service", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for Viewer deleting service, got %d", w.Code)
	}

	// 6. Operator CAN delete service
	req = httptest.NewRequest("DELETE", "/api/registry/services/order-service", nil)
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for Operator deleting service, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPhase3_EdgeAutoDiscoveryAndManualOverrideProtection(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase3TestRouter(t)

	// 1. Operator creates a manual override edge between service-A and service-B
	manualEdge := map[string]interface{}{
		"sourceServiceId":  "service-A",
		"targetServiceId":  "service-B",
		"relationship":     "grpc_client",
		"isManualOverride": true,
		"confidence":       1.0,
		"namespace":        "default",
		"clusterId":        "cluster-primary",
	}
	body, _ := json.Marshal(manualEdge)
	req := httptest.NewRequest("POST", "/api/registry/edges", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("Failed to create manual edge: %d: %s", w.Code, w.Body.String())
	}

	// 2. OpenTelemetry span arrives discovering service-A -> service-B and service-B -> service-C
	spanPayload := map[string]interface{}{
		"spans": []map[string]interface{}{
			{
				"traceId":          "trace-xyz-100",
				"spanId":           "span-001",
				"sourceServiceId":  "service-A",
				"targetServiceId":  "service-B",
				"protocol":         "http",
				"durationMs":       45.2,
				"statusCode":       200,
				"timestamp":        time.Now().UTC().Format(time.RFC3339),
			},
			{
				"traceId":          "trace-xyz-101",
				"spanId":           "span-002",
				"sourceServiceId":  "service-B",
				"targetServiceId":  "service-C", // Discovered edge
				"protocol":         "grpc",
				"durationMs":       12.1,
				"statusCode":       200,
				"timestamp":        time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	spanBody, _ := json.Marshal(spanPayload)
	req = httptest.NewRequest("POST", "/api/telemetry/spans", bytes.NewBuffer(spanBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to ingest spans: %d: %s", w.Code, w.Body.String())
	}

	// 3. Verify edges:
	// - service-A -> service-B must preserve isManualOverride=true and relationship=grpc_client
	// - service-B -> service-C is auto-discovered with sample count 1 and isManualOverride=false
	req = httptest.NewRequest("GET", "/api/registry/edges", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to list edges: %d", w.Code)
	}
	var edges []storage.TopologyEdge
	_ = json.NewDecoder(w.Body).Decode(&edges)

	var edgeAB, edgeBC *storage.TopologyEdge
	for i := range edges {
		if edges[i].SourceServiceID == "service-A" && edges[i].TargetServiceID == "service-B" {
			edgeAB = &edges[i]
		}
		if edges[i].SourceServiceID == "service-B" && edges[i].TargetServiceID == "service-C" {
			edgeBC = &edges[i]
		}
	}

	if edgeAB == nil || !edgeAB.IsManualOverride || edgeAB.Relationship != "grpc_client" {
		t.Fatalf("Manual override was violated on edge A->B: %+v", edgeAB)
	}

	if edgeBC == nil || edgeBC.IsManualOverride || edgeBC.SampleCount != 1 || edgeBC.Confidence < 0.70 {
		t.Fatalf("Discovered edge B->C missing or invalid: %+v", edgeBC)
	}
}

func TestPhase3_DynamicTopologyBulkImport(t *testing.T) {
	_, router, adminToken, _, viewToken := setupPhase3TestRouter(t)

	// Create a 25-node enterprise microservice topology
	nodes := make([]map[string]interface{}, 25)
	for i := 0; i < 25; i++ {
		svcID := fmt.Sprintf("service-node-%02d", i+1)
		deps := make([]string, 0)
		if i > 0 {
			deps = append(deps, fmt.Sprintf("service-node-%02d", i))
		}
		nodes[i] = map[string]interface{}{
			"id":           svcID,
			"name":         fmt.Sprintf("Microservice Node %02d", i+1),
			"type":         "microservice",
			"namespace":    "ecommerce-core",
			"clusterId":    "us-west-2-prod",
			"tier":         "tier-2",
			"owner":        "platform@enterprise.org",
			"capabilities": "scale,restart,reroute",
			"replicas":     4,
			"maxReplicas":  20,
			"dependencies": deps,
		}
	}

	importPayload := map[string]interface{}{
		"systemName":  "Enterprise Scale Mesh (25 nodes)",
		"description": "Production dynamic mesh import with 25 services and dependency chaining",
		"nodes":       nodes,
	}
	importBody, _ := json.Marshal(importPayload)

	req := httptest.NewRequest("POST", "/api/registry/import", bytes.NewBuffer(importBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Bulk topology import failed with %d: %s", w.Code, w.Body.String())
	}

	var importResp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&importResp)
	if count, ok := importResp["servicesCount"].(float64); !ok || int(count) != 25 {
		t.Fatalf("Expected 25 imported services, got: %v", importResp)
	}

	// Verify versions list contains the new snapshot
	req = httptest.NewRequest("GET", "/api/registry/versions", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to fetch topology versions: %d", w.Code)
	}
	var versions []storage.TopologyVersion
	_ = json.NewDecoder(w.Body).Decode(&versions)
	if len(versions) == 0 || versions[0].Version <= 0 {
		t.Fatalf("Version snapshot not recorded or version invalid: %+v", versions)
	}
}

func TestPhase3_DemoModeToggleAndTelemetryIngestionWithMachineAPIKey(t *testing.T) {
	store, router, adminToken, opToken, viewToken := setupPhase3TestRouter(t)

	// 1. Create a machine API key for the telemetry agent
	keyPayload := map[string]interface{}{
		"name":        "prod-agent-k8s-cluster",
		"description": "Outbound telemetry agent machine key",
		"role":        storage.RoleOperator,
		"expiresIn":   "8760h",
	}
	keyBody, _ := json.Marshal(keyPayload)
	req := httptest.NewRequest("POST", "/api/keys", bytes.NewBuffer(keyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("Failed to create API key: %d: %s", w.Code, w.Body.String())
	}
	var createdKey struct {
		APIKey string `json:"apiKey"`
	}
	_ = json.NewDecoder(w.Body).Decode(&createdKey)
	if createdKey.APIKey == "" {
		t.Fatalf("Expected apiKey in API key response, got empty")
	}

	// 2. Query initial system mode (default must NOT be demo mode unless env is set)
	req = httptest.NewRequest("GET", "/api/system/mode", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get system mode: %d", w.Code)
	}
	var modeResp SystemModeResponse
	_ = json.NewDecoder(w.Body).Decode(&modeResp)
	if modeResp.DemoMode {
		t.Fatalf("Default system mode must not be DemoMode, got true")
	}

	// 3. Switch to DemoMode (Operator)
	setModePayload := map[string]interface{}{
		"demoMode": true,
	}
	smBody, _ := json.Marshal(setModePayload)
	req = httptest.NewRequest("POST", "/api/system/mode", bytes.NewBuffer(smBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to set system mode: %d", w.Code)
	}
	_ = json.NewDecoder(w.Body).Decode(&modeResp)
	if !modeResp.DemoMode || modeResp.ModeLabel != "DEMO MODE (Simulated Telemetry)" {
		t.Fatalf("Expected DEMO MODE label, got %+v", modeResp)
	}

	// Switch back to LIVE CLUSTER mode
	setModePayload["demoMode"] = false
	smBody, _ = json.Marshal(setModePayload)
	req = httptest.NewRequest("POST", "/api/system/mode", bytes.NewBuffer(smBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to reset system mode: %d", w.Code)
	}

	// 4. Ingest real metrics using the machine API key via X-API-Key header
	metricsPayload := map[string]interface{}{
		"serviceId":  "node-a",
		"clusterId":  "prod-k8s-cluster",
		"namespace":  "default",
		"cpu":        42.5,
		"memory":     68.1,
		"latency":    24.0,
		"rps":        350.0,
		"errorRate":  0.002,
		"queueDepth": 3,
	}
	metBody, _ := json.Marshal(metricsPayload)
	req = httptest.NewRequest("POST", "/api/telemetry/metrics", bytes.NewBuffer(metBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", createdKey.APIKey)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to ingest metrics with API key: %d: %s", w.Code, w.Body.String())
	}

	// Verify State was updated with the real metrics
	State.mu.RLock()
	svcA, ok := State.Services["node-a"]
	State.mu.RUnlock()
	if !ok || svcA.CPU != 42.5 || svcA.Memory != 68.1 || svcA.RPS != 350.0 {
		t.Fatalf("State.Services did not reflect real ingested metrics: %+v", svcA)
	}

	_ = store
}
