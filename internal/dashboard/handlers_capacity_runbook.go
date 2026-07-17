package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityRunbookResult auto-generates capacity planning documentation based
// on current cluster resource state, growth trends, and risk factors.
type CapacityRunbookResult struct {
	ScannedAt        time.Time                 `json:"scannedAt"`
	RunbookVersion   string                    `json:"runbookVersion"`
	Summary          CapacityRunbookSummary    `json:"summary"`
	Sections         []CapacityRunbookSection  `json:"sections"`
	HeadroomAnalysis CapacityRunbookHeadroom   `json:"headroomAnalysis"`
	GrowthProjection CapacityRunbookProjection `json:"growthProjection"`
	EmergencyRunbook []string                  `json:"emergencyRunbook"`
	CapacityScore    int                       `json:"capacityScore"`
	Grade            string                    `json:"grade"`
	Recommendations  []string                  `json:"recommendations"`
}

type CapacityRunbookSummary struct {
	ClusterName   string  `json:"clusterName"`
	NodeCount     int     `json:"nodeCount"`
	TotalCPUCores float64 `json:"totalCPUCores"`
	TotalMemoryGB float64 `json:"totalMemoryGB"`
	UsedCPUCores  float64 `json:"usedCPUCores"`
	UsedMemoryGB  float64 `json:"usedMemoryGB"`
	HeadroomCPU   float64 `json:"headroomCPUPct"`
	HeadroomMem   float64 `json:"headroomMemPct"`
	HeadroomPods  int     `json:"headroomPods"`
}

type CapacityRunbookSection struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Priority string `json:"priority"`
}

type CapacityRunbookHeadroom struct {
	CPUFreeCores    float64 `json:"cpuFreeCores"`
	MemFreeGB       float64 `json:"memFreeGB"`
	PodSlotsFree    int     `json:"podSlotsFree"`
	LargestPodCPU   float64 `json:"largestPodScheduleCPU"`
	LargestPodMem   float64 `json:"largestPodScheduleMemGB"`
	NewPodsFittable int     `json:"newPodsFittable"`
	Bottleneck      string  `json:"bottleneck"`
}

type CapacityRunbookProjection struct {
	CurrentPods       int    `json:"currentPods"`
	ProjectedPods30d  int    `json:"projectedPods30d"`
	CPUExhaustionDays int    `json:"cpuExhaustionDays"`
	MemExhaustionDays int    `json:"memExhaustionDays"`
	ScaleUrgency      string `json:"scaleUrgency"`
}

