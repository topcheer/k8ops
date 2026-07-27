package dashboard

import "testing"

func TestNodeBudgetResult1996(t *testing.T) {
	r := NodeBudgetResult1996{Summary: NodeBudgetSummary1996{TotalNodes: 5, CPUHeadroom: 30.5, MemHeadroom: 45.0, PressureLevel: "low"}}
	if r.Summary.CPUHeadroom != 30.5 {
		t.Errorf("expected 30.5")
	}
}
func TestNodeBudgetEntry1996(t *testing.T) {
	e := NodeBudgetEntry1996{Name: "node-1", CPUUtil: 70.0, MemUtil: 55.0, Pressure: "medium"}
	if e.Pressure != "medium" {
		t.Errorf("expected medium")
	}
}
func TestEventBudgetResult1996(t *testing.T) {
	r := EventBudgetResult1996{Summary: EventBudgetSummary1996{TotalEvents: 500, WarningRatio: 0.15, TopReason: "Pulled"}}
	if r.Summary.WarningRatio != 0.15 {
		t.Errorf("expected 0.15")
	}
}
func TestEventBudgetEntry1996(t *testing.T) {
	e := EventBudgetEntry1996{Reason: "FailedScheduling", Count: 10, Type: "Warning"}
	if e.Type != "Warning" {
		t.Errorf("expected Warning")
	}
}
func TestNetPolicyBudgetResult1996(t *testing.T) {
	r := NetPolicyBudgetResult1996{Summary: NetPolicyBudgetSummary1996{TotalNamespaces: 10, WithDefaultDeny: 3, Unprotected: 7}}
	if r.Summary.Unprotected != 7 {
		t.Errorf("expected 7")
	}
}
func TestNetPolicyBudgetEntry1996(t *testing.T) {
	e := NetPolicyBudgetEntry1996{Namespace: "prod", HasEgressDeny: false, HasIngressDeny: true, PolicyCount: 2}
	if e.HasEgressDeny {
		t.Errorf("expected false")
	}
}
func TestNodeBudgetSummary1996(t *testing.T) {
	s := NodeBudgetSummary1996{MemHeadroom: 45.0}
	if s.MemHeadroom != 45.0 {
		t.Errorf("expected 45")
	}
}
func TestEventBudgetSummary1996(t *testing.T) {
	s := EventBudgetSummary1996{TopReason: "Pulled"}
	if s.TopReason != "Pulled" {
		t.Errorf("expected Pulled")
	}
}
