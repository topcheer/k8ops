package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ggai/k8ops/internal/auth"
	corev1 "k8s.io/api/core/v1"
)

// registerRBACRoutes wires up all RBAC-related HTTP endpoints.
// All routes require admin role — wrapped with auth.AdminOnly middleware.
func (s *Server) registerRBACRoutes(mux *http.ServeMux) {
	admin := auth.AdminOnly

	// ClusterRoles
	mux.Handle("/api/rbac/clusterroles", admin(http.HandlerFunc(s.handleClusterRoles)))
	mux.Handle("/api/rbac/clusterroles/", admin(http.HandlerFunc(s.handleClusterRoleByName)))

	// Namespace-scoped Roles
	mux.Handle("/api/rbac/roles", admin(http.HandlerFunc(s.handleNamespaceRoles)))
	mux.Handle("/api/rbac/roles/", admin(http.HandlerFunc(s.handleNamespaceRoleByName)))

	// RoleBindings
	mux.Handle("/api/rbac/rolebindings", admin(http.HandlerFunc(s.handleRoleBindings)))
	mux.Handle("/api/rbac/rolebindings/", admin(http.HandlerFunc(s.handleRoleBindingByName)))

	// Namespaces + Subjects (for dropdowns)
	mux.Handle("/api/rbac/namespaces", admin(http.HandlerFunc(s.handleListNamespaces)))
	mux.Handle("/api/rbac/subjects", admin(http.HandlerFunc(s.handleListSubjects)))

	// Role Mapping (k8ops role -> K8s roles)
	mux.Handle("/api/rbac/role-mapping", admin(http.HandlerFunc(s.handleRoleMapping)))

	// Custom Role Definitions
	mux.Handle("/api/rbac/role-defs", admin(http.HandlerFunc(s.handleRoleDefs)))

	// API Resource discovery (for WYSIWYG rule editor)
	mux.Handle("/api/rbac/api-resources", admin(http.HandlerFunc(s.handleAPIResources)))
}

// --- ClusterRole handlers ---

type clusterRoleSummary struct {
	Name      string   `json:"name"`
	RuleCount int      `json:"ruleCount"`
	Scope     string   `json:"scope"`
	Builtin   bool     `json:"builtin"`
	Bindings  []string `json:"bindings"`
}

type createClusterRoleReq struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
}

