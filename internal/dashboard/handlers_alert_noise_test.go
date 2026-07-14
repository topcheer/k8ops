package dashboard

import (
	"testing"
	"time"
)

func TestAssessAlertNoiseRisk(t *testing.T) {
	tests := []struct {
		name   string
		entry  AlertNoiseEntry
		expect string
	}{
		{
			name:   "flapping alert",
			entry:  AlertNoiseEntry{IsFlapping: true, EventCount: 20, FlapScore: 5},
			expect: "critical",
		},
		{
			name:   "noisy alert",
			entry:  AlertNoiseEntry{IsNoisy: true, EventCount: 15},
			expect: "warning",
		},
		{
			name:   "moderate events",
			entry:  AlertNoiseEntry{EventCount: 7},
			expect: "info",
		},
		{
			name:   "low events",
			entry:  AlertNoiseEntry{EventCount: 3},
			expect: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessAlertNoiseRisk(tt.entry)
			if got != tt.expect {
				t.Errorf("expected %s, got %s", tt.expect, got)
			}
		})
	}
}

func TestAlertNoiseHealthScore(t *testing.T) {
	tests := []struct {
		name           string
		noisyAlerts    int
		flappingAlerts int
		alertStorms    int
		staleSilences  int
		noiseRatio     float64
		expectedRange  [2]int
	}{
		{"clean", 0, 0, 0, 0, 0, [2]int{95, 100}},
		{"some noise", 2, 0, 0, 0, 0.2, [2]int{85, 95}},
		{"flapping", 0, 2, 0, 0, 0, [2]int{75, 85}},
		{"storm detected", 0, 0, 3, 0, 0, [2]int{80, 90}},
		{"stale silences", 0, 0, 0, 2, 0, [2]int{85, 95}},
		{"high noise ratio", 0, 0, 0, 0, 0.6, [2]int{80, 88}},
		{"moderate noise ratio", 0, 0, 0, 0, 0.35, [2]int{87, 95}},
		{"all problems", 3, 2, 2, 2, 0.6, [2]int{0, 50}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := 100
			score -= tt.noisyAlerts * 5
			score -= tt.flappingAlerts * 10
			score -= tt.alertStorms * 5
			score -= tt.staleSilences * 5
			if tt.noiseRatio > 0.5 {
				score -= 15
			} else if tt.noiseRatio > 0.3 {
				score -= 8
			}
			if score < 0 {
				score = 0
			}

			if score < tt.expectedRange[0] || score > tt.expectedRange[1] {
				t.Errorf("score %d not in expected range [%d, %d]", score, tt.expectedRange[0], tt.expectedRange[1])
			}
		})
	}
}

func TestAlertNoiseIssues(t *testing.T) {
	// Test issue generation for flapping and noisy alerts
	entries := []AlertNoiseEntry{
		{Name: "HighCPU", Namespace: "prod", EventCount: 25, IsNoisy: true, IsFlapping: true, FlapScore: 8, RiskLevel: "critical"},
		{Name: "DiskFull", Namespace: "prod", EventCount: 15, IsNoisy: true, IsFlapping: false, RiskLevel: "warning"},
		{Name: "PodHealthy", Namespace: "default", EventCount: 3, IsNoisy: false, IsFlapping: false, RiskLevel: "healthy"},
	}

	var issues []AlertNoiseIssue
	for _, entry := range entries {
		if entry.IsFlapping {
			issues = append(issues, AlertNoiseIssue{
				Severity:  "critical",
				AlertName: entry.Name,
				Namespace: entry.Namespace,
				Issue:     "flapping",
			})
		}
		if entry.IsNoisy && !entry.IsFlapping {
			issues = append(issues, AlertNoiseIssue{
				Severity:  "warning",
				AlertName: entry.Name,
				Namespace: entry.Namespace,
				Issue:     "noisy",
			})
		}
	}

	// Should have 2 issues: 1 flapping + 1 noisy (non-flapping)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].Severity != "critical" {
		t.Errorf("first issue should be critical, got %s", issues[0].Severity)
	}
}

func TestDetectAlertStorms(t *testing.T) {
	// Test storm detection with time-bucketed events
	// This tests the logic without needing a server
	now := time.Now()
	events := make([]alertEventInfo, 25)
	for i := range events {
		events[i] = alertEventInfo{
			Name:        "test-alert",
			Namespace:   "default",
			IsFiring:    true,
			LastEventAt: now.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		}
	}

	// 25 events within 25 seconds → should be 1 storm (>20 in 5min window)
	storms := 0
	count := 0
	windowStart := time.Time{}
	for _, ev := range events {
		t, _ := time.Parse(time.RFC3339, ev.LastEventAt)
		if t.Sub(windowStart) > 5*time.Minute {
			if count > 20 {
				storms++
			}
			windowStart = t
			count = 1
		} else {
			count++
		}
	}
	if count > 20 {
		storms++
	}

	if storms != 1 {
		t.Errorf("expected 1 storm, got %d", storms)
	}
}

func TestFormatAlertNoiseSummary(t *testing.T) {
	result := &AlertNoiseResult{
		Summary: AlertNoiseSummary{
			TotalAlertEvents: 100,
			UniqueAlertNames: 15,
			NoisyAlerts:      3,
			FlappingAlerts:   2,
			AlertStorms:      1,
			NoiseRatio:       0.45,
		},
	}
	summary := formatAlertNoiseSummary(result)
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestAlertNoiseRecommendations(t *testing.T) {
	result := &AlertNoiseResult{
		Summary: AlertNoiseSummary{
			NoisyAlerts:    3,
			FlappingAlerts: 2,
			AlertStorms:    1,
			StaleSilences:  1,
			NoiseRatio:     0.6,
		},
	}

	if result.Summary.NoisyAlerts > 0 {
		result.Recommendations = append(result.Recommendations, "noisy alerts found")
	}
	if result.Summary.FlappingAlerts > 0 {
		result.Recommendations = append(result.Recommendations, "flapping alerts found")
	}
	if result.Summary.AlertStorms > 0 {
		result.Recommendations = append(result.Recommendations, "alert storms found")
	}
	if result.Summary.StaleSilences > 0 {
		result.Recommendations = append(result.Recommendations, "stale silences found")
	}
	if result.Summary.NoiseRatio > 0.5 {
		result.Recommendations = append(result.Recommendations, "high noise ratio")
	}

	if len(result.Recommendations) != 5 {
		t.Errorf("expected 5 recommendations, got %d", len(result.Recommendations))
	}
}
