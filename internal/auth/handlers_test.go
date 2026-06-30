package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- TestHandleLogin ---

func TestHandleLogin_LocalSuccess(t *testing.T) {
	a := testAuth(t)

	body := `{"username":"admin","password":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.handleLogin(rec, req)

	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, rec.Body.String())
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check user object
	user, ok := result["user"].(map[string]any)
	if !ok {
		t.Fatal("response missing 'user' object")
	}
	if user["username"] != "admin" {
		t.Errorf("user.username = %v, want 'admin'", user["username"])
	}
	if user["role"] != "admin" {
		t.Errorf("user.role = %v, want 'admin'", user["role"])
	}
	// password_hash must never be exposed
	if _, exists := user["password_hash"]; exists {
		t.Error("user object should not contain 'password_hash'")
	}
	if _, exists := user["PasswordHash"]; exists {
		t.Error("user object should not contain 'PasswordHash'")
	}

	// must_change flag (bootstrap admin has MustChangePwd=true)
	if mc, _ := result["must_change"].(bool); !mc {
		t.Error("expected must_change=true for bootstrap admin")
	}

	// Auth cookie must be set
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "k8ops_token" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("k8ops_token cookie not set in login response")
	}
	if cookie.Value == "" {
		t.Error("cookie value should not be empty")
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
}

func TestHandleLogin_InvalidBody(t *testing.T) {
	a := testAuth(t)

	tests := []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"missing password", `{"username":"admin"}`},
		{"missing username", `{"password":"admin"}`},
		{"both empty", `{"username":"","password":""}`},
		{"malformed json", `{not-json}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			a.handleLogin(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			var result map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &result)
			if errMsg, _ := result["error"].(string); errMsg == "" {
				t.Error("response should contain 'error' message")
			}
		})
	}
}

