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

// TestScaleSimulatorCanScale verifies a safe scale-up.
func TestScaleSimulatorCanScale(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "api", Image: "api:v1",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("500m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
								}},
						},
					},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("16"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scale-simulator?workload=api&namespace=prod&replicas=5", clientset)
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ScaleSimResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Verdict != "can-scale" {
		t.Errorf("expected can-scale, got %s", result.Verdict)
	}
	if result.CurrentState.Replicas != 2 {
		t.Errorf("expected current replicas 2, got %d", result.CurrentState.Replicas)
	}
	if result.SimulatedState.Replicas != 5 {
		t.Errorf("expected simulated replicas 5, got %d", result.SimulatedState.Replicas)
	}
	if result.Delta.ReplicaDelta != 3 {
		t.Errorf("expected delta 3, got %d", result.Delta.ReplicaDelta)
	}
}

// TestScaleSimulatorQuotaBlock verifies quota detection.
func TestScaleSimulatorQuotaBlock(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "big-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "big"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "big"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "c", Image: "app:v1",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2000m"),
										corev1.ResourceMemory: resource.MustParse("4Gi"),
									},
								}},
						},
					},
				},
			},
		},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "prod"},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceRequestsCPU:    resource.MustParse("4"),
					corev1.ResourceRequestsMemory: resource.MustParse("8Gi"),
					corev1.ResourcePods:           resource.MustParse("10"),
				},
			},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceRequestsCPU:    resource.MustParse("4"),
					corev1.ResourceRequestsMemory: resource.MustParse("8Gi"),
					corev1.ResourcePods:           resource.MustParse("10"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceRequestsCPU:    resource.MustParse("2"),
					corev1.ResourceRequestsMemory: resource.MustParse("4Gi"),
					corev1.ResourcePods:           resource.MustParse("3"),
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("32"), corev1.ResourceMemory: resource.MustParse("128Gi"),
			}},
		},
	)

	s := &Server{}
	// Scale to 5 replicas = 10 CPU needed, but quota is 4 CPU
	req := newReqWithClients("GET", "/api/scalability/scale-simulator?workload=big-app&namespace=prod&replicas=5", clientset)
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	var result ScaleSimResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Verdict != "cannot-scale" {
		t.Errorf("expected cannot-scale, got %s", result.Verdict)
	}

	// Should have quota blocker
	foundQuotaBlock := false
	for _, b := range result.Blockers {
		if b.Type == "quota" {
			foundQuotaBlock = true
		}
	}
	if !foundQuotaBlock {
		t.Error("expected quota blocker")
	}
}

// TestScaleSimulatorScaleDown verifies scale-down.
func TestScaleSimulatorScaleDown(t *testing.T) {
	replicas := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("8"), corev1.ResourceMemory: resource.MustParse("32Gi"),
			}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scale-simulator?workload=web&namespace=default&replicas=2", clientset)
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	var result ScaleSimResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Delta.ReplicaDelta != -3 {
		t.Errorf("expected delta -3, got %d", result.Delta.ReplicaDelta)
	}
}

// TestScaleSimulatorMissingWorkload verifies error handling.
func TestScaleSimulatorMissingWorkload(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scale-simulator?workload=nonexistent&namespace=default&replicas=3", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestScaleSimulatorMissingParams verifies parameter validation.
func TestScaleSimulatorMissingParams(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scale-simulator", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestScaleSimulatorStatefulSet verifies StatefulSet handling.
func TestScaleSimulatorStatefulSet(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "data"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "db", Image: "pg:v1", Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1000m")},
						}},
					}},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("32"), corev1.ResourceMemory: resource.MustParse("128Gi"),
			}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scale-simulator?workload=db&namespace=data&replicas=5", clientset)
	w := httptest.NewRecorder()
	s.handleScaleSimulator(w, req)

	var result ScaleSimResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.CurrentState.Replicas != 3 {
		t.Errorf("expected current 3, got %d", result.CurrentState.Replicas)
	}
	if result.Verdict != "can-scale" {
		t.Errorf("expected can-scale, got %s", result.Verdict)
	}
}
