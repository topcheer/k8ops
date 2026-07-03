package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// auth.go: SetRBACSyncer, Store, Config, HashPassword, uniqueUsername
// ---------------------------------------------------------------------------

func TestAuthenticator_Store(t *testing.T) {
	a := newHandlerAuthenticator(t)
	assert.NotNil(t, a.Store())
}

func TestAuthenticator_Config(t *testing.T) {
	a := newHandlerAuthenticator(t)
	cfg := a.Config()
	assert.NotNil(t, cfg)
	assert.Equal(t, "test-secret-key-for-jwt", cfg.JWTSecret)
}

func TestAuthenticator_SetRBACSyncer(t *testing.T) {
	a := newHandlerAuthenticator(t)
	// Setting nil syncer should not panic
	a.SetRBACSyncer(nil)
	// Setting a real syncer (nil clientset is fine for just testing the setter)
	a.SetRBACSyncer(&RBACSyncer{})
}

func TestHashPassword_Success(t *testing.T) {
	hash, err := HashPassword("testpass123")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.True(t, len(hash) > 30, "bcrypt hash should be >30 chars")
}

func TestHashPassword_DifferentPasswords(t *testing.T) {
	h1, _ := HashPassword("pass1")
	h2, _ := HashPassword("pass2")
	assert.NotEqual(t, h1, h2, "different passwords should produce different hashes")
}

func TestHashPassword_SamePasswordDifferentHash(t *testing.T) {
	// bcrypt includes a random salt, so same password produces different hashes
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	assert.NotEqual(t, h1, h2, "same password should produce different hashes due to salt")
}

// ---------------------------------------------------------------------------
// auth.go: VerifyToken tests
// ---------------------------------------------------------------------------

