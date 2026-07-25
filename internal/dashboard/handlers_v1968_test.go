package dashboard

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
)

func TestIngressCatalogResult1968(t *testing.T) {
	r := IngressCatalogResult1968{Summary: IngressCatalogSummary1968{TotalIngresses: 10, TotalHosts: 15, WithTLS: 8, WithoutTLS: 2}}
	if r.Summary.WithoutTLS != 2 {
		t.Errorf("expected 2")
	}
}
func TestIngressCatalogEntry1968(t *testing.T) {
	e := IngressCatalogEntry1968{Name: "api-ing", Namespace: "prod", Hosts: []string{"api.example.com"}, HasTLS: true, RuleCount: 1}
	if !e.HasTLS {
		t.Errorf("expected true")
	}
}
func TestNetPolCatalogResult1968(t *testing.T) {
	r := NetPolCatalogResult1968{Summary: NetPolCatalogSummary1968{TotalPolicies: 5, DenyAllIngress: 2, NamespacesCovered: 3}}
	if r.Summary.DenyAllIngress != 2 {
		t.Errorf("expected 2")
	}
}
func TestNetPolCatalogEntry1968(t *testing.T) {
	e := NetPolCatalogEntry1968{Name: "default-deny", Namespace: "prod", HasIngress: true, HasEgress: false}
	if !e.HasIngress {
		t.Errorf("expected true")
	}
}
func TestHasPolicyType1968(t *testing.T) {
	if !hasPolicyType1968([]networkingv1.PolicyType{"Ingress", "Egress"}, "Egress") {
		t.Errorf("expected true")
	}
	if hasPolicyType1968([]networkingv1.PolicyType{"Ingress"}, "Egress") {
		t.Errorf("expected false")
	}
}
func TestLabelInvResult1968(t *testing.T) {
	r := LabelInvResult1968{Summary: LabelInvSummary1968{TotalLabels: 25, StandardLabels: 15, NonStandard: 10}}
	if r.Summary.NonStandard != 10 {
		t.Errorf("expected 10")
	}
}
func TestLabelInvEntry1968(t *testing.T) {
	e := LabelInvEntry1968{Key: "app.kubernetes.io/name", Count: 50, IsStandard: true}
	if !e.IsStandard {
		t.Errorf("expected true")
	}
}
func TestLabelInvEntry1968NonStd(t *testing.T) {
	e := LabelInvEntry1968{Key: "custom-label", Count: 3, IsStandard: false}
	if e.IsStandard {
		t.Errorf("expected false")
	}
}
