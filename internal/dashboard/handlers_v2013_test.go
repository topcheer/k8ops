package dashboard

import "testing"

func TestHostnameResult2013(t *testing.T) {
	r := HostnameResult2013{Summary: HostnameSummary2013{TotalPods: 80, WithHostname: 5, WithSubdomain: 3}}
	if r.Summary.WithSubdomain != 3 {
		t.Errorf("expected 3")
	}
}
func TestHostnameEntry2013(t *testing.T) {
	e := HostnameEntry2013{Pod: "app", Namespace: "prod", Hostname: "web", Subdomain: "default"}
	if e.Hostname != "web" {
		t.Errorf("expected web")
	}
}
func TestTCEgressResult2013(t *testing.T) {
	r := TCEgressResult2013{Summary: TCEgressSummary2013{TotalPods: 50, WithBandwidth: 3, WithTCMark: 5}}
	if r.Summary.WithTCMark != 5 {
		t.Errorf("expected 5")
	}
}
func TestTCEgressEntry2013(t *testing.T) {
	e := TCEgressEntry2013{Pod: "app", Namespace: "prod", Annotation: "kubernetes.io/egress-bandwidth", Value: "1M"}
	if e.Value != "1M" {
		t.Errorf("expected 1M")
	}
}
func TestNSValidResult2013(t *testing.T) {
	r := NSValidResult2013{Summary: NSValidSummary2013{TotalPods: 90, WithSelector: 10, ValidKeys: 8, Suspicious: 2}}
	if r.Summary.Suspicious != 2 {
		t.Errorf("expected 2")
	}
}
func TestNSValidEntry2013(t *testing.T) {
	e := NSValidEntry2013{Pod: "app", Namespace: "prod", Selectors: map[string]string{"zone": "us-east"}}
	if len(e.Selectors) != 1 {
		t.Errorf("expected 1")
	}
}
func TestHostnameSummary2013(t *testing.T) {
	s := HostnameSummary2013{WithSetHostnameAsFQDN: 1}
	if s.WithSetHostnameAsFQDN != 1 {
		t.Errorf("expected 1")
	}
}
func TestTCEgressSummary2013(t *testing.T) {
	s := TCEgressSummary2013{WithBandwidth: 3}
	if s.WithBandwidth != 3 {
		t.Errorf("expected 3")
	}
}
