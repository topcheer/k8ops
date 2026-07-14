package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeTrendResult is the node condition trend & hardware failure prediction audit.
type NodeTrendResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         NodeTrendSummary     `json:"summary"`
	NodeConditions  []NodeConditionEntry `json:"nodeConditions"`
	AtRiskNodes     []NodeAtRisk         `json:"atRiskNodes"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// NodeTrendSummary aggregates node condition statistics.
type NodeTrendSummary struct {
	TotalNodes          int `json:"totalNodes"`
	HealthyNodes        int `json:"healthyNodes"`
	NodesWithConditions int `json:"nodesWithConditions"`
	MemoryPressure      int `json:"memoryPressure"`
	DiskPressure        int `json:"diskPressure"`
	PIDPressure         int `json:"pidPressure"`
	NetworkUnavailable  int `json:"networkUnavailable"`
	NotReady            int `json:"notReady"`
	NodesAtRisk         int `json:"nodesAtRisk"`
}

// NodeConditionEntry describes a node's condition status.
type NodeConditionEntry struct {
	NodeName         string            `json:"nodeName"`
	Status           string            `json:"status"` // Ready, NotReady
	Conditions       []ConditionDetail `json:"conditions"`
	KubeletVersion   string            `json:"kubeletVersion"`
	ContainerRuntime string            `json:"containerRuntime"`
	KernelVersion    string            `json:"kernelVersion"`
	OSImage          string            `json:"osImage"`
	LastHeartbeat    string            `json:"lastHeartbeat"`
	RiskLevel        string            `json:"riskLevel"`
}

// ConditionDetail describes a single node condition.
type ConditionDetail struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// NodeAtRisk describes a node at risk of failure.
type NodeAtRisk struct {
	NodeName    string   `json:"nodeName"`
	RiskFactors []string `json:"riskFactors"`
	Severity    string   `json:"severity"`
}

// handleNodeTrend audits node condition trends & predicts hardware failure risk.
// GET /api/operations/node-trend
func (s *Server) handleNodeTrend(w http.ResponseWriter, r *http.Request) {
	result := NodeTrendResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		nodeReady := false
		var conditions []ConditionDetail
		riskFactors := []string{}
		conditionCount := 0

		for _, cond := range node.Status.Conditions {
			cd := ConditionDetail{
				Type:    string(cond.Type),
				Status:  string(cond.Status),
				Reason:  cond.Reason,
				Message: cond.Message,
			}
			conditions = append(conditions, cd)

			switch cond.Type {
			case corev1.NodeReady:
				if cond.Status == corev1.ConditionTrue {
					nodeReady = true
				}
			case corev1.NodeMemoryPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Summary.MemoryPressure++
					conditionCount++
					riskFactors = append(riskFactors, "MemoryPressure")
				}
			case corev1.NodeDiskPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Summary.DiskPressure++
					conditionCount++
					riskFactors = append(riskFactors, "DiskPressure")
				}
			case corev1.NodePIDPressure:
				if cond.Status == corev1.ConditionTrue {
					result.Summary.PIDPressure++
					conditionCount++
					riskFactors = append(riskFactors, "PIDPressure")
				}
			case corev1.NodeNetworkUnavailable:
				if cond.Status == corev1.ConditionTrue {
					result.Summary.NetworkUnavailable++
					conditionCount++
					riskFactors = append(riskFactors, "NetworkUnavailable")
				}
			}
		}

		// Determine risk level
		riskLevel := "low"
		if !nodeReady {
			riskLevel = "critical"
			result.Summary.NotReady++
			result.Summary.NodesWithConditions++
			riskFactors = append(riskFactors, "NodeNotReady")
		} else if conditionCount > 0 {
			riskLevel = "high"
			result.Summary.NodesWithConditions++
		} else if len(riskFactors) > 0 {
			riskLevel = "medium"
		}

		// Check for stale heartbeat (last heartbeat > 5 min ago)
		lastHeartbeat := ""
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if !cond.LastHeartbeatTime.IsZero() {
					lastHeartbeat = cond.LastHeartbeatTime.Format(time.RFC3339)
					age := time.Since(cond.LastHeartbeatTime.Time)
					if age > 5*time.Minute {
						riskLevel = "critical"
						riskFactors = append(riskFactors, fmt.Sprintf("Stale heartbeat (%.0f min)", age.Minutes()))
					}
				}
			}
		}

		// Check kernel version consistency
		kernelVersion := node.Status.NodeInfo.KernelVersion
		osImage := node.Status.NodeInfo.OSImage
		kubeletVersion := node.Status.NodeInfo.KubeletVersion
		containerRuntime := node.Status.NodeInfo.ContainerRuntimeVersion

		result.NodeConditions = append(result.NodeConditions, NodeConditionEntry{
			NodeName:         node.Name,
			Status:           boolToStatus(nodeReady),
			Conditions:       conditions,
			KubeletVersion:   kubeletVersion,
			ContainerRuntime: containerRuntime,
			KernelVersion:    kernelVersion,
			OSImage:          osImage,
			LastHeartbeat:    lastHeartbeat,
			RiskLevel:        riskLevel,
		})

		if riskLevel == "healthy" || riskLevel == "low" {
			result.Summary.HealthyNodes++
		}

		if len(riskFactors) > 0 {
			severity := riskLevel
			if severity == "low" {
				severity = "info"
			}
			result.AtRiskNodes = append(result.AtRiskNodes, NodeAtRisk{
				NodeName:    node.Name,
				RiskFactors: riskFactors,
				Severity:    severity,
			})
			if riskLevel == "high" || riskLevel == "critical" {
				result.Summary.NodesAtRisk++
			}
		}
	}

	// Sort by risk level
	sort.Slice(result.NodeConditions, func(i, j int) bool {
		return riskLevelRank(result.NodeConditions[i].RiskLevel) > riskLevelRank(result.NodeConditions[j].RiskLevel)
	})
	sort.Slice(result.AtRiskNodes, func(i, j int) bool {
		return riskLevelRank(result.AtRiskNodes[i].Severity) > riskLevelRank(result.AtRiskNodes[j].Severity)
	})

	// Recommendations
	if result.Summary.NotReady > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes are NotReady — check kubelet, container runtime, and network connectivity", result.Summary.NotReady))
	}
	if result.Summary.MemoryPressure > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes under MemoryPressure — consider adding memory or draining workloads", result.Summary.MemoryPressure))
	}
	if result.Summary.DiskPressure > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes under DiskPressure — clean up images, logs, or increase disk capacity", result.Summary.DiskPressure))
	}
	if result.Summary.PIDPressure > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes under PIDPressure — check for process leaks or increase PID limits", result.Summary.PIDPressure))
	}
	if result.Summary.NetworkUnavailable > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have network issues — check CNI plugin and network configuration", result.Summary.NetworkUnavailable))
	}
	if result.Summary.NodesAtRisk > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes at risk of failure — schedule maintenance or cordoning", result.Summary.NodesAtRisk))
	}

	// Health score
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = (result.Summary.HealthyNodes * 100) / result.Summary.TotalNodes
		score -= result.Summary.MemoryPressure * 10
		score -= result.Summary.DiskPressure * 10
		score -= result.Summary.PIDPressure * 10
		score -= result.Summary.NetworkUnavailable * 10
		score -= result.Summary.NotReady * 20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score

	writeJSON(w, result)
}

func boolToStatus(ready bool) string {
	if ready {
		return "Ready"
	}
	return "NotReady"
}

func riskLevelRank(level string) int {
	switch level {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	case "healthy":
		return 0
	default:
		return 0
	}
}

var _ = strings.Contains
