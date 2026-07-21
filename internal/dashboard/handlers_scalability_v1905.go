package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.05 — Scalability & HA Dimension (Round 3)
// 1. HPA Effectiveness Analyzer
// 2. Pod Scheduling Latency
// 3. Capacity Headroom Calculator
// ============================================================

// ---------------------------------------------------------------
// 1. HPA Effectiveness Analyzer
// ---------------------------------------------------------------

type HPAEffResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	HealthScore     int           `json:"healthScore"`
	Grade           string        `json:"grade"`
	Summary         HPAEffSummary `json:"summary"`
	ByWorkload      []HPAEffEntry `json:"byWorkload"`
	Issues          []HPAEffEntry `json:"issues"`
	Recommendations []string      `json:"recommendations"`
}

type HPAEffSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithHPA          int `json:"withHPA"`
	WithoutHPA       int `json:"withoutHPA"`
	HPAScalingActive int `json:"hpaScalingActive"`
	HPAAtMax         int `json:"hpaAtMax"`
	HPAAtMin         int `json:"hpaAtMin"`
	MisconfiguredHPA int `json:"misconfiguredHPA"`
}

type HPAEffEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	HasHPA        bool   `json:"hasHPA"`
	MinReplicas   int32  `json:"minReplicas"`
	MaxReplicas   int32  `json:"maxReplicas"`
	CurrentReps   int32  `json:"currentReplicas"`
	TargetCPU     int32  `json:"targetCPU"`
	ScalingStatus string `json:"scalingStatus"`
	RiskLevel     string `json:"riskLevel"`
	Issue         string `json:"issue"`
}

func (s *Server) handleHPAEffectiveness1905(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := HPAEffResult{ScannedAt: time.Now()}

	// Get HPAs
	hpaMap := map[string]*HPAEffEntry{}
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	for _, hpa := range hpas.Items {
		if isSystemNamespace(hpa.Namespace) {
			continue
		}
		key := hpa.Namespace + "/" + hpa.Name
		entry := &HPAEffEntry{
			Name: hpa.Name, Namespace: hpa.Namespace, HasHPA: true,
		}
		if hpa.Spec.MinReplicas != nil {
			entry.MinReplicas = *hpa.Spec.MinReplicas
		}
		entry.MaxReplicas = hpa.Spec.MaxReplicas
		entry.CurrentReps = hpa.Status.CurrentReplicas

		// Target CPU from metrics
		for _, metric := range hpa.Spec.Metrics {
			if metric.Resource != nil && metric.Resource.Target.AverageUtilization != nil {
				entry.TargetCPU = *metric.Resource.Target.AverageUtilization
			}
		}

		// Status analysis
		switch {
		case hpa.Status.CurrentReplicas >= hpa.Spec.MaxReplicas:
			entry.ScalingStatus = "at-max"
			entry.RiskLevel = "high"
			entry.Issue = fmt.Sprintf("at max replicas (%d) - may need higher maxReplicas", hpa.Spec.MaxReplicas)
			result.Summary.HPAAtMax++
			result.Summary.MisconfiguredHPA++
		case hpa.Status.CurrentReplicas <= entry.MinReplicas:
			entry.ScalingStatus = "at-min"
			result.Summary.HPAAtMin++
		default:
			entry.ScalingStatus = "active"
			result.Summary.HPAScalingActive++
		}

		hpaMap[key] = entry
		result.ByWorkload = append(result.ByWorkload, *entry)
		if entry.RiskLevel != "" {
			result.Issues = append(result.Issues, *entry)
		}
	}

	// Check deployments without HPA
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++
		key := dep.Namespace + "/" + dep.Name
		if _, hasHPA := hpaMap[key]; hasHPA {
			result.Summary.WithHPA++
		} else {
			result.Summary.WithoutHPA++
			replicas := int32(1)
			if dep.Spec.Replicas != nil {
				replicas = *dep.Spec.Replicas
			}
			entry := HPAEffEntry{
				Name: dep.Name, Namespace: dep.Namespace, HasHPA: false,
				CurrentReps: replicas,
				RiskLevel:   "medium",
				Issue:       "no HPA configured - cannot auto-scale under load",
			}
			if replicas >= 3 {
				entry.RiskLevel = "low"
			}
			result.ByWorkload = append(result.ByWorkload, entry)
			if entry.RiskLevel == "medium" {
				result.Issues = append(result.Issues, entry)
			}
		}
	}

	// Score
	if result.Summary.TotalDeployments > 0 {
		hpaPct := result.Summary.WithHPA * 100 / result.Summary.TotalDeployments
		penalty := result.Summary.MisconfiguredHPA * 5
		result.HealthScore = hpaPct - penalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildHPAEffRecs1905(&result)
	writeJSON(w, result)
}

