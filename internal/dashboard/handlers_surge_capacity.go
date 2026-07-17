package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SurgeCapacityResult checks whether the cluster has enough resources to
// absorb maxSurge replicas during rolling updates of Deployments and StatefulSets.
type SurgeCapacityResult struct {
	ScannedAt        time.Time            `json:"scannedAt"`
	Summary          SurgeCapacitySummary `json:"summary"`
	ByWorkload       []SurgeEntry         `json:"byWorkload"`
	BlockedWorkloads []SurgeEntry         `json:"blockedWorkloads"`
	SurgeScore       int                  `json:"surgeScore"`
	Grade            string               `json:"grade"`
	Recommendations  []string             `json:"recommendations"`
}

type SurgeCapacitySummary struct {
	TotalWorkloads  int     `json:"totalWorkloads"`
	CanSurge        int     `json:"canSurge"`
	InsufficientCPU int     `json:"insufficientCPU"`
	InsufficientMem int     `json:"insufficientMem"`
	NoSurge         int     `json:"noSurgeConfig"`
	TotalSurgeCPU   float64 `json:"totalSurgeCpuNeeded"`
	TotalSurgeMemGB float64 `json:"totalSurgeMemNeededGB"`
	AvailableCPU    float64 `json:"availableClusterCpu"`
	AvailableMemGB  float64 `json:"availableClusterMemGB"`
}

