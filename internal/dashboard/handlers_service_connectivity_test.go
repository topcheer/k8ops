package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeMeta(name, ns string, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels}
}

func TestSVCHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  SVCSummary
		minScore int
		maxScore int
	}{
		{"all healthy", SVCSummary{TotalServices: 10, HealthyServices: 10}, 90, 100},
		{"all unhealthy", SVCSummary{TotalServices: 10, HealthyServices: 0, ZeroEndpoints: 10}, 0, 10},
		{"no services", SVCSummary{TotalServices: 0}, 95, 100},
		{"mixed", SVCSummary{TotalServices: 20, HealthyServices: 15, ZeroEndpoints: 3}, 60, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := svcHealthScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestFormatSelector(t *testing.T) {
	tests := []struct {
		input map[string]string
		want  string
	}{
		{nil, "<none>"},
		{map[string]string{}, "<none>"},
		{map[string]string{"app": "web"}, "app=web"},
		{map[string]string{"app": "web", "tier": "frontend"}, "app=web,tier=frontend"},
	}
	for _, tt := range tests {
		got := formatSelector(tt.input)
		if got != tt.want {
			t.Errorf("formatSelector(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCountMatchingPods(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: makeMeta("pod1", "default", map[string]string{"app": "web"})},
		{ObjectMeta: makeMeta("pod2", "default", map[string]string{"app": "db"})},
		{ObjectMeta: makeMeta("pod3", "default", map[string]string{"app": "web", "tier": "backend"})},
		{ObjectMeta: makeMeta("pod4", "default", map[string]string{})},
	}
	tests := []struct {
		selector map[string]string
		want     int
	}{
		{map[string]string{"app": "web"}, 2},
		{map[string]string{"app": "db"}, 1},
		{map[string]string{"app": "web", "tier": "backend"}, 1},
		{map[string]string{"app": "cache"}, 0},
		{map[string]string{}, 4},
	}
	for _, tt := range tests {
		got := countMatchingPods(pods, tt.selector)
		if got != tt.want {
			t.Errorf("countMatchingPods(%v) = %d, want %d", tt.selector, got, tt.want)
		}
	}
}

func TestSVCRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &SVCResult{Summary: SVCSummary{TotalServices: 10, HealthyServices: 10}}
		recs := svcRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &SVCResult{
			Summary: SVCSummary{
				TotalServices: 10, HealthyServices: 5,
				ZeroEndpoints: 3, NotReadyEndpoints: 2,
			},
			SelectorGaps: []SVCEntry{{Name: "svc1"}},
		}
		recs := svcRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
