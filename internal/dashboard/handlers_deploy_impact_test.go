package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestDeployImpactSingleReplica verifies high risk for single-replica workloads.
func TestDeployImpactSingleReplica(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "single", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "single"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "single"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "app:v1"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/impact-simulator", clientset)
	w := httptest.NewRecorder()

	s.handleDeployImpact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result DeployImpactResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.SingleReplicaWorkloads == 0 {
		t.Error("expected single replica workloads > 0")
	}
	if len(result.Simulations) == 0 {
		t.Fatal("expected simulations")
	}
	sim := result.Simulations[0]
	if sim.ImpactLevel != "high" {
		t.Errorf("expected high impact for single replica, got %s", sim.ImpactLevel)
	}
	if sim.RiskScore < 30 {
		t.Errorf("expected risk score >= 30, got %d", sim.RiskScore)
	}
}

// TestDeployImpactMultiReplicaSafe verifies low risk for well-configured workloads.
func TestDeployImpactMultiReplicaSafe(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "safe"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "safe"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "c", Image: "app:v1",
								ReadinessProbe: &corev1.Probe{},
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, AvailableReplicas: 3},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/impact-simulator", clientset)
	w := httptest.NewRecorder()

	s.handleDeployImpact(w, req)

	var result DeployImpactResult
	json.Unmarshal(w.Body.Bytes(), &result)

	sim := result.Simulations[0]
	if sim.ImpactLevel == "high" {
		t.Errorf("expected low/medium impact for safe workload, got %s", sim.ImpactLevel)
	}
}

// TestDeployImpactWithDependents verifies cascade detection.
func TestDeployImpactWithDependents(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api-internal", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports:    []corev1.ServicePort{{Port: 8080}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "metrics", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports:    []corev1.ServicePort{{Port: 9090}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "backend"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "backend"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "app:v1"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/impact-simulator", clientset)
	w := httptest.NewRecorder()

	s.handleDeployImpact(w, req)

	var result DeployImpactResult
	json.Unmarshal(w.Body.Bytes(), &result)

	if result.Summary.CriticalDependencies == 0 {
		t.Error("expected critical dependencies > 0")
	}
	if len(result.CascadeRisks) == 0 {
		t.Error("expected cascade risks for workload with 3 dependents")
	}
}

// TestDeployImpactEmpty verifies handler with empty cluster.
func TestDeployImpactEmpty(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/impact-simulator", clientset)
	w := httptest.NewRecorder()

	s.handleDeployImpact(w, req)

	var result DeployImpactResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
}

// TestGetWorkloadKey verifies label extraction.
func TestGetWorkloadKey(t *testing.T) {
	tests := []struct {
		labels map[string]string
		name   string
		want   string
	}{
		{map[string]string{"app": "web"}, "pod-1", "web"},
		{map[string]string{"k8s-app": "dns"}, "dns-pod", "dns"},
		{map[string]string{}, "bare-pod", "bare-pod"},
	}
	for _, tt := range tests {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: tt.name, Labels: tt.labels},
		}
		got := getWorkloadKey(pod)
		if got != tt.want {
			t.Errorf("getWorkloadKey(labels=%v) = %s, want %s", tt.labels, got, tt.want)
		}
	}
}

// TestTopRiskReason verifies reason generation.
func TestTopRiskReason(t *testing.T) {
	if r := topRiskReason([]string{"single replica"}, 40, 1, 0); r != "single replica" {
		t.Errorf("expected blocker as reason, got %s", r)
	}
	if r := topRiskReason(nil, 10, 3, 0); r != "low risk — safe to deploy" {
		t.Errorf("expected low risk for safe workload, got %s", r)
	}
	if r := topRiskReason(nil, 10, 3, 5); !containsHelper(r, "dependent") {
		t.Errorf("expected dependent mention, got %s", r)
	}
}

func containsHelper(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || strings.Contains(s, sub))
}

var _ = strings.TrimSpace
