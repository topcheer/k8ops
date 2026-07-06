package dashboard

import (
	"testing"
)

func TestTSAssessRisk(t *testing.T) {
	// Single replica
	entry := TSEntry{Replicas: 1, MaxPerNode: 1}
	if level := tsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for single replica, got %s", level)
	}

	// Critical: 70%+ on one node
	entry = TSEntry{Replicas: 5, MaxPerNode: 4} // 80%
	if level := tsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for 80%%, got %s", level)
	}

	// High: 51-70%
	entry = TSEntry{Replicas: 5, MaxPerNode: 3} // 60%
	if level := tsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for 60%%, got %s", level)
	}

	// Medium: 35-50%
	entry = TSEntry{Replicas: 4, MaxPerNode: 2} // 50%
	if level := tsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 50%%, got %s", level)
	}

	// Low: <35%
	entry = TSEntry{Replicas: 6, MaxPerNode: 2} // 33%
	if level := tsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for 33%%, got %s", level)
	}
}

func TestTSScore(t *testing.T) {
	// No workloads
	if score := tsScore(TSSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// All well spread
	s := TSSummary{TotalWorkloads: 10, WellSpread: 10}
	if score := tsScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With concentration + imbalance
	s = TSSummary{
		TotalWorkloads: 10,
		WellSpread:     5,
		Concentrated:   3,
		MaxNodeLoad:    30,
		MinNodeLoad:    2,
	}
	// spreadPct=50, concentrated penalty=-15, imbalance=(28/31)*100=90.3>70 → -10
	// 50 - 15 - 10 = 25
	score := tsScore(s)
	if score < 20 || score > 30 {
		t.Errorf("Expected ~25, got %d", score)
	}
}

func TestTSGenRecs(t *testing.T) {
	s := TSSummary{
		TotalWorkloads:    10,
		Concentrated:      3,
		NoConstraints:     4,
		AntiAffinitySet:   2,
		MaxNodeLoad:       25,
		MinNodeLoad:       2,
		DistributionScore: 35,
	}
	concentrated := []TSEntry{
		{Namespace: "default", Name: "app1", MaxPerNode: 4, Replicas: 5},
	}
	noConstraints := []TSEntry{
		{Namespace: "default", Name: "app2", Replicas: 3},
	}

	recs := tsGenRecs(s, concentrated, noConstraints)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundConc := false
	foundNoCons := false
	foundImbalance := false
	for _, r := range recs {
		if strContains(r, "pod concentration") {
			foundConc = true
		}
		if strContains(r, "topologySpreadConstraints") {
			foundNoCons = true
		}
		if strContains(r, "imbalance") {
			foundImbalance = true
		}
	}
	if !foundConc {
		t.Error("Expected recommendation about pod concentration")
	}
	if !foundNoCons {
		t.Error("Expected recommendation about missing constraints")
	}
	if !foundImbalance {
		t.Error("Expected recommendation about node load imbalance")
	}
}

func TestTSGenRecsClean(t *testing.T) {
	s := TSSummary{TotalWorkloads: 10, WellSpread: 10}
	recs := tsGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestTSRiskRank(t *testing.T) {
	if tsRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if tsRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if tsRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if tsRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestTSIssueRank(t *testing.T) {
	if tsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if tsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if tsIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
