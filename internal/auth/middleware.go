package auth

import (
	"net/http"
	"strings"
)

// AuthMiddleware protects routes that require authentication.
// It supports dual-mode authentication:
//   1. Session cookie (k8ops_token) for browser users
//   2. Bearer token (Authorization: Bearer k8ops_xxx) for CLI/API integrations
//
// Public routes (/api/health, /api/auth/*) are exempted.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public routes — no auth required
		if isPublicRoute(path) {
			next.ServeHTTP(w, r)
			return
		}

		// Try Bearer token first (Authorization: Bearer k8ops_xxx)
		if authHeader := r.Header.Get("Authorization"); authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				user, err := a.ValidateAPIKey(token)
				if err != nil {
					http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
					return
				}
				ctx := SetUserInContext(r.Context(), user)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Fall back to session cookie
		cookie, err := r.Cookie("k8ops_token")
		if err != nil {
			if isAPIRequest(path) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// Redirect browser to login page
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}

		claims, err := a.VerifyToken(cookie.Value)
		if err != nil {
			clearAuthCookie(w)
			if isAPIRequest(path) {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}

		// Add user info to context
		user, err := a.store.GetUserByID(claims.UserID)
		if err != nil {
			clearAuthCookie(w)
			if isAPIRequest(path) {
				http.Error(w, `{"error":"user not found"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}

		ctx := SetUserInContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminOnly wraps a handler to require admin role.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromRequest(r)
		if user == nil || user.Role != "admin" {
			http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isPublicRoute returns true for routes that don't require authentication.
func isPublicRoute(path string) bool {
	// Static assets (.css, .js, .ico, images) are always public
	if strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".ico") || strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".woff2") {
		return true
	}
	// Login page is always public
	if path == "/login.html" || path == "/favicon.ico" {
		return true
	}
	// Auth API endpoints (login, logout, status, oidc, presets)
	if strings.HasPrefix(path, "/api/auth/login") || strings.HasPrefix(path, "/api/auth/logout") ||
		strings.HasPrefix(path, "/api/auth/status") || strings.HasPrefix(path, "/api/auth/oidc") ||
		strings.HasPrefix(path, "/api/auth/callback") || strings.HasPrefix(path, "/api/auth/provider-presets") {
		return true
	}
	// Health check
	if path == "/api/health" {
		return true
	}
	return false
}

// isAPIRequest returns true for JSON API routes.
func isAPIRequest(path string) bool {
	return strings.HasPrefix(path, "/api/")
}

// SetAuthCookie sets the JWT token in an HttpOnly cookie.
func SetAuthCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "k8ops_token",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   false, // set to true behind HTTPS ingress in production
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookie removes the auth cookie.
func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "k8ops_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}
