package dashboard

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// genCertForTest creates a self-signed certificate valid for the given duration.
func genCertForTest(t *testing.T, notBefore, notAfter time.Time, dnsNames []string) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-cert"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              dnsNames,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
}

func TestParseCertFromPEM_Valid(t *testing.T) {
	now := time.Now()
	certPEM := genCertForTest(t, now.Add(-24*time.Hour), now.Add(90*24*time.Hour), []string{"example.com"})

	info := parseCertFromPEM(certPEM)

	if info.Level != CertLevelOK {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelOK)
	}
	if info.DaysUntilExpiry <= 0 {
		t.Errorf("DaysUntilExpiry = %d, want positive", info.DaysUntilExpiry)
	}
	if info.Subject != "test-cert" {
		t.Errorf("Subject = %q, want %q", info.Subject, "test-cert")
	}
	if len(info.DNSNames) != 1 || info.DNSNames[0] != "example.com" {
		t.Errorf("DNSNames = %v, want [example.com]", info.DNSNames)
	}
	if info.Error != "" {
		t.Errorf("Error = %q, want empty", info.Error)
	}
}

func TestParseCertFromPEM_Expired(t *testing.T) {
	now := time.Now()
	certPEM := genCertForTest(t, now.Add(-60*24*time.Hour), now.Add(-10*24*time.Hour), nil)

	info := parseCertFromPEM(certPEM)

	if info.Level != CertLevelExpired {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelExpired)
	}
	if info.DaysUntilExpiry >= 0 {
		t.Errorf("DaysUntilExpiry = %d, want negative", info.DaysUntilExpiry)
	}
}

func TestParseCertFromPEM_Critical(t *testing.T) {
	now := time.Now()
	// Expires in 3 days (<7 days threshold)
	certPEM := genCertForTest(t, now.Add(-30*24*time.Hour), now.Add(3*24*time.Hour), nil)

	info := parseCertFromPEM(certPEM)

	if info.Level != CertLevelCritical {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelCritical)
	}
	if info.DaysUntilExpiry < 0 || info.DaysUntilExpiry >= 7 {
		t.Errorf("DaysUntilExpiry = %d, want 0-6", info.DaysUntilExpiry)
	}
}

func TestParseCertFromPEM_Warning(t *testing.T) {
	now := time.Now()
	// Expires in 20 days (7-30 days threshold)
	certPEM := genCertForTest(t, now.Add(-30*24*time.Hour), now.Add(20*24*time.Hour), nil)

	info := parseCertFromPEM(certPEM)

	if info.Level != CertLevelWarning {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelWarning)
	}
}

func TestParseCertFromPEM_OK(t *testing.T) {
	now := time.Now()
	// Expires in 100 days (>30 days)
	certPEM := genCertForTest(t, now.Add(-10*24*time.Hour), now.Add(100*24*time.Hour), nil)

	info := parseCertFromPEM(certPEM)

	if info.Level != CertLevelOK {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelOK)
	}
}

func TestParseCertFromPEM_InvalidData(t *testing.T) {
	info := parseCertFromPEM([]byte("not a certificate"))

	if info.Level != CertLevelError {
		t.Errorf("Level = %q, want %q", info.Level, CertLevelError)
	}
	if info.Error == "" {
		t.Errorf("Error = empty, want non-empty for invalid data")
	}
}

func TestCertLevelPriority(t *testing.T) {
	// Lower priority = more urgent = sorts first
	if certLevelPriority(CertLevelExpired) >= certLevelPriority(CertLevelCritical) {
		t.Error("expired should have higher priority than critical")
	}
	if certLevelPriority(CertLevelCritical) >= certLevelPriority(CertLevelWarning) {
		t.Error("critical should have higher priority than warning")
	}
	if certLevelPriority(CertLevelWarning) >= certLevelPriority(CertLevelOK) {
		t.Error("warning should have higher priority than ok")
	}
}

func TestParseCertFromPEM_MultipleDNS(t *testing.T) {
	now := time.Now()
	dnsNames := []string{"api.example.com", "www.example.com", "example.com"}
	certPEM := genCertForTest(t, now.Add(-10*24*time.Hour), now.Add(200*24*time.Hour), dnsNames)

	info := parseCertFromPEM(certPEM)

	if len(info.DNSNames) != 3 {
		t.Errorf("DNSNames count = %d, want 3", len(info.DNSNames))
	}
}

// --- Handler integration test ---

func TestHandleCertExpiryScan_FullFlow(t *testing.T) {
	now := time.Now()
	okCert := genCertForTest(t, now.Add(-10*24*time.Hour), now.Add(100*24*time.Hour), []string{"ok.example.com"})
	criticalCert := genCertForTest(t, now.Add(-50*24*time.Hour), now.Add(3*24*time.Hour), []string{"critical.example.com"})
	expiredCert := genCertForTest(t, now.Add(-100*24*time.Hour), now.Add(-5*24*time.Hour), []string{"expired.example.com"})

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ok-cert", Namespace: "default"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{"tls.crt": okCert, "tls.key": []byte("fake")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "critical-cert", Namespace: "default"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{"tls.crt": criticalCert, "tls.key": []byte("fake")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "expired-cert", Namespace: "kube-system"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{"tls.crt": expiredCert, "tls.key": []byte("fake")},
		},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "test-ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{
					{Hosts: []string{"ok.example.com"}, SecretName: "ok-cert"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/certificates/expiry", clientset)
	rr := httptest.NewRecorder()

	s := &Server{}
	s.handleCertExpiryScan(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}

	var summary CertExpirySummary
	if err := json.Unmarshal(rr.Body.Bytes(), &summary); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}
	if summary.Expired != 1 {
		t.Errorf("Expired = %d, want 1", summary.Expired)
	}
	if summary.Critical != 1 {
		t.Errorf("Critical = %d, want 1", summary.Critical)
	}
	if summary.OK != 1 {
		t.Errorf("OK = %d, want 1", summary.OK)
	}

	// Verify sort order: expired first
	if summary.Certificates[0].Level != CertLevelExpired {
		t.Errorf("First cert level = %q, want %q (expired should sort first)",
			summary.Certificates[0].Level, CertLevelExpired)
	}

	// Verify ingress reference is detected
	for _, c := range summary.Certificates {
		if c.Name == "ok-cert" {
			if len(c.UsedByIngress) != 1 || c.UsedByIngress[0] != "test-ingress" {
				t.Errorf("ok-cert UsedByIngress = %v, want [test-ingress]", c.UsedByIngress)
			}
		}
	}
}

func TestHandleCertExpiryScan_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/certificates/expiry", clientset)
	rr := httptest.NewRecorder()

	s := &Server{}
	s.handleCertExpiryScan(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}

	var summary CertExpirySummary
	if err := json.Unmarshal(rr.Body.Bytes(), &summary); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if summary.Total != 0 {
		t.Errorf("Total = %d, want 0 for empty cluster", summary.Total)
	}
}
