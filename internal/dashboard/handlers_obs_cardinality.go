package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObsCardinalityResult analyzes observability data cardinality risk:
// Prometheus metric label explosion, log volume distribution, trace span volume,
// and observability data cost optimization opportunities.
type ObsCardinalityResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	Summary          ObsCardSummary    `json:"summary"`
	CardinalityRisks []CardinalityRisk `json:"cardinalityRisks"`
	LogVolumeByNS    []LogVolumeNS     `json:"logVolumeByNS"`
	CollectorHealth  []CollectorHealth `json:"collectorHealth"`
	RiskScore        int               `json:"riskScore"`
	Grade            string            `json:"grade"`
	EstMonthlyCost   float64           `json:"estMonthlyCost"`
	Recommendations  []string          `json:"recommendations"`
}

type ObsCardSummary struct {
	HasPrometheus     bool  `json:"hasPrometheus"`
	HasFluentBit      bool  `json:"hasFluentBit"`
	HasLoki           bool  `json:"hasLoki"`
	HasJaeger         bool  `json:"hasJaeger"`
	HasOTelCollector  bool  `json:"hasOTelCollector"`
	CollectorPods     int   `json:"collectorPods"`
	HighCardLabels    int   `json:"highCardLabels"`
	TotalNamespaces   int   `json:"totalNamespaces"`
	NSWithLogAgent    int   `json:"nsWithLogAgent"`
	NSWithoutLogAgent int   `json:"nsWithoutLogAgent"`
	EstActiveSeries   int64 `json:"estActiveSeries"`
}

type CardinalityRisk struct {
	Source     string `json:"source"`
	Namespace  string `json:"namespace"`
	RiskType   string `json:"riskType"`
	Severity   string `json:"severity"`
	Impact     string `json:"impact"`
	Suggestion string `json:"suggestion"`
}

type LogVolumeNS struct {
	Namespace   string `json:"namespace"`
	PodCount    int    `json:"podCount"`
	HasLogAgent bool   `json:"hasLogAgent"`
	EstVolumeMB int64  `json:"estVolumeMB"`
	Status      string `json:"status"`
}

type CollectorHealth struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Status    string `json:"status"`
}