func TestHandleLogin_BadCredentials(t *testing.T) {
	a := testAuth(t)

	body := `{"username":"admin","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.handleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if errMsg, _ := result["error"].(string); !strings.Contains(strings.ToLower(errMsg), "invalid") {
		t.Errorf("error message = %q, want it to contain 'invalid'", errMsg)
	}
}

// --- TestHandleStatus ---

func TestHandleStatus(t *testing.T) {
	a := testAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()

	a.handleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.NewDecoder(rec.Result().Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if enabled, _ := result["auth_enabled"].(bool); !enabled {
		t.Error("expected auth_enabled=true")
	}

	// Bootstrap admin should give at least 1 user
	count, _ := result["user_count"].(float64)
	if count < 1 {
		t.Errorf("user_count = %v, want >= 1", count)
	}

	// Should also have ldap_enabled and oidc_enabled fields
	if _, exists := result["ldap_enabled"]; !exists {
		t.Error("response should contain 'ldap_enabled'")
	}
	if _, exists := result["oidc_enabled"]; !exists {
		t.Error("response should contain 'oidc_enabled'")
	}
}

// --- TestSanitizeUser ---

func TestSanitizeUser(t *testing.T) {
	u := &User{
		ID:              42,
		Username:        "testuser",
		Email:           "test@example.com",
		DisplayName:     "Test User",
		PasswordHash:    "$2a$12$someverylongbcrypt hash value here",
		Role:            "viewer",
		Provider:        "local",
		MustChangePwd:   false,
		AllowedNamespaces: "default,kube-system",
	}

	sanitized := sanitizeUser(u)

	// Must NOT contain password_hash or PasswordHash
	if _, exists := sanitized["password_hash"]; exists {
		t.Error("sanitizeUser output should not contain 'password_hash'")
	}
	if _, exists := sanitized["PasswordHash"]; exists {
		t.Error("sanitizeUser output should not contain 'PasswordHash'")
	}
	if _, exists := sanitized["passwordHash"]; exists {
		t.Error("sanitizeUser output should not contain 'passwordHash'")
	}

	// Must contain expected fields
	expected := map[string]any{
		"id":                 uint(42),
		"username":           "testuser",
		"email":              "test@example.com",
		"display_name":       "Test User",
		"role":               "viewer",
		"provider":           "local",
		"must_change_pwd":    false,
		"allowed_namespaces": "default,kube-system",
	}
	for key, want := range expected {
		if got, exists := sanitized[key]; !exists {
			t.Errorf("sanitizeUser output missing key %q", key)
		} else if got != want {
			t.Errorf("sanitizeUser[%q] = %v, want %v", key, got, want)
		}
	}
}

// --- TestStrconvParseUint ---

func TestStrconvParseUint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint
		wantErr bool
	}{
		{"normal", "42", 42, false},
		{"zero", "0", 0, false},
		{"single digit", "7", 7, false},
		{"large number", "999999", 999999, false},
		{"empty string", "", 0, false},
		{"non-numeric", "abc", 0, true},
		{"mixed alpha-numeric", "12a", 0, true},
		{"special chars", "4-2", 0, true},
		{"spaces", " 42", 0, true},
		{"negative", "-1", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := strconvParseUint(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("strconvParseUint(%q) expected error, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("strconvParseUint(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("strconvParseUint(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper to create an Authenticator for handler tests
// ---------------------------------------------------------------------------

func newHandlerAuthenticator(t *testing.T) *Authenticator {
	t.Helper()
	a, err := New(&Config{
		JWTSecret:   "test-secret-key-for-jwt",
		JWTExpiry:   0,
		DBDriver:    "sqlite",
		DBDSN:       ":memory:",
		DefaultRole: "viewer",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// ---------------------------------------------------------------------------
// handleLogin table-driven tests
// ---------------------------------------------------------------------------

func TestHandleLogin_TableDriven(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	passwd := "test-password-123"
	hash, err := HashPassword(passwd)
	require.NoError(t, err)
	require.NoError(t, auth.store.CreateUser(&User{
		Username:     "loginuser",
		PasswordHash: hash,
		Role:         "viewer",
		Provider:     "local",
	}))

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantError  string
	}{
		{"valid login", `{"username":"loginuser","password":"test-password-123"}`, http.StatusOK, ""},
		{"invalid JSON", `{bad json`, http.StatusBadRequest, "invalid request body"},
		{"empty username", `{"username":"","password":"x"}`, http.StatusBadRequest, "username and password required"},
		{"empty password", `{"username":"x","password":""}`, http.StatusBadRequest, "username and password required"},
		{"missing username field", `{"password":"x"}`, http.StatusBadRequest, "username and password required"},
		{"missing both", `{}`, http.StatusBadRequest, "username and password required"},
		{"wrong password", `{"username":"loginuser","password":"wrong"}`, http.StatusUnauthorized, "invalid username or password"},
		{"nonexistent user", `{"username":"ghost","password":"x"}`, http.StatusUnauthorized, "invalid username or password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			auth.handleLogin(w, r)

			resp := w.Result()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			if tt.wantError != "" {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body["error"], tt.wantError)
			}
		})
	}
}

func TestHandleLogin_SetsCookie(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	hash, err := HashPassword("pass123")
	require.NoError(t, err)
	require.NoError(t, auth.store.CreateUser(&User{
		Username:     "cookieuser",
		PasswordHash: hash,
		Role:         "viewer",
		Provider:     "local",
	}))

	body := `{"username":"cookieuser","password":"pass123"}`
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	auth.handleLogin(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "k8ops_token" {
			found = true
			assert.NotEmpty(t, c.Value)
			assert.True(t, c.HttpOnly)
		}
	}
	assert.True(t, found)
}

func TestHandleLogin_MustChangePwd(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	hash, err := HashPassword("temp")
	require.NoError(t, err)
	require.NoError(t, auth.store.CreateUser(&User{
		Username:      "mustchange",
		PasswordHash:  hash,
		Role:          "viewer",
		Provider:      "local",
		MustChangePwd: true,
	}))

	body := `{"username":"mustchange","password":"temp"}`
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
	w := httptest.NewRecorder()

	auth.handleLogin(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body2 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body2))
	assert.Equal(t, true, body2["must_change"])
}

// ---------------------------------------------------------------------------
// handleLogout tests
// ---------------------------------------------------------------------------

func TestHandleLogout(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	w := httptest.NewRecorder()

	auth.handleLogout(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "k8ops_token" {
			found = true
			assert.Equal(t, -1, c.MaxAge)
		}
	}
	assert.True(t, found)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "/login.html", body["redirect_url"])
}

// ---------------------------------------------------------------------------
// handleMe tests
// ---------------------------------------------------------------------------

func TestHandleMe_Authenticated(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user := &User{Username: "meuser", Role: "admin", Provider: "local"}
	require.NoError(t, auth.store.CreateUser(user))

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	r = r.WithContext(SetUserInContext(r.Context(), user))
	w := httptest.NewRecorder()

	auth.handleMe(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	userMap, ok := body["user"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "meuser", userMap["username"])
	assert.Equal(t, "admin", userMap["role"])
}

func TestHandleMe_Unauthenticated(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("GET", "/api/auth/me", nil)
	w := httptest.NewRecorder()

	auth.handleMe(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "not authenticated", body["error"])
}

// ---------------------------------------------------------------------------
// handleChangePassword table-driven tests
// ---------------------------------------------------------------------------

func TestHandleChangePassword_TableDriven(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	// Fresh user for each subtest using unique usernames
	tests := []struct {
		name       string
		username   string
		createUser bool
		oldPass    string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			"valid change",
			"changepwd-ok",
			true,
			"old-pass-123",
			`{"old_password":"old-pass-123","new_password":"new-pass-456"}`,
			http.StatusOK,
			"",
		},
		{
			"nil user (unauthenticated)",
			"",
			false,
			"",
			`{"old_password":"x","new_password":"y"}`,
			http.StatusUnauthorized,
			"not authenticated",
		},
		{
			"invalid JSON",
			"changepwd-badjson",
			true,
			"pass123",
			`{bad json`,
			http.StatusBadRequest,
			"invalid request",
		},
		{
			"wrong old password",
			"changepwd-wrongold",
			true,
			"correct-old",
			`{"old_password":"wrong-old","new_password":"new-pass"}`,
			http.StatusBadRequest,
			"invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var user *User
			if tt.createUser {
				hash, err := HashPassword(tt.oldPass)
				require.NoError(t, err)
				user = &User{
					Username:      tt.username,
					PasswordHash:  hash,
					Role:          "viewer",
					Provider:      "local",
					MustChangePwd: false,
				}
				require.NoError(t, auth.store.CreateUser(user))
			}

			r := httptest.NewRequest("POST", "/api/auth/change-password", strings.NewReader(tt.body))
			if user != nil {
				r = r.WithContext(SetUserInContext(r.Context(), user))
			}
			w := httptest.NewRecorder()

			auth.handleChangePassword(w, r)

			resp := w.Result()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			if tt.wantError != "" {
				var body map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
				assert.Contains(t, body["error"], tt.wantError)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// handleStatus tests
// ---------------------------------------------------------------------------

func TestHandleStatus_BasicResponse(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()

	auth.handleStatus(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["auth_enabled"])
	assert.Equal(t, false, body["ldap_enabled"])
	assert.Equal(t, false, body["oidc_enabled"])
	assert.NotNil(t, body["user_count"])
}

func TestHandleStatus_WithProviders(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name:    "test-ldap",
		Type:    ProviderTypeLDAP,
		Enabled: true,
	}))
	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name:        "test-oidc",
		Type:        ProviderTypeOIDC,
		Enabled:     true,
		DisplayName: "Test OIDC",
		Icon:        "github",
	}))

	r := httptest.NewRequest("GET", "/api/auth/status", nil)
	w := httptest.NewRecorder()

	auth.handleStatus(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["ldap_enabled"])
	assert.Equal(t, true, body["oidc_enabled"])

	providers, ok := body["oidc_providers"].([]any)
	require.True(t, ok)
	require.Len(t, providers, 1)
	p := providers[0].(map[string]any)
	assert.Equal(t, "test-oidc", p["name"])
	assert.Equal(t, "Test OIDC", p["display_name"])
	assert.Equal(t, "github", p["icon"])
}

// ---------------------------------------------------------------------------
// handleMultiOIDC route tests
// ---------------------------------------------------------------------------

func TestHandleMultiOIDC_TableDriven(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	// Create a disabled OIDC provider
	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name: "disabled-oidc",
		Type: ProviderTypeOIDC,
	}))
	require.NoError(t, auth.store.db.Model(&AuthProvider{}).Where("name = ?", "disabled-oidc").Update("enabled", false).Error)

	// Create a wrong-type provider
	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name: "wrong-type",
		Type: ProviderTypeLDAP,
	}))

	// Create a correct OIDC provider
	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name: "good-oidc",
		Type: ProviderTypeOIDC,
	}))

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"invalid route prefix", "/api/auth/oidc/", http.StatusNotFound},
		{"nonexistent provider", "/api/auth/oidc/nonexistent/login", http.StatusNotFound},
		{"disabled provider", "/api/auth/oidc/disabled-oidc/login", http.StatusBadRequest},
		{"wrong type provider", "/api/auth/oidc/wrong-type/login", http.StatusBadRequest},
		{"unknown action", "/api/auth/oidc/good-oidc/register", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			auth.handleMultiOIDC(w, r)

			assert.Equal(t, tt.wantStatus, w.Result().StatusCode)
		})
	}
}

func TestHandleMultiOIDCCallback_MissingCodeOrState(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	require.NoError(t, auth.store.CreateAuthProvider(&AuthProvider{
		Name: "callback-test",
		Type: ProviderTypeOIDC,
	}))

	tests := []struct {
		name  string
		query string
	}{
		{"no code", "state=abc"},
		{"no state", "code=xyz"},
		{"both empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/auth/oidc/callback-test/callback?" + tt.query
			r := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			auth.handleMultiOIDC(w, r)

			resp := w.Result()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Contains(t, body["error"], "missing code or state")
		})
	}
}

// ---------------------------------------------------------------------------
// handleUsers (admin CRUD) tests
// ---------------------------------------------------------------------------

func TestHandleUsers_GetList(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("GET", "/api/admin/users", nil)
	w := httptest.NewRecorder()

	auth.handleUsers(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	users, ok := body["users"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, users)
}

func TestHandleUsers_CreateUser(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	body := `{"username":"newuser","password":"pass123","email":"new@test.com","display_name":"New User","role":"viewer"}`
	r := httptest.NewRequest("POST", "/api/admin/users", strings.NewReader(body))
	w := httptest.NewRecorder()

	auth.handleUsers(w, r)

	resp := w.Result()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var respBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&respBody))
	userMap, ok := respBody["user"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "newuser", userMap["username"])
	assert.Equal(t, "viewer", userMap["role"])
}

func TestHandleUsers_CreateUser_ValidationErrors(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing username", `{"password":"x"}`, http.StatusBadRequest},
		{"missing password", `{"username":"x"}`, http.StatusBadRequest},
		{"invalid JSON", `{bad`, http.StatusBadRequest},
		{"duplicate username", `{"username":"admin","password":"x"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/admin/users", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			auth.handleUsers(w, r)

			assert.Equal(t, tt.wantStatus, w.Result().StatusCode)
		})
	}
}

func TestHandleUsers_MethodNotAllowed(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("PUT", "/api/admin/users", nil)
	w := httptest.NewRecorder()

	auth.handleUsers(w, r)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

// ---------------------------------------------------------------------------
// handleUserByID tests
// ---------------------------------------------------------------------------

func TestHandleUserByID_Delete(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("delete-me", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	path := fmt.Sprintf("/api/admin/users/%d", user.ID)
	r := httptest.NewRequest("DELETE", path, nil)
	w := httptest.NewRecorder()

	auth.handleUserByID(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	_, err = auth.store.GetUserByID(user.ID)
	assert.Error(t, err)
}

func TestHandleUserByID_PatchUpdate(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("patch-me", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	body := `{"role":"operator","display_name":"Patched"}`
	path := fmt.Sprintf("/api/admin/users/%d", user.ID)
	r := httptest.NewRequest("PATCH", path, strings.NewReader(body))
	w := httptest.NewRecorder()

	auth.handleUserByID(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	updated, err := auth.store.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "operator", updated.Role)
	assert.Equal(t, "Patched", updated.DisplayName)
}

func TestHandleUserByID_ResetPassword(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("reset-me", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	body := `{"password":"newpass456"}`
	path := fmt.Sprintf("/api/admin/users/%d/reset-password", user.ID)
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	w := httptest.NewRecorder()

	auth.handleUserByID(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	updated, err := auth.store.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.True(t, updated.MustChangePwd)
}

func TestHandleUserByID_InvalidID(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("DELETE", "/api/admin/users/abc", nil)
	w := httptest.NewRecorder()

	auth.handleUserByID(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
}

func TestHandleUserByID_MethodNotAllowed(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	r := httptest.NewRequest("PUT", "/api/admin/users/1", nil)
	w := httptest.NewRecorder()

	auth.handleUserByID(w, r)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Result().StatusCode)
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSetUserInContext(t *testing.T) {
	user := &User{Username: "ctx-user", Role: "admin"}

	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(SetUserInContext(r.Context(), user))

	got := UserFromRequest(r)
	require.NotNil(t, got)
	assert.Equal(t, "ctx-user", got.Username)
}

func TestSetUserInContext_NilUser(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(SetUserInContext(r.Context(), nil))

	got := UserFromRequest(r)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// uniqueUsername tests
// ---------------------------------------------------------------------------

func TestUniqueUsername(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	t.Run("collision with existing", func(t *testing.T) {
		result := auth.uniqueUsername("admin", "ldap")
		assert.NotEqual(t, "admin", result)
	})

	t.Run("no collision", func(t *testing.T) {
		result := auth.uniqueUsername("newname", "ldap")
		assert.Equal(t, "newname", result)
	})
}

// ---------------------------------------------------------------------------
// AdminCreateUser / AdminUpdateUser / AdminResetPassword tests
// ---------------------------------------------------------------------------

func TestAdminCreateUser_DefaultRole(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("norole", "pass123", "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, "viewer", user.Role)
	assert.Equal(t, "local", user.Provider)
	assert.True(t, user.MustChangePwd)
}

func TestAdminCreateUser_Duplicate(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	_, err := auth.AdminCreateUser("dup-user", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	_, err = auth.AdminCreateUser("dup-user", "pass456", "", "", "viewer", "")
	assert.Error(t, err)
}

func TestAdminUpdateUser(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("update-admin", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	err = auth.AdminUpdateUser(user.ID, map[string]any{
		"role":         "operator",
		"display_name": "Updated via Admin",
	})
	assert.NoError(t, err)

	updated, err := auth.store.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "operator", updated.Role)
	assert.Equal(t, "Updated via Admin", updated.DisplayName)
}

// ---------------------------------------------------------------------------
// ChangePassword tests (only those NOT already in auth_test.go)
// ---------------------------------------------------------------------------

func TestChangePassword_NotFound_HandlerTest(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	err := auth.ChangePassword(99999, "x", "y")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestChangePassword_ClearsMustChange(t *testing.T) {
	auth := newHandlerAuthenticator(t)

	user, err := auth.AdminCreateUser("must-change-handler", "temp123", "", "", "viewer", "")
	require.NoError(t, err)
	require.True(t, user.MustChangePwd)

	require.NoError(t, auth.ChangePassword(user.ID, "temp123", "newpass456"))

	updated, err := auth.store.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.False(t, updated.MustChangePwd)
}

// ---------------------------------------------------------------------------
// defaultRole tests
// ---------------------------------------------------------------------------

func TestDefaultRole_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		role string
		want string
	}{
		{"configured", "operator", "operator"},
		{"empty defaults to viewer", "", "viewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(&Config{
				JWTSecret:   "x",
				DBDriver:    "sqlite",
				DBDSN:       ":memory:",
				DefaultRole: tt.role,
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = a.Close() })

			assert.Equal(t, tt.want, a.defaultRole())
		})
	}
}
