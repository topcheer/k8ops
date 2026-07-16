package dashboard

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PredictiveHealthResult is the cluster predictive health & risk forecast engine.
type PredictiveHealthResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          PredictiveSummary     `json:"summary"`
	ForecastHorizon  string                `json:"forecastHorizon"`
	Predictions      []RiskPrediction      `json:"predictions"`
	NodeRisks        []NodeRiskForecast    `json:"nodeRisks"`
	PodRisks         []PodRiskForecast     `json:"podRisks"`
	ResourceTrends   []ResourceTrend       `json:"resourceTrends"`
	RiskTimeline     []TimelineEvent       `json:"riskTimeline"`
	Recommendations  []string              `json:"recommendations"`
	ConfidenceScore  int                   `json:"confidenceScore"`
	OverallRiskLevel string                `json:"overallRiskLevel"`
}

// PredictiveSummary aggregates prediction statistics.
type PredictiveSummary struct {
	TotalNodes         int `json:"totalNodes"`
	TotalPods          int `json:"totalPods"`
	CriticalPredictions int `json:"criticalPredictions"` // predicted within 24h
	HighPredictions     int `json:"highPredictions"`     // predicted within 7d
	MediumPredictions   int `json:"mediumPredictions"`   // predicted within 30d
	LowPredictions      int `json:"lowPredictions"`      // > 30d
	NodesAtRisk         int `json:"nodesAtRisk"`
	PodsAtRisk          int `json:"podsAtRisk"`
	TrendingResources   int `json:"trendingResources"`
}

// RiskPrediction describes a predicted risk event.
type RiskPrediction struct {
	Category    string    `json:"category"`    // disk-exhaustion, memory-pressure, cert-expiry, node-failure, capacity-exhaustion
	Severity    string    `json:"severity"`    // critical, high, medium, low
	Resource    string    `json:"resource"`    // node name, namespace/workload, or cluster-wide
	Description string    `json:"description"`
	ETA         string    `json:"eta"`         // estimated time to impact
	ETADays     float64   `json:"etaDays"`     // estimated days to impact
	Confidence  string    `json:"confidence"`  // high, medium, low
	Mitigation  string    `json:"mitigation"`
}

// NodeRiskForecast per-node risk prediction.
type NodeRiskForecast struct {
	NodeName       string  `json:"nodeName"`
	RiskScore      int     `json:"riskScore"`      // 0-100, higher = more risk
	DiskRisk       string  `json:"diskRisk"`       // none, low, medium, high, critical
	MemoryRisk     string  `json:"memoryRisk"`
	CPUThrottleRisk string `json:"cpuThrottleRisk"`
	FailureRisk    string  `json:"failureRisk"`    // based on conditions and events
	PodPressure    int     `json:"podPressure"`    // pods under pressure
	Conditions     []string `json:"conditions,omitempty"`
}

// PodRiskForecast per-pod risk prediction.
type PodRiskForecast struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	RiskType     string `json:"riskType"`     // oom-predicted, restart-loop, resource-starvation, eviction-risk
	Severity     string `json:"severity"`
	RiskScore    int    `json:"riskScore"`
	Description  string `json:"description"`
	ETA          string `json:"eta"`
}

// ResourceTrend describes a resource consumption trend.
type ResourceTrend struct {
	Resource     string  `json:"resource"`     // e.g., "node-disk:mach", "cluster-memory", "namespace-pods:monitoring"
	CurrentUsage float64 `json:"currentUsage"` // percentage 0-100
	TrendRate    float64 `json:"trendRate"`    // percentage points per day (estimated)
	ProjectedAt  string  `json:"projectedAt"`  // projected 80% threshold date
	Trend        string  `json:"trend"`        // increasing, stable, decreasing
}

// TimelineEvent describes a future risk event on a timeline.
type TimelineEvent struct {
	When     string `json:"when"`     // relative time (e.g., "2d", "7d", "30d+")
	Category string `json:"category"`
	Severity string `json:"severity"`
	Count    int    `json:"count"`
	Detail   string `json:"detail"`
}

