package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScalingHistoryResult is the cluster scaling history & autoscaler event timeline audit.
type ScalingHistoryResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         ScalingHistorySummary `json:"summary"`
	Events          []ScalingEventEntry   `json:"events"`
	ByAction        []ScalingActionStat   `json:"byAction"`
	ByNamespace     []ScalingNSStat       `json:"byNamespace"`
	Timeline        []TimelineEntry       `json:"timeline"`
	Risks           []ScalingHistoryRisk  `json:"risks"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// ScalingHistorySummary aggregates scaling history metrics.
type ScalingHistorySummary struct {
	TotalEvents     int `json:"totalEvents"`
	ScaleUpEvents   int `json:"scaleUpEvents"`
	ScaleDownEvents int `json:"scaleDownEvents"`
	HPAEvents       int `json:"hpaEvents"`
	NodeEvents      int `json:"nodeEvents"`
	CAScaleUp       int `json:"caScaleUp"`    // cluster autoscaler scale-up
	CAScaleDown     int `json:"caScaleDown"`  // cluster autoscaler scale-down
	FailedScales    int `json:"failedScales"` // scaling operations that failed
	Last24h         int `json:"last24h"`
	Last1h          int `json:"last1h"`
	Throttled       int `json:"throttled"` // pods that were throttled during scaling
}

// ScalingEventEntry describes a scaling event.
type ScalingEventEntry struct {
	Time      string `json:"time"`
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource"` // e.g. "Deployment/myapp"
	Action    string `json:"action"`   // scale-up, scale-down, hpa-scale-up, etc.
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Severity  string `json:"severity"` // info, warning, error
}

// ScalingActionStat per-action stats.
type ScalingActionStat struct {
	Action string `json:"action"`
	Count  int    `json:"count"`
	Failed int    `json:"failed"`
}

// ScalingNSStat per-namespace scaling stats.
type ScalingNSStat struct {
	Namespace string `json:"namespace"`
	ScaleUp   int    `json:"scaleUp"`
	ScaleDown int    `json:"scaleDown"`
	Failed    int    `json:"failed"`
}

// TimelineEntry time-bucketed scaling activity.
type TimelineEntry struct {
	Hour      string `json:"hour"`
	ScaleUp   int    `json:"scaleUp"`
	ScaleDown int    `json:"scaleDown"`
	Total     int    `json:"total"`
}

