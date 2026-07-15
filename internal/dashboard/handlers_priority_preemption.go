package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PriPreemptionResult is the complete preemption & starvation risk report.
type PriPreemptionResult struct {
	Timestamp       time.Time            `json:"timestamp"`
	Score           int                  `json:"score"`
	Status          string               `json:"status"`
	Summary         PriPreemptionSummary `json:"summary"`
	PriorityClasses []PriClassInfo       `json:"priorityClasses"`
	PreemptionRisks []PriPreemptionRisk  `json:"preemptionRisks"`
	StarvationRisks []PriStarvationRisk  `json:"starvationRisks"`
	PendingPods     []PriPendingPod      `json:"pendingPods"`
	PriorityHeatmap []PriHeatmapEntry    `json:"priorityHeatmap"`
	Recommendations []string             `json:"recommendations"`
}

// PriPreemptionSummary holds aggregate counts.
type PriPreemptionSummary struct {
	TotalPriorityClasses int `json:"totalPriorityClasses"`
	PodsWithPriority     int `json:"podsWithPriority"`
	PodsWithoutPriority  int `json:"podsWithoutPriority"`
	HighPriorityPods     int `json:"highPriorityPods"`
	LowPriorityPods      int `json:"lowPriorityPods"`
	PendingPods          int `json:"pendingPods"`
	PreemptionRiskCount  int `json:"preemptionRiskCount"`
	StarvationRiskCount  int `json:"starvationRiskCount"`
	MaxPriorityValue     int `json:"maxPriorityValue"`
	MinPriorityValue     int `json:"minPriorityValue"`
}

// PriClassInfo describes a PriorityClass and its usage.
type PriClassInfo struct {
	Name             string `json:"name"`
	Value            int    `json:"value"`
	GlobalDefault    bool   `json:"globalDefault"`
	PreemptionPolicy string `json:"preemptionPolicy"`
	PodCount         int    `json:"podCount"`
	Description      string `json:"description"`
}

// PriPreemptionRisk identifies a pod vulnerable to preemption.
type PriPreemptionRisk struct {
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	Priority      int    `json:"priority"`
	PriorityClass string `json:"priorityClass"`
	NodeName      string `json:"nodeName"`
	Risk          string `json:"risk"`
	Impact        string `json:"impact"`
}

// PriStarvationRisk identifies a pending pod that may be starving.
type PriStarvationRisk struct {
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	Priority      int    `json:"priority"`
	PriorityClass string `json:"priorityClass"`
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	WaitingTime   string `json:"waitingTime"`
}

// PriPendingPod is a pending pod with priority context.
type PriPendingPod struct {
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	Priority      int    `json:"priority"`
	PriorityClass string `json:"priorityClass"`
}

