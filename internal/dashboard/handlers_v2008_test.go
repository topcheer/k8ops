package dashboard

import "testing"

func TestTaintMatchResult2008(t *testing.T) {
	r := TaintMatchResult2008{Summary: TaintMatchSummary2008{TotalNodes: 5, NodesWithTaints: 2, PodsWithToleration: 10}}
	if r.Summary.NodesWithTaints != 2 {
		t.Errorf("expected 2")
	}
}
func TestTaintMatchNodeEntry2008(t *testing.T) {
	e := TaintMatchNodeEntry2008{Node: "node-1", Taints: []string{"dedicated=gpu:NoSchedule"}, PodCount: 5}
	if e.PodCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeCondResult2008(t *testing.T) {
	r := NodeCondResult2008{Summary: NodeCondSummary2008{TotalNodes: 5, HealthyNodes: 4, WithDiskPressure: 1}}
	if r.Summary.WithDiskPressure != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeCondEntry2008(t *testing.T) {
	e := NodeCondEntry2008{Name: "node-1", Ready: true, Issues: []string{"DiskPressure"}}
	if !e.Ready {
		t.Errorf("expected true")
	}
}
func TestLogVolResult2008(t *testing.T) {
	r := LogVolResult2008{Summary: LogVolSummary2008{TotalPods: 90, EstLogMBPerDay: 4500.0, HighLogNS: 2}}
	if r.Summary.EstLogMBPerDay != 4500.0 {
		t.Errorf("expected 4500")
	}
}
func TestLogVolEntry2008(t *testing.T) {
	e := LogVolEntry2008{Namespace: "prod", PodCount: 30, EstLogMBPerDay: 1500.0}
	if e.PodCount != 30 {
		t.Errorf("expected 30")
	}
}
func TestNodeCondSummary2008(t *testing.T) {
	s := NodeCondSummary2008{WithPIDPressure: 0}
	if s.WithPIDPressure != 0 {
		t.Errorf("expected 0")
	}
}
func TestLogVolSummary2008(t *testing.T) {
	s := LogVolSummary2008{HighLogNS: 3}
	if s.HighLogNS != 3 {
		t.Errorf("expected 3")
	}
}
