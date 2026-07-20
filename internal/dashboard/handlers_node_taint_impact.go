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

// NodeTaintImpactResult analyzes how node taints affect workload scheduling.
type NodeTaintImpactResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         TaintImpactSummary    `json:"summary"`
	ByNode          []TaintNodeEntry      `json:"byNode"`
	UntoleratedPods []TaintImpactPodEntry `json:"untoleratedPods"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type TaintImpactSummary struct {
	TotalNodes      int `json:"totalNodes"`
	TaintedNodes    int `json:"taintedNodes"`
	TotalTaints     int `json:"totalTaints"`
	CriticalTaints  int `json:"criticalTaints"` // NoSchedule + NoExecute
	UntoleratedPods int `json:"untoleratedPods"`
	BlockedNodes    int `json:"blockedNodes"` // nodes where pods can't schedule
}

type TaintNodeEntry struct {
	NodeName    string        `json:"nodeName"`
	Taints      []TaintDetail `json:"taints"`
	TaintCount  int           `json:"taintCount"`
	BlocksSched bool          `json:"blocksScheduling"`
	RunningPods int           `json:"runningPods"`
	RiskLevel   string        `json:"riskLevel"`
}

type TaintDetail struct {
	Key     string `json:"key"`
	Effect  string `json:"effect"`
	Value   string `json:"value"`
	IsKnown bool   `json:"isKnownTaint"`
}

type TaintImpactPodEntry struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	NodeName  string `json:"nodeName"`
	Reason    string `json:"reason"`
}

// handleNodeTaintImpact handles GET /api/operations/node-taint-impact
func (s *Server) handleNodeTaintImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := NodeTaintImpactResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node pod count map
	nodePodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			nodePodCount[pod.Spec.NodeName]++
		}
	}

	knownTaintKeys := map[string]bool{
		"node-role.kubernetes.io/control-plane":          true,
		"node-role.kubernetes.io/master":                 true,
		"node.kubernetes.io/not-ready":                   true,
		"node.kubernetes.io/unreachable":                 true,
		"node.kubernetes.io/disk-pressure":               true,
		"node.kubernetes.io/memory-pressure":             true,
		"node.kubernetes.io/pid-pressure":                true,
		"node.kubernetes.io/network-unavailable":         true,
		"node.cloudprovider.kubernetes.io/uninitialized": true,
	}

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		entry := TaintNodeEntry{NodeName: node.Name}
		entry.RunningPods = nodePodCount[node.Name]

		for _, taint := range node.Spec.Taints {
			info := TaintDetail{
				Key:     taint.Key,
				Effect:  string(taint.Effect),
				Value:   taint.Value,
				IsKnown: knownTaintKeys[taint.Key],
			}
			entry.Taints = append(entry.Taints, info)
			result.Summary.TotalTaints++
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				result.Summary.CriticalTaints++
			}
		}

		entry.TaintCount = len(entry.Taints)
		entry.BlocksSched = result.Summary.CriticalTaints > 0 && !containsTaintKey(node.Spec.Taints, "node-role.kubernetes.io/control-plane") && !containsTaintKey(node.Spec.Taints, "node-role.kubernetes.io/master")

		switch {
		case entry.BlocksSched && !isKnownTaintOnly(node.Spec.Taints, knownTaintKeys):
			entry.RiskLevel = "high"
			result.Summary.BlockedNodes++
		case entry.BlocksSched:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.Summary.TaintedNodes++
		}
		result.ByNode = append(result.ByNode, entry)
	}

	// Check for pods that don't tolerate node taints (pending pods)
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodPending || len(pod.Status.Conditions) == 0 {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Reason == "Unschedulable" && strings.Contains(cond.Message, "taint") {
				result.UntoleratedPods = append(result.UntoleratedPods, TaintImpactPodEntry{
					PodName:   pod.Name,
					Namespace: pod.Namespace,
					Reason:    cond.Message,
				})
				result.Summary.UntoleratedPods++
			}
		}
	}

	sort.Slice(result.ByNode, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByNode[i].RiskLevel] < rank[result.ByNode[j].RiskLevel]
	})

	if result.Summary.TotalNodes > 0 {
		cleanRatio := float64(result.Summary.TotalNodes-result.Summary.BlockedNodes) / float64(result.Summary.TotalNodes)
		result.HealthScore = int(cleanRatio * 100)
		if result.Summary.UntoleratedPods > 0 {
			result.HealthScore -= 20
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("污点影响: %d 节点, %d 有污点, %d 严重污点, %d 调度受阻",
			result.Summary.TotalNodes, result.Summary.TaintedNodes, result.Summary.CriticalTaints, result.Summary.BlockedNodes),
	}
	if result.Summary.UntoleratedPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 因污点无法调度", result.Summary.UntoleratedPods))
	}
	if result.Summary.BlockedNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个工作节点有阻断性污点, 检查是否需要移除", result.Summary.BlockedNodes))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 清理不必要的污点, 确保工作负载有正确的 toleration")
	}
	writeJSON(w, result)
}

func containsTaintKey(taints []corev1.Taint, key string) bool {
	for _, t := range taints {
		if t.Key == key {
			return true
		}
	}
	return false
}

func isKnownTaintOnly(taints []corev1.Taint, known map[string]bool) bool {
	if len(taints) == 0 {
		return true
	}
	for _, t := range taints {
		if !known[t.Key] {
			return false
		}
	}
	return true
}
