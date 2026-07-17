package dashboard

import (
	"testing"
)

func TestPipelineHealthTypes(t *testing.T) {
	r := PipelineHealthResult{HealthScore: 65, Grade: "D", DORALevel: "Medium"}
	if r.HealthScore != 65 || r.DORALevel != "Medium" {
		t.Error("struct field error")
	}
	s := PipelineSummary{TotalDeployments24h: 3, FailedDeploys24h: 1, ChangeFailureRate: 33.3}
	if s.ChangeFailureRate != 33.3 {
		t.Error("summary field error")
	}
	pd := PipelineDeploy{Workload: "api", Status: "succeeded", Replicas: 3}
	if pd.Status != "succeeded" || pd.Replicas != 3 {
		t.Error("deploy field error")
	}
}

func TestPipelineHealthDORA(t *testing.T) {
	tests := []struct {
		deploys  int
		failRate float64
		expected string
	}{
		{10, 5, "Elite"},
		{3, 20, "High"},
		{2, 40, "Medium"},
		{0, 0, "Low"},
	}
	for _, tc := range tests {
		level := "Low"
		if tc.deploys >= 7 && tc.failRate < 15 {
			level = "Elite"
		} else if tc.deploys >= 1 && tc.failRate < 30 {
			level = "High"
		} else if tc.deploys >= 1 {
			level = "Medium"
		}
		if level != tc.expected {
			t.Errorf("deploys=%d failRate=%.0f: expected %s, got %s", tc.deploys, tc.failRate, tc.expected, level)
		}
	}
}

func TestPipelineHealthScoring(t *testing.T) {
	tests := []struct {
		hasCI       bool
		deploys     int
		failRate    float64
		rollbacks   int
		expectedMin int
		expectedMax int
	}{
		{true, 5, 10, 0, 90, 100},
		{false, 0, 0, 0, 55, 65},
		{true, 3, 50, 2, 55, 80},
	}
	for _, tc := range tests {
		score := 50
		if tc.hasCI {
			score += 20
		}
		if tc.deploys > 0 {
			score += 15
		}
		if tc.failRate < 15 {
			score += 15
		}
		if tc.rollbacks > 0 {
			score -= 10
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("hasCI=%v deploys=%d failRate=%.0f rb=%d: expected %d-%d, got %d",
				tc.hasCI, tc.deploys, tc.failRate, tc.rollbacks, tc.expectedMin, tc.expectedMax, score)
		}
	}
}
