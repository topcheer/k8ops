package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityGapResult identifies the gap between current cluster capacity and
// projected demand. It calculates node headroom, worst-case pod eviction
// capacity, and whether the cluster can survive node loss.
type CapacityGapResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CapacityGapSummary `json:"summary"`
	ByNode          []CapacityNodeGap  `json:"byNode"`
	Scenarios       []CapacityScenario `json:"scenarios"`
	RiskAssessment  CapacityRisk       `json:"riskAssessment"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type CapacityGapSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	TotalCPUCapacity float64 `json:"totalCPUCapacity"`
	TotalMemCapacity float64 `json:"totalMemCapacityGB"`
	UsedCPU          float64 `json:"usedCPU"`
	UsedMem          float64 `json:"usedMemGB"`
	CPUHeadroom      float64 `json:"cpuHeadroomPct"`
	MemHeadroom      float64 `json:"memHeadroomPct"`
	MaxPodCapacity   int     `json:"maxPodCapacity"`
	UsedPods         int     `json:"usedPods"`
	PodHeadroom      int     `json:"podHeadroom"`
}

type CapacityNodeGap struct {
	Node        string  `json:"node"`
	CPUCapacity float64 `json:"cpuCapacity"`
	CPUUsed     float64 `json:"cpuUsed"`
	CPUFree     float64 `json:"cpuFree"`
	MemCapacity float64 `json:"memCapacityGB"`
	MemUsed     float64 `json:"memUsedGB"`
	MemFree     float64 `json:"memFreeGB"`
	PodCapacity int     `json:"podCapacity"`
	PodsRunning int     `json:"podsRunning"`
}

type CapacityScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Survivable  bool   `json:"survivable"`
	Impact      string `json:"impact"`
	Severity    string `json:"severity"`
}

type CapacityRisk struct {
	NodeLossSurvivable bool   `json:"nodeLossSurvivable"`
	MaxEvictablePods   int    `json:"maxEvictablePods"`
	TimeToExhaustCPU   string `json:"timeToExhaustCPU"`
	TimeToExhaustMem   string `json:"timeToExhaustMem"`
	RiskLevel          string `json:"riskLevel"`
}

// handleCapacityGap handles GET /api/operations/capacity-gap
func (s *Server) handleCapacityGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CapacityGapResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	var nodeGaps []CapacityNodeGap
	totalCPUCap, totalMemCap, usedCPU, usedMem := 0.0, 0.0, 0.0, 0.0
	totalPodCap, usedPods := 0, 0
	workerCount := 0

	for _, node := range nodes.Items {
		isControlPlane := false
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			isControlPlane = true
		}
		if isControlPlane {
			continue
		}
		workerCount++

		cpuCap := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memCap := node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
		podCap := 0
		if v, ok := node.Status.Allocatable["pods"]; ok {
			podCap = int(v.AsApproximateFloat64())
		}

		nodeCPUUsed, nodeMemUsed, nodePods := 0.0, 0.0, 0
		for _, pod := range pods.Items {
			if pod.Spec.NodeName != node.Name {
				continue
			}
			nodePods++
			for _, c := range pod.Spec.Containers {
				if v, ok := c.Resources.Requests["cpu"]; ok {
					nodeCPUUsed += v.AsApproximateFloat64()
				}
				if v, ok := c.Resources.Requests["memory"]; ok {
					nodeMemUsed += v.AsApproximateFloat64() / 1e9
				}
			}
		}

		totalCPUCap += cpuCap
		totalMemCap += memCap
		usedCPU += nodeCPUUsed
		usedMem += nodeMemUsed
		totalPodCap += podCap
		usedPods += nodePods

		nodeGaps = append(nodeGaps, CapacityNodeGap{
			Node: node.Name, CPUCapacity: cpuCap, CPUUsed: nodeCPUUsed,
			CPUFree:     cpuCap - nodeCPUUsed,
			MemCapacity: memCap, MemUsed: nodeMemUsed,
			MemFree:     memCap - nodeMemUsed,
			PodCapacity: podCap, PodsRunning: nodePods,
		})
	}

	result.ByNode = nodeGaps
	result.Summary.TotalNodes = workerCount
	result.Summary.TotalCPUCapacity = totalCPUCap
	result.Summary.TotalMemCapacity = totalMemCap
	result.Summary.UsedCPU = usedCPU
	result.Summary.UsedMem = usedMem
	result.Summary.MaxPodCapacity = totalPodCap
	result.Summary.UsedPods = usedPods

	if totalCPUCap > 0 {
		result.Summary.CPUHeadroom = (totalCPUCap - usedCPU) / totalCPUCap * 100
	}
	if totalMemCap > 0 {
		result.Summary.MemHeadroom = (totalMemCap - usedMem) / totalMemCap * 100
	}
	result.Summary.PodHeadroom = totalPodCap - usedPods

	// Scenarios
	var scenarios []CapacityScenario
	if workerCount >= 2 {
		// Can we survive losing 1 node?
		perNodeCPU := totalCPUCap / float64(workerCount)
		perNodeMem := totalMemCap / float64(workerCount)
		afterLossCPU := totalCPUCap - perNodeCPU
		afterLossMem := totalMemCap - perNodeMem
		survivable := afterLossCPU >= usedCPU && afterLossMem >= usedMem

		scenarios = append(scenarios, CapacityScenario{
			Name: "单节点故障", Description: "失去 1 个工作节点后的容量",
			Survivable: survivable,
			Impact: fmt.Sprintf("剩余 CPU %.1f/%.1f cores, Mem %.1f/%.1f GB",
				afterLossCPU, usedCPU, afterLossMem, usedMem),
			Severity: sevFromBool(survivable),
		})
	} else {
		scenarios = append(scenarios, CapacityScenario{
			Name: "单节点故障", Description: "仅 1 个工作节点，无法容忍故障",
			Survivable: false, Impact: "全站中断", Severity: "critical",
		})
	}

	// Surge scenario: 2x current pods
	surgeCPU := usedCPU * 2
	surgeMem := usedMem * 2
	surgeSurvivable := surgeCPU <= totalCPUCap && surgeMem <= totalMemCap
	scenarios = append(scenarios, CapacityScenario{
		Name: "流量翻倍", Description: "Pod 数量翻倍后的容量需求",
		Survivable: surgeSurvivable,
		Impact: fmt.Sprintf("需要 CPU %.1f/%.1f, Mem %.1f/%.1f",
			surgeCPU, totalCPUCap, surgeMem, totalMemCap),
		Severity: sevFromBool(surgeSurvivable),
	})

	result.Scenarios = scenarios

	// Risk assessment
	result.RiskAssessment = CapacityRisk{
		NodeLossSurvivable: workerCount >= 2 && scenarios[0].Survivable,
		MaxEvictablePods:   result.Summary.PodHeadroom,
		TimeToExhaustCPU:   estimateExhaustion(usedCPU, totalCPUCap),
		TimeToExhaustMem:   estimateExhaustion(usedMem, totalMemCap),
	}
	if result.RiskAssessment.NodeLossSurvivable {
		result.RiskAssessment.RiskLevel = "low"
	} else {
		result.RiskAssessment.RiskLevel = "high"
	}

	// Score
	result.HealthScore = 0
	if result.Summary.CPUHeadroom > 30 {
		result.HealthScore += 30
	}
	if result.Summary.MemHeadroom > 30 {
		result.HealthScore += 30
	}
	if result.RiskAssessment.NodeLossSurvivable {
		result.HealthScore += 40
	}
	if result.Summary.PodHeadroom > 20 {
		result.HealthScore += 10
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildCapacityGapRecs(&result)
	writeJSON(w, result)
}

func sevFromBool(ok bool) string {
	if ok {
		return "low"
	}
	return "critical"
}

func estimateExhaustion(used, total float64) string {
	if used <= 0 || total <= 0 || used >= total {
		return "unknown"
	}
	growthRate := 0.05 // 5% monthly growth assumption
	months := 0
	current := used
	for current < total && months < 36 {
		current *= (1 + growthRate)
		months++
	}
	if months >= 36 {
		return ">36 months"
	}
	return fmt.Sprintf("~%d months", months)
}

func buildCapacityGapRecs(r *CapacityGapResult) []string {
	recs := []string{
		fmt.Sprintf("CPU 余量 %.0f%%, 内存余量 %.0f%%, Pod 余量 %d",
			r.Summary.CPUHeadroom, r.Summary.MemHeadroom, r.Summary.PodHeadroom),
	}
	if !r.RiskAssessment.NodeLossSurvivable {
		recs = append(recs, "集群无法容忍单节点故障，需要增加容量")
	}
	if r.Summary.CPUHeadroom < 20 {
		recs = append(recs, fmt.Sprintf("CPU 余量仅 %.0f%%，接近容量上限", r.Summary.CPUHeadroom))
	}
	if r.Summary.MemHeadroom < 20 {
		recs = append(recs, fmt.Sprintf("内存余量仅 %.0f%%，需要扩容", r.Summary.MemHeadroom))
	}
	for _, sc := range r.Scenarios {
		if !sc.Survivable {
			recs = append(recs, fmt.Sprintf("[%s] %s", sc.Name, sc.Impact))
		}
	}
	return recs
}