// handleCapacityRunbook handles GET /api/docs/capacity-runbook
func (s *Server) handleCapacityRunbook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CapacityRunbookResult{
		ScannedAt:      time.Now(),
		RunbookVersion: "1.0",
	}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Calculate totals
	totalCPU := 0.0
	totalMemGB := 0.0
	totalPodCapacity := 0
	workerNodes := 0
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		workerNodes++
		totalCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		totalMemGB += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		// Pod capacity: typically 110 per node
		if pods, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			totalPodCapacity += int(pods.Value())
		}
	}

	usedCPU := 0.0
	usedMemGB := 0.0
	runningPods := 0
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		runningPods++
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				usedCPU += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				usedMemGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
	}

	freeCPU := totalCPU - usedCPU
	if freeCPU < 0 {
		freeCPU = 0
	}
	freeMemGB := totalMemGB - usedMemGB
	if freeMemGB < 0 {
		freeMemGB = 0
	}
	freePods := totalPodCapacity - runningPods
	if freePods < 0 {
		freePods = 0
	}

	headroomCPU := safeDivPct(usedCPU, totalCPU)
	headroomMem := safeDivPct(usedMemGB, totalMemGB)

	result.Summary = CapacityRunbookSummary{
		NodeCount:     workerNodes,
		TotalCPUCores: totalCPU,
		TotalMemoryGB: totalMemGB,
		UsedCPUCores:  usedCPU,
		UsedMemoryGB:  usedMemGB,
		HeadroomCPU:   100 - headroomCPU,
		HeadroomMem:   100 - headroomMem,
		HeadroomPods:  freePods,
	}

	// Headroom analysis
	avgPodCPU := 0.0
	avgPodMem := 0.0
	if runningPods > 0 {
		avgPodCPU = usedCPU / float64(runningPods)
		avgPodMem = usedMemGB / float64(runningPods)
	}
	newPods := 0
	if avgPodCPU > 0 {
		newPods = int(freeCPU / avgPodCPU)
	}
	result.HeadroomAnalysis = CapacityRunbookHeadroom{
		CPUFreeCores:    freeCPU,
		MemFreeGB:       freeMemGB,
		PodSlotsFree:    freePods,
		LargestPodCPU:   avgPodCPU,
		LargestPodMem:   avgPodMem,
		NewPodsFittable: newPods,
	}
	if freeCPU/totalCPU < freeMemGB/totalMemGB {
		result.HeadroomAnalysis.Bottleneck = "CPU"
	} else {
		result.HeadroomAnalysis.Bottleneck = "Memory"
	}

	// Growth projection (simplified: assume 5% monthly growth)
	projected30d := int(float64(runningPods) * 1.05)
	cpuExhaustDays := 999
	memExhaustDays := 999
	if usedCPU > 0 && totalCPU > 0 && freeCPU > 0 {
		dailyGrowth := usedCPU * 0.05 / 30
		if dailyGrowth > 0 {
			cpuExhaustDays = int(freeCPU / dailyGrowth)
		}
	}
	if usedMemGB > 0 && totalMemGB > 0 && freeMemGB > 0 {
		dailyGrowth := usedMemGB * 0.05 / 30
		if dailyGrowth > 0 {
			memExhaustDays = int(freeMemGB / dailyGrowth)
		}
	}

	urgency := "normal"
	if cpuExhaustDays < 30 || memExhaustDays < 30 {
		urgency = "urgent"
	}
	if cpuExhaustDays < 7 || memExhaustDays < 7 {
		urgency = "immediate"
	}

	result.GrowthProjection = CapacityRunbookProjection{
		CurrentPods:       runningPods,
		ProjectedPods30d:  projected30d,
		CPUExhaustionDays: cpuExhaustDays,
		MemExhaustionDays: memExhaustDays,
		ScaleUrgency:      urgency,
	}

	// Generate runbook sections
	result.Sections = []CapacityRunbookSection{
		{
			Title:    "集群概况",
			Content:  fmt.Sprintf("集群包含 %d 个 worker 节点, 总计 %.1f CPU 核和 %.1f GB 内存。当前运行 %d 个 Pod, 使用 %.1f CPU (%.0f%%) 和 %.1f GB 内存 (%.0f%%)。", workerNodes, totalCPU, totalMemGB, runningPods, usedCPU, headroomCPU, usedMemGB, headroomMem),
			Priority: "info",
		},
		{
			Title:    "容量余量",
			Content:  fmt.Sprintf("可用: %.1f CPU 核, %.1f GB 内存, %d Pod 槽位。瓶颈资源: %s。预估可调度 %d 个新 Pod (平均大小)。", freeCPU, freeMemGB, freePods, result.HeadroomAnalysis.Bottleneck, newPods),
			Priority: "high",
		},
		{
			Title:    "增长预测",
			Content:  fmt.Sprintf("按 5%% 月增长率, 30 天后预计 %d 个 Pod。CPU 耗尽: %d 天, 内存耗尽: %d 天。扩展紧迫度: %s。", projected30d, cpuExhaustDays, memExhaustDays, urgency),
			Priority: "critical",
		},
	}

	// Emergency runbook steps
	result.EmergencyRunbook = []string{
		fmt.Sprintf("1. 如果 CPU 使用率 > 90%%: 立即扩容节点或缩减低优先级工作负载"),
		fmt.Sprintf("2. 如果内存使用率 > 90%%: 检查 OOM 历史, 增加节点内存"),
		fmt.Sprintf("3. 如果 Pod 调度失败: 检查节点资源碎片, 考虑 compact 重调度"),
		fmt.Sprintf("4. 当前瓶颈资源: %s - 优先监控和扩容此资源", result.HeadroomAnalysis.Bottleneck),
		fmt.Sprintf("5. 扩展紧迫度: %s (CPU 剩余 %d 天, 内存剩余 %d 天)", urgency, cpuExhaustDays, memExhaustDays),
	}

	// Capacity score
	result.CapacityScore = int((freeCPU/maxFloat(totalCPU, 1) + freeMemGB/maxFloat(totalMemGB, 1)) / 2 * 100)

	switch {
	case result.CapacityScore >= 50:
		result.Grade = "A"
	case result.CapacityScore >= 30:
		result.Grade = "B"
	case result.CapacityScore >= 15:
		result.Grade = "C"
	case result.CapacityScore >= 5:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildCapacityRunbookRecs(&result)
	writeJSON(w, result)
}

func safeDivPct(used, total float64) float64 {
	if total == 0 {
		return 0
	}
	return used / total * 100
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func buildCapacityRunbookRecs(r *CapacityRunbookResult) []string {
	recs := []string{
		fmt.Sprintf("容量评分: %d/100 (%s), 余量: %.1f%% CPU, %.1f%% 内存", r.CapacityScore, r.Grade, r.Summary.HeadroomCPU, r.Summary.HeadroomMem),
	}
	if r.GrowthProjection.ScaleUrgency != "normal" {
		recs = append(recs, fmt.Sprintf("扩容紧迫度: %s (CPU 剩余 %d 天, 内存剩余 %d 天)", r.GrowthProjection.ScaleUrgency, r.GrowthProjection.CPUExhaustionDays, r.GrowthProjection.MemExhaustionDays))
	}
	recs = append(recs, fmt.Sprintf("瓶颈资源: %s, 可调度新 Pod: %d", r.HeadroomAnalysis.Bottleneck, r.HeadroomAnalysis.NewPodsFittable))
	if r.CapacityScore < 30 {
		recs = append(recs, "建议: 预留 20% 资源余量, 配置 Cluster Autoscaler 或 Karpenter")
	}
	recs = append(recs, fmt.Sprintf("Emergency Runbook 包含 %d 步操作流程", len(r.EmergencyRunbook)))
	return recs
}
