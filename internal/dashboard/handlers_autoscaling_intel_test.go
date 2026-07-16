package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestAutoscalingIntelEmptyCluster verifies handler on empty cluster.
func TestAutoscalingIntelEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

// TestAutoscalingIntelNoHPA verifies detection of workloads without HPA.
func TestAutoscalingIntelNoHPA(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web-app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app", Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("200m"),
							},
						}},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: 3},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect scaling gap (3 replicas, has resources, no HPA)
	if len(result.ScalingGaps) == 0 {
		t.Error("Should detect scaling gap for multi-replica workload without HPA")
	}

	if result.Summary.WithoutHPA != 1 {
		t.Errorf("Expected 1 without HPA, got %d", result.Summary.WithoutHPA)
	}
}

// TestAutoscalingIntelWithHPA verifies HPA detection and scoring.
func TestAutoscalingIntelWithHPA(t *testing.T) {
	replicas := int32(3)
	minRep := int32(2)
	maxRep := int32(10)
	targetCPU := int32(70)

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app", Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("500m"),
							},
						}},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: 3},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment", Name: "api", APIVersion: "apps/v1",
				},
				MinReplicas: &minRep,
				MaxReplicas: maxRep,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: &targetCPU,
							},
						},
					},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.WithHPA != 1 {
		t.Errorf("Expected 1 with HPA, got %d", result.Summary.WithHPA)
	}

	if result.Summary.HPACoverage < 99 {
		t.Errorf("Expected 100%% HPA coverage, got %.0f%%", result.Summary.HPACoverage)
	}

	// Profile should show HPA details
	if len(result.ScalingProfiles) > 0 {
		p := result.ScalingProfiles[0]
		if !p.HasHPA {
			t.Error("Profile should show hasHPA=true")
		}
		if p.MinReplicas != 2 {
			t.Errorf("Expected minReplicas=2, got %d", p.MinReplicas)
		}
		if p.MaxReplicas != 10 {
			t.Errorf("Expected maxReplicas=10, got %d", p.MaxReplicas)
		}
		if p.TargetCPU != 70 {
			t.Errorf("Expected targetCPU=70, got %d", p.TargetCPU)
		}
	}
}

// TestAutoscalingIntelMisconfiguredHPA verifies detection of HPA without resources.
func TestAutoscalingIntelMisconfiguredHPA(t *testing.T) {
	replicas := int32(2)
	minRep := int32(1)
	maxRep := int32(5)
	targetCPU := int32(70)

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-hpa", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app"}, // No resources!
					}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 2, ReadyReplicas: 2},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment", Name: "bad-hpa", APIVersion: "apps/v1",
				},
				MinReplicas: &minRep,
				MaxReplicas: maxRep,
				Metrics: []autoscalingv2.MetricSpec{
					{Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: &targetCPU,
							},
						}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect misconfigured HPA (HPA exists but no CPU requests)
	foundMisconfig := false
	for _, gap := range result.ScalingGaps {
		if gap.Severity == "critical" {
			foundMisconfig = true
		}
	}
	if !foundMisconfig {
		t.Error("Should detect critical scaling gap for HPA without resources")
	}

	// Profile should be misconfigured
	if len(result.ScalingProfiles) > 0 {
		if result.ScalingProfiles[0].Verdict != "misconfigured" {
			t.Errorf("Expected verdict 'misconfigured', got %s", result.ScalingProfiles[0].Verdict)
		}
	}
}

// TestAutoscalingIntelSingleReplica verifies single replica detection.
func TestAutoscalingIntelSingleReplica(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "solo", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app"},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.ScalingProfiles) > 0 {
		p := result.ScalingProfiles[0]
		if p.Verdict != "single-replica" {
			t.Errorf("Expected verdict 'single-replica', got %s", p.Verdict)
		}
	}
}

// TestAutoscalingIntelSystemNSExclusion verifies system namespaces excluded.
func TestAutoscalingIntelSystemNSExclusion(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("kube-system should be excluded, got %d workloads", result.Summary.TotalWorkloads)
	}
}

// TestAutoscalingIntelRecommendations verifies recommendation generation.
func TestAutoscalingIntelRecommendations(t *testing.T) {
	replicas := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "busy", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "app", Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("100m"),
							},
						}},
					}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 5, ReadyReplicas: 5},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/autoscaling-intel", clientset)
	w := httptest.NewRecorder()

	s.handleAutoscalingIntel(w, req)

	var result AutoscalingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations for workload without HPA")
	}
}
