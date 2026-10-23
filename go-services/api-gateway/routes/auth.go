package routes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aegis/go-services/api-gateway/storage"
	"github.com/gorilla/mux"
)

var (
	authStorageMu sync.RWMutex
	authStorage   storage.Store = storage.NewMemoryStore()
)

func SetAuthStore(s storage.Store) {
	authStorageMu.Lock()
	defer authStorageMu.Unlock()
	authStorage = s
}

func GetAuthStore() storage.Store {
	authStorageMu.RLock()
	defer authStorageMu.RUnlock()
	return authStorage
}

// InitBootstrapAdmin creates an initial admin account if configured via environment variables
func InitBootstrapAdmin(ctx context.Context) {
	store := GetAuthStore()
	if store == nil {
		return
	}
	adminEmail := os.Getenv("AEGIS_BOOTSTRAP_ADMIN_EMAIL")
	adminPass := os.Getenv("AEGIS_BOOTSTRAP_ADMIN_PASSWORD")
	if adminEmail == "" || adminPass == "" {
		return
	}

	adminEmail = strings.ToLower(strings.TrimSpace(adminEmail))
	_, err := store.GetUserByEmail(ctx, adminEmail)
	if err == nil {
		// Admin already exists
		return
	}

	salt, err := storage.GenerateSalt()
	if err != nil {
		log.Printf("[auth] Failed to generate salt for bootstrap admin: %v", err)
		return
	}

	hash, err := storage.HashPassword(adminPass, salt)
	if err != nil {
		log.Printf("[auth] Failed to hash password for bootstrap admin: %v", err)
		return
	}

	adminUser := &storage.User{
		ID:           fmt.Sprintf("usr-%d", time.Now().UnixNano()),
		Name:         "Bootstrap Administrator",
		Email:        adminEmail,
		PasswordHash: hash,
		Salt:         salt,
		Role:         storage.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := store.CreateUser(ctx, adminUser); err != nil {
		log.Printf("[auth] Failed to persist bootstrap admin: %v", err)
	} else {
		log.Printf("[auth] Bootstrap admin created: %s (role: %s)", adminEmail, storage.RoleAdmin)
	}
}

type User = storage.User
type AuthResponse struct {
	Token string        `json:"token"`
	User  *storage.User `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"` // Ignored by server for security
}

func generateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetUserFromRequest extracts and verifies the user from request context or token
func GetUserFromRequest(r *http.Request) (*storage.User, error) {
	caller := GetCaller(r)
	if caller != nil && !caller.IsAPIKey {
		return GetAuthStore().GetUserByID(r.Context(), caller.ActorID)
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing Authorization header")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token == "" {
		return nil, errors.New("malformed or empty bearer token")
	}

	session, user, err := GetAuthStore().GetSession(r.Context(), token)
	if err != nil || session == nil || user == nil {
		return nil, errors.New("invalid or expired session token")
	}

	return user, nil
}

// LoginHandler handles user authentication with lockout & audit logging
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid login payload format"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	if email == "" || password == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Email and password are required"})
		return
	}

	ctx := r.Context()
	user, err := GetAuthStore().GetUserByEmail(ctx, email)
	if err != nil {
		LogAudit(r, "auth.login", "", "", "", fmt.Sprintf("email: %s", email), "FAILED", "Invalid email or credentials")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email or credentials"})
		return
	}

	// Check if account is locked
	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		remaining := time.Until(*user.LockedUntil).Round(time.Second)
		LogAudit(r, "auth.login", "", "", "", fmt.Sprintf("email: %s", email), "BLOCKED", fmt.Sprintf("Account locked. Remaining: %v", remaining))
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Account temporarily locked due to consecutive failed login attempts. Try again in %v", remaining),
		})
		return
	}

	// Verify password using Argon2id with constant-time comparison
	if !storage.VerifyPassword(password, user.Salt, user.PasswordHash) {
		failedAttempts := user.FailedLoginAttempts + 1
		var lockedUntil *time.Time
		if failedAttempts >= 5 {
			t := time.Now().Add(15 * time.Minute)
			lockedUntil = &t
			LogAudit(r, "auth.lockout", "", "", "", fmt.Sprintf("email: %s, attempts: %d", email, failedAttempts), "BLOCKED", "Exceeded maximum failed login attempts, account locked for 15 minutes")
		} else {
			LogAudit(r, "auth.login", "", "", "", fmt.Sprintf("email: %s, attempt: %d", email, failedAttempts), "FAILED", "Invalid password")
		}

		_ = GetAuthStore().UpdateUserLoginAttempt(ctx, email, failedAttempts, lockedUntil)

		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email or credentials"})
		return
	}

	// Successful login: reset failed attempts counter
	_ = GetAuthStore().UpdateUserLoginAttempt(ctx, email, 0, nil)

	token, err := generateSecureToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate secure session token"})
		return
	}

	session := &storage.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}

	if err := GetAuthStore().CreateSession(ctx, session); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to persist session"})
		return
	}

	LogAudit(r, "auth.login", "", "", "", fmt.Sprintf("user_id: %s, email: %s", user.ID, user.Email), "SUCCESS", "")

	_ = json.NewEncoder(w).Encode(AuthResponse{
		Token: token,
		User:  user,
	})
}

