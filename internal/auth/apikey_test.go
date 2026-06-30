package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- APIKey Model Tests ---

func TestGenerateAPIKey_FormatAndUniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		plaintext, hash, prefix, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey() error: %v", err)
		}
		// Must have the correct prefix
		if !strings.HasPrefix(plaintext, APIKeyPrefix) {
			t.Errorf("key %q should start with %q", plaintext, APIKeyPrefix)
		}
		// Must be sufficiently long (32 bytes = ~43 base64 chars + prefix)
		if len(plaintext) < 40 {
			t.Errorf("key too short: %d chars", len(plaintext))
		}
		// Hash must be non-empty and different from plaintext
		if hash == "" || hash == plaintext {
			t.Error("hash should be non-empty and different from plaintext")
		}
		// Prefix must be the first 12 chars
		if prefix != plaintext[:12] {
			t.Errorf("prefix %q should be first 12 chars of %q", prefix, plaintext)
		}
		// No duplicates
		if keys[plaintext] {
			t.Errorf("duplicate key generated at iteration %d", i)
		}
		keys[plaintext] = true
	}
}

func TestHashAPIKey_Deterministic(t *testing.T) {
	key := "k8ops_test_key_123"
	h1 := HashAPIKey(key)
	h2 := HashAPIKey(key)
	if h1 != h2 {
		t.Error("HashAPIKey should be deterministic")
	}
}

func TestHashAPIKey_DifferentKeysDifferentHashes(t *testing.T) {
	h1 := HashAPIKey("k8ops_key_a")
	h2 := HashAPIKey("k8ops_key_b")
	if h1 == h2 {
		t.Error("different keys should produce different hashes")
	}
}

// --- Store CRUD Tests ---

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	if err := store.AutoMigrateAPIKeys(); err != nil {
		t.Fatalf("AutoMigrateAPIKeys() error: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStore_CreateAndGetAPIKeyByHash(t *testing.T) {
	store := newTestStore(t)

	// Create a user first (FK constraint)
	user := &User{Username: "testuser", Role: "viewer"}
	if err := store.CreateUser(user); err != nil {
		t.Fatal(err)
	}

	plaintext, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "test-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	if err := store.CreateAPIKey(apiKey); err != nil {
		t.Fatalf("CreateAPIKey() error: %v", err)
	}
	if apiKey.ID == 0 {
		t.Error("ID should be set after create")
	}

	// Retrieve by hash
	got, err := store.GetAPIKeyByHash(hash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error: %v", err)
	}
	if got.Name != "test-key" {
		t.Errorf("Name = %q, want %q", got.Name, "test-key")
	}
	if got.UserID != user.ID {
		t.Errorf("UserID = %d, want %d", got.UserID, user.ID)
	}

	_ = plaintext
}

func TestStore_ListAPIKeysByUser(t *testing.T) {
	store := newTestStore(t)

	user := &User{Username: "listuser", Role: "viewer"}
	if err := store.CreateUser(user); err != nil {
		t.Fatal(err)
	}

	// Create 3 keys
	for i := 0; i < 3; i++ {
		_, hash, prefix, _ := GenerateAPIKey()
		store.CreateAPIKey(&APIKey{
			UserID:    user.ID,
			Name:      "key-" + string(rune('a'+i)),
			KeyHash:   hash,
			KeyPrefix: prefix,
		})
	}

	keys, err := store.ListAPIKeysByUser(user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeysByUser() error: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("len(keys) = %d, want 3", len(keys))
	}
}

func TestStore_DeleteAPIKey_SoftDelete(t *testing.T) {
	store := newTestStore(t)

	user := &User{Username: "deluser", Role: "viewer"}
	store.CreateUser(user)

	_, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "to-delete",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	store.CreateAPIKey(apiKey)

	// Revoke it
	if err := store.DeleteAPIKey(apiKey.ID); err != nil {
		t.Fatalf("DeleteAPIKey() error: %v", err)
	}

	// Should not appear in list (revoked = true)
	keys, _ := store.ListAPIKeysByUser(user.ID)
	if len(keys) != 0 {
		t.Errorf("after revoke: len(keys) = %d, want 0", len(keys))
	}

	// But still exists in DB (soft delete)
	got, err := store.GetAPIKeyByID(apiKey.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID should still find revoked key: %v", err)
	}
	if !got.Revoked {
		t.Error("Revoked should be true")
	}
}

