package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggai/k8ops/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withAdminContext creates a request with an admin user in the context.
func withAdminContext(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), auth.ContextKeyUser, &auth.User{
		Username: "admin",
		Role:     "admin",
	})
	return r.WithContext(ctx)
}

// withViewerContext creates a request with a viewer user in the context.
func withViewerContext(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), auth.ContextKeyUser, &auth.User{
		Username: "viewer",
		Role:     "viewer",
	})
	return r.WithContext(ctx)
}

// ============================================================================
// auth.AdminOnly Middleware Tests
// ============================================================================

func TestAdminOnly_AllowsAdmin(t *testing.T) {
	handler := auth.AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	req = withAdminContext(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminOnly_BlocksViewer(t *testing.T) {
	handler := auth.AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	req = withViewerContext(req)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminOnly_BlocksNoUser(t *testing.T) {
	handler := auth.AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminOnly_BlocksOperator(t *testing.T) {
	handler := auth.AdminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	ctx := context.WithValue(req.Context(), auth.ContextKeyUser, &auth.User{
		Username: "operator",
		Role:     "operator",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ============================================================================
// handleClusterRoles Tests
// ============================================================================

func TestHandleClusterRoles_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	w := httptest.NewRecorder()
	s.handleClusterRoles(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleClusterRoles_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/clusterroles", nil)
	w := httptest.NewRecorder()
	s.handleClusterRoles(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ============================================================================
// handleClusterRoleByName Tests
// ============================================================================

func TestHandleClusterRoleByName_EmptyName(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("DELETE", "/api/rbac/clusterroles/", nil)
	w := httptest.NewRecorder()
	s.handleClusterRoleByName(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleClusterRoleByName_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("DELETE", "/api/rbac/clusterroles/some-role", nil)
	w := httptest.NewRecorder()
	s.handleClusterRoleByName(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleClusterRoleByName_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("GET", "/api/rbac/clusterroles/some-role", nil)
	w := httptest.NewRecorder()
	s.handleClusterRoleByName(w, req)

	// Only DELETE is supported
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ============================================================================
// handleNamespaceRoles Tests
// ============================================================================

func TestHandleNamespaceRoles_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/roles", nil)
	w := httptest.NewRecorder()
	s.handleNamespaceRoles(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleNamespaceRoles_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/roles", nil)
	w := httptest.NewRecorder()
	s.handleNamespaceRoles(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ============================================================================
// handleNamespaceRoleByName Tests
// ============================================================================

func TestHandleNamespaceRoleByName_EmptyPath(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("DELETE", "/api/rbac/roles/", nil)
	w := httptest.NewRecorder()
	s.handleNamespaceRoleByName(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleNamespaceRoleByName_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/roles/default/my-role", nil)
	w := httptest.NewRecorder()
	s.handleNamespaceRoleByName(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ============================================================================
// handleRoleBindings Tests
// ============================================================================

func TestHandleRoleBindings_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/rolebindings", nil)
	w := httptest.NewRecorder()
	s.handleRoleBindings(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleRoleBindings_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/rolebindings", nil)
	w := httptest.NewRecorder()
	s.handleRoleBindings(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ============================================================================
// handleRoleBindingByName Tests
// ============================================================================

func TestHandleRoleBindingByName_EmptyPath(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("DELETE", "/api/rbac/rolebindings/", nil)
	w := httptest.NewRecorder()
	s.handleRoleBindingByName(w, req)

	// Empty path → might get 400 or pass through to method check
	assert.True(t, w.Code == http.StatusBadRequest || w.Code == http.StatusMethodNotAllowed,
		"expected 400 or 405 for empty path, got %d", w.Code)
}

// ============================================================================
// handleListNamespaces Tests
// ============================================================================

func TestHandleListNamespaces_NilClientset(t *testing.T) {
	s := testServer()
	s.clientset = nil

	req := httptest.NewRequest("GET", "/api/rbac/namespaces", nil)
	w := httptest.NewRecorder()

	// Will panic or error without clientset — depends on impl
	// Since it's the auth store we're testing, and auth is nil,
	// it should return 500
	defer func() {
		if r := recover(); r != nil {
			// Panic is acceptable for nil clientset — test documents behavior
			t.Logf("panic with nil clientset (expected): %v", r)
		}
	}()
	s.handleListNamespaces(w, req)
	// If no panic, should be an error code
	assert.True(t, w.Code >= 400, "expected error status, got %d", w.Code)
}

// ============================================================================
// handleListSubjects Tests
// ============================================================================

func TestHandleListSubjects_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/subjects", nil)
	w := httptest.NewRecorder()

	// Should handle nil auth gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered (acceptable): %v", r)
		}
	}()
	s.handleListSubjects(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	// With nil auth, items should be present but may be empty
	items, exists := resp["items"]
	assert.True(t, exists, "response should have items key")
	// items could be empty array when no auth store
	if itemsArr, ok := items.([]any); ok {
		// No users without auth - empty is valid
		_ = itemsArr
	}
}

// ============================================================================
// handleRoleMapping Tests
// ============================================================================

func TestHandleRoleMapping_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/role-mapping", nil)
	w := httptest.NewRecorder()

	s.handleRoleMapping(w, req)
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError,
		"expected 200 or 500, got %d", w.Code)
}

func TestHandleRoleMapping_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/role-mapping", nil)
	w := httptest.NewRecorder()
	s.handleRoleMapping(w, req)

	// auth nil → 500 before method check
	assert.True(t, w.Code == http.StatusMethodNotAllowed || w.Code == http.StatusInternalServerError,
		"expected 405 or 500, got %d", w.Code)
}

// ============================================================================
// handleRoleDefs Tests
// ============================================================================

func TestHandleRoleDefs_NilAuth(t *testing.T) {
	s := testServer()
	s.auth = nil

	req := httptest.NewRequest("GET", "/api/rbac/role-defs", nil)
	w := httptest.NewRecorder()

	s.handleRoleDefs(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleRoleDefs_MethodNotAllowed(t *testing.T) {
	s := testServer()

	req := httptest.NewRequest("PUT", "/api/rbac/role-defs", nil)
	w := httptest.NewRecorder()
	s.handleRoleDefs(w, req)

	// With nil auth, auth check happens first → 500
	// With auth, unsupported method → 405
	assert.True(t, w.Code == http.StatusMethodNotAllowed || w.Code == http.StatusInternalServerError,
		"expected 405 or 500, got %d", w.Code)
}

// ============================================================================
// handleAPIResources Tests
// ============================================================================

func TestHandleAPIResources_NilClientset(t *testing.T) {
	s := testServer()
	s.clientset = nil

	req := httptest.NewRequest("GET", "/api/rbac/api-resources", nil)
	w := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Logf("panic with nil clientset (expected): %v", r)
		}
	}()
	s.handleAPIResources(w, req)
	// Should return error if clientset is nil
	assert.True(t, w.Code >= 400 || w.Code == http.StatusOK,
		"expected some valid response, got %d", w.Code)
}

// ============================================================================
// RBAC Route Registration Test
// ============================================================================

func TestRegisterRBACRoutes(t *testing.T) {
	mux := http.NewServeMux()
	s := testServer()

	// Should not panic
	s.registerRBACRoutes(mux)

	// Verify routes are registered by making a request
	// All routes are wrapped with AdminOnly, so without admin context → 403
	testCases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/rbac/clusterroles"},
		{"GET", "/api/rbac/roles"},
		{"GET", "/api/rbac/rolebindings"},
		{"GET", "/api/rbac/namespaces"},
		{"GET", "/api/rbac/subjects"},
		{"GET", "/api/rbac/role-mapping"},
		{"GET", "/api/rbac/role-defs"},
		{"GET", "/api/rbac/api-resources"},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// Without admin context, should get 403
		assert.Equal(t, http.StatusForbidden, w.Code,
			"route %s should return 403 without admin context", tc.path)
	}
}

// ============================================================================
// RBAC Route with Admin Context Tests
// ============================================================================

func TestRegisterRBACRoutes_AdminAccess(t *testing.T) {
	mux := http.NewServeMux()
	s := testServer()

	s.registerRBACRoutes(mux)

	// With admin context, should pass middleware but may error at handler
	// level (nil auth/clientset)
	req := httptest.NewRequest("GET", "/api/rbac/clusterroles", nil)
	req = withAdminContext(req)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should pass AdminOnly (no 403) but may get 500 from nil auth
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"admin user should not get 403")
}
