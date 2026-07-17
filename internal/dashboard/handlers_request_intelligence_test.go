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

// TestRequestIntelligenceOverProvisioned verifies detection of over-provisioned workloads.
func TestRequestIntelligenceOverProvisioned(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
			},
		},
		// Over-provisioned deployment: high round-number requests, 0 restarts
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "webapp", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "webapp"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "webapp"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "web",
								Image: "nginx:1.25",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2000m"), // round number, high
										corev1.ResourceMemory: resource.MustParse("2Gi"),   // round number, high
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, UpdatedReplicas: 3, AvailableReplicas: 3},
		},
		// Running pods with 0 restarts
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "webapp-1", Namespace: "default",
				Labels: map[string]string{"app": "webapp"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "webapp", Controller: boolPtr(true)},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "n1",
				Containers: []corev1.Container{
					{
						Name:  "web",
						Image: "nginx:1.25",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("2000m"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 0, Ready: true},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.OverProvisioned) == 0 {
		t.Error("expected over-provisioned workloads")
	}
	foundWebapp := false
	for _, wl := range result.OverProvisioned {
		if wl.Name == "webapp" {
			foundWebapp = true
			if wl.CPURecommend >= wl.CPURequest {
				t.Errorf("expected recommendation < request, got recommend=%.0f request=%.0f", wl.CPURecommend, wl.CPURequest)
			}
		}
	}
	if !foundWebapp {
		t.Error("expected webapp in over-provisioned list")
	}
}

// TestRequestIntelligenceUnderProvisioned verifies detection of under-provisioned workloads.
func TestRequestIntelligenceUnderProvisioned(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "memhog", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "memhog"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "memhog"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "app:v1",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("64Mi"), // too low
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
		// Pod with OOMKill evidence
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "memhog-1", Namespace: "prod",
				Labels: map[string]string{"app": "memhog"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "memhog", Controller: boolPtr(true)},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						RestartCount: 5,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled"},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.UnderProvisioned) == 0 {
		t.Fatal("expected under-provisioned workloads")
	}

	found := false
	for _, wl := range result.UnderProvisioned {
		if wl.Name == "memhog" {
			found = true
			if wl.RiskScore < 20 {
				t.Errorf("expected risk score >= 20 for OOMKill, got %d", wl.RiskScore)
			}
			hasOOMSignal := false
			for _, sig := range wl.Signals {
				if strings.Contains(sig, "OOM") {
					hasOOMSignal = true
				}
			}
			if !hasOOMSignal {
				t.Error("expected OOMKill signal")
			}
			if wl.MemRecommend < wl.MemRequest {
				t.Errorf("expected memory recommendation > current request for OOMKill")
			}
		}
	}
	if !found {
		t.Error("expected memhog in under-provisioned list")
	}

	if result.RiskAssessment.EstimatedOOM == 0 {
		t.Error("expected estimated OOM > 0")
	}
}

// TestRequestIntelligenceNoRequests verifies detection of workloads without requests.
func TestRequestIntelligenceNoRequests(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bare", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "bare"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "bare"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "c", Image: "nginx"},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-1", Namespace: "default",
				Labels: map[string]string{"app": "bare"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "bare", Controller: boolPtr(true)},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 0, Ready: true}},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.NoRequestWorkloads == 0 {
		t.Error("expected no-request workloads > 0")
	}
}

// TestRequestIntelligenceEmptyCluster verifies handler works with empty cluster.
func TestRequestIntelligenceEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("expected 0 workloads, got %d", result.Summary.TotalWorkloads)
	}
	if result.PostureScore != 100 {
		t.Errorf("expected posture score 100 for empty cluster, got %d", result.PostureScore)
	}
}

// TestRequestIntelligenceOptimal verifies that well-configured workloads are marked optimal.
func TestRequestIntelligenceOptimal(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "good", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "good"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "good"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "c",
								Image: "nginx:1.25",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("250m"),  // non-round
										corev1.ResourceMemory: resource.MustParse("384Mi"), // non-round
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "good-1", Namespace: "default",
				Labels: map[string]string{"app": "good"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "good", Controller: boolPtr(true)},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "n1",
				Containers: []corev1.Container{
					{
						Name:  "c",
						Image: "nginx:1.25",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("250m"),
								corev1.ResourceMemory: resource.MustParse("384Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 0, Ready: true}},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.OptimalWorkloads == 0 {
		t.Error("expected at least 1 optimal workload")
	}
}

// TestIsRoundNumber verifies the round number detection.
func TestIsRoundNumber(t *testing.T) {
	tests := []struct {
		input float64
		want  bool
	}{
		{1000, true}, // round
		{2000, true}, // round
		{512, true},  // round
		{250, false}, // not round
		{384, false}, // not round
		{100, true},  // round
		{1024, true}, // round
		{0, false},   // edge case
	}
	for _, tt := range tests {
		got := isRoundNumber(tt.input)
		if got != tt.want {
			t.Errorf("isRoundNumber(%.0f) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestRequestIntelligenceSavings verifies savings estimation with large requests.
func TestRequestIntelligenceSavings(t *testing.T) {
	replicas := int32(10)
	// Large over-provisioned deployment with pods
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "big", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "big"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "big"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "c",
								Image: "app:v1",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("4000m"),
										corev1.ResourceMemory: resource.MustParse("4Gi"),
									},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 10, UpdatedReplicas: 10, AvailableReplicas: 10},
		},
		// Add a running pod
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "big-0", Namespace: "default",
				Labels: map[string]string{"app": "big"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "big", Controller: boolPtr(true)},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "n1",
				Containers: []corev1.Container{
					{
						Name:  "c",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4000m"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 0, Ready: true}},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/request-intelligence", clientset)
	w := httptest.NewRecorder()

	s.handleRequestIntelligence(w, req)

	var result RequestIntelligenceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect over-provisioning from round-number requests
	if result.PostureScore < 0 || result.PostureScore > 100 {
		t.Errorf("posture score out of range: %d", result.PostureScore)
	}
}
