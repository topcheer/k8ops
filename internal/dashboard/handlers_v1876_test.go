package dashboard

import (
	"testing"
)

func TestCertTransparencyResultStruct1876(t *testing.T) {
	r := CertTransparencyResult{
		Summary:     CertTransSummary{TotalCerts: 5, ValidCerts: 3, Expiring30d: 2},
		HealthScore: 60,
	}
	if r.Summary.ValidCerts != 3 {
		t.Errorf("expected 3, got %d", r.Summary.ValidCerts)
	}
}

func TestCoreDNSConfigAuditResultStruct1876(t *testing.T) {
	r := CoreDNSConfigAuditResult{
		Summary:     CoreDNSConfigSummary{CoreDNSPodCount: 2, Ready: true, MemoryLimitSet: false},
		HealthScore: 70,
	}
	if !r.Summary.Ready {
		t.Error("expected ready")
	}
}

func TestWebhookAuditResultStruct1876(t *testing.T) {
	r := WebhookAuditResult{
		Summary:     WebhookTimeoutSummary{TotalWebhooks: 10, FailOpen: 3, Timeout30s: 1},
		HealthScore: 60,
	}
	if r.Summary.FailOpen != 3 {
		t.Errorf("expected 3, got %d", r.Summary.FailOpen)
	}
}
