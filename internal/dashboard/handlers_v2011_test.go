package dashboard

import "testing"

func TestHPATargetResult2011(t *testing.T) {
	r := HPATargetResult2011{Summary: HPATargetSummary2011{TotalHPAs: 5, AvgTarget: 70.0, WithCPU: 5}}
	if r.Summary.AvgTarget != 70.0 {
		t.Errorf("expected 70")
	}
}
func TestHPATargetEntry2011(t *testing.T) {
	e := HPATargetEntry2011{Name: "api-hpa", Namespace: "prod", TargetCPU: 80, MinReplicas: 2, MaxReplicas: 10}
	if e.TargetCPU != 80 {
		t.Errorf("expected 80")
	}
}
func TestReplicaAgeResult2011(t *testing.T) {
	r := ReplicaAgeResult2011{Summary: ReplicaAgeSummary2011{TotalPods: 90, AvgAgeDays: 15.5, NewPods: 5}}
	if r.Summary.NewPods != 5 {
		t.Errorf("expected 5")
	}
}
func TestReplicaAgeBucket2011(t *testing.T) {
	e := ReplicaAgeBucket2011{Bucket: "1-7d", Count: 30}
	if e.Count != 30 {
		t.Errorf("expected 30")
	}
}
func TestNodeScoreResult2011(t *testing.T) {
	r := NodeScoreResult2011{Summary: NodeScoreSummary2011{TotalNodes: 3, AvgScore: 85.0, BestNode: "node-1"}}
	if r.Summary.BestNode != "node-1" {
		t.Errorf("expected node-1")
	}
}
func TestNodeScoreEntry2011(t *testing.T) {
	e := NodeScoreEntry2011{Name: "node-1", PodCount: 30, BalanceScore: 90.0}
	if e.BalanceScore != 90.0 {
		t.Errorf("expected 90")
	}
}
func TestAbs2011(t *testing.T) {
	if abs2011(-5.0) != 5.0 {
		t.Errorf("expected 5")
	}
	if abs2011(3.0) != 3.0 {
		t.Errorf("expected 3")
	}
}
func TestReplicaAgeSummary2011(t *testing.T) {
	s := ReplicaAgeSummary2011{OldPods: 20}
	if s.OldPods != 20 {
		t.Errorf("expected 20")
	}
}
