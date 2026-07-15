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

func TestEndpointMismatch_HealthyService(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "web"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "web", Image: "nginx"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
			}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-mismatch", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointMismatch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EndpointMismatchResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HealthyServices != 1 {
		t.Errorf("expected 1 healthy service, got %d", result.Summary.HealthyServices)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestEndpointMismatch_DeadService(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports:    []corev1.ServicePort{{Port: 8080}},
			},
		},
		// No pods matching selector, no endpoints
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Subsets:    []corev1.EndpointSubset{},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-mismatch", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointMismatch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EndpointMismatchResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.DeadServices != 1 {
		t.Errorf("expected 1 dead service, got %d", result.Summary.DeadServices)
	}
	if len(result.DeadServices) != 1 {
		t.Errorf("expected 1 dead service entry, got %d", len(result.DeadServices))
	}
	if result.HealthScore >= 90 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestEndpointMismatch_MismatchedService(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "web"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		// 2 ready pods matching selector
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "web", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-2", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Spec:       corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "web", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true}}},
		},
		// But only 1 ready endpoint — mismatch
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
			}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-mismatch", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointMismatch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EndpointMismatchResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MismatchedServices != 1 {
		t.Errorf("expected 1 mismatched service, got %d", result.Summary.MismatchedServices)
	}
}

func TestEndpointMismatch_NoSelector(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "external-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "external.example.com",
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-mismatch", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointMismatch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EndpointMismatchResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NoSelector != 1 {
		t.Errorf("expected 1 service without selector, got %d", result.Summary.NoSelector)
	}
}