func (s *Server) handleClusterRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listClusterRoles(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.createClusterRole(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) listClusterRoles(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	roles, err := s.auth.Store().ListRoleDefs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	result := make([]clusterRoleSummary, 0)
	for _, role := range roles {
		if role.Scope == "cluster" {
			result = append(result, clusterRoleSummary{
				Name:      role.Name,
				RuleCount: len(role.Description),
				Scope:     role.Scope,
				Builtin:   role.Builtin,
			})
		}
	}
	writeJSON(w, map[string]any{"items": result})
}

func (s *Server) createClusterRole(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	var req createClusterRoleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	role := &auth.RoleDef{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Group:       fmt.Sprintf("k8ops:%s", req.Name),
		Scope:       "cluster",
	}
	if err := s.auth.Store().CreateRoleDef(role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create role")
		return
	}
	writeJSON(w, role)
}

func (s *Server) handleClusterRoleByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/rbac/clusterroles/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "role name is required")
		return
	}
	if r.Method == http.MethodDelete {
		if s.auth == nil {
			writeError(w, http.StatusInternalServerError, "auth not initialized")
			return
		}
		if err := s.auth.Store().DeleteRoleDef(name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete role")
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// --- Namespace Role handlers ---

func (s *Server) handleNamespaceRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listNamespaceRoles(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.createNamespaceRole(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) listNamespaceRoles(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	roles, err := s.auth.Store().ListRoleDefs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	result := make([]clusterRoleSummary, 0)
	for _, role := range roles {
		if role.Scope == "namespace" {
			result = append(result, clusterRoleSummary{
				Name:      role.Name,
				RuleCount: len(role.Description),
				Scope:     role.Scope,
				Builtin:   role.Builtin,
			})
		}
	}
	writeJSON(w, map[string]any{"items": result})
}

func (s *Server) createNamespaceRole(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	var req createClusterRoleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	role := &auth.RoleDef{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Group:       fmt.Sprintf("k8ops:%s", req.Name),
		Scope:       "namespace",
	}
	if err := s.auth.Store().CreateRoleDef(role); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create role")
		return
	}
	writeJSON(w, role)
}

func (s *Server) handleNamespaceRoleByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/rbac/roles/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "role name is required")
		return
	}
	if r.Method == http.MethodDelete {
		if s.auth == nil {
			writeError(w, http.StatusInternalServerError, "auth not initialized")
			return
		}
		if err := s.auth.Store().DeleteRoleDef(name); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete role")
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// --- RoleBinding handlers ---

type bindingSummary struct {
	ID          uint   `json:"id"`
	RoleName    string `json:"roleName"`
	K8sRoleName string `json:"k8sRoleName"`
	Namespace   string `json:"namespace,omitempty"`
}

type addBindingReq struct {
	RoleName    string `json:"roleName"`
	K8sRoleKind string `json:"k8sRoleKind"`
	K8sRoleName string `json:"k8sRoleName"`
	Namespace   string `json:"namespace,omitempty"`
}

func (s *Server) handleRoleBindings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listRoleBindings(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.createRoleBinding(w, r)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) listRoleBindings(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	bindings, err := s.auth.Store().ListRoleBindings("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list bindings")
		return
	}
	result := make([]bindingSummary, 0, len(bindings))
	for _, b := range bindings {
		result = append(result, bindingSummary{
			ID:          b.ID,
			RoleName:    b.RoleName,
			K8sRoleName: b.K8sRoleName,
			Namespace:   b.Namespace,
		})
	}
	writeJSON(w, map[string]any{"items": result})
}

func (s *Server) createRoleBinding(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	var req addBindingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RoleName == "" || req.K8sRoleName == "" {
		writeError(w, http.StatusBadRequest, "roleName and k8sRoleName required")
		return
	}
	binding := &auth.RoleBindingDef{
		RoleName:    req.RoleName,
		K8sRoleKind: req.K8sRoleKind,
		K8sRoleName: req.K8sRoleName,
		Namespace:   req.Namespace,
	}
	if err := s.auth.Store().AddRoleBinding(binding); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create binding")
		return
	}
	writeJSON(w, binding)
}

func (s *Server) handleRoleBindingByName(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/rbac/rolebindings/")
	if r.Method == http.MethodDelete && idStr != "" {
		if s.auth == nil {
			writeError(w, http.StatusInternalServerError, "auth not initialized")
			return
		}
		// Try to parse as uint
		var id uint
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil && id > 0 {
			if err := s.auth.Store().RemoveRoleBinding(id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to delete binding")
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
			return
		}
		writeError(w, http.StatusBadRequest, "invalid binding ID")
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// --- Namespace listing ---

func (s *Server) handleListNamespaces(w http.ResponseWriter, r *http.Request) {
	nsList := &corev1.NamespaceList{}
	if err := s.k8sClient.List(r.Context(), nsList); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list namespaces")
		return
	}
	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)
	writeJSON(w, map[string]any{"items": names})
}

// --- Subject listing ---

func (s *Server) handleListSubjects(w http.ResponseWriter, r *http.Request) {
	users := make([]auth.User, 0)
	if s.auth != nil {
		var err error
		users, err = s.auth.Store().ListUsers()
		if err != nil {
			users = nil
		}
	}
	type subjectInfo struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Provider string `json:"provider"`
	}
	result := make([]subjectInfo, 0, len(users))
	for _, u := range users {
		result = append(result, subjectInfo{
			ID:       u.ID,
			Username: u.Username,
			Role:     u.Role,
			Provider: u.Provider,
		})
	}
	writeJSON(w, map[string]any{"items": result})
}

