package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// RegisterRoutes registers all auth-related HTTP routes on the mux.
func (a *Authenticator) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", a.loginRateLimitMiddleware(a.handleLogin))
	mux.HandleFunc("/api/auth/logout", a.handleLogout)
	mux.HandleFunc("/api/auth/me", a.handleMe)
	mux.HandleFunc("/api/auth/change-password", a.handleChangePassword)
	mux.HandleFunc("/api/auth/status", a.handleStatus)

	// Multi-provider OIDC routes (DB-backed): /api/auth/oidc/{provider}/login, /callback
	mux.HandleFunc("/api/auth/oidc/", a.handleMultiOIDC)

	// Multi-provider routes (DB-backed)
	a.RegisterProviderRoutes(mux)

	// Admin user management
	mux.Handle("/api/admin/users", a.Middleware(AdminOnly(http.HandlerFunc(a.handleUsers))))
	mux.Handle("/api/admin/users/", a.Middleware(AdminOnly(http.HandlerFunc(a.handleUserByID))))

	// Auth config management (legacy LDAP/OIDC settings + default role)
	mux.Handle("/api/admin/auth-config", a.Middleware(AdminOnly(http.HandlerFunc(a.handleAuthConfig))))

	// API key management (cookie or bearer auth)
	a.RegisterAPIKeyRoutes(mux)
}

// --- Login ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *Authenticator) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}

	// Try local auth first
	user, token, err := a.LoginLocal(req.Username, req.Password)
	if err == nil {
		SetAuthCookie(w, token, 86400)
		writeAuthJSON(w, http.StatusOK, map[string]any{
			"user":         sanitizeUser(user),
			"must_change":  user.MustChangePwd,
			"redirect_url": "/",
		})
		return
	}

	// If local login failed, try all enabled LDAP providers (DB + legacy)
	user, token, _, ldapErr := a.LoginLDAPAll(req.Username, req.Password)
	if ldapErr == nil {
		SetAuthCookie(w, token, 86400)
		writeAuthJSON(w, http.StatusOK, map[string]any{
			"user":         sanitizeUser(user),
			"must_change":  user.MustChangePwd,
			"redirect_url": "/",
		})
		return
	}

	writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid username or password"})
}

// --- Logout ---

func (a *Authenticator) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w)
	writeAuthJSON(w, http.StatusOK, map[string]any{"redirect_url": "/login.html"})
}

// --- Current User ---

func (a *Authenticator) handleMe(w http.ResponseWriter, r *http.Request) {
	user := UserFromRequest(r)
	if user == nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
		return
	}
	writeAuthJSON(w, http.StatusOK, map[string]any{"user": sanitizeUser(user)})
}

// --- Change Password ---

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (a *Authenticator) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := UserFromRequest(r)
	if user == nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "not authenticated"})
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	if err := a.ChangePassword(user.ID, req.OldPassword, req.NewPassword); err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeAuthJSON(w, http.StatusOK, map[string]any{"message": "password changed successfully"})
}

// --- Auth Status (public) ---

func (a *Authenticator) handleStatus(w http.ResponseWriter, r *http.Request) {
	count, _ := a.store.CountUsers()

	// Check if any DB-backed LDAP providers are enabled
	dbLDAPProviders, _ := a.store.GetEnabledProvidersByType(ProviderTypeLDAP)
	ldapEnabled := len(dbLDAPProviders) > 0

	// Build list of enabled OIDC providers for login buttons
	type oidcProviderInfo struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Icon        string `json:"icon"`
	}
	oidcProviders := []oidcProviderInfo{}
	if dbProviders, err := a.store.GetEnabledProvidersByType(ProviderTypeOIDC); err == nil {
		for _, p := range dbProviders {
			oidcProviders = append(oidcProviders, oidcProviderInfo{
				Name: p.Name, DisplayName: p.DisplayName, Icon: p.Icon,
			})
		}
	}

	writeAuthJSON(w, http.StatusOK, map[string]any{
		"auth_enabled":   true,
		"ldap_enabled":   ldapEnabled,
		"oidc_enabled":   len(oidcProviders) > 0,
		"oidc_providers": oidcProviders,
		"user_count":     count,
	})
}

// --- Multi-Provider OIDC ---

