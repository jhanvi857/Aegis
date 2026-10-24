package routes

import (
	"net/http"
	"sync"

	"github.com/aegis/go-services/api-gateway/config"
	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/aegis/go-services/api-gateway/telemetry"
	"github.com/gorilla/mux"
)

var (
	appConfigMu sync.RWMutex
	AppConfig   *config.Config
)

func SetConfig(c *config.Config) {
	appConfigMu.Lock()
	defer appConfigMu.Unlock()
	AppConfig = c
}

func GetConfig() *config.Config {
	appConfigMu.RLock()
	defer appConfigMu.RUnlock()
	return AppConfig
}

// CorsMiddleware enables cross-origin requests respecting the configured CORS allowlist
func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		allowed := false
		cfg := GetConfig()
		if cfg != nil {
			allowed = cfg.IsOriginAllowed(origin)
		} else {
			// Development fallback allowlist
			if origin == "" || origin == "http://localhost:5173" || origin == "http://localhost:3000" || origin == "http://127.0.0.1:5173" || origin == "*" {
				allowed = true
			}
		}

		if allowed && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-API-Key, X-Trace-ID, X-Request-ID, Idempotency-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SetupRouter constructs and configures the complete Aegis HTTP router with auth and RBAC
func SetupRouter(store storage.Store) *mux.Router {
	SetAuthStore(store)

	r := mux.NewRouter()

	// 1. Structured Access Logger
	r.Use(telemetry.RequestLoggerMiddleware)

	// 2. Prometheus Metrics Instrumentation
	r.Use(telemetry.HTTPMetricsMiddleware)

	// 3. CORS Middleware with Strict Origin Allowlist
	r.Use(CorsMiddleware)

	// 4. Global Authentication Middleware applied to all routes (with public whitelist)
	r.Use(AuthMiddleware(store))

	// ─── Public Endpoints (Probes & Auth) ─────────────────────────
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP","service":"api-gateway"}`))
	}).Methods("GET", "OPTIONS")

	r.HandleFunc("/healthz", LivenessHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/readyz", ReadinessHandler(store)).Methods("GET", "OPTIONS")
	r.HandleFunc("/metrics", telemetry.Metrics.Handler()).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/auth/login", LoginHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/signup", SignupHandler).Methods("POST", "OPTIONS")

	// ─── Authenticated: Viewer Role (Read-only + Self) ───────────
	r.HandleFunc("/api/auth/me", RequireRole(storage.RoleViewer, GetMeHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/auth/logout", RequireRole(storage.RoleViewer, LogoutHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/change-password", RequireRole(storage.RoleViewer, ChangePasswordHandler)).Methods("POST", "OPTIONS")

	// Topology (Read)
	r.HandleFunc("/api/topology", RequireRole(storage.RoleViewer, TopologyHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/topology/list", RequireRole(storage.RoleViewer, TopologyListHandler)).Methods("GET", "OPTIONS")

	// Telemetry (Read)
	r.HandleFunc("/api/telemetry/services", RequireRole(storage.RoleViewer, GetServicesHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/services/{id}", RequireRole(storage.RoleViewer, GetServiceByIDHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/alerts", RequireRole(storage.RoleViewer, GetAlertsHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/telemetry/logs", RequireRole(storage.RoleViewer, GetLogsHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/experiments", RequireRole(storage.RoleViewer, GetExperimentsHandler)).Methods("GET", "OPTIONS")

	// Chaos (Read)
	r.HandleFunc("/api/chaos/active", RequireRole(storage.RoleViewer, ChaosActiveHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/chaos", RequireRole(storage.RoleViewer, ChaosHandler)).Methods("GET", "OPTIONS")

	// Prediction (Read)
	r.HandleFunc("/api/predict", RequireRole(storage.RoleViewer, PredictHandler)).Methods("GET", "POST", "OPTIONS")
	r.HandleFunc("/api/prediction/latest", RequireRole(storage.RoleViewer, GetLatestPredictionHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/prediction/history", RequireRole(storage.RoleViewer, GetPredictionHistoryHandler)).Methods("GET", "OPTIONS")

	// Autonomous Recovery (Read)
	r.HandleFunc("/api/recovery", RequireRole(storage.RoleViewer, RecoveryHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/recommendations", RequireRole(storage.RoleViewer, GetRecommendationsHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/pending", RequireRole(storage.RoleViewer, GetPendingRecoveryHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/history", RequireRole(storage.RoleViewer, GetRecoveryHistoryHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/policy-check", RequireRole(storage.RoleViewer, PolicyCheckHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/status/{id}", RequireRole(storage.RoleViewer, StatusCheckHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/recovery/autonomous", RequireRole(storage.RoleViewer, GetAutonomousRecoveryStatusHandler)).Methods("GET", "OPTIONS")

	// Dynamic Service Registry & Topology Versions (Read)
	r.HandleFunc("/api/registry/services", RequireRole(storage.RoleViewer, ListRegistryServicesHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/registry/services/{id}", RequireRole(storage.RoleViewer, GetRegistryServiceHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/registry/edges", RequireRole(storage.RoleViewer, ListRegistryEdgesHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/registry/versions", RequireRole(storage.RoleViewer, ListTopologyVersionsHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/system/mode", RequireRole(storage.RoleViewer, GetSystemModeHandler)).Methods("GET", "OPTIONS")

	// ─── Authenticated: Operator Role (Approve, Execute Low/Med, Topology Select) ───
	r.HandleFunc("/api/topology/select", RequireRole(storage.RoleOperator, TopologySelectHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery", RequireRole(storage.RoleOperator, RecoveryHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/approve", RequireRole(storage.RoleOperator, ApproveRecoveryHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/reject", RequireRole(storage.RoleOperator, RejectRecoveryHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/execute", RequireRole(storage.RoleOperator, ExecuteRecoveryHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/rollback/{id}", RequireRole(storage.RoleOperator, RollbackRecoveryHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/recovery/autonomous", RequireRole(storage.RoleOperator, SetAutonomousRecoveryStatusHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/audit-logs", RequireRole(storage.RoleOperator, ListAuditLogsHandler)).Methods("GET", "OPTIONS")

	// Dynamic Service Registry Management (Operator)
	r.HandleFunc("/api/registry/services", RequireRole(storage.RoleOperator, CreateRegistryServiceHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/registry/services/{id}", RequireRole(storage.RoleOperator, UpdateRegistryServiceHandler)).Methods("PUT", "OPTIONS")
	r.HandleFunc("/api/registry/edges", RequireRole(storage.RoleOperator, CreateRegistryEdgeHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/registry/edges/{source}/{target}", RequireRole(storage.RoleOperator, DeleteRegistryEdgeHandler)).Methods("DELETE", "OPTIONS")

	// Real Telemetry Ingestion (OpenTelemetry Spans & Prometheus Metrics)
	r.HandleFunc("/api/telemetry/spans", RequireRole(storage.RoleOperator, IngestSpansHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/telemetry/metrics", RequireRole(storage.RoleOperator, IngestMetricsHandler)).Methods("POST", "OPTIONS")

	// System Mode Control (Operator)
	r.HandleFunc("/api/system/mode", RequireRole(storage.RoleOperator, SetSystemModeHandler)).Methods("POST", "OPTIONS")

	// ─── Authenticated: Admin Role (Chaos, High-risk, User Mgmt, API Keys) ────────
	r.HandleFunc("/api/chaos", RequireRole(storage.RoleAdmin, ChaosHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/chaos/inject", RequireRole(storage.RoleAdmin, ChaosInjectHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/chaos/stop/{id}", RequireRole(storage.RoleAdmin, ChaosStopHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/revoke", RequireRole(storage.RoleAdmin, RevokeSessionHandler)).Methods("POST", "OPTIONS")

	// Dynamic Registry Administration & Bulk Topology Import (Admin)
	r.HandleFunc("/api/registry/services/{id}", RequireRole(storage.RoleAdmin, DeleteRegistryServiceHandler)).Methods("DELETE", "OPTIONS")
	r.HandleFunc("/api/registry/import", RequireRole(storage.RoleAdmin, ImportTopologyHandler)).Methods("POST", "OPTIONS")

	// User Management
	r.HandleFunc("/api/users", RequireRole(storage.RoleAdmin, ListUsersHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/users/{id}/role", RequireRole(storage.RoleAdmin, UpdateUserRoleHandler)).Methods("PUT", "OPTIONS")

	// Machine API Key Management
	r.HandleFunc("/api/keys", RequireRole(storage.RoleAdmin, ListAPIKeysHandler)).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/keys", RequireRole(storage.RoleAdmin, CreateAPIKeyHandler)).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/keys/{id}", RequireRole(storage.RoleAdmin, RevokeAPIKeyHandler)).Methods("DELETE", "OPTIONS")

	return r
}
