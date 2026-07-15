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

// MetricsPipelineAuditResult is the metrics collection pipeline integrity audit.
type MetricsPipelineAuditResult struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	Summary         MetricsPipelineAuditSummary `json:"summary"`
	Components      []MetricsAuditComponent     `json:"components"`
	Coverage        MetricsAuditCoverage        `json:"coverage"`
	Gaps            []MetricsAuditGap           `json:"gaps,omitempty"`
	Scorecard       MetricsAuditScorecard       `json:"scorecard"`
	Recommendations []string                    `json:"recommendations"`
	HealthScore     int                         `json:"healthScore"`
}

// MetricsPipelineAuditSummary aggregates pipeline statistics.
type MetricsPipelineAuditSummary struct {
	HasScraper          bool `json:"hasScraper"`          // prometheus/victoria-metrics
	HasAgent            bool `json:"hasAgent"`            // node-exporter, dcgm-exporter
	HasStorage          bool `json:"hasStorage"`          // long-term retention
	HasVisualizer       bool `json:"hasVisualizer"`       // grafana
	HasAlerter          bool `json:"hasAlerter"`          // alertmanager
	HasKSM              bool `json:"hasKubeStateMetrics"` // kube-state-metrics
	HasNodeExporter     bool `json:"hasNodeExporter"`
	TotalComponents     int  `json:"totalComponents"`
	HealthyComponents   int  `json:"healthyComponents"`
	UnhealthyComponents int  `json:"unhealthyComponents"`
	MissingComponents   int  `json:"missingComponents"`
}

// MetricsAuditComponent describes one metrics pipeline component.
type MetricsAuditComponent struct {
	Name     string `json:"name"`
	Category string `json:"category"` // scraper, agent, storage, visualizer, alerter
	Image    string `json:"image,omitempty"`
	Replicas int    `json:"replicas"`
	Ready    int    `json:"readyReplicas"`
	Healthy  bool   `json:"healthy"`
	Found    bool   `json:"found"`
	Version  string `json:"version,omitempty"`
}

// MetricsAuditCoverage shows what metrics sources are covered.
type MetricsAuditCoverage struct {
	NodesCovered     int     `json:"nodesCovered"`
	TotalNodes       int     `json:"totalNodes"`
	PodsCovered      int     `json:"podsCovered"`
	TotalPods        int     `json:"totalPods"`
	NSWithMonitoring int     `json:"namespacesWithMonitoring"`
	TotalNamespaces  int     `json:"totalNamespaces"`
	NodeCoveragePct  float64 `json:"nodeCoveragePct"`
	PodCoveragePct   float64 `json:"podCoveragePct"`
	NSCoveragePct    float64 `json:"nsCoveragePct"`
}

// MetricsAuditGap identifies a missing metrics pipeline capability.
type MetricsAuditGap struct {
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Component  string `json:"component"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion,omitempty"`
}

// MetricsAuditScorecard rates the metrics pipeline completeness.
type MetricsAuditScorecard struct {
	Collection    int `json:"collection"`    // 0-100
	Storage       int `json:"storage"`       // 0-100
	Visualization int `json:"visualization"` // 0-100
	Alerting      int `json:"alerting"`      // 0-100
	Coverage      int `json:"coverage"`      // 0-100
	Overall       int `json:"overall"`       // 0-100
}

