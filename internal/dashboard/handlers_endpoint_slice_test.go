package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestEndpointSlice_HealthyService(t *testing.T) {
	ready := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-svc", Namespace: "app-prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "test"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-svc-abc",
				Namespace: "app-prod",
				Labels:    map[string]string{"kubernetes.io/service-name": "app-svc"},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Conditions: discoveryv1.EndpointConditions{Ready: &ready},
					Addresses:  []string{"10.0.0.1"},
				},
				{
					Conditions: discoveryv1.EndpointConditions{Ready: &ready},
					Addresses:  []string{"10.0.0.2"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-slice", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointSlice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EndpointSliceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalServices != 1 {
		t.Errorf("expected 1 service, got %d", result.Summary.TotalServices)
	}
	if result.Summary.ServicesWithEndpoints != 1 {
		t.Errorf("expected 1 service with endpoints, got %d", result.Summary.ServicesWithEndpoints)
	}
	if result.Summary.ReadyEndpoints != 2 {
		t.Errorf("expected 2 ready endpoints, got %d", result.Summary.ReadyEndpoints)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestEndpointSlice_NoEndpoints(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-empty-svc", Namespace: "app-prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "nonexistent"},
				Ports:    []corev1.ServicePort{{Port: 80}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-slice", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointSlice(rec, req)

	var result EndpointSliceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ServicesNoEndpoints != 1 {
		t.Errorf("expected 1 service with no endpoints, got %d", result.Summary.ServicesNoEndpoints)
	}
	found := false
	for _, g := range result.Gaps {
		if g.Service == "app-empty-svc" && g.Severity == "high" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find gap for app-empty-svc")
	}
}

func TestEndpointSlice_NotReadyEndpoints(t *testing.T) {
	ready := false
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-svc", Namespace: "app-prod"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-svc-abc",
				Namespace: "app-prod",
				Labels:    map[string]string{"kubernetes.io/service-name": "app-svc"},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Conditions: discoveryv1.EndpointConditions{Ready: &ready},
					Addresses:  []string{"10.0.0.1"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-slice", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointSlice(rec, req)

	var result EndpointSliceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NotReadyEndpoints != 1 {
		t.Errorf("expected 1 not ready endpoint, got %d", result.Summary.NotReadyEndpoints)
	}
}

func TestEndpointSlice_TopologyHints(t *testing.T) {
	ready := true
	zone := "us-east-1a"
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "app-svc", Namespace: "app-prod"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-svc-abc",
				Namespace: "app-prod",
				Labels:    map[string]string{"kubernetes.io/service-name": "app-svc"},
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Conditions: discoveryv1.EndpointConditions{Ready: &ready},
					Addresses:  []string{"10.0.0.1"},
					Zone:       &zone,
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-slice", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointSlice(rec, req)

	var result EndpointSliceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithTopologyHints != 1 {
		t.Errorf("expected 1 with topology hints, got %d", result.Summary.WithTopologyHints)
	}
}

func TestEndpointSlice_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/product/endpoint-slice", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEndpointSlice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result EndpointSliceResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalServices != 0 {
		t.Errorf("expected 0 services, got %d", result.Summary.TotalServices)
	}
}
