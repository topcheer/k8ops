package dashboard

import (
	"testing"
)

func TestAssessNodePressureRiskLow(t *testing.T) {
	entry := NodePressureEntry{
		CPUUsagePct: 50,
		MemUsagePct: 50,
		PodUsagePct: 40,
		Conditions:  []NodeCondition{{Type: "Ready", Status: "True"}},
		Status:      "ready",
	}
	if level := assessNodePressureRisk(entry); level != "low" {
		t.Errorf("Expected low for healthy node, got %s", level)
	}
}

func TestAssessNodePressureRiskDiskPressure(t *testing.T) {
	entry := NodePressureEntry{
		Conditions: []NodeCondition{
			{Type: "DiskPressure", Status: "True"},
		},
	}
	// 25 risk → medium... wait: 25 >= 20 → high
	if level := assessNodePressureRisk(entry); level != "high" {
		t.Errorf("Expected high for DiskPressure, got %s", level)
	}
}

func TestAssessNodePressureRiskCritical(t *testing.T) {
	entry := NodePressureEntry{
		CPUUsagePct: 95, // +20
		MemUsagePct: 98, // +20
		Conditions: []NodeCondition{
			{Type: "DiskPressure", Status: "True"}, // +25
		},
	}
	// 25+20+20 = 65 → critical
	if level := assessNodePressureRisk(entry); level != "critical" {
		t.Errorf("Expected critical for multiple pressures, got %s", level)
	}
}

func TestAssessNodePressureRiskHighCPU(t *testing.T) {
	entry := NodePressureEntry{
		CPUUsagePct: 85, // +10
		MemUsagePct: 90, // +10
		Conditions:  []NodeCondition{{Type: "Ready", Status: "True"}},
	}
	// 10+10 = 20 → high
	if level := assessNodePressureRisk(entry); level != "high" {
		t.Errorf("Expected high for high CPU+mem, got %s", level)
	}
}

func TestCalculateNodePressureScore(t *testing.T) {
	// Perfect
	perfect := NodePressureSummary{TotalNodes: 3}
	if score := calculateNodePressureScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := NodePressureSummary{
		TotalNodes:     5,
		DiskPressure:   1, // -10
		MemoryPressure: 1, // -10
		CPUHigh:        2, // -6
		MemoryHigh:     1, // -3
	}
	// 100 - 10 - 10 - 6 - 3 = 71
	score := calculateNodePressureScore(withIssues)
	if score != 71 {
		t.Errorf("Expected 71, got %d", score)
	}

	// Floor at 0
	terrible := NodePressureSummary{
		TotalNodes:    3,
		NotReadyNodes: 10, // -150
	}
	if score := calculateNodePressureScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty
	if score := calculateNodePressureScore(NodePressureSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateNodePressureRecs(t *testing.T) {
	s := NodePressureSummary{
		DiskPressure:   1,
		MemoryPressure: 1,
		PIDPressure:    1,
		NotReadyNodes:  1,
		CPUHigh:        2,
		MemoryHigh:     1,
		CordonedNodes:  1,
		PressureScore:  45,
	}

	recs := generateNodePressureRecs(s)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundDisk := false
	foundCPU := false
	foundCordon := false
	for _, r := range recs {
		if containsSubstr(r, "DiskPressure") {
			foundDisk = true
		}
		if containsSubstr(r, "CPU") {
			foundCPU = true
		}
		if containsSubstr(r, "cordoned") {
			foundCordon = true
		}
	}
	if !foundDisk {
		t.Error("Expected recommendation about DiskPressure")
	}
	if !foundCPU {
		t.Error("Expected recommendation about CPU saturation")
	}
	if !foundCordon {
		t.Error("Expected recommendation about cordoned nodes")
	}
}

func TestGenerateNodePressureRecsClean(t *testing.T) {
	s := NodePressureSummary{
		TotalNodes:    3,
		HealthyNodes:  3,
		PressureScore: 100,
	}

	recs := generateNodePressureRecs(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean cluster, got %d", len(recs))
	}
}

func TestNodePressureRank(t *testing.T) {
	if nodePressureRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if nodePressureRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if nodePressureRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if nodePressureRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}
