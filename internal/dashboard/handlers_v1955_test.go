package dashboard

import "testing"

func TestPSAViolationResult1955(t *testing.T) {
	r := PSAViolationResult1955{Summary: PSAViolationSummary1955{TotalNamespaces: 29, ViolationCount: 5, CriticalCount: 2}}
	if r.Summary.CriticalCount != 2 {
		t.Errorf("expected 2")
	}
}
func TestPSAViolationEntry1955(t *testing.T) {
	e := PSAViolationEntry1955{Namespace: "prod", Violation: "privileged", Severity: "critical"}
	if e.Severity != "critical" {
		t.Errorf("expected critical")
	}
}
func TestAutoMountResult1955(t *testing.T) {
	r := AutoMountResult1955{Summary: AutoMountSummary1955{TotalPods: 79, WithAutoMount: 75, UsingDefaultSA: 13}}
	if r.Summary.UsingDefaultSA != 13 {
		t.Errorf("expected 13")
	}
}
func TestAutoMountEntry1955(t *testing.T) {
	e := AutoMountEntry1955{PodName: "web", SAName: "default", IsDefault: true}
	if !e.IsDefault {
		t.Errorf("expected default")
	}
}
func TestRegistryTrustResult1955(t *testing.T) {
	r := RegistryTrustResult1955{Summary: RegistryTrustSummary1955{TotalImages: 79, UniqueRegistries: 5, UntrustedCount: 1}}
	if r.Summary.UntrustedCount != 1 {
		t.Errorf("expected 1")
	}
}
func TestRegistryTrustEntry1955(t *testing.T) {
	e := RegistryTrustEntry1955{Registry: "ghcr.io", ImageCount: 30, IsTrusted: true}
	if !e.IsTrusted {
		t.Errorf("expected trusted")
	}
}
func TestRegistryTrustUntrusted1955(t *testing.T) {
	e := RegistryTrustUntrusted1955{Image: "suspicious/app:latest", Registry: "unknown.io"}
	if e.Registry != "unknown.io" {
		t.Errorf("expected unknown.io")
	}
}
func TestPSAViolationNS1955(t *testing.T) {
	e := PSAViolationNS1955{Namespace: "dev", PSALevel: "restricted", Violations: 0}
	if e.PSALevel != "restricted" {
		t.Errorf("expected restricted")
	}
}