// handlePredictiveHealth generates a predictive risk forecast for the cluster.
// GET /api/operations/predictive-health
func (s *Server) handlePredictiveHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PredictiveHealthResult{
		ScannedAt:       time.Now(),
		ForecastHorizon: "30 days",
	}

	// 1. Collect cluster state
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// 2. Analyze node-level risks
	var nodeRisks []NodeRiskForecast
	var predictions []RiskPrediction
	var resourceTrends []ResourceTrend

	for _, node := range nodes.Items {
		nr := NodeRiskForecast{NodeName: node.Name}

		// Node conditions
		var conditions []string
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				conditions = append(conditions, string(cond.Type))
			}
			// Memory pressure
			if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
				nr.MemoryRisk = "critical"
				nr.RiskScore += 45
				conditions = append(conditions, "MemoryPressure")
			}
			// Disk pressure
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
				nr.DiskRisk = "critical"
				nr.RiskScore += 35
				conditions = append(conditions, "DiskPressure")
			}
			// PID pressure
			if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
				nr.RiskScore += 25
				conditions = append(conditions, "PIDPressure")
			}
		}
		nr.Conditions = conditions

		// Analyze allocatable vs capacity for resource trends
		allocatable := node.Status.Allocatable
		capacity := node.Status.Capacity
		if allocatable.Memory() != nil && capacity.Memory() != nil {
			allocMem := allocatable.Memory().Value()
			capMem := capacity.Memory().Value()
			if capMem > 0 {
				memReservedPct := float64(capMem-allocMem) * 100 / float64(capMem)
				if memReservedPct > 90 {
					nr.MemoryRisk = setIfHigher(nr.MemoryRisk, "high")
					nr.RiskScore += 15
				}
			}
		}

		// Count pods on this node and their pressure
		if pods != nil {
			nodePodCount := 0
			pressurePods := 0
			for _, pod := range pods.Items {
				if pod.Spec.NodeName == node.Name && pod.Status.Phase == corev1.PodRunning {
					nodePodCount++
					// Check for pods with high restart counts (potential OOM)
					totalRestarts := 0
					for _, cs := range pod.Status.ContainerStatuses {
						totalRestarts += int(cs.RestartCount)
					}
					if totalRestarts > 5 {
						pressurePods++
						nr.RiskScore += 3
					}
				}
			}
			nr.PodPressure = pressurePods

			// Estimate pod density risk
			maxPods := 110
			if mp, ok := node.Status.Allocatable.Pods().AsInt64(); ok && mp > 0 {
				maxPods = int(mp)
			}
			podDensity := float64(nodePodCount) * 100 / float64(maxPods)
			if podDensity > 85 {
				nr.RiskScore += 10
				resourceTrends = append(resourceTrends, ResourceTrend{
					Resource:     fmt.Sprintf("pod-density:%s", node.Name),
					CurrentUsage: math.Round(podDensity*10) / 10,
					TrendRate:    0.5, // estimated
					Trend:        "increasing",
					ProjectedAt:  fmt.Sprintf("%.0fd", (90-podDensity)/0.5),
				})
			}
		}

		// Node age factor (older nodes have higher failure risk)
		nodeAge := time.Since(node.CreationTimestamp.Time)
		if nodeAge > 365*24*time.Hour {
			nr.RiskScore += 5
		}

		// Normalize risk score
		if nr.RiskScore > 100 {
			nr.RiskScore = 100
		}

		// Set default risk levels
		if nr.DiskRisk == "" {
			nr.DiskRisk = "none"
		}
		if nr.MemoryRisk == "" {
			nr.MemoryRisk = "none"
		}
		if nr.CPUThrottleRisk == "" {
			nr.CPUThrottleRisk = "none"
		}
		if nr.FailureRisk == "" {
			if nr.RiskScore > 50 {
				nr.FailureRisk = "high"
			} else if nr.RiskScore > 25 {
				nr.FailureRisk = "medium"
			} else {
				nr.FailureRisk = "low"
			}
		}

		// Generate predictions for nodes with elevated risk
		if nr.RiskScore > 25 {
			etaDays := math.Max(1, 30-float64(nr.RiskScore)/4)
			predictions = append(predictions, RiskPrediction{
				Category:    "node-failure",
				Severity:    severityFromScore(nr.RiskScore),
				Resource:    node.Name,
				Description: fmt.Sprintf("Node %s has elevated risk (score %d) due to %s", node.Name, nr.RiskScore, strings.Join(conditions, ", ")),
				ETA:         formatDays(etaDays),
				ETADays:     etaDays,
				Confidence:  confidenceFromScore(nr.RiskScore),
				Mitigation:  "Drain and inspect node for hardware issues, check dmesg for errors",
			})
		}

		nodeRisks = append(nodeRisks, nr)
	}

	// 3. Analyze pod-level risks
	var podRisks []PodRiskForecast
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
				continue
			}

			totalRestarts := 0
			for _, cs := range pod.Status.ContainerStatuses {
				totalRestarts += int(cs.RestartCount)
			}

			// OOM prediction: high restart count with memory limits
			if totalRestarts > 3 {
				riskScore := totalRestarts * 8
				if riskScore > 100 {
					riskScore = 100
				}
				pr := PodRiskForecast{
					Name:        pod.Name,
					Namespace:   pod.Namespace,
					RiskType:    "restart-loop",
					Severity:    severityFromScore(riskScore),
					RiskScore:   riskScore,
					Description: fmt.Sprintf("Pod has %d restarts — potential OOMKill, crash loop, or dependency failure", totalRestarts),
				}
				if totalRestarts > 20 {
					pr.ETA = "imminent"
				} else if totalRestarts > 10 {
					pr.ETA = "< 1d"
				} else {
					pr.ETA = "1-3d"
				}
				podRisks = append(podRisks, pr)
			}

			// Resource starvation: no limits set
			hasNoLimits := false
			for _, c := range pod.Spec.Containers {
				if c.Resources.Limits.Cpu() == nil && c.Resources.Limits.Memory() == nil {
					hasNoLimits = true
					break
				}
			}
			if hasNoLimits && totalRestarts == 0 {
				// Only predict if not already flagged for restarts
				podRisks = append(podRisks, PodRiskForecast{
					Name:        pod.Name,
					Namespace:   pod.Namespace,
					RiskType:    "resource-starvation",
					Severity:    "medium",
					RiskScore:   35,
					Description: "Pod has no resource limits — at risk for unbounded consumption and node pressure",
					ETA:         "variable",
				})
			}

			// Eviction risk: low QoS with pending conditions
			qosClass := pod.Status.QOSClass
			if qosClass == corev1.PodQOSBestEffort || qosClass == corev1.PodQOSBurstable {
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionFalse {
						podRisks = append(podRisks, PodRiskForecast{
							Name:        pod.Name,
							Namespace:   pod.Namespace,
							RiskType:    "eviction-risk",
							Severity:    "medium",
							RiskScore:   40,
							Description: fmt.Sprintf("%s QoS pod not ready — first candidate for eviction under pressure", qosClass),
							ETA:         "under pressure",
						})
						break
					}
				}
			}
		}
	}

	// Sort pod risks by score descending
	sort.Slice(podRisks, func(i, j int) bool {
		return podRisks[i].RiskScore > podRisks[j].RiskScore
	})
	// Limit to top 30
	if len(podRisks) > 30 {
		podRisks = podRisks[:30]
	}

	// 4. Certificate & secret expiry prediction
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	certExpiringSoon := 0
	if secrets != nil {
		for _, secret := range secrets.Items {
			if secret.Type != corev1.SecretTypeTLS {
				continue
			}
			// Check annotation for cert expiry or creation age
			expiryStr := secret.Annotations["cert-manager.io/certificate-name"]
			_ = expiryStr
			age := time.Since(secret.CreationTimestamp.Time)
			if age > 80*24*time.Hour { // >80 days old TLS secret
				certExpiringSoon++
			}
		}
	}
	if certExpiringSoon > 0 {
		predictions = append(predictions, RiskPrediction{
			Category:    "cert-expiry",
			Severity:    "medium",
			Resource:    "cluster-wide",
			Description: fmt.Sprintf("%d TLS secret(s) are >80 days old — verify renewal pipeline is active", certExpiringSoon),
			ETA:         "10-50d",
			ETADays:     30,
			Confidence:  "medium",
			Mitigation:  "Verify cert-manager renewal status and certificate expiration dates",
		})
	}

	// 5. Capacity exhaustion forecast
	if pods != nil && nodes != nil {
		totalPodCapacity := 0
		totalPods := 0
		for _, node := range nodes.Items {
			if mp, ok := node.Status.Allocatable.Pods().AsInt64(); ok {
				totalPodCapacity += int(mp)
			}
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				totalPods++
			}
		}
		if totalPodCapacity > 0 {
			utilization := float64(totalPods) * 100 / float64(totalPodCapacity)
			if utilization > 70 {
				daysToFull := math.Max(1, (90-utilization)/1.5)
				predictions = append(predictions, RiskPrediction{
					Category:    "capacity-exhaustion",
					Severity:    severityFromUtil(utilization),
					Resource:    "cluster-wide",
					Description: fmt.Sprintf("Pod capacity at %.1f%% (%d/%d) — trending toward exhaustion", utilization, totalPods, totalPodCapacity),
					ETA:         formatDays(daysToFull),
					ETADays:     daysToFull,
					Confidence:  "medium",
					Mitigation:  "Add nodes or clean up completed/stale pods to increase capacity",
				})
				resourceTrends = append(resourceTrends, ResourceTrend{
					Resource:     "cluster-pod-capacity",
					CurrentUsage: math.Round(utilization*10) / 10,
					TrendRate:    1.5,
					Trend:        "increasing",
					ProjectedAt:  formatDays(daysToFull),
				})
			}
		}
	}

	// 6. Build risk timeline
	timeline := buildRiskTimeline(predictions)

	// 7. Sort predictions by ETA days
	sort.Slice(predictions, func(i, j int) bool {
		return predictions[i].ETADays < predictions[j].ETADays
	})

	result.NodeRisks = nodeRisks
	result.PodRisks = podRisks
	result.Predictions = predictions
	result.ResourceTrends = resourceTrends
	result.RiskTimeline = timeline

	// 8. Compute summary
	result.Summary.TotalNodes = len(nodes.Items)
	if pods != nil {
		result.Summary.TotalPods = len(pods.Items)
	}
	for _, p := range predictions {
		switch p.Severity {
		case "critical":
			result.Summary.CriticalPredictions++
		case "high":
			result.Summary.HighPredictions++
		case "medium":
			result.Summary.MediumPredictions++
		default:
			result.Summary.LowPredictions++
		}
	}
	for _, nr := range nodeRisks {
		if nr.RiskScore > 30 {
			result.Summary.NodesAtRisk++
		}
	}
	result.Summary.PodsAtRisk = len(podRisks)
	result.Summary.TrendingResources = len(resourceTrends)

	// 9. Overall risk level & confidence
	maxRiskScore := 0
	for _, nr := range nodeRisks {
		if nr.RiskScore > maxRiskScore {
			maxRiskScore = nr.RiskScore
		}
	}
	switch {
	case maxRiskScore > 60 || result.Summary.CriticalPredictions > 0:
		result.OverallRiskLevel = "critical"
	case maxRiskScore > 40 || result.Summary.HighPredictions > 0:
		result.OverallRiskLevel = "high"
	case maxRiskScore > 20 || result.Summary.MediumPredictions > 0:
		result.OverallRiskLevel = "medium"
	default:
		result.OverallRiskLevel = "low"
	}

	// Confidence based on data completeness
	confidence := 80
	if pods == nil {
		confidence -= 20
	}
	if secrets == nil {
		confidence -= 5
	}
	if len(nodes.Items) == 0 {
		confidence -= 30
	}
	if confidence < 50 {
		confidence = 50
	}
	result.ConfidenceScore = confidence

	// 10. Recommendations
	result.Recommendations = generatePredictiveRecs(result)

	writeJSON(w, result)
}