func TestStore_UpdateAPIKeyLastUsed(t *testing.T) {
	store := newTestStore(t)

	user := &User{Username: "useduser", Role: "viewer"}
	store.CreateUser(user)

	_, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "test-used",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	store.CreateAPIKey(apiKey)

	if apiKey.LastUsedAt != nil {
		t.Error("LastUsedAt should be nil initially")
	}

	store.UpdateAPIKeyLastUsed(apiKey.ID)
	time.Sleep(10 * time.Millisecond)

	got, _ := store.GetAPIKeyByID(apiKey.ID)
	if got.LastUsedAt == nil {
		t.Error("LastUsedAt should be set after UpdateAPIKeyLastUsed")
	}
}

// --- Authenticator.ValidateAPIKey Tests ---

func newTestAuthenticator(t *testing.T) *Authenticator {
	t.Helper()
	a, err := New(&Config{
		JWTSecret: "test-secret-key-for-testing-only",
		DBDriver:  "sqlite",
		DBDSN:     ":memory:",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	// Force single connection for SQLite in-memory tests (avoids per-connection DB isolation)
	a.store.db.Exec("PRAGMA journal_mode=WAL")
	sqlDB, _ := a.store.db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { a.Close() })
	return a
}

func TestValidateAPIKey_ValidKey(t *testing.T) {
	a := newTestAuthenticator(t)

	// Get the admin user
	user, err := a.store.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}

	// Create an API key
	plaintext, hash, prefix, _ := GenerateAPIKey()
	a.store.CreateAPIKey(&APIKey{
		UserID:    user.ID,
		Name:      "test",
		KeyHash:   hash,
		KeyPrefix: prefix,
	})

	// Validate
	gotUser, err := a.ValidateAPIKey(plaintext)
	if err != nil {
		t.Fatalf("ValidateAPIKey() error: %v", err)
	}
	if gotUser.ID != user.ID {
		t.Errorf("UserID = %d, want %d", gotUser.ID, user.ID)
	}
}

func TestValidateAPIKey_InvalidKey(t *testing.T) {
	a := newTestAuthenticator(t)

	_, err := a.ValidateAPIKey("k8ops_nonexistent_key")
	if err == nil {
		t.Error("ValidateAPIKey should fail for nonexistent key")
	}
}

func TestValidateAPIKey_RevokedKey(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	plaintext, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "revoked",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	a.store.CreateAPIKey(apiKey)

	// Revoke
	a.store.DeleteAPIKey(apiKey.ID)

	_, err := a.ValidateAPIKey(plaintext)
	if err == nil {
		t.Error("ValidateAPIKey should fail for revoked key")
	}
}

func TestValidateAPIKey_ExpiredKey(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	plaintext, hash, prefix, _ := GenerateAPIKey()

	expired := time.Now().Add(-1 * time.Hour)
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "expired",
		KeyHash:   hash,
		KeyPrefix: prefix,
		ExpiresAt: &expired,
	}
	a.store.CreateAPIKey(apiKey)

	_, err := a.ValidateAPIKey(plaintext)
	if err == nil {
		t.Error("ValidateAPIKey should fail for expired key")
	}
}

// --- Middleware Dual-Mode Tests ---

