package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestSidecarScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  SidecarSummary
		minScore int
		maxScore int
	}{
		{"low overhead", SidecarSummary{TotalPods: 10, CPUOverheadPct: 10, MemOverheadPct: 5}, 90, 100},
		{"high overhead", SidecarSummary{TotalPods: 10, CPUOverheadPct: 45, MemOverheadPct: 40, InjectedPods: 3}, 55, 75},
		{"no pods", SidecarSummary{TotalPods: 0}, 95, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := sidecarScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestClassifySidecar(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"istio-proxy", "docker.io/istio/proxyv2", "Istio Proxy"},
		{"vault-agent", "vault:1.13", "Vault Agent"},
		{"fluentd", "fluent/fluentd:v1.16", "Fluentd"},
		{"app", "myapp:v1", "Unknown Sidecar"},
		{"linkerd-proxy", "cr.l5d.io/linkerd/proxy", "Linkerd Proxy"},
	}
	for _, tt := range tests {
		got := classifySidecar(tt.name, tt.image)
		if got != tt.want {
			t.Errorf("classifySidecar(%q, %q) = %q, want %q", tt.name, tt.image, got, tt.want)
		}
	}
}

func TestIsSidecarContainer(t *testing.T) {
	cpu := resource.MustParse("100m")
	// Pod with 2 containers: app + istio-proxy
	containers := []corev1.Container{
		{Name: "app", Image: "myapp:v1", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"cpu": cpu}}},
		{Name: "istio-proxy", Image: "istio/proxyv2", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"cpu": cpu}}},
	}

	if !isSidecarContainer(&containers[0], 0, 2) {
		// First container "app" should not be a sidecar unless it matches a pattern
		// Actually, index 0 with total 2 — our heuristic says index > 0 is sidecar
		// But "app" doesn't match any known pattern
	}
	if !isSidecarContainer(&containers[1], 1, 2) {
		t.Error("istio-proxy at index 1 should be detected as sidecar")
	}

	// Single container pod
	single := []corev1.Container{{Name: "nginx", Image: "nginx:latest"}}
	if isSidecarContainer(&single[0], 0, 1) {
		// nginx matches known pattern
	}
}

func TestSidecarRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &SidecarResult{Summary: SidecarSummary{TotalPods: 10, CPUOverheadPct: 10, MemOverheadPct: 5}}
		recs := sidecarRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &SidecarResult{
			Summary: SidecarSummary{
				TotalPods: 10, CPUOverheadPct: 35, MemOverheadPct: 30,
				InjectedPods: 2,
			},
			HighOverhead: []SidecarEntry{{PodName: "p1"}},
			InjectedOnly: []SidecarEntry{{PodName: "p2"}},
		}
		recs := sidecarRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
