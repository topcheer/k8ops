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

// SchedulingResult is the full scheduling health analysis output.
type SchedulingResult struct {
	ScannedAt          time.Time         `json:"scannedAt"`
	Summary            SchedulingSummary `json:"summary"`
	Nodes              []SchedulingNode  `json:"nodes"`
	PendingPods        []PendingPod      `json:"pendingPods,omitempty"`
	Evictions          []EvictionRecord  `json:"evictions,omitempty"`
	LargestFittablePod FittablePod       `json:"largestFittablePod"`
	EffectiveCapacity  EffectiveCapacity `json:"effectiveCapacity"`
	Fragmentation      FragmentationInfo `json:"fragmentation"`
	Recommendations    []string          `json:"recommendations"`
}

// SchedulingSummary aggregates cluster-wide scheduling metrics.
type SchedulingSummary struct {
	TotalNodes         int `json:"totalNodes"`
	SchedulableNodes   int `json:"schedulableNodes"`
	UnschedulableNodes int `json:"unschedulableNodes"`
	CordonedNodes      int `json:"cordonedNodes"`
	TaintedNodes       int `json:"taintedNodes"`
	NodesUnderPressure int `json:"nodesUnderPressure"`
	PendingPods        int `json:"pendingPods"`
	FailedScheduling   int `json:"failedScheduling"`
	RecentEvictions    int `json:"recentEvictions"`
	TotalPods          int `json:"totalPods"`
	HealthScore        int `json:"healthScore"` // 0 (worst) to 100 (best)
}

// SchedulingNode describes scheduling-relevant info for a single node.
type SchedulingNode struct {
	Name             string      `json:"name"`
	Schedulable      bool        `json:"schedulable"`
	Ready            bool        `json:"ready"`
	UnderPressure    bool        `json:"underPressure"`
	PressureTypes    []string    `json:"pressureTypes,omitempty"`
	Taints           []TaintInfo `json:"taints,omitempty"`
	CPUAllocatable   int64       `json:"cpuAllocatableM"`
	CPUAvailable     int64       `json:"cpuAvailableM"`
	CPUAvailablePct  float64     `json:"cpuAvailablePct"`
	MemAllocatableGB float64     `json:"memAllocatableGB"`
	MemAvailableGB   float64     `json:"memAvailableGB"`
	MemAvailablePct  float64     `json:"memAvailablePct"`
	PodCapacity      int         `json:"podCapacity"`
	PodAvailable     int         `json:"podAvailable"`
	PodAvailablePct  float64     `json:"podAvailablePct"`
}

// TaintInfo describes a node taint.
type TaintInfo struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

// PendingPod describes a pod stuck in Pending with its scheduling failure reasons.
type PendingPod struct {
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	CPURequest     int64             `json:"cpuRequestM"`
	MemRequestGB   float64           `json:"memRequestGB"`
	NodeSelector   map[string]string `json:"nodeSelector,omitempty"`
	AgeHours       float64           `json:"ageHours"`
	FailureReasons []string          `json:"failureReasons"`
}

// EvictionRecord describes a recent pod eviction.
type EvictionRecord struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Node      string  `json:"node"`
	Reason    string  `json:"reason"`
	AgeHours  float64 `json:"ageHours"`
}

// FittablePod represents the largest pod that could be scheduled right now.
type FittablePod struct {
	MaxCPUm     int64   `json:"maxCpuM"`
	MaxMemoryGB float64 `json:"maxMemoryGB"`
	MaxPods     int     `json:"maxPods"`
	BestNode    string  `json:"bestNode"`
}

// EffectiveCapacity shows theoretical vs usable capacity after removing unschedulable nodes.
type EffectiveCapacity struct {
	TheoreticalCPUm  int64   `json:"theoreticalCpuM"`
	EffectiveCPUm    int64   `json:"effectiveCpuM"`
	CPULostPct       float64 `json:"cpuLostPct"`
	TheoreticalMemGB float64 `json:"theoreticalMemGB"`
	EffectiveMemGB   float64 `json:"effectiveMemGB"`
	MemLostPct       float64 `json:"memLostPct"`
	TheoreticalPods  int     `json:"theoreticalPods"`
	EffectivePods    int     `json:"effectivePods"`
	PodsLostPct      float64 `json:"podsLostPct"`
}

