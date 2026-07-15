package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNamespaceSecurity_NoPSA(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.ServiceAccount{
			ObjectMeta:                   metav1.ObjectMeta{Name: "default", Namespace: "default"},
			AutomountServiceAccountToken: boolPtr(true),
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/namespace-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNamespaceSecurity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NamespaceSecurityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalNamespaces != 1 {
		t.Errorf("expected 1 namespace, got %d", result.Summary.TotalNamespaces)
	}
	if result.Summary.NoPSA != 1 {
		t.Errorf("expected 1 namespace without PSA, got %d", result.Summary.NoPSA)
	}
	if result.HealthScore >= 90 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestNamespaceSecurity_WithPSA(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "production",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "restricted",
					"pod-security.kubernetes.io/warn":    "restricted",
					"pod-security.kubernetes.io/audit":   "restricted",
				},
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta:                   metav1.ObjectMeta{Name: "default", Namespace: "production"},
			AutomountServiceAccountToken: boolPtr(false),
		},
		&networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default-deny", Namespace: "production"},
		},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "production"},
		},
		&corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "limits", Namespace: "production"},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "production"},
			RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "r"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/namespace-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNamespaceSecurity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NamespaceSecurityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithPSAEnforce != 1 {
		t.Errorf("expected 1 namespace with PSA enforce, got %d", result.Summary.WithPSAEnforce)
	}
	// Find the namespace entry
	for _, entry := range result.ByNamespace {
		if entry.Namespace == "production" {
			if entry.PSAEnforce != "restricted" {
				t.Errorf("expected PSA enforce 'restricted', got '%s'", entry.PSAEnforce)
			}
			if entry.TrustLevel == "untrusted" || entry.TrustLevel == "low" {
				t.Errorf("expected high or medium trust level, got '%s'", entry.TrustLevel)
			}
		}
	}
}

func TestNamespaceSecurity_SystemNamespace(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-app"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/namespace-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNamespaceSecurity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NamespaceSecurityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.SystemNamespaces != 1 {
		t.Errorf("expected 1 system namespace, got %d", result.Summary.SystemNamespaces)
	}
	if result.Summary.UserNamespaces != 1 {
		t.Errorf("expected 1 user namespace, got %d", result.Summary.UserNamespaces)
	}
}

func TestNamespaceSecurity_WithRBACAndNP(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "secured",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "baseline",
				},
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta:                   metav1.ObjectMeta{Name: "default", Namespace: "secured"},
			AutomountServiceAccountToken: boolPtr(false),
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "app-role", Namespace: "secured"},
			Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "app-binding", Namespace: "secured"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "default", Namespace: "secured"}},
			RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "app-role"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/namespace-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNamespaceSecurity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NamespaceSecurityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithRBACBindings != 1 {
		t.Errorf("expected 1 namespace with RBAC, got %d", result.Summary.WithRBACBindings)
	}
}
