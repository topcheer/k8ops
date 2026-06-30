package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// RegisterProviderRoutes registers auth provider CRUD routes.
func (a *Authenticator) RegisterProviderRoutes(mux *http.ServeMux) {
	mux.Handle("/api/auth/providers", a.Middleware(AdminOnly(http.HandlerFunc(a.handleProviders))))
	mux.Handle("/api/auth/providers/", a.Middleware(AdminOnly(http.HandlerFunc(a.handleProviderByID))))
	mux.HandleFunc("/api/auth/provider-presets", a.handleProviderPresets) // public for login page

	// Per-provider OIDC routes — registered dynamically
	// Login:  GET /api/auth/oidc/{provider}/login
	// Callback: GET /api/auth/oidc/{provider}/callback
}

func (a *Authenticator) handleProviderPresets(w http.ResponseWriter, r *http.Request) {
	writeAuthJSON(w, http.StatusOK, map[string]any{"presets": ProviderPresets})
}

func (a *Authenticator) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := a.store.ListAuthProviders()
		if err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		result := make([]map[string]any, len(providers))
		for i, p := range providers {
			result[i] = providerToAPI(&p)
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{"providers": result})

	case http.MethodPost:
		var req createProviderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}
		if req.Name == "" || req.Type == "" {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "name and type required"})
			return
		}

		p := AuthProvider{
			Name:        req.Name,
			Type:        AuthProviderType(req.Type),
			DisplayName: req.DisplayName,
			Icon:        req.Icon,
			Enabled:     req.Enabled,
			Priority:    req.Priority,
		}
		if p.DisplayName == "" {
			p.DisplayName = p.Name
		}
		if p.Icon == "" {
			p.Icon = string(p.Type)
		}

		// Build config
		pc := ProviderConfig{}
		if req.LDAP != nil {
			pc.LDAP = req.LDAP
		}
		if req.OIDC != nil {
			pc.OIDC = req.OIDC
			if len(pc.OIDC.Scopes) == 0 {
				pc.OIDC.Scopes = []string{"openid", "profile", "email"}
			}
		}
		if err := p.SetConfig(pc); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		if err := a.store.CreateAuthProvider(&p); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}

		writeAuthJSON(w, http.StatusCreated, map[string]any{"provider": providerToAPI(&p), "message": "provider created"})

	default:
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (a *Authenticator) handleProviderByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/auth/providers/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "provider id required"})
		return
	}

	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
			return
		}
	}

	idStr := parts[0]
	p, err := a.store.GetAuthProviderByID(parseUint(idStr))
	if err != nil {
		writeAuthJSON(w, http.StatusNotFound, map[string]any{"error": "provider not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeAuthJSON(w, http.StatusOK, map[string]any{"provider": providerToAPI(p)})

	case http.MethodPut, http.MethodPatch:
		var req updateProviderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAuthJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}

		if req.DisplayName != "" {
			p.DisplayName = req.DisplayName
		}
		if req.Icon != "" {
			p.Icon = req.Icon
		}
		if req.Enabled != nil {
			p.Enabled = *req.Enabled
		}
		if req.Priority != nil {
			p.Priority = *req.Priority
		}

		// Update config (merge secrets: if masked "••••••••", keep existing)
		if req.LDAP != nil || req.OIDC != nil {
			pc := p.ParseConfig()
			if req.LDAP != nil {
				if pc.LDAP == nil {
					pc.LDAP = &LDAPConfig{}
				}
				mergeLDAPConfig(pc.LDAP, req.LDAP)
			}
			if req.OIDC != nil {
				if pc.OIDC == nil {
					pc.OIDC = &OIDCConfig{}
				}
				mergeOIDCConfig(pc.OIDC, req.OIDC)
			}
			if err := p.SetConfig(pc); err != nil {
				writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
		}

		if err := a.store.UpdateAuthProvider(p); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}

		writeAuthJSON(w, http.StatusOK, map[string]any{"provider": providerToAPI(p), "message": "provider updated"})

	case http.MethodDelete:
		if err := a.store.DeleteAuthProvider(p.ID); err != nil {
			writeAuthJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAuthJSON(w, http.StatusOK, map[string]any{"message": "provider deleted"})

	default:
		writeAuthJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// --- types ---

type createProviderReq struct {
	Name        string           `json:"name"`
	Type        string           `json:"type"`
	DisplayName string           `json:"display_name"`
	Icon        string           `json:"icon"`
	Enabled     bool             `json:"enabled"`
	Priority    int              `json:"priority"`
	LDAP        *LDAPConfig      `json:"ldap,omitempty"`
	OIDC        *OIDCConfig      `json:"oidc,omitempty"`
}

type updateProviderReq struct {
	DisplayName string      `json:"display_name"`
	Icon        string      `json:"icon"`
	Enabled     *bool       `json:"enabled"`
	Priority    *int        `json:"priority"`
	LDAP        *LDAPConfig `json:"ldap,omitempty"`
	OIDC        *OIDCConfig `json:"oidc,omitempty"`
}

// --- helpers ---

func providerToAPI(p *AuthProvider) map[string]any {
	m := map[string]any{
		"id":           p.ID,
		"name":         p.Name,
		"type":         p.Type,
		"display_name": p.DisplayName,
		"icon":         p.Icon,
		"enabled":      p.Enabled,
		"priority":     p.Priority,
		"created_at":   p.CreatedAt,
	}
	pc := p.ToAPIConfig()
	if pc.LDAP != nil {
		m["config"] = pc.LDAP
		m["config_type"] = "ldap"
	} else if pc.OIDC != nil {
		m["config"] = pc.OIDC
		m["config_type"] = "oidc"
	}
	return m
}

func mergeLDAPConfig(dst, src *LDAPConfig) {
	if src.Server != "" && src.Server != "••••••••" {
		dst.Server = src.Server
	}
	if src.BindDN != "" {
		dst.BindDN = src.BindDN
	}
	if src.BindPW != "" && src.BindPW != "••••••••" {
		dst.BindPW = src.BindPW
	}
	if src.SearchBase != "" {
		dst.SearchBase = src.SearchBase
	}
	if src.SearchFilter != "" {
		dst.SearchFilter = src.SearchFilter
	}
	dst.StartTLS = src.StartTLS
	dst.SkipTLSVerify = src.SkipTLSVerify
}

func mergeOIDCConfig(dst, src *OIDCConfig) {
	if src.Issuer != "" {
		dst.Issuer = src.Issuer
	}
	if src.ClientID != "" {
		dst.ClientID = src.ClientID
	}
	if src.ClientSecret != "" && src.ClientSecret != "••••••••" {
		dst.ClientSecret = src.ClientSecret
	}
	if src.RedirectURL != "" {
		dst.RedirectURL = src.RedirectURL
	}
	if len(src.Scopes) > 0 {
		dst.Scopes = src.Scopes
	}
}

func parseUint(s string) uint {
	var n uint
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + uint(c-'0')
		}
	}
	return n
}

// --- Multi-provider login helpers ---

// LoginLDAPAll tries all enabled LDAP providers in order.
// Returns the first successful login.
func (a *Authenticator) LoginLDAPAll(username, password string) (*User, string, *AuthProvider, error) {
	providers, err := a.store.GetEnabledProvidersByType(ProviderTypeLDAP)
	if err != nil || len(providers) == 0 {
		return nil, "", nil, ErrInvalidCredentials
	}

	var lastErr error
	for _, p := range providers {
		user, token, err := a.loginLDAPProvider(&p, username, password)
		if err == nil {
			return user, token, &p, nil
		}
		lastErr = err
	}
	return nil, "", nil, lastErr
}

// loginLDAPProvider authenticates against a specific LDAP provider from DB.
func (a *Authenticator) loginLDAPProvider(p *AuthProvider, username, password string) (*User, string, error) {
	var pc ProviderConfig
	if err := json.Unmarshal([]byte(p.Config), &pc); err != nil {
		slog.Warn("failed to unmarshal provider config for LDAP login", "provider", p.Name, "error", err)
	}
	if pc.LDAP == nil {
		return nil, "", ErrInvalidCredentials
	}

	dn, attrs, err := a.ldapAuthenticateWithConfig(pc.LDAP, username, password)
	if err != nil {
		return nil, "", err
	}

	// Find or create user by provider+DN
	providerKey := "ldap:" + p.Name
	user, err := a.store.GetUserByProvider(providerKey, dn)
	if err != nil {
		finalUsername := a.uniqueUsername(username, p.Name)
		user = &User{
			Username:    finalUsername,
			DisplayName: ldapAttr(attrs, "cn", username),
			Email:       ldapAttr(attrs, "mail"),
			Role:        a.defaultRole(),
			Provider:    providerKey,
			ProviderID:  dn,
		}
		if err := a.store.CreateUser(user); err != nil {
			return nil, "", err
		}
	} else {
		if email := ldapAttr(attrs, "mail"); email != "" {
			user.Email = email
		}
		if cn := ldapAttr(attrs, "cn"); cn != "" {
			user.DisplayName = cn
		}
		if err := a.store.UpdateUser(user); err != nil {
			slog.Warn("failed to update LDAP user on login", "user", user.Username, "error", err)
		}
	}

	token, err := a.generateToken(user)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}
