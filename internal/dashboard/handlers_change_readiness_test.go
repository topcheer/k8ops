package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestChangeReadinessAllPass verifies a healthy cluster returns proceed gate.
func TestChangeReadinessAllPass(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("110"),
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "app"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "app",
								Image: "nginx:1.25",
								Resources: corev1.ResourceRequirements{
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}},
								},
							},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2,
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.GateDecision != "proceed" {
		t.Errorf("expected proceed, got '%s' (score=%d, blockers=%d, warnings=%d)",
			result.GateDecision, result.ReadinessScore, len(result.Blockers), len(result.Warnings))
	}
	if result.ReadinessScore < 80 {
		t.Errorf("expected high readiness score, got %d", result.ReadinessScore)
	}
	if result.Summary.Failed != 0 {
		t.Errorf("expected 0 failed checks, got %d", result.Summary.Failed)
	}
}

// TestChangeReadinessNodePressure verifies blocked gate when node has pressure.
func TestChangeReadinessNodePressure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "pressured"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("110"),
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.GateDecision != "blocked" && result.GateDecision != "proceed-with-caution" {
		t.Errorf("expected blocked or caution, got '%s'", result.GateDecision)
	}

	// Should have node-stability check failing
	foundFail := false
	for _, c := range result.Checks {
		if c.Name == "node-stability" && c.Status == "fail" {
			foundFail = true
		}
	}
	if !foundFail {
		t.Error("expected node-stability check to fail")
	}

	if result.Summary.NodePressure == 0 {
		t.Error("expected node pressure count > 0")
	}
}

// TestChangeReadinessActiveRollouts verifies warning when rollouts are active.
func TestChangeReadinessActiveRollouts(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				Allocatable: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("110"),
				},
			},
		},
		// Deployment mid-rollout: updated=1, replicas=3
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "rolling", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "r"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "r"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx:v2"}}},
				},
			},
			Status: appsv1.DeploymentStatus{Replicas: 3, UpdatedReplicas: 1, AvailableReplicas: 1},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.ActiveRollouts == 0 {
		t.Error("expected active rollouts > 0")
	}

	// Should have active-rollouts check as warn
	found := false
	for _, c := range result.Checks {
		if c.Name == "active-rollouts" && c.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected active-rollouts check to be warn")
	}
}

// TestChangeReadinessFailedPods verifies blocked gate when many pods are failing.
func TestChangeReadinessFailedPods(t *testing.T) {
	var pods []runtime.Object
	pods = append(pods, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("110"),
			},
		},
	})

	// Create 15 crash-looping pods
	for i := 0; i < 15; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("crash-%d", i), Namespace: "broken",
				CreationTimestamp: metav1.Now(),
			},
			Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "c", Image: "broken:v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
				},
			},
		})
	}

	clientset := k8sfake.NewSimpleClientset(pods...)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.FailedPods < 10 {
		t.Errorf("expected >= 10 failed pods, got %d", result.Summary.FailedPods)
	}
	if result.GateDecision != "blocked" {
		t.Errorf("expected blocked gate with many failed pods, got '%s'", result.GateDecision)
	}
}

// TestChangeReadinessCapacity verifies capacity check.
func TestChangeReadinessCapacity(t *testing.T) {
	var objs []runtime.Object
	objs = append(objs, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "dense"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Allocatable: corev1.ResourceList{
				corev1.ResourcePods: resource.MustParse("100"),
			},
		},
	})

	// Add 90 pods to push utilization to 90%
	for i := 0; i < 90; i++ {
		objs = append(objs, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("p-%d", i), Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "dense", Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		})
	}

	clientset := k8sfake.NewSimpleClientset(objs...)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.CapacityHeadroom.Utilization < 85 {
		t.Errorf("expected utilization >= 85%%, got %.1f%%", result.CapacityHeadroom.Utilization)
	}

	// Should have capacity-headroom check failing
	found := false
	for _, c := range result.Checks {
		if c.Name == "capacity-headroom" && c.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected capacity-headroom check to fail at 90% utilization")
	}
}

// TestChangeReadinessEmptyCluster verifies handler with empty cluster.
func TestChangeReadinessEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/change-readiness", clientset)
	w := httptest.NewRecorder()

	s.handleChangeReadiness(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ChangeReadinessResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalChecks == 0 {
		t.Error("expected at least some checks to run")
	}
}

// TestUtilRound verifies the rounding function.
func TestUtilRound(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{85.67, 85.6},
		{90.0, 90.0},
		{33.33, 33.3},
		{0.0, 0.0},
	}
	for _, tt := range tests {
		got := utilRound(tt.input)
		if got != tt.want {
			t.Errorf("utilRound(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
