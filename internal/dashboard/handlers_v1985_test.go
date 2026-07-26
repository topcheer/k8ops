package dashboard

import "testing"

func TestRBACWildcardResult1985(t *testing.T) {
	r := RBACWildcardResult1985{Summary: RBACWildcardSummary1985{TotalRoles: 50, WithWildcardVerb: 5, HighRiskRoles: 3}}
	if r.Summary.HighRiskRoles != 3 {
		t.Errorf("expected 3")
	}
}
func TestRBACWildcardEntry1985(t *testing.T) {
	e := RBACWildcardEntry1985{Name: "admin-role", Namespace: "cluster", Kind: "ClusterRole", Verbs: []string{"*"}, Resources: []string{"*"}, RiskLevel: "critical"}
	if e.RiskLevel != "critical" {
		t.Errorf("expected critical")
	}
}
func TestAnonAuthResult1985(t *testing.T) {
	r := AnonAuthResult1985{Summary: AnonAuthSummary1985{TotalServices: 80, LoadBalancerSvcs: 5, WithoutAuth: 3}}
	if r.Summary.WithoutAuth != 3 {
		t.Errorf("expected 3")
	}
}
func TestAnonAuthEntry1985(t *testing.T) {
	e := AnonAuthEntry1985{Name: "api-lb", Namespace: "prod", Type: "LoadBalancer", HasAuth: false}
	if e.HasAuth {
		t.Errorf("expected false")
	}
}
func TestNodeRestrResult1985(t *testing.T) {
	r := NodeRestrResult1985{Summary: NodeRestrSummary1985{TotalNodes: 5, UserLabelCount: 15, SystemLabelCount: 30}}
	if r.Summary.UserLabelCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestNodeRestrEntry1985(t *testing.T) {
	e := NodeRestrEntry1985{Name: "node-1", UserLabels: []string{"env", "zone"}, SystemLabels: 8}
	if e.SystemLabels != 8 {
		t.Errorf("expected 8")
	}
}
func TestContainsInList1985(t *testing.T) {
	if !containsInList1985([]string{"get", "*", "list"}, "*") {
		t.Errorf("expected true")
	}
	if containsInList1985([]string{"get", "list"}, "*") {
		t.Errorf("expected false")
	}
}
func TestRBACWildcardSummary1985(t *testing.T) {
	s := RBACWildcardSummary1985{WithWildcardResource: 8, WithWildcardAPIGroup: 3}
	if s.WithWildcardAPIGroup != 3 {
		t.Errorf("expected 3")
	}
}