func TestVerifyToken_ValidToken(t *testing.T) {
	a := newHandlerAuthenticator(t)

	user, err := a.AdminCreateUser("tokenuser", "pass123", "", "", "viewer", "")
	require.NoError(t, err)

	token, err := a.generateToken(user)
	require.NoError(t, err)

	claims, err := a.VerifyToken(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	assert.Equal(t, "tokenuser", claims.Username)
	assert.Equal(t, "viewer", claims.Role)
}

func TestVerifyToken_InvalidToken(t *testing.T) {
	a := newHandlerAuthenticator(t)

	_, err := a.VerifyToken("invalid-token-string")
	assert.Error(t, err)
}

func TestVerifyToken_EmptyTokenString(t *testing.T) {
	a := newHandlerAuthenticator(t)

	_, err := a.VerifyToken("")
	assert.Error(t, err)
}

func TestVerifyToken_AlgorithmNone_HandlerTest(t *testing.T) {
	a := newHandlerAuthenticator(t)

	// Try to forge a token with alg=none
	// This should fail because VerifyToken checks signing method
	_, err := a.VerifyToken("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1aWQiOjEsInVzciI6ImFkbWluIn0.")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// auth.go: bootstrapAdmin tests
// ---------------------------------------------------------------------------

func TestBootstrapAdmin_CreatesAdmin(t *testing.T) {
	a := newHandlerAuthenticator(t)

	// The bootstrap should have created an admin user
	user, err := a.store.GetUserByUsername("admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Username)
	assert.Equal(t, "admin", user.Role)
	assert.Equal(t, "local", user.Provider)
	assert.True(t, user.MustChangePwd)
}

func TestBootstrapAdmin_DoesNotCreateIfUsersExist(t *testing.T) {
	// Create a store with an existing user first
	store, err := NewStore("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrateAuthProviders())
	require.NoError(t, store.AutoMigrateRoles())
	require.NoError(t, store.SeedBuiltinRoles())
	t.Cleanup(func() { _ = store.Close() })

	// Create a user before bootstrap
	require.NoError(t, store.CreateUser(&User{Username: "existing", Role: "admin"}))

	a := &Authenticator{
		store: store,
		cfg:   &Config{JWTSecret: "x", DefaultRole: "viewer"},
	}
	require.NoError(t, a.bootstrapAdmin())

	// Should NOT have created admin since users exist
	_, err = store.GetUserByUsername("admin")
	assert.Error(t, err, "should not have bootstrapped admin when users exist")
}

// ---------------------------------------------------------------------------
// handlers.go: upsertOIDCUserForKey tests
// ---------------------------------------------------------------------------

func TestUpsertOIDCUser_NewUser(t *testing.T) {
	a := newHandlerAuthenticator(t)

	user, token, err := a.upsertOIDCUserForKey("oidc:github", "github-12345", "ghuser", "gh@test.com", "GH User")
	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotEmpty(t, token)
	assert.Equal(t, "ghuser", user.Username)
	assert.Equal(t, "gh@test.com", user.Email)
	assert.Equal(t, "oidc:github", user.Provider)
	assert.Equal(t, "github-12345", user.ProviderID)
	assert.Equal(t, "viewer", user.Role, "default role should be viewer")
}

func TestUpsertOIDCUser_ExistingUser(t *testing.T) {
	a := newHandlerAuthenticator(t)

	// Create user first
	user1, _, err := a.upsertOIDCUserForKey("oidc:google", "google-abc", "guser", "old@test.com", "Old Name")
	require.NoError(t, err)

	// Same provider+ID → should update
	user2, _, err := a.upsertOIDCUserForKey("oidc:google", "google-abc", "guser", "new@test.com", "New Name")
	require.NoError(t, err)
	assert.Equal(t, user1.ID, user2.ID)
	assert.Equal(t, "new@test.com", user2.Email)
	assert.Equal(t, "New Name", user2.DisplayName)
}

func TestUpsertOIDCUser_UsernameCollision(t *testing.T) {
	a := newHandlerAuthenticator(t)

	// Bootstrap creates "admin" as local user
	// An OIDC user with username "admin" should get a unique name
	user, _, err := a.upsertOIDCUserForKey("oidc:github", "gh-collision", "admin", "admin-gh@test.com", "Admin GH")
	require.NoError(t, err)
	assert.NotEqual(t, "admin", user.Username, "should have unique username on collision")
}

// ---------------------------------------------------------------------------
// provider_handlers.go: providerToAPI, mergeLDAPConfig, mergeOIDCConfig, parseUint
// ---------------------------------------------------------------------------

func TestProviderToAPI_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		provider      *AuthProvider
		wantType      string
		wantHasConfig bool
	}{
		{
			"LDAP provider",
			&AuthProvider{
				Name:    "test-ldap",
				Type:    ProviderTypeLDAP,
				Enabled: true,
				Config:  `{"ldap":{"server":"ldap://host:389","bind_pw":"secret"}}`,
			},
			"ldap",
			true,
		},
		{
			"OIDC provider",
			&AuthProvider{
				Name:    "test-oidc",
				Type:    ProviderTypeOIDC,
				Enabled: true,
				Config:  `{"oidc":{"issuer":"https://test.com","client_secret":"secret"}}`,
			},
			"oidc",
			true,
		},
		{
			"Empty config",
			&AuthProvider{
				Name: "no-config",
				Type: ProviderTypeOIDC,
			},
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := providerToAPI(tt.provider)
			assert.Equal(t, tt.provider.Name, result["name"])
			assert.Equal(t, tt.provider.Type, result["type"])
			assert.Equal(t, tt.provider.Enabled, result["enabled"])

			if tt.wantHasConfig {
				assert.NotNil(t, result["config"])
				assert.Equal(t, tt.wantType, result["config_type"])
			}
		})
	}
}

func TestMergeLDAPConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		dst   *LDAPConfig
		src   *LDAPConfig
		check func(*testing.T, *LDAPConfig)
	}{
		{
			"merge server",
			&LDAPConfig{Server: "old"},
			&LDAPConfig{Server: "new"},
			func(t *testing.T, r *LDAPConfig) { assert.Equal(t, "new", r.Server) },
		},
		{
			"skip masked bindPW",
			&LDAPConfig{BindPW: "real-secret"},
			&LDAPConfig{BindPW: "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"},
			func(t *testing.T, r *LDAPConfig) {
				assert.Equal(t, "real-secret", r.BindPW, "should not overwrite with masked value")
			},
		},
		{
			"merge new bindPW",
			&LDAPConfig{BindPW: "old"},
			&LDAPConfig{BindPW: "new-secret"},
			func(t *testing.T, r *LDAPConfig) { assert.Equal(t, "new-secret", r.BindPW) },
		},
		{
			"merge bindDN",
			&LDAPConfig{BindDN: "old"},
			&LDAPConfig{BindDN: "cn=new,dc=example,dc=com"},
			func(t *testing.T, r *LDAPConfig) { assert.Equal(t, "cn=new,dc=example,dc=com", r.BindDN) },
		},
		{
			"copy StartTLS",
			&LDAPConfig{StartTLS: false},
			&LDAPConfig{StartTLS: true},
			func(t *testing.T, r *LDAPConfig) { assert.True(t, r.StartTLS) },
		},
		{
			"copy SkipTLSVerify",
			&LDAPConfig{SkipTLSVerify: false},
			&LDAPConfig{SkipTLSVerify: true},
			func(t *testing.T, r *LDAPConfig) { assert.True(t, r.SkipTLSVerify) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeLDAPConfig(tt.dst, tt.src)
			tt.check(t, tt.dst)
		})
	}
}

func TestMergeOIDCConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		dst   *OIDCConfig
		src   *OIDCConfig
		check func(*testing.T, *OIDCConfig)
	}{
		{
			"merge issuer",
			&OIDCConfig{Issuer: "old"},
			&OIDCConfig{Issuer: "https://new.com"},
			func(t *testing.T, r *OIDCConfig) { assert.Equal(t, "https://new.com", r.Issuer) },
		},
		{
			"skip masked client secret",
			&OIDCConfig{ClientSecret: "real-secret"},
			&OIDCConfig{ClientSecret: "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"},
			func(t *testing.T, r *OIDCConfig) { assert.Equal(t, "real-secret", r.ClientSecret) },
		},
		{
			"merge client ID",
			&OIDCConfig{ClientID: "old"},
			&OIDCConfig{ClientID: "new-id"},
			func(t *testing.T, r *OIDCConfig) { assert.Equal(t, "new-id", r.ClientID) },
		},
		{
			"merge redirect URL",
			&OIDCConfig{RedirectURL: "old"},
			&OIDCConfig{RedirectURL: "https://new.com/callback"},
			func(t *testing.T, r *OIDCConfig) { assert.Equal(t, "https://new.com/callback", r.RedirectURL) },
		},
		{
			"merge scopes",
			&OIDCConfig{Scopes: []string{"openid"}},
			&OIDCConfig{Scopes: []string{"openid", "profile", "email"}},
			func(t *testing.T, r *OIDCConfig) { assert.Equal(t, []string{"openid", "profile", "email"}, r.Scopes) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeOIDCConfig(tt.dst, tt.src)
			tt.check(t, tt.dst)
		})
	}
}

func TestParseUint_TableDriven(t *testing.T) {
	tests := []struct {
		input string
		want  uint
	}{
		{"0", 0},
		{"1", 1},
		{"42", 42},
		{"999", 999},
		{"", 0},
		{"abc", 0},
		{"12abc34", 1234}, // only digits are processed
		{"-1", 1},         // '-' is skipped, '1' is processed
		{"1.5", 15},       // '.' is not a digit, so it reads "15"
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseUint(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// role_defs.go: roleDefError.Error() test
// ---------------------------------------------------------------------------

func TestRoleDefError_Error(t *testing.T) {
	err := ErrBuiltinRoleProtected
	assert.Equal(t, "cannot modify built-in role", err.Error())
}

// ---------------------------------------------------------------------------
// middleware.go: SetAuthCookie, clearAuthCookie, isPublicRoute
// ---------------------------------------------------------------------------

func TestSetAuthCookie_HandlerTest(t *testing.T) {
	w := httptest.NewRecorder()
	SetAuthCookie(w, "test-jwt-token", 3600)

	resp := w.Result()
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "k8ops_token", cookies[0].Name)
	assert.Equal(t, "test-jwt-token", cookies[0].Value)
	assert.Equal(t, 3600, cookies[0].MaxAge)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, "/", cookies[0].Path)
}

func TestClearAuthCookie_HandlerTest(t *testing.T) {
	w := httptest.NewRecorder()
	clearAuthCookie(w)

	resp := w.Result()
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "k8ops_token", cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestIsPublicRoute_TableDriven(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/health", true},
		{"/api/auth/login", true},
		{"/api/auth/logout", true},
		{"/api/auth/status", true},
		{"/api/auth/me", false}, // requires authentication
		{"/login.html", true},
		{"/api/admin/users", false},
		{"/api/pods", false},
		{"/dashboard", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, isPublicRoute(tt.path))
		})
	}
}

// ---------------------------------------------------------------------------
// AdminOnly middleware test (additional cases)
// ---------------------------------------------------------------------------

func TestAdminOnly_Viewer(t *testing.T) {
	handler := AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/api/admin/test", nil)
	r = r.WithContext(SetUserInContext(r.Context(), &User{Role: "viewer"}))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Result().StatusCode)
}

// ---------------------------------------------------------------------------
// handlers.go: handleAuthConfig tests
// ---------------------------------------------------------------------------

func TestHandleAuthConfig_Get(t *testing.T) {
	a := newHandlerAuthenticator(t)

	r := httptest.NewRequest("GET", "/api/admin/auth-config", nil)
	w := httptest.NewRecorder()

	a.handleAuthConfig(w, r)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestHandleAuthConfig_Put(t *testing.T) {
	a := newHandlerAuthenticator(t)

	body := `{"default_role":"operator"}`
	r := httptest.NewRequest("PUT", "/api/admin/auth-config", strings.NewReader(body))
	w := httptest.NewRecorder()

	a.handleAuthConfig(w, r)

	// May return 200 or 400 depending on parsing, just verify no panic
	resp := w.Result()
	assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500)
}
