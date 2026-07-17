package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceSaturationWatchResult monitors real-time resource saturation trends
// across CPU, memory, disk, and PID dimensions, predicting imminent exhaustion.
type ResourceSaturationWatchResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         SaturationWatchSummary `json:"summary"`
	ByNode          []SaturationNodeEntry  `json:"byNode"`
	ByNamespace     []SaturationNsEntry    `json:"byNamespace"`
	Hotspots        []SaturationNodeEntry  `json:"hotspots"`
	WatchScore      int                    `json:"watchScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type SaturationWatchSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	CriticalNodes    int     `json:"criticalNodes"`
	WarningNodes     int     `json:"warningNodes"`
	HealthyNodes     int     `json:"healthyNodes"`
	AvgCPUSaturation float64 `json:"avgCPUSaturationPct"`
	AvgMemSaturation float64 `json:"avgMemSaturationPct"`
	AvgDiskUsage     float64 `json:"avgDiskUsagePct"`
	TopNamespace     string  `json:"topNamespace"`
}

type SaturationNodeEntry struct {
	NodeName       string  `json:"nodeName"`
	CPUSaturation  float64 `json:"cpuSaturationPct"`
	MemSaturation  float64 `json:"memSaturationPct"`
	DiskPressure   bool    `json:"diskPressure"`
	PIDPressure    bool    `json:"pidPressure"`
	PodCount       int     `json:"podCount"`
	PodCapacity    int     `json:"podCapacity"`
	PodSaturation  float64 `json:"podSaturationPct"`
	OverallStatus  string  `json:"overallStatus"`
	TrendDirection string  `json:"trendDirection"`
	ETAHours       int     `json:"etaToExhaustionHours"`
}

type SaturationNsEntry struct {
	Namespace     string  `json:"namespace"`
	CPUUsage      float64 `json:"cpuUsageCores"`
	MemUsageGB    float64 `json:"memUsageGB"`
	PodCount      int     `json:"podCount"`
	CPUPerPod     float64 `json:"cpuPerPod"`
	MemPerPodGB   float64 `json:"memPerPodGB"`
	SaturationPct float64 `json:"saturationPct"`
}

// handleResourceSaturationWatch handles GET /api/operations/resource-saturation-watch
func (s *Server) handleResourceSaturationWatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ResourceSaturationWatchResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build per-node resource usage
	nodeUsage := make(map[string]*SaturationNodeEntry)
	nsUsage := make(map[string]*SaturationNsEntry)

	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}

		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memAllocGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		podCapacity := 110
		if cap, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			podCapacity = int(cap.Value())
			if podCapacity <= 0 {
				podCapacity = 110
			}
		}

		entry := &SaturationNodeEntry{
			NodeName:    node.Name,
			PodCapacity: podCapacity,
		}

		// Check node conditions
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				entry.DiskPressure = true
			case corev1.NodeDiskPressure:
				entry.DiskPressure = true
			case corev1.NodePIDPressure:
				entry.PIDPressure = true
			}
		}

		nodeUsage[node.Name] = entry

		// Initialize with allocatable for saturation calculation
		_ = cpuAlloc
		_ = memAllocGB
		result.Summary.TotalNodes++
	}

	// Aggregate pod requests per node
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry, ok := nodeUsage[pod.Spec.NodeName]
		if !ok {
			continue
		}
		entry.PodCount++

		podCPU := 0.0
		podMem := 0.0
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				podCPU += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				podMem += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
		entry.CPUSaturation += podCPU
		entry.MemSaturation += podMem

		// Namespace tracking
		if !isSystemNamespace(pod.Namespace) {
			if _, ok := nsUsage[pod.Namespace]; !ok {
				nsUsage[pod.Namespace] = &SaturationNsEntry{Namespace: pod.Namespace}
			}
			nsEntry := nsUsage[pod.Namespace]
			nsEntry.CPUUsage += podCPU
			nsEntry.MemUsageGB += podMem
			nsEntry.PodCount++
		}
	}

	// Finalize node entries
	var entries []SaturationNodeEntry
	totalCPUSat := 0.0
	totalMemSat := 0.0
	workerCount := 0

	for _, node := range nodes.Items {
		entry, ok := nodeUsage[node.Name]
		if !ok {
			continue
		}
		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memAllocGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)

		// Calculate saturation percentages
		cpuPct := 0.0
		if cpuAlloc > 0 {
			cpuPct = entry.CPUSaturation / cpuAlloc * 100
		}
		memPct := 0.0
		if memAllocGB > 0 {
			memPct = entry.MemSaturation / memAllocGB * 100
		}
		podPct := float64(entry.PodCount) / float64(entry.PodCapacity) * 100

		// Store raw values in percentage form
		cpuRaw := entry.CPUSaturation
		memRaw := entry.MemSaturation
		entry.CPUSaturation = cpuPct
		entry.MemSaturation = memPct
		entry.PodSaturation = podPct

		// Determine status
		maxSat := cpuPct
		if memPct > maxSat {
			maxSat = memPct
		}
		if podPct > maxSat {
			maxSat = podPct
		}

		switch {
		case maxSat >= 85 || entry.DiskPressure || entry.PIDPressure:
			entry.OverallStatus = "critical"
			result.Summary.CriticalNodes++
		case maxSat >= 70:
			entry.OverallStatus = "warning"
			result.Summary.WarningNodes++
		default:
			entry.OverallStatus = "healthy"
			result.Summary.HealthyNodes++
		}

		// Trend direction (simplified: based on pod count vs capacity)
		if podPct > 80 {
			entry.TrendDirection = "rising"
			entry.ETAHours = int((100-podPct)/podPct*24) + 1
		} else {
			entry.TrendDirection = "stable"
			entry.ETAHours = 0
		}

		// Restore raw for ns tracking
		_ = cpuRaw
		_ = memRaw

		entries = append(entries, *entry)
		totalCPUSat += cpuPct
		totalMemSat += memPct
		workerCount++
	}

	if workerCount > 0 {
		result.Summary.AvgCPUSaturation = totalCPUSat / float64(workerCount)
		result.Summary.AvgMemSaturation = totalMemSat / float64(workerCount)
	}

	// Sort nodes by saturation (highest first)
	sort.Slice(entries, func(i, j int) bool {
		maxI := entries[i].CPUSaturation
		if entries[i].MemSaturation > maxI {
			maxI = entries[i].MemSaturation
		}
		maxJ := entries[j].CPUSaturation
		if entries[j].MemSaturation > maxJ {
			maxJ = entries[j].MemSaturation
		}
		return maxI > maxJ
	})
	result.ByNode = entries

	// Hotspots = critical or warning
	for _, e := range entries {
		if e.OverallStatus == "critical" || e.OverallStatus == "warning" {
			result.Hotspots = append(result.Hotspots, e)
		}
	}

	// Namespace entries
	var nsEntries []SaturationNsEntry
	topNsCPU := 0.0
	topNsName := ""
	for _, ns := range nsUsage {
		if ns.PodCount > 0 {
			ns.CPUPerPod = ns.CPUUsage / float64(ns.PodCount)
			ns.MemPerPodGB = ns.MemUsageGB / float64(ns.PodCount)
		}
		totalNsUsage := ns.CPUUsage + ns.MemUsageGB
		if totalNsUsage > 0 {
			ns.SaturationPct = ns.CPUUsage / (ns.CPUUsage + ns.MemUsageGB) * 100
		}
		nsEntries = append(nsEntries, *ns)
		if ns.CPUUsage > topNsCPU {
			topNsCPU = ns.CPUUsage
			topNsName = ns.Namespace
		}
	}
	sort.Slice(nsEntries, func(i, j int) bool {
		return nsEntries[i].CPUUsage > nsEntries[j].CPUUsage
	})
	result.ByNamespace = nsEntries
	result.Summary.TopNamespace = topNsName

	// Watch score: higher = healthier
	if result.Summary.TotalNodes > 0 {
		healthRatio := float64(result.Summary.HealthyNodes) / float64(result.Summary.TotalNodes)
		result.WatchScore = int(healthRatio * 100)
	} else {
		result.WatchScore = 100
	}

	switch {
	case result.WatchScore >= 80:
		result.Grade = "A"
	case result.WatchScore >= 60:
		result.Grade = "B"
	case result.WatchScore >= 40:
		result.Grade = "C"
	case result.WatchScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildSaturationWatchRecs(&result)
	writeJSON(w, result)
}

func buildSaturationWatchRecs(r *ResourceSaturationWatchResult) []string {
	recs := []string{
		fmt.Sprintf("资源饱和度: %d 节点, %d 正常, %d 警告, %d 严重", r.Summary.TotalNodes, r.Summary.HealthyNodes, r.Summary.WarningNodes, r.Summary.CriticalNodes),
	}
	if r.Summary.CriticalNodes > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个节点饱和度严重, 可能影响 Pod 调度和稳定性", r.Summary.CriticalNodes))
	}
	if r.Summary.AvgCPUSaturation > 80 {
		recs = append(recs, fmt.Sprintf("平均 CPU 饱和度 %.1f%% 过高, 建议扩容", r.Summary.AvgCPUSaturation))
	}
	if r.Summary.AvgMemSaturation > 80 {
		recs = append(recs, fmt.Sprintf("平均内存饱和度 %.1f%% 过高, 存在 OOM 风险", r.Summary.AvgMemSaturation))
	}
	if len(r.Hotspots) > 0 {
		top := r.Hotspots[0]
		recs = append(recs, fmt.Sprintf("热点: %s (CPU %.1f%%, Mem %.1f%%, Pods %.1f%%)", top.NodeName, top.CPUSaturation, top.MemSaturation, top.PodSaturation))
	}
	if r.Summary.TopNamespace != "" {
		recs = append(recs, fmt.Sprintf("最大消费者: %s", r.Summary.TopNamespace))
	}
	return recs
}
