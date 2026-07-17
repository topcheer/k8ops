package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeFailureBlastResult simulates single-node failure to calculate the blast
// radius: affected workloads, unavailable replicas, and recovery difficulty.
type NodeFailureBlastResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         NodeBlastSummary `json:"summary"`
	ByNode          []NodeBlastEntry `json:"byNode"`
	WorstCaseNode   NodeBlastEntry   `json:"worstCaseNode"`
	BlastScore      int              `json:"blastScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type NodeBlastSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	TotalPods           int     `json:"totalPods"`
	TotalWorkloads      int     `json:"totalWorkloads"`
	AvgPodsPerNode      float64 `json:"avgPodsPerNode"`
	MaxBlastPods        int     `json:"maxBlastPods"`
	MaxBlastPct         float64 `json:"maxBlastPct"`
	SingleReplicaAtRisk int     `json:"singleReplicaAtRisk"`
	AntiAffinityGaps    int     `json:"antiAffinityGaps"`
}

type NodeBlastEntry struct {
	NodeName          string   `json:"nodeName"`
	Role              string   `json:"role"`
	PodCount          int      `json:"podCount"`
	AffectedWorkloads []string `json:"affectedWorkloads"`
	SingleReplicaWL   []string `json:"singleReplicaWorkloads"`
	BlastPct          float64  `json:"blastPct"`
	RecoveryTime      string   `json:"recoveryEstimate"`
	HasLocalStorage   bool     `json:"hasLocalStorage"`
	RiskLevel         string   `json:"riskLevel"`
}

// handleNodeFailureBlast handles GET /api/scalability/node-failure-blast
func (s *Server) handleNodeFailureBlast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeFailureBlastResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node -> pods map and workload replica map
	nodePods := make(map[string][]corev1.Pod)
	wlReplicaCount := make(map[string]int) // ns/workload -> total replicas
	totalPodCount := 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}
		totalPodCount++
		nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName != "" {
			key := pod.Namespace + "/" + wlName
			wlReplicaCount[key]++
		}
	}

	// Collect unique workloads
	wlSet := make(map[string]bool)
	for key := range wlReplicaCount {
		wlSet[key] = true
	}

	result.Summary.TotalPods = totalPodCount
	result.Summary.TotalWorkloads = len(wlSet)
	result.Summary.TotalNodes = 0

	var entries []NodeBlastEntry
	maxBlast := 0
	antiAffinityGaps := 0
	singleReplicaAtRisk := 0

	for _, node := range nodes.Items {
		// Skip control-plane
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		if node.Spec.Unschedulable {
			continue
		}

		result.Summary.TotalNodes++
		role := "worker"

		entry := NodeBlastEntry{
			NodeName: node.Name,
			Role:     role,
		}

		// Check for local storage
		for _, vol := range node.Status.VolumesAttached {
			if vol.Name != "" {
				entry.HasLocalStorage = true
				break
			}
		}

		nodePodList := nodePods[node.Name]
		entry.PodCount = len(nodePodList)

		// Calculate affected workloads
		affectedWLs := make(map[string]int) // key -> count on this node
		for _, pod := range nodePodList {
			wlName := ""
			for _, ref := range pod.OwnerReferences {
				if ref.Controller != nil && *ref.Controller {
					wlName = ref.Name
					break
				}
			}
			if wlName == "" {
				continue
			}
			key := pod.Namespace + "/" + wlName
			affectedWLs[key]++

			// Check anti-affinity: if all replicas on same node
			totalReplicas := wlReplicaCount[key]
			if totalReplicas > 1 && affectedWLs[key] == totalReplicas {
				antiAffinityGaps++
			}
			if totalReplicas == 1 {
				singleReplicaAtRisk++
				entry.SingleReplicaWL = appendUniqueStr(entry.SingleReplicaWL, wlName)
			}
		}

		// Build affected workload names
		for key := range affectedWLs {
			entry.AffectedWorkloads = append(entry.AffectedWorkloads, key)
		}

		// Blast percentage
		if totalPodCount > 0 {
			entry.BlastPct = float64(entry.PodCount) / float64(totalPodCount) * 100
		}

		// Recovery time estimate
		switch {
		case entry.PodCount > 30:
			entry.RecoveryTime = "5-10min"
		case entry.PodCount > 10:
			entry.RecoveryTime = "2-5min"
		default:
			entry.RecoveryTime = "< 2min"
		}

		// Risk level
		switch {
		case entry.BlastPct >= 50:
			entry.RiskLevel = "critical"
		case entry.BlastPct >= 25:
			entry.RiskLevel = "high"
		case entry.BlastPct >= 10:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.PodCount > maxBlast {
			maxBlast = entry.PodCount
			result.WorstCaseNode = entry
		}

		entries = append(entries, entry)
	}

	result.Summary.MaxBlastPods = maxBlast
	result.Summary.AvgPodsPerNode = safeDivFloat(totalPodCount, result.Summary.TotalNodes)
	if totalPodCount > 0 {
		result.Summary.MaxBlastPct = float64(maxBlast) / float64(totalPodCount) * 100
	}
	result.Summary.SingleReplicaAtRisk = singleReplicaAtRisk
	result.Summary.AntiAffinityGaps = antiAffinityGaps

	// Sort by blast percentage descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BlastPct > entries[j].BlastPct
	})
	result.ByNode = entries

	// Blast score: lower blast percentage = higher score
	result.BlastScore = int(100 - result.Summary.MaxBlastPct)
	if result.BlastScore < 0 {
		result.BlastScore = 0
	}

	switch {
	case result.BlastScore >= 80:
		result.Grade = "A"
	case result.BlastScore >= 60:
		result.Grade = "B"
	case result.BlastScore >= 40:
		result.Grade = "C"
	case result.BlastScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildNodeBlastRecs(&result)
	writeJSON(w, result)
}

func safeDivFloat(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func buildNodeBlastRecs(r *NodeFailureBlastResult) []string {
	recs := []string{
		fmt.Sprintf("节点故障爆炸半径: %d 节点, %d Pod, 最大爆炸 %.1f%%", r.Summary.TotalNodes, r.Summary.TotalPods, r.Summary.MaxBlastPct),
	}
	if r.Summary.MaxBlastPct >= 50 {
		recs = append(recs, fmt.Sprintf("严重: 单节点故障影响 %.0f%% Pod", r.Summary.MaxBlastPct))
	}
	if r.Summary.SingleReplicaAtRisk > 0 {
		recs = append(recs, fmt.Sprintf("%d 个单副本工作负载在节点故障时完全不可用", r.Summary.SingleReplicaAtRisk))
	}
	if r.Summary.AntiAffinityGaps > 0 {
		recs = append(recs, fmt.Sprintf("%d 个多副本工作负载全部副本在同一节点 (反亲和性缺失)", r.Summary.AntiAffinityGaps))
	}
	if r.BlastScore < 60 {
		recs = append(recs, "建议: 增加 podAntiAffinity 规则确保副本跨节点分布, 提升最少副本数")
	}
	return recs
}
