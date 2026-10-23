package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/config"
	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/aegis/go-services/api-gateway/telemetry"
)

func TestPhase7_LivenessProbe(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	LivenessHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /healthz, got %d", rr.Code)
	}

	var resp struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode /healthz JSON: %v", err)
	}

	if resp.Status != "UP" {
		t.Errorf("expected status 'UP', got '%s'", resp.Status)
	}
	if resp.Service != "aegis-api-gateway" {
		t.Errorf("expected service 'aegis-api-gateway', got '%s'", resp.Service)
	}
}

func TestPhase7_ReadinessProbe_Success(t *testing.T) {
	memStore := storage.NewMemoryStore()
	defer memStore.Close()

	InitState()

	handler := ReadinessHandler(memStore)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /readyz, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp ReadinessResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode /readyz response: %v", err)
	}

	if resp.Status != "READY" {
		t.Errorf("expected readiness status 'READY', got '%s'", resp.Status)
	}

	if comp, ok := resp.Components["storage"]; !ok || comp.Status != "UP" {
		t.Errorf("expected storage component UP, got %v", comp)
	}
	if comp, ok := resp.Components["state_engine"]; !ok || comp.Status != "UP" {
		t.Errorf("expected state_engine component UP, got %v", comp)
	}
}

func TestPhase7_PrometheusMetrics(t *testing.T) {
	// Record test metrics using precise MetricsRegistry APIs
	telemetry.Metrics.IncHTTPRequest("GET", "/api/topology", 200)
	telemetry.Metrics.IncHTTPRequest("POST", "/api/chaos/inject", 200)
	telemetry.Metrics.IncRemediation("restart", "success")
	telemetry.Metrics.IncVerification("VERIFIED")
	telemetry.Metrics.SetActiveDispatches(2)
	telemetry.Metrics.SetMonitoredServices(map[string]int{"healthy": 40, "degraded": 2})
	telemetry.Metrics.ObserveHTTPRequestDuration("GET", "/api/topology", 0.045)

	handler := telemetry.Metrics.Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /metrics, got %d", rr.Code)
	}

	body := rr.Body.String()
	expectedMetrics := []string{
		"aegis_http_requests_total",
		"aegis_remediations_total",
		"aegis_remediation_verifications_total",
		"aegis_monitored_services_total",
		"aegis_active_dispatches",
		"aegis_http_request_duration_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected /metrics exposition to contain '%s', but was missing", metric)
		}
	}
}

func TestPhase7_CORSAllowlist(t *testing.T) {
	memStore := storage.NewMemoryStore()
	defer memStore.Close()

	// Configure strict CORS origins
	cfg := &config.Config{
		ServerPort:         "8000",
		Environment:        "production",
		CORSAllowedOrigins: []string{"https://dashboard.aegis.prod", "https://admin.aegis.prod"},
	}
	SetConfig(cfg)

	r := SetupRouter(memStore)

	// Case 1: Authorized origin in allowlist
	reqValid := httptest.NewRequest("GET", "/healthz", nil)
	reqValid.Header.Set("Origin", "https://dashboard.aegis.prod")
	rrValid := httptest.NewRecorder()

	r.ServeHTTP(rrValid, reqValid)

	allowOrigin := rrValid.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "https://dashboard.aegis.prod" {
		t.Errorf("expected Access-Control-Allow-Origin: https://dashboard.aegis.prod, got '%s'", allowOrigin)
	}
	if rrValid.Header().Get("Vary") != "Origin" {
		t.Errorf("expected Vary: Origin header on allowed CORS response")
	}

	// Case 2: Unauthorized origin (not in allowlist)
	reqInvalid := httptest.NewRequest("GET", "/healthz", nil)
	reqInvalid.Header.Set("Origin", "https://unauthorized-attacker.com")
	rrInvalid := httptest.NewRecorder()

	r.ServeHTTP(rrInvalid, reqInvalid)

	invalidAllowOrigin := rrInvalid.Header().Get("Access-Control-Allow-Origin")
	if invalidAllowOrigin != "" {
		t.Errorf("expected NO Access-Control-Allow-Origin for unauthorized origin, got '%s'", invalidAllowOrigin)
	}
}

