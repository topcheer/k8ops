package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestIngressTLS_NoTLS(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "app-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{Host: "app.example.com"}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/ingress-tls", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleIngressTLS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result IngressTLSResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithoutTLS != 1 {
		t.Errorf("expected 1 without TLS, got %d", result.Summary.WithoutTLS)
	}
}

func TestIngressTLS_WithTLS(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-ingress", Namespace: "default",
				Annotations: map[string]string{
					"cert-manager.io/cluster-issuer":           "letsencrypt-prod",
					"nginx.ingress.kubernetes.io/ssl-redirect": "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"app.example.com"},
					SecretName: "app-tls",
				}},
				Rules: []networkingv1.IngressRule{{Host: "app.example.com"}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/ingress-tls", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleIngressTLS(rec, req)

	var result IngressTLSResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithTLS != 1 {
		t.Errorf("expected 1 with TLS, got %d", result.Summary.WithTLS)
	}
	if !result.Ingresses[0].HasCertManager {
		t.Error("expected cert-manager to be detected")
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestIngressTLS_TLSMismatch(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "app-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"api.example.com"},
					SecretName: "api-tls",
				}},
				Rules: []networkingv1.IngressRule{
					{Host: "api.example.com"},
					{Host: "admin.example.com"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/ingress-tls", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleIngressTLS(rec, req)

	var result IngressTLSResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	found := false
	for _, g := range result.Gaps {
		if g.Issue == "Rule host admin.example.com not covered by TLS certificate" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find TLS mismatch for admin.example.com")
	}
}

func TestIngressTLS_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/product/ingress-tls", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleIngressTLS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result IngressTLSResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalIngresses != 0 {
		t.Errorf("expected 0 ingresses, got %d", result.Summary.TotalIngresses)
	}
}
