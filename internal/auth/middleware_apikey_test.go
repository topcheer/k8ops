package auth

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- AdminOnly middleware tests ---

func TestAdminOnly_AllowsAdmin(t *testing.T) {
	a := testAuth(t)

	user, err := a.store.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	token, err := a.generateToken(user)
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}

	called := false
	h := a.Middleware(AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: token})

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("admin handler should be called for admin user")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAdminOnly_RejectsNilUser(t *testing.T) {
	called := false
	h := AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called when no user in context")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// --- UserFromRequest tests ---

func TestUserFromRequest_WithUser(t *testing.T) {
	expectedUser := &User{ID: 42, Email: "test@test.com", Role: "admin"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := SetUserInContext(req.Context(), expectedUser)
	req = req.WithContext(ctx)

	got := UserFromRequest(req)
	if got == nil {
		t.Fatal("UserFromRequest() returned nil")
	}
	if got.ID != 42 {
		t.Errorf("user ID = %d, want 42", got.ID)
	}
	if got.Role != "admin" {
		t.Errorf("user Role = %q, want %q", got.Role, "admin")
	}
}

func TestUserFromRequest_WithoutUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := UserFromRequest(req)
	if got != nil {
		t.Errorf("UserFromRequest() should return nil, got %+v", got)
	}
}

func TestSetUserInContext_RoundTrip(t *testing.T) {
	user := &User{ID: 99, Email: "round@trip.com", Role: "operator"}
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	ctx := SetUserInContext(req.Context(), user)
	req = req.WithContext(ctx)

	got := UserFromRequest(req)
	if got == nil || got.ID != 99 {
		t.Errorf("round-trip failed: got %+v", got)
	}
}

// --- Middleware integration tests ---

func TestMiddleware_PublicRoutePassThrough(t *testing.T) {
	a := testAuth(t)

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("public route should pass through")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_InvalidBearerToken(t *testing.T) {
	a := testAuth(t)

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer k8ops_invalid_key")

	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called with invalid bearer token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	cfg := &Config{
		DBDriver:  "sqlite",
		DBDSN:     ":memory:",
		JWTSecret: "test-secret-key",
		JWTExpiry: 1 * time.Millisecond,
	}
	shortAuth, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer shortAuth.Close()

	user, _ := shortAuth.store.GetUserByUsername("admin")
	token, err := shortAuth.generateToken(user)
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	called := false
	h := shortAuth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: token})

	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called with expired token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_NoAuthRedirectsBrowser(t *testing.T) {
	a := testAuth(t)

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called without auth")
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login.html" {
		t.Errorf("Location = %q, want %q", loc, "/login.html")
	}
}

func TestMiddleware_NoAuthReturns401ForAPI(t *testing.T) {
	a := testAuth(t)

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/history", nil)
	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called without auth on API route")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_ValidCookieAuth(t *testing.T) {
	a := testAuth(t)

	user, _ := a.store.GetUserByUsername("admin")
	token, err := a.generateToken(user)
	if err != nil {
		t.Fatalf("generateToken() error: %v", err)
	}

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		u := UserFromRequest(r)
		if u == nil {
			t.Error("user should be in context after cookie auth")
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: token})

	h.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called with valid cookie")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_InvalidCookie(t *testing.T) {
	a := testAuth(t)

	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: "invalid.jwt.token"})

	h.ServeHTTP(rec, req)

	if called {
		t.Error("handler should NOT be called with invalid cookie")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- ValidateAPIKey concurrent test ---

func TestValidateAPIKey_ConcurrentAccess(t *testing.T) {
	a := testAuth(t)

	plaintext, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error: %v", err)
	}
	apiKey := &APIKey{
		UserID:    1,
		Name:      "concurrent-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	if err := a.store.CreateAPIKey(apiKey); err != nil {
		t.Fatalf("CreateAPIKey() error: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := a.ValidateAPIKey(plaintext)
			if err != nil {
				t.Errorf("concurrent ValidateAPIKey() error: %v", err)
			}
		}()
	}
	wg.Wait()
}