func TestPhase7_ConfigValidation(t *testing.T) {
	// Case 1: Production forbids wildcard '*' CORS origin
	invalidCfg1 := &config.Config{
		Environment:        "production",
		CORSAllowedOrigins: []string{"*"},
	}
	if err := invalidCfg1.Validate(); err == nil {
		t.Errorf("expected validation failure for wildcard CORS origin in production, got nil")
	}

	// Case 2: Production with TLS enabled requires cert and key
	invalidCfg2 := &config.Config{
		Environment:        "production",
		TLSEnabled:         true,
		TLSCertFile:        "",
		TLSKeyFile:         "",
		CORSAllowedOrigins: []string{"https://aegis.example.com"},
	}
	if err := invalidCfg2.Validate(); err == nil {
		t.Errorf("expected validation failure for missing TLS cert/key in production, got nil")
	}

	// Case 3: Production requires explicit non-default database password
	invalidCfg3 := &config.Config{
		Environment:         "production",
		DatabaseURL:         "postgres://aegis:aegissecretpassword@localhost:5432/aegis_db",
		CORSAllowedOrigins:  []string{"https://aegis.example.com"},
		BootstrapAdminPass:  "UltraSecureProdPassword#2026",
	}
	if err := invalidCfg3.Validate(); err == nil {
		t.Errorf("expected validation failure for default database password in production, got nil")
	}

	// Case 4: Production restricts chaos unless explicitly authorized
	invalidCfg4 := &config.Config{
		Environment:          "production",
		DatabaseURL:          "postgres://aegis:custom_prod_pass@localhost:5432/aegis_db",
		CORSAllowedOrigins:   []string{"https://aegis.example.com"},
		BootstrapAdminPass:   "UltraSecureProdPassword#2026",
		ChaosEnabled:         true,
		AllowProductionChaos: false,
	}
	if err := invalidCfg4.Validate(); err == nil {
		t.Errorf("expected validation failure for unauthorized chaos in production, got nil")
	}

	// Case 5: Valid production configuration passes
	validCfg := &config.Config{
		Environment:          "production",
		TLSEnabled:           true,
		TLSCertFile:          "/etc/ssl/certs/aegis.crt",
		TLSKeyFile:           "/etc/ssl/private/aegis.key",
		DatabaseURL:          "postgres://aegis_prod:StrongProdPass2026!@postgres:5432/aegis_prod",
		CORSAllowedOrigins:   []string{"https://aegis.example.com"},
		ShutdownTimeout:      15 * time.Second,
		BootstrapAdminEmail:  "security-ops@enterprise.com",
		BootstrapAdminPass:   "UltraSecureProdPassword#2026",
		ChaosEnabled:         false,
		AllowProductionChaos: false,
	}
	if err := validCfg.Validate(); err != nil {
		t.Errorf("expected valid production config to pass validation, got: %v", err)
	}
}

func TestPhase7_ChaosGatingAndSecurity(t *testing.T) {
	memStore := storage.NewMemoryStore()
	defer memStore.Close()

	InitState()

	// 1. Chaos disabled configuration
	disabledCfg := &config.Config{
		ServerPort:   "8000",
		Environment:  "staging",
		ChaosEnabled: false,
	}
	SetConfig(disabledCfg)

	injectPayload := []byte(`{"faultType":"latency","targetServiceId":"node-a","durationSec":30}`)
	req1 := httptest.NewRequest("POST", "/api/chaos/inject", bytes.NewBuffer(injectPayload))
	rr1 := httptest.NewRecorder()

	ChaosInjectHandler(rr1, req1)
	if rr1.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when ChaosEnabled=false, got %d", rr1.Code)
	}

	// 2. Production environment strictly blocks chaos unless AllowProductionChaos is true
	prodCfg := &config.Config{
		ServerPort:           "8000",
		Environment:          "production",
		ChaosEnabled:         true,
		AllowProductionChaos: false,
	}
	SetConfig(prodCfg)

	req2 := httptest.NewRequest("POST", "/api/chaos/inject", bytes.NewBuffer(injectPayload))
	rr2 := httptest.NewRecorder()

	ChaosInjectHandler(rr2, req2)
	if rr2.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden in production when AllowProductionChaos=false, got %d", rr2.Code)
	}
}

