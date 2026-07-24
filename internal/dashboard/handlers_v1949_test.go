package dashboard

import "testing"

func TestPodForensicsResult1949(t *testing.T) {
	r := PodForensicsResult1949{Summary: PodForensicsSummary1949{TotalPods: 79, SuspiciousCount: 5, WithPrivileged: 2}}
	if r.Summary.WithPrivileged != 2 {
		t.Errorf("expected 2")
	}
}
func TestPodForensicsEntry1949(t *testing.T) {
	e := PodForensicsEntry1949{Name: "suspicious", Indicators: []string{"hostNetwork", "privileged"}, Severity: "critical"}
	if e.Severity != "critical" {
		t.Errorf("expected critical")
	}
}
func TestEgressResult1949(t *testing.T) {
	r := EgressResult1949{Summary: EgressSummary1949{TotalNamespaces: 29, WithoutEgress: 27, ExternalDestinations: 5}}
	if r.Summary.ExternalDestinations != 5 {
		t.Errorf("expected 5")
	}
}
func TestEgressEntry1949(t *testing.T) {
	e := EgressEntry1949{Namespace: "prod", NetPolName: "deny-all", AllowsAll: true}
	if !e.AllowsAll {
		t.Errorf("expected allows all")
	}
}
func TestSATokenAgeResult1949(t *testing.T) {
	r := SATokenAgeResult1949{Summary: SATokenAgeSummary1949{TotalSAs: 54, OlderThan180d: 10, ProjectedTokens: 30}}
	if r.Summary.ProjectedTokens != 30 {
		t.Errorf("expected 30")
	}
}
func TestSATokenEntry1949(t *testing.T) {
	e := SATokenEntry1949{Name: "old-sa", AgeDays: 400, Severity: "high"}
	if e.AgeDays != 400 {
		t.Errorf("expected 400")
	}
}
func TestEgressUnrestricted1949(t *testing.T) {
	e := EgressUnrestricted1949{Namespace: "dev", PodCount: 15, Severity: "medium"}
	if e.PodCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestSATokenNS1949(t *testing.T) {
	e := SATokenNS1949{Namespace: "prod", SACount: 10, OldCount: 3}
	if e.OldCount != 3 {
		t.Errorf("expected 3")
	}
}
