package dashboard

import (
	"testing"
)

func TestEnvVarDriftResultStruct1885(t *testing.T) {
	r := EnvVarDriftResult{
		Summary:     EnvVarDriftSummary{TotalDeployments: 59, HardcodedSecrets: 2, DriftDetected: 1},
		HealthScore: 55,
	}
	if r.Summary.HardcodedSecrets != 2 {
		t.Errorf("expected 2, got %d", r.Summary.HardcodedSecrets)
	}
}

func TestDNSRecordAuditResultStruct1885(t *testing.T) {
	r := DNSRecordAuditResult{
		Summary:     DNSAuditSummary{TotalServices: 91, NoEndpoints: 5},
		HealthScore: 94,
	}
	if r.Summary.NoEndpoints != 5 {
		t.Errorf("expected 5, got %d", r.Summary.NoEndpoints)
	}
}

func TestWorkloadStartupProfileResultStruct1885(t *testing.T) {
	r := WorkloadStartupProfileResult{
		Summary:     StartupProfileSummary{TotalPods: 83, WithStartupProbe: 8},
		HealthScore: 90,
	}
	if r.Summary.WithStartupProbe != 8 {
		t.Errorf("expected 8, got %d", r.Summary.WithStartupProbe)
	}
}
