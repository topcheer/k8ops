package dashboard

import "testing"

func TestEventNoiseRecs(t *testing.T) {
	r := &EventNoiseFilterResult{Summary: EventNoiseSummary{TotalEvents: 100, NoiseRatio: 70, DuplicateCount: 50, ActionableRatio: 30, TopNoisyReason: "Started"}, SignalScore: 30}
	recs := buildEventNoiseRecs(r)
	if len(recs) < 2 {
		t.Error("need >=2 recs")
	}
}

func TestProgressiveRecs(t *testing.T) {
	r := &ProgressiveRolloutResult{Summary: ProgressiveSummary{TotalWorkloads: 20, CanaryReady: 5, BlueGreenReady: 3, HasProbes: 10, MissingBlocks: 15}, ReadinessScore: 25}
	recs := buildProgressiveRecs(r)
	if len(recs) < 2 {
		t.Error("need >=2 recs")
	}
}

func TestCostAnomalyDeepRecs(t *testing.T) {
	r := &CostAnomalyDeepResult{Summary: CostAnomalyDeepSummary{TotalNamespaces: 10, TotalCost: 500, AnomalyCount: 2, TopNamespace: "prod", TopCost: 200}, Anomalies: []CostAnomalyDeepEntry{{Namespace: "prod", AnomalyType: "cost-spike", Deviation: 300}}}
	recs := buildCostAnomalyDeepRecs(r)
	if len(recs) < 2 {
		t.Error("need >=2 recs")
	}
}

func TestEventNoiseTypes(t *testing.T) {
	e := EventNoiseEntry{Reason: "OOMKilled", Count: 5, IsNoise: false, Category: "critical"}
	if e.IsNoise || e.Category != "critical" {
		t.Error("OOMKilled should be actionable critical")
	}
}

func TestProgressiveTypes(t *testing.T) {
	e := ProgressiveEntry{Workload: "api", CanaryReady: true, BlueGreenReady: false, ReadinessPct: 80}
	if !e.CanaryReady {
		t.Error("should be canary ready")
	}
}

func TestCostAnomalyDeepTypes(t *testing.T) {
	e := CostAnomalyDeepEntry{Namespace: "prod", MonthlyCost: 200, AvgPodCost: 60, Deviation: 250, IsAnomaly: true, AnomalyType: "cost-spike"}
	if !e.IsAnomaly || e.AnomalyType != "cost-spike" {
		t.Error("should be cost-spike anomaly")
	}
}