// --- Role Mapping ---

func (s *Server) handleRoleMapping(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	roles, err := s.auth.Store().ListRoleDefs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}
	bindings, err := s.auth.Store().ListRoleBindings("")
	if err != nil {
		bindings = nil
	}
	bindingMap := make(map[string][]bindingSummary)
	for _, b := range bindings {
		bindingMap[b.RoleName] = append(bindingMap[b.RoleName], bindingSummary{
			ID:          b.ID,
			RoleName:    b.RoleName,
			K8sRoleName: b.K8sRoleName,
			Namespace:   b.Namespace,
		})
	}
	type roleMappingEntry struct {
		RoleName string           `json:"roleName"`
		Bindings []bindingSummary `json:"bindings"`
	}
	result := make([]roleMappingEntry, 0, len(roles))
	for _, role := range roles {
		result = append(result, roleMappingEntry{
			RoleName: role.Name,
			Bindings: bindingMap[role.Name],
		})
	}
	writeJSON(w, map[string]any{"items": result})
}

// --- Role Defs (custom role CRUD) ---

func (s *Server) handleRoleDefs(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	switch r.Method {
	case http.MethodGet:
		roles, err := s.auth.Store().ListRoleDefs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list role defs")
			return
		}
		writeJSON(w, map[string]any{"items": roles})
	case http.MethodPost:
		var role auth.RoleDef
		if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if err := s.auth.Store().CreateRoleDef(&role); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create role def")
			return
		}
		writeJSON(w, role)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// k8opsBuiltinRoles defines the built-in role-to-group mapping.
// Used as fallback when the auth database is not available.
var k8opsBuiltinRoles = []struct {
	Role  string
	Group string
	Scope string
}{
	{"admin", "k8ops:admin", "cluster"},
	{"operator", "k8ops:operator", "cluster"},
	{"viewer", "k8ops:viewer", "cluster"},
}

// --- API Resource discovery (for WYSIWYG rule editor) ---

type apiResourceInfo struct {
	Name         string   `json:"name"`
	SingularName string   `json:"singularName"`
	Namespaced   bool     `json:"namespaced"`
	Kind         string   `json:"kind"`
	Group        string   `json:"group,omitempty"`
	Version      string   `json:"version,omitempty"`
	Verbs        []string `json:"verbs"`
	ShortNames   []string `json:"shortNames,omitempty"`
	Categories   []string `json:"categories,omitempty"`
}

func (s *Server) handleAPIResources(w http.ResponseWriter, r *http.Request) {
	resources, err := s.clientset.Discovery().ServerPreferredResources()
	if err != nil {
		// Partial errors may contain useful data
		if len(resources) == 0 {
			writeK8sError(w, err)
			return
		}
	}

	// Flatten group resources
	result := make([]apiResourceInfo, 0, 100)
	for _, gr := range resources {
		group := gr.GroupVersion
		for _, r := range gr.APIResources {
			// Skip subresources (contain "/")
			if contains(r.Name, "/") {
				continue
			}
			result = append(result, apiResourceInfo{
				Name:         r.Name,
				SingularName: r.SingularName,
				Namespaced:   r.Namespaced,
				Kind:         r.Kind,
				Group:        extractGroup(group),
				Version:      extractVersion(group),
				Verbs:        r.Verbs,
				ShortNames:   r.ShortNames,
				Categories:   r.Categories,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	writeJSON(w, map[string]any{"items": result})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

func extractGroup(groupVersion string) string {
	for i := len(groupVersion) - 1; i >= 0; i-- {
		if groupVersion[i] == '/' {
			return groupVersion[:i]
		}
	}
	return ""
}

func extractVersion(groupVersion string) string {
	for i := len(groupVersion) - 1; i >= 0; i-- {
		if groupVersion[i] == '/' {
			return groupVersion[i+1:]
		}
	}
	return groupVersion
}

func isCoreGroup(groupVersion string) bool {
	return groupVersion == "v1"
}
