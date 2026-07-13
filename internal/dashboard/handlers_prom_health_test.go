package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPromHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		s        PromHealthSummary
		minScore int
		maxScore int
	}{
		{"nothing", PromHealthSummary{}, 0, 5},
		{"all components", PromHealthSummary{HasPrometheus: true, HasAlertmanager: true, HasGrafana: true, HasMetricsServer: true, HasKubeStateMetrics: true}, 85, 100},
		{"prom only", PromHealthSummary{HasPrometheus: true}, 20, 30},
		{"missing alertmanager", PromHealthSummary{HasPrometheus: true, HasGrafana: true, HasMetricsServer: true, HasKubeStateMetrics: true}, 65, 80},
		{"with no-alert penalty", PromHealthSummary{HasPrometheus: true, HasAlertmanager: true, HasGrafana: true, NoAlertRules: 5}, 40, 55},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := promHealthScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestPromHealthRecommendations(t *testing.T) {
	t.Run("nothing installed", func(t *testing.T) {
		recs := promHealthRecommendations(PromHealthSummary{})
		if len(recs) < 5 {
			t.Errorf("expected at least 5 recommendations, got %d", len(recs))
		}
	})
	t.Run("all complete", func(t *testing.T) {
		recs := promHealthRecommendations(PromHealthSummary{
			HasPrometheus: true, HasAlertmanager: true, HasGrafana: true,
			HasMetricsServer: true, HasKubeStateMetrics: true, NoAlertRules: 0,
		})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
}

func TestPromHealthAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Prometheus pod
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-0", Namespace: "monitoring"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "prom/prometheus:v2.50.0"},
			}},
		},
		// Alertmanager pod
		{
			ObjectMeta: metav1.ObjectMeta{Name: "alertmanager-0", Namespace: "monitoring"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "prom/alertmanager:v0.27.0"},
			}},
		},
		// Grafana pod
		{
			ObjectMeta: metav1.ObjectMeta{Name: "grafana-0", Namespace: "monitoring"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "grafana/grafana:10.4.0"},
			}},
		},
		// metrics-server pod
		{
			ObjectMeta: metav1.ObjectMeta{Name: "metrics-server-abc", Namespace: "kube-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "registry.k8s.io/metrics-server/metrics-server:v0.7.0"},
			}},
		},
		// kube-state-metrics pod
		{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-state-metrics-xyz", Namespace: "kube-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.12.0"},
			}},
		},
		// App pod in default namespace (no alert rules)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod-1", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "nginx:1.27"},
			}},
		},
	}

	configMaps := []corev1.ConfigMap{
		// Prometheus rule config map with alerts
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "prometheus-rules",
				Namespace:   "monitoring",
				Annotations: map[string]string{"prometheus.io/rules": "true"},
			},
			Data: map[string]string{
				"rules.yaml": `
groups:
- name: k8s-alerts
  rules:
  - alert: PodCrashLooping
    expr: rate(kube_pod_container_status_restarts_total[5m]) > 0
    for: 5m
  - alert: NodeNotReady
    expr: kube_node_status_condition{condition="Ready",status="true"} == 0
    for: 2m
  - record: pod:cpu_usage
    expr: sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)
`,
			},
		},
		// Non-rule config map (should be ignored)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-config", Namespace: "monitoring"},
			Data:       map[string]string{"prometheus.yaml": "global:\n  scrape_interval: 30s"},
		},
	}

	result := promHealthAuditCore(pods, configMaps)

	if !result.Summary.HasPrometheus {
		t.Error("expected hasPrometheus=true")
	}
	if !result.Summary.HasAlertmanager {
		t.Error("expected hasAlertmanager=true")
	}
	if !result.Summary.HasGrafana {
		t.Error("expected hasGrafana=true")
	}
	if !result.Summary.HasMetricsServer {
		t.Error("expected hasMetricsServer=true")
	}
	if !result.Summary.HasKubeStateMetrics {
		t.Error("expected hasKubeStateMetrics=true")
	}
	if result.Summary.TotalRuleFiles != 1 {
		t.Errorf("expected totalRuleFiles=1, got %d", result.Summary.TotalRuleFiles)
	}
	if result.Summary.AlertRules != 2 {
		t.Errorf("expected alertRules=2, got %d", result.Summary.AlertRules)
	}
	if result.Summary.RecordingRules != 1 {
		t.Errorf("expected recordingRules=1, got %d", result.Summary.RecordingRules)
	}
	// default namespace has pods but no alerts
	if result.Summary.NoAlertRules < 1 {
		t.Errorf("expected noAlertRules>=1, got %d", result.Summary.NoAlertRules)
	}
	if len(result.ConfigMaps) != 1 {
		t.Errorf("expected 1 rule config map, got %d", len(result.ConfigMaps))
	}
	if len(result.Recommendations) < 1 {
		t.Error("expected at least 1 recommendation")
	}
}