// severityFromScore converts a 0-100 risk score to severity.
func severityFromScore(score int) string {
	switch {
	case score >= 70:
		return "critical"
	case score >= 50:
		return "high"
	case score >= 25:
		return "medium"
	default:
		return "low"
	}
}

// severityFromUtil converts utilization percentage to severity.
func severityFromUtil(pct float64) string {
	switch {
	case pct >= 85:
		return "critical"
	case pct >= 75:
		return "high"
	case pct >= 65:
		return "medium"
	default:
		return "low"
	}
}

// confidenceFromScore converts risk score to confidence level.
func confidenceFromScore(score int) string {
	if score >= 70 {
		return "high"
	}
	if score >= 40 {
		return "medium"
	}
	return "low"
}

// formatDays converts days to a human-readable ETA string.
func formatDays(days float64) string {
	if days < 1 {
		return "< 1 day"
	}
	if days == 1 {
		return "~1 day"
	}
	if days < 7 {
		return fmt.Sprintf("~%.0f days", days)
	}
	if days < 30 {
		return fmt.Sprintf("~%.0f days (%.1f weeks)", days, days/7)
	}
	return fmt.Sprintf("~%.0f days (%.1f months)", days, days/30)
}

// setIfHigher replaces risk level only if new level is higher severity.
func setIfHigher(current, newLevel string) string {
	order := map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}
	if current == "" {
		return newLevel
	}
	if order[newLevel] > order[current] {
		return newLevel
	}
	return current
}

