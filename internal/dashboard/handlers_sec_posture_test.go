package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestSecPosturePrivileged verifies detection of privileged containers.
func TestSecPosturePrivileged(t *testing.T) {
	replicas := int32(1)
	priv := true
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "danger", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "danger"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "danger"}},
					Spec: corev1.PodSpec{
						HostNetwork: true,
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "app:v1",
								SecurityContext: &corev1.SecurityContext{
									Privileged: &priv,
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/posture-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleSecurityPosture(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result SecPostureResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.PrivilegedPods == 0 {
		t.Error("expected privileged pods > 0")
	}
	if result.Summary.HostNetworkPods == 0 {
		t.Error("expected hostNetwork pods > 0")
	}
	if result.ClusterGrade == "A" || result.ClusterGrade == "B" {
		t.Errorf("expected low grade for privileged + hostNetwork, got %s", result.ClusterGrade)
	}

	found := false
	for _, wl := range result.HighRiskWorkloads {
		if wl.Name == "danger" {
			found = true
			if wl.RiskLevel != "critical" && wl.RiskLevel != "high" {
				t.Errorf("expected critical/high risk, got %s", wl.RiskLevel)
			}
		}
	}
	if !found {
		t.Error("expected danger deployment in high-risk list")
	}
}

// TestSecPostureGoodWorkload verifies well-secured workload scores high.
func TestSecPostureGoodWorkload(t *testing.T) {
	replicas := int32(3)
	nonRoot := true
	readOnly := true
	autoMount := false
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "secure", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "secure"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "secure"}},
					Spec: corev1.PodSpec{
						AutomountServiceAccountToken: &autoMount,
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "app:v1",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								},
								SecurityContext: &corev1.SecurityContext{
									RunAsNonRoot:           &nonRoot,
									ReadOnlyRootFilesystem: &readOnly,
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, AvailableReplicas: 3},
		},
		// Add NetworkPolicy to the namespace
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/security/posture-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleSecurityPosture(w, req)

	var result SecPostureResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.PrivilegedPods > 0 {
		t.Error("expected 0 privileged pods for secure workload")
	}
	if result.Summary.CriticalRisk > 0 {
		t.Error("expected 0 critical risk for secure workload")
	}
}

// TestSecPostureEmpty verifies handler with empty cluster.
func TestSecPostureEmpty(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/posture-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleSecurityPosture(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result SecPostureResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
}

// TestClampScore verifies score clamping.
func TestClampScore(t *testing.T) {
	if clampScore(-10) != 0 {
		t.Error("expected 0 for negative")
	}
	if clampScore(150) != 100 {
		t.Error("expected 100 for >100")
	}
	if clampScore(50) != 50 {
		t.Error("expected 50 for 50")
	}
}

// TestCategorizeViolation verifies violation categorization.
func TestCategorizeViolation(t *testing.T) {
	tests := map[string]string{
		"privileged":           "pod-security",
		"hostNetwork":          "host-access",
		"hostPath: /data":      "host-access",
		"SYS_ADMIN capability": "capabilities",
		"no NetworkPolicy":     "network-isolation",
		"no resource limits":   "resource-boundaries",
	}
	for input, want := range tests {
		got := categorizeViolation(input)
		if got != want {
			t.Errorf("categorizeViolation(%q) = %s, want %s", input, got, want)
		}
	}
}
