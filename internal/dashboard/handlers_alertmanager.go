package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertmanagerResult is the Alertmanager config & alert routing health audit.
type AlertmanagerResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         AlertmanagerSummary  `json:"summary"`
	Configs         []AlertmanagerConfig `json:"configs"`
	Issues          []AlertmanagerIssue  `json:"issues"`
	ByNamespace     []AlertmanagerNSStat `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

// AlertmanagerSummary aggregates Alertmanager health statistics.
type AlertmanagerSummary struct {
	HasAlertmanager    bool `json:"hasAlertmanager"`
	TotalConfigs       int  `json:"totalConfigs"`
	TotalReceivers     int  `json:"totalReceivers"`
	TotalRoutes        int  `json:"totalRoutes"`
	NoSilencePitfalls  int  `json:"noSilencePitfalls"`  // configs with no group_by
	NoSlackOrPagerDuty int  `json:"noSlackOrPagerDuty"` // no real notification channel
	ConfigsWithErrors  int  `json:"configsWithErrors"`
	HealthScore        int  `json:"healthScore"`
}

// AlertmanagerConfig describes one Alertmanager config found in a ConfigMap.
type AlertmanagerConfig struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Receivers    int    `json:"receivers"`
	Routes       int    `json:"routes"`
	HasGroupBy   bool   `json:"hasGroupBy"`
	HasSlack     bool   `json:"hasSlack"`
	HasPagerDuty bool   `json:"hasPagerDuty"`
	HasEmail     bool   `json:"hasEmail"`
	HasWebhook   bool   `json:"hasWebhook"`
}

// AlertmanagerIssue is a detected Alertmanager config issue.
type AlertmanagerIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// AlertmanagerNSStat shows Alertmanager presence per namespace.
type AlertmanagerNSStat struct {
	Namespace string `json:"namespace"`
	HasConfig bool   `json:"hasConfig"`
}

// alertmanagerAuditCore performs the audit on pods and configmaps (testable).
func alertmanagerAuditCore(pods []corev1.Pod, configMaps []corev1.ConfigMap) AlertmanagerResult {
	result := AlertmanagerResult{
		ScannedAt: time.Now(),
	}

	// Detect Alertmanager pods
	for i := range pods {
		pod := &pods[i]
		podName := strings.ToLower(pod.Name)
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			if strings.Contains(podName, "alertmanager") || strings.Contains(img, "alertmanager") {
				result.Summary.HasAlertmanager = true
				break
			}
		}
	}

	nsStats := make(map[string]*AlertmanagerNSStat)

	// Scan ConfigMaps for Alertmanager config
	for i := range configMaps {
		cm := &configMaps[i]
		isAMConfig := false
		if strings.Contains(strings.ToLower(cm.Name), "alertmanager") {
			isAMConfig = true
		}
		for _, data := range cm.Data {
			if strings.Contains(data, "alertmanager") || strings.Contains(data, "receivers:") || strings.Contains(data, "route:") {
				isAMConfig = true
				break
			}
		}
		if !isAMConfig {
			continue
		}

		ns := cm.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &AlertmanagerNSStat{Namespace: ns}
		}
		nsStats[ns].HasConfig = true

		entry := AlertmanagerConfig{
			Name:      cm.Name,
			Namespace: ns,
		}

		// Analyze config data
		for _, data := range cm.Data {
			entry.Receivers += strings.Count(data, "name:") // simplified receiver count
			entry.Routes += strings.Count(data, "matchers:")
			entry.HasGroupBy = strings.Contains(data, "group_by:")
			entry.HasSlack = strings.Contains(data, "slack_configs:") || strings.Contains(data, "slack")
			entry.HasPagerDuty = strings.Contains(data, "pagerduty_configs:") || strings.Contains(data, "pagerduty")
			entry.HasEmail = strings.Contains(data, "email_configs:")
			entry.HasWebhook = strings.Contains(data, "webhook_configs:")
		}

		result.Summary.TotalConfigs++
		result.Summary.TotalReceivers += entry.Receivers
		result.Summary.TotalRoutes += entry.Routes

		if !entry.HasGroupBy {
			result.Summary.NoSilencePitfalls++
			result.Issues = append(result.Issues, AlertmanagerIssue{
				Name: cm.Name, Namespace: ns,
				Issue:    "no group_by configured — alerts will fire individually instead of grouped",
				Severity: "medium",
			})
		}

		if !entry.HasSlack && !entry.HasPagerDuty && !entry.HasEmail && !entry.HasWebhook {
			result.Summary.NoSlackOrPagerDuty++
			result.Issues = append(result.Issues, AlertmanagerIssue{
				Name: cm.Name, Namespace: ns,
				Issue:    "no notification channel detected (no slack/pagerduty/email/webhook) — alerts won't be delivered",
				Severity: "high",
			})
		}

		result.Configs = append(result.Configs, entry)
	}

	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].HasConfig && !result.ByNamespace[j].HasConfig
	})

	result.Summary.HealthScore = alertmanagerScore(result.Summary)
	result.Recommendations = alertmanagerRecommendations(result.Summary)

	return result
}

// alertmanagerScore calculates health score.
func alertmanagerScore(s AlertmanagerSummary) int {
	if !s.HasAlertmanager {
		return 50
	}
	base := 100
	base -= s.NoSlackOrPagerDuty * 15
	base -= s.NoSilencePitfalls * 5
	base -= s.ConfigsWithErrors * 10
	if base < 0 {
		base = 0
	}
	return base
}

// alertmanagerRecommendations generates recommendations.
func alertmanagerRecommendations(s AlertmanagerSummary) []string {
	var recs []string
	if !s.HasAlertmanager {
		recs = append(recs, "Alertmanager not detected — install for alert routing and notification")
		return recs
	}
	if s.NoSlackOrPagerDuty > 0 {
		recs = append(recs, fmt.Sprintf("%d Alertmanager configs have no notification channel — add slack, pagerduty, or email receivers", s.NoSlackOrPagerDuty))
	}
	if s.NoSilencePitfalls > 0 {
		recs = append(recs, fmt.Sprintf("%d configs lack group_by — add grouping to prevent alert storms", s.NoSilencePitfalls))
	}
	if s.TotalReceivers == 0 {
		recs = append(recs, "no alert receivers configured — alerts won't be delivered to any channel")
	}
	if s.NoSlackOrPagerDuty == 0 && s.NoSilencePitfalls == 0 && s.TotalReceivers > 0 {
		recs = append(recs, "Alertmanager is properly configured with notification channels and grouping")
	}
	return recs
}

// handleAlertmanager audits Alertmanager config and alert routing health.
// GET /api/operations/alertmanager-health
func (s *Server) handleAlertmanager(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	configMaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	result := alertmanagerAuditCore(pods.Items, configMaps.Items)
	writeJSON(w, result)
}
