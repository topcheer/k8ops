package dashboard

import "testing"

func TestForensicsScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  ForensicsSummary
		minScore int
		maxScore int
	}{
		{"clean", ForensicsSummary{TotalPods: 10}, 95, 100},
		{"oom kills", ForensicsSummary{TotalPods: 10, OOMKills: 3, ExitCodeErrors: 3}, 60, 85},
		{"privileged escapes", ForensicsSummary{TotalPods: 10, PrivilegedEscapes: 2}, 55, 85},
		{"no pods", ForensicsSummary{}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := forensicsScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestExitCodeMeaning(t *testing.T) {
	tests := []struct {
		code int32
		want string
	}{
		{0, "Success"},
		{137, "OOMKilled (SIGKILL)"},
		{143, "Terminated (SIGTERM)"},
		{1, "General error"},
		{127, "Command not found"},
		{139, "Segmentation fault (SIGSEGV)"},
		{255, "Killed by signal 127"},
	}
	for _, tt := range tests {
		got := exitCodeMeaning(tt.code)
		if got != tt.want {
			t.Errorf("exitCodeMeaning(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestExitCodeSeverity(t *testing.T) {
	tests := []struct {
		code int32
		want string
	}{
		{137, "high"},
		{143, "low"},
		{1, "medium"},
		{0, "medium"},
		{126, "high"},
	}
	for _, tt := range tests {
		got := exitCodeSeverity(tt.code)
		if got != tt.want {
			t.Errorf("exitCodeSeverity(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestExtractContainerHash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"containerd://sha256:abc123def", "abc123def"},
		{"docker://sha256:xyz", "xyz"},
		{"plainid", "plainid"},
	}
	for _, tt := range tests {
		got := extractContainerHash(tt.input)
		if got != tt.want {
			t.Errorf("extractContainerHash(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractImageHash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sha256:abc123", "abc123"},
		{"docker.io/repo@sha256:xyz789", "xyz789"},
		{"noprefix", "noprefix"},
	}
	for _, tt := range tests {
		got := extractImageHash(tt.input)
		if got != tt.want {
			t.Errorf("extractImageHash(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestForensicsRecommendations(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		r := &ForensicsResult{Summary: ForensicsSummary{TotalPods: 10}}
		recs := forensicsRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &ForensicsResult{Summary: ForensicsSummary{
			OOMKills: 2, SIGKILLTerminations: 1,
			PrivilegedEscapes: 1, HashMismatches: 3,
			ExitCodeErrors: 5,
		}}
		recs := forensicsRecommendations(r)
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}
