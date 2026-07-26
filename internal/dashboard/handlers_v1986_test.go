package dashboard

import "testing"

func TestTaintCatResult1986(t *testing.T) {
	r := TaintCatResult1986{Summary: TaintCatSummary1986{TotalNodes: 5, NodesWithTaints: 2, TotalTaints: 3}}
	if r.Summary.TotalTaints != 3 {
		t.Errorf("expected 3")
	}
}
func TestTaintCatEntry1986(t *testing.T) {
	e := TaintCatEntry1986{Node: "node-1", Key: "dedicated", Value: "gpu", Effect: "NoSchedule"}
	if e.Effect != "NoSchedule" {
		t.Errorf("expected NoSchedule")
	}
}
func TestVolSnapCatResult1986(t *testing.T) {
	r := VolSnapCatResult1986{Summary: VolSnapCatSummary1986{TotalSnapshots: 10, ReadySnapshots: 8, NotReadySnapshots: 2}}
	if r.Summary.NotReadySnapshots != 2 {
		t.Errorf("expected 2")
	}
}
func TestVolSnapCatEntry1986(t *testing.T) {
	e := VolSnapCatEntry1986{Name: "snap-1", Namespace: "prod", PVCName: "data-vol", Ready: true}
	if !e.Ready {
		t.Errorf("expected true")
	}
}
func TestSCCatResult1986(t *testing.T) {
	t1 := true
	r := SCCatResult1986{Summary: SCCatSummary1986{TotalClasses: 3, DefaultClass: &t1, WithWaitForFirstConsumer: 1}}
	if *r.Summary.DefaultClass != true {
		t.Errorf("expected true")
	}
}
func TestSCCatEntry1986(t *testing.T) {
	e := SCCatEntry1986{Name: "fast-ssd", Provisioner: "csi-driver", ReclaimPolicy: "Delete", VolumeBindingMode: "WaitForFirstConsumer", IsDefault: true}
	if !e.IsDefault {
		t.Errorf("expected true")
	}
}
func TestTaintCatSummary1986(t *testing.T) {
	s := TaintCatSummary1986{NoScheduleTaints: 2, NoExecuteTaints: 1}
	if s.NoExecuteTaints != 1 {
		t.Errorf("expected 1")
	}
}
func TestVolSnapCatSummary1986(t *testing.T) {
	s := VolSnapCatSummary1986{SnapshotClasses: 2}
	if s.SnapshotClasses != 2 {
		t.Errorf("expected 2")
	}
}
