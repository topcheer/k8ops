package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDNSHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  DNSHealthSummary
		minScore int
		maxScore int
	}{
		{"healthy", DNSHealthSummary{CoreDNSFound: 2, CoreDNSReady: 2, ConfigMapFound: true}, 90, 100},
		{"no coredns", DNSHealthSummary{CoreDNSFound: 0}, 0, 0},
		{"partial unhealthy", DNSHealthSummary{CoreDNSFound: 2, CoreDNSReady: 1, CoreDNSNotReady: 1, ConfigMapFound: true, PodsMissingDNS: 3}, 55, 80},
		{"no configmap", DNSHealthSummary{CoreDNSFound: 1, CoreDNSReady: 1, ConfigMapFound: false}, 60, 80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := dnsHealthScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestIsCoreDNSPod(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			"coredns pod",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "coredns-abc"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Image: "registry.k8s.io/coredns:v1.10.1"},
				}},
			},
			true,
		},
		{
			"normal pod",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "app-pod"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Image: "nginx:latest"},
				}},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCoreDNSPod(tt.pod); got != tt.want {
				t.Errorf("isCoreDNSPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzeDNSConfig(t *testing.T) {
	t.Run("missing Corefile", func(t *testing.T) {
		cm := &corev1.ConfigMap{Data: map[string]string{}}
		issues := analyzeDNSConfig(cm)
		if len(issues) == 0 {
			t.Error("expected issues for missing Corefile")
		}
	})

	t.Run("full config", func(t *testing.T) {
		cm := &corev1.ConfigMap{Data: map[string]string{
			"Corefile": ".:53 {\n  cache 30\n  ready\n  health\n  prometheus :9153\n  kubernetes cluster.local\n}",
		}}
		issues := analyzeDNSConfig(cm)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for complete config, got %d", len(issues))
		}
	})

	t.Run("missing plugins", func(t *testing.T) {
		cm := &corev1.ConfigMap{Data: map[string]string{
			"Corefile": ".:53 {\n  kubernetes cluster.local\n}",
		}}
		issues := analyzeDNSConfig(cm)
		if len(issues) < 3 {
			t.Errorf("expected at least 3 issues, got %d", len(issues))
		}
	})
}

func TestDNSHealthRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &DNSHealthResult{Summary: DNSHealthSummary{CoreDNSFound: 2, CoreDNSReady: 2, ConfigMapFound: true}}
		recs := dnsHealthRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("critical no dns", func(t *testing.T) {
		r := &DNSHealthResult{Summary: DNSHealthSummary{CoreDNSFound: 0}}
		recs := dnsHealthRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
}
