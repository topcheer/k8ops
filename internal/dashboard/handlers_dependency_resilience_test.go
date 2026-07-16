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

func TestDepResilienceEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestDepResilienceOrphaned(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Selector:  map[string]string{"app": "nonexistent"},
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", clientset)
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.OrphanedServices == 0 {
		t.Error("Should detect orphaned service")
	}
}

func TestDepResilienceMultiBackend(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "good-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.2",
				Selector:  map[string]string{"app": "good"},
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "good-svc", Namespace: "default"},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{IP: "10.1.0.1"},
						{IP: "10.1.0.2"},
					},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", clientset)
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.MultiBackendSvc == 0 {
		t.Error("Should detect multi-backend service")
	}
}

func TestDepResilienceExternalName(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "ext-svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceTypeExternalName,
				ExternalName: "api.external.com",
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", clientset)
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.CrossNSServices == 0 {
		t.Error("Should detect ExternalName service")
	}
}

func TestDepResilienceSystemNSExcluded(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.10",
				Selector:  map[string]string{"k8s-app": "kube-dns"},
				Ports:     []corev1.ServicePort{{Port: 53}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", clientset)
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.TotalServices != 0 {
		t.Errorf("kube-system should be excluded, got %d services", result.Summary.TotalServices)
	}
}

func TestDepResilienceRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.1",
				Selector:  map[string]string{"app": "missing"},
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/dependency-resilience", clientset)
	w := httptest.NewRecorder()
	s.handleDependencyResilience(w, req)

	var result DependencyResilienceResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}