type SurgeEntry struct {
	Workload    string  `json:"workload"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	Replicas    int     `json:"replicas"`
	MaxSurge    int     `json:"maxSurge"`
	PerPodCPU   float64 `json:"perPodCpu"`
	PerPodMemGB float64 `json:"perPodMemGB"`
	SurgeCPU    float64 `json:"surgeCpuNeeded"`
	SurgeMemGB  float64 `json:"surgeMemNeededGB"`
	CanDeploy   bool    `json:"canDeploy"`
	Blocker     string  `json:"blocker"`
}

// handleSurgeCapacity handles GET /api/deployment/surge-capacity
func (s *Server) handleSurgeCapacity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SurgeCapacityResult{ScannedAt: time.Now()}

	// Calculate cluster-wide available resources
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	totalAllocatableCPU := 0.0
	totalAllocatableMemGB := 0.0
	usedCPU := 0.0
	usedMemGB := 0.0

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		// Skip control-plane
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		totalAllocatableCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		totalAllocatableMemGB += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				usedCPU += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				usedMemGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
	}

	availableCPU := totalAllocatableCPU - usedCPU
	if availableCPU < 0 {
		availableCPU = 0
	}
	availableMemGB := totalAllocatableMemGB - usedMemGB
	if availableMemGB < 0 {
		availableMemGB = 0
	}

	result.Summary.AvailableCPU = availableCPU
	result.Summary.AvailableMemGB = availableMemGB

	// Analyze Deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		entry := analyzeSurgeDeployment(&d, availableCPU, availableMemGB)
		result.Summary.TotalWorkloads++
		result.Summary.TotalSurgeCPU += entry.SurgeCPU
		result.Summary.TotalSurgeMemGB += entry.SurgeMemGB

		if entry.MaxSurge == 0 {
			result.Summary.NoSurge++
		}
		if entry.CanDeploy {
			result.Summary.CanSurge++
		} else {
			result.BlockedWorkloads = append(result.BlockedWorkloads, entry)
			if entry.Blocker == "insufficient-cpu" {
				result.Summary.InsufficientCPU++
			}
			if entry.Blocker == "insufficient-mem" {
				result.Summary.InsufficientMem++
			}
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Analyze StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, s := range sts.Items {
		if isSystemNamespace(s.Namespace) {
			continue
		}
		entry := analyzeSurgeStatefulSet(&s, availableCPU, availableMemGB)
		result.Summary.TotalWorkloads++
		result.Summary.TotalSurgeCPU += entry.SurgeCPU
		result.Summary.TotalSurgeMemGB += entry.SurgeMemGB

		if entry.MaxSurge == 0 {
			result.Summary.NoSurge++
		}
		if entry.CanDeploy {
			result.Summary.CanSurge++
		} else {
			result.BlockedWorkloads = append(result.BlockedWorkloads, entry)
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort by surge CPU needed descending
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].SurgeCPU > result.ByWorkload[j].SurgeCPU
	})

	// Surge score: ratio of can-surge workloads
	if result.Summary.TotalWorkloads > 0 {
		result.SurgeScore = result.Summary.CanSurge * 100 / result.Summary.TotalWorkloads
	}

	switch {
	case result.SurgeScore >= 90:
		result.Grade = "A"
	case result.SurgeScore >= 70:
		result.Grade = "B"
	case result.SurgeScore >= 50:
		result.Grade = "C"
	case result.SurgeScore >= 30:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildSurgeCapacityRecs(&result)
	writeJSON(w, result)
}

func analyzeSurgeDeployment(d *appsv1.Deployment, availCPU, availMem float64) SurgeEntry {
	replicas := 1
	if d.Spec.Replicas != nil {
		replicas = int(*d.Spec.Replicas)
	}

	maxSurge := 1 // default 25% rounded up
	if d.Spec.Strategy.RollingUpdate != nil && d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
		maxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()
	}

	// Calculate per-pod resources
	perPodCPU := 0.0
	perPodMemGB := 0.0
	for _, c := range d.Spec.Template.Spec.Containers {
		if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			perPodCPU += req.AsApproximateFloat64()
		}
		if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			perPodMemGB += float64(req.Value()) / (1024 * 1024 * 1024)
		}
	}

	surgeCPU := perPodCPU * float64(maxSurge)
	surgeMemGB := perPodMemGB * float64(maxSurge)

	entry := SurgeEntry{
		Workload:    d.Name,
		Namespace:   d.Namespace,
		Kind:        "Deployment",
		Replicas:    replicas,
		MaxSurge:    maxSurge,
		PerPodCPU:   perPodCPU,
		PerPodMemGB: perPodMemGB,
		SurgeCPU:    surgeCPU,
		SurgeMemGB:  surgeMemGB,
		CanDeploy:   true,
	}

	if surgeCPU > availCPU {
		entry.CanDeploy = false
		entry.Blocker = "insufficient-cpu"
	} else if surgeMemGB > availMem {
		entry.CanDeploy = false
		entry.Blocker = "insufficient-mem"
	}

	return entry
}

func analyzeSurgeStatefulSet(s *appsv1.StatefulSet, availCPU, availMem float64) SurgeEntry {
	replicas := 1
	if s.Spec.Replicas != nil {
		replicas = int(*s.Spec.Replicas)
	}

	// StatefulSets use partition for rolling updates; surge is typically 1
	maxSurge := 1

	perPodCPU := 0.0
	perPodMemGB := 0.0
	for _, c := range s.Spec.Template.Spec.Containers {
		if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			perPodCPU += req.AsApproximateFloat64()
		}
		if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			perPodMemGB += float64(req.Value()) / (1024 * 1024 * 1024)
		}
	}

	surgeCPU := perPodCPU * float64(maxSurge)
	surgeMemGB := perPodMemGB * float64(maxSurge)

	entry := SurgeEntry{
		Workload:    s.Name,
		Namespace:   s.Namespace,
		Kind:        "StatefulSet",
		Replicas:    replicas,
		MaxSurge:    maxSurge,
		PerPodCPU:   perPodCPU,
		PerPodMemGB: perPodMemGB,
		SurgeCPU:    surgeCPU,
		SurgeMemGB:  surgeMemGB,
		CanDeploy:   true,
	}

	if surgeCPU > availCPU {
		entry.CanDeploy = false
		entry.Blocker = "insufficient-cpu"
	} else if surgeMemGB > availMem {
		entry.CanDeploy = false
		entry.Blocker = "insufficient-mem"
	}

	return entry
}

func buildSurgeCapacityRecs(r *SurgeCapacityResult) []string {
	recs := []string{
		fmt.Sprintf("集群可用: %.1f CPU 核, %.1f GB 内存", r.Summary.AvailableCPU, r.Summary.AvailableMemGB),
	}
	if r.Summary.InsufficientCPU > 0 || r.Summary.InsufficientMem > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个工作负载滚动更新时资源不足", r.Summary.InsufficientCPU+r.Summary.InsufficientMem))
	}
	if len(r.BlockedWorkloads) > 0 {
		recs = append(recs, fmt.Sprintf("最高风险: %s 需要 %.2f CPU", r.BlockedWorkloads[0].Workload, r.BlockedWorkloads[0].SurgeCPU))
	}
	if r.Summary.NoSurge > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载未配置 maxSurge，将使用默认值", r.Summary.NoSurge))
	}
	if r.SurgeScore < 50 {
		recs = append(recs, "建议: 扩展节点或降低 maxSurge 值以避免部署阻塞")
	}
	return recs
}
