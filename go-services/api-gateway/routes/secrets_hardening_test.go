package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aegis/go-services/api-gateway/config"
	"github.com/aegis/go-services/api-gateway/storage"
)

func TestPasswordComplexityValidation(t *testing.T) {
	cases := []struct {
		name    string
		pass    string
		wantErr bool
	}{
		{"TooShort", "Aa1!short", true},
		{"MissingUpper", "lowercase123!@#", true},
		{"MissingLower", "UPPERCASE123!@#", true},
		{"MissingDigit", "NoDigitsHere!@#$", true},
		{"MissingSpecial", "NoSpecialChars1234", true},
		{"ValidComplex", "ValidP@ssw0rd!2026", false},
		{"ValidComplex2", "Str0ng#Secret_Key2026", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := config.ValidatePasswordComplexity(tc.pass)
			if tc.wantErr && err == nil {
				t.Fatalf("expected complexity error for password %q, got nil", tc.pass)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected password %q to pass, got: %v", tc.pass, err)
			}
		})
	}
}

func TestBootstrapAdmin_MustChangePassword_Forced(t *testing.T) {
	memStore := storage.NewMemoryStore()
	defer memStore.Close()
	SetAuthStore(memStore)

	testEmail := "bootstrap-test@enterprise.local"
	testPass := "InitialStr0ng!Pass2026"

	os.Setenv("AEGIS_BOOTSTRAP_ADMIN_EMAIL", testEmail)
	os.Setenv("AEGIS_BOOTSTRAP_ADMIN_PASSWORD", testPass)
	defer os.Unsetenv("AEGIS_BOOTSTRAP_ADMIN_EMAIL")
	defer os.Unsetenv("AEGIS_BOOTSTRAP_ADMIN_PASSWORD")

	ctx := context.Background()
	InitBootstrapAdmin(ctx)

	admin, err := memStore.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("failed to retrieve bootstrap admin: %v", err)
	}
	if !admin.MustChangePassword {
		t.Fatalf("expected MustChangePassword=true for newly created bootstrap admin")
	}

	// Test LoginHandler reflects MustChangePassword
	loginPayload := []byte(`{"email":"` + testEmail + `","password":"` + testPass + `"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	LoginHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected login status 200, got %d", w.Code)
	}

	var authResp AuthResponse
	if err := json.NewDecoder(w.Body).Decode(&authResp); err != nil {
		t.Fatalf("failed to parse auth response: %v", err)
	}
	if !authResp.MustChangePassword {
		t.Errorf("expected AuthResponse.MustChangePassword=true, got false")
	}
}

func TestChangePassword_SuccessAndFailure(t *testing.T) {
	memStore := storage.NewMemoryStore()
	defer memStore.Close()
	SetAuthStore(memStore)

	testEmail := "change-pass-test@enterprise.local"
	initialPass := "OldStr0ng!Pass2026"
	salt, _ := storage.GenerateSalt()
	hash, _ := storage.HashPassword(initialPass, salt)

	user := &storage.User{
		ID:                  "usr-changepw-test",
		Name:                "Password Test User",
		Email:               testEmail,
		PasswordHash:        hash,
		Salt:                salt,
		Role:                storage.RoleAdmin,
		MustChangePassword:  true,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}
	if err := memStore.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	token := "session-token-for-changepw-test"
	sess := &storage.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	_ = memStore.CreateSession(context.Background(), sess)

	// Case 1: Incorrect current password
	wrongPassReq := httptest.NewRequest("POST", "/api/auth/change-password", bytes.NewBufferString(`{
		"currentPassword": "WrongPassword123!",
		"newPassword": "BrandNew!Pass2026"
	}`))
	wrongPassReq.Header.Set("Authorization", "Bearer "+token)
	w1 := httptest.NewRecorder()
	ChangePasswordHandler(w1, wrongPassReq)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong current password, got %d", w1.Code)
	}

	// Case 2: Weak new password
	weakPassReq := httptest.NewRequest("POST", "/api/auth/change-password", bytes.NewBufferString(`{
		"currentPassword": "` + initialPass + `",
		"newPassword": "weak"
	}`))
	weakPassReq.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	ChangePasswordHandler(w2, weakPassReq)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for weak new password, got %d", w2.Code)
	}

	// Case 3: Successful password change meeting complexity
	newPass := "SuperSecure#NewPass2026!"
	validReq := httptest.NewRequest("POST", "/api/auth/change-password", bytes.NewBufferString(`{
		"currentPassword": "` + initialPass + `",
		"newPassword": "` + newPass + `"
	}`))
	validReq.Header.Set("Authorization", "Bearer "+token)
	w3 := httptest.NewRecorder()
	ChangePasswordHandler(w3, validReq)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid password change, got %d: %s", w3.Code, w3.Body.String())
	}

	// Verify MustChangePassword is now false in memory store
	updatedUser, _ := memStore.GetUserByID(context.Background(), user.ID)
	if updatedUser.MustChangePassword {
		t.Errorf("expected MustChangePassword=false after successful password change")
	}

	// Verify new password works and old password fails
	if storage.VerifyPassword(initialPass, updatedUser.Salt, updatedUser.PasswordHash) {
		t.Errorf("old password should no longer be valid")
	}
	if !storage.VerifyPassword(newPass, updatedUser.Salt, updatedUser.PasswordHash) {
		t.Errorf("new password should be successfully verified")
	}
}

func TestProductionConfig_RequiresAdminPasswordAndRejectsInsecureCredentials(t *testing.T) {
	// Missing admin password in production
	cfgMissingPass := &config.Config{
		Environment:         "production",
		DatabaseURL:         "postgres://aegis_prod:ValidProdPass123!@localhost:5432/aegis_prod",
		CORSAllowedOrigins:  []string{"https://aegis.example.com"},
		BootstrapAdminPass:  "",
	}
	if err := cfgMissingPass.Validate(); err == nil {
		t.Errorf("expected validation failure for missing bootstrap admin password in production")
	}

	// Insecure database password in production
	cfgInsecureDB := &config.Config{
		Environment:         "production",
		DatabaseURL:         "postgres://aegis:insecure_default_pass@localhost:5432/aegis_prod",
		CORSAllowedOrigins:  []string{"https://aegis.example.com"},
		BootstrapAdminPass:  "ValidAdminPass!2026",
	}
	if err := cfgInsecureDB.Validate(); err == nil {
		t.Errorf("expected validation failure for insecure database password in production")
	}
}
