package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// TestMain sets bcrypt to MinCost for all auth tests (~0.001s per hash vs ~0.4s at cost 12).
func TestMain(m *testing.M) {
	bcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}

// testAuth creates an Authenticator backed by an in-memory SQLite database,
// pre-seeded with the bootstrap admin (admin/admin). The DB connection pool
// is limited to 1 to ensure all queries hit the same in-memory database.
func testAuth(t *testing.T) *Authenticator {
	t.Helper()
	cfg := &Config{
		DBDriver:  "sqlite",
		DBDSN:     ":memory:",
		JWTSecret: "test-secret-key",
		JWTExpiry: time.Hour,
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}
	sqlDB, err := a.store.db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func TestIsPublicRoute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Static assets
		{"/style.css", true},
		{"/app.js", true},
		{"/favicon.ico", true},
		{"/logo.png", true},
		{"/icon.svg", true},
		{"/font.woff2", true},

		// Login page
		{"/login.html", true},

		// Auth API endpoints
		{"/api/auth/login", true},
		{"/api/auth/logout", true},
		{"/api/auth/status", true},
		{"/api/auth/oidc/google/login", true},
		{"/api/auth/callback", true},
		{"/api/auth/provider-presets", true},

		// Health
		{"/api/health", true},

		// Protected routes
		{"/api/diagnostics", false},
		{"/api/nodes", false},
		{"/api/admin/users", false},
		{"/api/chat", false},
		{"/index.html", false},
		{"/dashboard", false},
		{"/api/auth/me", false},
		{"/api/auth/change-password", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPublicRoute(tt.path); got != tt.want {
				t.Errorf("isPublicRoute(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsAPIRequest(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/health", true},
		{"/api/auth/login", true},
		{"/api/diagnostics", true},
		{"/api/admin/users", true},
		{"/index.html", false},
		{"/login.html", false},
		{"/dashboard", false},
		{"/", false},
		{"/static/app.js", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isAPIRequest(tt.path); got != tt.want {
				t.Errorf("isAPIRequest(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestAdminOnly(t *testing.T) {
	tests := []struct {
		name       string
		user       *User
		wantStatus int
	}{
		{
			name:       "admin passes through",
			user:       &User{Role: "admin"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "viewer gets 403",
			user:       &User{Role: "viewer"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "nil user gets 403",
			user:       nil,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "operator gets 403",
			user:       &User{Role: "operator"},
			wantStatus: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
			if tt.user != nil {
				req = req.WithContext(SetUserInContext(req.Context(), tt.user))
			}

			next.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && !called {
				t.Error("next handler was not called for admin user")
			}
			if tt.wantStatus == http.StatusForbidden && called {
				t.Error("next handler should not be called for non-admin")
			}
		})
	}
}

func TestSetAuthCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	SetAuthCookie(rec, "test-jwt-token", 86400)

	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })

	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "k8ops_token" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("k8ops_token cookie not set")
	}

	if cookie.Value != "test-jwt-token" {
		t.Errorf("cookie Value = %q, want %q", cookie.Value, "test-jwt-token")
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if cookie.Path != "/" {
		t.Errorf("cookie Path = %q, want %q", cookie.Path, "/")
	}
	if cookie.MaxAge != 86400 {
		t.Errorf("cookie MaxAge = %d, want %d", cookie.MaxAge, 86400)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want %v", cookie.SameSite, http.SameSiteLaxMode)
	}
}

func TestClearAuthCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	clearAuthCookie(rec)

	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })

	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "k8ops_token" {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("k8ops_token clear cookie not set")
	}

	if cookie.MaxAge != -1 {
		t.Errorf("cookie MaxAge = %d, want -1", cookie.MaxAge)
	}
	if cookie.Value != "" {
		t.Errorf("cookie Value = %q, want empty", cookie.Value)
	}
}
