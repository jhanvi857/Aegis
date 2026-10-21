package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuth_SignupAndLoginFlow(t *testing.T) {
	// 1. Signup a new operator account
	signupPayload, _ := json.Marshal(map[string]string{
		"name":     "Jane Doe",
		"email":    "jane.doe@enterprise.io",
		"password": "SecurePassword#2026",
		"role":     "SecOps Lead",
	})

	req := httptest.NewRequest("POST", "/api/auth/signup", bytes.NewBuffer(signupPayload))
	rr := httptest.NewRecorder()
	SignupHandler(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created from SignupHandler, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var signupResp AuthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &signupResp); err != nil {
		t.Fatalf("failed to parse signup response: %v", err)
	}

	if signupResp.Token == "" {
		t.Errorf("expected secure token, got empty")
	}
	if signupResp.User.Email != "jane.doe@enterprise.io" {
		t.Errorf("expected email 'jane.doe@enterprise.io', got '%s'", signupResp.User.Email)
	}

	// Verify that password is never exposed in JSON output
	if signupResp.User.PasswordHash != "" {
		t.Errorf("password hash must not be exposed in user JSON response")
	}

	// 2. Duplicate signup should be rejected with 409 Conflict
	dupReq := httptest.NewRequest("POST", "/api/auth/signup", bytes.NewBuffer(signupPayload))
	dupRr := httptest.NewRecorder()
	SignupHandler(dupRr, dupReq)
	if dupRr.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict for duplicate email, got %d", dupRr.Code)
	}

	// 3. Login with correct credentials
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    "jane.doe@enterprise.io",
		"password": "SecurePassword#2026",
	})
	loginReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(loginPayload))
	loginRr := httptest.NewRecorder()
	LoginHandler(loginRr, loginReq)

	if loginRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from LoginHandler, got %d. Body: %s", loginRr.Code, loginRr.Body.String())
	}

	var loginResp AuthResponse
	_ = json.Unmarshal(loginRr.Body.Bytes(), &loginResp)
	if loginResp.Token == "" {
		t.Errorf("expected token on login, got empty")
	}

	// 4. Login with incorrect password should be rejected with 401 Unauthorized
	badLoginPayload, _ := json.Marshal(map[string]string{
		"email":    "jane.doe@enterprise.io",
		"password": "WrongPassword",
	})
	badReq := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(badLoginPayload))
	badRr := httptest.NewRecorder()
	LoginHandler(badRr, badReq)
	if badRr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for incorrect password, got %d", badRr.Code)
	}

	// 5. Test GetMeHandler with valid bearer token
	meReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginResp.Token)
	meRr := httptest.NewRecorder()
	GetMeHandler(meRr, meReq)

	if meRr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from GetMeHandler, got %d. Body: %s", meRr.Code, meRr.Body.String())
	}

	// 6. Test GetMeHandler with invalid bearer token
	badMeReq := httptest.NewRequest("GET", "/api/auth/me", nil)
	badMeReq.Header.Set("Authorization", "Bearer invalid_forged_token")
	badMeRr := httptest.NewRecorder()
	GetMeHandler(badMeRr, badMeReq)
	if badMeRr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for forged token, got %d", badMeRr.Code)
	}
}
