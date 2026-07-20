package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeLifeForecastResult predicts node lifecycle events (upgrade needed,
// replacement due, capacity exhaustion) based on age, health, and pressure.
type NodeLifeForecastResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         NodeLifeForecastSummary `json:"summary"`
	ByNode          []NodeLifeEntry         `json:"byNode"`
	ActionNeeded    []NodeLifeEntry         `json:"actionNeeded"`
	ForecastScore   int                     `json:"forecastScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type NodeLifeForecastSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	HealthyNodes     int     `json:"healthyNodes"`
	AgingNodes       int     `json:"agingNodes"`
	PressuredNodes   int     `json:"pressuredNodes"`
	AvgAgeDays       float64 `json:"avgAgeDays"`
	NodesNeedReplace int     `json:"nodesNeedReplacement"`
	NodesNeedUpgrade int     `json:"nodesNeedUpgrade"`
	ForecastHorizon  string  `json:"forecastHorizon"`
}

type NodeLifeEntry struct {
	NodeName       string   `json:"nodeName"`
	Role           string   `json:"role"`
	AgeDays        int      `json:"ageDays"`
	KubeletVersion string   `json:"kubeletVersion"`
	OSImage        string   `json:"osImage"`
	HasPressure    bool     `json:"hasPressure"`
	PressureTypes  []string `json:"pressureTypes"`
	PodCount       int      `json:"podCount"`
	HealthStatus   string   `json:"healthStatus"`
	ActionNeeded   string   `json:"actionNeeded"`
	Priority       string   `json:"priority"`
	ForecastAction string   `json:"forecastAction"`
	EstTimeline    string   `json:"estimatedTimeline"`
}

// handleNodeLifeForecast handles GET /api/scalability/node-life-forecast
func (s *Server) handleNodeLifeForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeLifeForecastResult{ScannedAt: time.Now()}
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod-per-node counts
	nodePodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			nodePodCount[pod.Spec.NodeName]++
		}
	}

	now := time.Now()
	totalAge := 0
	ageCount := 0

	for _, node := range nodes.Items {
		entry := NodeLifeEntry{
			NodeName:       node.Name,
			KubeletVersion: node.Status.NodeInfo.KubeletVersion,
			OSImage:        node.Status.NodeInfo.OSImage,
		}

		// Role
		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			role = "master"
		}
		entry.Role = role

		// Age
		ageDays := int(now.Sub(node.CreationTimestamp.Time).Hours() / 24)
		if ageDays < 0 {
			ageDays = 0
		}
		entry.AgeDays = ageDays
		totalAge += ageDays
		ageCount++

		// Pod count
		entry.PodCount = nodePodCount[node.Name]

		// Check conditions
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				entry.HasPressure = true
				entry.PressureTypes = append(entry.PressureTypes, "Memory")
			case corev1.NodeDiskPressure:
				entry.HasPressure = true
				entry.PressureTypes = append(entry.PressureTypes, "Disk")
			case corev1.NodePIDPressure:
				entry.HasPressure = true
				entry.PressureTypes = append(entry.PressureTypes, "PID")
			}
		}

		// Health status
		switch {
		case entry.HasPressure:
			entry.HealthStatus = "pressured"
			result.Summary.PressuredNodes++
		case ageDays > 365:
			entry.HealthStatus = "aging"
			result.Summary.AgingNodes++
		default:
			entry.HealthStatus = "healthy"
			result.Summary.HealthyNodes++
		}

		// Action needed
		switch {
		case entry.HasPressure:
			entry.ActionNeeded = "investigate-pressure"
			entry.Priority = "critical"
			entry.ForecastAction = "Drain and investigate node pressure"
			entry.EstTimeline = "immediate"
			result.Summary.NodesNeedReplace++
		case ageDays > 730: // > 2 years
			entry.ActionNeeded = "replace"
			entry.Priority = "high"
			entry.ForecastAction = "Plan node replacement (end of life)"
			entry.EstTimeline = "1-3 months"
			result.Summary.NodesNeedReplace++
		case ageDays > 365: // > 1 year
			entry.ActionNeeded = "upgrade"
			entry.Priority = "medium"
			entry.ForecastAction = "Schedule OS/kubelet upgrade"
			entry.EstTimeline = "3-6 months"
			result.Summary.NodesNeedUpgrade++
		default:
			entry.ActionNeeded = "none"
			entry.Priority = "low"
			entry.ForecastAction = "Continue monitoring"
			entry.EstTimeline = "routine"
		}

		result.Summary.TotalNodes++
		result.ByNode = append(result.ByNode, entry)
		if entry.ActionNeeded != "none" {
			result.ActionNeeded = append(result.ActionNeeded, entry)
		}
	}

	if ageCount > 0 {
		result.Summary.AvgAgeDays = float64(totalAge) / float64(ageCount)
	}
	result.Summary.ForecastHorizon = "6 months"

	// Sort by priority
	sort.Slice(result.ActionNeeded, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ActionNeeded[i].Priority] < rank[result.ActionNeeded[j].Priority]
	})

	if result.Summary.TotalNodes > 0 {
		healthyRatio := float64(result.Summary.HealthyNodes) / float64(result.Summary.TotalNodes)
		result.ForecastScore = int(healthyRatio * 100)
	}
	gradeFromScore(&result.Grade, result.ForecastScore)

	result.Recommendations = []string{
		fmt.Sprintf("节点生命周期预测: %d 节点, %d 健康, %d 老化, %d 压力, 平均 %.0f 天", result.Summary.TotalNodes, result.Summary.HealthyNodes, result.Summary.AgingNodes, result.Summary.PressuredNodes, result.Summary.AvgAgeDays),
		fmt.Sprintf("需要操作: %d 替换, %d 升级", result.Summary.NodesNeedReplace, result.Summary.NodesNeedUpgrade),
	}
	if len(result.ActionNeeded) > 0 {
		top := result.ActionNeeded[0]
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("最高优先: %s (%s, %s)", top.NodeName, top.ForecastAction, top.EstTimeline))
	}
	if result.ForecastScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 制定节点轮换计划, 定期升级 kubelet 和 OS")
	}
	writeJSON(w, result)
}
