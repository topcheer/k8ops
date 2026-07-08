package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPAResult is the HPA health & scaling analysis.
type HPAResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         HPASummary `json:"summary"`
	ByHPA           []HPAEntry `json:"byHPA"`
	AtMaxReplicas   []HPAEntry `json:"atMaxReplicas"`
	AtMinReplicas   []HPAEntry `json:"atMinReplicas"`
	NoMetrics       []HPAEntry `json:"noMetrics"`
	Issues          []HPAIssue `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// HPASummary aggregates HPA stats.
type HPASummary struct {
	TotalHPAs     int `json:"totalHPAs"`
	AtMaxReplicas int `json:"atMaxReplicas"`
	AtMinReplicas int `json:"atMinReplicas"`
	ScalingActive int `json:"scalingActive"`
	NoMetrics     int `json:"noMetrics"`
	AbleToScale   int `json:"ableToScale"`
	HealthScore   int `json:"healthScore"`
}

// HPAEntry describes one HPA's health.
type HPAEntry struct {
	Name            string         `json:"name"`
	Namespace       string         `json:"namespace"`
	TargetRef       string         `json:"targetRef"`
	MinReplicas     int32          `json:"minReplicas"`
	MaxReplicas     int32          `json:"maxReplicas"`
	CurrentReplicas int32          `json:"currentReplicas"`
	DesiredReplicas int32          `json:"desiredReplicas"`
	ScalingActive   bool           `json:"scalingActive"`
	MetricsCount    int            `json:"metricsCount"`
	Conditions      []HPACondition `json:"conditions,omitempty"`
	RiskLevel       string         `json:"riskLevel"`
}

// HPACondition describes one HPA condition.
type HPACondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// HPAIssue is a detected HPA problem.
type HPAIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleHPAHealth analyzes HPA health and scaling activity.
// GET /api/product/hpa-health
func (s *Server) handleHPAHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Try v2beta2 fallback
		writeK8sError(w, err)
		return
	}

	result := HPAResult{ScannedAt: time.Now()}

	for _, hpa := range hpas.Items {
		result.Summary.TotalHPAs++

		entry := HPAEntry{
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
			TargetRef: fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name),
		}

		if hpa.Spec.MinReplicas != nil {
			entry.MinReplicas = *hpa.Spec.MinReplicas
		}
		entry.MaxReplicas = hpa.Spec.MaxReplicas
		entry.CurrentReplicas = hpa.Status.CurrentReplicas
		entry.DesiredReplicas = hpa.Status.DesiredReplicas
		entry.MetricsCount = len(hpa.Spec.Metrics)

		// Parse conditions
		scalingActive := false
		ableToScale := false
		for _, cond := range hpa.Status.Conditions {
			ec := HPACondition{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			}
			entry.Conditions = append(entry.Conditions, ec)

			if cond.Type == autoscalingv2.ScalingActive && cond.Status == "True" {
				scalingActive = true
			}
			if cond.Type == autoscalingv2.AbleToScale && cond.Status == "True" {
				ableToScale = true
			}
		}
		entry.ScalingActive = scalingActive

		// Check if at max replicas
		if entry.CurrentReplicas >= entry.MaxReplicas && entry.MaxReplicas > 0 {
			result.Summary.AtMaxReplicas++
			result.AtMaxReplicas = append(result.AtMaxReplicas, entry)
			result.Issues = append(result.Issues, HPAIssue{
				Severity: "warning", Type: "at-max-replicas",
				Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
				Message:  fmt.Sprintf("HPA %s/%s is at max replicas (%d/%d) — consider increasing maxReplicas if traffic is still high", hpa.Namespace, hpa.Name, entry.CurrentReplicas, entry.MaxReplicas),
			})
		}

		// Check if at min replicas
		if entry.CurrentReplicas <= entry.MinReplicas && entry.MinReplicas > 0 {
			result.Summary.AtMinReplicas++
		}

		// Check for no metrics
		if entry.MetricsCount == 0 {
			result.Summary.NoMetrics++
			result.NoMetrics = append(result.NoMetrics, entry)
			result.Issues = append(result.Issues, HPAIssue{
				Severity: "warning", Type: "no-metrics",
				Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
				Message:  fmt.Sprintf("HPA %s/%s has no metrics configured — cannot auto-scale", hpa.Namespace, hpa.Name),
			})
		}

		// Check scaling disabled
		if !scalingActive && entry.CurrentReplicas > 0 {
			result.Issues = append(result.Issues, HPAIssue{
				Severity: "info", Type: "scaling-inactive",
				Resource: fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Name),
				Message:  fmt.Sprintf("HPA %s/%s scaling is not active — check metrics server and target resource", hpa.Namespace, hpa.Name),
			})
		}

		if scalingActive {
			result.Summary.ScalingActive++
		}
		if ableToScale {
			result.Summary.AbleToScale++
		}

		entry.RiskLevel = hpaAssessRisk(entry)
		result.ByHPA = append(result.ByHPA, entry)
	}

	// Sort
	sort.Slice(result.ByHPA, func(i, j int) bool {
		return hpaRiskRank(result.ByHPA[i].RiskLevel) < hpaRiskRank(result.ByHPA[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return hpaIssueRank(result.Issues[i].Severity) < hpaIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = hpaScore(result.Summary)
	result.Recommendations = hpaGenRecs(result.Summary, result.AtMaxReplicas, result.NoMetrics)

	writeJSON(w, result)
}

// hpaAssessRisk determines risk level.
func hpaAssessRisk(entry HPAEntry) string {
	if entry.MetricsCount == 0 {
		return "high"
	}
	if entry.CurrentReplicas >= entry.MaxReplicas && entry.MaxReplicas > 0 {
		return "medium"
	}
	if !entry.ScalingActive && entry.CurrentReplicas > 0 {
		return "medium"
	}
	return "low"
}

// hpaScore computes health score 0-100.
func hpaScore(s HPASummary) int {
	if s.TotalHPAs == 0 {
		return 100
	}
	score := 100
	score -= s.NoMetrics * 15
	score -= s.AtMaxReplicas * 5
	if score < 0 {
		score = 0
	}
	return score
}

// hpaGenRecs produces actionable advice.
func hpaGenRecs(s HPASummary, atMax []HPAEntry, noMetrics []HPAEntry) []string {
	var recs []string

	if s.NoMetrics > 0 {
		top := ""
		if len(noMetrics) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", noMetrics[0].Namespace, noMetrics[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d HPA(s) have no metrics configured%s — add CPU/memory resource requests or custom metrics", s.NoMetrics, top))
	}
	if s.AtMaxReplicas > 0 {
		recs = append(recs, fmt.Sprintf("%d HPA(s) are at maxReplicas — workload may be under-provisioned, increase maxReplicas or optimize resource usage", s.AtMaxReplicas))
	}
	if s.TotalHPAs-s.ScalingActive > 0 && s.TotalHPAs > 0 {
		inactive := s.TotalHPAs - s.ScalingActive
		recs = append(recs, fmt.Sprintf("%d HPA(s) have scaling inactive — verify metrics server is running and target resources have requests set", inactive))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("HPA health score is %d/100 — review autoscaling configuration", s.HealthScore))
	}
	if s.NoMetrics == 0 && s.AtMaxReplicas == 0 && s.TotalHPAs > 0 {
		recs = append(recs, fmt.Sprintf("All %d HPA(s) are healthy — good autoscaling posture", s.TotalHPAs))
	}

	return recs
}

func hpaRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func hpaIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
