package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceRanking represents resource consumption for a single namespace.
type NamespaceRanking struct {
	Name          string  `json:"name"`
	PodCount      int     `json:"podCount"`
	CPURequest    int64   `json:"cpuRequestMcores"` // milli-cores
	CPULimit      int64   `json:"cpuLimitMcores"`   // milli-cores
	MemRequest    int64   `json:"memRequestMB"`     // MB
	MemLimit      int64   `json:"memLimitMB"`       // MB
	CPURequestPct float64 `json:"cpuRequestPct"`    // % of cluster allocatable
	MemRequestPct float64 `json:"memRequestPct"`    // % of cluster allocatable
	PVCCount      int     `json:"pvcCount"`
	PVCStorageGB  float64 `json:"pvcStorageGB"`
}

// handleNamespaceRanking returns resource consumption ranked by namespace.
// GET /api/namespaces/ranking
func (s *Server) handleNamespaceRanking(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Get all pods for resource request/limit aggregation
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get PVCs for storage consumption
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err == nil {
		// continue even if PVCs fail
	}

	// Calculate cluster-wide allocatable for percentage
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	var totalAllocatableCPU, totalAllocatableMem int64
	if err == nil {
		for _, n := range nodes.Items {
			totalAllocatableCPU += n.Status.Allocatable.Cpu().MilliValue()
			totalAllocatableMem += n.Status.Allocatable.Memory().Value()
		}
	}

	// Aggregate per namespace
	nsData := map[string]*NamespaceRanking{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		ns := pod.Namespace
		if ns == "" {
			continue
		}

		entry, ok := nsData[ns]
		if !ok {
			entry = &NamespaceRanking{Name: ns}
			nsData[ns] = entry
		}
		entry.PodCount++

		for _, c := range pod.Spec.Containers {
			if req := c.Resources.Requests.Cpu(); req != nil {
				entry.CPURequest += req.MilliValue()
			}
			if lim := c.Resources.Limits.Cpu(); lim != nil {
				entry.CPULimit += lim.MilliValue()
			}
			if req := c.Resources.Requests.Memory(); req != nil {
				entry.MemRequest += req.Value()
			}
			if lim := c.Resources.Limits.Memory(); lim != nil {
				entry.MemLimit += lim.Value()
			}
		}
	}

	// Aggregate PVC storage per namespace
	if pvcs != nil {
		for _, pvc := range pvcs.Items {
			ns := pvc.Namespace
			entry, ok := nsData[ns]
			if !ok {
				entry = &NamespaceRanking{Name: ns}
				nsData[ns] = entry
			}
			entry.PVCCount++
			if pvc.Status.Capacity != nil {
				if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
					entry.PVCStorageGB += float64(storage.Value()) / 1024 / 1024 / 1024
				}
			}
		}
	}

	// Convert to slice and calculate percentages
	rankings := make([]NamespaceRanking, 0, len(nsData))
	for _, entry := range nsData {
		// Convert memory bytes to MB
		entry.MemRequest = entry.MemRequest / 1024 / 1024
		entry.MemLimit = entry.MemLimit / 1024 / 1024

		// Calculate percentage of cluster allocatable
		if totalAllocatableCPU > 0 {
			entry.CPURequestPct = float64(entry.CPURequest) / float64(totalAllocatableCPU) * 100
		}
		if totalAllocatableMem > 0 {
			entry.MemRequestPct = float64(entry.MemRequest*1024*1024) / float64(totalAllocatableMem) * 100
		}
		rankings = append(rankings, *entry)
	}

	// Sort by CPU request descending (top consumers first)
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].CPURequest > rankings[j].CPURequest
	})

	// Build summary
	totalPods, totalCPUReq, totalMemReq := 0, int64(0), int64(0)
	for _, r := range rankings {
		totalPods += r.PodCount
		totalCPUReq += r.CPURequest
		totalMemReq += r.MemRequest
	}

	writeJSON(w, map[string]any{
		"count": len(rankings),
		"summary": map[string]any{
			"totalNamespaces":         len(rankings),
			"totalPods":               totalPods,
			"totalCPURequestM":        totalCPUReq,
			"totalMemRequestMB":       totalMemReq,
			"clusterAllocatableCPU":   totalAllocatableCPU,
			"clusterAllocatableMemMB": totalAllocatableMem / 1024 / 1024,
		},
		"items": rankings,
	})
}

// handleNamespaceDetail returns detailed resource info for a single namespace.
// GET /api/namespaces/{name}/detail
func (s *Server) handleNamespaceDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nsName := r.URL.Path
	// Extract namespace name from path: /api/namespaces/{name}/detail
	parts := splitPath(nsName)
	if len(parts) < 4 {
		writeError(w, http.StatusBadRequest, "namespace name required")
		return
	}
	nsName = parts[3] // /api/namespaces/{name}/detail

	// Get ResourceQuotas for this namespace
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas(nsName).List(ctx, metav1.ListOptions{})
	quotaList := make([]map[string]any, 0)
	for _, q := range quotas.Items {
		hard := map[string]string{}
		used := map[string]string{}
		for k, v := range q.Status.Hard {
			hard[string(k)] = v.String()
		}
		for k, v := range q.Status.Used {
			used[string(k)] = v.String()
		}
		quotaList = append(quotaList, map[string]any{
			"name": q.Name,
			"hard": hard,
			"used": used,
		})
	}

	// Get LimitRanges
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges(nsName).List(ctx, metav1.ListOptions{Limit: 10})
	lrList := make([]map[string]any, 0)
	for _, lr := range limitRanges.Items {
		limits := make([]map[string]string, 0)
		for _, lim := range lr.Spec.Limits {
			entry := map[string]string{
				"type": string(lim.Type),
			}
			for k, v := range lim.Default {
				entry["default_"+string(k)] = v.String()
			}
			for k, v := range lim.DefaultRequest {
				entry["defaultRequest_"+string(k)] = v.String()
			}
			for k, v := range lim.Max {
				entry["max_"+string(k)] = v.String()
			}
			for k, v := range lim.Min {
				entry["min_"+string(k)] = v.String()
			}
			limits = append(limits, entry)
		}
		lrList = append(lrList, map[string]any{
			"name":   lr.Name,
			"limits": limits,
		})
	}

	// Get events for this namespace (warnings only)
	events, _ := rc.clientset.CoreV1().Events(nsName).List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         10,
	})
	eventList := make([]map[string]any, 0)
	for _, e := range events.Items {
		eventList = append(eventList, map[string]any{
			"reason":   e.Reason,
			"message":  truncate(e.Message, 200),
			"object":   fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			"count":    e.Count,
			"lastTime": e.LastTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, map[string]any{
		"namespace":      nsName,
		"quotas":         quotaList,
		"limitRanges":    lrList,
		"recentWarnings": eventList,
	})
}

// splitPath splits a URL path into segments, filtering empty strings.
func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// formatMilliCores converts milli-cores to a human-readable string.
func formatMilliCores(m int64) string {
	if m >= 1000 {
		return strconv.FormatFloat(float64(m)/1000, 'f', 2, 64) + " cores"
	}
	return strconv.FormatInt(m, 10) + "m"
}
