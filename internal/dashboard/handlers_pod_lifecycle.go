package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LifecycleResult is the pod lifecycle stage analyzer & dwell-time tracker.
type LifecycleResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         LifecycleSummary    `json:"summary"`
	ByPhase         []PhaseStat         `json:"byPhase"`
	StuckPods       []StuckPod          `json:"stuckPods,omitempty"`
	DwellTime       DwellTimeStats      `json:"dwellTime"`
	ByWorkloadType  []WorkloadLifecycle `json:"byWorkloadType,omitempty"`
	ByNode          []NodeLifecycle     `json:"byNode,omitempty"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// LifecycleSummary aggregates lifecycle statistics.
type LifecycleSummary struct {
	TotalPods       int     `json:"totalPods"`
	Running         int     `json:"running"`
	Pending         int     `json:"pending"`
	Failed          int     `json:"failed"`
	Succeeded       int     `json:"succeeded"`
	Unknown         int     `json:"unknown"`
	Terminating     int     `json:"terminating"`
	StuckCount      int     `json:"stuckCount"`
	AvgPendingMin   float64 `json:"avgPendingMinutes"`
	MaxPendingMin   float64 `json:"maxPendingMinutes"`
	NewestPodAgeMin float64 `json:"newestPodAgeMin"`
	OldestPodAgeHr  float64 `json:"oldestPodAgeHr"`
}

// PhaseStat shows pod count and average age per phase.
type PhaseStat struct {
	Phase    string  `json:"phase"`
	Count    int     `json:"count"`
	AvgAgeHr float64 `json:"avgAgeHr"`
	MaxAgeHr float64 `json:"maxAgeHr"`
}

// StuckPod identifies a pod that is stuck in a non-Running state.
type StuckPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	StuckMin  int    `json:"stuckMinutes"`
	NodeName  string `json:"nodeName,omitempty"`
	Reason    string `json:"reason"`
	Workload  string `json:"workload,omitempty"`
	Severity  string `json:"severity"`
}

// DwellTimeStats shows time distribution for pods in each stage.
type DwellTimeStats struct {
	PendingP50     float64 `json:"pendingP50Min"`
	PendingP90     float64 `json:"pendingP90Min"`
	PendingP99     float64 `json:"pendingP99Min"`
	CreatingP50    float64 `json:"creatingP50Min"`
	CreatingP90    float64 `json:"creatingP90Min"`
	TerminatingP50 float64 `json:"terminatingP50Min"`
	TerminatingP90 float64 `json:"terminatingP90Min"`
}

// WorkloadLifecycle shows lifecycle stats per workload kind.
type WorkloadLifecycle struct {
	Kind       string  `json:"kind"`
	PodCount   int     `json:"podCount"`
	RunningPct float64 `json:"runningPct"`
	AvgAgeHr   float64 `json:"avgAgeHr"`
	RestartAvg float64 `json:"restartAvg"`
}

// NodeLifecycle shows pod lifecycle stats per node.
type NodeLifecycle struct {
	NodeName   string  `json:"nodeName"`
	PodCount   int     `json:"podCount"`
	RunningPct float64 `json:"runningPct"`
	AvgAgeHr   float64 `json:"avgAgeHr"`
}

// handlePodLifecycle analyzes pod lifecycle stages and dwell times.
// GET /api/operations/pod-lifecycle
func (s *Server) handlePodLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := LifecycleResult{ScannedAt: time.Now()}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// Phase tracking
	phaseCounts := map[corev1.PodPhase]int{}
	phaseAges := map[corev1.PodPhase][]float64{} // hours
	var pendingDurations []float64               // minutes
	var stuckPods []StuckPod
	var allAges []float64

	// Node tracking
	nodePodCount := map[string]int{}
	nodeRunning := map[string]int{}
	nodeAges := map[string][]float64{}

	// Workload tracking
	wlPodCount := map[string]int{}
	wlRunning := map[string]int{}
	wlAges := map[string][]float64{}
	wlRestarts := map[string][]float64{}

	var maxPendingMin, newestAgeMin, oldestAgeHr float64
	oldestAgeHr = 0
	newestAgeMin = 999999

	terminatingDurations := []float64{}
	creatingDurations := []float64{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		age := time.Since(pod.CreationTimestamp.Time)
		ageHr := age.Hours()
		ageMin := age.Minutes()
		allAges = append(allAges, ageHr)

		if ageHr > oldestAgeHr {
			oldestAgeHr = ageHr
		}
		if ageMin < newestAgeMin {
			newestAgeMin = ageMin
		}

		phase := pod.Status.Phase
		phaseCounts[phase]++
		phaseAges[phase] = append(phaseAges[phase], ageHr)

		// Check for terminating pods
		isTerminating := pod.DeletionTimestamp != nil
		if isTerminating {
			termDur := time.Since(pod.DeletionTimestamp.Time).Minutes()
			terminatingDurations = append(terminatingDurations, termDur)
			result.Summary.Terminating++
		}

		// Check for pending pods
		if phase == corev1.PodPending {
			pendingDurations = append(pendingDurations, ageMin)
			if ageMin > maxPendingMin {
				maxPendingMin = ageMin
			}

			// Determine if stuck
			stuckMin := int(ageMin)
			severity := "info"
			if ageMin > 30 {
				severity = "critical"
			} else if ageMin > 10 {
				severity = "warning"
			} else if ageMin > 5 {
				severity = "info"
			} else {
				continue // not stuck, just starting
			}

			reason := extractPendingReason(pod)
			wlName, _ := extractWorkloadFromPod(pod)
			stuckPods = append(stuckPods, StuckPod{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Phase:     "Pending",
				StuckMin:  stuckMin,
				NodeName:  pod.Spec.NodeName,
				Reason:    reason,
				Workload:  wlName,
				Severity:  severity,
			})
		}

		// Check for failed pods
		if phase == corev1.PodFailed {
			failedDur := ageMin
			if failedDur > 60 {
				reason := "Pod failed and not cleaned up"
				if len(pod.Status.ContainerStatuses) > 0 && pod.Status.ContainerStatuses[0].State.Terminated != nil {
					reason = fmt.Sprintf("Terminated: %s", pod.Status.ContainerStatuses[0].State.Terminated.Reason)
				}
				wlName, _ := extractWorkloadFromPod(pod)
				stuckPods = append(stuckPods, StuckPod{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Phase:     "Failed",
					StuckMin:  int(failedDur),
					Reason:    reason,
					Workload:  wlName,
					Severity:  "warning",
				})
			}
		}

		// Track container creating durations from conditions
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
				scheduledDur := cond.LastTransitionTime.Sub(pod.CreationTimestamp.Time).Minutes()
				if scheduledDur > 0 {
					creatingDurations = append(creatingDurations, scheduledDur)
				}
			}
		}

		// Node stats
		if pod.Spec.NodeName != "" {
			nodePodCount[pod.Spec.NodeName]++
			nodeAges[pod.Spec.NodeName] = append(nodeAges[pod.Spec.NodeName], ageHr)
			if phase == corev1.PodRunning && !isTerminating {
				nodeRunning[pod.Spec.NodeName]++
			}
		}

		// Workload stats
		_, wlKind := extractWorkloadFromPod(pod)
		if wlKind == "" {
			wlKind = "Pod"
		}
		wlPodCount[wlKind]++
		wlAges[wlKind] = append(wlAges[wlKind], ageHr)
		if phase == corev1.PodRunning && !isTerminating {
			wlRunning[wlKind]++
		}
		totalR := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalR += int(cs.RestartCount)
		}
		wlRestarts[wlKind] = append(wlRestarts[wlKind], float64(totalR))
	}

	// Build summary
	result.Summary.TotalPods = len(pods.Items)
	result.Summary.Running = phaseCounts[corev1.PodRunning]
	result.Summary.Pending = phaseCounts[corev1.PodPending]
	result.Summary.Failed = phaseCounts[corev1.PodFailed]
	result.Summary.Succeeded = phaseCounts[corev1.PodSucceeded]
	result.Summary.Unknown = phaseCounts[corev1.PodUnknown]
	result.Summary.StuckCount = len(stuckPods)
	result.Summary.MaxPendingMin = maxPendingMin
	result.Summary.NewestPodAgeMin = newestAgeMin
	result.Summary.OldestPodAgeHr = oldestAgeHr
	if len(pendingDurations) > 0 {
		sum := 0.0
		for _, d := range pendingDurations {
			sum += d
		}
		result.Summary.AvgPendingMin = sum / float64(len(pendingDurations))
	}

	// Build phase stats
	for phase, ages := range phaseAges {
		avgAge := avgFloat64(ages)
		maxAge := maxFloat64(ages)
		result.ByPhase = append(result.ByPhase, PhaseStat{
			Phase:    string(phase),
			Count:    phaseCounts[phase],
			AvgAgeHr: avgAge,
			MaxAgeHr: maxAge,
		})
	}
	sort.Slice(result.ByPhase, func(i, j int) bool {
		return result.ByPhase[i].Count > result.ByPhase[j].Count
	})

	// Sort stuck pods by duration descending
	sort.Slice(stuckPods, func(i, j int) bool {
		return stuckPods[i].StuckMin > stuckPods[j].StuckMin
	})
	if len(stuckPods) > 50 {
		stuckPods = stuckPods[:50]
	}
	result.StuckPods = stuckPods

	// Dwell time percentiles
	result.DwellTime = DwellTimeStats{
		PendingP50:     lifecyclePercentile(pendingDurations, 50),
		PendingP90:     lifecyclePercentile(pendingDurations, 90),
		PendingP99:     lifecyclePercentile(pendingDurations, 99),
		CreatingP50:    lifecyclePercentile(creatingDurations, 50),
		CreatingP90:    lifecyclePercentile(creatingDurations, 90),
		TerminatingP50: lifecyclePercentile(terminatingDurations, 50),
		TerminatingP90: lifecyclePercentile(terminatingDurations, 90),
	}

	// Workload type stats
	for kind, count := range wlPodCount {
		runningPct := 0.0
		if count > 0 {
			runningPct = float64(wlRunning[kind]) / float64(count) * 100
		}
		result.ByWorkloadType = append(result.ByWorkloadType, WorkloadLifecycle{
			Kind:       kind,
			PodCount:   count,
			RunningPct: runningPct,
			AvgAgeHr:   avgFloat64(wlAges[kind]),
			RestartAvg: avgFloat64(wlRestarts[kind]),
		})
	}
	sort.Slice(result.ByWorkloadType, func(i, j int) bool {
		return result.ByWorkloadType[i].PodCount > result.ByWorkloadType[j].PodCount
	})

	// Node stats - only show nodes with issues or top 20
	for nodeName, count := range nodePodCount {
		runningPct := 0.0
		if count > 0 {
			runningPct = float64(nodeRunning[nodeName]) / float64(count) * 100
		}
		result.ByNode = append(result.ByNode, NodeLifecycle{
			NodeName:   nodeName,
			PodCount:   count,
			RunningPct: runningPct,
			AvgAgeHr:   avgFloat64(nodeAges[nodeName]),
		})
	}
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].PodCount > result.ByNode[j].PodCount
	})
	if len(result.ByNode) > 20 {
		result.ByNode = result.ByNode[:20]
	}

	// Health score
	score := 100
	score -= result.Summary.Pending * 2
	score -= result.Summary.Failed * 3
	score -= result.Summary.StuckCount * 5
	if result.DwellTime.PendingP90 > 5 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	result.Recommendations = generateLifecycleRecs(result)

	writeJSON(w, result)
}

// extractPendingReason determines why a pod is pending.
func extractPendingReason(pod *corev1.Pod) string {
	// Check container statuses for waiting state
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			if cs.State.Waiting.Reason != "" {
				return fmt.Sprintf("%s: %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
			}
		}
	}
	// Check conditions
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			return fmt.Sprintf("Unscheduled: %s", cond.Message)
		}
	}
	if pod.Spec.NodeName == "" {
		return "No node assigned — may be unschedulable"
	}
	return "Unknown pending reason"
}

// generateLifecycleRecs produces actionable recommendations.
func generateLifecycleRecs(result LifecycleResult) []string {
	var recs []string

	if result.Summary.StuckCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) stuck in non-Running state — investigate pending reasons and failed containers", result.Summary.StuckCount))
	}

	if result.Summary.Pending > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) currently Pending — avg %.1f min, max %.1f min", result.Summary.Pending, result.Summary.AvgPendingMin, result.Summary.MaxPendingMin))
	}

	if result.DwellTime.PendingP90 > 5 {
		recs = append(recs, fmt.Sprintf("P90 pending time is %.1f min — check scheduler capacity and resource requests", result.DwellTime.PendingP90))
	}

	if result.DwellTime.TerminatingP90 > 2 {
		recs = append(recs, fmt.Sprintf("P90 termination time is %.1f min — check finalizers and graceful shutdown hooks", result.DwellTime.TerminatingP90))
	}

	if result.Summary.Failed > 0 {
		recs = append(recs, fmt.Sprintf("%d Failed pod(s) — clean up with job history limits or failed pod GC", result.Summary.Failed))
	}

	// Workload running percentage
	for _, wl := range result.ByWorkloadType {
		if wl.PodCount > 3 && wl.RunningPct < 80 {
			recs = append(recs, fmt.Sprintf("%s pods: only %.0f%% running (%d pods) — check for crash loops or scheduling issues", wl.Kind, wl.RunningPct, wl.PodCount))
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Pod lifecycle is healthy — all pods running, no stuck or failed pods detected")
	}

	return recs
}

// percentile calculates the p-th percentile of a slice.
func lifecyclePercentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

// avgFloat64 calculates the mean of a float64 slice.
func avgFloat64(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// maxFloat64 returns the maximum value.
func maxFloat64(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	m := data[0]
	for _, v := range data {
		if v > m {
			m = v
		}
	}
	return m
}
