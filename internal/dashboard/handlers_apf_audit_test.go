package dashboard

import (
	"testing"
)

func TestAPFScore(t *testing.T) {
	tests := []struct {
		name     string
		s        APFSummary
		minScore int
		maxScore int
	}{
		{"perfect", APFSummary{FlowSchemaCount: 10, PriorityLevelCount: 5, GlobalDefaultExists: true, LeaderElectionExists: true}, 95, 100},
		{"missing PL", APFSummary{MissingPL: 2, GlobalDefaultExists: true, LeaderElectionExists: true}, 55, 65},
		{"missing defaults", APFSummary{GlobalDefaultExists: false, LeaderElectionExists: false}, 75, 85},
		{"many issues", APFSummary{MissingPL: 3, GlobalDefaultExists: false, LeaderElectionExists: false}, 15, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := apfScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAPFRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		recs := apfRecommendations(APFSummary{FlowSchemaCount: 10, PriorityLevelCount: 5}, []APFIssue{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		issues := []APFIssue{
			{Name: "flow-1", Severity: "critical", Detail: "missing PL"},
			{Name: "global-default", Severity: "warning", Detail: "missing"},
		}
		recs := apfRecommendations(APFSummary{MissingPL: 1, GlobalDefaultExists: false, ExemptFlows: 5}, issues)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
