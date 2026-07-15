package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestAdmissionPolicyEmptyCluster verifies empty cluster behavior.
func TestAdmissionPolicyEmptyCluster(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalValidatingWebhooks != 0 {
		t.Errorf("expected 0 validating webhooks, got %d", result.Summary.TotalValidatingWebhooks)
	}
}

// TestAdmissionPolicyWithWebhooks verifies webhook detection.
func TestAdmissionPolicyWithWebhooks(t *testing.T) {
	failPolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	mp := admissionregistrationv1.Exact
	timeoutSecs := int32(10)

	clientset := k8sfake.NewSimpleClientset(
		&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-security"},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{
				{
					Name: "pod-security.k8s.io",
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1.OperationType{"CREATE", "UPDATE"},
							Rule: admissionregistrationv1.Rule{
								Resources:   []string{"pods"},
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
							},
						},
					},
					FailurePolicy:  &failPolicy,
					MatchPolicy:    &mp,
					SideEffects:    &sideEffects,
					TimeoutSeconds: &timeoutSecs,
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{
							Name:      "pod-security-webhook",
							Namespace: "kube-system",
						},
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "app"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", clientset)
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalValidatingWebhooks != 1 {
		t.Errorf("expected 1 validating webhook, got %d", result.Summary.TotalValidatingWebhooks)
	}

	if result.Summary.ActiveWebhooks != 1 {
		t.Errorf("expected 1 active webhook, got %d", result.Summary.ActiveWebhooks)
	}

	// Deployment should be covered by the pods webhook
	// Note: hasAdmissionProtection checks for "Deployment" in resources, but webhook only has "pods"
	// So deployment won't be covered — which is correct behavior
}

// TestAdmissionPolicyIgnoreFailurePolicy verifies detection of Ignore failurePolicy.
func TestAdmissionPolicyIgnoreFailurePolicy(t *testing.T) {
	ignorePolicy := admissionregistrationv1.Ignore
	sideEffects := admissionregistrationv1.SideEffectClassNone
	mp := admissionregistrationv1.Exact
	timeoutSecs := int32(10)

	clientset := k8sfake.NewSimpleClientset(
		&admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "ignore-policy"},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{
				{
					Name: "ignore-wh.k8s.io",
					Rules: []admissionregistrationv1.RuleWithOperations{
						{
							Operations: []admissionregistrationv1.OperationType{"CREATE"},
							Rule: admissionregistrationv1.Rule{
								Resources: []string{"pods"},
							},
						},
					},
					FailurePolicy:  &ignorePolicy,
					MatchPolicy:    &mp,
					SideEffects:    &sideEffects,
					TimeoutSeconds: &timeoutSecs,
					ClientConfig: admissionregistrationv1.WebhookClientConfig{
						Service: &admissionregistrationv1.ServiceReference{Name: "svc", Namespace: "ns"},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", clientset)
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect Ignore failurePolicy risk
	foundIgnoreRisk := false
	for _, risk := range result.Risks {
		if risk.Category == "ignore-failure-policy" {
			foundIgnoreRisk = true
		}
	}
	if !foundIgnoreRisk {
		t.Error("expected ignore-failure-policy risk")
	}
}

// TestAdmissionPolicyNoWebhooks verifies risk detection when no webhooks exist.
func TestAdmissionPolicyNoWebhooks(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "unprotected-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "u"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "u"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", clientset)
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect no-validating-webhooks risk
	foundNoWebhook := false
	for _, risk := range result.Risks {
		if risk.Category == "no-validating-webhooks" {
			foundNoWebhook = true
		}
	}
	if !foundNoWebhook {
		t.Error("expected no-validating-webhooks risk")
	}

	// Health score should be low
	if result.HealthScore >= 80 {
		t.Errorf("expected low health score with no webhooks, got %d", result.HealthScore)
	}

	// Should have unprotected workloads
	if result.Summary.UnprotectedWorkloads == 0 {
		t.Error("expected unprotected workloads")
	}
}

// TestAdmissionPolicyGatekeeperDetection verifies Gatekeeper/Kyverno detection.
func TestAdmissionPolicyGatekeeperDetection(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "gatekeeper-controller-0", Namespace: "gatekeeper-system"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "kyverno-admission-controller-0", Namespace: "kyverno"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", clientset)
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.NamespacesWithGatekeeper != 1 {
		t.Errorf("expected 1 gatekeeper namespace, got %d", result.Summary.NamespacesWithGatekeeper)
	}
	if result.Summary.NamespacesWithKyverno != 1 {
		t.Errorf("expected 1 kyverno namespace, got %d", result.Summary.NamespacesWithKyverno)
	}
}

// TestAdmissionPolicyRecommendations verifies recommendation generation.
func TestAdmissionPolicyRecommendations(t *testing.T) {
	result := AdmissionPolicyResult{
		Summary: AdmissionPolicySummary{
			TotalValidatingWebhooks: 0,
			UnprotectedWorkloads:    5,
			TotalWorkloads:          10,
			FailingWebhooks:         0,
		},
		Coverage: AdmissionCoverage{
			DeploymentCoverage: 30,
		},
	}

	recs := generateAdmissionRecommendations(result)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundNoWebhook := false
	foundCEL := false
	foundCoverage := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "no validating") {
			foundNoWebhook = true
		}
		if strings.Contains(lower, "cel") {
			foundCEL = true
		}
		if strings.Contains(lower, "coverage") {
			foundCoverage = true
		}
	}
	if !foundNoWebhook {
		t.Error("expected no-validating-webhooks recommendation")
	}
	if !foundCEL {
		t.Error("expected CEL admission policy recommendation")
	}
	if !foundCoverage {
		t.Error("expected coverage recommendation")
	}
}

