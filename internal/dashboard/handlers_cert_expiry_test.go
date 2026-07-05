package dashboard

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestCertExpRisk(t *testing.T) {
	// Already expired
	if level := certExpRisk(-5); level != "critical" {
		t.Errorf("Expected critical for expired, got %s", level)
	}
	// Expires in 10 days
	if level := certExpRisk(10); level != "critical" {
		t.Errorf("Expected critical for 10 days, got %s", level)
	}
	// Expires in 45 days
	if level := certExpRisk(45); level != "high" {
		t.Errorf("Expected high for 45 days, got %s", level)
	}
	// Expires in 75 days
	if level := certExpRisk(75); level != "medium" {
		t.Errorf("Expected medium for 75 days, got %s", level)
	}
	// Healthy
	if level := certExpRisk(180); level != "low" {
		t.Errorf("Expected low for 180 days, got %s", level)
	}
}

func TestCertExpScore(t *testing.T) {
	// No certs
	if score := certExpScore(CertExpSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// Clean
	s := CertExpSummary{TotalCerts: 5, Healthy: 5}
	if score := certExpScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With expired
	s = CertExpSummary{
		TotalCerts:  10,
		Expired:     2, // -60
		Expiring30d: 1, // -15
		Expiring60d: 1, // -8
		Expiring90d: 1, // -4
		SelfSigned:  2, // -4
	}
	// 100 - 60 - 15 - 8 - 4 - 4 = 9
	if score := certExpScore(s); score != 9 {
		t.Errorf("Expected 9, got %d", score)
	}

	// Floor at 0
	s = CertExpSummary{TotalCerts: 5, Expired: 10}
	if score := certExpScore(s); score != 0 {
		t.Errorf("Expected 0 for catastrophic, got %d", score)
	}
}

func TestCertExpRecs(t *testing.T) {
	s := CertExpSummary{
		TotalCerts:  10,
		Expired:     2,
		Expiring30d: 1,
		Expiring60d: 1,
		SelfSigned:  3,
		HealthScore: 35,
	}
	expired := []CertExpEntry{
		{Namespace: "default", Name: "cert-1"},
		{Namespace: "default", Name: "cert-2"},
	}

	recs := certExpRecs(s, expired, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundExpired := false
	foundSelfSigned := false
	foundScore := false
	for _, r := range recs {
		if strContains(r, "ALREADY EXPIRED") {
			foundExpired = true
		}
		if strContains(r, "self-signed") {
			foundSelfSigned = true
		}
		if strContains(r, "health score") {
			foundScore = true
		}
	}
	if !foundExpired {
		t.Error("Expected recommendation about expired certs")
	}
	if !foundSelfSigned {
		t.Error("Expected recommendation about self-signed certs")
	}
	if !foundScore {
		t.Error("Expected recommendation about low health score")
	}
}

func TestCertExpRecsClean(t *testing.T) {
	s := CertExpSummary{
		TotalCerts: 5,
		Healthy:    5,
	}
	recs := certExpRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected at least 1 recommendation (cert-manager suggestion)")
	}
}

func TestCertExpGetOrCreateNS(t *testing.T) {
	m := make(map[string]*CertExpNSStat)

	e1 := certExpGetOrCreateNS(m, "default")
	e1.CertCount = 5

	e2 := certExpGetOrCreateNS(m, "default")
	if e2.CertCount != 5 {
		t.Errorf("Expected same entry with CertCount=5, got %d", e2.CertCount)
	}

	e3 := certExpGetOrCreateNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestCertExpRank(t *testing.T) {
	if certExpRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if certExpRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if certExpRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if certExpRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestCertExpIssueRank(t *testing.T) {
	if certExpIssueRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if certExpIssueRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if certExpIssueRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}

func TestCertExpParse(t *testing.T) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		Issuer:       pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(30 * 24 * time.Hour),
		DNSNames:     []string{"test.example.com", "www.example.com"},
	}

	certDER, err := x509.CreateCertificate(nil, template, template, nil, nil)
	if err != nil {
		t.Skipf("Cannot generate test cert (no key): %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	entry, err := certExpParse(string(pemData), "test-secret", "default")
	if err != nil {
		t.Fatalf("certExpParse failed: %v", err)
	}

	if entry.CN != "test.example.com" {
		t.Errorf("Expected CN 'test.example.com', got '%s'", entry.CN)
	}
	if !entry.IsSelfSigned {
		t.Error("Expected self-signed cert")
	}
	if len(entry.SANs) < 2 {
		t.Errorf("Expected at least 2 SANs, got %d", len(entry.SANs))
	}
	daysRemaining := int(entry.NotAfter.Sub(time.Now()).Hours() / 24)
	if daysRemaining < 28 || daysRemaining > 32 {
		t.Errorf("Expected ~30 days remaining, got %d", daysRemaining)
	}
}

func TestCertExpParseInvalid(t *testing.T) {
	_, err := certExpParse("not a certificate", "bad", "default")
	if err == nil {
		t.Error("Expected error for invalid PEM")
	}

	_, err = certExpParse("", "empty", "default")
	if err == nil {
		t.Error("Expected error for empty PEM")
	}
}
