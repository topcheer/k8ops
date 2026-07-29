package dashboard

import "testing"

func TestReplicaAvailResult2060(t *testing.T) {
	r := ReplicaAvailResult2060{Summary: ReplicaAvailSummary2060{TotalWorkloads: 50, FullyAvailable: 45, UnderReplicated: 5}}
	if r.Summary.UnderReplicated != 5 {
		t.Errorf("expected 5")
	}
}
func TestWkldDistResult2060(t *testing.T) {
	r := WkldDistResult2060{Summary: WkldDistSummary2060{TotalNodes: 5, TotalPods: 100, AvgPodsPerNode: 20, Unbalanced: 1}}
	if r.Summary.Unbalanced != 1 {
		t.Errorf("expected 1")
	}
}
func TestFailoverResult2060(t *testing.T) {
	r := FailoverResult2060{Summary: FailoverSummary2060{CMReplicas: 1, SchedReplicas: 1, EtcdReplicas: 1, TotalHAComponents: 0}}
	if r.Summary.CMReplicas != 1 {
		t.Errorf("expected 1")
	}
}