// PriHeatmapEntry maps a priority range to pod count.
type PriHeatmapEntry struct {
	Range     string `json:"range"`
	PodCount  int    `json:"podCount"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handlePriorityPreemption(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	pcs, err := rc.clientset.SchedulingV1().PriorityClasses().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list priority classes: %v", err))
		return
	}

	result := analyzePriorityPreemption(pods.Items, pcs.Items)
	writeJSON(w, result)
}

func analyzePriorityPreemption(pods []corev1.Pod, pcs []schedulingv1.PriorityClass) PriPreemptionResult {
	now := time.Now()

	// Build priority class lookup
	pcMap := make(map[string]schedulingv1.PriorityClass)
	for _, pc := range pcs {
		pcMap[pc.Name] = pc
	}

	pcPodCount := make(map[string]int)
	summary := PriPreemptionSummary{
		TotalPriorityClasses: len(pcs),
	}

	var preemptionRisks []PriPreemptionRisk
	var starvationRisks []PriStarvationRisk
	var pendingPods []PriPendingPod
	var allPriorities []int
	minPri := 0

	for _, pod := range pods {
		var priority int
		if pod.Spec.Priority != nil {
			priority = int(*pod.Spec.Priority)
		}
		pcName := pod.Spec.PriorityClassName

		if pcName != "" {
			pcPodCount[pcName]++
			summary.PodsWithPriority++
		} else {
			summary.PodsWithoutPriority++
		}

		if priority > 1000000 {
			summary.HighPriorityPods++
		}
		if priority < 0 {
			summary.LowPriorityPods++
		}
		if priority > summary.MaxPriorityValue {
			summary.MaxPriorityValue = priority
		}
		if priority < minPri {
			minPri = priority
		}
		allPriorities = append(allPriorities, priority)

		// Pending pods: check for starvation
		if pod.Status.Phase == corev1.PodPending {
			pendingPods = append(pendingPods, PriPendingPod{
				Namespace:     pod.Namespace,
				Pod:           pod.Name,
				Priority:      priority,
				PriorityClass: pcName,
			})
			summary.PendingPods++

			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					waitDuration := now.Sub(cond.LastTransitionTime.Time)
					risk := PriStarvationRisk{
						Namespace:     pod.Namespace,
						Pod:           pod.Name,
						Priority:      priority,
						PriorityClass: pcName,
						Reason:        cond.Reason,
						Message:       cond.Message,
						WaitingTime:   formatDuration(waitDuration),
					}
					if priority < 1000 && waitDuration > 5*time.Minute {
						risk.Reason = "StarvationSuspected"
						starvationRisks = append(starvationRisks, risk)
						summary.StarvationRiskCount++
					} else if waitDuration > 10*time.Minute {
						starvationRisks = append(starvationRisks, risk)
						summary.StarvationRiskCount++
					}
				}
			}
		}

		// Preemption risk: pods with negative or very low priority that are running
		if pod.Status.Phase == corev1.PodRunning && priority < 0 {
			preemptionRisks = append(preemptionRisks, PriPreemptionRisk{
				Namespace:     pod.Namespace,
				Pod:           pod.Name,
				Priority:      priority,
				PriorityClass: pcName,
				NodeName:      pod.Spec.NodeName,
				Risk:          "LowPriorityPreemptible",
				Impact:        "Pod can be evicted by any higher-priority pod; workload disruption likely",
			})
			summary.PreemptionRiskCount++
		}

		// Pods with low priority that have PreemptLowerPriority
		if pc, ok := pcMap[pcName]; ok {
			policy := corev1.PreemptLowerPriority
			if pc.PreemptionPolicy != nil {
				policy = *pc.PreemptionPolicy
			}
			if policy == corev1.PreemptLowerPriority && priority < 100 && priority >= 0 {
				if pod.Status.Phase == corev1.PodRunning {
					preemptionRisks = append(preemptionRisks, PriPreemptionRisk{
						Namespace:     pod.Namespace,
						Pod:           pod.Name,
						Priority:      priority,
						PriorityClass: pcName,
						NodeName:      pod.Spec.NodeName,
						Risk:          "Preemptible",
						Impact:        fmt.Sprintf("Priority %d is very low; susceptible to preemption by higher-priority workloads", priority),
					})
					summary.PreemptionRiskCount++
				}
			}
		}
	}
	summary.MinPriorityValue = minPri

	// Build priority class info sorted by value descending
	var pcInfos []PriClassInfo
	for _, pc := range pcs {
		prePolicy := "PreemptLowerPriority"
		if pc.PreemptionPolicy != nil {
			prePolicy = string(*pc.PreemptionPolicy)
		}
		pcInfos = append(pcInfos, PriClassInfo{
			Name:             pc.Name,
			Value:            int(pc.Value),
			GlobalDefault:    pc.GlobalDefault,
			PreemptionPolicy: prePolicy,
			PodCount:         pcPodCount[pc.Name],
			Description:      pc.Description,
		})
	}
	sort.Slice(pcInfos, func(i, j int) bool {
		return pcInfos[i].Value > pcInfos[j].Value
	})

	heatmap := buildPriorityHeatmap(allPriorities)

	// Score
	score := 100
	score -= summary.PreemptionRiskCount * 3
	score -= summary.StarvationRiskCount * 5
	if summary.PodsWithoutPriority > 0 {
		ratio := float64(summary.PodsWithoutPriority) / float64(summary.PodsWithPriority+summary.PodsWithoutPriority)
		if ratio > 0.5 {
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	var recs []string
	if summary.PodsWithoutPriority > 0 {
		recs = append(recs, fmt.Sprintf("%d pods have no explicit priority class; consider assigning PriorityClass for predictable eviction behavior", summary.PodsWithoutPriority))
	}
	if summary.PreemptionRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pods have negative/low priority and are preemptible; ensure workloads can tolerate disruption", summary.PreemptionRiskCount))
	}
	if summary.StarvationRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pods appear to be starving due to low priority; consider increasing priority or adding node capacity", summary.StarvationRiskCount))
	}
	if summary.TotalPriorityClasses > 10 {
		recs = append(recs, "Large number of PriorityClasses detected; consolidate to simplify preemption behavior")
	}
	if len(recs) == 0 {
		recs = append(recs, "Priority class configuration looks healthy; preemption risk is minimal")
	}

	return PriPreemptionResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		PriorityClasses: pcInfos,
		PreemptionRisks: preemptionRisks,
		StarvationRisks: starvationRisks,
		PendingPods:     pendingPods,
		PriorityHeatmap: heatmap,
		Recommendations: recs,
	}
}

func buildPriorityHeatmap(priorities []int) []PriHeatmapEntry {
	buckets := map[string]int{
		"<0 (Preemptible)": 0,
		"0-999 (Low)":      0,
		"1K-99K (Normal)":  0,
		"100K-1M (High)":   0,
		">1M (System)":     0,
	}
	riskLevels := map[string]string{
		"<0 (Preemptible)": "high",
		"0-999 (Low)":      "medium",
		"1K-99K (Normal)":  "low",
		"100K-1M (High)":   "low",
		">1M (System)":     "info",
	}

	for _, p := range priorities {
		switch {
		case p < 0:
			buckets["<0 (Preemptible)"]++
		case p < 1000:
			buckets["0-999 (Low)"]++
		case p < 100000:
			buckets["1K-99K (Normal)"]++
		case p < 1000000:
			buckets["100K-1M (High)"]++
		default:
			buckets[">1M (System)"]++
		}
	}

	order := []string{"<0 (Preemptible)", "0-999 (Low)", "1K-99K (Normal)", "100K-1M (High)", ">1M (System)"}
	var entries []PriHeatmapEntry
	for _, k := range order {
		entries = append(entries, PriHeatmapEntry{
			Range:     k,
			PodCount:  buckets[k],
			RiskLevel: riskLevels[k],
		})
	}
	return entries
}
