package dashboard

import (
	"testing"
)

func TestAssessDensityRisk(t *testing.T) {
	// Cordoned node — low risk
	entry := DensityNodeEntry{IsSchedulable: false, PodCapacityPct: 100}
	if level := assessDensityRisk(entry); level != "low" {
		t.Errorf("Expected low for cordoned, got %s", level)
	}

	// Critical — at max pods
	entry = DensityNodeEntry{IsSchedulable: true, PodCapacityPct: 100}
	if level := assessDensityRisk(entry); level != "critical" {
		t.Errorf("Expected critical for 100%%, got %s", level)
	}

	// High — near full
	entry = DensityNodeEntry{IsSchedulable: true, PodCapacityPct: 88}
	if level := assessDensityRisk(entry); level != "high" {
		t.Errorf("Expected high for 88%%, got %s", level)
	}

	// High — high CPU usage
	entry = DensityNodeEntry{IsSchedulable: true, PodCapacityPct: 50, CPUUsagePct: 92}
	if level := assessDensityRisk(entry); level != "high" {
		t.Errorf("Expected high for 92%% CPU, got %s", level)
	}

	// Medium
	entry = DensityNodeEntry{IsSchedulable: true, PodCapacityPct: 72}
	if level := assessDensityRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 72%%, got %s", level)
	}

	// Low
	entry = DensityNodeEntry{IsSchedulable: true, PodCapacityPct: 30, CPUUsagePct: 40, MemUsagePct: 40}
	if level := assessDensityRisk(entry); level != "low" {
		t.Errorf("Expected low for healthy node, got %s", level)
	}
}

func TestStdDev(t *testing.T) {
	// All same values — std dev = 0
	if sd := stdDev([]float64{50, 50, 50, 50}); sd != 0 {
		t.Errorf("Expected 0 for uniform, got %f", sd)
	}

	// Known values
	sd := stdDev([]float64{0, 100})
	// mean=50, diff=[-50, 50], sq=[2500, 2500], var=2500, sd=50
	if sd != 50 {
		t.Errorf("Expected 50, got %f", sd)
	}

	// Empty
	if sd := stdDev(nil); sd != 0 {
		t.Errorf("Expected 0 for empty, got %f", sd)
	}
}

func TestSqrt(t *testing.T) {
	if r := sqrt(0); r != 0 {
		t.Errorf("Expected 0 for sqrt(0), got %f", r)
	}
	if r := sqrt(100); r < 9.9 || r > 10.1 {
		t.Errorf("Expected ~10 for sqrt(100), got %f", r)
	}
	if r := sqrt(-1); r != 0 {
		t.Errorf("Expected 0 for sqrt(-1), got %f", r)
	}
}

func TestAnalyzeBinPacking(t *testing.T) {
	// Perfect balance
	bp := analyzeBinPacking([]float64{50, 50, 50}, []float64{50, 50, 50}, []float64{20, 20, 20})
	if bp.Score != 100 {
		t.Errorf("Expected score 100 for balanced, got %d", bp.Score)
	}
	if bp.Strategy != "spread" {
		t.Errorf("Expected spread strategy, got %s", bp.Strategy)
	}

	// Very uneven
	bp = analyzeBinPacking([]float64{10, 90, 30}, []float64{20, 85, 40}, []float64{5, 50, 15})
	if bp.ImbalanceScore <= 25 {
		t.Errorf("Expected high imbalance score >25, got %f", bp.ImbalanceScore)
	}
	if bp.Strategy != "uneven" {
		t.Errorf("Expected uneven strategy, got %s", bp.Strategy)
	}

	// Empty
	bp = analyzeBinPacking(nil, nil, nil)
	if bp.Score != 100 {
		t.Errorf("Expected 100 for empty, got %d", bp.Score)
	}
}

func TestCalculateDensityScore(t *testing.T) {
	// Clean
	s := DensitySummary{SchedulableNodes: 3, TotalHeadroomPods: 200}
	if score := calculateDensityScore(s); score != 100 {
		t.Errorf("Expected 100 for clean, got %d", score)
	}

	// With issues
	s = DensitySummary{
		SchedulableNodes:  3,
		NodesFull:         1,  // -15
		NodesNearFull:     1,  // -5
		TotalHeadroomPods: 30, // -10
	}
	// 100 - 15 - 5 - 10 = 70
	if score := calculateDensityScore(s); score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	// No schedulable nodes
	s = DensitySummary{SchedulableNodes: 0}
	if score := calculateDensityScore(s); score != 0 {
		t.Errorf("Expected 0 for no schedulable nodes, got %d", score)
	}
}

func TestGenerateDensityRecs(t *testing.T) {
	s := DensitySummary{
		NodesFull:         1,
		NodesNearFull:     2,
		TotalHeadroomPods: 50,
		CPUHeadroomCores:  2,
		MemHeadroomGB:     4,
		HealthScore:       40,
	}
	bp := BinPackingAnalysis{ImbalanceScore: 30}
	fragments := []FragmentEntry{{Node: "node-1", Cause: "CPU exhausted"}}

	recs := generateDensityRecs(s, bp, []string{"node-1"}, fragments)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundFull := false
	foundLowHeadroom := false
	foundFragment := false
	foundImbalance := false
	for _, r := range recs {
		if containsSubstr(r, "max pod capacity") {
			foundFull = true
		}
		if containsSubstr(r, "50 pod slots") {
			foundLowHeadroom = true
		}
		if containsSubstr(r, "fragmentation") {
			foundFragment = true
		}
		if containsSubstr(r, "imbalance") {
			foundImbalance = true
		}
	}
	if !foundFull {
		t.Error("Expected recommendation about full nodes")
	}
	if !foundLowHeadroom {
		t.Error("Expected recommendation about low headroom")
	}
	if !foundFragment {
		t.Error("Expected recommendation about fragmentation")
	}
	if !foundImbalance {
		t.Error("Expected recommendation about imbalance")
	}
}

func TestGenerateDensityRecsClean(t *testing.T) {
	s := DensitySummary{
		SchedulableNodes:  5,
		TotalHeadroomPods: 500,
		CPUHeadroomCores:  20,
		MemHeadroomGB:     64,
		HealthScore:       100,
	}
	recs := generateDensityRecs(s, BinPackingAnalysis{ImbalanceScore: 5}, nil, nil)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestDensityRiskRank(t *testing.T) {
	if densityRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if densityRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if densityRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if densityRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
