package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestMeshTraffic_NoMesh(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/mesh-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleMeshTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result MeshTrafficResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.HasIstio || result.Summary.HasLinkerd {
		t.Error("expected no mesh detected")
	}
	if result.HealthScore > 60 {
		t.Errorf("expected low health score without mesh, got %d", result.HealthScore)
	}
}

func TestMeshTraffic_WithIstioSidecar(t *testing.T) {
	httpProto := "http"
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "istio-system",
				Labels: map[string]string{},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "app-prod",
				Labels: map[string]string{"istio-injection": "enabled"},
			},
		},
		// Pod with Istio sidecar
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx"},
					{Name: "istio-proxy", Image: "istio/proxyv2:1.20"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Service with mesh
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-svc", Namespace: "app-prod"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, AppProtocol: &httpProto},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/mesh-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleMeshTraffic(rec, req)

	var result MeshTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasIstio {
		t.Error("expected Istio to be detected")
	}
	if result.Summary.NamespacesWithMesh < 1 {
		t.Errorf("expected at least 1 namespace with mesh, got %d", result.Summary.NamespacesWithMesh)
	}
	if result.Summary.ServicesWithMesh < 1 {
		t.Errorf("expected at least 1 service with mesh, got %d", result.Summary.ServicesWithMesh)
	}
}

func TestMeshTraffic_WithLinkerd(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "linkerd"}},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "app-prod",
				Labels: map[string]string{"linkerd.io/inject": "enabled"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-pod",
				Namespace:   "app-prod",
				Annotations: map[string]string{"linkerd.io/inject": "enabled"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx"},
					{Name: "linkerd-proxy", Image: "linkerd/proxy:2.14"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/mesh-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleMeshTraffic(rec, req)

	var result MeshTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasLinkerd {
		t.Error("expected Linkerd to be detected")
	}
}

func TestMeshTraffic_MixedNamespaces(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "istio-system"}},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "app-mesh",
				Labels: map[string]string{"istio-injection": "enabled"},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-no-mesh"}},
		// Pod in mesh namespace
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "mesh-pod", Namespace: "app-mesh"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx"},
					{Name: "istio-proxy", Image: "istio/proxyv2:1.20"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Pod in non-mesh namespace
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "no-mesh-pod", Namespace: "app-no-mesh"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/mesh-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleMeshTraffic(rec, req)

	var result MeshTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NamespacesWithMesh < 1 {
		t.Errorf("expected at least 1 namespace with mesh, got %d", result.Summary.NamespacesWithMesh)
	}
	if result.Summary.NamespacesNoMesh < 1 {
		t.Errorf("expected at least 1 namespace without mesh, got %d", result.Summary.NamespacesNoMesh)
	}

	// Should have a gap for the non-mesh namespace
	foundGap := false
	for _, gap := range result.Gaps {
		if gap.Namespace == "app-no-mesh" {
			foundGap = true
		}
	}
	if !foundGap {
		t.Error("expected to find gap for app-no-mesh namespace")
	}
}

func TestMeshTraffic_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/product/mesh-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleMeshTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result MeshTrafficResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.HasIstio || result.Summary.HasLinkerd {
		t.Error("expected no mesh in empty cluster")
	}
}