// ScalingHistoryRisk describes a scaling-related risk.
type ScalingHistoryRisk struct {
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleScalingHistory audits cluster scaling history & autoscaler event timeline.
// GET /api/scalability/scaling-history
func (s *Server) handleScalingHistory(w http.ResponseWriter, r *http.Request) {
	result := ScalingHistoryResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get events from all namespaces
	events, err := rc.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list events: %v", err))
		return
	}

	now := time.Now()
	actionStats := map[string]*ScalingActionStat{}
	nsStats := map[string]*ScalingNSStat{}
	timelineMap := map[string]*TimelineEntry{}

	// Scaling-related event keywords
	scaleUpKeywords := []string{"scaled up", "scale up", "scaledup", "increased replicas", "successfully resized"}
	scaleDownKeywords := []string{"scaled down", "scale down", "scaleddown", "decreased replicas", "downscaled"}
	hpaKeywords := []string{"hpa", "horizontal pod autoscaler", "autoscaler"}
	caKeywords := []string{"cluster autoscaler", "scale up", "node group", "machine group", "node added", "node removed"}
	failedKeywords := []string{"failed", "error", "backoff", "unable to scale", "throttl"}

	for _, evt := range events.Items {
		msgLower := strings.ToLower(evt.Message)
		reasonLower := strings.ToLower(evt.Reason)
		combined := msgLower + " " + reasonLower

		// Check if this is a scaling event
		isScaleUp := false
		isScaleDown := false
		isHPA := false
		isCA := false

		for _, kw := range scaleUpKeywords {
			if strings.Contains(combined, kw) {
				isScaleUp = true
				break
			}
		}
		for _, kw := range scaleDownKeywords {
			if strings.Contains(combined, kw) {
				isScaleDown = true
				break
			}
		}
		for _, kw := range hpaKeywords {
			if strings.Contains(combined, kw) {
				isHPA = true
				break
			}
		}
		for _, kw := range caKeywords {
			if strings.Contains(combined, kw) {
				isCA = true
				break
			}
		}

		if !isScaleUp && !isScaleDown && !isHPA && !isCA {
			continue
		}

		// Skip non-scaling HPA events
		if isHPA && !isScaleUp && !isScaleDown {
			continue
		}

		// Determine action
		action := "scale-up"
		if isScaleDown {
			action = "scale-down"
		}
		if isHPA {
			action = "hpa-" + action
		}
		if isCA {
			action = "ca-" + action
			result.Summary.NodeEvents++
			if isScaleUp {
				result.Summary.CAScaleUp++
			} else {
				result.Summary.CAScaleDown++
			}
		}
		if isHPA {
			result.Summary.HPAEvents++
		}

		// Check for failed scaling
		isFailed := false
		severity := "info"
		for _, kw := range failedKeywords {
			if strings.Contains(combined, kw) {
				isFailed = true
				severity = "warning"
				result.Summary.FailedScales++
				break
			}
		}

		// Throttled detection
		if strings.Contains(combined, "throttl") {
			result.Summary.Throttled++
			severity = "warning"
		}

		result.Summary.TotalEvents++
		if isScaleUp {
			result.Summary.ScaleUpEvents++
		} else if isScaleDown {
			result.Summary.ScaleDownEvents++
		}

		// Time-based stats
		ts := evt.LastTimestamp.Time
		if ts.IsZero() {
			ts = evt.CreationTimestamp.Time
		}
		age := now.Sub(ts)
		if age <= 24*time.Hour {
			result.Summary.Last24h++
		}
		if age <= 1*time.Hour {
			result.Summary.Last1h++
		}

		// Build event entry
		ns := evt.Namespace
		resource := fmt.Sprintf("%s/%s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name)

		entry := ScalingEventEntry{
			Time:      ts.Format(time.RFC3339),
			Namespace: ns,
			Resource:  resource,
			Action:    action,
			Reason:    evt.Reason,
			Message:   evt.Message,
			Severity:  severity,
		}
		result.Events = append(result.Events, entry)

		// Action stats
		if actionStats[action] == nil {
			actionStats[action] = &ScalingActionStat{Action: action}
		}
		actionStats[action].Count++
		if isFailed {
			actionStats[action].Failed++
		}

		// Namespace stats
		if nsStats[ns] == nil {
			nsStats[ns] = &ScalingNSStat{Namespace: ns}
		}
		if isScaleUp {
			nsStats[ns].ScaleUp++
		} else if isScaleDown {
			nsStats[ns].ScaleDown++
		}
		if isFailed {
			nsStats[ns].Failed++
		}

		// Timeline (hourly buckets)
		hour := ts.Truncate(time.Hour).Format("15:04")
		if timelineMap[hour] == nil {
			timelineMap[hour] = &TimelineEntry{Hour: hour}
		}
		if isScaleUp {
			timelineMap[hour].ScaleUp++
		} else if isScaleDown {
			timelineMap[hour].ScaleDown++
		}
		timelineMap[hour].Total++
	}

	// 2. Build action stats slice
	for _, stat := range actionStats {
		result.ByAction = append(result.ByAction, *stat)
	}
	sort.Slice(result.ByAction, func(i, j int) bool {
		return result.ByAction[i].Count > result.ByAction[j].Count
	})

	// 3. Build namespace stats slice
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ScaleUp+result.ByNamespace[i].ScaleDown > result.ByNamespace[j].ScaleUp+result.ByNamespace[j].ScaleDown
	})

	// 4. Build timeline
	for _, entry := range timelineMap {
		result.Timeline = append(result.Timeline, *entry)
	}
	sort.Slice(result.Timeline, func(i, j int) bool {
		return result.Timeline[i].Hour < result.Timeline[j].Hour
	})

	// 5. Sort events by time (most recent first)
	sort.Slice(result.Events, func(i, j int) bool {
		return result.Events[i].Time > result.Events[j].Time
	})

	// 6. Calculate health score
	score := 100
	if result.Summary.FailedScales > 0 {
		score -= min(30, result.Summary.FailedScales*10)
	}
	if result.Summary.Throttled > 0 {
		score -= min(20, result.Summary.Throttled*5)
	}
	if result.Summary.ScaleUpEvents > 0 && result.Summary.ScaleDownEvents == 0 {
		// Only scaling up, never down — possible over-provisioning
		result.Risks = append(result.Risks, ScalingHistoryRisk{
			Issue:    "Cluster only scaling up, never down — possible over-provisioning or scale-down disabled",
			Severity: "low",
		})
	}
	if result.Summary.ScaleDownEvents > result.Summary.ScaleUpEvents*3 && result.Summary.ScaleUpEvents > 0 {
		result.Risks = append(result.Risks, ScalingHistoryRisk{
			Issue:    "Scale-down events significantly exceed scale-up — possible workload instability",
			Severity: "medium",
		})
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	if result.Summary.FailedScales > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d scaling operation(s) failed — check resource limits, scheduling constraints, and node capacity", result.Summary.FailedScales))
	}
	if result.Summary.Throttled > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d throttled scaling event(s) — increase resource requests or add more nodes", result.Summary.Throttled))
	}
	if result.Summary.TotalEvents == 0 {
		result.Recommendations = append(result.Recommendations,
			"No recent scaling events detected — cluster may be statically sized or autoscaling is not configured")
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Scaling operations are functioning normally — no failed or throttled events detected")
	}

	writeJSON(w, result)
}