// handleMetricsPipeline audits the metrics collection pipeline integrity.
// GET /api/operations/metrics-pipeline-audit
func (s *Server) handleMetricsPipelineHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MetricsPipelineAuditResult{ScannedAt: time.Now()}

	// 1. Detect metrics pipeline components from pods
	componentPatterns := map[string]string{
		"prometheus":         "scraper",
		"victoria-metrics":   "scraper",
		"thanos":             "storage",
		"mimir":              "storage",
		"node-exporter":      "agent",
		"dcgm-exporter":      "agent",
		"kube-state-metrics": "agent",
		"metrics-server":     "agent",
		"grafana":            "visualizer",
		"alertmanager":       "alerter",
		"opa-exporter":       "agent",
		"blackbox-exporter":  "agent",
	}

	componentMap := map[string]*MetricsAuditComponent{}
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			nameLower := strings.ToLower(pod.Name)
			for pattern, category := range componentPatterns {
				if strings.Contains(nameLower, pattern) {
					compName := pattern
					if componentMap[compName] == nil {
						componentMap[compName] = &MetricsAuditComponent{
							Name:     compName,
							Category: category,
							Found:    true,
						}
					}
					componentMap[compName].Replicas++
					allReady := true
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							allReady = false
						}
						if componentMap[compName].Image == "" {
							componentMap[compName].Image = cs.Image
							imgParts := strings.Split(cs.Image, ":")
							if len(imgParts) > 1 {
								componentMap[compName].Version = imgParts[len(imgParts)-1]
							}
						}
					}
					if allReady {
						componentMap[compName].Ready++
					}
					break
				}
			}
		}
	}

	// 2. Build component list and health
	var components []MetricsAuditComponent
	for _, comp := range componentMap {
		comp.Healthy = comp.Ready > 0
		components = append(components, *comp)
	}
	sort.Slice(components, func(i, j int) bool {
		return components[i].Category < components[j].Category
	})
	result.Components = components

	// 3. Summary
	result.Summary.TotalComponents = len(components)
	for _, comp := range components {
		if comp.Healthy {
			result.Summary.HealthyComponents++
		} else {
			result.Summary.UnhealthyComponents++
		}
	}

	// Check specific components
	for _, comp := range components {
		switch comp.Name {
		case "prometheus", "victoria-metrics":
			result.Summary.HasScraper = true
		case "node-exporter":
			result.Summary.HasNodeExporter = true
		case "kube-state-metrics":
			result.Summary.HasKSM = true
		case "thanos", "mimir":
			result.Summary.HasStorage = true
		case "grafana":
			result.Summary.HasVisualizer = true
		case "alertmanager":
			result.Summary.HasAlerter = true
		case "metrics-server":
			result.Summary.HasAgent = true
		}
	}

	// 4. Calculate coverage
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	result.Coverage.TotalNodes = len(nodes.Items)
	result.Coverage.TotalNamespaces = len(namespaces.Items)

	// Estimate coverage from DaemonSet presence (node-exporter runs as DS)
	if result.Summary.HasNodeExporter {
		result.Coverage.NodesCovered = result.Coverage.TotalNodes
	} else {
		// Prometheus still scrapes kubelet/cAdvisor on all nodes
		if result.Summary.HasScraper {
			result.Coverage.NodesCovered = result.Coverage.TotalNodes
		}
	}

	if pods != nil {
		result.Coverage.TotalPods = len(pods.Items)
		// Pods covered if kube-state-metrics exists (it exports all pod metrics)
		if result.Summary.HasKSM {
			result.Coverage.PodsCovered = result.Coverage.TotalPods
		}
	}

	// Namespaces with monitoring annotations
	if pods != nil {
		nsWithMon := map[string]bool{}
		for _, pod := range pods.Items {
			if pod.Annotations != nil {
				if _, ok := pod.Annotations["prometheus.io/scrape"]; ok {
					nsWithMon[pod.Namespace] = true
				}
			}
		}
		result.Coverage.NSWithMonitoring = len(nsWithMon)
	}

	if result.Coverage.TotalNodes > 0 {
		result.Coverage.NodeCoveragePct = float64(result.Coverage.NodesCovered) / float64(result.Coverage.TotalNodes) * 100
	}
	if result.Coverage.TotalPods > 0 {
		result.Coverage.PodCoveragePct = float64(result.Coverage.PodsCovered) / float64(result.Coverage.TotalPods) * 100
	}
	if result.Coverage.TotalNamespaces > 0 {
		result.Coverage.NSCoveragePct = float64(result.Coverage.NSWithMonitoring) / float64(result.Coverage.TotalNamespaces) * 100
	}

	// 5. Detect gaps
	var gaps []MetricsAuditGap
	if !result.Summary.HasScraper {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "critical", Category: "collection", Component: "prometheus",
			Issue:      "No metrics scraper detected (Prometheus/VictoriaMetrics)",
			Suggestion: "Deploy Prometheus or VictoriaMetrics for metrics collection",
		})
	}
	if !result.Summary.HasNodeExporter {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "high", Category: "agent", Component: "node-exporter",
			Issue:      "No node-exporter — missing host-level CPU/memory/disk metrics",
			Suggestion: "Deploy node-exporter as DaemonSet for infrastructure visibility",
		})
	}
	if !result.Summary.HasKSM {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "high", Category: "agent", Component: "kube-state-metrics",
			Issue:      "No kube-state-metrics — missing object state metrics (pods, deployments, nodes)",
			Suggestion: "Deploy kube-state-metrics for Kubernetes object metrics",
		})
	}
	if !result.Summary.HasVisualizer {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "medium", Category: "visualization", Component: "grafana",
			Issue:      "No Grafana — no dashboard visualization layer",
			Suggestion: "Deploy Grafana for metrics visualization and dashboards",
		})
	}
	if !result.Summary.HasAlerter {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "high", Category: "alerting", Component: "alertmanager",
			Issue:      "No Alertmanager — alerting pipeline incomplete",
			Suggestion: "Deploy Alertmanager for alert routing and notification",
		})
	}
	if !result.Summary.HasStorage && result.Summary.HasScraper {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "medium", Category: "storage", Component: "thanos/mimir",
			Issue:      "No long-term metrics storage — historical data limited to Prometheus retention",
			Suggestion: "Deploy Thanos or Mimir for long-term metrics storage and query federation",
		})
	}
	if result.Coverage.NSCoveragePct < 50 && result.Coverage.TotalNamespaces > 3 {
		gaps = append(gaps, MetricsAuditGap{
			Severity: "info", Category: "coverage", Component: "scrape-annotations",
			Issue:      fmt.Sprintf("Only %.0f%% of namespaces have prometheus.io/scrape annotations", result.Coverage.NSCoveragePct),
			Suggestion: "Add scrape annotations to application pods in uncovered namespaces",
		})
	}
	result.Gaps = gaps
	result.Summary.MissingComponents = len(gaps)

	// 6. Scorecard
	collection := 0
	if result.Summary.HasScraper {
		collection += 50
	}
	if result.Summary.HasNodeExporter {
		collection += 25
	}
	if result.Summary.HasKSM {
		collection += 25
	}

	storage := 50
	if result.Summary.HasStorage {
		storage = 100
	}

	visualizer := 0
	if result.Summary.HasVisualizer {
		visualizer = 100
	}

	alerting := 0
	if result.Summary.HasAlerter {
		alerting = 100
	}

	coverage := int(result.Coverage.NodeCoveragePct*0.4 + result.Coverage.PodCoveragePct*0.4 + result.Coverage.NSCoveragePct*0.2)

	overall := (collection + storage + visualizer + alerting + coverage) / 5

	result.Scorecard = MetricsAuditScorecard{
		Collection:    collection,
		Storage:       storage,
		Visualization: visualizer,
		Alerting:      alerting,
		Coverage:      coverage,
		Overall:       overall,
	}
	result.HealthScore = overall

	// 7. Recommendations
	result.Recommendations = generateMetricsPipelineRecs(result)

	writeJSON(w, result)
}

