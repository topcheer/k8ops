package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCCapacityInfo describes a single PVC's storage status.
type PVCCapacityInfo struct {
	Name         string  `json:"name"`
	Namespace    string  `json:"namespace"`
	Status       string  `json:"status"`
	CapacityGB   float64 `json:"capacityGB"`
	RequestedGB  float64 `json:"requestedGB"`
	StorageClass string  `json:"storageClass"`
	VolumeName   string  `json:"volumeName"`
	AccessMode   string  `json:"accessMode"`
}

// NodeCapacityInfo describes a single node's capacity vs. allocated.
type NodeCapacityInfo struct {
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	CPUAllocatable   int64   `json:"cpuAllocatableM"` // milli-cores
	CPURequested     int64   `json:"cpuRequestedM"`
	CPURequestedPct  float64 `json:"cpuRequestedPct"`
	CPULimit         int64   `json:"cpuLimitM"`
	CPULimitPct      float64 `json:"cpuLimitPct"`
	MemAllocatableGB float64 `json:"memAllocatableGB"`
	MemRequestedGB   float64 `json:"memRequestedGB"`
	MemRequestedPct  float64 `json:"memRequestedPct"`
	MemLimitGB       float64 `json:"memLimitGB"`
	MemLimitPct      float64 `json:"memLimitPct"`
	PodCount         int     `json:"podCount"`
	PodCapacity      int     `json:"podCapacity"`
	PodUsedPct       float64 `json:"podUsedPct"`
}

// handleStorageCapacity returns PVC/PV storage overview with capacity info.
// GET /api/storage/capacity
func (s *Server) handleStorageCapacity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Get all PVCs
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	items := make([]PVCCapacityInfo, 0, len(pvcs.Items))
	var totalCapacityGB, totalRequestedGB float64
	boundCount, pendingCount, releasedCount := 0, 0, 0

	for _, pvc := range pvcs.Items {
		status := string(pvc.Status.Phase)
		if status == "" {
			status = "Unknown"
		}

		var capacityGB float64
		if pvc.Status.Capacity != nil {
			if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				capacityGB = float64(storage.Value()) / 1024 / 1024 / 1024
			}
		}

		var requestedGB float64
		if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			requestedGB = float64(req.Value()) / 1024 / 1024 / 1024
		}

		accessMode := ""
		if len(pvc.Spec.AccessModes) > 0 {
			accessMode = string(pvc.Spec.AccessModes[0])
		}

		sc := pvc.Spec.StorageClassName
		if sc == nil {
			sc = &[]string{""}[0]
		}

		items = append(items, PVCCapacityInfo{
			Name:         pvc.Name,
			Namespace:    pvc.Namespace,
			Status:       status,
			CapacityGB:   capacityGB,
			RequestedGB:  requestedGB,
			StorageClass: *sc,
			VolumeName:   pvc.Spec.VolumeName,
			AccessMode:   accessMode,
		})

		totalCapacityGB += capacityGB
		totalRequestedGB += requestedGB

		switch status {
		case "Bound":
			boundCount++
		case "Pending":
			pendingCount++
		case "Released", "Lost":
			releasedCount++
		}
	}

	// Sort by capacity descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].CapacityGB > items[j].CapacityGB
	})

	writeJSON(w, map[string]any{
		"count": len(items),
		"summary": map[string]any{
			"totalPVCs":        len(items),
			"bound":            boundCount,
			"pending":          pendingCount,
			"released":         releasedCount,
			"totalCapacityGB":  totalCapacityGB,
			"totalRequestedGB": totalRequestedGB,
		},
		"items": items,
	})
}

