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
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestGatewayAuditEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/gateway-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleGatewayAudit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result GatewayAuditResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestGatewayAuditOrphanRoute(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-ingress", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{Host: "app.example.com", IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{Path: "/", Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{Name: "missing-svc"},
								}},
							},
						},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/gateway-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGatewayAudit(w, req)
	var result GatewayAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.OrphanRoutes) == 0 {
		t.Error("expected orphan routes")
	}
}

func TestGatewayAuditTLSGap(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "notls", Namespace: "prod"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{
					{Host: "public.example.com", IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{Path: "/", Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{Name: "web"},
								}},
							},
						},
					}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/gateway-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGatewayAudit(w, req)
	var result GatewayAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.WithoutTLS != 1 {
		t.Errorf("expected 1 without TLS, got %d", result.Summary.WithoutTLS)
	}
	found := false
	for _, gap := range result.TLSGaps {
		if gap.Host == "public.example.com" {
			found = true
		}
	}
	if !found {
		t.Error("expected TLS gap for public host")
	}
}

func TestGatewayAuditHostConflict(t *testing.T) {
	ing1 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing1", Namespace: "prod"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "shared.example.com", IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{Path: "/a", Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{Name: "svc-a"}}},
						},
					},
				}},
			},
		},
	}
	ing2 := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing2", Namespace: "prod"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "shared.example.com", IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{Path: "/b", Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{Name: "svc-b"}}},
						},
					},
				}},
			},
		},
	}
	clientset := k8sfake.NewSimpleClientset(ing1, ing2,
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "prod"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "prod"}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/gateway-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGatewayAudit(w, req)
	var result GatewayAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.HostConflicts == 0 {
		t.Error("expected host conflict")
	}
}

func TestGatewayAuditControllerDetection(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-ingress-controller-abc", Namespace: "ingress-nginx"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "controller", Ready: true}}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/gateway-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGatewayAudit(w, req)
	var result GatewayAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	found := false
	for _, c := range result.Controllers {
		if c.Type == "nginx" && c.Healthy {
			found = true
		}
	}
	if !found {
		t.Error("expected nginx controller detected")
	}
}

func TestGatewayAuditRecommendations(t *testing.T) {
	result := GatewayAuditResult{
		Summary: GatewaySummary{
			WithoutTLS: 3, HostConflicts: 1, UnhealthyIngresses: 2,
		},
		OrphanRoutes: []OrphanRoute{{Ingress: "bad"}},
	}
	recs := generateGatewayRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundTLS := false
	foundOrphan := false
	for _, r := range recs {
		l := strings.ToLower(r)
		if strings.Contains(l, "tls") {
			foundTLS = true
		}
		if strings.Contains(l, "orphan") {
			foundOrphan = true
		}
	}
	if !foundTLS {
		t.Error("expected TLS recommendation")
	}
	if !foundOrphan {
		t.Error("expected orphan recommendation")
	}
}