func (a *Authenticator) handleMultiOIDC(w http.ResponseWriter, r *http.Request) {
	// Path format: /api/auth/oidc/{provider}/login  or /api/auth/oidc/{provider}/callback
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/oidc/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		writeAuthJSON(w, http.StatusNotFound, map[string]any{"error": "invalid OIDC route"})
		return
	}

	providerName := parts[0]
	action := parts[1] // "login" or "callback"

	// Look up provider from DB
	p, err := a.store.GetAuthProvider(providerName)
	if err != nil {
		writeAuthJSON(w, http.StatusNotFound, map[string]any{"error": "OIDC provider not found: " + providerName})
		return
	}
	if !p.Enabled || p.Type != ProviderTypeOIDC {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "provider not enabled or not OIDC"})
		return
	}

	switch action {
	case "login":
		a.handleMultiOIDCLogin(w, r, p)
	case "callback":
		a.handleMultiOIDCCallback(w, r, p)
	default:
		writeAuthJSON(w, http.StatusNotFound, map[string]any{"error": "unknown OIDC action"})
	}
}

func (a *Authenticator) handleMultiOIDCLogin(w http.ResponseWriter, r *http.Request, p *AuthProvider) {
	h, err := NewOIDCHandlerFromProvider(r.Context(), p)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "OIDC init failed: " + err.Error()})
		return
	}

	url, state, err := h.AuthURL()
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate OIDC URL"})
		return
	}

	// Store state in a per-provider httpOnly cookie with Secure flag for HTTPS.
	// This binds the callback to the same browser that initiated the login,
	// providing CSRF protection for the OIDC flow.
	SetStateCookie(w, r, p.Name, state)

	http.Redirect(w, r, url, http.StatusFound)
}

func (a *Authenticator) handleMultiOIDCCallback(w http.ResponseWriter, r *http.Request, p *AuthProvider) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "missing code or state"})
		return
	}

	// Verify state from per-provider cookie (constant-time comparison)
	stateCookie, err := r.Cookie(stateCookieName(p.Name))
	if err != nil || !VerifyState(stateCookie.Value, state) {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid or expired state"})
		return
	}

	h, err := NewOIDCHandlerFromProvider(r.Context(), p)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "OIDC init failed"})
		return
	}

	info, err := h.Exchange(r.Context(), code)
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, map[string]any{"error": "OIDC exchange failed: " + err.Error()})
		return
	}

	// Use provider name as part of the provider key for uniqueness
	providerKey := "oidc:" + p.Name
	user, token, err := a.upsertOIDCUserForKey(providerKey, info.ProviderID, info.Username, info.Email, info.DisplayName)
	if err != nil {
		writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create user: " + err.Error()})
		return
	}

	SetAuthCookie(w, token, 86400)

	// Clear the per-provider state cookie after successful verification
	ClearStateCookie(w, p.Name)

	_ = user
	http.Redirect(w, r, "/", http.StatusFound)
}