// FragmentationInfo describes resource fragmentation in the cluster.
type FragmentationInfo struct {
	AvgCPUFragmentPct float64 `json:"avgCpuFragmentPct"`
	AvgMemFragmentPct float64 `json:"avgMemFragmentPct"`
	WorstFragmentNode string  `json:"worstFragmentNode"`
	WorstFragmentPct  float64 `json:"worstFragmentPct"`
	OversizedPodCount int     `json:"oversizedPodCount"`
}

// handleSchedulingHealth analyzes scheduling health, resource fragmentation,
// and node availability for accepting new workloads.
// GET /api/scheduling/health?namespace=xxx
func (s *Server) handleSchedulingHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SchedulingResult{
		ScannedAt: time.Now(),
	}

	// --- Get nodes ---
	nodeList, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// --- Get all pods ---
	podList, err := rc.clientset.CoreV1().Pods(r.URL.Query().Get("namespace")).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build per-node allocation map
	nodeAlloc := buildNodeAllocation(nodeList.Items, podList.Items)

	// --- Analyze each node ---
	var schedulableCPU, schedulableMem, schedulablePods int64
	var theoreticalCPU, theoreticalMem, theoreticalPods int64
	var maxFittableCPU, maxFittableMem int64
	maxFittableNode := ""
	maxFittablePods := 0

	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		sn := analyzeSchedulingNode(node, nodeAlloc[node.Name])
		result.Nodes = append(result.Nodes, sn)

		result.Summary.TotalNodes++
		theoreticalCPU += sn.CPUAllocatable
		theoreticalMem += int64(sn.MemAllocatableGB * 1024) // in Mi
		theoreticalPods += int64(sn.PodCapacity)

		if sn.Schedulable && sn.Ready && !sn.UnderPressure {
			result.Summary.SchedulableNodes++
			schedulableCPU += sn.CPUAllocatable
			schedulableMem += int64(sn.MemAllocatableGB * 1024)
			schedulablePods += int64(sn.PodCapacity)

			if sn.CPUAvailable > maxFittableCPU {
				maxFittableCPU = sn.CPUAvailable
				maxFittableMem = int64(sn.MemAvailableGB * 1024)
				maxFittableNode = sn.Name
			}
			if sn.PodAvailable > maxFittablePods {
				maxFittablePods = sn.PodAvailable
			}
		} else {
			result.Summary.UnschedulableNodes++
			if !sn.Schedulable {
				result.Summary.CordonedNodes++
			}
			if len(sn.Taints) > 0 {
				result.Summary.TaintedNodes++
			}
			if sn.UnderPressure {
				result.Summary.NodesUnderPressure++
			}
		}
	}

	// --- Analyze pending pods ---
	for i := range podList.Items {
		pod := &podList.Items[i]
		result.Summary.TotalPods++

		if pod.Status.Phase == corev1.PodPending {
			pp := buildPendingPod(pod)
			result.PendingPods = append(result.PendingPods, pp)
			result.Summary.PendingPods++
		}
	}

	// --- Get FailedScheduling events ---
	eventList, _ := rc.clientset.CoreV1().Events(r.URL.Query().Get("namespace")).List(ctx, metav1.ListOptions{
		FieldSelector: "reason=FailedScheduling",
		Limit:         500,
	})
	if eventList != nil {
		// Deduplicate: only count unique pods
		seenPods := make(map[string]bool)
		for _, ev := range eventList.Items {
			podKey := fmt.Sprintf("%s/%s", ev.InvolvedObject.Namespace, ev.InvolvedObject.Name)
			if !seenPods[podKey] {
				seenPods[podKey] = true
				result.Summary.FailedScheduling++
			}
			// Attach reasons to pending pods
			for i := range result.PendingPods {
				pp := &result.PendingPods[i]
				if pp.Namespace == ev.InvolvedObject.Namespace && pp.Name == ev.InvolvedObject.Name {
					reason := parseSchedulingFailure(ev.Message)
					if reason != "" {
						pp.FailureReasons = appendUnique(pp.FailureReasons, reason)
					}
				}
			}
		}
	}

	// --- Get eviction events ---
	evictionList, _ := rc.clientset.CoreV1().Events(r.URL.Query().Get("namespace")).List(ctx, metav1.ListOptions{
		FieldSelector: "reason=Evicted",
		Limit:         100,
	})
	if evictionList != nil {
		for _, ev := range evictionList.Items {
			if time.Since(ev.LastTimestamp.Time).Hours() < 24 {
				result.Evictions = append(result.Evictions, EvictionRecord{
					Pod:       ev.InvolvedObject.Name,
					Namespace: ev.InvolvedObject.Namespace,
					Node:      getEventNode(&ev),
					Reason:    parseEvictionReason(ev.Message),
					AgeHours:  time.Since(ev.LastTimestamp.Time).Hours(),
				})
				result.Summary.RecentEvictions++
			}
		}
	}

	// --- Calculate effective capacity ---
	result.EffectiveCapacity = EffectiveCapacity{
		TheoreticalCPUm:  theoreticalCPU,
		EffectiveCPUm:    schedulableCPU,
		TheoreticalMemGB: float64(theoreticalMem) / 1024,
		EffectiveMemGB:   float64(schedulableMem) / 1024,
		TheoreticalPods:  int(theoreticalPods),
		EffectivePods:    int(schedulablePods),
	}
	if theoreticalCPU > 0 {
		result.EffectiveCapacity.CPULostPct = pct(theoreticalCPU-schedulableCPU, theoreticalCPU)
	}
	if theoreticalMem > 0 {
		result.EffectiveCapacity.MemLostPct = pct(theoreticalMem-schedulableMem, theoreticalMem)
	}
	if theoreticalPods > 0 {
		result.EffectiveCapacity.PodsLostPct = pct(theoreticalPods-schedulablePods, theoreticalPods)
	}

	// --- Largest fittable pod ---
	result.LargestFittablePod = FittablePod{
		MaxCPUm:     maxFittableCPU,
		MaxMemoryGB: float64(maxFittableMem) / 1024,
		MaxPods:     maxFittablePods,
		BestNode:    maxFittableNode,
	}

	// --- Fragmentation analysis ---
	result.Fragmentation = analyzeFragmentation(result.Nodes)

	// --- Oversized pods (request more than any node can provide) ---
	maxNodeCPU := int64(0)
	maxNodeMem := 0.0
	for _, n := range result.Nodes {
		if n.Schedulable && n.Ready {
			if n.CPUAllocatable > maxNodeCPU {
				maxNodeCPU = n.CPUAllocatable
			}
			if n.MemAllocatableGB > maxNodeMem {
				maxNodeMem = n.MemAllocatableGB
			}
		}
	}
	for _, pp := range result.PendingPods {
		if pp.CPURequest > maxNodeCPU || pp.MemRequestGB > maxNodeMem {
			result.Fragmentation.OversizedPodCount++
		}
	}

	// --- Health score ---
	result.Summary.HealthScore = computeSchedulingScore(result.Summary, result.EffectiveCapacity, result.Fragmentation)

	// --- Recommendations ---
	result.Recommendations = generateSchedulingRecommendations(result)

	// Sort nodes by available capacity descending
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].CPUAvailable > result.Nodes[j].CPUAvailable
	})

	writeJSON(w, result)
}

