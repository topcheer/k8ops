package dashboard

import "testing"

func TestSecretVersionResult1927(t *testing.T) {
	r := SecretVersionResult1927{
		Summary: SecretVersionSummary1927{TotalSecrets: 50, OlderThan90d: 15, EmptySecrets: 5},
	}
	if r.Summary.OlderThan90d != 15 {
		t.Errorf("expected 15, got %d", r.Summary.OlderThan90d)
	}
}

func TestSecretVersionEntry1927(t *testing.T) {
	e := SecretVersionEntry1927{Name: "db-cred", Namespace: "prod", Type: "Opaque", KeyCount: 3, AgeDays: 45.5}
	if e.KeyCount != 3 {
		t.Errorf("expected 3 keys")
	}
}

func TestCRDHealthResult1927(t *testing.T) {
	r := CRDHealthResult1927{
		Summary: CRDHealthSummary1927{TotalCRDs: 12, DeprecatedVer: 2},
	}
	if r.Summary.DeprecatedVer != 2 {
		t.Errorf("expected 2 deprecated versions")
	}
}

func TestCRDEntry1927(t *testing.T) {
	e := CRDEntry1927{Name: "certificates.cert-manager.io", Group: "cert-manager.io", Kind: "Certificate", VersionCount: 1}
	if e.Kind != "Certificate" {
		t.Errorf("expected Certificate")
	}
}

func TestAutosizeResult1927(t *testing.T) {
	r := AutosizeResult1927{
		Summary: AutosizeSummary1927{TotalWorkloads: 30, OverProvisioned: 8, UnderProvisioned: 3, NoRequests: 5, EstMonthlySavings: 42.5},
	}
	if r.Summary.OverProvisioned != 8 {
		t.Errorf("expected 8, got %d", r.Summary.OverProvisioned)
	}
}

func TestAutosizeEntry1927(t *testing.T) {
	e := AutosizeEntry1927{Name: "api", CurrentCPU: "4", RecommendedCPU: "2.0", SavingsUSD: 56.0}
	if e.SavingsUSD != 56.0 {
		t.Errorf("expected 56.0 savings")
	}
}

func TestSecretTypeStat1927(t *testing.T) {
	s := SecretTypeStat1927{Type: "Opaque", Count: 45}
	if s.Count != 45 {
		t.Errorf("expected 45")
	}
}

func TestCRDIssue1927(t *testing.T) {
	i := CRDIssue1927{Name: "mycrd.example.com", IssueType: "deprecated-version", Severity: "medium"}
	if i.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