func buildHPAEffRecs1905(r *HPAEffResult) []string {
	recs := []string{fmt.Sprintf("HPA effectiveness: %d/%d deployments with HPA, %d active, %d at-max, %d misconfigured",
		r.Summary.WithHPA, r.Summary.TotalDeployments, r.Summary.HPAScalingActive, r.Summary.HPAAtMax, r.Summary.MisconfiguredHPA)}
	if r.Summary.WithoutHPA > 0 {
		recs = append(recs, fmt.Sprintf("%d deployments without HPA - add autoscaling for production workloads", r.Summary.WithoutHPA))
	}
	if r.Summary.HPAAtMax > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs at max replicas - increase maxReplicas or optimize workloads", r.Summary.HPAAtMax))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Pod Scheduling Latency
// ---------------------------------------------------------------

type SchedulingLatencyResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SchedLatencySummary   `json:"summary"`
	ByNS            []SchedLatencyNSEntry `json:"byNamespace"`
	SlowPods        []SchedLatencyEntry   `json:"slowPods"`
	ByReason        map[string]int        `json:"byReason"`
	Recommendations []string              `json:"recommendations"`
}

type SchedLatencySummary struct {
	TotalPods      int `json:"totalPods"`
	ScheduledOK    int `json:"scheduledOK"`
	PendingPods    int `json:"pendingPods"`
	AvgLatencySec  int `json:"avgLatencySec"`
	P95LatencySec  int `json:"p95LatencySec"`
	SlowScheduling int `json:"slowScheduling"`
	Unschedulable  int `json:"unschedulable"`
}

type SchedLatencyNSEntry struct {
	Namespace     string `json:"namespace"`
	PodCount      int    `json:"podCount"`
	AvgLatencySec int    `json:"avgLatencySec"`
	MaxLatencySec int    `json:"maxLatencySec"`
}

type SchedLatencyEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	LatencySec int    `json:"latencySec"`
	Reason     string `json:"reason"`
	RiskLevel  string `json:"riskLevel"`
}

