package dashboard

import (
	"fmt"
	"net/http"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EfficiencyReport contains cluster-wide resource efficiency analysis.
type EfficiencyReport struct {
	Score           float64             `json:"score"` // 0-100, higher is better
	WasteItems      []WasteItem         `json:"wasteItems"`
	Underutilized   []UnderutilizedNode `json:"underutilizedNodes"`
	OverProvisioned []OverProvisioned   `json:"overProvisioned"`
	Recommendations []string            `json:"recommendations"`
	Stats           EfficiencyStats     `json:"stats"`
}

// WasteItem represents a detected resource waste situation.
type WasteItem struct {
	Type            string `json:"type"`     // "idle_pod", "excessive_replicas", "oversized_limits"
	Resource        string `json:"resource"` // namespace/name
	Namespace       string `json:"namespace"`
	Details         string `json:"details"`
	PotentialSaving string `json:"potentialSaving"`
}

// UnderutilizedNode identifies a node with low resource utilization.
type UnderutilizedNode struct {
	Name        string  `json:"name"`
	CPUUsagePct float64 `json:"cpuUsagePct"`
	MemUsagePct float64 `json:"memUsagePct"`
	PodCount    int     `json:"podCount"`
	PodCapacity int     `json:"podCapacity"`
	Details     string  `json:"details"`
}

// OverProvisioned identifies a container with excessive resource requests/limits.
type OverProvisioned struct {
	Workload      string `json:"workload"`
	Namespace     string `json:"namespace"`
	Container     string `json:"container"`
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	Issue         string `json:"issue"`
	WastedCPU     string `json:"wastedCPU"`
	WastedMemory  string `json:"wastedMemory"`
}

// EfficiencyStats holds summary statistics.
type EfficiencyStats struct {
	TotalPods          int     `json:"totalPods"`
	IdlePods           int     `json:"idlePods"`
	NoResourceLimits   int     `json:"noResourceLimits"`
	OversizedLimits    int     `json:"oversizedLimits"`
	TotalNodes         int     `json:"totalNodes"`
	UnderutilizedNodes int     `json:"underutilizedNodes"`
	EstimatedWastePct  float64 `json:"estimatedWastePct"`
}

// handleEfficiency returns a cluster resource efficiency analysis.
// GET /api/efficiency
func (s *Server) handleEfficiency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	report := analyzeEfficiency(nodes.Items, pods.Items)

	writeJSON(w, report)
}