// analyzeSchedulingNode extracts scheduling-relevant info from a node.
func analyzeSchedulingNode(node *corev1.Node, alloc *nodeAllocationData) SchedulingNode {
	sn := SchedulingNode{
		Name:             node.Name,
		Schedulable:      !node.Spec.Unschedulable,
		Ready:            isNodeReady(node),
		CPUAllocatable:   node.Status.Allocatable.Cpu().MilliValue(),
		MemAllocatableGB: float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024),
		PodCapacity:      int(node.Status.Allocatable.Pods().Value()),
	}

	// Extract taints
	for _, t := range node.Spec.Taints {
		sn.Taints = append(sn.Taints, TaintInfo{
			Key:    t.Key,
			Value:  t.Value,
			Effect: string(t.Effect),
		})
	}

	// Check pressure conditions
	for _, cond := range node.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		switch cond.Type {
		case corev1.NodeMemoryPressure:
			sn.UnderPressure = true
			sn.PressureTypes = append(sn.PressureTypes, "memory")
		case corev1.NodeDiskPressure:
			sn.UnderPressure = true
			sn.PressureTypes = append(sn.PressureTypes, "disk")
		case corev1.NodePIDPressure:
			sn.UnderPressure = true
			sn.PressureTypes = append(sn.PressureTypes, "pid")
		case corev1.NodeNetworkUnavailable:
			sn.UnderPressure = true
			sn.PressureTypes = append(sn.PressureTypes, "network")
		}
	}

	// Calculate available resources
	if alloc != nil {
		sn.CPUAvailable = sn.CPUAllocatable - alloc.cpuM
		sn.MemAvailableGB = sn.MemAllocatableGB - alloc.memGB
		sn.PodAvailable = sn.PodCapacity - alloc.pods
	} else {
		sn.CPUAvailable = sn.CPUAllocatable
		sn.MemAvailableGB = sn.MemAllocatableGB
		sn.PodAvailable = sn.PodCapacity
	}

	if sn.CPUAllocatable > 0 {
		sn.CPUAvailablePct = pct(sn.CPUAvailable, sn.CPUAllocatable)
	}
	if sn.MemAllocatableGB > 0 {
		sn.MemAvailablePct = pct(int64(sn.MemAvailableGB*1024), int64(sn.MemAllocatableGB*1024))
	}
	if sn.PodCapacity > 0 {
		sn.PodAvailablePct = float64(sn.PodAvailable) / float64(sn.PodCapacity) * 100
	}

	return sn
}

