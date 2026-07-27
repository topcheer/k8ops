package dashboard

import "testing"

func TestSAPullSecResult2009(t *testing.T) {
	r := SAPullSecResult2009{Summary: SAPullSecSummary2009{TotalSAs: 50, WithPullSec: 10, Without: 40}}
	if r.Summary.Without != 40 {
		t.Errorf("expected 40")
	}
}
func TestSAPullSecEntry2009(t *testing.T) {
	e := SAPullSecEntry2009{Name: "app-sa", Namespace: "prod"}
	if e.Name != "app-sa" {
		t.Errorf("expected app-sa")
	}
}
func TestDNSPolResult2009(t *testing.T) {
	r := DNSPolResult2009{Summary: DNSPolSummary2009{TotalPods: 80, ClusterFirst: 75, DNSNone: 2}}
	if r.Summary.DNSNone != 2 {
		t.Errorf("expected 2")
	}
}
func TestDNSPolEntry2009(t *testing.T) {
	e := DNSPolEntry2009{Pod: "app", Namespace: "prod", Policy: "None", Issue: "dns none"}
	if e.Policy != "None" {
		t.Errorf("expected None")
	}
}
func TestRunAsUserResult2009(t *testing.T) {
	r := RunAsUserResult2009{Summary: RunAsUserSummary2009{TotalContainers: 100, RunAsRoot: 20, RunAsNonRoot: 30, NotSet: 50}}
	if r.Summary.RunAsRoot != 20 {
		t.Errorf("expected 20")
	}
}
func TestRunAsUserEntry2009(t *testing.T) {
	e := RunAsUserEntry2009{Pod: "app", Namespace: "prod", Container: "web", UID: 0}
	if e.UID != 0 {
		t.Errorf("expected 0")
	}
}
func TestDNSPolSummary2009(t *testing.T) {
	s := DNSPolSummary2009{DefaultPol: 70}
	if s.DefaultPol != 70 {
		t.Errorf("expected 70")
	}
}
func TestRunAsUserSummary2009(t *testing.T) {
	s := RunAsUserSummary2009{NotSet: 50}
	if s.NotSet != 50 {
		t.Errorf("expected 50")
	}
}
