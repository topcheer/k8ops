package dashboard

import "testing"

func TestMACScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  MACSummary
		minScore int
		maxScore int
	}{
		{
			name: "all AppArmor+SELinux",
			summary: MACSummary{
				TotalPods:      10,
				WithAppArmor:   10,
				WithSELinux:    10,
				HasNodeSELinux: true,
			},
			minScore: 50,
			maxScore: 100,
		},
		{
			name: "nothing set",
			summary: MACSummary{
				TotalPods:          10,
				UnconfinedAppArmor: 3,
				MissingAppArmor:    7,
			},
			minScore: 0,
			maxScore: 20,
		},
		{
			name: "no pods",
			summary: MACSummary{
				TotalPods: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "partial compliance",
			summary: MACSummary{
				TotalPods:       20,
				WithAppArmor:    12,
				WithSELinux:     8,
				HasNodeSELinux:  true,
				MissingAppArmor: 8,
			},
			minScore: 5,
			maxScore: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := macScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestMACRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &MACResult{
			Summary: MACSummary{
				TotalPods:       10,
				WithAppArmor:    10,
				WithSELinux:     10,
				HasNodeAppArmor: true,
				HasNodeSELinux:  true,
			},
		}
		recs := macRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with issues", func(t *testing.T) {
		r := &MACResult{
			Summary: MACSummary{
				TotalPods:          10,
				UnconfinedAppArmor: 2,
				PermissiveSELinux:  1,
				MissingAppArmor:    5,
				MissingSELinux:     3,
				HasNodeSELinux:     true,
			},
		}
		recs := macRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
