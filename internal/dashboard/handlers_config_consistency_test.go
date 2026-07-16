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

func TestConfigConsistencyEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestConfigConsistencyNonConformant(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "nginx"},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", clientset)
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.NonConformantCount == 0 {
		t.Error("Should detect non-conformant workload")
	}
	if len(result.NonConformants) == 0 {
		t.Error("Should have non-conformant entries")
	}
}

func TestConfigConsistencyConformant(t *testing.T) {
	replicas := int32(3)
	revLimit := int32(10)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "good", Namespace: "default",
				Labels: map[string]string{"app": "good"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas, RevisionHistoryLimit: &revLimit,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "registry.internal/app:v1.2.3",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("200m"),
								},
							},
							SecurityContext: &corev1.SecurityContext{RunAsNonRoot: boolPtr(true)},
							ReadinessProbe:  &corev1.Probe{},
							LivenessProbe:   &corev1.Probe{},
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", clientset)
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.NonConformantCount > 0 {
		t.Errorf("Expected 0 non-conformant, got %d", result.Summary.NonConformantCount)
	}
	if result.Summary.ConsistentWorkloads != 1 {
		t.Errorf("Expected 1 consistent, got %d", result.Summary.ConsistentWorkloads)
	}
}

func TestConfigConsistencyImageRegistry(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "registry.io/app:v1"}}},
			}},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx:v1"}}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", clientset)
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.ImageRegistry) < 2 {
		t.Errorf("Expected at least 2 registries, got %d", len(result.ImageRegistry))
	}
}

func TestConfigConsistencySystemNSExcluded(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", clientset)
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("kube-system should be excluded, got %d", result.Summary.TotalWorkloads)
	}
}

func TestConfigConsistencyRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx:latest"}}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/config-consistency", clientset)
	w := httptest.NewRecorder()
	s.handleConfigConsistency(w, req)

	var result ConfigConsistencyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}

func TestExtractImageRegistry(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"registry.io/app:v1", "registry.io"},
		{"docker.io/nginx:v1", "docker.io"},
		{"nginx:v1", "docker.io"},
		{"nginx", "docker.io"},
		{"localhost:5000/app:v1", "localhost:5000"},
	}
	for _, tc := range tests {
		got := extractImageRegistry(tc.image)
		if got != tc.expected {
			t.Errorf("extractImageRegistry(%q) = %q, want %q", tc.image, got, tc.expected)
		}
	}
}

func TestClassifyResourceTier(t *testing.T) {
	tests := []struct {
		cpu, mem string
		tier     string
	}{
		{"25m", "64Mi", "nano"},
		{"100m", "256Mi", "small"},
		{"500m", "512Mi", "medium"},
		{"2", "4Gi", "xl"},
	}
	for _, tc := range tests {
		cpu := resource.MustParse(tc.cpu)
		mem := resource.MustParse(tc.mem)
		got := classifyResourceTier(cpu, mem)
		if got != tc.tier {
			t.Errorf("classifyResourceTier(%s, %s) = %s, want %s", tc.cpu, tc.mem, got, tc.tier)
		}
	}
}
