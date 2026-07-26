package dashboard

import "testing"

func TestPCClassResult1992(t *testing.T) {
	r := PCClassResult1992{Summary: PCClassSummary1992{TotalClasses: 5, SystemCritical: 2, UserClasses: 3}}
	if r.Summary.UserClasses != 3 {
		t.Errorf("expected 3")
	}
}
func TestPCClassEntry1992(t *testing.T) {
	e := PCClassEntry1992{Name: "high", Value: 1000000, IsGlobalDefault: false, IsSystem: true}
	if !e.IsSystem {
		t.Errorf("expected true")
	}
}
func TestRBListResult1992(t *testing.T) {
	r := RBListResult1992{Summary: RBListSummary1992{TotalRoleBindings: 20, TotalClusterBindings: 10, ToSA: 15}}
	if r.Summary.ToSA != 15 {
		t.Errorf("expected 15")
	}
}
func TestRBListEntry1992(t *testing.T) {
	e := RBListEntry1992{Name: "rb-1", Namespace: "prod", RoleRef: "ClusterRole/admin", Scope: "namespace", Subjects: 2}
	if e.Subjects != 2 {
		t.Errorf("expected 2")
	}
}
func TestEPSliceResult1992(t *testing.T) {
	r := EPSliceResult1992{Summary: EPSliceSummary1992{TotalSlices: 10, TotalEndpoints: 30, ReadyEndpoints: 28}}
	if r.Summary.ReadyEndpoints != 28 {
		t.Errorf("expected 28")
	}
}
func TestEPSliceEntry1992(t *testing.T) {
	e := EPSliceEntry1992{Name: "ep-1", Namespace: "prod", Service: "api", Addresses: 3, Ports: 2}
	if e.Addresses != 3 {
		t.Errorf("expected 3")
	}
}
func TestPCClassSummary1992(t *testing.T) {
	s := PCClassSummary1992{HasDefault: true}
	if !s.HasDefault {
		t.Errorf("expected true")
	}
}
func TestRBListSummary1992(t *testing.T) {
	s := RBListSummary1992{ToUser: 5, ToGroup: 3}
	if s.ToGroup != 3 {
		t.Errorf("expected 3")
	}
}
