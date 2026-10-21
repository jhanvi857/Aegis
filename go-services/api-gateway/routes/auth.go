package routes

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Salt         string `json:"-"`
	Role         string `json:"role"`
	CreatedAt    string `json:"createdAt"`
}

type TokenSession struct {
	User      User
	ExpiresAt time.Time
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignupRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

var (
	usersLock    sync.RWMutex
	usersDB      = map[string]User{}
	activeTokens = map[string]TokenSession{}
	serverSecret []byte
)

func init() {
	secretEnv := os.Getenv("AEGIS_AUTH_SECRET")
	if secretEnv != "" {
		serverSecret = []byte(secretEnv)
	} else {
		// Generate cryptographically secure random 256-bit secret key at startup (zero hardcoding)
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic(fmt.Sprintf("Failed to initialize cryptographic entropy: %v", err))
		}
		serverSecret = key
	}
}

func generateCryptographicSalt() (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hex.EncodeToString(salt), nil
}

func hashPasswordWithSalt(password string, saltHex string) string {
	saltBytes, _ := hex.DecodeString(saltHex)
	mac := hmac.New(sha256.New, serverSecret)
	mac.Write(saltBytes)
	mac.Write([]byte(password))
	return hex.EncodeToString(mac.Sum(nil))
}

func generateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GetUserFromRequest extracts and verifies the bearer token from the HTTP Authorization header
func GetUserFromRequest(r *http.Request) (*User, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing Authorization header")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token == "" {
		return nil, errors.New("malformed or empty bearer token")
	}

	usersLock.RLock()
	defer usersLock.RUnlock()

	session, exists := activeTokens[token]
	if !exists {
		return nil, errors.New("invalid or unauthenticated token")
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, errors.New("authentication token expired")
	}

	return &session.User, nil
}

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

	usersLock.Lock()
	defer usersLock.Unlock()

	user, exists := usersDB[email]
	if !exists {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email or credentials"})
		return
	}

	// Constant-time hash verification
	expectedHash := hashPasswordWithSalt(password, user.Salt)
	if !hmac.Equal([]byte(expectedHash), []byte(user.PasswordHash)) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email or credentials"})
		return
	}

	token, err := generateSecureToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to generate secure session token"})
		return
	}

	activeTokens[token] = TokenSession{
		User:      user,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	_ = json.NewEncoder(w).Encode(AuthResponse{
		Token: token,
		User:  user,
	})
}

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

	if len(password) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 6 characters long"})
		return
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "Site Reliability Engineer"
	}

	usersLock.Lock()
	defer usersLock.Unlock()

	if _, exists := usersDB[email]; exists {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "An operator account with this email already exists"})
		return
	}

	salt, err := generateCryptographicSalt()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Cryptographic salt generation failure"})
		return
	}

	passwordHash := hashPasswordWithSalt(password, salt)
	newUser := User{
		ID:           fmt.Sprintf("usr-%d", time.Now().UnixNano()),
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Salt:         salt,
		Role:         role,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	usersDB[email] = newUser

	token, err := generateSecureToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Session token generation failure"})
		return
	}

	activeTokens[token] = TokenSession{
		User:      newUser,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(AuthResponse{
		Token: token,
		User:  newUser,
	})
}

func GetMeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, err := GetUserFromRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"user":          user,
	})
}
