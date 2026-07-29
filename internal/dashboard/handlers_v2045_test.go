package dashboard

import "testing"

func TestPodQPSResult2045(t *testing.T) {
	r := PodQPSResult2045{Summary: PodQPSSummary2045{TotalPods: 100, HighQPSPods: 5, TotalEstQPS: 500}}
	if r.Summary.HighQPSPods != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodQPSEntry2045(t *testing.T) {
	e := PodQPSEntry2045{Pod: "api", Namespace: "prod", EstQPS: 50}
	if e.EstQPS != 50 {
		t.Errorf("expected 50")
	}
}
func TestLogVolResult2045(t *testing.T) {
	r := LogVolResult2045{Summary: LogVolSummary2045{TotalPods: 100, NoisyPods: 10}}
	if r.Summary.NoisyPods != 10 {
		t.Errorf("expected 10")
	}
}
func TestLogVolEntry2045(t *testing.T) {
	e := LogVolEntry2045{Pod: "app", Namespace: "prod", Restarts: 15, Events: 200}
	if e.Restarts != 15 {
		t.Errorf("expected 15")
	}
}
func TestNodeCondBudgetResult2045(t *testing.T) {
	r := NodeCondBudgetResult2045{Summary: NodeCondBudgetSummary2045{TotalNodes: 5, HealthyNodes: 4, NodesWithIssues: 1, TotalConditions: 2}}
	if r.Summary.NodesWithIssues != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeCondBudgetEntry2045(t *testing.T) {
	e := NodeCondBudgetEntry2045{Node: "node-1", Conditions: []string{"DiskPressure"}}
	if len(e.Conditions) != 1 {
		t.Errorf("expected 1 condition")
	}
}
