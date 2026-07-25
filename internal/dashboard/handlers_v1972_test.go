package dashboard

import "testing"

func TestPodAgeDistResult1972(t *testing.T) {
	r := PodAgeDistResult1972{Summary: PodAgeDistSummary1972{TotalPods: 100, AvgAgeHours: 48.5, StalePods: 5}}
	if r.Summary.StalePods != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodAgeBucket1972(t *testing.T) {
	e := PodAgeBucket1972{Label: "1-7d", Count: 30}
	if e.Count != 30 {
		t.Errorf("expected 30")
	}
}
func TestPodAgeStaleEntry1972(t *testing.T) {
	e := PodAgeStaleEntry1972{Name: "app-1", Namespace: "prod", AgeDays: 45.5}
	if e.AgeDays != 45.5 {
		t.Errorf("expected 45.5")
	}
}
func TestNodeFlapResult1972(t *testing.T) {
	r := NodeFlapResult1972{Summary: NodeFlapSummary1972{TotalNodes: 5, StableNodes: 4, FlappingNodes: 1}}
	if r.Summary.FlappingNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeFlapEntry1972(t *testing.T) {
	e := NodeFlapEntry1972{Name: "node-1", Conditions: []string{"DiskPressure"}, RecentTransitions: 2}
	if e.RecentTransitions != 2 {
		t.Errorf("expected 2")
	}
}
func TestCSIAttachResult1972(t *testing.T) {
	r := CSIAttachResult1972{Summary: CSIAttachSummary1972{TotalPVCs: 20, BoundPVCs: 18, PendingPVCs: 2, AvgAttachTimeMin: 0.5}}
	if r.Summary.PendingPVCs != 2 {
		t.Errorf("expected 2")
	}
}
func TestCSIAttachEntry1972(t *testing.T) {
	e := CSIAttachEntry1972{Namespace: "prod", PVCName: "data", Status: "Bound", StorageClass: "fast-ssd", SizeGB: 100.0}
	if e.SizeGB != 100.0 {
		t.Errorf("expected 100")
	}
}
func TestNodeFlapSummary1972(t *testing.T) {
	s := NodeFlapSummary1972{TotalConditions: 15}
	if s.TotalConditions != 15 {
		t.Errorf("expected 15")
	}
}
