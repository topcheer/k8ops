package auth

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// --- splitNamespaces ---

func TestSplitNamespaces(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single", "default", []string{"default"}},
		{"multiple", "a,b,c", []string{"a", "b", "c"}},
		{"with_spaces", "a, b , c", []string{"a", "b", "c"}},
		{"empty", "", []string{}},
		{"only_commas", ",,", []string{}},
		{"trailing_comma", "a,b,", []string{"a", "b"}},
		{"leading_comma", ",a,b", []string{"a", "b"}},
		{"tabs_and_spaces", "  ns-1  ,\tns-2  ", []string{"ns-1", "ns-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitNamespaces(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d namespaces, got %d: %v", len(tt.want), len(got), got)
			}
			for i, ns := range got {
				if ns != tt.want[i] {
					t.Errorf("index %d: expected %q, got %q", i, tt.want[i], ns)
				}
			}
		})
	}
}

// --- NewRBACSyncer ---

func TestNewRBACSyncer(t *testing.T) {
	cs := fake.NewSimpleClientset()
	s := NewRBACSyncer(cs)
	if s == nil {
		t.Fatal("expected non-nil RBACSyncer")
	}
}

// --- SyncUserRBAC ---

func TestSyncUserRBAC_NsAdmin_CreatesRoleBindings(t *testing.T) {
	// Pre-create namespaces
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
	)
	s := NewRBACSyncer(cs)

	user := &User{
		Username:          "alice",
		Role:              "ns-admin",
		AllowedNamespaces: "ns-a,ns-b",
	}

	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Fatalf("SyncUserRBAC failed: %v", err)
	}

	// Verify RoleBindings created in both namespaces
	for _, ns := range []string{"ns-a", "ns-b"} {
		rbs, err := cs.RbacV1().RoleBindings(ns).List(context.Background(), metav1.ListOptions{
			LabelSelector: "k8ops.io/user=alice",
		})
		if err != nil {
			t.Fatalf("failed to list RoleBindings in %s: %v", ns, err)
		}
		if len(rbs.Items) != 1 {
			t.Fatalf("expected 1 RoleBinding in %s, got %d", ns, len(rbs.Items))
		}
		rb := rbs.Items[0]
		if rb.RoleRef.Name != "k8ops-role-ns-admin" {
			t.Errorf("expected RoleRef k8ops-role-ns-admin, got %s", rb.RoleRef.Name)
		}
		expectedGroup := "k8ops:ns-admin:ns-a"
		if ns == "ns-b" {
			expectedGroup = "k8ops:ns-admin:ns-b"
		}
		if len(rb.Subjects) != 1 || rb.Subjects[0].Name != expectedGroup {
			t.Errorf("expected subject %s, got %+v", expectedGroup, rb.Subjects)
		}
	}
}

func TestSyncUserRBAC_NsViewer_CreatesRoleBindings(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}},
	)
	s := NewRBACSyncer(cs)

	user := &User{
		Username:          "bob",
		Role:              "ns-viewer",
		AllowedNamespaces: "production",
	}

	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Fatalf("SyncUserRBAC failed: %v", err)
	}

	rbs, err := cs.RbacV1().RoleBindings("production").List(context.Background(), metav1.ListOptions{
		LabelSelector: "k8ops.io/user=bob",
	})
	if err != nil {
		t.Fatalf("failed to list RoleBindings: %v", err)
	}
	if len(rbs.Items) != 1 {
		t.Fatalf("expected 1 RoleBinding, got %d", len(rbs.Items))
	}
	if rbs.Items[0].RoleRef.Name != "k8ops-role-ns-viewer" {
		t.Errorf("expected RoleRef k8ops-role-ns-viewer, got %s", rbs.Items[0].RoleRef.Name)
	}
}

func TestSyncUserRBAC_ClusterRole_NoBindings(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	s := NewRBACSyncer(cs)

	// admin/operator/viewer are cluster-scoped, should NOT create RoleBindings
	user := &User{
		Username: "admin-user",
		Role:     "admin",
	}

	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Fatalf("SyncUserRBAC failed: %v", err)
	}

	nsList, _ := cs.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		rbs, _ := cs.RbacV1().RoleBindings(ns.Name).List(context.Background(), metav1.ListOptions{
			LabelSelector: "k8ops.io/user=admin-user",
		})
		if len(rbs.Items) > 0 {
			t.Errorf("expected no RoleBindings for cluster-scoped admin in %s, got %d", ns.Name, len(rbs.Items))
		}
	}
}