// generateMetricsPipelineRecs produces recommendations.
func generateMetricsPipelineRecs(result MetricsPipelineAuditResult) []string {
	var recs []string

	if result.Summary.HasScraper {
		recs = append(recs, fmt.Sprintf("Metrics scraper detected (%d components) — collection pipeline is active", result.Summary.HealthyComponents))
	} else {
		recs = append(recs, "No metrics scraper — deploy Prometheus or VictoriaMetrics for basic observability")
	}

	for _, gap := range result.Gaps {
		recs = append(recs, fmt.Sprintf("[%s] %s — %s", gap.Severity, gap.Issue, gap.Suggestion))
	}

	if result.Scorecard.Overall >= 80 {
		recs = append(recs, fmt.Sprintf("Metrics pipeline scorecard: %d/100 — pipeline is comprehensive", result.Scorecard.Overall))
	} else if result.Scorecard.Overall >= 50 {
		recs = append(recs, fmt.Sprintf("Metrics pipeline scorecard: %d/100 — some gaps exist", result.Scorecard.Overall))
	} else {
		recs = append(recs, fmt.Sprintf("Metrics pipeline scorecard: %d/100 — significant gaps require attention", result.Scorecard.Overall))
	}

	if result.Coverage.NSCoveragePct < 50 {
		recs = append(recs, fmt.Sprintf("Namespace coverage %.0f%% — add prometheus.io/scrape annotations to application pods", result.Coverage.NSCoveragePct))
	}

	return recs
}
