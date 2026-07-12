package dashboard

import "testing"

func TestScaleReadyScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  ScaleReadySummary
		minScore int
		maxScore int
	}{
		{"all ready", ScaleReadySummary{TotalWorkloads: 10, WithHPA: 10, WithPDB: 10, HasResources: 10, CanScale: 10}, 90, 100},
		{"no HPA no PDB", ScaleReadySummary{TotalWorkloads: 10, WithoutHPA: 10, WithoutPDB: 10, HasResources: 10}, 40, 55},
		{"no resources", ScaleReadySummary{TotalWorkloads: 10, NoResources: 5, WithoutHPA: 5, WithoutPDB: 5}, 35, 55},
		{"empty", ScaleReadySummary{}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scaleReadyScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestLabelsOverlap(t *testing.T) {
	tests := []struct {
		a    map[string]string
		b    map[string]string
		want bool
	}{
		{map[string]string{"app": "web"}, map[string]string{"app": "web"}, true},
		{map[string]string{"app": "web"}, map[string]string{"app": "db"}, false},
		{map[string]string{"app": "web"}, map[string]string{"tier": "frontend"}, false},
		{map[string]string{"app": "web", "env": "prod"}, map[string]string{"app": "web"}, true},
		{nil, nil, false},
	}
	for _, tt := range tests {
		got := labelsOverlap(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("labelsOverlap(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestScaleReadyRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &ScaleReadyResult{Summary: ScaleReadySummary{TotalWorkloads: 10, WithHPA: 10, WithPDB: 10, HasResources: 10, CanScale: 10}}
		recs := scaleReadyRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with gaps", func(t *testing.T) {
		r := &ScaleReadyResult{Summary: ScaleReadySummary{
			NoResources: 2, WithoutHPA: 5, WithoutPDB: 3, SingleReplica: 4,
		}}
		recs := scaleReadyRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
