package dashboard

import "testing"

func TestVolSnapshotResult1945(t *testing.T) {
	r := VolSnapshotResult1945{Summary: VolSnapshotSummary1945{TotalPVCs: 15, WithSnapshot: 3, WithoutSnapshot: 12}}
	if r.Summary.WithoutSnapshot != 12 {
		t.Errorf("expected 12")
	}
}
func TestVolSnapshotEntry1945(t *testing.T) {
	e := VolSnapshotEntry1945{Name: "snap-1", PVCName: "data-pvc", Ready: true}
	if !e.Ready {
		t.Errorf("expected ready")
	}
}
func TestPriorityClassResult1945(t *testing.T) {
	r := PriorityClassResult1945{Summary: PriorityClassSummary1945{TotalPriorityClasses: 5, SystemPriorityCount: 3, CustomPriorityCount: 2}}
	if r.Summary.CustomPriorityCount != 2 {
		t.Errorf("expected 2")
	}
}
func TestPriorityClassEntry1945(t *testing.T) {
	e := PriorityClassEntry1945{Name: "critical", PriorityValue: 1000000, IsDefault: false}
	if e.PriorityValue != 1000000 {
		t.Errorf("expected 1000000")
	}
}
func TestPullPolicyResult1945(t *testing.T) {
	r := PullPolicyResult1945{Summary: PullPolicySummary1945{TotalContainers: 87, AlwaysPull: 20, LatestWithIfNot: 5}}
	if r.Summary.LatestWithIfNot != 5 {
		t.Errorf("expected 5")
	}
}
func TestPullPolicyEntry1945(t *testing.T) {
	e := PullPolicyEntry1945{Container: "main", PullPolicy: "Never", Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
}
func TestVolSnapshotUnprotEntry1945(t *testing.T) {
	e := VolSnapshotUnprotEntry1945{PVCName: "data", Severity: "medium"}
	if e.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
func TestPriorityPodStat1945(t *testing.T) {
	s := PriorityPodStat1945{PriorityClass: "system-critical", PodCount: 10}
	if s.PodCount != 10 {
		t.Errorf("expected 10")
	}
}
