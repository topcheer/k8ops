package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testAuthT1 creates an Authenticator backed by in-memory SQLite.
// The helper in middleware_test.go (testAuth) is shared across files.
// This variant returns the authenticator and the bootstrap admin user.
func testAuthT1(t *testing.T) (*Authenticator, *User) {
	t.Helper()
	a := testAuth(t)
	admin, err := a.store.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("failed to get bootstrap admin: %v", err)
	}
	return a, admin
}

// --- TestNew_BootstrapAdmin ---

func TestNew_BootstrapAdmin(t *testing.T) {
	a := testAuth(t)

	// Bootstrap admin should exist
	admin, err := a.store.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("expected bootstrap admin user: %v", err)
	}

	if admin.Role != "admin" {
		t.Errorf("admin.Role = %q, want 'admin'", admin.Role)
	}
	if admin.Provider != "local" {
		t.Errorf("admin.Provider = %q, want 'local'", admin.Provider)
	}
	if !admin.MustChangePwd {
		t.Error("bootstrap admin MustChangePwd should be true")
	}
	if admin.DisplayName != "Administrator" {
		t.Errorf("admin.DisplayName = %q, want 'Administrator'", admin.DisplayName)
	}

	// Only 1 user (the bootstrap admin)
	count, err := a.store.CountUsers()
	if err != nil {
		t.Fatalf("CountUsers failed: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}
}

func TestNew_DoesNotDuplicateAdmin(t *testing.T) {
	// Create first instance — bootstraps admin
	a1 := testAuth(t)

	// Get the store DB so we can reuse the same in-memory database
	sqlDB, _ := a1.store.db.DB()
	// Close first authenticator's wrapper but keep DB alive via pool

	// Create second authenticator using the same DB path
	// Since :memory: is per-connection, we need to test with a file
	// Instead, just verify that calling bootstrapAdmin again doesn't create duplicates
	if err := a1.bootstrapAdmin(); err != nil {
		t.Fatalf("second bootstrapAdmin call failed: %v", err)
	}
	count, _ := a1.store.CountUsers()
	if count != 1 {
		t.Errorf("after re-bootstrap, user count = %d, want 1 (no duplicates)", count)
	}

	_ = sqlDB
}

// --- TestLoginLocal ---

func TestLoginLocal_Success(t *testing.T) {
	a, admin := testAuthT1(t)

	// Change admin's password to something known (bootstrap has "admin")
	user, token, err := a.LoginLocal("admin", "admin")
	if err != nil {
		t.Fatalf("LoginLocal failed: %v", err)
	}

	if user.ID != admin.ID {
		t.Errorf("user.ID = %d, want %d", user.ID, admin.ID)
	}
	if user.Username != "admin" {
		t.Errorf("user.Username = %q, want 'admin'", user.Username)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	if len(strings.Split(token, ".")) != 3 {
		t.Error("token should be a valid JWT with 3 parts")
	}
}

func TestLoginLocal_WrongPassword(t *testing.T) {
	a := testAuth(t)

	_, _, err := a.LoginLocal("admin", "wrong-password")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLoginLocal_NonExistentUser(t *testing.T) {
	a := testAuth(t)

	_, _, err := a.LoginLocal("ghost-user", "anything")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLoginLocal_NonLocalUser(t *testing.T) {
	a := testAuth(t)

	// Create a non-local (OIDC) user directly in the store
	oidcUser := &User{
		Username:    "github-user",
		DisplayName: "GitHub User",
		Role:        "viewer",
		Provider:    "oidc:github",
		ProviderID:  "gh-12345",
	}
	if err := a.store.CreateUser(oidcUser); err != nil {
		t.Fatalf("failed to create OIDC user: %v", err)
	}

	_, _, err := a.LoginLocal("github-user", "any-password")
	if err == nil {
		t.Fatal("expected error for non-local user login, got nil")
	}
	if !strings.Contains(err.Error(), "not a local user") {
		t.Errorf("error should mention 'not a local user', got: %v", err)
	}
}

// --- TestVerifyToken ---

func TestVerifyToken_Valid(t *testing.T) {
	a, admin := testAuthT1(t)

	// Login to get a real token
	_, token, err := a.LoginLocal("admin", "admin")
	if err != nil {
		t.Fatalf("LoginLocal failed: %v", err)
	}

	// Verify the token
	claims, err := a.VerifyToken(token)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if claims.UserID != admin.ID {
		t.Errorf("claims.UserID = %d, want %d", claims.UserID, admin.ID)
	}
	if claims.Username != "admin" {
		t.Errorf("claims.Username = %q, want 'admin'", claims.Username)
	}
	if claims.Role != "admin" {
		t.Errorf("claims.Role = %q, want 'admin'", claims.Role)
	}
	if claims.Issuer != "k8ops" {
		t.Errorf("claims.Issuer = %q, want 'k8ops'", claims.Issuer)
	}
	if claims.Subject != "admin" {
		t.Errorf("claims.Subject = %q, want 'admin'", claims.Subject)
	}
	// Token should expire in the future
	if claims.ExpiresAt == nil {
		t.Fatal("claims.ExpiresAt should not be nil")
	}
	if claims.ExpiresAt.Before(time.Now()) {
		t.Error("token should not be expired")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	a := testAuth(t)

	// Generate an already-expired token by using a custom authenticator
	// with a negative expiry
	original := a.cfg.JWTExpiry
	a.cfg.JWTExpiry = -1 * time.Hour
	expiredToken, err := a.generateToken(&User{
		ID:       1,
		Username: "admin",
		Role:     "admin",
	})
	a.cfg.JWTExpiry = original
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	_, err = a.VerifyToken(expiredToken)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	a := testAuth(t)

	// Generate a token with the test secret
	_, token, _ := a.LoginLocal("admin", "admin")

	// Create a second authenticator with a different secret
	a2 := &Authenticator{
		store: a.store,
		cfg: &Config{
			JWTSecret: "different-secret-key",
		},
	}

	_, err := a2.VerifyToken(token)
	if err == nil {
		t.Fatal("expected error for token signed with wrong secret, got nil")
	}
}

func TestVerifyToken_Malformed(t *testing.T) {
	a := testAuth(t)

	tests := []string{
		"not-a-jwt",
		"",
		"aaa.bbb.ccc",
		"header.payload.",
	}

	for _, badToken := range tests {
		_, err := a.VerifyToken(badToken)
		if err == nil {
			t.Errorf("expected error for malformed token %q, got nil", badToken)
		}
	}
}

func TestVerifyToken_RejectsNoneAlgorithm(t *testing.T) {
	a := testAuth(t)

	// Forge a token with alg=none (JWT security bypass attempt)
	claims := &UserClaims{
		UserID:   1,
		Username: "admin",
		Role:     "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "k8ops",
		},
	}
	// alg=none produces an unsigned token
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	forged, _ := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := a.VerifyToken(forged)
	if err == nil {
		t.Fatal("VerifyToken must reject alg=none tokens")
	}
}

// --- TestChangePassword ---

func TestChangePassword_Success(t *testing.T) {
	a, admin := testAuthT1(t)

	err := a.ChangePassword(admin.ID, "admin", "new-secret-password")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Old password should no longer work
	_, _, err = a.LoginLocal("admin", "admin")
	if err != ErrInvalidCredentials {
		t.Errorf("old password should fail after change, got: %v", err)
	}

	// New password should work
	user, _, err := a.LoginLocal("admin", "new-secret-password")
	if err != nil {
		t.Fatalf("new password login failed: %v", err)
	}

	// MustChangePwd should be cleared
	if user.MustChangePwd {
		t.Error("MustChangePwd should be false after ChangePassword")
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	a, admin := testAuthT1(t)

	err := a.ChangePassword(admin.ID, "wrong-old-password", "new-password")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestChangePassword_NonExistentUser(t *testing.T) {
	a := testAuth(t)

	err := a.ChangePassword(99999, "any", "new")
	if err != ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got: %v", err)
	}
}

func TestChangePassword_NonLocalUser(t *testing.T) {
	a := testAuth(t)

	// Create an OIDC user
	oidcUser := &User{
		Username:    "gitlab-user",
		DisplayName: "GitLab User",
		Role:        "viewer",
		Provider:    "oidc:gitlab",
		ProviderID:  "gl-67890",
	}
	if err := a.store.CreateUser(oidcUser); err != nil {
		t.Fatalf("failed to create OIDC user: %v", err)
	}

	err := a.ChangePassword(oidcUser.ID, "any", "new")
	if err == nil {
		t.Fatal("expected error for non-local user, got nil")
	}
	if !strings.Contains(err.Error(), "cannot change password") {
		t.Errorf("error should mention 'cannot change password', got: %v", err)
	}
}

// --- TestAdminCreateUser ---

func TestAdminCreateUser(t *testing.T) {
	a := testAuth(t)

	t.Run("with explicit role", func(t *testing.T) {
		user, err := a.AdminCreateUser("operator1", "op-password", "op@example.com", "Operator One", "operator", "default,kube-system")
		if err != nil {
			t.Fatalf("AdminCreateUser failed: %v", err)
		}
		if user.Username != "operator1" {
			t.Errorf("Username = %q, want 'operator1'", user.Username)
		}
		if user.Role != "operator" {
			t.Errorf("Role = %q, want 'operator'", user.Role)
		}
		if user.Provider != "local" {
			t.Errorf("Provider = %q, want 'local'", user.Provider)
		}
		if user.Email != "op@example.com" {
			t.Errorf("Email = %q", user.Email)
		}
		if !user.MustChangePwd {
			t.Error("MustChangePwd should be true for newly created user")
		}
		if user.AllowedNamespaces != "default,kube-system" {
			t.Errorf("AllowedNamespaces = %q", user.AllowedNamespaces)
		}
		// Should be able to login with the new password
		_, _, err = a.LoginLocal("operator1", "op-password")
		if err != nil {
			t.Errorf("new user should be able to login: %v", err)
		}
	})

	t.Run("default role is viewer", func(t *testing.T) {
		user, err := a.AdminCreateUser("viewer1", "vw-password", "", "", "", "")
		if err != nil {
			t.Fatalf("AdminCreateUser failed: %v", err)
		}
		if user.Role != "viewer" {
			t.Errorf("Role = %q, want default 'viewer'", user.Role)
		}
	})

	t.Run("duplicate username fails", func(t *testing.T) {
		_, err := a.AdminCreateUser("admin", "password", "", "", "", "")
		if err == nil {
			t.Error("expected error for duplicate username, got nil")
		}
	})
}

// --- TestAdminResetPassword ---

func TestAdminResetPassword(t *testing.T) {
	a := testAuth(t)

	// Create a test user who has already changed password
	user, err := a.AdminCreateUser("resetuser", "initial-password", "", "Reset User", "viewer", "")
	if err != nil {
		t.Fatalf("AdminCreateUser failed: %v", err)
	}

	// Simulate user changed their password (MustChangePwd=false)
	if err := a.ChangePassword(user.ID, "initial-password", "user-chosen-pw"); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}
	updated, _ := a.store.GetUserByID(user.ID)
	if updated.MustChangePwd {
		t.Fatal("precondition: MustChangePwd should be false before reset")
	}

	// Admin resets password
	err = a.AdminResetPassword(user.ID, "admin-reset-pw")
	if err != nil {
		t.Fatalf("AdminResetPassword failed: %v", err)
	}

	// Verify MustChangePwd is true after reset
	reset, err := a.store.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !reset.MustChangePwd {
		t.Error("MustChangePwd should be true after admin reset")
	}

	// Old user-chosen password should no longer work
	_, _, err = a.LoginLocal("resetuser", "user-chosen-pw")
	if err != ErrInvalidCredentials {
		t.Errorf("old password should fail after reset, got: %v", err)
	}

	// New admin-reset password should work
	_, _, err = a.LoginLocal("resetuser", "admin-reset-pw")
	if err != nil {
		t.Errorf("reset password should work: %v", err)
	}
}