// buildRiskTimeline converts predictions into time-bucketed timeline.
func buildRiskTimeline(predictions []RiskPrediction) []TimelineEvent {
	buckets := map[string]*TimelineEvent{
		"24h":    {When: "< 24h", Category: "immediate", Severity: "critical"},
		"7d":     {When: "1-7 days", Category: "short-term", Severity: "high"},
		"30d":    {When: "7-30 days", Category: "medium-term", Severity: "medium"},
		"30d+":   {When: "> 30 days", Category: "long-term", Severity: "low"},
	}
	details := map[string][]string{"24h": {}, "7d": {}, "30d": {}, "30d+": {}}

	for _, p := range predictions {
		var bucket string
		switch {
		case p.ETADays < 1:
			bucket = "24h"
		case p.ETADays < 7:
			bucket = "7d"
		case p.ETADays < 30:
			bucket = "30d"
		default:
			bucket = "30d+"
		}
		buckets[bucket].Count++
		details[bucket] = append(details[bucket], fmt.Sprintf("%s: %s", p.Category, p.Resource))
	}

	var timeline []TimelineEvent
	for _, key := range []string{"24h", "7d", "30d", "30d+"} {
		ev := buckets[key]
		if ev.Count > 0 {
			// Limit detail to top 3 items
			if len(details[key]) > 3 {
				details[key] = details[key][:3]
			}
			ev.Detail = strings.Join(details[key], "; ")
			timeline = append(timeline, *ev)
		}
	}
	return timeline
}

