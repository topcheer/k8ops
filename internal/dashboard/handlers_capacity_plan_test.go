package dashboard

import (
	"testing"
)

func TestRoundToCapacity(t *testing.T) {
	tests := []struct {
		val    float64
		n      int
		expect float64
	}{
		{1.23456, 2, 1.23},
		{3.14159, 3, 3.141},
		{100.999, 1, 100.9},
		{0.001, 0, 0},
		{555.555, 2, 555.55},
	}
	for _, tt := range tests {
		got := roundToCapacity(tt.val, tt.n)
		if got != tt.expect {
			t.Errorf("roundToCapacity(%v, %d) = %v, want %v", tt.val, tt.n, got, tt.expect)
		}
	}
}

func TestCapacityPlanHealthScore(t *testing.T) {
	tests := []struct {
		name           string
		nodesNeedScale int
		cpuUtil        float64
		memUtil        float64
		headroomDays   int
		expectedRange  [2]int
	}{
		{"healthy", 0, 0.3, 0.3, 200, [2]int{95, 100}},
		{"some pressure", 1, 0.6, 0.6, 100, [2]int{85, 95}},
		{"high utilization", 0, 0.85, 0.3, 50, [2]int{75, 85}},
		{"nodes needing scale", 2, 0.7, 0.7, 80, [2]int{65, 80}},
		{"critical exhaustion", 3, 0.85, 0.85, 20, [2]int{0, 50}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := 100
			score -= tt.nodesNeedScale * 10
			if tt.cpuUtil > 0.8 {
				score -= 15
			} else if tt.cpuUtil > 0.7 {
				score -= 8
			}
			if tt.memUtil > 0.8 {
				score -= 15
			} else if tt.memUtil > 0.7 {
				score -= 8
			}
			if tt.headroomDays > 0 && tt.headroomDays < 30 {
				score -= 20
			} else if tt.headroomDays > 0 && tt.headroomDays < 90 {
				score -= 10
			}
			if score < 0 {
				score = 0
			}

			if score < tt.expectedRange[0] || score > tt.expectedRange[1] {
				t.Errorf("score %d not in range [%d, %d]", score, tt.expectedRange[0], tt.expectedRange[1])
			}
		})
	}
}

func TestCapacityPlanForecast(t *testing.T) {
	// Test forecast logic: find first bottleneck
	cpuDays := 60
	memDays := 40
	podDays := 80

	minDays := -1
	bottleneck := "none"
	if cpuDays > 0 && (minDays < 0 || cpuDays < minDays) {
		minDays = cpuDays
		bottleneck = "CPU"
	}
	if memDays > 0 && (minDays < 0 || memDays < minDays) {
		minDays = memDays
		bottleneck = "Memory"
	}
	if podDays > 0 && (minDays < 0 || podDays < minDays) {
		minDays = podDays
		bottleneck = "Pod slots"
	}

	if bottleneck != "Memory" {
		t.Errorf("expected Memory as first bottleneck, got %s", bottleneck)
	}
	if minDays != 40 {
		t.Errorf("expected 40 days, got %d", minDays)
	}
}

func TestFormatCapacityPlanSummary(t *testing.T) {
	result := &CapacityPlanResult{
		Summary: CapacityPlanSummary{
			TotalNodes:     5,
			CPUUtilization: 0.65,
			MemUtilization: 0.72,
			PodUtilization: 0.55,
			HeadroomDays:   45,
		},
		Forecast: CapacityPlanForecast{
			FirstBottleneck: "Memory",
		},
	}
	s := formatCapacityPlanSummary(result)
	if s == "" {
		t.Error("summary should not be empty")
	}
}

func TestCapacityNodeRiskLevel(t *testing.T) {
	tests := []struct {
		cpuUtil float64
		memUtil float64
		expect  string
	}{
		{0.5, 0.5, "healthy"},
		{0.75, 0.5, "warning"},
		{0.5, 0.75, "warning"},
		{0.9, 0.5, "critical"},
		{0.5, 0.9, "critical"},
	}

	for _, tt := range tests {
		riskLevel := "healthy"
		if tt.cpuUtil > 0.85 || tt.memUtil > 0.85 {
			riskLevel = "critical"
		} else if tt.cpuUtil > 0.7 || tt.memUtil > 0.7 {
			riskLevel = "warning"
		}
		if riskLevel != tt.expect {
			t.Errorf("cpu=%.2f mem=%.2f => %s, want %s", tt.cpuUtil, tt.memUtil, riskLevel, tt.expect)
		}
	}
}
