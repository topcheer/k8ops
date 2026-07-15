package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestExposureMapEmpty verifies empty cluster behavior.
func TestExposureMapEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.RiskScore != 100 {
		t.Errorf("expected risk score 100, got %d", result.RiskScore)
	}
}

// TestExposureMapWithIngress verifies ingress detection.
func TestExposureMapWithIngress(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "web-ingress", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{Hosts: []string{"app.example.com"}}},
				Rules: []networkingv1.IngressRule{
					{
						Host: "app.example.com",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path: "/",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{
												Name: "web-svc",
												Port: networkingv1.ServiceBackendPort{Number: 80},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web-svc", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "web"},
				Type:     corev1.ServiceTypeClusterIP,
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-pod", Namespace: "prod",
				Labels: map[string]string{"app": "web"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalIngresses != 1 {
		t.Errorf("expected 1 ingress, got %d", result.Summary.TotalIngresses)
	}

	// Should trace to backend workload
	found := false
	for _, e := range result.EntryPoints {
		if e.Type == "ingress" && e.BackendWorkload != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected to trace ingress to backend workload")
	}

	// Should have TLS since we configured it
	if result.Summary.WithTLS != 1 {
		t.Errorf("expected 1 with TLS, got %d", result.Summary.WithTLS)
	}
}

// TestExposureMapNoTLS verifies detection of insecure exposure.
func TestExposureMapNoTLS(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "insecure", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: "api.example.com",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{
										Path: "/api",
										Backend: networkingv1.IngressBackend{
											Service: &networkingv1.IngressServiceBackend{Name: "api-svc"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.WithoutTLS != 1 {
		t.Errorf("expected 1 without TLS, got %d", result.Summary.WithoutTLS)
	}

	// Risk score should be < 100
	if result.RiskScore >= 100 {
		t.Errorf("expected risk score < 100 with no TLS, got %d", result.RiskScore)
	}
}

// TestExposureMapLoadBalancer verifies LB detection.
func TestExposureMapLoadBalancer(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "lb-svc", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeLoadBalancer,
				Selector: map[string]string{"app": "lb"},
				Ports: []corev1.ServicePort{
					{Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(8443)},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "lb-pod", Namespace: "prod",
				Labels: map[string]string{"app": "lb"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalLoadBalancers != 1 {
		t.Errorf("expected 1 LB, got %d", result.Summary.TotalLoadBalancers)
	}

	// Should be classified as high risk
	foundLB := false
	for _, e := range result.EntryPoints {
		if e.Type == "loadbalancer" {
			foundLB = true
			if e.RiskLevel != "high" {
				t.Errorf("expected LB risk high, got %s", e.RiskLevel)
			}
		}
	}
	if !foundLB {
		t.Error("expected to find loadbalancer entry")
	}
}

// TestExposureMapHighRiskPath verifies sensitive path detection.
func TestExposureMapHighRiskPath(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "admin-ingress", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: "admin.example.com",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{Path: "/admin", Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "admin-svc"}}},
									{Path: "/debug/pprof", Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "admin-svc"}}},
								},
							},
						},
					},
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.HighRiskPaths) < 2 {
		t.Errorf("expected >=2 high-risk paths, got %d", len(result.HighRiskPaths))
	}
}

// TestExposureMapOrphanEndpoint verifies orphan detection.
func TestExposureMapOrphanEndpoint(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan-ingress", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{
						Host: "orphan.example.com",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{
									{Path: "/", Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "missing-svc"}}},
								},
							},
						},
					},
				},
			},
		},
		// No pods matching the service selector → orphan
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-svc", Namespace: "prod"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "nonexistent"}},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect orphan endpoint (service exists but no pods)
	if len(result.OrphanExposure) == 0 {
		t.Error("expected orphan exposure detection")
	}
}

// TestExposureMapNodePort verifies NodePort detection.
func TestExposureMapNodePort(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "nodeport-svc", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeNodePort,
				Selector: map[string]string{"app": "np"},
				Ports: []corev1.ServicePort{
					{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "np-pod", Namespace: "prod",
				Labels: map[string]string{"app": "np"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/product/exposure-map", clientset)
	w := httptest.NewRecorder()
	s.handleExposureMap(w, req)

	var result ExposureMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNodePorts != 1 {
		t.Errorf("expected 1 NodePort, got %d", result.Summary.TotalNodePorts)
	}
}

// TestExposureMapRecommendations verifies recommendation generation.
func TestExposureMapRecommendations(t *testing.T) {
	result := ExposureMapResult{
		Summary: ExposureSummary{
			WithoutTLS:         5,
			TotalLoadBalancers: 2,
			TotalIngresses:     10,
			WithoutAuth:        8,
		},
		OrphanExposure: []OrphanEndpoint{{Name: "orphan-1"}},
		HighRiskPaths:  []HighRiskPath{{Path: "/admin"}},
		RiskScore:      35,
	}

	recs := generateExposureRecommendations(result)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundTLS := false
	foundLB := false
	foundOrphan := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "tls") {
			foundTLS = true
		}
		if strings.Contains(lower, "loadbalancer") {
			foundLB = true
		}
		if strings.Contains(lower, "orphan") {
			foundOrphan = true
		}
	}
	if !foundTLS {
		t.Error("expected TLS recommendation")
	}
	if !foundLB {
		t.Error("expected LB recommendation")
	}
	if !foundOrphan {
		t.Error("expected orphan recommendation")
	}
}

// TestClassifyExposure verifies host classification.
func TestClassifyExposure(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"", "internal"},
		{"localhost", "internal"},
		{"app.example.com", "public"},
		{"api.internal.corp", "internal"},
		{"service.local", "internal"},
	}

	for _, tt := range tests {
		got := classifyExposure(tt.host)
		if got != tt.expected {
			t.Errorf("classifyExposure(%q) = %s, want %s", tt.host, got, tt.expected)
		}
	}
}

// TestIsHighRiskPath verifies sensitive path detection.
func TestIsHighRiskPath(t *testing.T) {
	highRisk := []string{"/admin", "/debug", "/metrics", "/actuator", "/swagger", "/.env"}
	safe := []string{"/", "/api/v1/users", "/health", "/static"}

	for _, p := range highRisk {
		if !isHighRiskPath(p) {
			t.Errorf("isHighRiskPath(%q) = false, want true", p)
		}
	}
	for _, p := range safe {
		if isHighRiskPath(p) {
			t.Errorf("isHighRiskPath(%q) = true, want false", p)
		}
	}
}