// handleCapacityPlanning returns node capacity vs. allocated resources with recommendations.
// GET /api/capacity/planning
func (s *Server) handleCapacityPlanning(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Get nodes
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get all pods for per-node resource aggregation
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	type nodeUsage struct {
		cpuReq, cpuLim int64 // milli-cores
		memReq, memLim int64 // bytes
		pods           int
	}
	usageMap := make(map[string]*nodeUsage)

	for _, pod := range allPods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		u, ok := usageMap[pod.Spec.NodeName]
		if !ok {
			u = &nodeUsage{}
			usageMap[pod.Spec.NodeName] = u
		}
		u.pods++
		for _, c := range pod.Spec.Containers {
			if req := c.Resources.Requests.Cpu(); req != nil {
				u.cpuReq += req.MilliValue()
			}
			if lim := c.Resources.Limits.Cpu(); lim != nil {
				u.cpuLim += lim.MilliValue()
			}
			if req := c.Resources.Requests.Memory(); req != nil {
				u.memReq += req.Value()
			}
			if lim := c.Resources.Limits.Memory(); lim != nil {
				u.memLim += lim.Value()
			}
		}
	}

	// Build node capacity info
	items := make([]NodeCapacityInfo, 0, len(nodes.Items))
	recommendations := make([]string, 0)

	for _, n := range nodes.Items {
		status := "Ready"
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionFalse {
				status = "NotReady"
			}
		}

		allocCPU := n.Status.Allocatable.Cpu().MilliValue()
		allocMem := n.Status.Allocatable.Memory().Value()
		podCap := int(n.Status.Allocatable.Pods().Value())

		u := usageMap[n.Name]
		if u == nil {
			u = &nodeUsage{}
		}

		info := NodeCapacityInfo{
			Name:             n.Name,
			Status:           status,
			CPUAllocatable:   allocCPU,
			CPURequested:     u.cpuReq,
			CPULimit:         u.cpuLim,
			MemAllocatableGB: float64(allocMem) / 1024 / 1024 / 1024,
			MemRequestedGB:   float64(u.memReq) / 1024 / 1024 / 1024,
			MemLimitGB:       float64(u.memLim) / 1024 / 1024 / 1024,
			PodCount:         u.pods,
			PodCapacity:      podCap,
		}

		if allocCPU > 0 {
			info.CPURequestedPct = float64(u.cpuReq) / float64(allocCPU) * 100
			info.CPULimitPct = float64(u.cpuLim) / float64(allocCPU) * 100
		}
		if allocMem > 0 {
			info.MemRequestedPct = float64(u.memReq) / float64(allocMem) * 100
			info.MemLimitPct = float64(u.memLim) / float64(allocMem) * 100
		}
		if podCap > 0 {
			info.PodUsedPct = float64(u.pods) / float64(podCap) * 100
		}

		items = append(items, info)

		// Generate recommendations for stressed nodes
		if info.CPURequestedPct > 80 {
			recommendations = append(recommendations,
				fmt.Sprintf("Node %q CPU request at %.0f%% — consider adding nodes or reducing workload", n.Name, info.CPURequestedPct))
		}
		if info.MemRequestedPct > 80 {
			recommendations = append(recommendations,
				fmt.Sprintf("Node %q memory request at %.0f%% — at risk of OOMKill under pressure", n.Name, info.MemRequestedPct))
		}
		if info.PodUsedPct > 85 {
			recommendations = append(recommendations,
				fmt.Sprintf("Node %q pod count at %.0f%% (%d/%d) — approaching pod limit", n.Name, info.PodUsedPct, u.pods, podCap))
		}
	}

	// Sort by CPU pressure descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].CPURequestedPct > items[j].CPURequestedPct
	})

	// Aggregate cluster totals
	var totalCPUAlloc, totalCPUReq, totalMemAlloc, totalMemReq int64
	totalPods, totalPodCap := 0, 0
	for _, n := range items {
		if n.Status != "Ready" {
			continue
		}
		totalCPUAlloc += n.CPUAllocatable
		totalCPUReq += n.CPURequested
		totalMemAlloc += int64(n.MemAllocatableGB * 1024 * 1024 * 1024)
		totalMemReq += int64(n.MemRequestedGB * 1024 * 1024 * 1024)
		totalPods += n.PodCount
		totalPodCap += n.PodCapacity
	}

	clusterCPUUtil := 0.0
	if totalCPUAlloc > 0 {
		clusterCPUUtil = float64(totalCPUReq) / float64(totalCPUAlloc) * 100
	}
	clusterMemUtil := 0.0
	if totalMemAlloc > 0 {
		clusterMemUtil = float64(totalMemReq) / float64(totalMemAlloc) * 100
	}

	// Cluster-level recommendations
	if clusterCPUUtil > 70 {
		recommendations = append(recommendations,
			fmt.Sprintf("Cluster CPU utilization at %.0f%% — plan for capacity expansion", clusterCPUUtil))
	}
	if clusterMemUtil > 70 {
		recommendations = append(recommendations,
			fmt.Sprintf("Cluster memory utilization at %.0f%% — plan for capacity expansion", clusterMemUtil))
	}
	if totalPodCap > 0 && float64(totalPods)/float64(totalPodCap) > 0.7 {
		recommendations = append(recommendations,
			fmt.Sprintf("Cluster pod density at %.0f%% (%d/%d) — consider increasing max-pods or adding nodes",
				float64(totalPods)/float64(totalPodCap)*100, totalPods, totalPodCap))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "All resources within healthy thresholds. No expansion needed.")
	}

	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"nodeCount":             len(items),
			"clusterCPUUtilPct":     clusterCPUUtil,
			"clusterMemUtilPct":     clusterMemUtil,
			"totalPods":             totalPods,
			"totalPodCapacity":      totalPodCap,
			"totalCPUAllocatable":   fmt.Sprintf("%.1f cores", float64(totalCPUAlloc)/1000),
			"totalCPURequested":     fmt.Sprintf("%.1f cores", float64(totalCPUReq)/1000),
			"totalMemAllocatableGB": float64(totalMemAlloc) / 1024 / 1024 / 1024,
			"totalMemRequestedGB":   float64(totalMemReq) / 1024 / 1024 / 1024,
		},
		"recommendations": recommendations,
		"nodes":           items,
	})
}

// formatStorageSize converts bytes to a human-readable GB string.
func formatStorageGB(bytes int64) string {
	gb := float64(bytes) / 1024 / 1024 / 1024
	if gb < 1 {
		return fmt.Sprintf("%.0f MB", gb*1024)
	}
	return fmt.Sprintf("%.1f GB", gb)
}

// safeDeref returns the dereferenced string or empty.
func safeDeref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
