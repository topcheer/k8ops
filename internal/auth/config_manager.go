package auth

import (
	"encoding/json"
	"net/http"
	"sync"
)

// ConfigManager provides thread-safe access to mutable auth configuration.
// It allows runtime updates to default role settings.
type ConfigManager struct {
	mu  sync.RWMutex
	cfg *Config
}

// configResponse is the API representation of auth config.
type configResponse struct {
	JWTExpiry string `json:"jwt_expiry"`

	DefaultRole              string `json:"default_role"`
	DefaultAllowedNamespaces string `json:"default_allowed_namespaces"`
}

// configRequest is used to update auth config via API.
type configRequest struct {
	JWTExpiry string `json:"jwt_expiry"`

	DefaultRole              string `json:"default_role"`
	DefaultAllowedNamespaces string `json:"default_allowed_namespaces"`
}

// GetConfigResponse returns a safe (secrets masked) representation of the auth config.
func (a *Authenticator) GetConfigResponse() configResponse {
	c := a.cfg
	return configResponse{
		JWTExpiry:                c.JWTExpiry.String(),
		DefaultRole:              c.DefaultRole,
		DefaultAllowedNamespaces: c.DefaultAllowedNamespaces,
	}
}

// UpdateConfig applies config changes from the API request.
// Empty password/secret fields are ignored (keep existing).
func (a *Authenticator) UpdateConfig(req configRequest) {
	a.cfg.DefaultRole = req.DefaultRole
	a.cfg.DefaultAllowedNamespaces = req.DefaultAllowedNamespaces
}

// --- HTTP Handlers ---

// handleAuthConfig handles GET/PUT /api/admin/auth-config
func (a *Authenticator) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeAuthJSON(w, http.StatusOK, a.GetConfigResponse())

	case http.MethodPut, http.MethodPost:
		var req configRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		a.UpdateConfig(req)
		writeAuthJSON(w, http.StatusOK, map[string]any{
			"message": "auth config updated",
			"config":  a.GetConfigResponse(),
		})

	default:
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}
