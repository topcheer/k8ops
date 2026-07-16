package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestScoreToLevel(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{95, "elite"},
		{80, "advanced"},
		{65, "intermediate"},
		{45, "developing"},
		{30, "initial"},
	}
	for _, tt := range tests {
		got := scoreToLevel(tt.score)
		if got != tt.want {
			t.Errorf("scoreToLevel(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestScoreToStatusDim(t *testing.T) {
	if s := scoreToStatusDim(85); s != "healthy" {
		t.Errorf("expected 'healthy' for 85, got %q", s)
	}
	if s := scoreToStatusDim(65); s != "warning" {
		t.Errorf("expected 'warning' for 65, got %q", s)
	}
	if s := scoreToStatusDim(40); s != "critical" {
		t.Errorf("expected 'critical' for 40, got %q", s)
	}
}

func TestComputeInfraScore(t *testing.T) {
	// All healthy nodes and pods
	nodes := []corev1.Node{}
	pods := []corev1.Pod{}
	score, detail := computeInfraScore(nodes, pods)
	if score != 100 {
		t.Errorf("expected 100 for empty cluster, got %d (%s)", score, detail)
	}
}

func TestGenerateScorecardRoadmap(t *testing.T) {
	dims := []ScorecardDim{
		{Name: "A", Score: 90},
		{Name: "B", Score: 50},
		{Name: "C", Score: 75},
	}
	roadmap := generateScorecardRoadmap(dims)
	// Only B (50) and C (75) should be in roadmap, A is >= 80
	if len(roadmap) != 2 {
		t.Errorf("expected 2 roadmap items, got %d", len(roadmap))
	}
	// B should be first (lowest score)
	if roadmap[0].Dimension != "B" {
		t.Errorf("expected 'B' first, got %q", roadmap[0].Dimension)
	}
}

func TestGeneratePlatformScorecardRecs(t *testing.T) {
	r := PlatformScorecardResult{
		OverallScore: 75,
		Grade:        "C",
		Level:        "intermediate",
		Dimensions: []ScorecardDim{
			{Name: "Infra", Score: 85, Weight: 0.25, Status: "healthy", Detail: "all good"},
			{Name: "Security", Score: 50, Weight: 0.20, Status: "critical", Detail: "issues found"},
		},
		Strengths:  []ScorecardStrength{{Dimension: "Infra", Detail: "all good"}},
		Weaknesses: []ScorecardWeakness{{Dimension: "Security", Score: 50, Detail: "issues found"}},
		ImprovementRoadmap: []ScorecardRoadmapItem{{Priority: 1, Dimension: "Security", Action: "Fix security"}},
	}
	recs := generatePlatformScorecardRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recs, got %d", len(recs))
	}
}