// TestExtractWebhookResources verifies resource extraction.
func TestExtractWebhookResources(t *testing.T) {
	rules := []admissionregistrationv1.RuleWithOperations{
		{
			Rule: admissionregistrationv1.Rule{
				Resources: []string{"pods", "deployments"},
			},
		},
		{
			Rule: admissionregistrationv1.Rule{
				Resources: []string{"pods", "services"},
			},
		},
	}

	resources := extractWebhookResources(rules)

	// Should deduplicate: pods, deployments, services
	if len(resources) != 3 {
		t.Errorf("expected 3 unique resources, got %d: %v", len(resources), resources)
	}
}

// TestHasAdmissionProtection verifies protection detection.
func TestHasAdmissionProtection(t *testing.T) {
	webhooks := []WebhookPolicy{
		{
			Type:      "Validating",
			Resources: []string{"pods", "deployments"},
		},
		{
			Type:      "Mutating",
			Resources: []string{"pods"},
		},
	}

	if !hasAdmissionProtection("Deployment", "default", webhooks) {
		t.Error("expected Deployment to be protected")
	}
	if !hasAdmissionProtection("pods", "default", webhooks) {
		t.Error("expected pods to be protected")
	}
	if hasAdmissionProtection("ingresses", "default", webhooks) {
		t.Error("ingresses should not be protected")
	}
}

// TestPct verifies percentage calculation.
func TestAdmissionPolicyPct(t *testing.T) {
	tests := []struct {
		num, den, expected int
	}{
		{8, 10, 80},
		{0, 10, 0},
		{10, 10, 100},
		{5, 0, 100}, // zero denominator = 100%
	}

	for _, tt := range tests {
		got := pctAdmission(tt.num, tt.den)
		if got != tt.expected {
			t.Errorf("pctAdmission(%d, %d) = %d, want %d", tt.num, tt.den, got, tt.expected)
		}
	}
}

// TestAdmissionPolicyPrivilegedWorkload verifies severity classification for privileged pods.
func TestAdmissionPolicyPrivilegedWorkload(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "privileged-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "p"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "p"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "c",
								SecurityContext: &corev1.SecurityContext{
									Privileged: &privileged,
								},
							},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/admission-policy-audit", clientset)
	w := httptest.NewRecorder()
	s.handleAdmissionPolicyAudit(w, req)

	var result AdmissionPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should classify privileged workload as critical severity gap
	foundCritical := false
	for _, gap := range result.GapByResource {
		if gap.Name == "privileged-app" && gap.Severity == "critical" {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Error("expected critical severity for privileged unprotected workload")
	}
}
