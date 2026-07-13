package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPAPerformanceResult is the HPA autoscaling performance & scaling event analysis.
type HPAPerformanceResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         HPAPerfSummary  `json:"summary"`
	HPAs            []HPAPerfEntry  `json:"hpas"`
	ByNamespace     []HPAPerfNSStat `json:"byNamespace"`
	Issues          []HPAPerfIssue  `json:"issues"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// HPAPerfSummary aggregates HPA performance statistics.
type HPAPerfSummary struct {
	TotalHPAs       int `json:"totalHPAs"`
	WithMetrics     int `json:"withMetrics"`
	NoMetrics       int `json:"noMetrics"`
	ScalingActive   int `json:"scalingActive"`
	ScalingLimited  int `json:"scalingLimited"`
	DesiredReplicas int `json:"desiredReplicas"`
	CurrentReplicas int `json:"currentReplicas"`
	Underutilized   int `json:"underutilized"`
	Overutilized    int `json:"overutilized"`
	NoMinReplicas   int `json:"noMinReplicas"`
	NoMaxReplicas   int `json:"noMaxReplicas"`
}

// HPAPerfEntry describes one HPA's performance.
type HPAPerfEntry struct {
	Name             string     `json:"name"`
	Namespace        string     `json:"namespace"`
	TargetRef        string     `json:"targetRef"`
	MinReplicas      int        `json:"minReplicas"`
	MaxReplicas      int        `json:"maxReplicas"`
	CurrentReplicas  int        `json:"currentReplicas"`
	DesiredReplicas  int        `json:"desiredReplicas"`
	CurrentMetric    string     `json:"currentMetric,omitempty"`
	TargetMetric     string     `json:"targetMetric,omitempty"`
	UtilizationRatio float64    `json:"utilizationRatio,omitempty"`
	ScalingActive    bool       `json:"scalingActive"`
	ScalingLimited   bool       `json:"scalingLimited"`
	LastScaleTime    *time.Time `json:"lastScaleTime,omitempty"`
	RiskLevel        string     `json:"riskLevel"`
}

// HPAPerfNSStat per-namespace HPA stats.
type HPAPerfNSStat struct {
	Namespace     string `json:"namespace"`
	HPACount      int    `json:"hpaCount"`
	ScalingActive int    `json:"scalingActive"`
}

// HPAPerfIssue is a detected HPA performance problem.
type HPAPerfIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleHPAPerformance audits HPA autoscaling performance and scaling events.
// GET /api/scalability/hpa-performance
func (s *Server) handleHPAPerformance(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &HPAPerformanceResult{
		ScannedAt: time.Now(),
	}

	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var entries []HPAPerfEntry
	var issues []HPAPerfIssue
	nsStats := make(map[string]*HPAPerfNSStat)

	noMetrics := 0
	scalingActive := 0
	scalingLimited := 0
	underutilized := 0
	overutilized := 0
	noMinRepl := 0
	noMaxRepl := 0

	for i := range hpas.Items {
		hpa := &hpas.Items[i]
		if isSystemNamespace(hpa.Namespace) {
			continue
		}

		entry := HPAPerfEntry{
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
			TargetRef: fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name),
		}

		// Min/max replicas
		minRepl := 1
		if hpa.Spec.MinReplicas != nil {
			minRepl = int(*hpa.Spec.MinReplicas)
		} else {
			noMinRepl++
		}
		entry.MinReplicas = minRepl

		if hpa.Spec.MaxReplicas > 0 {
			entry.MaxReplicas = int(hpa.Spec.MaxReplicas)
		} else {
			noMaxRepl++
			entry.MaxReplicas = 0
		}

		// Current and desired replicas
		entry.CurrentReplicas = int(hpa.Status.CurrentReplicas)
		entry.DesiredReplicas = int(hpa.Status.DesiredReplicas)

		// Check metrics
		if len(hpa.Status.CurrentMetrics) > 0 {
			metric := hpa.Status.CurrentMetrics[0]
			if metric.Resource != nil && metric.Resource.Current.AverageUtilization != nil {
				entry.CurrentMetric = fmt.Sprintf("%d%%", *metric.Resource.Current.AverageUtilization)
			}
		} else {
			noMetrics++
			issues = append(issues, HPAPerfIssue{
				Severity: "warning",
				Type:     "no-metrics",
				Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
				Message:  "HPA has no current metrics — autoscaling cannot function. Check metrics-server and HPA metric configuration",
			})
		}

		// Target utilization
		if len(hpa.Spec.Metrics) > 0 {
			specMetric := hpa.Spec.Metrics[0]
			if specMetric.Resource != nil && specMetric.Resource.Target.AverageUtilization != nil {
				entry.TargetMetric = fmt.Sprintf("%d%%", *specMetric.Resource.Target.AverageUtilization)

				// Calculate utilization ratio
				if len(hpa.Status.CurrentMetrics) > 0 {
					curMetric := hpa.Status.CurrentMetrics[0]
					if curMetric.Resource != nil && curMetric.Resource.Current.AverageUtilization != nil {
						target := float64(*specMetric.Resource.Target.AverageUtilization)
						if target > 0 {
							entry.UtilizationRatio = float64(*curMetric.Resource.Current.AverageUtilization) / target
							if entry.UtilizationRatio < 0.3 {
								underutilized++
							} else if entry.UtilizationRatio > 1.5 {
								overutilized++
								issues = append(issues, HPAPerfIssue{
									Severity: "warning",
									Type:     "overutilized",
									Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
									Message:  fmt.Sprintf("HPA utilization ratio %.2f — current usage significantly exceeds target, scaling may be insufficient", entry.UtilizationRatio),
								})
							}
						}
					}
				}
			}
		}

		// Scaling conditions
		for _, cond := range hpa.Status.Conditions {
			if cond.Type == autoscalingv2.ScalingActive && cond.Status == corev1.ConditionTrue {
				entry.ScalingActive = true
				scalingActive++
			}
			if cond.Type == autoscalingv2.ScalingLimited && cond.Status == corev1.ConditionTrue {
				entry.ScalingLimited = true
				scalingLimited++
				issues = append(issues, HPAPerfIssue{
					Severity: "warning",
					Type:     "scaling-limited",
					Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
					Message:  fmt.Sprintf("HPA scaling is limited — desired replicas (%d) cannot be reached. Check maxReplicas or resource constraints", entry.DesiredReplicas),
				})
			}
		}

		// Last scale time
		if hpa.Status.LastScaleTime != nil {
			t := hpa.Status.LastScaleTime.Time
			entry.LastScaleTime = &t

			// Check if HPA hasn't scaled in a long time
			sinceLastScale := time.Since(t)
			if sinceLastScale > 7*24*time.Hour && entry.CurrentReplicas == entry.DesiredReplicas {
				issues = append(issues, HPAPerfIssue{
					Severity: "info",
					Type:     "stale-hpa",
					Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
					Message:  fmt.Sprintf("HPA hasn't scaled in %d days — verify metrics and target are configured correctly", int(sinceLastScale.Hours()/24)),
				})
			}
		}

		// Max replicas = min replicas (no scaling room)
		if entry.MaxReplicas > 0 && entry.MaxReplicas == minRepl {
			issues = append(issues, HPAPerfIssue{
				Severity: "warning",
				Type:     "no-scaling-room",
				Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
				Message:  fmt.Sprintf("maxReplicas (%d) equals minReplicas (%d) — HPA has no scaling room", entry.MaxReplicas, minRepl),
			})
		}

		entry.RiskLevel = assessHPAPerfRisk(entry)
		entries = append(entries, entry)

		// Update namespace stats
		if _, ok := nsStats[hpa.Namespace]; !ok {
			nsStats[hpa.Namespace] = &HPAPerfNSStat{Namespace: hpa.Namespace}
		}
		nsStats[hpa.Namespace].HPACount++
		if entry.ScalingActive {
			nsStats[hpa.Namespace].ScalingActive++
		}
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].HPACount > result.ByNamespace[j].HPACount
	})

	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if noMetrics > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d HPA(s) have no current metrics — ensure metrics-server is running and HPA resource metrics are configured", noMetrics))
	}
	if scalingLimited > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d HPA(s) are scaling-limited — increase maxReplicas or address resource constraints", scalingLimited))
	}
	if overutilized > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d HPA(s) are overutilized — consider adjusting target utilization or increasing maxReplicas", overutilized))
	}
	if noMaxRepl > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d HPA(s) have maxReplicas=0 — set a reasonable maxReplicas to allow scaling", noMaxRepl))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "All HPAs are performing well — autoscaling is active and metrics are healthy")
	}

	result.HPAs = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = HPAPerfSummary{
		TotalHPAs:      len(entries),
		WithMetrics:    len(entries) - noMetrics,
		NoMetrics:      noMetrics,
		ScalingActive:  scalingActive,
		ScalingLimited: scalingLimited,
		Underutilized:  underutilized,
		Overutilized:   overutilized,
		NoMinReplicas:  noMinRepl,
		NoMaxReplicas:  noMaxRepl,
	}
	result.HealthScore = computeHPAPerfScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessHPAPerfRisk determines risk level of an HPA.
func assessHPAPerfRisk(entry HPAPerfEntry) string {
	risk := 0
	if !entry.ScalingActive {
		risk += 2
	}
	if entry.ScalingLimited {
		risk += 2
	}
	if entry.MaxReplicas > 0 && entry.MaxReplicas == entry.MinReplicas {
		risk += 2
	}
	if entry.UtilizationRatio > 1.5 {
		risk += 1
	}
	switch {
	case risk >= 4:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computeHPAPerfScore computes a 0-100 health score.
func computeHPAPerfScore(s HPAPerfSummary, issueCount int) int {
	if s.TotalHPAs == 0 {
		return 100
	}
	score := 100
	score -= s.NoMetrics * 10
	score -= s.ScalingLimited * 5
	score -= s.Overutilized * 3
	score -= s.NoMaxReplicas * 3
	score -= s.NoMinReplicas * 1
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
