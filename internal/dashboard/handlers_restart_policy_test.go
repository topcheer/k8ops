package dashboard

import "testing"

func TestRestartPolicyScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  RestartPolicySummary
		minScore int
		maxScore int
	}{
		{"clean", RestartPolicySummary{TotalWorkloads: 10}, 90, 100},
		{"mismatches", RestartPolicySummary{TotalWorkloads: 10, PolicyMismatches: 3, NoLifecycleHook: 5}, 45, 70},
		{"no workloads", RestartPolicySummary{}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := restartPolicyScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestRestartPolicyRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &RestartPolicyResult{Summary: RestartPolicySummary{TotalWorkloads: 10}}
		recs := restartPolicyRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &RestartPolicyResult{Summary: RestartPolicySummary{PolicyMismatches: 2, NoLifecycleHook: 5}}
		recs := restartPolicyRecommendations(r)
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
}
