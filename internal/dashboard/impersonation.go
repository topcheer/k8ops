package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ggai/k8ops/internal/auth"
	k8stools "github.com/ggai/k8ops/internal/tools/k8s"
)

// contextKey is a private type for context keys in this package.
type contextKey string

const clientsKey contextKey = "requestClients"

// requestClients holds the per-request Kubernetes clients that respect
// the authenticated user's RBAC permissions via impersonation.
type requestClients struct {
	clientset  kubernetes.Interface // for core K8s resources (nodes, pods, events)
	ctrlClient client.Client        // for CRD resources (diagnostics, remediations)
	restConfig *rest.Config         // for specialized operations (exec, logs streaming)
	user       *auth.User           // the authenticated user, for audit/conditional logic
}

// RoleToGroups maps a k8ops role to the Kubernetes impersonation group(s).
// The K8s API server enforces RBAC based on these groups.
// Built-in roles: admin, operator, viewer, ns-admin, ns-viewer
// Custom roles: loaded from DB at startup, group = "k8ops:<roleName>"
func RoleToGroups(role, allowedNamespaces string) []string {
	switch role {
	case "admin":
		return []string{"k8ops:admin"}
	case "operator":
		return []string{"k8ops:operator"}
	case "viewer":
		return []string{"k8ops:viewer"}
	case "ns-admin":
		groups := []string{}
		for _, ns := range splitNamespaces(allowedNamespaces) {
			groups = append(groups, "k8ops:ns-admin:"+ns)
		}
		return groups
	case "ns-viewer":
		groups := []string{}
		for _, ns := range splitNamespaces(allowedNamespaces) {
			groups = append(groups, "k8ops:ns-viewer:"+ns)
		}
		return groups
	default:
		// Custom role: group is k8ops:<roleName>
		// For namespace-scoped custom roles, append namespace suffix
		if role != "" {
			return []string{"k8ops:" + role}
		}
		return []string{"k8ops:viewer"} // default to safest
	}
}

func splitNamespaces(s string) []string {
	parts := strings.Split(s, ",")
	result := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ImpersonationMiddleware creates per-request K8s clients with impersonation
// headers based on the authenticated user's role. This ensures the K8s API
// server enforces RBAC per user.
func (s *Server) ImpersonationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromRequest(r)

		// If no user (auth disabled or public route), use server's default clients
		if user == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Create impersonated rest config
		// Use AnonymousClientConfig to clear cached transport, then restore SA credentials.
		// CopyConfig alone shares the cached transport, causing impersonation headers to leak between requests.
		impersonatedConfig := rest.AnonymousClientConfig(s.restConfig)
		impersonatedConfig.BearerToken = s.restConfig.BearerToken
		impersonatedConfig.BearerTokenFile = s.restConfig.BearerTokenFile
		impersonatedConfig.Impersonate = rest.ImpersonationConfig{
			UserName: user.Username,
			Groups:   RoleToGroups(user.Role, user.AllowedNamespaces),
		}

		s.log.Info("impersonation config",
			"user", user.Username,
			"role", user.Role,
			"ns", user.AllowedNamespaces,
			"groups", impersonatedConfig.Impersonate.Groups)

		// Create clientset with impersonation
		impClientset, err := kubernetes.NewForConfig(impersonatedConfig)
		if err != nil {
			s.log.Error("failed to create impersonated clientset", "user", user.Username, "error", err)
			// Fall back to default client
			next.ServeHTTP(w, r)
			return
		}

		// Create controller-runtime client with impersonation
		impCtrlClient, err := client.New(impersonatedConfig, client.Options{Scheme: s.scheme})
		if err != nil {
			s.log.Error("failed to create impersonated ctrl client", "user", user.Username, "error", err)
			next.ServeHTTP(w, r)
			return
		}

		rc := &requestClients{
			clientset:  impClientset,
			ctrlClient: impCtrlClient,
			restConfig: impersonatedConfig,
			user:       user,
		}

		ctx := context.WithValue(r.Context(), clientsKey, rc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// clientsFromReq returns the per-request clients if available (auth enabled),
// otherwise falls back to the server's shared clients (auth disabled or public route).
func (s *Server) clientsFromReq(r *http.Request) *requestClients {
	if v := r.Context().Value(clientsKey); v != nil {
		return v.(*requestClients)
	}
	// Fallback: use server's shared clients
	return &requestClients{
		clientset:  s.clientset,
		ctrlClient: s.k8sClient,
		restConfig: s.restConfig,
		user:       nil,
	}
}

// CurrentUser returns the authenticated user from request context, or nil.
func CurrentUser(r *http.Request) *auth.User {
	return auth.UserFromRequest(r)
}

// CanEdit returns true if the current user has write permissions.
func CanEdit(r *http.Request) bool {
	u := auth.UserFromRequest(r)
	if u == nil {
		return true // auth disabled
	}
	return u.Role == "admin" || u.Role == "operator" || u.Role == "ns-admin"
}

// CanManage returns true if the current user can manage cluster-wide settings.
func CanManage(r *http.Request) bool {
	u := auth.UserFromRequest(r)
	if u == nil {
		return true // auth disabled
	}
	return u.Role == "admin"
}

// DescribeRoleForLog returns a human-readable description for logging.
func DescribeRoleForLog(u *auth.User) string {
	if u == nil {
		return "anonymous"
	}
	ns := u.AllowedNamespaces
	if ns != "" {
		return fmt.Sprintf("%s(%s)", u.Role, ns)
	}
	return u.Role
}

// ImpersonatedKubeClient creates a KubeClient with impersonation headers
// matching the authenticated user's RBAC role. Used by chat tools.
func (s *Server) ImpersonatedKubeClient(r *http.Request) *k8stools.KubeClient {
	user := auth.UserFromRequest(r)
	if user == nil {
		return s.k8sClientTool // fallback to shared client (auth disabled)
	}

	impersonatedConfig := rest.AnonymousClientConfig(s.restConfig)
	impersonatedConfig.BearerToken = s.restConfig.BearerToken
	impersonatedConfig.BearerTokenFile = s.restConfig.BearerTokenFile
	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: user.Username,
		Groups:   RoleToGroups(user.Role, user.AllowedNamespaces),
	}

	kc, err := k8stools.NewKubeClientFromConfig(impersonatedConfig)
	if err != nil {
		s.log.Error("failed to create impersonated kube client for chat", "user", user.Username, "error", err)
		return s.k8sClientTool
	}
	return kc
}
