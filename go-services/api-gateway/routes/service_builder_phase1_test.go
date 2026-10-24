package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aegis/go-services/api-gateway/storage"
)

func TestServiceBuilder_RBACMatrix(t *testing.T) {
	_, router, adminToken, opToken, viewToken := setupPhase3TestRouter(t)

	// Test 1: Service Creation
	// - Unauthenticated -> 401
	// - Viewer -> 403
	// - Operator -> 201 (mode=observe)
	// - Operator -> 400 (mode=auto_remediate on create)
	// - Operator -> 403 (trying to set recovery_binding)
	// - Admin -> 201
	validSvc := map[string]interface{}{
		"id":             "svc-rbac-test",
		"name":           "RBAC Test Service",
		"tier":           "tier-2",
		"monitoringMode": "observe",
		"description":    "Valid service for RBAC matrix test",
	}
	body, _ := json.Marshal(validSvc)

	// Unauthenticated
	req := httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for unauthenticated create service, got %d", w.Code)
	}

	// Viewer
	req = httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 for viewer creating service, got %d", w.Code)
	}

	// Operator setting auto_remediate on create -> 400
	autoRemSvc := map[string]interface{}{
		"id":             "svc-auto-create",
		"name":           "Auto Remediate Create",
		"monitoringMode": "auto_remediate",
	}
	autoBody, _ := json.Marshal(autoRemSvc)
	req = httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(autoBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request when creating service with auto_remediate mode, got %d", w.Code)
	}

	// Operator setting recovery_binding on create -> 403
	bindingSvc := map[string]interface{}{
		"id":             "svc-binding-create",
		"name":           "Binding Create",
		"monitoringMode": "recommend",
		"recoveryBinding": map[string]interface{}{
			"runtime":        "kubernetes",
			"target":         "default/my-deployment",
			"allowedActions": []string{"restart", "scale"},
		},
	}
	bindBody, _ := json.Marshal(bindingSvc)
	req = httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(bindBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden for operator setting recovery_binding, got %d", w.Code)
	}

	// Operator creating valid service -> 201
	req = httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created for operator creating valid service, got %d: %s", w.Code, w.Body.String())
	}

	// Test 2: Service Update (Operator vs Admin permissions)
	// Operator trying to set auto_remediate mode -> 403
	upAuto := map[string]interface{}{
		"monitoringMode": "auto_remediate",
	}
	upAutoBody, _ := json.Marshal(upAuto)
	req = httptest.NewRequest("PUT", "/api/registry/services/svc-rbac-test", bytes.NewBuffer(upAutoBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 for operator setting auto_remediate mode, got %d", w.Code)
	}

	// Admin setting recovery binding on service -> 200
	adminBinding := map[string]interface{}{
		"monitoringMode": "recommend",
		"recoveryBinding": map[string]interface{}{
			"runtime":        "kubernetes",
			"target":         "default/my-deployment",
			"allowedActions": []string{"restart", "scale"},
		},
	}
	abBody, _ := json.Marshal(adminBinding)
	req = httptest.NewRequest("PUT", "/api/registry/services/svc-rbac-test", bytes.NewBuffer(abBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for admin setting recovery binding, got %d: %s", w.Code, w.Body.String())
	}

	// Admin setting auto_remediate with binding -> 200
	req = httptest.NewRequest("PUT", "/api/registry/services/svc-rbac-test", bytes.NewBuffer(upAutoBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for admin setting auto_remediate with existing binding, got %d: %s", w.Code, w.Body.String())
	}

	// Test 3: Read Endpoints for Viewer
	// GET /api/registry/services -> 200
	req = httptest.NewRequest("GET", "/api/registry/services", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for viewer listing services, got %d", w.Code)
	}

	// GET /api/registry/suggestions -> 200
	req = httptest.NewRequest("GET", "/api/registry/suggestions", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for viewer listing suggestions, got %d", w.Code)
	}

	// POST /api/registry/validate -> 200
	req = httptest.NewRequest("POST", "/api/registry/validate", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for viewer validating topology, got %d", w.Code)
	}

	// GET /api/registry/export -> 200
	req = httptest.NewRequest("GET", "/api/registry/export?format=json", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for viewer exporting topology, got %d", w.Code)
	}
}

func TestServiceBuilder_SSRFProtection(t *testing.T) {
	// Ensure internal metrics bypass is off
	_ = os.Setenv("AEGIS_ALLOW_INTERNAL_METRICS", "false")
	defer os.Unsetenv("AEGIS_ALLOW_INTERNAL_METRICS")

	blockedURLs := []string{
		"http://127.0.0.1:9090",
		"http://127.0.0.1/api/v1/query",
		"http://localhost:9090",
		"http://test.localhost:8080",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.1.50:9090",
		"http://192.168.1.100:9090",
		"http://172.16.5.10:9090",
		"http://metadata.google.internal/computeMetadata/v1/",
	}

	for _, u := range blockedURLs {
		err := ValidateSafeURL(u)
		if err == nil {
			t.Errorf("Expected SSRF protection to block '%s', but it was allowed", u)
		}
	}

	// Test connection endpoint SSRF enforcement
	_, router, _, opToken, _ := setupPhase3TestRouter(t)

	// Create service pointing to metadata IP
	svc := map[string]interface{}{
		"id":   "ssrf-target-svc",
		"name": "SSRF Target",
	}
	body, _ := json.Marshal(svc)
	req := httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Update service with private telemetry source BaseURL (should be blocked by validation or test-connection)
	ssrfPayload := map[string]interface{}{
		"telemetrySource": map[string]interface{}{
			"type":    "prometheus",
			"baseUrl": "http://169.254.169.254:9090",
		},
	}
	sBody, _ := json.Marshal(ssrfPayload)
	req = httptest.NewRequest("PUT", "/api/registry/services/ssrf-target-svc", bytes.NewBuffer(sBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request when setting SSRF blocked URL, got %d: %s", w.Code, w.Body.String())
	}
}

func TestServiceBuilder_SuggestionLifecycleAndRejectPersistence(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase3TestRouter(t)

	// Ingest span for new nodes -> should create a SUGGESTED edge
	spanPayload := map[string]interface{}{
		"spans": []map[string]interface{}{
			{
				"traceId":         "trace-sugg-100",
				"spanId":          "span-100",
				"sourceServiceId": "service-checkout",
				"targetServiceId": "service-inventory",
				"durationMs":      18.5,
				"statusCode":      200,
			},
		},
	}
	sBody, _ := json.Marshal(spanPayload)
	req := httptest.NewRequest("POST", "/api/telemetry/spans", bytes.NewBuffer(sBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to ingest spans: %d: %s", w.Code, w.Body.String())
	}

	// 1. Verify suggestion appears in GET /api/registry/suggestions
	req = httptest.NewRequest("GET", "/api/registry/suggestions", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to get suggestions: %d", w.Code)
	}
	var suggestions []*storage.TopologyEdge
	_ = json.NewDecoder(w.Body).Decode(&suggestions)
	if len(suggestions) == 0 {
		t.Fatalf("Expected at least 1 suggestion, got %d", len(suggestions))
	}
	suggEdge := suggestions[0]
	if suggEdge.State != "suggested" || suggEdge.Source != "suggested" {
		t.Fatalf("Expected suggested edge, got %+v", suggEdge)
	}

	// 2. Reject suggestion
	req = httptest.NewRequest("POST", fmt.Sprintf("/api/registry/suggestions/%s/reject", suggEdge.ID), nil)
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to reject suggestion: %d", w.Code)
	}

	// 3. Ingest spans again for the SAME edge -> MUST NOT reactivate or re-create suggestion!
	req = httptest.NewRequest("POST", "/api/telemetry/spans", bytes.NewBuffer(sBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Failed to re-ingest spans: %d", w.Code)
	}

	// Verify suggestion list is now empty!
	req = httptest.NewRequest("GET", "/api/registry/suggestions", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var activeSuggestions []*storage.TopologyEdge
	_ = json.NewDecoder(w.Body).Decode(&activeSuggestions)
	if len(activeSuggestions) != 0 {
		t.Fatalf("Expected 0 suggestions after reject, got %d (rejected suggestion re-appeared!)", len(activeSuggestions))
	}
}

func TestServiceBuilder_ManualEdgeNeverOverwritten(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase3TestRouter(t)

	// Create manual edge
	manualEdge := map[string]interface{}{
		"sourceServiceId":  "user-frontend",
		"targetServiceId":  "auth-backend",
		"relationship":     "secure_rpc",
		"edgeType":         "sync",
		"isManualOverride": true,
		"confidence":       1.0,
	}
	body, _ := json.Marshal(manualEdge)
	req := httptest.NewRequest("POST", "/api/registry/edges", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("Failed to create manual edge: %d: %s", w.Code, w.Body.String())
	}

	// Auto-discovery span arrives with errors and different relationship
	spanPayload := map[string]interface{}{
		"spans": []map[string]interface{}{
			{
				"traceId":         "trace-override-test",
				"spanId":          "span-1",
				"sourceServiceId": "user-frontend",
				"targetServiceId": "auth-backend",
				"durationMs":      800.0,
				"statusCode":      500,
			},
		},
	}
	sBody, _ := json.Marshal(spanPayload)
	req = httptest.NewRequest("POST", "/api/telemetry/spans", bytes.NewBuffer(sBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Query edge: must remain manual with confidence 1.0 and relationship preserved
	req = httptest.NewRequest("GET", "/api/registry/edges", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var edges []storage.TopologyEdge
	_ = json.NewDecoder(w.Body).Decode(&edges)

	var targetEdge *storage.TopologyEdge
	for _, e := range edges {
		if e.SourceServiceID == "user-frontend" && e.TargetServiceID == "auth-backend" {
			targetEdge = &e
			break
		}
	}
	if targetEdge == nil {
		t.Fatalf("Manual edge user-frontend -> auth-backend not found")
	}
	if !targetEdge.IsManualOverride || targetEdge.Confidence != 1.0 || targetEdge.Relationship != "secure_rpc" || targetEdge.Source != "manual" {
		t.Fatalf("Manual override edge was corrupted by discovery: %+v", targetEdge)
	}
}

func TestServiceBuilder_ValidationResults(t *testing.T) {
	_, router, _, _, viewToken := setupPhase3TestRouter(t)

	// Create test payload with self-loop, cycle (SCC), orphan node, unmonitored dependency
	valPayload := ValidatePayload{
		Services: []storage.TopologyService{
			{ID: "svc-alpha", Name: "Alpha", MonitoringMode: "observe", TelemetrySource: &storage.TelemetrySource{Type: "agent_push"}},
			{ID: "svc-beta", Name: "Beta", MonitoringMode: "observe", TelemetrySource: &storage.TelemetrySource{Type: "agent_push"}},
			{ID: "svc-gamma", Name: "Gamma", MonitoringMode: "ignore"}, // Unmonitored dependency
			{ID: "svc-isolated", Name: "Isolated", MonitoringMode: "observe"},
			{ID: "svc-need-binding", Name: "Need Binding", MonitoringMode: "recommend"}, // Missing binding
		},
		Edges: []storage.TopologyEdge{
			// Cycle between Alpha and Beta
			{SourceServiceID: "svc-alpha", TargetServiceID: "svc-beta", State: "active"},
			{SourceServiceID: "svc-beta", TargetServiceID: "svc-alpha", State: "active"},
			// Self loop
			{SourceServiceID: "svc-alpha", TargetServiceID: "svc-alpha", State: "active"},
			// Dependency to unmonitored node
			{SourceServiceID: "svc-beta", TargetServiceID: "svc-gamma", State: "active"},
		},
	}

	body, _ := json.Marshal(valPayload)
	req := httptest.NewRequest("POST", "/api/registry/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Validation endpoint failed: %d", w.Code)
	}

	var resp ValidationResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	hasSelfLoop := false
	hasCycle := false
	hasOrphan := false
	hasUnmonitored := false
	hasMissingBinding := false

	for _, iss := range resp.Issues {
		switch iss.Code {
		case "SELF_LOOP":
			hasSelfLoop = true
		case "CYCLE_DETECTED":
			hasCycle = true
		case "ORPHAN_NODE":
			hasOrphan = true
		case "UNMONITORED_DEPENDENCY":
			hasUnmonitored = true
		case "MISSING_RECOVERY_BINDING":
			hasMissingBinding = true
		}
	}

	if !hasSelfLoop {
		t.Errorf("Validation failed to detect SELF_LOOP error")
	}
	if !hasCycle {
		t.Errorf("Validation failed to detect CYCLE_DETECTED warning with SCC")
	}
	if !hasOrphan {
		t.Errorf("Validation failed to detect ORPHAN_NODE for svc-isolated")
	}
	if !hasUnmonitored {
		t.Errorf("Validation failed to detect UNMONITORED_DEPENDENCY for svc-gamma")
	}
	if !hasMissingBinding {
		t.Errorf("Validation failed to detect MISSING_RECOVERY_BINDING for svc-need-binding")
	}
}

func TestServiceBuilder_DryRunImportCorrectness(t *testing.T) {
	_, router, _, opToken, viewToken := setupPhase3TestRouter(t)

	// Create 1 service first
	initialSvc := map[string]interface{}{
		"id":   "existing-svc-1",
		"name": "Existing Service 1",
	}
	body, _ := json.Marshal(initialSvc)
	req := httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Dry run import with 1 update and 1 new service
	importPayload := map[string]interface{}{
		"dryRun": true,
		"services": []map[string]interface{}{
			{"id": "existing-svc-1", "name": "Existing Service 1 Renamed"},
			{"id": "new-svc-2", "name": "Brand New Service 2"},
		},
		"edges": []map[string]interface{}{
			{"sourceServiceId": "existing-svc-1", "targetServiceId": "new-svc-2"},
		},
	}
	impBody, _ := json.Marshal(importPayload)
	req = httptest.NewRequest("POST", "/api/registry/import?dryRun=true", bytes.NewBuffer(impBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Dry run import failed: %d: %s", w.Code, w.Body.String())
	}

	var dryResp struct {
		DryRun  bool       `json:"dryRun"`
		Preview DryRunDiff `json:"preview"`
	}
	_ = json.NewDecoder(w.Body).Decode(&dryResp)
	if !dryResp.DryRun {
		t.Fatalf("Expected dryRun=true in response, got false")
	}
	if len(dryResp.Preview.AddedServices) != 1 || dryResp.Preview.AddedServices[0] != "new-svc-2" {
		t.Errorf("Expected 1 added service 'new-svc-2', got: %+v", dryResp.Preview.AddedServices)
	}
	if len(dryResp.Preview.UpdatedServices) != 1 || dryResp.Preview.UpdatedServices[0] != "existing-svc-1" {
		t.Errorf("Expected 1 updated service 'existing-svc-1', got: %+v", dryResp.Preview.UpdatedServices)
	}

	// Verify that dry-run did NOT persist new-svc-2
	req = httptest.NewRequest("GET", "/api/registry/services/new-svc-2", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("Dry run altered database! new-svc-2 was found with status %d", w.Code)
	}
}

func TestServiceBuilder_CredentialNeverReturnedInResponse(t *testing.T) {
	origAllow := os.Getenv("AEGIS_ALLOW_INTERNAL_METRICS")
	os.Setenv("AEGIS_ALLOW_INTERNAL_METRICS", "true")
	defer os.Setenv("AEGIS_ALLOW_INTERNAL_METRICS", origAllow)

	_, router, adminToken, _, viewToken := setupPhase3TestRouter(t)

	// Admin creates service with sensitive credentials
	secretToken := "ghp_super_secret_pat_token_987654321"
	secretPass := "MySecretPassword#2026!"
	svc := map[string]interface{}{
		"id":             "cred-test-svc",
		"name":           "Credential Test Service",
		"monitoringMode": "observe",
		"telemetrySource": map[string]interface{}{
			"type":         "prometheus",
			"baseUrl":      "https://prometheus.example.com/metrics",
			"authToken":    secretToken,
			"authPassword": secretPass,
		},
	}
	body, _ := json.Marshal(svc)
	req := httptest.NewRequest("POST", "/api/registry/services", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("Failed to create service: %d: %s", w.Code, w.Body.String())
	}

	// 1. Check create response body
	respStr := w.Body.String()
	if strings.Contains(respStr, secretToken) || strings.Contains(respStr, secretPass) {
		t.Fatalf("Credentials leaked in POST /api/registry/services response!")
	}

	// 2. Check GET /api/registry/services/{id}
	req = httptest.NewRequest("GET", "/api/registry/services/cred-test-svc", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	respStr = w.Body.String()
	if strings.Contains(respStr, secretToken) || strings.Contains(respStr, secretPass) {
		t.Fatalf("Credentials leaked in GET /api/registry/services/{id} response!")
	}

	// 3. Check GET /api/registry/services
	req = httptest.NewRequest("GET", "/api/registry/services", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	respStr = w.Body.String()
	if strings.Contains(respStr, secretToken) || strings.Contains(respStr, secretPass) {
		t.Fatalf("Credentials leaked in GET /api/registry/services list response!")
	}

	// 4. Check GET /api/registry/export?format=yaml
	req = httptest.NewRequest("GET", "/api/registry/export?format=yaml", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	respStr = w.Body.String()
	if strings.Contains(respStr, secretToken) || strings.Contains(respStr, secretPass) {
		t.Fatalf("Credentials leaked in GET /api/registry/export (yaml) response!")
	}

	// 5. Check GET /api/registry/export?format=json
	req = httptest.NewRequest("GET", "/api/registry/export?format=json", nil)
	req.Header.Set("Authorization", "Bearer "+viewToken)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	respStr = w.Body.String()
	if strings.Contains(respStr, secretToken) || strings.Contains(respStr, secretPass) {
		t.Fatalf("Credentials leaked in GET /api/registry/export (json) response!")
	}
}