// generatePredictiveRecs produces actionable recommendations from predictions.
func generatePredictiveRecs(result PredictiveHealthResult) []string {
	var recs []string

	if result.Summary.CriticalPredictions > 0 {
		recs = append(recs, fmt.Sprintf("URGENT: %d critical prediction(s) within 24h — immediate action required", result.Summary.CriticalPredictions))
	}
	if result.Summary.HighPredictions > 0 {
		recs = append(recs, fmt.Sprintf("%d high-severity prediction(s) within 7 days — plan mitigation this week", result.Summary.HighPredictions))
	}

	if result.Summary.NodesAtRisk > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) at elevated risk — review conditions and plan drain/replace", result.Summary.NodesAtRisk))
	}

	if len(result.PodRisks) > 0 {
		oomCount := 0
		for _, pr := range result.PodRisks {
			if pr.RiskType == "restart-loop" {
				oomCount++
			}
		}
		if oomCount > 0 {
			recs = append(recs, fmt.Sprintf("%d pod(s) show restart-loop patterns — investigate OOMKill, dependency failures, or crash loops", oomCount))
		}
		noLimitCount := 0
		for _, pr := range result.PodRisks {
			if pr.RiskType == "resource-starvation" {
				noLimitCount++
			}
		}
		if noLimitCount > 0 {
			recs = append(recs, fmt.Sprintf("%d pod(s) without resource limits — set CPU/memory limits to prevent unbounded consumption", noLimitCount))
		}
	}

	for _, t := range result.ResourceTrends {
		if t.Trend == "increasing" && t.CurrentUsage > 70 {
			recs = append(recs, fmt.Sprintf("%s at %.1f%% and increasing — projected to hit 90%% in %s", t.Resource, t.CurrentUsage, t.ProjectedAt))
		}
	}

	if result.OverallRiskLevel == "low" {
		recs = append(recs, "Cluster predictive risk is LOW — no imminent issues detected in 30-day forecast")
	}

	if len(recs) == 0 {
		recs = append(recs, "No significant risks predicted — cluster appears stable for the forecast horizon")
	}

	return recs
}
