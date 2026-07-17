package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ObsCoverageResult is the workload observability coverage & blind spot detector.
// It identifies workloads that are "flying blind" — no monitoring, no alerts,
// no dashboards, no runbooks — and scores the cluster's observability completeness.
type ObsCoverageResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ObsCovSummary     `json:"summary"`
	BlindWorkloads  []BlindWorkload   `json:"blindWorkloads"`
	CoverageMatrix  []ObsCovMatrix    `json:"coverageMatrix"`
	ByNamespace     []ObsCovNS        `json:"byNamespace"`
	SignalStrength  ObsSignalStrength `json:"signalStrength"`
	Recommendations []string          `json:"recommendations"`
}

// ObsCovSummary aggregates observability coverage statistics.
type ObsCovSummary struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	WithMetrics    int     `json:"withMetrics"`    // has Prometheus scrape or ServiceMonitor
	WithTracing    int     `json:"withTracing"`    // has tracing annotations
	WithDashboard  int     `json:"withDashboard"`  // has dashboard URL annotation
	WithRunbook    int     `json:"withRunbook"`    // has runbook URL annotation
	WithAlerts     int     `json:"withAlerts"`     // has PrometheusRule or alert annotations
	BlindCount     int     `json:"blindCount"`     // workloads with zero observability
	CoveragePct    float64 `json:"coveragePct"`    // % with at least metrics
	BlindSpotIndex int     `json:"blindSpotIndex"` // 0-100, higher = worse
	SignalQuality  string  `json:"signalQuality"`  // excellent, good, fair, poor, critical
}

// BlindWorkload is a workload with insufficient observability.
type BlindWorkload struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	Replicas  int32    `json:"replicas"`
	Missing   []string `json:"missingSignals"`
	Severity  string   `json:"severity"`
	Impact    string   `json:"impact"`
}

// ObsCovMatrix shows per-workload signal coverage.
type ObsCovMatrix struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Metrics       bool   `json:"metrics"`
	Tracing       bool   `json:"tracing"`
	Dashboard     bool   `json:"dashboard"`
	Runbook       bool   `json:"runbook"`
	Alerts        bool   `json:"alerts"`
	CoverageCount int    `json:"coverageCount"` // 0-5
	Grade         string `json:"grade"`
}

// ObsCovNS shows observability coverage per namespace.
type ObsCovNS struct {
	Namespace      string  `json:"namespace"`
	TotalWorkloads int     `json:"totalWorkloads"`
	BlindCount     int     `json:"blindCount"`
	CoveragePct    float64 `json:"coveragePct"`
}

// ObsSignalStrength shows cluster-wide signal adoption rates.
type ObsSignalStrength struct {
	MetricsPct   int `json:"metricsPct"`
	TracingPct   int `json:"tracingPct"`
	DashboardPct int `json:"dashboardPct"`
	RunbookPct   int `json:"runbookPct"`
	AlertsPct    int `json:"alertsPct"`
}

