package dashboard

import "testing"

func TestNetPolCoverageResult1943(t *testing.T) {
	r := NetPolCoverageResult1943{Summary: NetPolCoverageSummary1943{TotalNamespaces: 29, WithNetPol: 2, WithoutNetPol: 27}}
	if r.Summary.WithoutNetPol != 27 {
		t.Errorf("expected 27")
	}
}
func TestNetPolUncoveredNS1943(t *testing.T) {
	e := NetPolUncoveredNS1943{Namespace: "prod", PodCount: 15, Severity: "high"}
	if e.PodCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestNetPolCoverageEntry1943(t *testing.T) {
	e := NetPolCoverageEntry1943{Namespace: "dev", NetPolCount: 3, HasDefaultDeny: true}
	if !e.HasDefaultDeny {
		t.Errorf("expected default deny")
	}
}
func TestSeccompResult1943(t *testing.T) {
	r := SeccompResult1943{Summary: SeccompSummary1943{TotalContainers: 87, WithSeccomp: 10, WithoutSeccomp: 70, Unconfined: 7}}
	if r.Summary.Unconfined != 7 {
		t.Errorf("expected 7")
	}
}
func TestSeccompEntry1943(t *testing.T) {
	e := SeccompEntry1943{Container: "main", SeccompProfile: "RuntimeDefault", HasCapAdd: false}
	if e.SeccompProfile != "RuntimeDefault" {
		t.Errorf("expected RuntimeDefault")
	}
}
func TestSeccompRisk1943(t *testing.T) {
	e := SeccompRisk1943{RiskType: "seccomp-unconfined", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestAPIDiscoveryResult1943(t *testing.T) {
	r := APIDiscoveryResult1943{Summary: APIDiscoverySummary1943{TotalAPIResources: 222, AnonymousAccess: 0}}
	if r.Summary.TotalAPIResources != 222 {
		t.Errorf("expected 222")
	}
}
func TestAPIDiscoveryEntry1943(t *testing.T) {
	e := APIDiscoveryEntry1943{Resource: "pods", Group: "core", VerbCount: 7}
	if e.VerbCount != 7 {
		t.Errorf("expected 7")
	}
}
