package dashboard

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateTestCert creates a self-signed cert/key pair for testing.
// Returns (certPath, keyPath, cleanupFunc).
func generateTestCert(t *testing.T) (string, string, func()) {
	t.Helper()
	dir := t.TempDir()

	// Generate ECDSA private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create self-signed certificate
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "k8ops-test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", "127.0.0.1"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	// Write certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}

	// Write private key
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	cleanup := func() { os.RemoveAll(dir) }
	return certPath, keyPath, cleanup
}

func TestSetTLS(t *testing.T) {
	s := &Server{}

	// Before SetTLS, IsTLS should be false
	if s.IsTLS() {
		t.Error("IsTLS() should be false before SetTLS")
	}

	s.SetTLS("/path/to/cert.pem", "/path/to/key.pem")

	if !s.IsTLS() {
		t.Error("IsTLS() should be true after SetTLS")
	}
	if s.tlsCert != "/path/to/cert.pem" {
		t.Errorf("tlsCert = %q, want /path/to/cert.pem", s.tlsCert)
	}
	if s.tlsKey != "/path/to/key.pem" {
		t.Errorf("tlsKey = %q, want /path/to/key.pem", s.tlsKey)
	}
}

func TestTLSConfigFromEnv(t *testing.T) {
	certPath, keyPath, cleanup := generateTestCert(t)
	defer cleanup()

	// Simulate env-based TLS configuration
	t.Setenv("DASHBOARD_TLS_CERT", certPath)
	t.Setenv("DASHBOARD_TLS_KEY", keyPath)

	cert := os.Getenv("DASHBOARD_TLS_CERT")
	key := os.Getenv("DASHBOARD_TLS_KEY")

	s := &Server{}
	if cert != "" && key != "" {
		s.SetTLS(cert, key)
	}

	if !s.IsTLS() {
		t.Error("server should have TLS configured from env vars")
	}

	// Verify cert file is readable
	if _, err := os.Stat(cert); err != nil {
		t.Errorf("cert file not accessible: %v", err)
	}
	if _, err := os.Stat(key); err != nil {
		t.Errorf("key file not accessible: %v", err)
	}
}

func TestTLSPartialConfig(t *testing.T) {
	s := &Server{}

	// Only cert, no key → should not enable TLS
	s.SetTLS("/path/to/cert.pem", "")
	if s.IsTLS() {
		t.Error("IsTLS() should be false when only cert is set")
	}

	// Only key, no cert → should not enable TLS
	s.SetTLS("", "/path/to/key.pem")
	if s.IsTLS() {
		t.Error("IsTLS() should be false when only key is set")
	}

	// Both empty → should not enable TLS
	s.SetTLS("", "")
	if s.IsTLS() {
		t.Error("IsTLS() should be false when neither is set")
	}
}
