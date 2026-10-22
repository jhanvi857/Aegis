package routes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
)

// Helper to set up an isolated test router with fresh in-memory store
func setupSecurityTestEnvironment(t *testing.T) (http.Handler, storage.Store, string, string, string, string) {
	t.Helper()
	store := storage.NewMemoryStore()
	SetAuthStore(store)
	InitState()

	// 1. Create Admin User (first user)
	saltAdmin, _ := storage.GenerateSalt()
	hashAdmin, _ := storage.HashPassword("AdminPass#2026", saltAdmin)
	adminUser := &storage.User{
		ID:           "usr-admin",
		Name:         "Admin Alice",
		Email:        "admin@enterprise.io",
		PasswordHash: hashAdmin,
		Salt:         saltAdmin,
		Role:         storage.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(context.Background(), adminUser)
	adminToken := "tok-admin-12345"
	_ = store.CreateSession(context.Background(), &storage.Session{
		ID:        adminToken,
		UserID:    adminUser.ID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	// 2. Create Operator User
	saltOp, _ := storage.GenerateSalt()
	hashOp, _ := storage.HashPassword("OperatorPass#2026", saltOp)
	opUser := &storage.User{
		ID:           "usr-operator",
		Name:         "Operator Bob",
		Email:        "operator@enterprise.io",
		PasswordHash: hashOp,
		Salt:         saltOp,
		Role:         storage.RoleOperator,
		CreatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(context.Background(), opUser)
	operatorToken := "tok-operator-12345"
	_ = store.CreateSession(context.Background(), &storage.Session{
		ID:        operatorToken,
		UserID:    opUser.ID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	// 3. Create Viewer User
	saltViewer, _ := storage.GenerateSalt()
	hashViewer, _ := storage.HashPassword("ViewerPass#2026", saltViewer)
	viewerUser := &storage.User{
		ID:           "usr-viewer",
		Name:         "Viewer Charlie",
		Email:        "viewer@enterprise.io",
		PasswordHash: hashViewer,
		Salt:         saltViewer,
		Role:         storage.RoleViewer,
		CreatedAt:    time.Now().UTC(),
	}
	_ = store.CreateUser(context.Background(), viewerUser)
	viewerToken := "tok-viewer-12345"
	_ = store.CreateSession(context.Background(), &storage.Session{
		ID:        viewerToken,
		UserID:    viewerUser.ID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC(),
	})

	// 4. Create Machine API Key (Operator role)
	rawKey := "aegis_ak_testmachinekey1234567890"
	h := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(h[:])
	apiKey := &storage.APIKey{
		ID:        "key-machine-1",
		Name:      "telemetry-agent",
		KeyPrefix: rawKey[:15],
		KeyHash:   keyHash,
		Scopes:    "telemetry:write,chaos:inject",
		Role:      storage.RoleOperator,
		CreatedBy: adminUser.ID,
		CreatedAt: time.Now().UTC(),
	}
	_ = store.CreateAPIKey(context.Background(), apiKey)

	router := SetupRouter(store)
	return router, store, adminToken, operatorToken, viewerToken, rawKey
}

func TestSecurity_UnauthenticatedCallsReturn401(t *testing.T) {
	router, _, _, _, _, _ := setupSecurityTestEnvironment(t)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/auth/me"},
		{"POST", "/api/auth/logout"},
		{"GET", "/api/topology"},
		{"GET", "/api/topology/list"},
		{"POST", "/api/topology/select"},
		{"GET", "/api/telemetry/services"},
		{"GET", "/api/telemetry/alerts"},
		{"GET", "/api/telemetry/logs"},
		{"GET", "/api/experiments"},
		{"GET", "/api/chaos/active"},
		{"GET", "/api/chaos"},
		{"POST", "/api/chaos/inject"},
		{"POST", "/api/chaos/stop/fault-1"},
		{"GET", "/api/predict"},
		{"GET", "/api/prediction/latest"},
		{"GET", "/api/prediction/history"},
		{"GET", "/api/recovery"},
		{"GET", "/api/recovery/recommendations"},
		{"GET", "/api/recovery/pending"},
		{"POST", "/api/recovery/approve"},
		{"POST", "/api/recovery/reject"},
		{"POST", "/api/recovery/execute"},
		{"GET", "/api/recovery/history"},
		{"GET", "/api/audit-logs"},
		{"GET", "/api/users"},
		{"GET", "/api/keys"},
		{"POST", "/api/keys"},
	}

	for _, route := range protectedRoutes {
		t.Run(fmt.Sprintf("%s %s without auth", route.method, route.path), func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, bytes.NewBuffer([]byte(`{}`)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 Unauthorized for unauthenticated call to %s %s, got %d. Body: %s",
					route.method, route.path, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestSecurity_PublicEndpointsRemainOpen(t *testing.T) {
	router, _, _, _, _, _ := setupSecurityTestEnvironment(t)

	publicRoutes := []struct {
		method         string
		path           string
		body           string
		expectedStatus int
	}{
		{"GET", "/health", "", http.StatusOK},
		{"POST", "/api/auth/login", `{"email":"unknown@domain.com","password":"bad"}`, http.StatusUnauthorized}, // 401 is normal for bad creds, not blocked by middleware
		{"POST", "/api/auth/signup", `{}`, http.StatusBadRequest},                                                // 400 validation error, not 401 auth rejection
	}

	for _, r := range publicRoutes {
		t.Run(fmt.Sprintf("Public %s %s", r.method, r.path), func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, bytes.NewBufferString(r.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != r.expectedStatus {
				t.Errorf("expected %d for public route %s, got %d", r.expectedStatus, r.path, rr.Code)
			}
		})
	}
}

func TestSecurity_ViewerCannotAccessOperatorOrAdminRoutes(t *testing.T) {
	router, _, _, _, viewerToken, _ := setupSecurityTestEnvironment(t)

	restrictedRoutes := []struct {
		method string
		path   string
	}{
		// Operator routes
		{"POST", "/api/topology/select"},
		{"POST", "/api/recovery/approve"},
		{"POST", "/api/recovery/reject"},
		{"POST", "/api/recovery/execute"},
		{"GET", "/api/audit-logs"},
		// Admin routes
		{"POST", "/api/chaos"},
		{"POST", "/api/chaos/inject"},
		{"POST", "/api/chaos/stop/node-a"},
		{"GET", "/api/users"},
		{"PUT", "/api/users/usr-1/role"},
		{"GET", "/api/keys"},
		{"POST", "/api/keys"},
		{"POST", "/api/auth/revoke"},
	}

	for _, route := range restrictedRoutes {
		t.Run(fmt.Sprintf("Viewer forbidden on %s %s", route.method, route.path), func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, bytes.NewBuffer([]byte(`{}`)))
			req.Header.Set("Authorization", "Bearer "+viewerToken)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for viewer on %s %s, got %d. Body: %s",
					route.method, route.path, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestSecurity_OperatorCannotAccessAdminRoutes(t *testing.T) {
	router, _, _, operatorToken, _, _ := setupSecurityTestEnvironment(t)

	adminRoutes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/chaos"},
		{"POST", "/api/chaos/inject"},
		{"POST", "/api/chaos/stop/node-a"},
		{"GET", "/api/users"},
		{"PUT", "/api/users/usr-1/role"},
		{"GET", "/api/keys"},
		{"POST", "/api/keys"},
		{"POST", "/api/auth/revoke"},
	}

	for _, route := range adminRoutes {
		t.Run(fmt.Sprintf("Operator forbidden on %s %s", route.method, route.path), func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, bytes.NewBuffer([]byte(`{}`)))
			req.Header.Set("Authorization", "Bearer "+operatorToken)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden for operator on %s %s, got %d. Body: %s",
					route.method, route.path, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestSecurity_AdminHasFullAccess(t *testing.T) {
	router, _, adminToken, _, _, _ := setupSecurityTestEnvironment(t)

	// Admin accesses Users endpoint
	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin on /api/users, got %d", rr.Code)
	}

	// Admin accesses Keys endpoint
	reqKeys := httptest.NewRequest("GET", "/api/keys", nil)
	reqKeys.Header.Set("Authorization", "Bearer "+adminToken)
	rrKeys := httptest.NewRecorder()
	router.ServeHTTP(rrKeys, reqKeys)
	if rrKeys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin on /api/keys, got %d", rrKeys.Code)
	}

	// Admin accesses Audit Logs endpoint
	reqAudit := httptest.NewRequest("GET", "/api/audit-logs", nil)
	reqAudit.Header.Set("Authorization", "Bearer "+adminToken)
	rrAudit := httptest.NewRecorder()
	router.ServeHTTP(rrAudit, reqAudit)
	if rrAudit.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin on /api/audit-logs, got %d", rrAudit.Code)
	}
}

func TestSecurity_MachineAPIKeyAuthentication(t *testing.T) {
	router, _, _, _, _, rawKey := setupSecurityTestEnvironment(t)

	// 1. Authenticate with valid machine API key using X-API-Key header
	req := httptest.NewRequest("GET", "/api/telemetry/services", nil)
	req.Header.Set("X-API-Key", rawKey)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid X-API-Key, got %d", rr.Code)
	}

	// 2. Authenticate with valid machine API key using Bearer token
	reqBearer := httptest.NewRequest("GET", "/api/telemetry/services", nil)
	reqBearer.Header.Set("Authorization", "Bearer "+rawKey)
	rrBearer := httptest.NewRecorder()
	router.ServeHTTP(rrBearer, reqBearer)
	if rrBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with Bearer API key, got %d", rrBearer.Code)
	}

	// 3. Forged or invalid machine API key returns 401
	badReq := httptest.NewRequest("GET", "/api/telemetry/services", nil)
	badReq.Header.Set("X-API-Key", "aegis_ak_invalidkeyfortesting123456")
	badRr := httptest.NewRecorder()
	router.ServeHTTP(badRr, badReq)
	if badRr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for forged API key, got %d", badRr.Code)
	}
}

func TestSecurity_SignupIgnoresClientSuppliedRole(t *testing.T) {
	router, store, _, _, _, _ := setupSecurityTestEnvironment(t)

	// Since 3 users already exist in setupSecurityTestEnvironment, any new signup must default to viewer,
	// even if the client explicitly requests "admin" or "SecOps Lead"!
	signupPayload, _ := json.Marshal(map[string]string{
		"name":     "Attacker Mallory",
		"email":    "mallory@attacker.io",
		"password": "Password#Mallory2026",
		"role":     "admin", // Client tries to elevate privilege
	})

	req := httptest.NewRequest("POST", "/api/auth/signup", bytes.NewBuffer(signupPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created from Signup, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp AuthResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.User.Role != storage.RoleViewer {
		t.Errorf("CRITICAL SECURITY VULNERABILITY: signup honored client-supplied role! Expected '%s', got '%s'",
			storage.RoleViewer, resp.User.Role)
	}

	// Verify in DB directly
	dbUser, err := store.GetUserByEmail(context.Background(), "mallory@attacker.io")
	if err != nil || dbUser.Role != storage.RoleViewer {
		t.Errorf("expected user in DB to have role '%s', got '%s'", storage.RoleViewer, dbUser.Role)
	}
}

func TestSecurity_AccountLockoutAfterFailedAttempts(t *testing.T) {
	router, _, _, _, _, _ := setupSecurityTestEnvironment(t)

	email := "operator@enterprise.io"
	badLoginPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": "WrongPasswordAttempt",
	})

	// Send 5 consecutive incorrect password requests
	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(badLoginPayload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 Unauthorized, got %d", i, rr.Code)
		}
	}

	// 6th attempt should be blocked with 429 Too Many Requests (or 401 with lockout message)
	lockoutReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(badLoginPayload))
	lockoutReq.Header.Set("Content-Type", "application/json")
	lockoutRr := httptest.NewRecorder()
	router.ServeHTTP(lockoutRr, lockoutReq)

	if lockoutRr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 Too Many Requests after 5 failed login attempts, got %d. Body: %s",
			lockoutRr.Code, lockoutRr.Body.String())
	}
}

func TestSecurity_SessionRevocationAndExpiry(t *testing.T) {
	router, store, _, _, _, _ := setupSecurityTestEnvironment(t)

	// 1. Create a session that expires in 1 second
	shortToken := "tok-short-lived"
	_ = store.CreateSession(context.Background(), &storage.Session{
		ID:        shortToken,
		UserID:    "usr-admin",
		ExpiresAt: time.Now().Add(-10 * time.Second), // Already expired
		CreatedAt: time.Now().Add(-1 * time.Hour),
	})

	reqExpired := httptest.NewRequest("GET", "/api/auth/me", nil)
	reqExpired.Header.Set("Authorization", "Bearer "+shortToken)
	rrExpired := httptest.NewRecorder()
	router.ServeHTTP(rrExpired, reqExpired)
	if rrExpired.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for expired token, got %d", rrExpired.Code)
	}

	// 2. Active token revoked via /api/auth/logout
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    "admin@enterprise.io",
		"password": "AdminPass#2026",
	})
	loginReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRr := httptest.NewRecorder()
	router.ServeHTTP(loginRr, loginReq)

	var authResp AuthResponse
	_ = json.Unmarshal(loginRr.Body.Bytes(), &authResp)

	// Logout to revoke token
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+authResp.Token)
	logoutRr := httptest.NewRecorder()
	router.ServeHTTP(logoutRr, logoutReq)
	if logoutRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on logout, got %d", logoutRr.Code)
	}

	// Subsequent request with the revoked token must return 401
	afterLogoutReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	afterLogoutReq.Header.Set("Authorization", "Bearer "+authResp.Token)
	afterLogoutRr := httptest.NewRecorder()
	router.ServeHTTP(afterLogoutRr, afterLogoutReq)
	if afterLogoutRr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized after token revocation, got %d", afterLogoutRr.Code)
	}
}

func TestSecurity_AppendOnlyAuditLogRecorded(t *testing.T) {
	router, store, adminToken, _, _, _ := setupSecurityTestEnvironment(t)

	// Perform an admin action (create an API key)
	keyPayload, _ := json.Marshal(map[string]interface{}{
		"name":   "ci-cd-runner",
		"role":   "operator",
		"scopes": "chaos:inject",
	})
	req := httptest.NewRequest("POST", "/api/keys", bytes.NewBuffer(keyPayload))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on key generation, got %d", rr.Code)
	}

	// Verify that the action was recorded in the append-only audit log
	logs, err := store.ListAuditLogs(context.Background(), 10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("expected audit logs to contain records, got %v (err: %v)", logs, err)
	}

	foundKeyCreate := false
	for _, l := range logs {
		if l.Action == "key.create" && l.ActorRole == storage.RoleAdmin {
			foundKeyCreate = true
			break
		}
	}

	if !foundKeyCreate {
		t.Errorf("expected audit log to record 'key.create' by admin, logs: %+v", logs)
	}
}
