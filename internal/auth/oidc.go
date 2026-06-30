package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCHandler manages the OIDC OAuth2 flow.
type OIDCHandler struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   *oauth2.Config
}

// NewOIDCHandlerFromProvider creates an OIDC handler from a DB-backed AuthProvider.
func NewOIDCHandlerFromProvider(ctx context.Context, p *AuthProvider) (*OIDCHandler, error) {
	pc := p.ParseConfig()
	if pc.OIDC == nil {
		return nil, fmt.Errorf("provider %s has no OIDC config", p.Name)
	}
	oc := pc.OIDC

	provider, err := oidc.NewProvider(ctx, oc.Issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider %s: %w", p.Name, err)
	}

	scopes := oc.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     oc.ClientID,
		ClientSecret: oc.ClientSecret,
		RedirectURL:  oc.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	return &OIDCHandler{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: oc.ClientID}),
		oauth2:   oauth2Cfg,
	}, nil
}

// AuthURL generates the OIDC authorization URL with a random state.
func (h *OIDCHandler) AuthURL() (string, string, error) {
	state, err := randomString(32)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomString(32)
	if err != nil {
		return "", "", err
	}
	url := h.oauth2.AuthCodeURL(state, oidc.Nonce(nonce))
	return url, state, nil
}

// Exchange exchanges the authorization code for tokens and returns user info.
func (h *OIDCHandler) Exchange(ctx context.Context, code string) (*OIDCUserInfo, error) {
	token, err := h.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange OIDC code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in OIDC response")
	}

	idToken, err := h.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify id_token: %w", err)
	}

	var claims struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Username string `json:"preferred_username"`
		Sub      string `json:"sub"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse id_token claims: %w", err)
	}

	// Also try userinfo endpoint for more data
	userInfo, err := h.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err == nil {
		var uiClaims struct {
			Email    string `json:"email"`
			Name     string `json:"name"`
			Username string `json:"preferred_username"`
		}
		if err := userInfo.Claims(&uiClaims); err != nil {
			slog.Warn("failed to unmarshal OIDC userinfo claims", "error", err)
		}
		if claims.Email == "" {
			claims.Email = uiClaims.Email
		}
		if claims.Name == "" {
			claims.Name = uiClaims.Name
		}
		if claims.Username == "" {
			claims.Username = uiClaims.Username
		}
	}

	if claims.Username == "" {
		claims.Username = claims.Email
	}

	return &OIDCUserInfo{
		ProviderID:  claims.Sub,
		Username:    claims.Username,
		Email:       claims.Email,
		DisplayName: claims.Name,
	}, nil
}

// OIDCUserInfo holds the normalized user info from the IdP.
type OIDCUserInfo struct {
	ProviderID  string
	Username    string
	Email       string
	DisplayName string
}

// randomString generates a cryptographically secure random string.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// stateCookieName returns the per-provider cookie name for storing the OIDC
// state value. Using a per-provider name prevents collisions when a user
// initiates flows for multiple OIDC providers in parallel.
func stateCookieName(providerName string) string {
	return "oidc_state_" + providerName
}

// providerCookieName returns the per-provider cookie name for remembering
// which OIDC provider was selected during the login redirect.
func providerCookieName(providerName string) string {
	return "oidc_provider_" + providerName
}

// SetStateCookie stores the OIDC state in an httpOnly cookie with SameSite=Lax
// and Secure (when the request is over TLS). The cookie is short-lived (5 min)
// to match the typical authorization-code exchange window.
func SetStateCookie(w http.ResponseWriter, r *http.Request, providerName, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName(providerName),
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	})
}

// ClearStateCookie removes the OIDC state cookie after a successful (or failed)
// callback. Called on every callback to ensure no stale state lingers.
func ClearStateCookie(w http.ResponseWriter, providerName string) {
	http.SetCookie(w, &http.Cookie{
		Name:   stateCookieName(providerName),
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// isHTTPS returns true when the request was made over TLS, either directly or
// behind a reverse proxy that sets X-Forwarded-Proto.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Check X-Forwarded-Proto for reverse-proxy setups
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		return true
	}
	return false
}

// VerifyState performs a constant-time comparison of the expected and actual
// state values to prevent timing attacks. Returns true only when both values
// are non-empty and byte-for-byte equal.
//
// The expected value should come from the server-side state cookie and the
// actual value from the IdP callback URL query parameter.
func VerifyState(expected, actual string) bool {
	if expected == "" || actual == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

// ContextKey is used to store values in request context.
type ContextKey string

const (
	// ContextKeyUser stores the authenticated user in request context.
	ContextKeyUser ContextKey = "user"
)

// UserFromRequest extracts the authenticated user from the request context.
func UserFromRequest(r *http.Request) *User {
	if u, ok := r.Context().Value(ContextKeyUser).(*User); ok {
		return u
	}
	return nil
}
