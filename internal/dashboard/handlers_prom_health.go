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

// PromHealthResult is the Prometheus rule health & alert coverage audit.
type PromHealthResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         PromHealthSummary    `json:"summary"`
	ByNamespace     []PromNSStat         `json:"byNamespace"`
	ConfigMaps      []PromConfigMapEntry `json:"configMaps"`
	Issues          []PromIssue          `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

// PromHealthSummary aggregates Prometheus health statistics.
type PromHealthSummary struct {
	HasPrometheus       bool `json:"hasPrometheus"`
	HasAlertmanager     bool `json:"hasAlertmanager"`
	HasGrafana          bool `json:"hasGrafana"`
	HasMetricsServer    bool `json:"hasMetricsServer"`
	HasKubeStateMetrics bool `json:"hasKubeStateMetrics"`
	TotalRuleFiles      int  `json:"totalRuleFiles"`
	TotalRules          int  `json:"totalRules"`
	AlertRules          int  `json:"alertRules"`
	RecordingRules      int  `json:"recordingRules"`
	NoAlertRules        int  `json:"noAlertRules"` // namespaces with no alerting coverage
	HealthScore         int  `json:"healthScore"`
}

// PromConfigMapEntry describes a PrometheusRule config map.
type PromConfigMapEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	RuleCount  int    `json:"ruleCount"`
	AlertCount int    `json:"alertCount"`
	HasAlerts  bool   `json:"hasAlerts"`
}

// PromNSStat shows Prometheus coverage per namespace.
type PromNSStat struct {
	Namespace string `json:"namespace"`
	HasRules  bool   `json:"hasRules"`
	HasAlerts bool   `json:"hasAlerts"`
	PodCount  int    `json:"podCount"`
}

// PromIssue is a detected observability issue.
type PromIssue struct {
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// promHealthAuditCore performs the observability stack audit (testable).
func promHealthAuditCore(pods []corev1.Pod, configMaps []corev1.ConfigMap) PromHealthResult {
	result := PromHealthResult{
		ScannedAt: time.Now(),
	}

	// Detect observability stack components by image/name patterns
	for i := range pods {
		pod := &pods[i]
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			name := strings.ToLower(pod.Name)

			if strings.Contains(img, "prometheus") && !strings.Contains(img, "alertmanager") && !strings.Contains(img, "exporter") {
				result.Summary.HasPrometheus = true
			}
			if strings.Contains(img, "alertmanager") || strings.Contains(name, "alertmanager") {
				result.Summary.HasAlertmanager = true
			}
			if strings.Contains(img, "grafana") || strings.Contains(name, "grafana") {
				result.Summary.HasGrafana = true
			}
			if strings.Contains(img, "metrics-server") || strings.Contains(name, "metrics-server") {
				result.Summary.HasMetricsServer = true
			}
			if strings.Contains(img, "kube-state-metrics") || strings.Contains(name, "kube-state-metrics") {
				result.Summary.HasKubeStateMetrics = true
			}
		}
	}

	// Count pods per namespace for coverage analysis
	podCountByNS := make(map[string]int)
	for i := range pods {
		podCountByNS[pods[i].Namespace]++
	}

	// Scan ConfigMaps for Prometheus rules
	rulesByNS := make(map[string]*PromNSStat)
	for i := range configMaps {
		cm := &configMaps[i]
		// Prometheus rules are typically in ConfigMaps with prometheus.io/rule annotation or in PrometheusRule CRD
		isRuleCM := false
		if _, ok := cm.Annotations["prometheus.io/rules"]; ok {
			isRuleCM = true
		}
		if _, ok := cm.Labels["prometheus"]; ok {
			isRuleCM = true
		}
		if strings.Contains(strings.ToLower(cm.Name), "prometheus") && strings.Contains(strings.ToLower(cm.Name), "rule") {
			isRuleCM = true
		}

		if !isRuleCM {
			continue
		}

		ns := cm.Namespace
		if _, ok := rulesByNS[ns]; !ok {
			rulesByNS[ns] = &PromNSStat{Namespace: ns, PodCount: podCountByNS[ns]}
		}
		rulesByNS[ns].HasRules = true

		entry := PromConfigMapEntry{
			Name:      cm.Name,
			Namespace: ns,
		}

		ruleCount := 0
		alertCount := 0
		for _, data := range cm.Data {
			// Count alert: and record: keywords in rule data
			lines := strings.Split(data, "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "- alert:") || strings.HasPrefix(trimmed, "alert:") {
					alertCount++
					ruleCount++
				} else if strings.HasPrefix(trimmed, "- record:") || strings.HasPrefix(trimmed, "record:") {
					ruleCount++
				}
			}
		}

		entry.RuleCount = ruleCount
		entry.AlertCount = alertCount
		entry.HasAlerts = alertCount > 0
		if alertCount > 0 {
			rulesByNS[ns].HasAlerts = true
		}

		result.Summary.TotalRuleFiles++
		result.Summary.TotalRules += ruleCount
		result.Summary.AlertRules += alertCount
		result.Summary.RecordingRules += ruleCount - alertCount
		result.ConfigMaps = append(result.ConfigMaps, entry)
	}

	// Build namespace stats and find namespaces with pods but no alerts
	allNamespaces := make(map[string]bool)
	for ns := range podCountByNS {
		allNamespaces[ns] = true
	}
	for ns := range rulesByNS {
		allNamespaces[ns] = true
	}

	for ns := range allNamespaces {
		if _, ok := rulesByNS[ns]; !ok {
			rulesByNS[ns] = &PromNSStat{Namespace: ns, PodCount: podCountByNS[ns]}
		}
		if podCountByNS[ns] > 0 && !rulesByNS[ns].HasAlerts {
			result.Summary.NoAlertRules++
			result.Issues = append(result.Issues, PromIssue{
				Namespace: ns,
				Issue:     fmt.Sprintf("namespace has %d pods but no Prometheus alert rules — blind spot for alerting", podCountByNS[ns]),
				Severity:  "medium",
			})
		}
	}

	for _, stat := range rulesByNS {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodCount > result.ByNamespace[j].PodCount
	})

	sort.Slice(result.ConfigMaps, func(i, j int) bool {
		return result.ConfigMaps[i].AlertCount > result.ConfigMaps[j].AlertCount
	})

	result.Summary.HealthScore = promHealthScore(result.Summary)
	result.Recommendations = promHealthRecommendations(result.Summary)

	return result
}

// promHealthScore calculates health score.
func promHealthScore(s PromHealthSummary) int {
	base := 0
	if s.HasPrometheus {
		base += 25
	}
	if s.HasAlertmanager {
		base += 20
	}
	if s.HasGrafana {
		base += 15
	}
	if s.HasMetricsServer {
		base += 15
	}
	if s.HasKubeStateMetrics {
		base += 15
	}
	// Penalty for namespaces without alerts
	base -= s.NoAlertRules * 2
	if base < 0 {
		base = 0
	}
	if base > 100 {
		base = 100
	}
	return base
}

// promHealthRecommendations generates recommendations.
func promHealthRecommendations(s PromHealthSummary) []string {
	var recs []string
	if !s.HasPrometheus {
		recs = append(recs, "Prometheus not detected — install Prometheus for metrics collection and alerting")
	}
	if !s.HasAlertmanager {
		recs = append(recs, "Alertmanager not detected — install Alertmanager for alert routing and notification")
	}
	if !s.HasGrafana {
		recs = append(recs, "Grafana not detected — install Grafana for metrics visualization and dashboards")
	}
	if !s.HasMetricsServer {
		recs = append(recs, "metrics-server not detected — install for HPA and kubectl top support")
	}
	if !s.HasKubeStateMetrics {
		recs = append(recs, "kube-state-metrics not detected — install for cluster state metrics (pods, deployments, etc.)")
	}
	if s.NoAlertRules > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have no alert rules — add PrometheusRule for critical workload alerts", s.NoAlertRules))
	}
	if s.HasPrometheus && s.HasAlertmanager && s.HasGrafana && s.HasMetricsServer && s.HasKubeStateMetrics && s.NoAlertRules == 0 {
		recs = append(recs, "observability stack is complete — all components detected and alerting coverage is comprehensive")
	}
	return recs
}

// handlePromHealth audits Prometheus rule health and alert coverage.
// GET /api/operations/prom-health
func (s *Server) handlePromHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	configMaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	result := promHealthAuditCore(pods.Items, configMaps.Items)
	writeJSON(w, result)
}