// upsertOIDCUserForKey creates or updates a user with a custom provider key.
func (a *Authenticator) upsertOIDCUserForKey(providerKey, providerID, username, email, displayName string) (*User, string, error) {
	user, err := a.store.GetUserByProvider(providerKey, providerID)
	if err != nil {
		finalUsername := a.uniqueUsername(username, providerKey)
		user = &User{
			Username:    finalUsername,
			Email:       email,
			DisplayName: displayName,
			Role:        a.defaultRole(),
			Provider:    providerKey,
			ProviderID:  providerID,
		}
		if err := a.store.CreateUser(user); err != nil {
			return nil, "", fmt.Errorf("failed to create user: %w", err)
		}
	} else {
		if email != "" {
			user.Email = email
		}
		if displayName != "" {
			user.DisplayName = displayName
		}
		if err := a.store.UpdateUser(user); err != nil {
			slog.Warn("failed to update OIDC user on login", "user", user.Username, "error", err)
		}
	}

	token, err := a.generateToken(user)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// --- Admin User Management ---

func (a *Authenticator) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		users, err := a.store.ListUsers()
		if err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		result := make([]map[string]any, len(users))
		for i, u := range users {
			result[i] = sanitizeUser(&u)
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{"users": result})

	case http.MethodPost:
		var req struct {
			Username          string `json:"username"`
			Password          string `json:"password"`
			Email             string `json:"email"`
			DisplayName       string `json:"display_name"`
			Role              string `json:"role"`
			AllowedNamespaces string `json:"allowed_namespaces"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}
		if req.Username == "" || req.Password == "" {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
			return
		}
		user, err := a.AdminCreateUser(req.Username, req.Password, req.Email, req.DisplayName, req.Role, req.AllowedNamespaces)
		if err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		// Sync namespace RBAC if applicable
		if a.rbacSyncer != nil {
			if syncErr := a.rbacSyncer.SyncUserRBAC(r.Context(), user); syncErr != nil {
				slog.Warn("failed to sync user RBAC after creation", "user", user.Username, "error", syncErr)
			}
		}
		writeAuthJSON(w, http.StatusCreated, map[string]any{"user": sanitizeUser(user)})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (a *Authenticator) handleUserByID(w http.ResponseWriter, r *http.Request) {
	// Extract user ID from path /api/admin/users/{id} or /api/admin/users/{id}/reset-password
	parts := strings.SplitN(r.URL.Path, "/", 6)
	if len(parts) < 5 {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "user ID required"})
		return
	}
	// Extract user ID from path /api/admin/users/{id} or /api/admin/users/{id}/reset-password
	idStr := r.URL.Path
	if idx := strings.LastIndexByte(idStr, '/'); idx > 0 {
		segment := idStr[idx+1:]
		if segment == "reset-password" {
			// Strip the /reset-password suffix, then extract ID
			idStr = idStr[:idx]
		}
	}
	// Now extract the ID (last segment of the remaining path)
	if idx := strings.LastIndexByte(idStr, '/'); idx >= 0 {
		idStr = idStr[idx+1:]
	}
	id, err := strconvParseUint(idStr)
	if err != nil {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid user ID"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		// Clean up namespace RBAC before deleting user
		if a.rbacSyncer != nil {
			if user, err := a.store.GetUserByID(id); err == nil {
				if err := a.rbacSyncer.cleanupUserRBAC(r.Context(), user.Username); err != nil {
					slog.Warn("failed to cleanup user RBAC before deletion", "user", user.Username, "error", err)
				}
			}
		}
		if err := a.store.DeleteUser(id); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{"message": "user deleted"})

	case http.MethodPatch:
		var updates map[string]any
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}
		// Allow updating role, display_name, email, allowed_namespaces
		allowed := map[string]bool{"role": true, "display_name": true, "email": true, "allowed_namespaces": true}
		filtered := make(map[string]any)
		for k, v := range updates {
			if allowed[k] {
				filtered[k] = v
			}
		}
		if err := a.AdminUpdateUser(id, filtered); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// Sync namespace RBAC if role or namespaces changed
		if a.rbacSyncer != nil && (filtered["role"] != nil || filtered["allowed_namespaces"] != nil) {
			if updatedUser, err := a.store.GetUserByID(id); err == nil {
				if err := a.rbacSyncer.SyncUserRBAC(r.Context(), updatedUser); err != nil {
					slog.Warn("failed to sync user RBAC after update", "user", updatedUser.Username, "error", err)
				}
			}
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{"message": "user updated"})

	case http.MethodPost:
		// Reset password: POST /api/admin/users/{id}/reset-password
		if strings.HasSuffix(r.URL.Path, "/reset-password") {
			var req struct {
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
				return
			}
			if req.Password == "" {
				writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "password required"})
				return
			}
			if err := a.AdminResetPassword(id, req.Password); err != nil {
				writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeAuthJSON(w, http.StatusOK, map[string]any{"message": "password reset"})
			return
		}

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// --- Helpers ---

func sanitizeUser(u *User) map[string]any {
	return map[string]any{
		"id":                 u.ID,
		"username":           u.Username,
		"email":              u.Email,
		"display_name":       u.DisplayName,
		"role":               u.Role,
		"provider":           u.Provider,
		"must_change_pwd":    u.MustChangePwd,
		"allowed_namespaces": u.AllowedNamespaces,
		"created_at":         u.CreatedAt,
	}
}

func writeAuthJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// SetUserInContext stores the user in the request context.
func SetUserInContext(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, ContextKeyUser, user)
}

// strconvParseUint wraps strconv.ParseUint to avoid importing strconv in this file.
func strconvParseUint(s string) (uint, error) {
	var n uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrInvalidCredentials
		}
		n = n*10 + uint(c-'0')
	}
	return n, nil
}
