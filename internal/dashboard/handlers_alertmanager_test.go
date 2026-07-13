package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAlertmanagerScore(t *testing.T) {
	tests := []struct {
		name     string
		s        AlertmanagerSummary
		minScore int
		maxScore int
	}{
		{"no alertmanager", AlertmanagerSummary{}, 48, 52},
		{"healthy", AlertmanagerSummary{HasAlertmanager: true, TotalReceivers: 5, TotalRoutes: 3}, 95, 100},
		{"no channels", AlertmanagerSummary{HasAlertmanager: true, NoSlackOrPagerDuty: 2}, 65, 75},
		{"no group_by", AlertmanagerSummary{HasAlertmanager: true, NoSilencePitfalls: 3}, 80, 90},
		{"all bad", AlertmanagerSummary{HasAlertmanager: true, NoSlackOrPagerDuty: 3, NoSilencePitfalls: 3}, 30, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := alertmanagerScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAlertmanagerRecommendations(t *testing.T) {
	t.Run("no alertmanager", func(t *testing.T) {
		recs := alertmanagerRecommendations(AlertmanagerSummary{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := alertmanagerRecommendations(AlertmanagerSummary{
			HasAlertmanager: true, NoSlackOrPagerDuty: 1, NoSilencePitfalls: 2,
		})
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
	t.Run("healthy", func(t *testing.T) {
		recs := alertmanagerRecommendations(AlertmanagerSummary{
			HasAlertmanager: true, TotalReceivers: 3, NoSlackOrPagerDuty: 0, NoSilencePitfalls: 0,
		})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
}

func TestAlertmanagerAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "alertmanager-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Image: "prom/alertmanager:v0.27"}}},
		},
	}

	configMaps := []corev1.ConfigMap{
		// Good config with slack and group_by
		{
			ObjectMeta: metav1.ObjectMeta{Name: "alertmanager-config", Namespace: "monitoring"},
			Data: map[string]string{
				"alertmanager.yml": `
global:
  resolve_timeout: 5m
route:
  group_by: ['alertname', 'namespace']
  receivers: ['slack']
receivers:
  - name: 'slack'
    slack_configs:
      - channel: '#alerts'
`,
			},
		},
		// Bad config: no group_by, no notification channel
		{
			ObjectMeta: metav1.ObjectMeta{Name: "alertmanager-bad", Namespace: "default"},
			Data: map[string]string{
				"config.yaml": `
route:
  receivers: ['null']
receivers:
  - name: 'null'
`,
			},
		},
		// Non-AM config (should be ignored)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-config", Namespace: "monitoring"},
			Data:       map[string]string{"prometheus.yml": "global:\n  scrape_interval: 30s"},
		},
	}

	result := alertmanagerAuditCore(pods, configMaps)

	if !result.Summary.HasAlertmanager {
		t.Error("expected hasAlertmanager=true")
	}
	if result.Summary.TotalConfigs != 2 {
		t.Errorf("expected totalConfigs=2, got %d", result.Summary.TotalConfigs)
	}
	if len(result.Issues) < 2 {
		t.Errorf("expected at least 2 issues, got %d", len(result.Issues))
	}
	if len(result.Configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result.Configs))
	}
	// First config should have slack and group_by
	if !result.Configs[0].HasSlack {
		t.Error("expected first config to have slack")
	}
	if !result.Configs[0].HasGroupBy {
		t.Error("expected first config to have group_by")
	}
	// Second config should NOT have slack or group_by
	if result.Configs[1].HasGroupBy {
		t.Error("expected second config to NOT have group_by")
	}
	if len(result.Recommendations) < 1 {
		t.Error("expected at least 1 recommendation")
	}
}