func TestSyncUserRBAC_NamespaceCleanup_RemovesUndesired(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-old"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-new"}},
	)
	s := NewRBACSyncer(cs)

	// Step 1: Create user with ns-old
	user := &User{
		Username:          "carol",
		Role:              "ns-admin",
		AllowedNamespaces: "ns-old",
	}
	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}

	// Verify binding exists in ns-old
	rbs, _ := cs.RbacV1().RoleBindings("ns-old").List(context.Background(), metav1.ListOptions{
		LabelSelector: "k8ops.io/user=carol",
	})
	if len(rbs.Items) != 1 {
		t.Fatalf("expected 1 binding in ns-old, got %d", len(rbs.Items))
	}

	// Step 2: Update user to ns-new only (remove ns-old)
	user.AllowedNamespaces = "ns-new"
	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	// ns-old should have no bindings
	rbsOld, _ := cs.RbacV1().RoleBindings("ns-old").List(context.Background(), metav1.ListOptions{
		LabelSelector: "k8ops.io/user=carol",
	})
	if len(rbsOld.Items) != 0 {
		t.Errorf("expected 0 bindings in ns-old after cleanup, got %d", len(rbsOld.Items))
	}

	// ns-new should have 1 binding
	rbsNew, _ := cs.RbacV1().RoleBindings("ns-new").List(context.Background(), metav1.ListOptions{
		LabelSelector: "k8ops.io/user=carol",
	})
	if len(rbsNew.Items) != 1 {
		t.Errorf("expected 1 binding in ns-new, got %d", len(rbsNew.Items))
	}
}

func TestSyncUserRBAC_NonExistentNamespace_SkipsSilently(t *testing.T) {
	cs := fake.NewSimpleClientset(
		// No namespace created
	)
	s := NewRBACSyncer(cs)

	user := &User{
		Username:          "dave",
		Role:              "ns-viewer",
		AllowedNamespaces: "does-not-exist",
	}

	err := s.SyncUserRBAC(context.Background(), user)
	// Should not error — namespace not found is skipped silently
	if err != nil {
		t.Errorf("expected no error for non-existent namespace, got: %v", err)
	}
}

func TestSyncUserRBAC_EmptyNamespaces_NoError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	s := NewRBACSyncer(cs)

	user := &User{
		Username:          "eve",
		Role:              "ns-admin",
		AllowedNamespaces: "",
	}

	if err := s.SyncUserRBAC(context.Background(), user); err != nil {
		t.Errorf("expected no error for empty namespaces, got: %v", err)
	}
}

// --- cleanupUserRBAC ---

func TestCleanupUserRBAC_RemovesAllBindings(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-2"}},
	)
	s := NewRBACSyncer(cs)

	// Create RoleBindings for "frank" in two namespaces
	for _, ns := range []string{"ns-1", "ns-2"} {
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "k8ops:frank:" + ns,
				Namespace: ns,
				Labels: map[string]string{
					"k8ops.io/managed-by": "rbac-sync",
					"k8ops.io/user":       "frank",
				},
			},
			Subjects: []rbacv1.Subject{{
				Kind: "Group", Name: "k8ops:ns-admin:" + ns,
				APIGroup: "rbac.authorization.k8s.io",
			}},
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole", Name: "k8ops-role-ns-admin",
				APIGroup: "rbac.authorization.k8s.io",
			},
		}
		_, err := cs.RbacV1().RoleBindings(ns).Create(context.Background(), rb, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create test RoleBinding in %s: %v", ns, err)
		}
	}

	// Cleanup
	if err := s.cleanupUserRBAC(context.Background(), "frank"); err != nil {
		t.Fatalf("cleanupUserRBAC failed: %v", err)
	}

	// Verify all removed
	for _, ns := range []string{"ns-1", "ns-2"} {
		rbs, _ := cs.RbacV1().RoleBindings(ns).List(context.Background(), metav1.ListOptions{
			LabelSelector: "k8ops.io/user=frank",
		})
		if len(rbs.Items) != 0 {
			t.Errorf("expected 0 bindings for frank in %s, got %d", ns, len(rbs.Items))
		}
	}
}

// --- EnsureNamespaceExists ---

func TestEnsureNamespaceExists_CreateIfMissing(t *testing.T) {
	cs := fake.NewSimpleClientset()
	s := NewRBACSyncer(cs)

	err := s.EnsureNamespaceExists(context.Background(), "brand-new-ns")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	ns, err := cs.CoreV1().Namespaces().Get(context.Background(), "brand-new-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected namespace to be created: %v", err)
	}
	if ns.Name != "brand-new-ns" {
		t.Errorf("expected namespace name 'brand-new-ns', got %q", ns.Name)
	}
}

func TestEnsureNamespaceExists_AlreadyExists_NoError(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existing-ns"}},
	)
	s := NewRBACSyncer(cs)

	err := s.EnsureNamespaceExists(context.Background(), "existing-ns")
	if err != nil {
		t.Errorf("expected no error for existing namespace, got: %v", err)
	}
}

// --- Nil safety ---

func TestSyncUserRBAC_NilSyncer_NoPanic(t *testing.T) {
	var s *RBACSyncer
	user := &User{Username: "test", Role: "admin"}
	err := s.SyncUserRBAC(context.Background(), user)
	if err != nil {
		t.Errorf("expected nil error for nil syncer, got: %v", err)
	}
}

func TestSyncUserRBAC_NilClientset_NoPanic(t *testing.T) {
	s := &RBACSyncer{clientset: nil}
	user := &User{Username: "test", Role: "admin"}
	err := s.SyncUserRBAC(context.Background(), user)
	if err != nil {
		t.Errorf("expected nil error for nil clientset, got: %v", err)
	}
}
