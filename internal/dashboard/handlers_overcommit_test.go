package dashboard

import (
	"sort"
	"testing"
)

func TestCalculatePressureScore(t *testing.T) {
	tests := []struct {
		name   string
		node   OvercommitNode
		expect int
	}{
		{
			"safe",
			OvercommitNode{CPUCommitPct: 30, MemCommitPct: 30, CPULimitCommitPct: 50, MemLimitCommitPct: 50},
			0,
		},
		{
			"moderate-cpu",
			OvercommitNode{CPUCommitPct: 70, MemCommitPct: 30, CPULimitCommitPct: 100, MemLimitCommitPct: 50},
			10,
		},
		{
			"high-request",
			OvercommitNode{CPUCommitPct: 85, MemCommitPct: 85, CPULimitCommitPct: 100, MemLimitCommitPct: 100},
			40,
		},
		{
			"critical-overcommit",
			OvercommitNode{CPUCommitPct: 90, MemCommitPct: 90, CPULimitCommitPct: 350, MemLimitCommitPct: 350},
			100,
		},
		{
			"capped-at-100",
			OvercommitNode{CPUCommitPct: 95, MemCommitPct: 95, CPULimitCommitPct: 400, MemLimitCommitPct: 400},
			100,
		},
	}

	for _, tt := range tests {
		got := calculatePressureScore(tt.node)
		if got != tt.expect {
			t.Errorf("calculatePressureScore(%s) = %d, want %d", tt.name, got, tt.expect)
		}
	}
}

func TestCalculateOvercommitScore(t *testing.T) {
	// Perfect
	perfect := OvercommitSummary{
		TotalNodes:          3,
		TotalCPULimitCommit: 1.0,
		TotalMemLimitCommit: 1.0,
	}
	if score := calculateOvercommitScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := OvercommitSummary{
		TotalNodes:          3,
		NodesCritical:       1,   // -15
		NodesAtRisk:         2,   // includes 1 critical, 1 more at -5
		TotalCPULimitCommit: 2.5, // -5
		TotalMemLimitCommit: 1.5,
		PodsWithoutLimits:   5, // -5
	}
	// 100 - 15 - 5 - 5 - 5 = 70
	score := calculateOvercommitScore(withIssues)
	if score != 70 {
		t.Errorf("Expected 70, got %d", score)
	}

	// Floor at 0
	terrible := OvercommitSummary{
		TotalNodes:          3,
		NodesCritical:       3,   // -45
		TotalCPULimitCommit: 4.0, // -10
		TotalMemLimitCommit: 4.0, // -10
		PodsWithoutLimits:   50,  // -50
	}
	if score := calculateOvercommitScore(terrible); score != 0 {
		t.Errorf("Expected 0 for terrible, got %d", score)
	}
}

func TestGenerateOvercommitRecs(t *testing.T) {
	result := OvercommitResult{
		Summary: OvercommitSummary{
			NodesCritical:       1,
			NodesAtRisk:         3,
			PodsWithoutLimits:   5,
			PodsWithoutRequests: 2,
			TotalCPULimitCommit: 2.5,
			TotalMemLimitCommit: 3.5,
			ClusterScore:        40,
		},
	}

	recs := generateOvercommitRecs(result)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundCritical := false
	foundNoLimits := false
	foundOOMKill := false
	for _, r := range recs {
		if containsSubstr(r, "critical over-commit") {
			foundCritical = true
		}
		if containsSubstr(r, "no resource limits") {
			foundNoLimits = true
		}
		if containsSubstr(r, "OOMKill") {
			foundOOMKill = true
		}
	}
	if !foundCritical {
		t.Error("Expected recommendation about critical over-commit")
	}
	if !foundNoLimits {
		t.Error("Expected recommendation about pods without limits")
	}
	if !foundOOMKill {
		t.Error("Expected recommendation about OOMKill risk")
	}
}

func TestGenerateOvercommitRecsClean(t *testing.T) {
	result := OvercommitResult{
		Summary: OvercommitSummary{
			TotalNodes:          3,
			TotalCPULimitCommit: 1.0,
			TotalMemLimitCommit: 1.0,
			ClusterScore:        100,
		},
	}

	recs := generateOvercommitRecs(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean cluster, got %d", len(recs))
	}
}

func TestGetOrCreateOvercommitNs(t *testing.T) {
	m := make(map[string]*OvercommitNsStat)

	e1 := getOrCreateOvercommitNs(m, "default")
	e1.PodCount = 5

	e2 := getOrCreateOvercommitNs(m, "default")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry with PodCount=5, got %d", e2.PodCount)
	}

	e3 := getOrCreateOvercommitNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected kube-system, got %s", e3.Namespace)
	}
}

func TestOvercommitNodeRiskLevel(t *testing.T) {
	// Test via sorting - nodes with higher pressure should come first
	nodes := []OvercommitNode{
		{Name: "safe", PressureScore: 10, CPULimitCommitPct: 50},
		{Name: "critical", PressureScore: 80, CPULimitCommitPct: 350},
		{Name: "moderate", PressureScore: 30, CPULimitCommitPct: 120},
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].PressureScore > nodes[j].PressureScore
	})

	if nodes[0].Name != "critical" {
		t.Errorf("Expected critical first, got %s", nodes[0].Name)
	}
	if nodes[2].Name != "safe" {
		t.Errorf("Expected safe last, got %s", nodes[2].Name)
	}
}
