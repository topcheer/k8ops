package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRuntimeClass_ImageCompliance(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-latest", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:latest"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pinned", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-untrusted", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "random-registry.io/app:v1"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/runtime-class", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeClass(rec, req)

	var result RuntimeClassResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ImagesWithLatest != 1 {
		t.Errorf("expected 1 image with latest, got %d", result.Summary.ImagesWithLatest)
	}

	foundUntrusted := false
	for _, ic := range result.ImageCompliance {
		if ic.Issue == "Image from untrusted registry" {
			foundUntrusted = true
		}
	}
	if !foundUntrusted {
		t.Error("expected to find untrusted registry image")
	}
}

func TestRuntimeClass_WithRuntimeClass(t *testing.T) {
	runtimeClassName := "kata"
	clientset := k8sfake.NewSimpleClientset(
		&nodev1.RuntimeClass{
			ObjectMeta: metav1.ObjectMeta{Name: "kata-runtime"},
			Handler:    "kata",
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-sandboxed", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				RuntimeClassName: &runtimeClassName,
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/app:v1.0"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-default", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/app:v1.0"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/runtime-class", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeClass(rec, req)

	var result RuntimeClassResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalRuntimeClasses != 1 {
		t.Errorf("expected 1 runtime class, got %d", result.Summary.TotalRuntimeClasses)
	}
	if !result.Summary.HasKata {
		t.Error("expected kata handler to be detected")
	}
	if result.Summary.PodsUsingRuntime != 1 {
		t.Errorf("expected 1 pod using runtime, got %d", result.Summary.PodsUsingRuntime)
	}
	if result.Summary.PodsNoRuntime != 1 {
		t.Errorf("expected 1 pod without runtime, got %d", result.Summary.PodsNoRuntime)
	}
}

func TestRuntimeClass_NodeRuntime(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					ContainerRuntimeVersion: "containerd://1.7.0",
					OSImage:                 "Ubuntu 22.04",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					ContainerRuntimeVersion: "cri-o://1.27.0",
					OSImage:                 "RHEL 9.2",
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/runtime-class", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeClass(rec, req)

	var result RuntimeClassResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasContainerd {
		t.Error("expected containerd to be detected")
	}
	if !result.Summary.HasCrio {
		t.Error("expected cri-o to be detected")
	}
	if len(result.ByNode) != 2 {
		t.Errorf("expected 2 node stats, got %d", len(result.ByNode))
	}
}

func TestRuntimeClass_HealthScore(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-clean", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.k8s.io/nginx:1.25.3"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/runtime-class", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeClass(rec, req)

	var result RuntimeClassResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.HealthScore < 90 {
		t.Errorf("expected high health score for clean images, got %d", result.HealthScore)
	}
}

func TestRuntimeClass_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/product/runtime-class", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeClass(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result RuntimeClassResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalPods != 0 {
		t.Errorf("expected 0 pods, got %d", result.Summary.TotalPods)
	}
}
