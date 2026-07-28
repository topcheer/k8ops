package dashboard

import "testing"

func TestCapEffResult2021(t *testing.T) {
	r := CapEffResult2021{Summary: CapEffSummary2021{TotalContainers: 100, WithCapDrop: 30, WithDropAll: 10}}
	if r.Summary.WithDropAll != 10 {
		t.Errorf("expected 10")
	}
}
func TestCapEffEntry2021(t *testing.T) {
	e := CapEffEntry2021{Pod: "app", Container: "web", Added: []string{"NET_ADMIN"}, Dropped: []string{"ALL"}}
	if len(e.Added) != 1 {
		t.Errorf("expected 1")
	}
}
func TestSecTypeResult2021(t *testing.T) {
	r := SecTypeResult2021{Summary: SecTypeSummary2021{TotalSecrets: 50, TLS: 10, Opaque: 35}}
	if r.Summary.Opaque != 35 {
		t.Errorf("expected 35")
	}
}
func TestSecTypeEntry2021(t *testing.T) {
	e := SecTypeEntry2021{Type: "Opaque", Count: 35}
	if e.Count != 35 {
		t.Errorf("expected 35")
	}
}
func TestSAPodMapResult2021(t *testing.T) {
	r := SAPodMapResult2021{Summary: SAPodMapSummary2021{TotalPods: 90, UsingDefault: 70, UsingCustom: 20}}
	if r.Summary.UsingDefault != 70 {
		t.Errorf("expected 70")
	}
}
func TestSAPodMapEntry2021(t *testing.T) {
	e := SAPodMapEntry2021{SAName: "app-sa", Namespace: "prod", PodCount: 5}
	if e.PodCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestHighRiskCaps2021(t *testing.T) {
	if !highRiskCaps2021["CAP_SYS_ADMIN"] {
		t.Errorf("expected CAP_SYS_ADMIN to be high-risk")
	}
	if highRiskCaps2021["CAP_CHOWN"] {
		t.Errorf("CAP_CHOWN should not be high-risk")
	}
}
func TestSecTypeSummary2021(t *testing.T) {
	s := SecTypeSummary2021{Dockerconfig: 5}
	if s.Dockerconfig != 5 {
		t.Errorf("expected 5")
	}
}
