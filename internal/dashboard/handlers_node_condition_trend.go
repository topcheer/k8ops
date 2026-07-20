package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeConditionTrendResult tracks node condition flapping and stability.
type NodeConditionTrendResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         NodeCondTrendSummary `json:"summary"`
	Nodes           []NodeCondTrendEntry `json:"nodes"`
	UnstableNodes   []NodeCondTrendEntry `json:"unstableNodes"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type NodeCondTrendSummary struct {
	TotalNodes         int `json:"totalNodes"`
	ReadyNodes         int `json:"readyNodes"`
	NotReadyNodes      int `json:"notReadyNodes"`
	PressureNodes      int `json:"pressureNodes"`
	FlappingNodes      int `json:"flappingNodes"`
	DiskPressure       int `json:"diskPressureNodes"`
	MemPressure        int `json:"memoryPressureNodes"`
	PIDPressure        int `json:"pidPressureNodes"`
	NetworkUnavailable int `json:"networkUnavailableNodes"`
}

type NodeCondTrendEntry struct {
	Name       string   `json:"name"`
	Ready      bool     `json:"ready"`
	Conditions []string `json:"activeConditions"`
	Age        string   `json:"age"`
	Version    string   `json:"kubeletVersion"`
	PodCount   int      `json:"podCount"`
	RiskLevel  string   `json:"riskLevel"`
}

// handleNodeConditionTrend handles GET /api/operations/node-condition-trend
func (s *Server) handleNodeConditionTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := NodeConditionTrendResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nodePodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" {
			nodePodCount[pod.Spec.NodeName]++
		}
	}

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		entry := NodeCondTrendEntry{
			Name:     node.Name,
			Age:      time.Since(node.CreationTimestamp.Time).Round(time.Hour).String(),
			Version:  node.Status.NodeInfo.KubeletVersion,
			PodCount: nodePodCount[node.Name],
		}

		var activeConds []string
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeReady:
				entry.Ready = true
				result.Summary.ReadyNodes++
			case corev1.NodeDiskPressure:
				activeConds = append(activeConds, "DiskPressure")
				result.Summary.DiskPressure++
				result.Summary.PressureNodes++
			case corev1.NodeMemoryPressure:
				activeConds = append(activeConds, "MemoryPressure")
				result.Summary.MemPressure++
				result.Summary.PressureNodes++
			case corev1.NodePIDPressure:
				activeConds = append(activeConds, "PIDPressure")
				result.Summary.PIDPressure++
				result.Summary.PressureNodes++
			case corev1.NodeNetworkUnavailable:
				activeConds = append(activeConds, "NetworkUnavailable")
				result.Summary.NetworkUnavailable++
			}
		}

		if !entry.Ready {
			result.Summary.NotReadyNodes++
			activeConds = append(activeConds, "NotReady")
		}

		entry.Conditions = activeConds
		switch {
		case len(activeConds) >= 3:
			entry.RiskLevel = "critical"
			result.Summary.FlappingNodes++
			result.UnstableNodes = append(result.UnstableNodes, entry)
		case len(activeConds) >= 1:
			entry.RiskLevel = "high"
			result.UnstableNodes = append(result.UnstableNodes, entry)
		default:
			entry.RiskLevel = "low"
		}

		result.Nodes = append(result.Nodes, entry)
	}

	if result.Summary.TotalNodes > 0 {
		result.HealthScore = result.Summary.ReadyNodes * 100 / result.Summary.TotalNodes
		result.HealthScore -= result.Summary.PressureNodes * 10
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("节点状态趋势: %d 节点, %d 就绪, %d 未就绪, %d 压力, %d 磁盘, %d 内存, %d PID",
			result.Summary.TotalNodes, result.Summary.ReadyNodes,
			result.Summary.NotReadyNodes, result.Summary.PressureNodes,
			result.Summary.DiskPressure, result.Summary.MemPressure, result.Summary.PIDPressure),
	}
	if result.Summary.PressureNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个节点处于资源压力状态", result.Summary.PressureNodes))
	}
	writeJSON(w, result)
}
