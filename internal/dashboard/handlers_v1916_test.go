package dashboard

import "testing"

func TestAPIThrottleResult1916(t *testing.T) {
	r := APIThrottleResult{
		Summary:     APIThrottleSummary{EstimatedQPS: 150, HighQPSNamespaces: 3, TotalWatchEvents: 500},
		HealthScore: 80,
	}
	if r.Summary.EstimatedQPS != 150 {
		t.Errorf("expected 150, got %d", r.Summary.EstimatedQPS)
	}
}

func TestPodDensityResult1916(t *testing.T) {
	r := PodDensityResult1916{
		Summary:     PodDensitySummary{TotalNodes: 1, TotalPods: 80, AvgPodsPerNode: 80, DensityPct: 72, UnderutilNodes: 0, OverutilNodes: 0},
		HealthScore: 100,
	}
	if r.Summary.DensityPct != 72 {
		t.Errorf("expected 72, got %d", r.Summary.DensityPct)
	}
}

func TestOvercommitForecastResult1916(t *testing.T) {
	r := OvercommitForecastResult1916{
		Summary:     OvercommitSummary1916{OvercommitRatio: 2.5, UnboundedCount: 10, AtRiskCount: 5, TotalCapacity: 2000},
		HealthScore: 60,
	}
	if r.Summary.UnboundedCount != 10 {
		t.Errorf("expected 10, got %d", r.Summary.UnboundedCount)
	}
}

func TestBuildAPIThrottleRecs1916(t *testing.T) {
	r := &APIThrottleResult{Summary: APIThrottleSummary{EstimatedQPS: 150, HighQPSNamespaces: 3, TotalWatchEvents: 500}, ByNamespace: []APIThrottleNS{{}}, HighConsumers: []APIThrottleNS{{}}}
	recs := buildAPIThrottleRecs1916(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildPodDensityRecs1916(t *testing.T) {
	r := &PodDensityResult1916{Summary: PodDensitySummary{TotalNodes: 1, TotalPods: 80, AvgPodsPerNode: 80, DensityPct: 72, UnderutilNodes: 1, OverutilNodes: 0}}
	recs := buildPodDensityRecs1916(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildOvercommitRecs1916(t *testing.T) {
	r := &OvercommitForecastResult1916{Summary: OvercommitSummary1916{OvercommitRatio: 3.5, UnboundedCount: 10, AtRiskCount: 5, TotalCapacity: 2000}}
	recs := buildOvercommitRecs1916(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
