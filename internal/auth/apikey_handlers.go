package auth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RegisterAPIKeyRoutes registers API key management routes on the mux.
// These routes are protected by the auth middleware (cookie or bearer).
func (a *Authenticator) RegisterAPIKeyRoutes(mux *http.ServeMux) {
	mux.Handle("/api/auth/api-keys", a.Middleware(http.HandlerFunc(a.handleAPIKeys)))
	mux.Handle("/api/auth/api-keys/", a.Middleware(http.HandlerFunc(a.handleAPIKeyByID)))
}

// --- API Key CRUD ---

type createAPIKeyRequest struct {
	Name      string `json:"name"`       // human-readable label
	ExpiresIn int    `json:"expires_in"` // expiry in days, 0 = never
}

// handleAPIKeys handles POST (create) and GET (list) for API keys.
func (a *Authenticator) handleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleCreateAPIKey(w, r)
	case http.MethodGet:
		a.handleListAPIKeys(w, r)
	default:
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (a *Authenticator) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := UserFromRequest(r)
	if user == nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if req.Name == "" {
		req.Name = "default"
	}

	// Generate the key
	plaintext, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate API key"})
		return
	}

	apiKey := &APIKey{
		UserID:    user.ID,
		Name:      req.Name,
		KeyHash:   hash,
		KeyPrefix: prefix,
	}

	// Set expiry if requested
	if req.ExpiresIn > 0 {
		expires := time.Now().Add(time.Duration(req.ExpiresIn) * 24 * time.Hour)
		apiKey.ExpiresAt = &expires
	}

	if err := a.store.CreateAPIKey(apiKey); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save API key"})
		return
	}

	// Return the plaintext key ONCE — the user must save it now.
	writeAuthJSON(w, http.StatusCreated, map[string]any{
		"id":         apiKey.ID,
		"name":       apiKey.Name,
		"key":        plaintext, // only returned once
		"key_prefix": prefix,
		"expires_at": apiKey.ExpiresAt,
		"created_at": apiKey.CreatedAt,
		"message":    "Save this key securely. It will not be shown again.",
	})
}

func (a *Authenticator) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := UserFromRequest(r)
	if user == nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
		return
	}

	keys, err := a.store.ListAPIKeysByUser(user.ID)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list API keys"})
		return
	}

	// Sanitize: never return the hash
	result := make([]map[string]any, len(keys))
	for i, k := range keys {
		result[i] = map[string]any{
			"id":           k.ID,
			"name":         k.Name,
			"key_prefix":   k.KeyPrefix,
			"last_used_at": k.LastUsedAt,
			"expires_at":   k.ExpiresAt,
			"created_at":   k.CreatedAt,
		}
	}

	writeAuthJSON(w, http.StatusOK, map[string]any{"api_keys": result})
}

// handleAPIKeyByID handles DELETE (revoke) for a specific API key.
func (a *Authenticator) handleAPIKeyByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	user := UserFromRequest(r)
	if user == nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
		return
	}

	// Extract ID from path: /api/auth/api-keys/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/api-keys/")
	idStr := strings.TrimSuffix(path, "/")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid API key ID"})
		return
	}

	// Verify ownership before revoking
	apiKey, err := a.store.GetAPIKeyByID(uint(id))
	if err != nil {
		writeAuthJSON(w, http.StatusNotFound, map[string]any{"error": "API key not found"})
		return
	}
	if apiKey.UserID != user.ID && user.Role != "admin" {
		writeAuthJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden: not your API key"})
		return
	}

	if err := a.store.DeleteAPIKey(uint(id)); err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to revoke API key"})
		return
	}

	writeAuthJSON(w, http.StatusOK, map[string]any{"message": "API key revoked"})
}
