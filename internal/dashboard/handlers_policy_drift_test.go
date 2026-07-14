package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPolicyDrift_PSAFlags(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		// Namespace without PSA labels (should be flagged)
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "app-prod",
				Labels: map[string]string{}, // no PSA labels
			},
		},
		// Namespace with privileged PSA (should be flagged as critical)
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-unsafe",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "privileged",
				},
			},
		},
		// Namespace with baseline PSA (should not be flagged)
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-good",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "baseline",
					"pod-security.kubernetes.io/audit":   "restricted",
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/policy-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePolicyDrift(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PolicyDriftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have 2 PSA gaps: app-prod (missing) + app-unsafe (privileged)
	if len(result.PSALabelGaps) < 2 {
		t.Errorf("expected at least 2 PSA gaps, got %d", len(result.PSALabelGaps))
	}

	foundMissing := false
	foundPrivileged := false
	for _, gap := range result.PSALabelGaps {
		if gap.Namespace == "app-prod" && gap.Severity == "high" {
			foundMissing = true
		}
		if gap.Namespace == "app-unsafe" && gap.Severity == "critical" {
			foundPrivileged = true
		}
	}
	if !foundMissing {
		t.Error("expected to find app-prod with high severity (missing PSA labels)")
	}
	if !foundPrivileged {
		t.Error("expected to find app-unsafe with critical severity (privileged enforce)")
	}
}

func TestPolicyDrift_DefaultRoleBinding(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-risky",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "baseline",
				},
			},
		},
		// Risky RoleBinding: cluster-admin -> default SA
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "risky-binding", Namespace: "app-risky"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "default", Namespace: "app-risky"},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/policy-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePolicyDrift(rec, req)

	var result PolicyDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	found := false
	for _, risk := range result.DefaultRoleRisk {
		if risk.SubjectName == "default" && risk.RoleName == "cluster-admin" {
			found = true
			if risk.Severity != "critical" {
				t.Errorf("expected critical severity for cluster-admin binding, got %s", risk.Severity)
			}
		}
	}
	if !found {
		t.Error("expected to find cluster-admin binding to default SA")
	}
}

func TestPolicyDrift_NetworkBaseline(t *testing.T) {
	ctx := context.Background()
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-nettest",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "baseline",
				},
			},
		},
		// Create a Deployment pod so workloadCount > 0
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "app-nettest",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "app-deploy"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c1", Image: "nginx"}},
			},
		},
	)

	// No network policies → should be flagged as no default deny
	req := newReqWithClients(http.MethodGet, "/api/security/policy-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePolicyDrift(rec, req)

	var result PolicyDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	found := false
	for _, nb := range result.NetworkBaseline {
		if nb.Namespace == "app-nettest" && !nb.HasDefaultDeny {
			found = true
			if nb.Severity != "high" {
				t.Errorf("expected high severity for no netpol, got %s", nb.Severity)
			}
		}
	}
	if !found {
		t.Error("expected to find app-nettest with no default deny network policy")
	}

	// Now add a default deny policy and verify it's no longer flagged
	clientset.NetworkingV1().NetworkPolicies("app-nettest").Create(ctx, &netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-deny"},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []netv1.PolicyType{
				netv1.PolicyTypeIngress,
				netv1.PolicyTypeEgress,
			},
		},
	}, metav1.CreateOptions{})

	rec2 := httptest.NewRecorder()
	srv.handlePolicyDrift(rec2, req)

	var result2 PolicyDriftResult
	json.Unmarshal(rec2.Body.Bytes(), &result2)

	for _, nb := range result2.NetworkBaseline {
		if nb.Namespace == "app-nettest" {
			if !nb.HasDefaultDeny {
				t.Error("expected app-nettest to have default deny after adding netpol")
			}
		}
	}
}

func TestPolicyDrift_HealthScore(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		// All namespaces properly configured → high score
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "clean-ns",
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce":         "baseline",
					"pod-security.kubernetes.io/enforce-version": "latest",
					"pod-security.kubernetes.io/audit":           "restricted",
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/policy-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePolicyDrift(rec, req)

	var result PolicyDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.HealthScore < 90 {
		t.Errorf("expected health score >= 90 for clean cluster, got %d", result.HealthScore)
	}
}

func TestPolicyDrift_EmptyCluster(t *testing.T) {
	// Test with only system namespaces (should not crash)
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/security/policy-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePolicyDrift(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result PolicyDriftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNamespaces != 0 {
		t.Errorf("expected 0 non-system namespaces, got %d", result.Summary.TotalNamespaces)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100 for empty cluster, got %d", result.HealthScore)
	}
}