// SignupHandler handles user registration with strict server-side RBAC
func SignupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid registration payload format"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	name := strings.TrimSpace(req.Name)
	password := req.Password
	if email == "" || password == "" || name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Full name, email, and password are required"})
		return
	}

	if len(password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 8 characters long"})
		return
	}

	ctx := r.Context()
	if _, err := GetAuthStore().GetUserByEmail(ctx, email); err == nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "An operator account with this email already exists"})
		return
	}

	// Server-side RBAC: Role is NEVER taken from signup input.
	// First user = admin (or if matches bootstrap admin env var); others default to viewer.
	role := storage.RoleViewer
	totalUsers, err := GetAuthStore().CountUsers(ctx)
	if err != nil {
		log.Printf("[auth] Error checking user count: %v", err)
	}

	bootstrapEmail := strings.ToLower(strings.TrimSpace(os.Getenv("AEGIS_BOOTSTRAP_ADMIN_EMAIL")))
	if totalUsers == 0 || (bootstrapEmail != "" && email == bootstrapEmail) {
		role = storage.RoleAdmin
	}

	salt, err := storage.GenerateSalt()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Cryptographic salt generation failure"})
		return
	}

	passwordHash, err := storage.HashPassword(password, salt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password hashing failure"})
		return
	}

	newUser := &storage.User{
		ID:           fmt.Sprintf("usr-%d", time.Now().UnixNano()),
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Salt:         salt,
		Role:         role,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := GetAuthStore().CreateUser(ctx, newUser); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create user account"})
		return
	}

	token, err := generateSecureToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Session token generation failure"})
		return
	}

	session := &storage.Session{
		ID:        token,
		UserID:    newUser.ID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	_ = GetAuthStore().CreateSession(ctx, session)

	LogAudit(r, "auth.signup", "", "", "", fmt.Sprintf("user_id: %s, email: %s, role: %s", newUser.ID, newUser.Email, newUser.Role), "SUCCESS", "")

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(AuthResponse{
		Token: token,
		User:  newUser,
	})
}

// GetMeHandler returns current caller identity and permissions
func GetMeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	caller := GetCaller(r)
	if caller == nil {
		user, err := GetUserFromRequest(r)
		if err != nil || user == nil {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthenticated"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": true,
			"isApiKey":      false,
			"user":          user,
			"role":          user.Role,
		})
		return
	}

	if caller.IsAPIKey {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": true,
			"isApiKey":      true,
			"apiKey":        caller.APIKeyObj,
			"role":          caller.Role,
		})
		return
	}

	user, err := GetAuthStore().GetUserByID(r.Context(), caller.ActorID)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "User not found"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"isApiKey":      false,
		"user":          user,
		"role":          user.Role,
	})
}

// LogoutHandler revokes the active session token
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token != "" {
		_ = GetAuthStore().RevokeSession(r.Context(), token)
		LogAudit(r, "auth.logout", "", "", "", "Session revoked", "SUCCESS", "")
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Successfully logged out"})
}

// RevokeSessionRequest represents a request to revoke sessions
type RevokeSessionRequest struct {
	Token  string `json:"token,omitempty"`
	UserID string `json:"userId,omitempty"`
}

// RevokeSessionHandler revokes a session or all sessions for a user
func RevokeSessionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req RevokeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if req.Token != "" {
		if err := GetAuthStore().RevokeSession(ctx, req.Token); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to revoke session: %v"}`, err), http.StatusInternalServerError)
			return
		}
		LogAudit(r, "auth.revoke_token", "", "", "", req.Token, "SUCCESS", "")
	}
	if req.UserID != "" {
		if err := GetAuthStore().RevokeUserSessions(ctx, req.UserID); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to revoke user sessions: %v"}`, err), http.StatusInternalServerError)
			return
		}
		LogAudit(r, "auth.revoke_user_sessions", "", "", "", req.UserID, "SUCCESS", "")
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Session(s) revoked successfully"})
}

