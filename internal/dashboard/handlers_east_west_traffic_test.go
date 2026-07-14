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

func TestEastWestTraffic_NoServices(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/east-west-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEastWestTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EastWestTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalServices != 0 {
		t.Errorf("expected 0 services, got %d", result.Summary.TotalServices)
	}
	if result.HealthScore < 80 {
		t.Errorf("expected high health score with no services, got %d", result.HealthScore)
	}
}

func TestEastWestTraffic_WithExposedService(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "internal-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.43.0.1",
				Ports:     []corev1.ServicePort{{Port: 8080, Protocol: corev1.ProtocolTCP}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/east-west-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEastWestTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EastWestTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalServices != 2 {
		t.Errorf("expected 2 services, got %d", result.Summary.TotalServices)
	}
	if result.Summary.LoadBalancerSvcs != 1 {
		t.Errorf("expected 1 LB service, got %d", result.Summary.LoadBalancerSvcs)
	}
	if result.Summary.PubliclyExposed != 1 {
		t.Errorf("expected 1 publicly exposed, got %d", result.Summary.PubliclyExposed)
	}
	if result.Summary.WithoutNetworkPolicy < 2 {
		t.Errorf("expected 2 services without NP, got %d", result.Summary.WithoutNetworkPolicy)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestEastWestTraffic_WithNetworkPolicy(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.43.0.2",
				Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/east-west-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEastWestTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EastWestTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ClusterIPServices != 1 {
		t.Errorf("expected 1 ClusterIP service, got %d", result.Summary.ClusterIPServices)
	}
	if result.Summary.InternalOnly != 1 {
		t.Errorf("expected 1 internal-only service, got %d", result.Summary.InternalOnly)
	}
}

func TestEastWestTraffic_ExternalName(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "ext-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "external.example.com",
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/east-west-traffic", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEastWestTraffic(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EastWestTrafficResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ExternalNameSvcs != 1 {
		t.Errorf("expected 1 ExternalName service, got %d", result.Summary.ExternalNameSvcs)
	}
	// Should have a risk about ExternalName bypassing cluster networking
	foundRisk := false
	for _, risk := range result.Risks {
		if risk.Service == "ext-svc" && risk.Severity == "warning" {
			foundRisk = true
		}
	}
	if !foundRisk {
		t.Error("expected risk about ExternalName bypassing cluster networking")
	}
}