func (s *Server) handleSchedulingLatency1905(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SchedulingLatencyResult{
		ScannedAt: time.Now(),
		ByReason:  map[string]int{},
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	now := time.Now()

	nsLatencies := map[string][]int{}
	var latencies []int

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++

		if pod.Spec.NodeName == "" {
			// Pending/unschedulable
			result.Summary.PendingPods++
			// Check for scheduling failure reason
			for _, cond := range pod.Status.Conditions {
				if cond.Reason != "" {
					result.ByReason[cond.Reason]++
					if strings.Contains(strings.ToLower(cond.Reason), "unschedulable") {
						result.Summary.Unschedulable++
						result.SlowPods = append(result.SlowPods, SchedLatencyEntry{
							Name: pod.Name, Namespace: pod.Namespace,
							Reason: cond.Reason, RiskLevel: "high",
							LatencySec: int(now.Sub(pod.CreationTimestamp.Time).Seconds()),
						})
					}
				}
			}
			continue
		}

		// Calculate scheduling latency from events
		scheduledTime := pod.Status.StartTime
		if scheduledTime == nil {
			continue
		}
		latency := int(scheduledTime.Time.Sub(pod.CreationTimestamp.Time).Seconds())
		if latency < 0 {
			latency = 0
		}

		result.Summary.ScheduledOK++
		latencies = append(latencies, latency)
		nsLatencies[pod.Namespace] = append(nsLatencies[pod.Namespace], latency)

		if latency > 300 { // > 5 minutes
			result.Summary.SlowScheduling++
			result.SlowPods = append(result.SlowPods, SchedLatencyEntry{
				Name: pod.Name, Namespace: pod.Namespace,
				LatencySec: latency, Reason: "slow-scheduling", RiskLevel: "high",
			})
		}
	}

	// Calculate average and P95
	if len(latencies) > 0 {
		sort.Ints(latencies)
		total := 0
		for _, l := range latencies {
			total += l
		}
		result.Summary.AvgLatencySec = total / len(latencies)
		p95Idx := len(latencies) * 95 / 100
		if p95Idx >= len(latencies) {
			p95Idx = len(latencies) - 1
		}
		result.Summary.P95LatencySec = latencies[p95Idx]
	}

	// Per-NS
	for ns, lats := range nsLatencies {
		sort.Ints(lats)
		total := 0
		for _, l := range lats {
			total += l
		}
		avg := 0
		if len(lats) > 0 {
			avg = total / len(lats)
		}
		result.ByNS = append(result.ByNS, SchedLatencyNSEntry{
			Namespace: ns, PodCount: len(lats),
			AvgLatencySec: avg, MaxLatencySec: lats[len(lats)-1],
		})
	}
	sort.Slice(result.ByNS, func(i, j int) bool {
		return result.ByNS[i].AvgLatencySec > result.ByNS[j].AvgLatencySec
	})

	// Score: lower latency = better
	if result.Summary.TotalPods > 0 {
		schedPct := result.Summary.ScheduledOK * 100 / result.Summary.TotalPods
		latencyPenalty := result.Summary.AvgLatencySec / 10
		unschedPenalty := result.Summary.Unschedulable * 10
		result.HealthScore = schedPct - latencyPenalty - unschedPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildSchedLatencyRecs1905(&result)
	writeJSON(w, result)
}

func buildSchedLatencyRecs1905(r *SchedulingLatencyResult) []string {
	recs := []string{fmt.Sprintf("Scheduling latency: %d pods scheduled, avg %ds, P95 %ds, %d slow, %d unschedulable",
		r.Summary.ScheduledOK, r.Summary.AvgLatencySec, r.Summary.P95LatencySec,
		r.Summary.SlowScheduling, r.Summary.Unschedulable)}
	if r.Summary.Unschedulable > 0 {
		recs = append(recs, fmt.Sprintf("%d unschedulable pods - check resource requests, node affinity, and taints", r.Summary.Unschedulable))
	}
	if r.Summary.P95LatencySec > 120 {
		recs = append(recs, fmt.Sprintf("P95 scheduling latency %ds > 120s - investigate scheduler capacity and node availability", r.Summary.P95LatencySec))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Capacity Headroom Calculator
// ---------------------------------------------------------------

type CapacityHeadroomResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HeadroomSummary     `json:"summary"`
	ByNode          []HeadroomNodeEntry `json:"byNode"`
	ByNS            []HeadroomNSEntry   `json:"byNamespace"`
	ScalingCapacity map[string]int      `json:"scalingCapacity"`
	Recommendations []string            `json:"recommendations"`
}

type HeadroomSummary struct {
	TotalNodes     int `json:"totalNodes"`
	TotalCPUm      int `json:"totalCPUm"`
	UsedCPUm       int `json:"usedCPUm"`
	AvailCPUm      int `json:"availCPUm"`
	TotalMemMB     int `json:"totalMemMB"`
	UsedMemMB      int `json:"usedMemMB"`
	AvailMemMB     int `json:"availMemMB"`
	TotalPodCap    int `json:"totalPodCapacity"`
	UsedPods       int `json:"usedPods"`
	AvailPods      int `json:"availPods"`
	AdditionalPods int `json:"additionalPods"`
}

type HeadroomNodeEntry struct {
	Node        string `json:"node"`
	AvailCPUm   int    `json:"availCPUm"`
	AvailMemMB  int    `json:"availMemMB"`
	AvailPods   int    `json:"availPods"`
	HeadroomPct int    `json:"headroomPct"`
}

type HeadroomNSEntry struct {
	Namespace string `json:"namespace"`
	CPUm      int    `json:"cpuMilli"`
	MemMB     int    `json:"memMB"`
	Pods      int    `json:"pods"`
}

func (s *Server) handleCapacityHeadroom1905(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CapacityHeadroomResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nsResources := map[string]*HeadroomNSEntry{}

	// Per-node usage
	nodeUsedCPUm := map[string]int{}
	nodeUsedMemMB := map[string]int{}
	nodeUsedPods := map[string]int{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nodeUsedPods[pod.Spec.NodeName]++

		nsE, ok := nsResources[pod.Namespace]
		if !ok {
			nsE = &HeadroomNSEntry{Namespace: pod.Namespace}
			nsResources[pod.Namespace] = nsE
		}
		nsE.Pods++

		for _, c := range pod.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue())
				nodeUsedCPUm[pod.Spec.NodeName] += m
				nsE.CPUm += m
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mb := int(qty.Value() / (1024 * 1024))
				nodeUsedMemMB[pod.Spec.NodeName] += mb
				nsE.MemMB += mb
			}
		}
	}

	// Calculate headroom per node
	for _, node := range nodes.Items {
		if !isNodeReady1893(&node) {
			continue
		}
		result.Summary.TotalNodes++

		allocCPUm := 0
		allocMemMB := 0
		allocPods := 0
		if qty, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			allocCPUm = int(qty.MilliValue())
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			allocMemMB = int(qty.Value() / (1024 * 1024))
		}
		if qty, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			allocPods = int(qty.Value())
		}

		result.Summary.TotalCPUm += allocCPUm
		result.Summary.TotalMemMB += allocMemMB
		result.Summary.TotalPodCap += allocPods
		result.Summary.UsedCPUm += nodeUsedCPUm[node.Name]
		result.Summary.UsedMemMB += nodeUsedMemMB[node.Name]
		result.Summary.UsedPods += nodeUsedPods[node.Name]

		availCPUm := allocCPUm - nodeUsedCPUm[node.Name]
		availMemMB := allocMemMB - nodeUsedMemMB[node.Name]
		availPods := allocPods - nodeUsedPods[node.Name]

		headroomPct := 0
		if allocCPUm > 0 {
			headroomPct = availCPUm * 100 / allocCPUm
		}

		result.ByNode = append(result.ByNode, HeadroomNodeEntry{
			Node: node.Name, AvailCPUm: availCPUm,
			AvailMemMB: availMemMB, AvailPods: availPods,
			HeadroomPct: headroomPct,
		})
	}

	result.Summary.AvailCPUm = result.Summary.TotalCPUm - result.Summary.UsedCPUm
	result.Summary.AvailMemMB = result.Summary.TotalMemMB - result.Summary.UsedMemMB
	result.Summary.AvailPods = result.Summary.TotalPodCap - result.Summary.UsedPods

	// Calculate additional pods that can fit (based on average pod resource)
	if result.Summary.UsedPods > 0 {
		avgPodCPUm := result.Summary.UsedCPUm / result.Summary.UsedPods
		if avgPodCPUm > 0 {
			result.ScalingCapacity = map[string]int{
				"byCPU":    result.Summary.AvailCPUm / avgPodCPUm,
				"byMemory": result.Summary.AvailMemMB / (result.Summary.UsedMemMB / result.Summary.UsedPods),
				"byPods":   result.Summary.AvailPods,
			}
			// Take the minimum
			minAdditional := result.ScalingCapacity["byCPU"]
			if result.ScalingCapacity["byMemory"] < minAdditional {
				minAdditional = result.ScalingCapacity["byMemory"]
			}
			if result.ScalingCapacity["byPods"] < minAdditional {
				minAdditional = result.ScalingCapacity["byPods"]
			}
			result.Summary.AdditionalPods = minAdditional
		}
	}

	for _, ns := range nsResources {
		result.ByNS = append(result.ByNS, *ns)
	}
	sort.Slice(result.ByNS, func(i, j int) bool {
		return result.ByNS[i].CPUm > result.ByNS[j].CPUm
	})

	// Score: higher headroom = better (but not too high = waste)
	if result.Summary.TotalCPUm > 0 {
		usedPct := result.Summary.UsedCPUm * 100 / result.Summary.TotalCPUm
		// Optimal range: 50-80%
		if usedPct >= 50 && usedPct <= 80 {
			result.HealthScore = 100
		} else if usedPct < 50 {
			result.HealthScore = 70 // underutilized
		} else {
			result.HealthScore = 100 - (usedPct-80)*2 // over-utilized
			if result.HealthScore < 0 {
				result.HealthScore = 0
			}
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildHeadroomRecs1905(&result)
	writeJSON(w, result)
}

func buildHeadroomRecs1905(r *CapacityHeadroomResult) []string {
	recs := []string{fmt.Sprintf("Capacity headroom: %d nodes, %dm/%dm CPU used (%d%%), %dMB/%dMB mem, %d additional pods possible",
		r.Summary.TotalNodes, r.Summary.UsedCPUm, r.Summary.TotalCPUm,
		r.Summary.TotalCPUm, r.Summary.UsedMemMB, r.Summary.TotalMemMB,
		r.Summary.AdditionalPods)}
	if r.Summary.AdditionalPods < 10 {
		recs = append(recs, fmt.Sprintf("Only %d additional pods can fit - plan capacity expansion", r.Summary.AdditionalPods))
	}
	return recs
}