// nodeAllocationData tracks how much resources are allocated per node.
type nodeAllocationData struct {
	cpuM  int64
	memGB float64
	pods  int
}

// buildNodeAllocation sums up resource requests per node from running pods.
func buildNodeAllocation(nodes []corev1.Node, pods []corev1.Pod) map[string]*nodeAllocationData {
	allocMap := make(map[string]*nodeAllocationData)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		data, ok := allocMap[pod.Spec.NodeName]
		if !ok {
			data = &nodeAllocationData{}
			allocMap[pod.Spec.NodeName] = data
		}
		data.pods++
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu() != nil {
				data.cpuM += c.Resources.Requests.Cpu().MilliValue()
			}
			if c.Resources.Requests.Memory() != nil {
				data.memGB += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
			}
		}
	}
	return allocMap
}

// buildPendingPod extracts scheduling-relevant data from a Pending pod.
func buildPendingPod(pod *corev1.Pod) PendingPod {
	pp := PendingPod{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		NodeSelector: pod.Spec.NodeSelector,
		AgeHours:     hoursSince(pod.CreationTimestamp),
	}
	for _, c := range pod.Spec.Containers {
		if c.Resources.Requests.Cpu() != nil {
			pp.CPURequest += c.Resources.Requests.Cpu().MilliValue()
		}
		if c.Resources.Requests.Memory() != nil {
			pp.MemRequestGB += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}
	return pp
}

// parseSchedulingFailure extracts human-readable reasons from FailedScheduling event messages.
func parseSchedulingFailure(msg string) string {
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "insufficient cpu"):
		return "insufficient-cpu"
	case strings.Contains(lower, "insufficient memory"):
		return "insufficient-memory"
	case strings.Contains(lower, "untolerated taint"):
		return "untolerated-taint"
	case strings.Contains(lower, "volume") || strings.Contains(lower, "pvc"):
		return "volume-binding-failure"
	case strings.Contains(lower, "node affinity"):
		return "node-affinity-mismatch"
	case strings.Contains(lower, "node selector") || strings.Contains(lower, "didn't match"):
		return "node-selector-mismatch"
	case strings.Contains(lower, "pod affinity") || strings.Contains(lower, "pod anti-affinity"):
		return "pod-affinity-conflict"
	case strings.Contains(lower, "insufficient pods"):
		return "pod-limit-reached"
	case strings.Contains(lower, "0/") && strings.Contains(lower, "nodes are available"):
		return "no-nodes-available"
	}
	return "unknown"
}

// parseEvictionReason extracts the reason from an eviction event message.
func parseEvictionReason(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "memory"):
		return "low-memory"
	case strings.Contains(lower, "disk") || strings.Contains(lower, "storage"):
		return "low-disk-space"
	case strings.Contains(lower, "pid"):
		return "pid-pressure"
	}
	if len(msg) > 80 {
		return msg[:80]
	}
	return msg
}

// getEventNode extracts the node name from an event's involved object or message.
func getEventNode(ev *corev1.Event) string {
	if ev.Source.Host != "" {
		return ev.Source.Host
	}
	return ""
}

