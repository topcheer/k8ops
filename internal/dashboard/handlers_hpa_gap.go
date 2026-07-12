package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPAGapResult is the HPA target utilization gap & scaling behavior audit.
type HPAGapResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         HPAGapSummary   `json:"summary"`
	ByHPA           []HPAGapEntry   `json:"byHPA"`
	Issues          []HPAGapIssue   `json:"issues"`
	ByNamespace     []HPAGapNSEntry `json:"byNamespace"`
	Recommendations []string        `json:"recommendations"`
}

// HPAGapSummary aggregates HPA gap statistics.
type HPAGapSummary struct {
	TotalHPAs       int `json:"totalHPAs"`
	HPAsWithMetrics int `json:"hpasWithMetrics"`
	HPAsNoMetrics   int `json:"hpasNoMetrics"`
	TargetTooHigh   int `json:"targetTooHigh"`   // target > 90%
	TargetTooLow    int `json:"targetTooLow"`    // target < 30%
	NoScaleBehavior int `json:"noScaleBehavior"` // no scaleDown/stabilization
	MinEqualsMax    int `json:"minEqualsMax"`    // minReplicas == maxReplicas (no scaling room)
	HighGapHPAs     int `json:"highGapHPAs"`     // actual >> target (over-provisioned)
	HealthScore     int `json:"healthScore"`
}

// HPAGapEntry describes one HPA's configuration and gap.
type HPAGapEntry struct {
	Name             string  `json:"name"`
	Namespace        string  `json:"namespace"`
	TargetCPU        int32   `json:"targetCPU,omitempty"`
	TargetMemory     int32   `json:"targetMemory,omitempty"`
	CurrentCPU       int32   `json:"currentCPU,omitempty"`
	CurrentMemory    int32   `json:"currentMemory,omitempty"`
	CPUGap           float64 `json:"cpuGap,omitempty"`
	MemGap           float64 `json:"memGap,omitempty"`
	MinReplicas      int32   `json:"minReplicas"`
	MaxReplicas      int32   `json:"maxReplicas"`
	CurrentReplicas  int32   `json:"currentReplicas"`
	DesiredReplicas  int32   `json:"desiredReplicas"`
	HasScaleDown     bool    `json:"hasScaleDown"`
	HasStabilization bool    `json:"hasStabilization"`
	RiskLevel        string  `json:"riskLevel"`
}

// HPAGapIssue is a detected HPA issue.
type HPAGapIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// HPAGapNSEntry shows HPA stats per namespace.
type HPAGapNSEntry struct {
	Namespace  string `json:"namespace"`
	TotalHPAs  int    `json:"totalHPAs"`
	IssueCount int    `json:"issueCount"`
}

