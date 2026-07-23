package dashboard

import "testing"

func TestLabelStdResult1932(t *testing.T) {
	r := LabelStdResult1932{Summary: LabelStdSummary1932{TotalResources: 200, WithLabels: 150, UniqueLabelKeys: 25}}
	if r.Summary.WithLabels != 150 {
		t.Errorf("expected 150")
	}
}
func TestLabelStat1932(t *testing.T) {
	s := LabelStat1932{Key: "app", Count: 50, Category: "standard"}
	if s.Count != 50 {
		t.Errorf("expected 50")
	}
}
func TestLabelViolation1932(t *testing.T) {
	v := LabelViolation1932{Name: "web", Kind: "Deployment", Severity: "medium"}
	if v.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
func TestResAgeResult1932(t *testing.T) {
	r := ResAgeResult1932{Summary: ResAgeSummary1932{TotalResources: 100, AvgAgeDays: 120.5, OlderThan1Year: 15}}
	if r.Summary.AvgAgeDays != 120.5 {
		t.Errorf("expected 120.5")
	}
}
func TestResAgeBucket1932(t *testing.T) {
	b := ResAgeBucket1932{Range: "90-180 days", Count: 20}
	if b.Count != 20 {
		t.Errorf("expected 20")
	}
}
func TestNSIsolationResult1932(t *testing.T) {
	r := NSIsolationResult1932{Summary: NSIsolationSummary1932{TotalNamespaces: 29, WithNetPol: 5, OpenNS: 15}}
	if r.Summary.OpenNS != 15 {
		t.Errorf("expected 15")
	}
}
func TestNSIsolationEntry1932(t *testing.T) {
	e := NSIsolationEntry1932{Namespace: "prod", HasNetPol: true, Isolation: "high", PodCount: 10}
	if !e.HasNetPol {
		t.Errorf("expected netpol=true")
	}
}
func TestNSIsolationRisk1932(t *testing.T) {
	r := NSIsolationRisk1932{Namespace: "dev", RiskType: "no-isolation", Severity: "high"}
	if r.RiskType != "no-isolation" {
		t.Errorf("expected no-isolation")
	}
}