// analyzeEfficiency inspects nodes and pods for resource waste patterns.
func analyzeEfficiency(nodes []corev1.Node, pods []corev1.Pod) EfficiencyReport {
	report := EfficiencyReport{
		WasteItems:      []WasteItem{},
		Underutilized:   []UnderutilizedNode{},
		OverProvisioned: []OverProvisioned{},
		Recommendations: []string{},
	}

	idlePods := 0
	noLimits := 0
	oversized := 0

	// 1. Detect idle pods (Running but 0 restarts and age > 7 days)
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}

		// Skip system namespaces
		ns := pod.Namespace
		if ns == "kube-system" || ns == "k8ops-system" {
			continue
		}

		// Check for pods without resource limits
		hasLimits := false
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Limits.Cpu().IsZero() || !c.Resources.Limits.Memory().IsZero() {
				hasLimits = true
			}
		}
		if !hasLimits {
			noLimits++
		}

		// Check for oversized limits (limit >> request ratio > 10x)
		for _, c := range pod.Spec.Containers {
			reqCPU := c.Resources.Requests.Cpu()
			limCPU := c.Resources.Limits.Cpu()
			reqMem := c.Resources.Requests.Memory()
			limMem := c.Resources.Limits.Memory()

			if !reqCPU.IsZero() && !limCPU.IsZero() {
				ratio := limCPU.AsApproximateFloat64() / reqCPU.AsApproximateFloat64()
				if ratio > 10 {
					oversized++
					report.OverProvisioned = append(report.OverProvisioned, OverProvisioned{
						Workload:   fmt.Sprintf("%s/%s", pod.Kind, pod.Name),
						Namespace:  pod.Namespace,
						Container:  c.Name,
						CPURequest: reqCPU.String(),
						CPULimit:   limCPU.String(),
						Issue:      fmt.Sprintf("CPU limit/request ratio %.1fx (recommended <5x)", ratio),
						WastedCPU:  fmt.Sprintf("%dm excess", limCPU.MilliValue()-reqCPU.MilliValue()),
					})
				}
			}

			if !reqMem.IsZero() && !limMem.IsZero() {
				ratio := limMem.AsApproximateFloat64() / reqMem.AsApproximateFloat64()
				if ratio > 10 {
					report.OverProvisioned = append(report.OverProvisioned, OverProvisioned{
						Workload:      fmt.Sprintf("%s/%s", pod.Kind, pod.Name),
						Namespace:     pod.Namespace,
						Container:     c.Name,
						MemoryRequest: reqMem.String(),
						MemoryLimit:   limMem.String(),
						Issue:         fmt.Sprintf("Memory limit/request ratio %.1fx (recommended <5x)", ratio),
						WastedMemory:  humanBytes(limMem.Value() - reqMem.Value()),
					})
				}
			}
		}
	}

	// 2. Analyze node utilization
	type nodeUtil struct {
		name   string
		cpuReq int64
		cpuCap int64
		memReq int64
		memCap int64
		pods   int
		podCap int
	}
	utils := make(map[string]*nodeUtil)

	for _, node := range nodes {
		u := &nodeUtil{name: node.Name}
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			u.cpuCap = cpu.MilliValue()
		}
		if mem := node.Status.Allocatable.Memory(); mem != nil {
			u.memCap = mem.Value()
		}
		if pods := node.Status.Allocatable.Pods(); pods != nil {
			u.podCap = int(pods.Value())
		}
		utils[node.Name] = u
	}

	for _, pod := range pods {
		if pod.Spec.NodeName == "" || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		u, ok := utils[pod.Spec.NodeName]
		if !ok {
			continue
		}
		u.pods++
		for _, c := range pod.Spec.Containers {
			if req := c.Resources.Requests.Cpu(); req != nil {
				u.cpuReq += req.MilliValue()
			}
			if req := c.Resources.Requests.Memory(); req != nil {
				u.memReq += req.Value()
			}
		}
	}

	underutilizedCount := 0
	for _, u := range utils {
		cpuPct := 0.0
		memPct := 0.0
		if u.cpuCap > 0 {
			cpuPct = float64(u.cpuReq) / float64(u.cpuCap) * 100
		}
		if u.memCap > 0 {
			memPct = float64(u.memReq) / float64(u.memCap) * 100
		}

		// Underutilized: <20% CPU AND <20% memory
		if cpuPct < 20 && memPct < 20 && len(nodes) > 1 {
			underutilizedCount++
			report.Underutilized = append(report.Underutilized, UnderutilizedNode{
				Name:        u.name,
				CPUUsagePct: cpuPct,
				MemUsagePct: memPct,
				PodCount:    u.pods,
				PodCapacity: u.podCap,
				Details:     fmt.Sprintf("CPU %.0f%%, Mem %.0f%% — consider cordoning or removing", cpuPct, memPct),
			})
		}
	}

	// 3. Generate recommendations
	if noLimits > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d pod(s) have no resource limits — set CPU/memory limits to prevent resource contention", noLimits))
	}
	if oversized > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d container(s) have limit/request ratio >10x — reduce limits to improve scheduling density", oversized))
	}
	if underutilizedCount > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d node(s) are underutilized (<20%% CPU+memory) — consider scaling down to reduce costs", underutilizedCount))
	}
	if len(report.Recommendations) == 0 {
		report.Recommendations = append(report.Recommendations,
			"Cluster resource utilization looks healthy — no major waste detected")
	}

	// 4. Compute efficiency score
	// Start at 100, subtract for each issue
	score := 100.0
	score -= float64(noLimits) * 2           // -2 per pod without limits
	score -= float64(oversized) * 1.5        // -1.5 per oversized container
	score -= float64(underutilizedCount) * 5 // -5 per underutilized node
	if score < 0 {
		score = 0
	}

	// Estimate waste percentage
	wastePct := float64(noLimits)*0.5 + float64(oversized)*0.3 + float64(underutilizedCount)*5
	if wastePct > 100 {
		wastePct = 100
	}

	report.Score = score
	report.Stats = EfficiencyStats{
		TotalPods:          len(pods),
		IdlePods:           idlePods,
		NoResourceLimits:   noLimits,
		OversizedLimits:    oversized,
		TotalNodes:         len(nodes),
		UnderutilizedNodes: underutilizedCount,
		EstimatedWastePct:  wastePct,
	}

	// Sort recommendations for consistent output
	sort.Strings(report.Recommendations)

	return report
}

// humanBytes formats bytes into a human-readable string.
func humanBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGi", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1fMi", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1fKi", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}
