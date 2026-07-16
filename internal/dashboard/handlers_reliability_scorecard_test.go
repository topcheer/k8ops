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
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestReliabilityScorecardHighGrade verifies a well-configured workload gets grade A/B.
func TestReliabilityScorecardHighGrade(t *testing.T) {
	replicas := int32(3)
	grace := int64(30)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-app", Namespace: "production"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "prod"}},
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "prod"}},
					Spec: corev1.PodSpec{
						TerminationGracePeriodSeconds: &grace,
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "nginx:1.25",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("250m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								},
								ReadinessProbe: &corev1.Probe{
ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}},
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}},
								},
								SecurityContext: &corev1.SecurityContext{
									RunAsNonRoot:         boolPtr(true),
									ReadOnlyRootFilesystem: boolPtr(true),
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, UpdatedReplicas: 3, AvailableReplicas: 3},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/reliability-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleReliabilityScorecard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result ReliabilityScorecardResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.Workloads) == 0 {
		t.Fatal("expected at least 1 workload")
	}

	sc := result.Workloads[0]
	if sc.Grade == "F" || sc.Grade == "D" {
		t.Errorf("expected grade A-C for well-configured workload, got %s (score=%d)", sc.Grade, sc.Score)
	}
	// Should have good replication score
	for _, d := range sc.Dimensions {
		if d.Name == "replication" && d.Score < 60 {
			t.Errorf("expected replication score >= 60 for 3 replicas, got %d", d.Score)
		}
	}
}

// TestReliabilityScorecardLowGrade verifies a poorly-configured workload gets grade D/F.
func TestReliabilityScorecardLowGrade(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "bad"}},
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "bad"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "nginx",
								// No resources, no probes, no security context
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/reliability-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleReliabilityScorecard(w, req)

	var result ReliabilityScorecardResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.Workloads) == 0 {
		t.Fatal("expected at least 1 workload")
	}

	sc := result.Workloads[0]
	if sc.Grade != "D" && sc.Grade != "F" {
		t.Errorf("expected grade D/F for poorly-configured workload, got %s (score=%d)", sc.Grade, sc.Score)
	}

	// Should have multiple risks
	if len(sc.Risks) < 3 {
		t.Errorf("expected >= 3 risks for bad workload, got %d", len(sc.Risks))
	}

	// Should have a non-trivial top fix
	if sc.TopFix == "workload meets reliability baseline" {
		t.Error("expected actionable topFix for bad workload")
	}
}

// TestReliabilityScorecardEmptyCluster verifies handler works with empty cluster.
func TestReliabilityScorecardEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/reliability-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleReliabilityScorecard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ReliabilityScorecardResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
	if result.ClusterScore != 100 {
		t.Errorf("expected cluster score 100 for empty, got %d", result.ClusterScore)
	}
}

// TestReliabilityScorecardSystemNS verifies system namespaces are excluded.
func TestReliabilityScorecardSystemNS(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dns"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "dns"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "dns"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "my"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "my"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "app"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/reliability-scorecard", clientset)
	w := httptest.NewRecorder()

	s.handleReliabilityScorecard(w, req)

	var result ReliabilityScorecardResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// kube-system should be excluded
	if result.Summary.TotalWorkloads != 1 {
		t.Errorf("expected 1 workload (kube-system excluded), got %d", result.Summary.TotalWorkloads)
	}
	if len(result.Workloads) > 0 && result.Workloads[0].Namespace == "kube-system" {
		t.Error("kube-system workload should be excluded")
	}
}

// TestReliabilityScoreToGrade verifies grade conversion.
func TestReliabilityScoreToGrade(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{95, "A"},
		{90, "A"},
		{89, "B"},
		{80, "B"},
		{79, "C"},
		{70, "C"},
		{69, "D"},
		{60, "D"},
		{59, "F"},
		{0, "F"},
	}
	for _, tt := range tests {
	got := scoreToGradeReliability(tt.score)
		if got != tt.want {
			t.Errorf("scoreToGradeReliability(%d) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

// TestDimStatus verifies dimension status conversion.
func TestDimStatus(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{100, "good"},
		{70, "good"},
		{69, "warning"},
		{50, "warning"},
		{49, "critical"},
		{0, "critical"},
	}
	for _, tt := range tests {
		got := dimStatus(tt.score)
		if got != tt.want {
			t.Errorf("dimStatus(%d) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

