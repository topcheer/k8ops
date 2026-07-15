package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestSchedulingFit_NoRequests(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scheduling-fit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSchedulingFit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SchedulingFitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NoRequest != 1 {
		t.Errorf("expected 1 pod with no request, got %d", result.Summary.NoRequest)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestSchedulingFit_OptimalPacking(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scheduling-fit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSchedulingFit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SchedulingFitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if len(result.ByNode) != 1 {
		t.Fatalf("expected 1 node stat, got %d", len(result.ByNode))
	}
	if result.ByNode[0].FitCategory != "optimal" {
		t.Errorf("expected optimal fit, got %s (packing: %.1f%%)", result.ByNode[0].FitCategory, result.ByNode[0].PackingPct)
	}
}

func TestSchedulingFit_Overpacked(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "big-app", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("900m"),
							corev1.ResourceMemory: resource.MustParse("1800Mi"),
						},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scheduling-fit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSchedulingFit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SchedulingFitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.OverpackedNodes != 1 {
		t.Errorf("expected 1 overpacked node, got %d", result.Summary.OverpackedNodes)
	}
}

func TestSchedulingFit_OverProvisioned(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("16"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "big-app", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "nginx",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("16Gi"),
						},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/scheduling-fit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSchedulingFit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SchedulingFitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.OverProvisioned != 1 {
		t.Errorf("expected 1 over-provisioned pod, got %d", result.Summary.OverProvisioned)
	}
	if len(result.OverProvisioned) != 1 {
		t.Errorf("expected 1 over-provisioned entry, got %d", len(result.OverProvisioned))
	}
}
