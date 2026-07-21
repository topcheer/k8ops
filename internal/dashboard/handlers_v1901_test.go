package dashboard

import "testing"

func TestNodeMaintWindowResult1901(t *testing.T) {
	r := NodeMaintWindowResult{
		Summary:     NodeMaintSummary{TotalNodes: 1, ReadyNodes: 1, CordonedNodes: 0, DisruptablePods: 90},
		HealthScore: 100,
	}
	if r.Summary.DisruptablePods != 90 {
		t.Errorf("expected 90, got %d", r.Summary.DisruptablePods)
	}
}

func TestResourceLeakResult1901(t *testing.T) {
	r := ResourceLeakResult{
		Summary:     LeakSummary{TotalCMs: 100, OrphanedCMs: 30, OrphanedSecrets: 15, OrphanedPVCs: 3},
		HealthScore: 52,
	}
	if r.Summary.OrphanedSecrets != 15 {
		t.Errorf("expected 15, got %d", r.Summary.OrphanedSecrets)
	}
}

func TestLogAggHealthResult1901(t *testing.T) {
	r := LogAggHealthResult{
		Summary:     LogAggSummary{TotalContainers: 120, NoisyLoggers: 10, HighRestartRate: 3, TotalRestarts: 400},
		HealthScore: 97,
	}
	if r.Summary.TotalRestarts != 400 {
		t.Errorf("expected 400, got %d", r.Summary.TotalRestarts)
	}
}

func TestBuildNodeMaintRecs1901(t *testing.T) {
	r := &NodeMaintWindowResult{Summary: NodeMaintSummary{TotalNodes: 3, ReadyNodes: 2, CordonedNodes: 1, DisruptablePods: 50}}
	recs := buildNodeMaintRecs1901(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildLeakRecs1901(t *testing.T) {
	r := &ResourceLeakResult{Summary: LeakSummary{OrphanedCMs: 20, OrphanedSecrets: 10, OrphanedPVCs: 3, EstimatedWasteKB: 5000}}
	recs := buildLeakRecs1901(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildLogAggRecs1901(t *testing.T) {
	r := &LogAggHealthResult{Summary: LogAggSummary{TotalContainers: 100, NoisyLoggers: 15, HighRestartRate: 5, TotalRestarts: 300}}
	recs := buildLogAggRecs1901(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
