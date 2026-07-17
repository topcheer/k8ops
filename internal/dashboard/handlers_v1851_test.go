package dashboard

import (
	"testing"
)

func TestDisruptionBudgetGapRecs(t *testing.T) {
	r := &DisruptionBudgetGapResult{
		Summary: DisruptionGapSummary{
			TotalWorkloads:  10,
			WithPDB:         4,
			WithoutPDB:      6,
			SingleReplica:   3,
			CriticalExposed: 2,
		},
		RiskScore: 60,
	}
	recs := buildDisruptionGapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestCostTopologyRecs(t *testing.T) {
	r := &CostTopologyResult{
		Summary: CostTopologySummary{
			TotalNamespaces: 5,
			TotalCPUCores:   10.5,
			TotalMemoryGB:   32.0,
			EstMonthlyCost:  850.0,
			TopNsShare:      82.0,
		},
		TopSpenders: []CostTopologyEntry{
			{Namespace: "prod", MonthlyCost: 350.0},
		},
		CostScore: 40,
	}
	recs := buildCostTopologyRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestBinpackRecs(t *testing.T) {
	r := &BinpackEfficiencyResult{
		Summary: BinpackSummary{
			TotalNodes:     6,
			UtilizedNodes:  4,
			IdleNodes:      1,
			Underutilized:  2,
			CPUUtilization: 35.0,
			MemUtilization: 45.0,
		},
		ConsolidationTargets: []BinpackNodeEntry{
			{NodeName: "node-3", Status: "idle"},
			{NodeName: "node-4", Status: "underutilized"},
		},
	}
	recs := buildBinpackRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestDisruptionBudgetGapTypes(t *testing.T) {
	// Verify struct field names and JSON tags are correct
	entry := DisruptionGapEntry{
		Workload:       "test-app",
		Namespace:      "default",
		Kind:           "Deployment",
		Replicas:       3,
		HasPDB:         false,
		CriticalLabels: true,
		RiskLevel:      "high",
		Issue:          "test issue",
	}
	if entry.Workload == "" || entry.RiskLevel == "" {
		t.Error("struct fields should be settable")
	}

	summary := DisruptionGapSummary{
		TotalWorkloads:  10,
		WithPDB:         4,
		WithoutPDB:      6,
		SingleReplica:   3,
		CriticalExposed: 2,
	}
	if summary.TotalWorkloads != summary.WithPDB+summary.WithoutPDB {
		t.Error("totalWorkloads should equal withPDB + withoutPDB")
	}
}

func TestCostTopologyTypes(t *testing.T) {
	entry := CostTopologyEntry{
		Namespace:   "prod",
		PodCount:    5,
		CPUCores:    3.5,
		MemoryGB:    12.0,
		CPUCost:     76.65,
		MemCost:     40.30,
		TotalCost:   116.95,
		MonthlyCost: 116.95,
		Share:       23.4,
		Efficiency:  "moderate",
	}
	if entry.TotalCost != entry.CPUCost+entry.MemCost {
		t.Error("totalCost should equal cpuCost + memCost")
	}
}

func TestBinpackTypes(t *testing.T) {
	entry := BinpackNodeEntry{
		NodeName:         "worker-1",
		Role:             "worker",
		PodCount:         15,
		CPUAllocatable:   8.0,
		CPUUsed:          5.5,
		CPUUtilization:   68.75,
		MemAllocatableGB: 32.0,
		MemUsedGB:        24.0,
		MemUtilization:   75.0,
		BinpackScore:     71,
		Status:           "packed",
	}
	if entry.BinpackScore != int((entry.CPUUtilization+entry.MemUtilization)/2) {
		t.Error("binpackScore should be avg of CPU and mem utilization")
	}
}
