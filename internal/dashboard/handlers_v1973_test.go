package dashboard

import "testing"

func TestUIDGIDResult1973(t *testing.T) {
	r := UIDGIDResult1973{Summary: UIDGIDSummary1973{TotalContainers: 50, RootUID: 10, WithFSGroup: 20}}
	if r.Summary.RootUID != 10 {
		t.Errorf("expected 10")
	}
}
func TestUIDGIDEntry1973(t *testing.T) {
	e := UIDGIDEntry1973{Pod: "app", Namespace: "prod", Container: "web", UID: 0, Issue: "root"}
	if e.UID != 0 {
		t.Errorf("expected 0")
	}
}
func TestDefaultSAResult1973(t *testing.T) {
	r := DefaultSAResult1973{Summary: DefaultSASummary1973{TotalPods: 80, UsingDefaultSA: 30, WithAutomount: 25}}
	if r.Summary.UsingDefaultSA != 30 {
		t.Errorf("expected 30")
	}
}
func TestDefaultSAEntry1973(t *testing.T) {
	e := DefaultSAEntry1973{Pod: "app", Namespace: "prod", SAName: "default", Automount: true}
	if e.SAName != "default" {
		t.Errorf("expected default")
	}
}
func TestSecPostureV2Result1973(t *testing.T) {
	r := SecPostureV2Result1973{Summary: SecPostureV2Summary1973{TotalPods: 100, PostureScore: 72.5, PrivilegedPods: 5, RootPods: 30}}
	if r.Summary.PostureScore != 72.5 {
		t.Errorf("expected 72.5")
	}
}
func TestSecPostureCheck1973(t *testing.T) {
	e := SecPostureCheck1973{Check: "non-root", Passed: 70, Failed: 30, Score: 70.0}
	if e.Score != 70.0 {
		t.Errorf("expected 70")
	}
}
func TestSecPostureNSEntry1973(t *testing.T) {
	e := SecPostureNSEntry1973{Namespace: "prod", PodCount: 15, Issues: 5, Score: 66.7}
	if e.Score != 66.7 {
		t.Errorf("expected 66.7")
	}
}
func TestUIDGIDSummary1973(t *testing.T) {
	s := UIDGIDSummary1973{WithUID: 40, RootGID: 8}
	if s.RootGID != 8 {
		t.Errorf("expected 8")
	}
}
