package dashboard

import (
	"testing"
)

func TestKubeletHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  KubeletHealthSummary
		minScore int
		maxScore int
	}{
		{
			name:     "all healthy",
			summary:  KubeletHealthSummary{TotalNodes: 10, HealthyNodes: 10},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "many unhealthy",
			summary: KubeletHealthSummary{
				TotalNodes: 10, UnhealthyNodes: 5,
				VersionSkew: 3, RuntimeSkew: 2,
				OldHeartbeatNodes: 2,
			},
			minScore: 0,
			maxScore: 55,
		},
		{
			name: "no nodes",
			summary: KubeletHealthSummary{
				TotalNodes: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "minor skew",
			summary: KubeletHealthSummary{
				TotalNodes: 10, HealthyNodes: 8, UnhealthyNodes: 2,
				VersionSkew: 1,
			}, minScore: 80,
			maxScore: 95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := kubeletHealthScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestParseRuntimeType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"containerd://1.7.0", "containerd"},
		{"docker://20.10.7", "docker"},
		{"cri-o://1.28.0", "cri-o"},
		{"unknown", "unknown"},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseRuntimeType(tt.input)
		if got != tt.want {
			t.Errorf("parseRuntimeType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.28.4", "v1.28"},
		{"v1.28.4+k3s1", "v1.28"},
		{"1.28.0", "1.28"},
		{"v1.30.2-eks", "v1.30"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := extractMajorVersion(tt.input)
		if got != tt.want {
			t.Errorf("extractMajorVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsMajorVersionDiff(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want bool
	}{
		{"v1.28.4", "v1.28.2", false},
		{"v1.28.4", "v1.29.0", true},
		{"v1.28.4+k3s1", "v1.28.2+k3s1", false},
		{"v1.28.4", "v1.30.0", true},
	}
	for _, tt := range tests {
		got := isMajorVersionDiff(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("isMajorVersionDiff(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestGetMajorityKey(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]int
		want  string
	}{
		{"simple", map[string]int{"a": 3, "b": 1}, "a"},
		{"single", map[string]int{"only": 5}, "only"},
		{"empty", map[string]int{}, ""},
		{"tie", map[string]int{"a": 2, "b": 2}, ""}, // first one found or empty (implementation dependent)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getMajorityKey(tt.input)
			if tt.name != "tie" && got != tt.want {
				t.Errorf("getMajorityKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssessKubeletRisk(t *testing.T) {
	tests := []struct {
		heartbeatAge float64
		activeConds  int
		versionSkew  bool
		want         string
	}{
		{0, 0, false, "low"},
		{70, 0, false, "medium"},
		{130, 0, false, "high"},
		{310, 0, false, "critical"},
		{0, 1, false, "medium"},
		{0, 2, true, "critical"},
		{0, 3, true, "critical"},
		{50, 0, true, "medium"},
	}

	for _, tt := range tests {
		got := assessKubeletRisk(tt.heartbeatAge, tt.activeConds, tt.versionSkew)
		if got != tt.want {
			t.Errorf("assessKubeletRisk(%.0f, %d, %v) = %q, want %q", tt.heartbeatAge, tt.activeConds, tt.versionSkew, got, tt.want)
		}
	}
}

func TestKubeletHealthRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &KubeletHealthResult{
			Summary: KubeletHealthSummary{
				TotalNodes: 10, HealthyNodes: 10,
			},
		}
		recs := kubeletHealthRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with issues", func(t *testing.T) {
		r := &KubeletHealthResult{
			Summary: KubeletHealthSummary{
				TotalNodes: 10, UnhealthyNodes: 3,
				VersionSkew: 2, RuntimeSkew: 1,
				OldHeartbeatNodes:   1,
				NodesWithConditions: 2,
			},
		}
		recs := kubeletHealthRecommendations(r)
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}