func TestMiddleware_BearerTokenAuth(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	plaintext, hash, prefix, _ := GenerateAPIKey()
	a.store.CreateAPIKey(&APIKey{
		UserID:    user.ID,
		Name:      "middleware-test",
		KeyHash:   hash,
		KeyPrefix: prefix,
	})

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromRequest(r)
		if u == nil {
			t.Error("user should be set in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_BearerTokenInvalid(t *testing.T) {
	a := newTestAuthenticator(t)

	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Authorization", "Bearer k8ops_invalid_key_12345")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMiddleware_CookieStillWorks(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	token, err := a.generateToken(user)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called with valid cookie")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// --- HTTP Handler Tests ---

func TestHandleCreateAPIKey_ReturnsPlaintextOnce(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")

	body := `{"name": "ci-key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/api-keys", strings.NewReader(body))

	// Set user in context (normally done by middleware)
	ctx := SetUserInContext(req.Context(), user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	a.handleAPIKeys(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	key, ok := resp["key"]
	if !ok || key == "" {
		t.Error("response should contain plaintext 'key'")
	}
	if !strings.HasPrefix(key.(string), APIKeyPrefix) {
		t.Errorf("key should start with %q", APIKeyPrefix)
	}
	if resp["key_prefix"] == "" {
		t.Error("response should contain key_prefix")
	}
}

func TestHandleListAPIKeys_NoHashInResponse(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")

	// Create a key
	plaintext, hash, prefix, _ := GenerateAPIKey()
	a.store.CreateAPIKey(&APIKey{
		UserID:    user.ID,
		Name:      "list-test",
		KeyHash:   hash,
		KeyPrefix: prefix,
	})

	_ = plaintext

	req := httptest.NewRequest(http.MethodGet, "/api/auth/api-keys", nil)

	// Set user in context (normally done by middleware)
	ctx := SetUserInContext(req.Context(), user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	a.handleAPIKeys(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if strings.Contains(body, "key_hash") {
		t.Error("response should not contain key_hash")
	}
	if strings.Contains(body, hash) {
		t.Error("response should not contain the actual hash value")
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	keys, ok := resp["api_keys"].([]any)
	if !ok || len(keys) != 1 {
		t.Errorf("expected 1 key, got %v", resp["api_keys"])
	}
}

func TestHandleDeleteAPIKey_RevokesOwnKey(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	token, _ := a.generateToken(user)

	_, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      "delete-test",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	a.store.CreateAPIKey(apiKey)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/auth/api-keys/%d", apiKey.ID), nil)
	req.AddCookie(&http.Cookie{Name: "k8ops_token", Value: token})

	// Need to set the user in context (normally done by middleware)
	ctx := SetUserInContext(req.Context(), user)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	a.handleAPIKeyByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify key is revoked
	got, _ := a.store.GetAPIKeyByID(apiKey.ID)
	if !got.Revoked {
		t.Error("key should be revoked after delete")
	}
}

func TestHandleDeleteAPIKey_OwnershipCheck(t *testing.T) {
	a := newTestAuthenticator(t)

	// Create another user
	user2 := &User{Username: "otheruser", Role: "viewer", PasswordHash: "$2a$12$dummy"}
	a.store.CreateUser(user2)

	// user2 creates a key
	_, hash, prefix, _ := GenerateAPIKey()
	apiKey := &APIKey{
		UserID:    user2.ID,
		Name:      "other-user-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
	}
	a.store.CreateAPIKey(apiKey)

	// admin tries to delete — should succeed
	admin, _ := a.store.GetUserByUsername("admin")
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/auth/api-keys/%d", apiKey.ID), nil)
	ctx := SetUserInContext(req.Context(), admin)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	a.handleAPIKeyByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("admin should be able to revoke any key: status = %d", rec.Code)
	}
}

// --- Concurrency Test ---

func TestAPIKey_ConcurrentValidation(t *testing.T) {
	a := newTestAuthenticator(t)

	user, _ := a.store.GetUserByUsername("admin")
	plaintext, hash, prefix, _ := GenerateAPIKey()
	a.store.CreateAPIKey(&APIKey{
		UserID:    user.ID,
		Name:      "concurrent",
		KeyHash:   hash,
		KeyPrefix: prefix,
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := a.ValidateAPIKey(plaintext)
			if err != nil || u == nil {
				t.Errorf("concurrent ValidateAPIKey failed: %v", err)
			}
		}()
	}
	wg.Wait()
	// Give spawned UpdateAPIKeyLastUsed goroutines time to finish before t.Cleanup closes DB
	time.Sleep(200 * time.Millisecond)
}

// --- Helper ---

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}