// handleObsCardinality analyzes observability data cardinality and volume.
// GET /api/operations/obs-cardinality
func (s *Server) handleObsCardinality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ObsCardinalityResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Detect observability stack components
	collectorKeywords := map[string]string{
		"prometheus": "prometheus", "grafana-agent": "grafana-agent",
		"fluent-bit": "fluent-bit", "fluentbit": "fluent-bit",
		"loki": "loki", "promtail": "promtail",
		"jaeger": "jaeger", "otel": "otel-collector",
		"opentelemetry": "otel-collector", "vector": "vector",
		"datadog": "datadog", "telegraf": "telegraf",
		"victoria-metrics": "victoria-metrics", "vmagent": "victoria-metrics",
	}

	detectedCollectors := map[string]bool{}
	collectorPodCount := 0
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			nameLower := strings.ToLower(c.Name)
			for kw, collector := range collectorKeywords {
				if strings.Contains(imgLower, kw) || strings.Contains(nameLower, kw) {
					detectedCollectors[collector] = true
					if !systemNS[pod.Namespace] || strings.Contains(pod.Namespace, "monitor") || strings.Contains(pod.Namespace, "logging") {
						collectorPodCount++
					}
					break
				}
			}
		}
	}

	// Set stack detection
	result.Summary.HasPrometheus = detectedCollectors["prometheus"] || detectedCollectors["victoria-metrics"]
	result.Summary.HasFluentBit = detectedCollectors["fluent-bit"] || detectedCollectors["vector"] || detectedCollectors["promtail"]
	result.Summary.HasLoki = detectedCollectors["loki"]
	result.Summary.HasJaeger = detectedCollectors["jaeger"]
	result.Summary.HasOTelCollector = detectedCollectors["otel-collector"]
	result.Summary.CollectorPods = collectorPodCount

	// Check DaemonSet-based log agents per namespace
	nsHasLogAgent := map[string]bool{}
	for _, ds := range daemonsets.Items {
		dsNameLower := strings.ToLower(ds.Name)
		isLogAgent := false
		for kw := range collectorKeywords {
			if strings.Contains(dsNameLower, kw) || strings.Contains(strings.ToLower(ds.Namespace), "monitor") || strings.Contains(strings.ToLower(ds.Namespace), "logging") {
				isLogAgent = true
				break
			}
		}
		if isLogAgent && ds.Status.NumberReady > 0 {
			// DaemonSet runs on all nodes, so all namespaces are covered
			for _, ns := range namespaces.Items {
				if !systemNS[ns.Name] {
					nsHasLogAgent[ns.Name] = true
				}
			}
		}
	}

	// Analyze per-namespace
	totalPods := 0
	for _, ns := range namespaces.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNamespaces++

		nsPodCount := 0
		for _, pod := range pods.Items {
			if pod.Namespace == ns.Name && pod.Status.Phase == "Running" {
				nsPodCount++
				totalPods++
			}
		}

		hasAgent := nsHasLogAgent[ns.Name]
		if hasAgent {
			result.Summary.NSWithLogAgent++
		} else {
			result.Summary.NSWithoutLogAgent++
		}

		// Estimate log volume: ~10MB/day per pod
		estVol := int64(nsPodCount) * 10

		status := "covered"
		if !hasAgent {
			status = "no-agent"
			if nsPodCount > 5 {
				status = "blind-spot"
			}
		}

		result.LogVolumeByNS = append(result.LogVolumeByNS, LogVolumeNS{
			Namespace:   ns.Name,
			PodCount:    nsPodCount,
			HasLogAgent: hasAgent,
			EstVolumeMB: estVol,
			Status:      status,
		})

		// Cardinality risk: high-pod-count namespaces without metrics relabeling
		if nsPodCount > 10 {
			result.CardinalityRisks = append(result.CardinalityRisks, CardinalityRisk{
				Source:     ns.Name,
				Namespace:  ns.Name,
				RiskType:   "metric-cardinality-explosion",
				Severity:   "high",
				Impact:     fmt.Sprintf("%d pods × multiple containers = high label cardinality from pod_name/container_name", nsPodCount),
				Suggestion: "Use relabeling rules to drop pod_name labels and aggregate by deployment/workload",
			})
			result.Summary.HighCardLabels++
		} else if nsPodCount > 3 && !hasAgent {
			result.CardinalityRisks = append(result.CardinalityRisks, CardinalityRisk{
				Source:     ns.Name,
				Namespace:  ns.Name,
				RiskType:   "log-gap",
				Severity:   "medium",
				Impact:     fmt.Sprintf("%d pods with no log collection agent — blind to application logs", nsPodCount),
				Suggestion: "Deploy DaemonSet-based log collector (Fluent Bit/Vector) on all nodes",
			})
		}
	}

	// Check collector health
	for _, dep := range deployments.Items {
		depNameLower := strings.ToLower(dep.Name)
		for kw := range collectorKeywords {
			if strings.Contains(depNameLower, kw) || strings.Contains(strings.ToLower(dep.Namespace), "monitor") {
				ready := dep.Status.ReadyReplicas == dep.Status.Replicas && dep.Status.Replicas > 0
				collectorName := dep.Name
				status := "healthy"
				if !ready {
					status = "unhealthy"
					if dep.Status.Replicas == 0 {
						status = "stopped"
					}
				}
				result.CollectorHealth = append(result.CollectorHealth, CollectorHealth{
					Name: collectorName, Namespace: dep.Namespace,
					Kind: "Deployment", Ready: ready, Restarts: 0,
					Status: status,
				})
				break
			}
		}
	}
	for _, ds := range daemonsets.Items {
		dsNameLower := strings.ToLower(ds.Name)
		for kw := range collectorKeywords {
			if strings.Contains(dsNameLower, kw) {
				ready := ds.Status.NumberReady > 0
				status := "healthy"
				if !ready {
					status = "unhealthy"
				}
				result.CollectorHealth = append(result.CollectorHealth, CollectorHealth{
					Name: ds.Name, Namespace: ds.Namespace,
					Kind: "DaemonSet", Ready: ready, Restarts: 0,
					Status: status,
				})
				break
			}
		}
	}

	// Estimate active series: ~5000 series per pod (rough average)
	result.Summary.EstActiveSeries = int64(totalPods) * 5000

	// Estimate cost: $0.5 per 1M series/month + $0.02 per GB log/month
	seriesCost := float64(result.Summary.EstActiveSeries) / 1000000.0 * 50.0
	totalLogMB := int64(0)
	for _, lv := range result.LogVolumeByNS {
		totalLogMB += lv.EstVolumeMB
	}
	logCost := float64(totalLogMB) / 1024.0 * 2.0
	result.EstMonthlyCost = seriesCost + logCost

	// Risk score
	score := 100
	if !result.Summary.HasPrometheus {
		score -= 30
	}
	if !result.Summary.HasFluentBit {
		score -= 20
	}
	if result.Summary.NSWithoutLogAgent > 0 {
		score -= result.Summary.NSWithoutLogAgent * 2
	}
	for _, risk := range result.CardinalityRisks {
		if risk.Severity == "high" {
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	score = min(100, score)
	result.RiskScore = score
	result.Grade = goldenScoreToGrade(score)

	// Sort
	sort.Slice(result.LogVolumeByNS, func(i, j int) bool {
		return result.LogVolumeByNS[i].EstVolumeMB > result.LogVolumeByNS[j].EstVolumeMB
	})
	sort.Slice(result.CardinalityRisks, func(i, j int) bool {
		return result.CardinalityRisks[i].Severity > result.CardinalityRisks[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Observability cardinality risk: %d/100 (grade %s) — est. $%.2f/month data cost", score, result.Grade, result.EstMonthlyCost))
	if !result.Summary.HasPrometheus {
		recs = append(recs, "No Prometheus/metrics backend detected — deploy Victoria Metrics or Prometheus for metric collection")
	}
	if !result.Summary.HasFluentBit {
		recs = append(recs, "No log collection agent detected — deploy Fluent Bit or Vector DaemonSet for centralized logging")
	}
	if result.Summary.NSWithoutLogAgent > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d namespaces have no log collection — blind to application logs", result.Summary.NSWithoutLogAgent, result.Summary.TotalNamespaces))
	}
	if result.Summary.HighCardLabels > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces at risk of metric cardinality explosion — add relabeling rules to drop high-cardinality labels", result.Summary.HighCardLabels))
	}
	if len(recs) == 1 {
		recs = append(recs, "Observability stack is comprehensive with good cardinality management")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
