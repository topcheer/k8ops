package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestCertManager_NotInstalled(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/product/cert-manager", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCertManager(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result CertManagerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.CertManagerInstalled {
		t.Error("expected cert-manager not installed")
	}
	if result.HealthScore > 85 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestCertManager_Installed(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "cert-manager"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/cert-manager", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCertManager(rec, req)

	var result CertManagerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.CertManagerInstalled {
		t.Error("expected cert-manager to be detected")
	}
}

func TestCertManager_TLSSecretWithoutAnnotation(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tls-cert", Namespace: "app-prod"},
			Type:       corev1.SecretTypeTLS,
			Data: map[string][]byte{
				corev1.TLSCertKey:       []byte("fake-cert"),
				corev1.TLSPrivateKeyKey: []byte("fake-key"),
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/cert-manager", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCertManager(rec, req)

	var result CertManagerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TLSSecrets != 1 {
		t.Errorf("expected 1 TLS secret, got %d", result.Summary.TLSSecrets)
	}
	if result.Summary.WithoutRenewal != 1 {
		t.Errorf("expected 1 without renewal, got %d", result.Summary.WithoutRenewal)
	}
}

func TestCertManager_ExpiringCert(t *testing.T) {
	expiryTime := time.Now().Add(15 * 24 * time.Hour) // 15 days
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "expiring-cert",
				Namespace: "app-prod",
				Annotations: map[string]string{
					"cert-manager.io/not-after":   expiryTime.Format(time.RFC3339),
					"cert-manager.io/issuer-name": "letsencrypt-prod",
				},
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				corev1.TLSCertKey:       []byte("fake-cert"),
				corev1.TLSPrivateKeyKey: []byte("fake-key"),
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/cert-manager", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCertManager(rec, req)

	var result CertManagerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ExpiringSoon != 1 {
		t.Errorf("expected 1 expiring soon, got %d", result.Summary.ExpiringSoon)
	}
	found := false
	for _, g := range result.Gaps {
		if g.Name == "expiring-cert" && g.Severity == "high" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find expiring cert gap with high severity")
	}
}

func TestCertManager_ExpiredCert(t *testing.T) {
	expiryTime := time.Now().Add(-5 * 24 * time.Hour) // expired 5 days ago
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "expired-cert",
				Namespace: "app-prod",
				Annotations: map[string]string{
					"cert-manager.io/not-after": expiryTime.Format(time.RFC3339),
				},
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				corev1.TLSCertKey: []byte("fake-cert"),
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/cert-manager", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCertManager(rec, req)

	var result CertManagerResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.Expired != 1 {
		t.Errorf("expected 1 expired, got %d", result.Summary.Expired)
	}
	if result.HealthScore > 70 {
		t.Errorf("expected reduced health score for expired cert, got %d", result.HealthScore)
	}
}