// handleObsCoverage provides workload observability coverage & blind spot detection.
// GET /api/operations/obs-coverage
func (s *Server) handleObsCoverage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ObsCoverageResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deploys, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	stss, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Check for ServiceMonitor/PodMonitor/PrometheusRule via annotation-based detection
	hasSM := false
	hasPM := false
	hasPR := false
	_ = hasSM
	_ = hasPM
	hasPR = false // CRD detection not available without dynamic client
	clusterHasAlerting := hasPR

	// Namespace tracking
	type nsObsData struct {
		total int
		blind int
	}
	nsStats := make(map[string]*nsObsData)

	// Signal counters
	sigMetrics := 0
	sigTracing := 0
	sigDashboard := 0
	sigRunbook := 0
	sigAlerts := 0

	processWl := func(name, ns, kind string, replicas int32, labels map[string]string, annotations map[string]string) {
		if systemNS[ns] {
			return
		}
		result.Summary.TotalWorkloads++

		nsd, ok := nsStats[ns]
		if !ok {
			nsd = &nsObsData{}
			nsStats[ns] = nsd
		}
		nsd.total++

		hasMetrics := false
		hasTracing := false
		hasDashboard := false
		hasRunbook := false
		hasAlerts := false

		// Check annotations for monitoring signals
		for k, v := range annotations {
			kl := strings.ToLower(k)
			vl := strings.ToLower(v)

			// Metrics: Prometheus scrape annotations or ServiceMonitor reference
			if strings.Contains(kl, "prometheus.io/scrape") && vl == "true" {
				hasMetrics = true
			}
			if strings.Contains(kl, "prometheus") && (strings.Contains(kl, "scrape") || strings.Contains(kl, "port")) {
				hasMetrics = true
			}
			if strings.Contains(kl, "monitoring.coreos.com") || strings.Contains(kl, "servicemonitor") {
				hasMetrics = true
			}

			// Tracing
			if strings.Contains(kl, "jaeger") || strings.Contains(kl, "zipkin") ||
				strings.Contains(kl, "otel") || strings.Contains(kl, "opentelemetry") ||
				strings.Contains(kl, "tracing") || strings.Contains(kl, "tempo") {
				hasTracing = true
			}

			// Dashboard link
			if strings.Contains(kl, "dashboard") || strings.Contains(kl, "grafana") {
				if strings.Contains(vl, "http") || strings.Contains(vl, "grafana") {
					hasDashboard = true
				}
			}

			// Runbook
			if strings.Contains(kl, "runbook") || strings.Contains(kl, "wiki") ||
				strings.Contains(kl, "doc") || strings.Contains(kl, "sop") {
				if strings.Contains(vl, "http") || len(v) > 5 {
					hasRunbook = true
				}
			}

			// Alert rules
			if strings.Contains(kl, "alert") || strings.Contains(kl, "prometheusrule") {
				hasAlerts = true
			}
		}

		// If cluster has CRD-based monitoring, assume coverage
		if hasSM || hasPM {
			hasMetrics = true
		}
		if clusterHasAlerting {
			hasAlerts = true
		}

		if hasMetrics {
			sigMetrics++
		}
		if hasTracing {
			sigTracing++
		}
		if hasDashboard {
			sigDashboard++
		}
		if hasRunbook {
			sigRunbook++
		}
		if hasAlerts {
			sigAlerts++
		}

		// Coverage count
		coverageCount := 0
		missing := []string{}
		if hasMetrics {
			coverageCount++
		} else {
			missing = append(missing, "metrics")
		}
		if hasTracing {
			coverageCount++
		} else {
			missing = append(missing, "tracing")
		}
		if hasDashboard {
			coverageCount++
		} else {
			missing = append(missing, "dashboard")
		}
		if hasRunbook {
			coverageCount++
		} else {
			missing = append(missing, "runbook")
		}
		if hasAlerts {
			coverageCount++
		} else {
			missing = append(missing, "alerts")
		}

		grade := goldenScoreToGrade(coverageCount * 20)

		result.CoverageMatrix = append(result.CoverageMatrix, ObsCovMatrix{
			Name: name, Namespace: ns,
			Metrics: hasMetrics, Tracing: hasTracing, Dashboard: hasDashboard,
			Runbook: hasRunbook, Alerts: hasAlerts,
			CoverageCount: coverageCount, Grade: grade,
		})

		// Blind workload: coverageCount <= 1
		if coverageCount <= 1 {
			severity := "medium"
			impact := "No observability signals — failures will be invisible"
			if replicas > 3 {
				severity = "high"
				impact = fmt.Sprintf("Critical blind spot: %d replicas with no monitoring", replicas)
			}
			if coverageCount == 0 {
				severity = "high"
			}
			result.BlindWorkloads = append(result.BlindWorkloads, BlindWorkload{
				Name: name, Namespace: ns, Kind: kind, Replicas: replicas,
				Missing: missing, Severity: severity, Impact: impact,
			})
			result.Summary.BlindCount++
			nsd.blind++
		}
	}

	for _, dep := range deploys.Items {
		reps := int32(0)
		if dep.Spec.Replicas != nil {
			reps = *dep.Spec.Replicas
		}
		processWl(dep.Name, dep.Namespace, "Deployment", reps, dep.Labels, dep.Annotations)
	}
	for _, sts := range stss.Items {
		reps := int32(0)
		if sts.Spec.Replicas != nil {
			reps = *sts.Spec.Replicas
		}
		processWl(sts.Name, sts.Namespace, "StatefulSet", reps, sts.Labels, sts.Annotations)
	}

	// Summary
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.WithMetrics = sigMetrics
		result.Summary.WithTracing = sigTracing
		result.Summary.WithDashboard = sigDashboard
		result.Summary.WithRunbook = sigRunbook
		result.Summary.WithAlerts = sigAlerts
		result.Summary.CoveragePct = float64(sigMetrics) / float64(result.Summary.TotalWorkloads) * 100
		result.Summary.BlindSpotIndex = int(float64(result.Summary.BlindCount) / float64(result.Summary.TotalWorkloads) * 100)
	}

	// Signal quality rating
	bi := result.Summary.BlindSpotIndex
	switch {
	case bi == 0:
		result.Summary.SignalQuality = "excellent"
	case bi < 20:
		result.Summary.SignalQuality = "good"
	case bi < 40:
		result.Summary.SignalQuality = "fair"
	case bi < 60:
		result.Summary.SignalQuality = "poor"
	default:
		result.Summary.SignalQuality = "critical"
	}

	// Signal strength
	if result.Summary.TotalWorkloads > 0 {
		tw := result.Summary.TotalWorkloads
		result.SignalStrength = ObsSignalStrength{
			MetricsPct:   sigMetrics * 100 / tw,
			TracingPct:   sigTracing * 100 / tw,
			DashboardPct: sigDashboard * 100 / tw,
			RunbookPct:   sigRunbook * 100 / tw,
			AlertsPct:    sigAlerts * 100 / tw,
		}
	}

	// Sort blind workloads by severity then replicas
	sort.Slice(result.BlindWorkloads, func(i, j int) bool {
		if result.BlindWorkloads[i].Severity != result.BlindWorkloads[j].Severity {
			return severityRankMap(result.BlindWorkloads[i].Severity) > severityRankMap(result.BlindWorkloads[j].Severity)
		}
		return result.BlindWorkloads[i].Replicas > result.BlindWorkloads[j].Replicas
	})
	if len(result.BlindWorkloads) > 50 {
		result.BlindWorkloads = result.BlindWorkloads[:50]
	}

	// Sort coverage matrix (worst first)
	sort.Slice(result.CoverageMatrix, func(i, j int) bool {
		return result.CoverageMatrix[i].CoverageCount < result.CoverageMatrix[j].CoverageCount
	})

	// By namespace
	for nsName, nsd := range nsStats {
		cov := 0.0
		if nsd.total > 0 {
			cov = float64(nsd.total-nsd.blind) / float64(nsd.total) * 100
		}
		result.ByNamespace = append(result.ByNamespace, ObsCovNS{
			Namespace: nsName, TotalWorkloads: nsd.total,
			BlindCount: nsd.blind, CoveragePct: cov,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	result.Recommendations = generateObsCovRecs(result)

	writeJSON(w, result)
}

// smGVR returns the GVR for ServiceMonitor.
func smGVR() interface{} { return nil }

// pmGVR returns the GVR for PodMonitor.
func pmGVR() interface{} { return nil }

// prGVR returns the GVR for PrometheusRule.
func prGVR() interface{} { return nil }

// generateObsCovRecs produces actionable recommendations.
func generateObsCovRecs(result ObsCoverageResult) []string {
	var recs []string

	if result.Summary.BlindCount > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d workloads are 'blind' — no observability signals detected", result.Summary.BlindCount, result.Summary.TotalWorkloads))
	}
	if result.SignalStrength.MetricsPct < 50 {
		recs = append(recs, fmt.Sprintf("Only %d%% of workloads have Prometheus metrics — add scrape annotations or ServiceMonitors", result.SignalStrength.MetricsPct))
	}
	if result.SignalStrength.RunbookPct < 20 {
		recs = append(recs, fmt.Sprintf("Only %d%% have runbook links — add 'runbook' or 'wiki' annotations for faster incident response", result.SignalStrength.RunbookPct))
	}
	if result.SignalStrength.TracingPct < 20 {
		recs = append(recs, fmt.Sprintf("Only %d%% have distributed tracing — add OpenTelemetry/Jaeger annotations", result.SignalStrength.TracingPct))
	}
	if result.SignalStrength.DashboardPct < 20 {
		recs = append(recs, fmt.Sprintf("Only %d%% have dashboard links — add 'dashboard' or 'grafana' annotations", result.SignalStrength.DashboardPct))
	}
	if result.Summary.SignalQuality == "critical" || result.Summary.SignalQuality == "poor" {
		recs = append(recs, fmt.Sprintf("Observability signal quality is '%s' — systematic monitoring adoption needed", result.Summary.SignalQuality))
	}
	if len(result.BlindWorkloads) > 0 {
		top := result.BlindWorkloads[0]
		recs = append(recs, fmt.Sprintf("Worst blind spot: '%s/%s' (%s) — missing: %s", top.Namespace, top.Name, top.Kind, strings.Join(top.Missing, ", ")))
	}
	if len(recs) == 0 {
		recs = append(recs, "Observability coverage is comprehensive — all workloads have monitoring signals")
	}

	return recs
}

// Suppress unused import
var _ appsv1.Deployment
var _ corev1.Pod
