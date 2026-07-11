package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestTopoSpreadScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  TopoSpreadSummary
		minScore int
		maxScore int
	}{
		{"all with spread", TopoSpreadSummary{TotalWorkloads: 10, WithSpread: 10}, 90, 100},
		{"none with spread", TopoSpreadSummary{TotalWorkloads: 10, WithoutSpread: 10, ViolationCount: 10}, 0, 30},
		{"no workloads", TopoSpreadSummary{}, 95, 100},
		{"partial", TopoSpreadSummary{TotalWorkloads: 20, WithSpread: 12, WithoutSpread: 8, ViolationCount: 3}, 55, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := topoSpreadScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestComputeSkew(t *testing.T) {
	tests := []struct {
		dist map[string]int
		want int
	}{
		{map[string]int{"a": 5, "b": 5, "c": 5}, 0},
		{map[string]int{"a": 10, "b": 2}, 8},
		{map[string]int{"a": 3}, 0},
		{map[string]int{}, 0},
	}
	for _, tt := range tests {
		got := computeSkew(tt.dist)
		if got != tt.want {
			t.Errorf("computeSkew(%v) = %d, want %d", tt.dist, got, tt.want)
		}
	}
}

func TestAnalyzeSpreadFromPodTemplate(t *testing.T) {
	maxSkew := int32(1)
	t.Run("with constraints", func(t *testing.T) {
		replicas := int32(3)
		spec := &corev1.PodSpec{
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
				{MaxSkew: maxSkew, TopologyKey: "topology.kubernetes.io/zone", WhenUnsatisfiable: corev1.DoNotSchedule},
			},
		}
		entry := analyzeSpreadFromPodTemplate("app", "default", "Deployment", &replicas, spec)
		if !entry.HasConstraints {
			t.Error("expected hasConstraints")
		}
		if entry.MaxSkew != 1 {
			t.Errorf("maxSkew = %d, want 1", entry.MaxSkew)
		}
	})

	t.Run("without constraints multi-replica", func(t *testing.T) {
		replicas := int32(3)
		spec := &corev1.PodSpec{}
		entry := analyzeSpreadFromPodTemplate("app", "default", "Deployment", &replicas, spec)
		if entry.HasConstraints {
			t.Error("expected no constraints")
		}
		if entry.RiskLevel != "high" {
			t.Errorf("risk = %s, want high", entry.RiskLevel)
		}
	})

	t.Run("single replica no constraints", func(t *testing.T) {
		replicas := int32(1)
		spec := &corev1.PodSpec{}
		entry := analyzeSpreadFromPodTemplate("app", "default", "Deployment", &replicas, spec)
		if entry.RiskLevel != "low" {
			t.Errorf("risk = %s, want low", entry.RiskLevel)
		}
	})
}

func TestTopoSpreadRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &TopoSpreadResult{Summary: TopoSpreadSummary{TotalWorkloads: 10, WithSpread: 10}}
		recs := topoSpreadRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &TopoSpreadResult{
			Summary:        TopoSpreadSummary{WithoutSpread: 5},
			DomainAnalysis: []DomainStat{{TopologyKey: "zone", MaxSkew: 5}},
		}
		recs := topoSpreadRecommendations(r)
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
}
