package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// RBACSyncer manages Kubernetes RoleBindings for namespace-scoped users.
// When a user with role ns-admin or ns-viewer is created/updated, it creates
// RoleBindings in their AllowedNamespaces so the impersonated group has the
// correct permissions in those namespaces.
type RBACSyncer struct {
	clientset kubernetes.Interface
}

// NewRBACSyncer creates a new RBACSyncer.
func NewRBACSyncer(clientset kubernetes.Interface) *RBACSyncer {
	return &RBACSyncer{clientset: clientset}
}

// SyncUserRBAC creates, updates, or removes RoleBindings for a user.
// This should be called whenever a user's role or AllowedNamespaces changes.
func (s *RBACSyncer) SyncUserRBAC(ctx context.Context, user *User) error {
	if s == nil || s.clientset == nil {
		return nil
	}

	// Only ns-admin and ns-viewer need RoleBindings
	if user.Role != "ns-admin" && user.Role != "ns-viewer" {
		// Clean up any existing RoleBindings if user was previously ns-scoped
		return s.cleanupUserRBAC(ctx, user.Username)
	}

	desiredNs := splitNamespaces(user.AllowedNamespaces)

	// Determine role reference — use k8ops custom roles (vendor-independent)
	var roleName string
	switch user.Role {
	case "ns-admin":
		roleName = "k8ops-role-ns-admin"
	case "ns-viewer":
		roleName = "k8ops-role-ns-viewer"
	default:
		return nil
	}

	// Create/update RoleBindings in desired namespaces
	for _, ns := range desiredNs {
		group := fmt.Sprintf("k8ops:%s:%s", user.Role, ns)
		rbName := fmt.Sprintf("k8ops:%s:%s", user.Username, ns)

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name": "k8ops",
					"k8ops.io/managed-by":    "rbac-sync",
					"k8ops.io/user":          user.Username,
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:     "Group",
					Name:     group,
					APIGroup: "rbac.authorization.k8s.io",
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     roleName,
				APIGroup: "rbac.authorization.k8s.io",
			},
		}

		// Try to create, fall back to update
		_, err := s.clientset.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			_, err = s.clientset.RbacV1().RoleBindings(ns).Update(ctx, rb, metav1.UpdateOptions{})
		}
		if err != nil {
			// Namespace might not exist yet, skip silently
			if errors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to sync RoleBinding %s/%s: %w", ns, rbName, err)
		}
	}

	// Clean up RoleBindings in namespaces that are no longer desired
	return s.cleanupUndesiredRBAC(ctx, user.Username, desiredNs)
}

// cleanupUserRBAC removes all RoleBindings created for a user.
func (s *RBACSyncer) cleanupUserRBAC(ctx context.Context, username string) error {
	// List all RoleBindings with the user's label across all namespaces
	// We need to list per-namespace since RoleBindings are namespace-scoped
	nsList, err := s.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	for _, ns := range nsList.Items {
		rbs, err := s.clientset.RbacV1().RoleBindings(ns.Name).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("k8ops.io/user=%s", username),
		})
		if err != nil {
			continue
		}
		for _, rb := range rbs.Items {
			if err := s.clientset.RbacV1().RoleBindings(ns.Name).Delete(ctx, rb.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				slog.Warn("failed to delete rolebinding during cleanup", "namespace", ns.Name, "rolebinding", rb.Name, "error", err)
			}
		}
	}
	return nil
}

// cleanupUndesiredRBAC removes RoleBindings in namespaces no longer in the user's allowed list.
func (s *RBACSyncer) cleanupUndesiredRBAC(ctx context.Context, username string, desiredNs []string) error {
	desired := make(map[string]bool)
	for _, ns := range desiredNs {
		desired[ns] = true
	}

	nsList, err := s.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ns := range nsList.Items {
		if desired[ns.Name] {
			continue // keep this one
		}
		rbs, err := s.clientset.RbacV1().RoleBindings(ns.Name).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("k8ops.io/user=%s", username),
		})
		if err != nil {
			continue
		}
		for _, rb := range rbs.Items {
			if err := s.clientset.RbacV1().RoleBindings(ns.Name).Delete(ctx, rb.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
				slog.Warn("failed to delete undesired rolebinding", "namespace", ns.Name, "rolebinding", rb.Name, "error", err)
			}
		}
	}
	return nil
}

// EnsureNamespaceExists checks if a namespace exists, creates it if not.
// Useful when admin creates namespace-scoped users for namespaces that don't exist yet.
func (s *RBACSyncer) EnsureNamespaceExists(ctx context.Context, ns string) error {
	_, err := s.clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if errors.IsNotFound(err) {
		_, err = s.clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
		return err
	}
	return err
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