// hpaGapAuditCore performs the audit on HPA list and metrics (testable).
func hpaGapAuditCore(hpas []autoscalingv2.HorizontalPodAutoscaler) HPAGapResult {
	result := HPAGapResult{
		ScannedAt: time.Now(),
	}

	nsStats := make(map[string]*HPAGapNSEntry)

	for i := range hpas {
		hpa := &hpas[i]
		ns := hpa.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &HPAGapNSEntry{Namespace: ns}
		}
		nsStats[ns].TotalHPAs++
		result.Summary.TotalHPAs++

		entry := HPAGapEntry{
			Name:            hpa.Name,
			Namespace:       ns,
			MinReplicas:     defaultMinReplicas(hpa.Spec.MinReplicas),
			MaxReplicas:     hpa.Spec.MaxReplicas,
			CurrentReplicas: hpa.Status.CurrentReplicas,
			DesiredReplicas: hpa.Status.DesiredReplicas,
		}

		if hpa.Spec.Behavior != nil && hpa.Spec.Behavior.ScaleDown != nil {
			entry.HasScaleDown = true
			if hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds != nil {
				entry.HasStabilization = true
			}
		}

		issueCount := 0
		var issues []HPAGapIssue

		// Check each metric spec
		hasMetrics := false
		for _, metric := range hpa.Spec.Metrics {
			if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
				if metric.Resource.Target.AverageUtilization != nil {
					target := *metric.Resource.Target.AverageUtilization
					switch metric.Resource.Name {
					case corev1.ResourceCPU:
						entry.TargetCPU = target
					case corev1.ResourceMemory:
						entry.TargetMemory = target
					}

					// Check target range
					if target > 90 {
						result.Summary.TargetTooHigh++
						issues = append(issues, HPAGapIssue{
							Name: hpa.Name, Namespace: ns,
							Issue:    fmt.Sprintf("%s target utilization %d%% is too high (>90%%) — little headroom before throttling", metric.Resource.Name, target),
							Severity: "high",
						})
						issueCount++
					} else if target < 30 {
						result.Summary.TargetTooLow++
						issues = append(issues, HPAGapIssue{
							Name: hpa.Name, Namespace: ns,
							Issue:    fmt.Sprintf("%s target utilization %d%% is too low (<30%%) — over-provisioning and premature scaling", metric.Resource.Name, target),
							Severity: "medium",
						})
						issueCount++
					}
					hasMetrics = true
				}
			}
		}

		// Check current utilization from status
		for _, metric := range hpa.Status.CurrentMetrics {
			if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
				if metric.Resource.Current.AverageUtilization != nil {
					current := *metric.Resource.Current.AverageUtilization
					switch metric.Resource.Name {
					case corev1.ResourceCPU:
						entry.CurrentCPU = current
						if entry.TargetCPU > 0 {
							entry.CPUGap = float64(current-entry.TargetCPU) / float64(entry.TargetCPU) * 100
						}
					case corev1.ResourceMemory:
						entry.CurrentMemory = current
						if entry.TargetMemory > 0 {
							entry.MemGap = float64(current-entry.TargetMemory) / float64(entry.TargetMemory) * 100
						}
					}
				}
			}
		}

		if hasMetrics {
			result.Summary.HPAsWithMetrics++
		} else {
			result.Summary.HPAsNoMetrics++
			issues = append(issues, HPAGapIssue{
				Name: hpa.Name, Namespace: ns,
				Issue:    "no resource metrics configured — HPA cannot scale based on utilization",
				Severity: "high",
			})
			issueCount++
		}

		// Check min == max (no scaling room)
		if entry.MinReplicas == entry.MaxReplicas {
			result.Summary.MinEqualsMax++
			issues = append(issues, HPAGapIssue{
				Name: hpa.Name, Namespace: ns,
				Issue:    fmt.Sprintf("minReplicas == maxReplicas (%d) — HPA has no scaling room", entry.MinReplicas),
				Severity: "medium",
			})
			issueCount++
		}

		// Check no scale down behavior
		if !entry.HasScaleDown {
			result.Summary.NoScaleBehavior++
			issues = append(issues, HPAGapIssue{
				Name: hpa.Name, Namespace: ns,
				Issue:    "no scaleDown behavior configured — default cooldown may cause flapping",
				Severity: "low",
			})
			issueCount++
		}

		// Check high gap (current >> target)
		if entry.CPUGap > 50 || entry.MemGap > 50 {
			result.Summary.HighGapHPAs++
			issues = append(issues, HPAGapIssue{
				Name: hpa.Name, Namespace: ns,
				Issue:    fmt.Sprintf("current utilization significantly exceeds target (CPU gap: %.0f%%, Mem gap: %.0f%%) — HPA should have scaled out", entry.CPUGap, entry.MemGap),
				Severity: "high",
			})
			issueCount++
		}

		// Risk level
		entry.RiskLevel = "low"
		for _, iss := range issues {
			if iss.Severity == "high" {
				entry.RiskLevel = "high"
				break
			}
			if iss.Severity == "medium" && entry.RiskLevel == "low" {
				entry.RiskLevel = "medium"
			}
		}

		result.ByHPA = append(result.ByHPA, entry)
		result.Issues = append(result.Issues, issues...)
		nsStats[ns].IssueCount += issueCount
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].IssueCount > result.ByNamespace[j].IssueCount
	})

	sort.Slice(result.ByHPA, func(i, j int) bool {
		riskOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return riskOrder[result.ByHPA[i].RiskLevel] < riskOrder[result.ByHPA[j].RiskLevel]
	})

	result.Summary.HealthScore = hpaGapScore(result.Summary)
	result.Recommendations = hpaGapRecommendations(result.Summary)

	return result
}

// defaultMinReplicas returns *MinReplicas or 1 if nil.
func defaultMinReplicas(ptr *int32) int32 {
	if ptr == nil {
		return 1
	}
	return *ptr
}

// hpaGapScore calculates health score.
func hpaGapScore(s HPAGapSummary) int {
	if s.TotalHPAs == 0 {
		return 100
	}
	base := 100
	base -= s.NoScaleBehavior * 3
	base -= s.MinEqualsMax * 5
	base -= s.TargetTooHigh * 8
	base -= s.TargetTooLow * 4
	base -= s.HighGapHPAs * 10
	base -= s.HPAsNoMetrics * 6
	if base < 0 {
		base = 0
	}
	return base
}

// hpaGapRecommendations generates recommendations.
func hpaGapRecommendations(s HPAGapSummary) []string {
	var recs []string
	if s.HPAsNoMetrics > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have no resource metrics — add CPU/memory utilization targets", s.HPAsNoMetrics))
	}
	if s.TargetTooHigh > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have target utilization >90%% — lower to 70-80%% for headroom", s.TargetTooHigh))
	}
	if s.TargetTooLow > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have target utilization <30%% — raise to 50-70%% to avoid over-provisioning", s.TargetTooLow))
	}
	if s.MinEqualsMax > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have minReplicas == maxReplicas — increase maxReplicas to allow scaling", s.MinEqualsMax))
	}
	if s.NoScaleBehavior > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs lack scaleDown behavior — add stabilization window to prevent flapping", s.NoScaleBehavior))
	}
	if s.HighGapHPAs > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have current utilization far exceeding target — verify metrics-server is working and HPA can scale", s.HighGapHPAs))
	}
	if s.TargetTooHigh == 0 && s.TargetTooLow == 0 && s.HighGapHPAs == 0 && s.MinEqualsMax == 0 {
		recs = append(recs, "HPA configurations are well-tuned — no significant issues detected")
	}
	return recs
}

// handleHPAGap audits HPA target utilization gap & scaling behavior.
// GET /api/product/hpa-gap
func (s *Server) handleHPAGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := hpaGapAuditCore(hpas.Items)
	writeJSON(w, result)
}
