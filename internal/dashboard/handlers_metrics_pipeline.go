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

// MetricsPipelineResult is the metrics pipeline & kube-state-metrics health analysis.
type MetricsPipelineResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         MetricsPipelineSummary `json:"summary"`
	Components      []MetricsComponent     `json:"components"`
	ByNamespace     []MetricsNSStat        `json:"byNamespace"`
	Issues          []MetricsIssue         `json:"issues"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// MetricsPipelineSummary aggregates metrics pipeline statistics.
type MetricsPipelineSummary struct {
	MetricsServerDetected bool `json:"metricsServerDetected"`
	KSMDetected           bool `json:"kubeStateMetricsDetected"`
	NodeExporterDetected  bool `json:"nodeExporterDetected"`
	PrometheusDetected    bool `json:"prometheusDetected"`
	TotalComponents       int  `json:"totalComponents"`
	HealthyComponents     int  `json:"healthyComponents"`
	UnhealthyComponents   int  `json:"unhealthyComponents"`
	MetricsServerReady    bool `json:"metricsServerReady"`
	KSMReady              bool `json:"ksmReady"`
	NodeExporterReady     bool `json:"nodeExporterReady"`
}

// MetricsComponent describes one observability component.
type MetricsComponent struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // metrics-server, kube-state-metrics, node-exporter, prometheus
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Image     string `json:"image"`
	Replicas  int    `json:"replicas"`
	RiskLevel string `json:"riskLevel"`
}

// MetricsNSStat per-namespace metrics component stats.
type MetricsNSStat struct {
	Namespace      string `json:"namespace"`
	ComponentCount int    `json:"componentCount"`
	Unhealthy      int    `json:"unhealthy"`
}

// MetricsIssue is a detected metrics pipeline problem.
type MetricsIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleMetricsPipeline audits metrics pipeline & kube-state-metrics health.
// GET /api/operations/metrics-pipeline
func (s *Server) handleMetricsPipeline(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &MetricsPipelineResult{
		ScannedAt: time.Now(),
	}

	// Detect observability components by scanning all pods
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	componentMap := map[string]bool{
		"metrics-server":     false,
		"kube-state-metrics": false,
		"node-exporter":      false,
		"prometheus":         false,
	}

	var components []MetricsComponent
	nsStats := make(map[string]*MetricsNSStat)

	for i := range pods.Items {
		pod := &pods.Items[i]
		podName := strings.ToLower(pod.Name)

		// Detect component kind
		kind := ""
		for key := range componentMap {
			if strings.Contains(podName, key) {
				kind = key
				componentMap[key] = true
				break
			}
		}
		// Also check container images
		if kind == "" {
			for _, c := range pod.Spec.Containers {
				img := strings.ToLower(c.Image)
				for key := range componentMap {
					if strings.Contains(img, key) {
						kind = key
						componentMap[key] = true
						break
					}
				}
				if kind != "" {
					break
				}
			}
		}
		if kind == "" {
			continue
		}

		// Check pod health
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}

		image := ""
		if len(pod.Spec.Containers) > 0 {
			image = pod.Spec.Containers[0].Image
		}

		entry := MetricsComponent{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Kind:      kind,
			Ready:     ready,
			Restarts:  restarts,
			Image:     image,
			Replicas:  1,
			RiskLevel: "healthy",
		}

		if !ready {
			entry.RiskLevel = "critical"
			result.Issues = append(result.Issues, MetricsIssue{
				Severity: "critical",
				Type:     "component-not-ready",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("%s pod is not ready — metrics collection may be impaired", kind),
			})
		}
		if restarts > 3 {
			entry.RiskLevel = "warning"
			result.Issues = append(result.Issues, MetricsIssue{
				Severity: "warning",
				Type:     "high-restarts",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("%s pod has %d restarts — may indicate instability", kind, restarts),
			})
		}

		components = append(components, entry)

		// Update namespace stats
		if _, ok := nsStats[pod.Namespace]; !ok {
			nsStats[pod.Namespace] = &MetricsNSStat{Namespace: pod.Namespace}
		}
		nsStats[pod.Namespace].ComponentCount++
		if !ready {
			nsStats[pod.Namespace].Unhealthy++
		}
	}

	// Aggregate
	healthyCount := 0
	unhealthyCount := 0
	for _, c := range components {
		if c.Ready {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	// Check for missing critical components
	if !componentMap["metrics-server"] {
		result.Issues = append(result.Issues, MetricsIssue{
			Severity: "critical",
			Type:     "metrics-server-missing",
			Resource: "cluster",
			Message:  "metrics-server is not detected — HPA and kubectl top will not work. Install metrics-server for resource metrics",
		})
	}
	if !componentMap["kube-state-metrics"] {
		result.Issues = append(result.Issues, MetricsIssue{
			Severity: "warning",
			Type:     "ksm-missing",
			Resource: "cluster",
			Message:  "kube-state-metrics is not detected — Prometheus will lack cluster state metrics. Install kube-state-metrics for object state metrics",
		})
	}
	if !componentMap["node-exporter"] {
		result.Issues = append(result.Issues, MetricsIssue{
			Severity: "info",
			Type:     "node-exporter-missing",
			Resource: "cluster",
			Message:  "node-exporter is not detected — Prometheus will lack node-level metrics (CPU, memory, disk, network). Install node-exporter for node metrics",
		})
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Unhealthy > result.ByNamespace[j].Unhealthy
	})

	sort.Slice(components, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[components[i].RiskLevel] < riskOrder[components[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if !componentMap["metrics-server"] {
		recommendations = append(recommendations, "Install metrics-server for CPU/memory metrics — required for HPA and kubectl top")
	}
	if !componentMap["kube-state-metrics"] {
		recommendations = append(recommendations, "Install kube-state-metrics for cluster object state metrics — required for Prometheus alerting on deployments, pods, etc.")
	}
	if !componentMap["node-exporter"] {
		recommendations = append(recommendations, "Install node-exporter for node-level hardware metrics — enables capacity planning and node health monitoring")
	}
	if unhealthyCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d observability component(s) are unhealthy — check pod logs and events", unhealthyCount))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Metrics pipeline is healthy — all components are running and ready")
	}

	result.Components = components
	result.Recommendations = recommendations
	result.Summary = MetricsPipelineSummary{
		MetricsServerDetected: componentMap["metrics-server"],
		KSMDetected:           componentMap["kube-state-metrics"],
		NodeExporterDetected:  componentMap["node-exporter"],
		PrometheusDetected:    componentMap["prometheus"],
		TotalComponents:       len(components),
		HealthyComponents:     healthyCount,
		UnhealthyComponents:   unhealthyCount,
		MetricsServerReady:    componentMap["metrics-server"] && anyReady(components, "metrics-server"),
		KSMReady:              componentMap["kube-state-metrics"] && anyReady(components, "kube-state-metrics"),
		NodeExporterReady:     componentMap["node-exporter"] && anyReady(components, "node-exporter"),
	}
	result.HealthScore = computeMetricsPipelineScore(result.Summary, len(result.Issues))

	writeJSON(w, result)
}

// anyReady checks if any component of the given kind is ready.
func anyReady(components []MetricsComponent, kind string) bool {
	for _, c := range components {
		if c.Kind == kind && c.Ready {
			return true
		}
	}
	return false
}

// computeMetricsPipelineScore computes a 0-100 health score.
func computeMetricsPipelineScore(s MetricsPipelineSummary, issueCount int) int {
	score := 100
	if !s.MetricsServerDetected {
		score -= 25
	}
	if !s.KSMDetected {
		score -= 15
	}
	if !s.NodeExporterDetected {
		score -= 10
	}
	score -= s.UnhealthyComponents * 10
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