// ListUsersHandler returns all registered users
func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	users, err := GetAuthStore().ListUsers(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to list users: %v"}`, err), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(users)
}

type UpdateRoleRequest struct {
	Role string `json:"role"`
}

// UpdateUserRoleHandler updates a user's authorization role
func UpdateUserRoleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	targetUserID := vars["id"]

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid payload"}`, http.StatusBadRequest)
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != storage.RoleViewer && role != storage.RoleOperator && role != storage.RoleAdmin {
		http.Error(w, `{"error":"Invalid role. Must be 'viewer', 'operator', or 'admin'"}`, http.StatusBadRequest)
		return
	}

	if err := GetAuthStore().UpdateUserRole(r.Context(), targetUserID, role); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to update role: %v"}`, err), http.StatusInternalServerError)
		return
	}

	LogAudit(r, "user.role_change", "", "", "", fmt.Sprintf("target_user_id: %s, new_role: %s", targetUserID, role), "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("User role updated to %s", role),
		"userId":  targetUserID,
		"role":    role,
	})
}

// Machine API Key Handlers

type CreateKeyRequest struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Scopes    string `json:"scopes"`
	ExpiresIn int    `json:"expiresInDays,omitempty"` // days
}

// CreateAPIKeyHandler generates a machine API key (admin only)
func CreateAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req CreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, `{"error":"API Key name is required"}`, http.StatusBadRequest)
		return
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role == "" {
		role = storage.RoleOperator
	}
	if role != storage.RoleViewer && role != storage.RoleOperator && role != storage.RoleAdmin {
		http.Error(w, `{"error":"Invalid role. Must be 'viewer', 'operator', or 'admin'"}`, http.StatusBadRequest)
		return
	}

	scopes := strings.TrimSpace(req.Scopes)
	if scopes == "" {
		scopes = "*"
	}

	// Generate secure secret key: aegis_ak_<random 24 bytes hex>
	randBytes := make([]byte, 24)
	if _, err := rand.Read(randBytes); err != nil {
		http.Error(w, `{"error":"Cryptographic generation failure"}`, http.StatusInternalServerError)
		return
	}
	rawSecret := "aegis_ak_" + hex.EncodeToString(randBytes)
	keyPrefix := rawSecret[:15]

	// Hash the raw secret for storage
	h := sha256.Sum256([]byte(rawSecret))
	keyHash := hex.EncodeToString(h[:])

	caller := GetCaller(r)
	createdBy := "admin"
	if caller != nil {
		createdBy = caller.ActorID
	}

	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(req.ExpiresIn) * 24 * time.Hour)
		expiresAt = &exp
	}

	apiKeyObj := &storage.APIKey{
		ID:        fmt.Sprintf("key-%d", time.Now().UnixNano()),
		Name:      name,
		KeyPrefix: keyPrefix,
		KeyHash:   keyHash,
		Scopes:    scopes,
		Role:      role,
		CreatedBy: createdBy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}

	if err := GetAuthStore().CreateAPIKey(r.Context(), apiKeyObj); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to store API key: %v"}`, err), http.StatusInternalServerError)
		return
	}

	LogAudit(r, "key.create", "", "", "", fmt.Sprintf("key_id: %s, name: %s, role: %s, scopes: %s", apiKeyObj.ID, name, role, scopes), "SUCCESS", "")

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"apiKey":    rawSecret, // ONLY EXPOSED ONCE UPON CREATION
		"keyId":     apiKeyObj.ID,
		"keyPrefix": keyPrefix,
		"name":      name,
		"role":      role,
		"scopes":    scopes,
		"expiresAt": expiresAt,
		"message":   "Save this API key now. It will not be shown again.",
	})
}

// ListAPIKeysHandler lists all machine API keys (admin only)
func ListAPIKeysHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	keys, err := GetAuthStore().ListAPIKeys(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to list API keys: %v"}`, err), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(keys)
}

// RevokeAPIKeyHandler revokes a machine API key (admin only)
func RevokeAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	keyID := vars["id"]

	if err := GetAuthStore().RevokeAPIKey(r.Context(), keyID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to revoke API key: %v"}`, err), http.StatusInternalServerError)
		return
	}

	LogAudit(r, "key.revoke", "", "", "", fmt.Sprintf("key_id: %s", keyID), "SUCCESS", "")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "API key revoked successfully",
		"keyId":   keyID,
	})
}

// ListAuditLogsHandler returns audit log entries
func ListAuditLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	entries, err := GetAuthStore().ListAuditLogs(r.Context(), limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to list audit logs: %v"}`, err), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(entries)
}
