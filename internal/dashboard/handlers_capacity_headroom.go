package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CHRResult is the cluster capacity headroom & scale-out readiness analysis.
type CHRResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         CHRSummary     `json:"summary"`
	ByNode          []CHRNodeEntry `json:"byNode"`
	BottleneckNodes []CHRNodeEntry `json:"bottleneckNodes"` // nodes that are "full"
	PodProfiles     []CHRPodFit    `json:"podProfiles"`     // how many pods of each size fit
	ScaleOutReady   CHRScaleOut    `json:"scaleOutReady"`
	Issues          []CHRIssue     `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// CHRSummary aggregates cluster capacity.
type CHRSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	SchedulableNodes int     `json:"schedulableNodes"`
	FullNodes        int     `json:"fullNodes"`    // <10% headroom on bottleneck resource
	TotalCPUmCPU     float64 `json:"totalCPUmCPU"` // allocatable
	TotalMemMB       float64 `json:"totalMemMB"`
	UsedCPUmCPU      float64 `json:"usedCPUmCPU"` // requested
	UsedMemMB        float64 `json:"usedMemMB"`
	FreeCPUmCPU      float64 `json:"freeCPUmCPU"`
	FreeMemMB        float64 `json:"freeMemMB"`
	CPUUtilization   float64 `json:"cpuUtilization"` // %
	MemUtilization   float64 `json:"memUtilization"` // %
	Bottleneck       string  `json:"bottleneck"`     // "cpu" / "memory" / "pods" / "none"
	HeadroomScore    int     `json:"headroomScore"`  // 0-100 (higher = more room)
	MaxPodSlots      int     `json:"maxPodSlots"`    // total pod capacity
	UsedPodSlots     int     `json:"usedPodSlots"`
}

// CHRNodeEntry describes one node's capacity headroom.
type CHRNodeEntry struct {
	Name               string  `json:"name"`
	IsSchedulable      bool    `json:"isSchedulable"`
	AllocatableCPUmCPU float64 `json:"allocatableCPUmCPU"`
	AllocatableMemMB   float64 `json:"allocatableMemMB"`
	UsedCPUmCPU        float64 `json:"usedCPUmCPU"`
	UsedMemMB          float64 `json:"usedMemMB"`
	FreeCPUmCPU        float64 `json:"freeCPUmCPU"`
	FreeMemMB          float64 `json:"freeMemMB"`
	CPUHeadroomPct     float64 `json:"cpuHeadroomPct"`
	MemHeadroomPct     float64 `json:"memHeadroomPct"`
	MaxPods            int     `json:"maxPods"`
	RunningPods        int     `json:"runningPods"`
	Bottleneck         string  `json:"bottleneck"` // resource that's most constrained
	IsFull             bool    `json:"isFull"`
	RiskLevel          string  `json:"riskLevel"`
}

// CHRPodFit estimates how many pods of a given profile fit.
type CHRPodFit struct {
	Profile        string  `json:"profile"` // small / medium / large
	CPUmCPU        float64 `json:"cpuReqmCPU"`
	MemMB          float64 `json:"memReqMB"`
	FitByCPU       int     `json:"fitByCPU"`  // how many fit by CPU
	FitByMem       int     `json:"fitByMem"`  // how many fit by memory
	FitByPods      int     `json:"fitByPods"` // how many fit by pod slots
	MaxFit         int     `json:"maxFit"`    // min of above
	LimitingFactor string  `json:"limitingFactor"`
}

// CHRScaleOut checks scale-out readiness.
type CHRScaleOut struct {
	HasClusterAutoscaler bool   `json:"hasClusterAutoscaler"`
	HasKarpenter         bool   `json:"hasKarpenter"`
	AutoscalerNS         string `json:"autoscalerNS,omitempty"`
	NeedsScaleOut        bool   `json:"needsScaleOut"`
	UrgencyLevel         string `json:"urgencyLevel"` // immediate / soon / no
}

// CHRIssue is a detected capacity problem.
type CHRIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleCapacityHeadroom analyzes cluster capacity and scale-out readiness.
// GET /api/scalability/capacity-headroom
func (s *Server) handleCapacityHeadroom(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
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

	// Check for Cluster Autoscaler / Karpenter
	autoscalerPods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cluster-autoscaler",
	})
	karpenterPods, _ := rc.clientset.CoreV1().Pods("karpenter").List(ctx, metav1.ListOptions{})

	result := CHRResult{ScannedAt: time.Now()}
	result.Summary.TotalNodes = len(nodes.Items)

	// Build node → pod resource map
	nodeUsage := make(map[string]chrUsage)
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}
		usage := nodeUsage[pod.Spec.NodeName]
		usage.podCount++
		for _, c := range pod.Spec.Containers {
			if req := c.Resources.Requests.Cpu(); req != nil {
				usage.cpuMilli += float64(req.MilliValue())
			}
			if req := c.Resources.Requests.Memory(); req != nil {
				usage.memMB += float64(req.Value()) / (1024 * 1024)
			}
		}
		nodeUsage[pod.Spec.NodeName] = usage
	}

	// Analyze each node
	for _, node := range nodes.Items {
		entry := CHRNodeEntry{
			Name:          node.Name,
			IsSchedulable: !node.Spec.Unschedulable,
		}

		entry.AllocatableCPUmCPU = float64(node.Status.Allocatable.Cpu().MilliValue())
		entry.AllocatableMemMB = float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024)
		entry.MaxPods = int(node.Status.Allocatable.Pods().Value())

		usage := nodeUsage[node.Name]
		entry.UsedCPUmCPU = usage.cpuMilli
		entry.UsedMemMB = usage.memMB
		entry.RunningPods = usage.podCount

		entry.FreeCPUmCPU = entry.AllocatableCPUmCPU - entry.UsedCPUmCPU
		entry.FreeMemMB = entry.AllocatableMemMB - entry.UsedMemMB

		if entry.AllocatableCPUmCPU > 0 {
			entry.CPUHeadroomPct = entry.FreeCPUmCPU / entry.AllocatableCPUmCPU * 100
		}
		if entry.AllocatableMemMB > 0 {
			entry.MemHeadroomPct = entry.FreeMemMB / entry.AllocatableMemMB * 100
		}

		// Bottleneck resource
		entry.Bottleneck = chrNodeBottleneck(entry)
		entry.IsFull = entry.CPUHeadroomPct < 10 || entry.MemHeadroomPct < 10

		if !node.Spec.Unschedulable {
			result.Summary.SchedulableNodes++
		}

		// Aggregate
		result.Summary.TotalCPUmCPU += entry.AllocatableCPUmCPU
		result.Summary.TotalMemMB += entry.AllocatableMemMB
		result.Summary.UsedCPUmCPU += entry.UsedCPUmCPU
		result.Summary.UsedMemMB += entry.UsedMemMB
		result.Summary.MaxPodSlots += entry.MaxPods
		result.Summary.UsedPodSlots += entry.RunningPods

		entry.RiskLevel = chrNodeRisk(entry)
		result.ByNode = append(result.ByNode, entry)

		if entry.IsFull && entry.IsSchedulable {
			result.Summary.FullNodes++
			result.BottleneckNodes = append(result.BottleneckNodes, entry)
			result.Issues = append(result.Issues, CHRIssue{
				Severity: "warning", Type: "full-node",
				Resource: node.Name,
				Message:  fmt.Sprintf("Node %s is near capacity — %.0f%% CPU, %.0f%% memory used, %d/%d pods", node.Name, 100-entry.CPUHeadroomPct, 100-entry.MemHeadroomPct, entry.RunningPods, entry.MaxPods),
			})
		}
	}

	// Cluster-wide headroom
	result.Summary.FreeCPUmCPU = result.Summary.TotalCPUmCPU - result.Summary.UsedCPUmCPU
	result.Summary.FreeMemMB = result.Summary.TotalMemMB - result.Summary.UsedMemMB

	if result.Summary.TotalCPUmCPU > 0 {
		result.Summary.CPUUtilization = result.Summary.UsedCPUmCPU / result.Summary.TotalCPUmCPU * 100
	}
	if result.Summary.TotalMemMB > 0 {
		result.Summary.MemUtilization = result.Summary.UsedMemMB / result.Summary.TotalMemMB * 100
	}
	result.Summary.Bottleneck = chrClusterBottleneck(result.Summary)

	// Pod profiles: how many of each size can fit?
	profiles := []chrProfile{
		{"small", 100, 128},    // 100m CPU, 128MB
		{"medium", 500, 512},   // 500m CPU, 512MB
		{"large", 1000, 1024},  // 1 core, 1GB
		{"xlarge", 2000, 4096}, // 2 cores, 4GB
	}

	for _, p := range profiles {
		fit := CHRPodFit{
			Profile: p.name,
			CPUmCPU: p.cpuMilli,
			MemMB:   p.memMB,
		}

		if p.cpuMilli > 0 {
			fit.FitByCPU = int(result.Summary.FreeCPUmCPU / p.cpuMilli)
		} else {
			fit.FitByCPU = 9999
		}
		if p.memMB > 0 {
			fit.FitByMem = int(result.Summary.FreeMemMB / p.memMB)
		} else {
			fit.FitByMem = 9999
		}
		freePodSlots := result.Summary.MaxPodSlots - result.Summary.UsedPodSlots
		fit.FitByPods = freePodSlots

		fit.MaxFit = min3int(fit.FitByCPU, fit.FitByMem, fit.FitByPods)
		fit.LimitingFactor = chrLimitingFactor(fit)

		result.PodProfiles = append(result.PodProfiles, fit)
	}

	// Scale-out readiness
	result.ScaleOutReady = CHRScaleOut{
		HasClusterAutoscaler: len(autoscalerPods.Items) > 0,
		HasKarpenter:         len(karpenterPods.Items) > 0,
	}
	if result.ScaleOutReady.HasClusterAutoscaler {
		result.ScaleOutReady.AutoscalerNS = "kube-system"
	} else if result.ScaleOutReady.HasKarpenter {
		result.ScaleOutReady.AutoscalerNS = "karpenter"
	}

	// Urgency: if headroom < 15%, need immediate scale-out
	headroomPct := float64(result.Summary.HeadroomScore)
	result.ScaleOutReady.NeedsScaleOut = headroomPct < 20
	if headroomPct < 10 {
		result.ScaleOutReady.UrgencyLevel = "immediate"
	} else if headroomPct < 20 {
		result.ScaleOutReady.UrgencyLevel = "soon"
	} else {
		result.ScaleOutReady.UrgencyLevel = "no"
	}

	// Score
	result.Summary.HeadroomScore = chrScore(result.Summary)

	// Sort
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].CPUHeadroomPct < result.ByNode[j].CPUHeadroomPct
	})
	sort.Slice(result.BottleneckNodes, func(i, j int) bool {
		return result.BottleneckNodes[i].CPUHeadroomPct < result.BottleneckNodes[j].CPUHeadroomPct
	})

	result.Recommendations = chrGenRecs(result.Summary, result.ScaleOutReady, result.PodProfiles, result.BottleneckNodes)

	writeJSON(w, result)
}

type chrUsage struct {
	cpuMilli float64
	memMB    float64
	podCount int
}

type chrProfile struct {
	name     string
	cpuMilli float64
	memMB    float64
}

func chrNodeBottleneck(entry CHRNodeEntry) string {
	cpuUsed := 100 - entry.CPUHeadroomPct
	memUsed := 100 - entry.MemHeadroomPct
	podUsed := 0.0
	if entry.MaxPods > 0 {
		podUsed = float64(entry.RunningPods) / float64(entry.MaxPods) * 100
	}

	maxUsed := cpuUsed
	bottleneck := "cpu"
	if memUsed > maxUsed {
		maxUsed = memUsed
		bottleneck = "memory"
	}
	if podUsed > maxUsed {
		bottleneck = "pods"
	}
	return bottleneck
}

func chrNodeRisk(entry CHRNodeEntry) string {
	minHeadroom := entry.CPUHeadroomPct
	if entry.MemHeadroomPct < minHeadroom {
		minHeadroom = entry.MemHeadroomPct
	}
	switch {
	case minHeadroom < 10:
		return "critical"
	case minHeadroom < 25:
		return "high"
	case minHeadroom < 50:
		return "medium"
	default:
		return "low"
	}
}

func chrClusterBottleneck(s CHRSummary) string {
	cpuFree := 100 - s.CPUUtilization
	memFree := 100 - s.MemUtilization
	podFree := 100.0
	if s.MaxPodSlots > 0 {
		podFree = float64(s.MaxPodSlots-s.UsedPodSlots) / float64(s.MaxPodSlots) * 100
	}

	minFree := cpuFree
	bottleneck := "cpu"
	if memFree < minFree {
		minFree = memFree
		bottleneck = "memory"
	}
	if podFree < minFree {
		bottleneck = "pods"
	}
	return bottleneck
}

func chrScore(s CHRSummary) int {
	cpuFree := 100 - s.CPUUtilization
	memFree := 100 - s.MemUtilization
	podFree := 100.0
	if s.MaxPodSlots > 0 {
		podFree = float64(s.MaxPodSlots-s.UsedPodSlots) / float64(s.MaxPodSlots) * 100
	}

	// Headroom score = min of free percentages
	score := int(min3(cpuFree, memFree, podFree))
	if score < 0 {
		score = 0
	}
	return score
}

func chrLimitingFactor(fit CHRPodFit) string {
	if fit.MaxFit == fit.FitByCPU {
		return "cpu"
	}
	if fit.MaxFit == fit.FitByMem {
		return "memory"
	}
	return "pod-slots"
}

func chrGenRecs(s CHRSummary, scaleOut CHRScaleOut, profiles []CHRPodFit, bottlenecks []CHRNodeEntry) []string {
	var recs []string

	if len(profiles) > 0 {
		medium := profiles[1] // medium profile
		recs = append(recs, fmt.Sprintf("Cluster can schedule ~%d additional medium pods (%dm CPU, %dMB mem) before hitting %s limit", medium.MaxFit, int(medium.CPUmCPU), int(medium.MemMB), medium.LimitingFactor))
	}

	if s.FullNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are near capacity (<10%% headroom) — pods will fail to schedule on these nodes", s.FullNodes))
	}

	if s.CPUUtilization > 80 {
		recs = append(recs, fmt.Sprintf("CPU utilization is %.0f%% — approaching cluster limit, consider adding nodes or right-sizing workloads", s.CPUUtilization))
	}
	if s.MemUtilization > 80 {
		recs = append(recs, fmt.Sprintf("Memory utilization is %.0f%% — approaching cluster limit, consider adding nodes or reducing memory limits", s.MemUtilization))
	}

	if scaleOut.NeedsScaleOut {
		if !scaleOut.HasClusterAutoscaler && !scaleOut.HasKarpenter {
			recs = append(recs, fmt.Sprintf("Cluster needs scale-out (headroom %d%%) but NO autoscaler detected — install Cluster Autoscaler or Karpenter for automatic node provisioning", s.HeadroomScore))
		} else {
			tool := "Cluster Autoscaler"
			if scaleOut.HasKarpenter {
				tool = "Karpenter"
			}
			recs = append(recs, fmt.Sprintf("Cluster needs scale-out (headroom %d%%) — %s will provision new nodes automatically", s.HeadroomScore, tool))
		}
	}

	if s.HeadroomScore < 15 {
		recs = append(recs, fmt.Sprintf("CRITICAL: cluster headroom is only %d%% — immediate node addition required to avoid scheduling failures", s.HeadroomScore))
	}

	if len(bottlenecks) > 0 {
		top := bottlenecks[0]
		recs = append(recs, fmt.Sprintf("Most constrained node: %s (%.1f%% CPU headroom, %.1f%% memory headroom)", top.Name, top.CPUHeadroomPct, top.MemHeadroomPct))
	}

	if s.HeadroomScore > 50 && s.FullNodes == 0 {
		recs = append(recs, fmt.Sprintf("Cluster has healthy headroom (%d%%) — no immediate capacity concerns", s.HeadroomScore))
	}

	return recs
}

func min3(a, b, c float64) float64 {
	min := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	return min
}

func min3int(a, b, c int) int {
	min := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	return min
}

// Ensure resource import is used
var _ = resource.Quantity{}
