package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStore returns a Store backed by in-memory SQLite with all tables migrated.
// Connection pool is limited to 1 so every query hits the same :memory: database.
func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore("sqlite", ":memory:")
	require.NoError(t, err, "failed to create store")

	require.NoError(t, store.AutoMigrateAuthProviders(), "migrate auth_providers")
	require.NoError(t, store.AutoMigrateRoles(), "migrate role_defs")

	sqlDB, err := store.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	t.Cleanup(func() { _ = store.Close() })
	return store
}

// ---------------------------------------------------------------------------
// TestStore_UserCRUD — Create→GetByUsername→GetByID→Update→Delete
// ---------------------------------------------------------------------------

func TestStore_UserCRUD(t *testing.T) {
	store := testStore(t)

	// Create
	u := &User{
		Username:     "alice",
		Email:        "alice@example.com",
		DisplayName:  "Alice",
		PasswordHash: "$2a$10$xyz",
		Role:         "admin",
		Provider:     "local",
	}
	require.NoError(t, store.CreateUser(u))
	assert.NotZero(t, u.ID, "ID should be set after create")

	// GetByUsername
	got, err := store.GetUserByUsername("alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, "admin", got.Role)

	// GetByID
	gotByID, err := store.GetUserByID(u.ID)
	require.NoError(t, err)
	assert.Equal(t, got.Username, gotByID.Username)

	// Update
	gotByID.DisplayName = "Alice Updated"
	gotByID.Role = "operator"
	require.NoError(t, store.UpdateUser(gotByID))

	updated, err := store.GetUserByUsername("alice")
	require.NoError(t, err)
	assert.Equal(t, "Alice Updated", updated.DisplayName)
	assert.Equal(t, "operator", updated.Role)

	// Delete
	require.NoError(t, store.DeleteUser(u.ID))
	_, err = store.GetUserByUsername("alice")
	assert.Error(t, err, "should not find deleted user")
}

// ---------------------------------------------------------------------------
// TestStore_GetUserByProvider — provider + providerID lookup
// ---------------------------------------------------------------------------

func TestStore_GetUserByProvider(t *testing.T) {
	store := testStore(t)

	u := &User{
		Username:   "ldapuser",
		Provider:   "ldap",
		ProviderID: "uid=jdoe,ou=users,dc=example,dc=com",
		Role:       "viewer",
	}
	require.NoError(t, store.CreateUser(u))

	// Found
	got, err := store.GetUserByProvider("ldap", "uid=jdoe,ou=users,dc=example,dc=com")
	require.NoError(t, err)
	assert.Equal(t, "ldapuser", got.Username)
	assert.Equal(t, "ldap", got.Provider)

	// Wrong providerID → not found
	_, err = store.GetUserByProvider("ldap", "wrong")
	assert.Error(t, err)

	// Wrong provider → not found
	_, err = store.GetUserByProvider("oidc", "uid=jdoe,ou=users,dc=example,dc=com")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// TestStore_ListUsers — multiple users ordered by created_at DESC
// ---------------------------------------------------------------------------

func TestStore_ListUsers(t *testing.T) {
	store := testStore(t)

	// Create 3 users with slight time gaps
	users := []*User{
		{Username: "alpha", Role: "admin"},
		{Username: "beta", Role: "viewer"},
		{Username: "gamma", Role: "operator"},
	}
	for _, u := range users {
		require.NoError(t, store.CreateUser(u))
	}

	list, err := store.ListUsers()
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Ordered by created_at DESC (most recent first)
	// gamma was created last, so should be first
	assert.Equal(t, "gamma", list[0].Username)
	assert.Equal(t, "beta", list[1].Username)
	assert.Equal(t, "alpha", list[2].Username)
}

// ---------------------------------------------------------------------------
// TestStore_CountUsers — counting
// ---------------------------------------------------------------------------

func TestStore_CountUsers(t *testing.T) {
	store := testStore(t)

	count, err := store.CountUsers()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	require.NoError(t, store.CreateUser(&User{Username: "u1"}))
	require.NoError(t, store.CreateUser(&User{Username: "u2"}))
	require.NoError(t, store.CreateUser(&User{Username: "u3"}))

	count, err = store.CountUsers()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Delete one
	_ = store.DeleteUser(1)
	count, err = store.CountUsers()
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

// ---------------------------------------------------------------------------
// TestStore_AuthProviderCRUD — Create→Get→List→Update→Delete
// ---------------------------------------------------------------------------

func TestStore_AuthProviderCRUD(t *testing.T) {
	store := testStore(t)

	// Create
	p := &AuthProvider{
		Name:        "company-ldap",
		Type:        ProviderTypeLDAP,
		DisplayName: "Company LDAP",
		Icon:        "ldap",
		Enabled:     true,
		Priority:    10,
	}
	require.NoError(t, store.CreateAuthProvider(p))
	assert.NotZero(t, p.ID)

	// Get by name
	got, err := store.GetAuthProvider("company-ldap")
	require.NoError(t, err)
	assert.Equal(t, "Company LDAP", got.DisplayName)
	assert.True(t, got.Enabled)

	// Get by ID
	gotByID, err := store.GetAuthProviderByID(p.ID)
	require.NoError(t, err)
	assert.Equal(t, got.Name, gotByID.Name)

	// Update
	got.Enabled = false
	got.Priority = 20
	require.NoError(t, store.UpdateAuthProvider(got))

	updated, err := store.GetAuthProvider("company-ldap")
	require.NoError(t, err)
	assert.False(t, updated.Enabled)
	assert.Equal(t, 20, updated.Priority)

	// Delete
	require.NoError(t, store.DeleteAuthProvider(p.ID))
	_, err = store.GetAuthProvider("company-ldap")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// TestStore_GetEnabledProvidersByType — filter by type + enabled, ordered by priority
// ---------------------------------------------------------------------------

func TestStore_GetEnabledProvidersByType(t *testing.T) {
	store := testStore(t)

	// GORM default:true on Enabled means false zero-value gets overridden on Create.
	// So we create all providers (defaulting to enabled), then disable ldap-2 via Update.
	for _, p := range []AuthProvider{
		{Name: "ldap-1", Type: ProviderTypeLDAP, Priority: 10},
		{Name: "ldap-2", Type: ProviderTypeLDAP, Priority: 5},
		{Name: "ldap-3", Type: ProviderTypeLDAP, Priority: 1},
		{Name: "oidc-1", Type: ProviderTypeOIDC, Priority: 0},
		{Name: "oidc-2", Type: ProviderTypeOIDC, Priority: 100},
	} {
		require.NoError(t, store.CreateAuthProvider(&p))
	}

	// Must use Update to set Enabled=false (struct-based Create skips zero-value bools)
	require.NoError(t, store.db.Model(&AuthProvider{}).Where("name = ?", "ldap-2").Update("enabled", false).Error)

	// Enabled LDAP providers — should get ldap-3 (priority 1) then ldap-1 (priority 10)
	// ldap-2 is disabled, should be excluded
	ldapEnabled, err := store.GetEnabledProvidersByType(ProviderTypeLDAP)
	require.NoError(t, err)
	assert.Len(t, ldapEnabled, 2)
	assert.Equal(t, "ldap-3", ldapEnabled[0].Name, "lower priority first")
	assert.Equal(t, "ldap-1", ldapEnabled[1].Name)

	// Enabled OIDC providers — oidc-1 (priority 0) then oidc-2 (priority 100)
	oidcEnabled, err := store.GetEnabledProvidersByType(ProviderTypeOIDC)
	require.NoError(t, err)
	assert.Len(t, oidcEnabled, 2)
	assert.Equal(t, "oidc-1", oidcEnabled[0].Name)
	assert.Equal(t, "oidc-2", oidcEnabled[1].Name)

	// No matching results for unknown type
	none, err := store.GetEnabledProvidersByType("saml")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// ---------------------------------------------------------------------------
// TestStore_RoleDefCRUD — Create + Get + List + Builtin protection
// ---------------------------------------------------------------------------

func TestStore_RoleDefCRUD(t *testing.T) {
	store := testStore(t)

	// Create custom role
	role := &RoleDef{
		Name:        "devops",
		DisplayName: "DevOps Engineer",
		Description: "CI/CD + deploy access",
		Group:       "k8ops:devops",
		Scope:       "cluster",
	}
	require.NoError(t, store.CreateRoleDef(role))
	assert.NotZero(t, role.ID)

	// Get
	got, err := store.GetRoleDef("devops")
	require.NoError(t, err)
	assert.Equal(t, "DevOps Engineer", got.DisplayName)
	assert.Equal(t, "k8ops:devops", got.Group)

	// List
	list, err := store.ListRoleDefs()
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Create a builtin role for deletion test
	builtin := &RoleDef{
		Name:    "admin",
		Group:   "k8ops:admin",
		Scope:   "cluster",
		Builtin: true,
	}
	require.NoError(t, store.CreateRoleDef(builtin))

	// Delete custom role — should succeed
	require.NoError(t, store.DeleteRoleDef("devops"))
	_, err = store.GetRoleDef("devops")
	assert.Error(t, err, "custom role should be deleted")

	// Delete builtin role — should fail
	err = store.DeleteRoleDef("admin")
	assert.ErrorIs(t, err, ErrBuiltinRoleProtected, "builtin role should be protected")

	// Builtin role should still exist
	stillThere, err := store.GetRoleDef("admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", stillThere.Name)
}

// ---------------------------------------------------------------------------
// TestStore_DuplicateUsername — unique index constraint
// ---------------------------------------------------------------------------

func TestStore_DuplicateUsername(t *testing.T) {
	store := testStore(t)

	u1 := &User{Username: "dup", Email: "first@example.com"}
	require.NoError(t, store.CreateUser(u1))

	// Second user with same username → error
	u2 := &User{Username: "dup", Email: "second@example.com"}
	err := store.CreateUser(u2)
	assert.Error(t, err, "duplicate username should error")

	// SQLite returns "UNIQUE constraint failed" — check error message
	// (gorm.ErrDuplicatedKey is not reliably translated by the SQLite driver)
	assert.Contains(t, err.Error(), "UNIQUE constraint")

	// Verify only 1 user exists
	count, _ := store.CountUsers()
	assert.Equal(t, int64(1), count)

	// Original user should be unchanged
	got, err := store.GetUserByUsername("dup")
	require.NoError(t, err)
	assert.Equal(t, "first@example.com", got.Email)
}

// ---------------------------------------------------------------------------
// Table-driven edge cases for Store operations
// ---------------------------------------------------------------------------

func TestStore_GetUserByUsername_NotFound(t *testing.T) {
	store := testStore(t)

	tests := []struct {
		name     string
		username string
	}{
		{"empty", ""},
		{"nonexistent", "ghost"},
		{"with spaces", "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.GetUserByUsername(tt.username)
			assert.Error(t, err)
		})
	}
}

func TestStore_GetUserByID_NotFound(t *testing.T) {
	store := testStore(t)

	tests := []struct {
		name string
		id   uint
	}{
		{"zero", 0},
		{"nonexistent", 9999},
		{"large", 999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.GetUserByID(tt.id)
			assert.Error(t, err)
		})
	}
}

func TestStore_GetUserByProvider_NotFound(t *testing.T) {
	store := testStore(t)

	tests := []struct {
		name       string
		provider   string
		providerID string
	}{
		{"both empty", "", ""},
		{"empty provider", "", "some-id"},
		{"empty id", "ldap", ""},
		{"nonexistent", "saml", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.GetUserByProvider(tt.provider, tt.providerID)
			assert.Error(t, err)
		})
	}
}

func TestStore_DeleteUser_NotFound(t *testing.T) {
	store := testStore(t)

	// DeleteUser now returns ErrUserNotFound when no rows are affected.
	err := store.DeleteUser(9999)
	assert.Error(t, err, "deleting non-existent user should return error")

	// Verify the count is still 0
	count, err := store.CountUsers()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestStore_UpdateUser_NoChange(t *testing.T) {
	store := testStore(t)

	u := &User{Username: "update-test", Role: "viewer", Provider: "local"}
	require.NoError(t, store.CreateUser(u))

	require.NoError(t, store.UpdateUser(u))

	got, err := store.GetUserByUsername("update-test")
	require.NoError(t, err)
	assert.Equal(t, "viewer", got.Role)
}

func TestStore_ListUsers_Empty(t *testing.T) {
	store := testStore(t)

	list, err := store.ListUsers()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_Close_DoubleClose(t *testing.T) {
	store, err := NewStore("sqlite", ":memory:")
	require.NoError(t, err)

	require.NoError(t, store.Close())
	_ = store.Close()
}

func TestStore_RoleBinding_CRUD(t *testing.T) {
	store := testStore(t)

	role := &RoleDef{Name: "rb-test", Group: "k8ops:rb-test", Scope: "cluster"}
	require.NoError(t, store.CreateRoleDef(role))

	binding := &RoleBindingDef{
		RoleName:    "rb-test",
		K8sRoleKind: "ClusterRole",
		K8sRoleName: "cluster-admin",
	}
	require.NoError(t, store.AddRoleBinding(binding))
	assert.NotZero(t, binding.ID)

	bindings, err := store.ListRoleBindings("rb-test")
	require.NoError(t, err)
	assert.Len(t, bindings, 1)
	assert.Equal(t, "cluster-admin", bindings[0].K8sRoleName)

	allBindings, err := store.ListRoleBindings("")
	require.NoError(t, err)
	assert.Len(t, allBindings, 1)

	binding2 := &RoleBindingDef{
		RoleName:    "rb-test",
		K8sRoleKind: "Role",
		K8sRoleName: "pod-reader",
		Namespace:   "default",
	}
	require.NoError(t, store.AddRoleBinding(binding2))

	bindings, err = store.ListRoleBindings("rb-test")
	require.NoError(t, err)
	assert.Len(t, bindings, 2)

	require.NoError(t, store.RemoveRoleBinding(binding.ID))
	bindings, err = store.ListRoleBindings("rb-test")
	require.NoError(t, err)
	assert.Len(t, bindings, 1)
	assert.Equal(t, "pod-reader", bindings[0].K8sRoleName)

	err = store.RemoveRoleBinding(99999)
	assert.NoError(t, err)
}

func TestStore_SeedBuiltinRoles(t *testing.T) {
	store := testStore(t)

	require.NoError(t, store.SeedBuiltinRoles())

	roles, err := store.ListRoleDefs()
	require.NoError(t, err)
	assert.Len(t, roles, 5, "should have exactly 5 builtin roles")

	assert.Equal(t, "admin", roles[0].Name)
	assert.Equal(t, "ns-admin", roles[1].Name)
	assert.Equal(t, "ns-viewer", roles[2].Name)
	assert.Equal(t, "operator", roles[3].Name)
	assert.Equal(t, "viewer", roles[4].Name)

	for _, r := range roles {
		assert.True(t, r.Builtin, "role %s should be builtin", r.Name)
	}

	require.NoError(t, store.SeedBuiltinRoles())
	roles, err = store.ListRoleDefs()
	require.NoError(t, err)
	assert.Len(t, roles, 5, "should still have 5 builtin roles after re-seed")
}

func TestStore_AuthProvider_GetByName_NotFound(t *testing.T) {
	store := testStore(t)

	tests := []struct {
		name  string
		qname string
	}{
		{"empty", ""},
		{"nonexistent", "ghost-provider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.GetAuthProvider(tt.qname)
			assert.Error(t, err)
		})
	}
}

func TestStore_AuthProvider_GetByID_NotFound(t *testing.T) {
	store := testStore(t)

	_, err := store.GetAuthProviderByID(99999)
	assert.Error(t, err)
}

func TestStore_AuthProvider_ListEmpty(t *testing.T) {
	store := testStore(t)

	list, err := store.ListAuthProviders()
	require.NoError(t, err)
	assert.Empty(t, list)
}
