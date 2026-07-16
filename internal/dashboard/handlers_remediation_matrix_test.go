package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestRemediationMatrixEmptyCluster verifies handler works on empty cluster.
func TestRemediationMatrixEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

// TestRemediationMatrixPrivilegedContainer verifies privileged container detection.
func TestRemediationMatrixPrivilegedContainer(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "privileged-pod", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should find a critical finding
	foundCritical := false
	for _, f := range result.Findings {
		if f.Severity == "critical" && f.Category == "pod-security" {
			foundCritical = true
			if f.RiskScore < 90 {
				t.Errorf("Critical finding risk score should be >= 90, got %d", f.RiskScore)
			}
		}
	}
	if !foundCritical {
		t.Error("Should detect privileged container as critical finding")
	}

	if result.Summary.CriticalCount == 0 {
		t.Error("Should have at least 1 critical finding")
	}
}

// TestRemediationMatrixQuickWins verifies quick win categorization.
func TestRemediationMatrixQuickWins(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		// Privileged container: critical + quick = quick win
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have at least 1 quick win (privileged container)
	if len(result.QuickWins) == 0 {
		t.Error("Should detect at least 1 quick win")
	}

	// Quick wins should be sorted by risk score descending
	for i := 1; i < len(result.QuickWins); i++ {
		if result.QuickWins[i].RiskScore > result.QuickWins[i-1].RiskScore {
			t.Error("Quick wins should be sorted by risk descending")
		}
	}

	// All quick wins should have risk >= 60
	for _, qw := range result.QuickWins {
		if qw.RiskScore < 60 {
			t.Errorf("Quick win risk score should be >= 60, got %d", qw.RiskScore)
		}
		if qw.Effort != "quick" {
			t.Errorf("Quick win effort should be 'quick', got %s", qw.Effort)
		}
	}
}

// TestRemediationMatrixCategoryRisk verifies category aggregation.
func TestRemediationMatrixCategoryRisk(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		// Pod without limits → pod-security finding
		// Pod running as root → pod-security finding
		// No NetworkPolicy → network finding
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app"},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have categories
	if len(result.ByCategory) == 0 {
		t.Error("Should have at least one category")
	}

	// Categories should be sorted by total risk descending
	for i := 1; i < len(result.ByCategory); i++ {
		if result.ByCategory[i].TotalRisk > result.ByCategory[i-1].TotalRisk {
			t.Error("Categories should be sorted by total risk descending")
		}
	}

	// Each category should have valid fields
	for _, cat := range result.ByCategory {
		if cat.Category == "" {
			t.Error("Category name should not be empty")
		}
		if cat.FindingCount == 0 {
			t.Error("Category should have at least 1 finding")
		}
	}
}

// TestRemediationMatrixRemediationPlan verifies plan generation.
func TestRemediationMatrixRemediationPlan(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have remediation steps
	if len(result.RemediationPlan) == 0 {
		t.Error("Should generate remediation plan")
	}

	// Steps should have sequential priorities
	for i, step := range result.RemediationPlan {
		if step.Priority != i+1 {
			t.Errorf("Step priority should be %d, got %d", i+1, step.Priority)
		}
		if step.Action == "" {
			t.Error("Step should have an action/fix command")
		}
	}

	// Should be limited to 15 steps
	if len(result.RemediationPlan) > 15 {
		t.Errorf("Should have max 15 steps, got %d", len(result.RemediationPlan))
	}
}

// TestRemediationMatrixSystemNSExclusion verifies system namespaces excluded.
func TestRemediationMatrixSystemNSExclusion(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-pod", Namespace: "kube-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// kube-system should NOT appear in findings
	for _, f := range result.Findings {
		if f.Namespace == "kube-system" {
			t.Error("kube-system should be excluded from findings")
		}
	}
}

// TestRemediationMatrixRecommendations verifies recommendations.
func TestRemediationMatrixRecommendations(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/remediation-matrix", clientset)
	w := httptest.NewRecorder()

	s.handleRemediationMatrix(w, req)

	var result RemediationMatrixResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have recommendations
	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}

// TestLastTag verifies image tag extraction.
func TestLastTag(t *testing.T) {
	tests := []struct {
		image  string
		expect string
	}{
		{"nginx:1.21", ":1.21"},
		{"registry.io/app:v2.0", ":v2.0"},
		{"nginx", ""},
		{"nginx:latest", ":latest"},
	}
	for _, tc := range tests {
		got := lastTag(tc.image)
		if got != tc.expect {
			t.Errorf("lastTag(%q) = %q, expected %q", tc.image, got, tc.expect)
		}
	}
}
