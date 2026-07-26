package dashboard

import "testing"

func TestPrivEscResult1991(t *testing.T) {
	r := PrivEscResult1991{Summary: PrivEscSummary1991{TotalContainers: 100, ExplicitFalse: 30, ExplicitTrue: 5, NotSet: 65}}
	if r.Summary.ExplicitTrue != 5 {
		t.Errorf("expected 5")
	}
}
func TestPrivEscEntry1991(t *testing.T) {
	e := PrivEscEntry1991{Pod: "app", Namespace: "prod", Container: "web", Value: true, Issue: "priv esc"}
	if !e.Value {
		t.Errorf("expected true")
	}
}
func TestSeccompResult1991(t *testing.T) {
	r := SeccompResult1991{Summary: SeccompSummary1991{TotalContainers: 50, RuntimeDefault: 10, Unconfined: 3, NotSet: 37}}
	if r.Summary.Unconfined != 3 {
		t.Errorf("expected 3")
	}
}
func TestSeccompEntry1991(t *testing.T) {
	e := SeccompEntry1991{Pod: "app", Namespace: "prod", Container: "web", Profile: "Unconfined"}
	if e.Profile != "Unconfined" {
		t.Errorf("expected Unconfined")
	}
}
func TestCapDropResult1991(t *testing.T) {
	r := CapDropResult1991{Summary: CapDropSummary1991{TotalContainers: 80, WithCapDrop: 20, DroppedAll: 5, HighRiskCapAdd: 3}}
	if r.Summary.DroppedAll != 5 {
		t.Errorf("expected 5")
	}
}
func TestCapDropEntry1991(t *testing.T) {
	e := CapDropEntry1991{Pod: "app", Namespace: "prod", Container: "web", CapDrop: []string{"ALL"}, CapAdd: []string{"NET_BIND_SERVICE"}}
	if e.CapDrop[0] != "ALL" {
		t.Errorf("expected ALL")
	}
}
func TestHighRiskCaps1991(t *testing.T) {
	if !highRiskCaps1991["SYS_ADMIN"] {
		t.Errorf("expected SYS_ADMIN to be high-risk")
	}
	if highRiskCaps1991["CHOWN"] {
		t.Errorf("CHOWN not in high-risk list")
	}
}
func TestCapDropSummary1991(t *testing.T) {
	s := CapDropSummary1991{WithCapAdd: 15, WithCapDrop: 25}
	if s.WithCapDrop != 25 {
		t.Errorf("expected 25")
	}
}
