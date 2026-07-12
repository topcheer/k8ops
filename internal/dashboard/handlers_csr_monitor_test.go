package dashboard

import "testing"

func TestCSRScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  CSRSummary
		minScore int
		maxScore int
	}{
		{"clean", CSRSummary{}, 95, 100},
		{"pending", CSRSummary{Pending: 3, StalePending: 1}, 25, 55},
		{"many denied", CSRSummary{Denied: 10}, 85, 95},
		{"all stale", CSRSummary{Pending: 5, StalePending: 5}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := csrScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestCSRRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &CSRResult{Summary: CSRSummary{}}
		recs := csrRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &CSRResult{Summary: CSRSummary{Pending: 3, StalePending: 2, Denied: 5, Total: 150}}
		recs := csrRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
