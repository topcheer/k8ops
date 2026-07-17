package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsPipeResult analyzes metrics pipeline integrity: scraping coverage,
// exporter health, ServiceMonitor presence, and pipeline gaps.
type MetricsPipeResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         MetricsPipeSummary `json:"summary"`
	PipelineGaps    []MetricsGap       `json:"pipelineGaps"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type MetricsPipeSummary struct {
	HasPrometheus    bool `json:"hasPrometheus"`
	HasVMAgent       bool `json:"hasVMAgent"`
	HasGrafanaAgent  bool `json:"hasGrafanaAgent"`
	HasOTelCollector bool `json:"hasOTelCollector"`
	ExportersFound   int  `json:"exportersFound"`
	ScrapingTargets  int  `json:"scrapingTargets"`
	BlindWorkloads   int  `json:"blindWorkloads"`
}

type MetricsGap struct {
	Category string `json:"category"`
	Gap      string `json:"gap"`
	Severity string `json:"severity"`
}

// handleMetricsPipe analyzes metrics pipeline integrity.
// GET /api/operations/metrics-pipeline
func (s *Server) handleMetricsPipe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MetricsPipeResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Detect metrics backends and agents
	metricsKeywords := map[string]string{
		"prometheus": "prometheus", "victoria-metrics": "prometheus", "vmagent": "vmagent",
		"grafana-agent": "grafana-agent", "otel": "otel", "opentelemetry": "otel",
		"node-exporter": "exporter", "kube-state-metrics": "exporter",
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, tool := range metricsKeywords {
				if strings.Contains(imgLower, kw) {
					switch tool {
					case "prometheus":
						result.Summary.HasPrometheus = true
					case "vmagent":
						result.Summary.HasVMAgent = true
					case "grafana-agent":
						result.Summary.HasGrafanaAgent = true
					case "otel":
						result.Summary.HasOTelCollector = true
					case "exporter":
						result.Summary.ExportersFound++
					}
				}
			}
		}
	}

	// Count scraping targets: services with prometheus annotations
	scrapingTargets := 0
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		for k := range svc.Annotations {
			if strings.Contains(strings.ToLower(k), "prometheus.io/scrape") {
				scrapingTargets++
				break
			}
		}
	}
	result.Summary.ScrapingTargets = scrapingTargets

	// Count blind workloads (no annotations, no scrape targets in their namespace)
	nsHasScrape := map[string]bool{}
	for _, svc := range services.Items {
		for k := range svc.Annotations {
			if strings.Contains(strings.ToLower(k), "prometheus.io/scrape") {
				nsHasScrape[svc.Namespace] = true
			}
		}
	}
	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		if !nsHasScrape[dep.Namespace] {
			result.Summary.BlindWorkloads++
		}
	}

	// Gaps
	if !result.Summary.HasPrometheus {
		result.PipelineGaps = append(result.PipelineGaps, MetricsGap{Category: "backend", Gap: "No Prometheus/VictoriaMetrics backend", Severity: "critical"})
	}
	if result.Summary.ExportersFound == 0 {
		result.PipelineGaps = append(result.PipelineGaps, MetricsGap{Category: "exporters", Gap: "No node-exporter or kube-state-metrics found", Severity: "high"})
	}
	if result.Summary.BlindWorkloads > 0 {
		result.PipelineGaps = append(result.PipelineGaps, MetricsGap{Category: "coverage", Gap: fmt.Sprintf("%d workloads without metrics scraping", result.Summary.BlindWorkloads), Severity: "medium"})
	}

	// Score
	score := 0
	if result.Summary.HasPrometheus {
		score += 35
	}
	if result.Summary.ExportersFound >= 2 {
		score += 25
	}
	if result.Summary.ExportersFound >= 1 {
		score += 10
	}
	if result.Summary.ScrapingTargets > 0 {
		score += 15
	}
	if len(deployments.Items) > 0 && result.Summary.BlindWorkloads < len(deployments.Items)/2 {
		score += 15
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.PipelineGaps, func(i, j int) bool {
		return result.PipelineGaps[i].Severity > result.PipelineGaps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Metrics pipeline: %d/100 (grade %s) — exporters:%d targets:%d blind:%d", result.HealthScore, result.Grade, result.Summary.ExportersFound, result.Summary.ScrapingTargets, result.Summary.BlindWorkloads))
	if !result.Summary.HasPrometheus {
		recs = append(recs, "Deploy Prometheus or VictoriaMetrics for metric storage")
	}
	if result.Summary.ExportersFound == 0 {
		recs = append(recs, "Install node-exporter and kube-state-metrics")
	}
	if result.Summary.BlindWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without metrics — add prometheus.io/scrape annotations", result.Summary.BlindWorkloads))
	}
	if len(recs) == 1 {
		recs = append(recs, "Metrics pipeline is comprehensive")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
