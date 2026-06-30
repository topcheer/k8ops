package auth

import (
	"time"
)

// RoleDef represents a k8ops role definition (built-in or custom).
// Custom roles are stored in the DB and can be created/edited by admin.
type RoleDef struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"uniqueIndex;size:64;not null" json:"name"` // e.g. "devops", "readonly-dev"
	DisplayName string    `gorm:"size:255" json:"display_name"`
	Description string    `gorm:"size:512" json:"description"`
	Group       string    `gorm:"size:128;not null" json:"group"` // K8s impersonation group, e.g. "k8ops:devops"
	Scope       string    `gorm:"size:16;default:'cluster'" json:"scope"` // "cluster" or "namespace"
	Builtin     bool      `gorm:"default:false" json:"builtin"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName overrides the table name.
func (RoleDef) TableName() string { return "role_defs" }

// RoleBinding represents a binding from a k8ops role to a K8s ClusterRole or Role.
// A role can have multiple bindings — the effective permissions are the union (K8s RBAC additive model).
type RoleBindingDef struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RoleName     string    `gorm:"index;size:64;not null" json:"role_name"`       // references RoleDef.Name
	K8sRoleKind  string    `gorm:"size:16;not null" json:"k8s_role_kind"`         // "ClusterRole" or "Role"
	K8sRoleName  string    `gorm:"size:255;not null" json:"k8s_role_name"`        // K8s role name
	Namespace    string    `gorm:"size:128" json:"namespace"`                     // for namespace-scoped K8s Role
	CreatedAt    time.Time `json:"created_at"`
}

// TableName overrides the table name.
func (RoleBindingDef) TableName() string { return "role_binding_defs" }

// AutoMigrateRoles runs migration for role tables.
func (s *Store) AutoMigrateRoles() error {
	return s.db.AutoMigrate(&RoleDef{}, &RoleBindingDef{})
}

// --- RoleDef CRUD ---

func (s *Store) ListRoleDefs() ([]RoleDef, error) {
	var roles []RoleDef
	if err := s.db.Order("builtin DESC, name ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Store) GetRoleDef(name string) (*RoleDef, error) {
	var role RoleDef
	if err := s.db.Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (s *Store) CreateRoleDef(role *RoleDef) error {
	return s.db.Create(role).Error
}

func (s *Store) DeleteRoleDef(name string) error {
	// Don't delete built-in roles
	var role RoleDef
	if err := s.db.Where("name = ?", name).First(&role).Error; err != nil {
		return err
	}
	if role.Builtin {
		return ErrBuiltinRoleProtected
	}
	// Delete bindings first
	s.db.Where("role_name = ?", name).Delete(&RoleBindingDef{})
	return s.db.Where("name = ?", name).Delete(&role).Error
}

// --- RoleBindingDef CRUD ---

func (s *Store) ListRoleBindings(roleName string) ([]RoleBindingDef, error) {
	var bindings []RoleBindingDef
	q := s.db.Order("id ASC")
	if roleName != "" {
		q = q.Where("role_name = ?", roleName)
	}
	if err := q.Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *Store) AddRoleBinding(b *RoleBindingDef) error {
	return s.db.Create(b).Error
}

func (s *Store) RemoveRoleBinding(id uint) error {
	return s.db.Delete(&RoleBindingDef{}, id).Error
}

// --- Built-in roles seeding ---

var builtinRoles = []RoleDef{
	{Name: "admin", DisplayName: "Admin", Description: "Full cluster access", Group: "k8ops:admin", Scope: "cluster", Builtin: true},
	{Name: "operator", DisplayName: "Operator", Description: "Read/write most resources, no RBAC management", Group: "k8ops:operator", Scope: "cluster", Builtin: true},
	{Name: "viewer", DisplayName: "Viewer", Description: "Cluster-wide read-only", Group: "k8ops:viewer", Scope: "cluster", Builtin: true},
	{Name: "ns-admin", DisplayName: "Namespace Admin", Description: "Namespace-scoped admin", Group: "k8ops:ns-admin", Scope: "namespace", Builtin: true},
	{Name: "ns-viewer", DisplayName: "Namespace Viewer", Description: "Namespace-scoped read-only", Group: "k8ops:ns-viewer", Scope: "namespace", Builtin: true},
}

// SeedBuiltinRoles inserts built-in roles if they don't exist.
func (s *Store) SeedBuiltinRoles() error {
	for _, r := range builtinRoles {
		var existing RoleDef
		result := s.db.Where("name = ?", r.Name).First(&existing)
		if result.Error != nil {
			// Not found — create it
			if err := s.db.Create(&r).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// RoleDefError represents errors for role operations.
var ErrBuiltinRoleProtected = &roleDefError{"cannot modify built-in role"}

type roleDefError struct{ msg string }

func (e *roleDefError) Error() string { return e.msg }