// analyzeFragmentation computes resource fragmentation metrics.
func analyzeFragmentation(nodes []SchedulingNode) FragmentationInfo {
	info := FragmentationInfo{}
	var totalCPUFrag, totalMemFrag float64
	count := 0

	for _, n := range nodes {
		if !n.Schedulable || !n.Ready {
			continue
		}
		count++
		// Fragmentation = available resources that are scattered across nodes
		// High availability % + low absolute quantity = high fragmentation risk
		cpuFrag := n.CPUAvailablePct
		memFrag := n.MemAvailablePct
		totalCPUFrag += cpuFrag
		totalMemFrag += memFrag

		combinedFrag := (cpuFrag + memFrag) / 2
		if combinedFrag > info.WorstFragmentPct {
			info.WorstFragmentPct = combinedFrag
			info.WorstFragmentNode = n.Name
		}
	}

	if count > 0 {
		info.AvgCPUFragmentPct = totalCPUFrag / float64(count)
		info.AvgMemFragmentPct = totalMemFrag / float64(count)
	}

	return info
}

// computeSchedulingScore computes a 0-100 health score for scheduling.
func computeSchedulingScore(summary SchedulingSummary, cap EffectiveCapacity, frag FragmentationInfo) int {
	score := 100

	// Penalty for unschedulable nodes
	if summary.TotalNodes > 0 {
		unschedPct := float64(summary.UnschedulableNodes) / float64(summary.TotalNodes) * 100
		score -= int(unschedPct * 0.3)
	}

	// Penalty for pending pods
	score -= summary.PendingPods * 5

	// Penalty for nodes under pressure
	score -= summary.NodesUnderPressure * 10

	// Penalty for capacity lost to unschedulable nodes
	score -= int(cap.CPULostPct * 0.2)
	score -= int(cap.MemLostPct * 0.2)

	// Penalty for high fragmentation
	if frag.AvgCPUFragmentPct > 50 && summary.SchedulableNodes > 1 {
		score -= 5
	}

	// Penalty for oversized pods
	score -= frag.OversizedPodCount * 5

	// Penalty for recent evictions
	score -= summary.RecentEvictions * 3

	if score < 0 {
		score = 0
	}
	return score
}

// generateSchedulingRecommendations produces actionable recommendations.
func generateSchedulingRecommendations(result SchedulingResult) []string {
	var recs []string

	if result.Summary.PendingPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) are stuck in Pending — check FailedScheduling events for specific constraints", result.Summary.PendingPods))
	}

	if result.Summary.CordonedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are cordoned — uncordon if maintenance is complete to restore capacity", result.Summary.CordonedNodes))
	}

	if result.EffectiveCapacity.CPULostPct > 25 {
		recs = append(recs, fmt.Sprintf("%.0f%% of CPU capacity is lost to unschedulable nodes — consider adding nodes or uncordoning", result.EffectiveCapacity.CPULostPct))
	}

	if result.EffectiveCapacity.MemLostPct > 25 {
		recs = append(recs, fmt.Sprintf("%.0f%% of memory capacity is lost to unschedulable nodes — consider adding nodes or uncordoning", result.EffectiveCapacity.MemLostPct))
	}

	if result.Fragmentation.OversizedPodCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pending pod(s) request more resources than any single node can provide — either add larger nodes or reduce resource requests", result.Fragmentation.OversizedPodCount))
	}

	if result.Summary.RecentEvictions > 0 {
		recs = append(recs, fmt.Sprintf("%d eviction(s) in the last 24h — investigate node pressure conditions (memory/disk/PID)", result.Summary.RecentEvictions))
	}

	if result.Summary.NodesUnderPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) under pressure conditions — drain and investigate resource exhaustion", result.Summary.NodesUnderPressure))
	}

	if result.LargestFittablePod.MaxCPUm < 1000 && result.Summary.SchedulableNodes > 0 {
		recs = append(recs, fmt.Sprintf("Largest schedulable pod is only %dm CPU / %.1fGB memory — cluster is near capacity, consider scaling out", result.LargestFittablePod.MaxCPUm, result.LargestFittablePod.MaxMemoryGB))
	}

	return recs
}

// isNodeReady returns true if the node has Ready condition = True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// pct computes a percentage safely.
func pct(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// appendUnique adds a string to a slice only if not already present.
func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
