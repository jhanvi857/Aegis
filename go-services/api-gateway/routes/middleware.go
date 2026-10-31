package routes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/aegis/go-services/api-gateway/storage"
)

type contextKey string

const CallerCtxKey = contextKey("callerContext")

// GetCaller extracts the CallerContext from the request
func GetCaller(r *http.Request) *storage.CallerContext {
	val := r.Context().Value(CallerCtxKey)
	if val == nil {
		return nil
	}
	caller, ok := val.(*storage.CallerContext)
	if !ok {
		return nil
	}
	return caller
}

// LogAudit logs an audit entry using the current request's caller context
func LogAudit(r *http.Request, action string, targetSvc string, risk string, approval string, payload string, result string, errMsg string) {
	store := GetAuthStore()
	if store == nil {
		return
	}
	caller := GetCaller(r)
	actorID := "anonymous"
	actorEmail := "unauthenticated"
	actorRole := "none"
	if caller != nil {
		actorID = caller.ActorID
		actorEmail = caller.Email
		actorRole = caller.Role
	}

	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}

	entry := &storage.AuditLogEntry{
		ActorID:         actorID,
		ActorEmail:      actorEmail,
		ActorRole:       actorRole,
		Action:          action,
		TargetServiceID: targetSvc,
		RiskLevel:       risk,
		ApprovalStatus:  approval,
		Payload:         payload,
		ResultStatus:    result,
		ErrorMessage:    errMsg,
		IPAddress:       ip,
	}
	_ = store.AppendAuditLog(r.Context(), entry)
}

// AuthMiddleware enforces authentication for all non-whitelisted routes
func AuthMiddleware(store storage.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Allow CORS OPTIONS requests
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Public endpoints whitelist
			if path == "/health" || path == "/healthz" || path == "/readyz" || path == "/metrics" || path == "/v1/traces" || path == "/v1/metrics" || path == "/v1/logs" || path == "/api/auth/login" || path == "/api/auth/signup" {
				next.ServeHTTP(w, r)
				return
			}

			// Check for Machine API Key in X-API-Key or Bearer token
			apiKeyHeader := r.Header.Get("X-API-Key")
			authHeader := r.Header.Get("Authorization")
			token := ""

			if apiKeyHeader != "" {
				token = strings.TrimSpace(apiKeyHeader)
			} else if authHeader != "" {
				token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
			}

			if token == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: missing Authorization header or API key"})
				return
			}

			// Machine API Key path
			if strings.HasPrefix(token, "aegis_ak_") {
				h := sha256.Sum256([]byte(token))
				keyHash := hex.EncodeToString(h[:])
				apiKey, err := store.GetAPIKeyByHash(r.Context(), keyHash)
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: invalid, expired, or revoked API key"})
					return
				}

				caller := &storage.CallerContext{
					ActorID:   apiKey.ID,
					Email:     apiKey.Name,
					Role:      apiKey.Role,
					IsAPIKey:  true,
					APIKeyObj: apiKey,
				}
				ctx := context.WithValue(r.Context(), CallerCtxKey, caller)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Bearer Session Token path
			session, user, err := store.GetSession(r.Context(), token)
			if err != nil || session == nil || user == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: invalid, expired, or revoked session token"})
				return
			}

			caller := &storage.CallerContext{
				ActorID:   user.ID,
				Email:     user.Email,
				Role:      user.Role,
				IsAPIKey:  false,
				APIKeyObj: nil,
			}
			ctx := context.WithValue(r.Context(), CallerCtxKey, caller)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole ensures the caller has at least the minimum role level
func RequireRole(minRole string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller := GetCaller(r)
		if caller == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: authentication required"})
			return
		}

		if !storage.HasMinRole(caller.Role, minRole) {
			LogAudit(r, "rbac.forbidden", "", "", "", "", "BLOCKED", fmt.Sprintf("Role '%s' cannot access route requiring '%s'", caller.Role, minRole))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":        fmt.Sprintf("Forbidden: role '%s' lacks required role '%s'", caller.Role, minRole),
				"currentRole":  caller.Role,
				"requiredRole": minRole,
			})
			return
		}

		next(w, r)
	}
}
