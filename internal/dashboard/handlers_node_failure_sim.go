package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nfsNodeInfo holds parsed node info for failure simulation.
type nfsNodeInfo struct {
	name     string
	ready    bool
	cpuCap   int64 // milli-cores allocatable
	memCapMB float64
	taints   []corev1.Taint
	labels   map[string]string
}

// NFSResult is the node failure impact simulation.
type NFSResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         NFSSummary `json:"summary"`
	ByNode          []NFSEntry `json:"byNode"`
	CriticalNodes   []NFSEntry `json:"criticalNodes"`
	SingleReplica   []NFSEntry `json:"singleReplica"`
	Issues          []NFSIssue `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// NFSSummary aggregates node failure simulation statistics.
type NFSSummary struct {
	TotalNodes         int `json:"totalNodes"`
	TotalPods          int `json:"totalPods"`
	AffectedPerNode    int `json:"affectedPerNodeAvg"`
	MaxAffected        int `json:"maxAffected"`
	CriticalNodes      int `json:"criticalNodes"`
	UnschedulableAvg   int `json:"unschedulableAvg"`
	SingleReplicaNodes int `json:"singleReplicaNodes"`
	ResilienceScore    int `json:"resilienceScore"`
}

// NFSEntry describes the impact of one node failing.
type NFSEntry struct {
	NodeName        string   `json:"nodeName"`
	Ready           bool     `json:"ready"`
	PodCount        int      `json:"podCount"`
	AffectedPods    int      `json:"affectedPods"`
	CanReschedule   int      `json:"canReschedule"`
	Unschedulable   int      `json:"unschedulable"`
	SingleReplicaWL int      `json:"singleReplicaWL"`
	CPURequests     string   `json:"cpuRequests"`
	MemRequestsMB   float64  `json:"memRequestsMB"`
	WorkloadList    []string `json:"workloadList,omitempty"`
	RiskLevel       string   `json:"riskLevel"`
}

// NFSIssue is a detected resilience problem.
type NFSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleNodeFailureSim simulates node failure impact across the cluster.
// GET /api/scalability/node-failure-sim
func (s *Server) handleNodeFailureSim(w http.ResponseWriter, r *http.Request) {
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

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build replica count per workload (ns/name → replicas)
	replicaMap := make(map[string]int32)
	for _, dep := range deployments.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		replicaMap[dep.Namespace+"/"+dep.Name] = replicas
	}

	// Build node info
	nodeInfos := make(map[string]*nfsNodeInfo)
	for _, node := range nodes.Items {
		ni := &nfsNodeInfo{
			name:   node.Name,
			labels: node.Labels,
			taints: node.Spec.Taints,
		}
		ni.ready = isNodeReady(&node)
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			ni.cpuCap = cpu.MilliValue()
		}
		if mem := node.Status.Allocatable.Memory(); mem != nil {
			ni.memCapMB = float64(mem.Value()) / (1024 * 1024)
		}
		nodeInfos[node.Name] = ni
	}

	// Build node → pods map (exclude DS, completed, system pods)
	nodePods := make(map[string][]corev1.Pod)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if isDaemonSetPod(pod) {
			continue
		}
		if pod.Namespace == "kube-system" {
			continue
		}
		nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)
	}

	result := NFSResult{ScannedAt: time.Now()}
	result.Summary.TotalNodes = len(nodes.Items)
	result.Summary.TotalPods = len(pods.Items)

	// Simulate each node failure
	for nodeName, ni := range nodeInfos {
		podsOnNode := nodePods[nodeName]
		entry := NFSEntry{
			NodeName:     nodeName,
			Ready:        ni.ready,
			PodCount:     len(podsOnNode),
			AffectedPods: len(podsOnNode),
		}

		var totalCPUMilli int64
		var totalMemMB float64
		wlSet := make(map[string]bool)

		for _, pod := range podsOnNode {
			// Get workload name
			wlName := ""
			for _, ref := range pod.OwnerReferences {
				wlName = ref.Name
				break
			}

			// Sum resource requests
			for _, c := range pod.Spec.Containers {
				if req := c.Resources.Requests.Cpu(); req != nil {
					totalCPUMilli += req.MilliValue()
				}
				if req := c.Resources.Requests.Memory(); req != nil {
					totalMemMB += float64(req.Value()) / (1024 * 1024)
				}
			}

			// Check if single-replica workload
			if wlName != "" {
				key := pod.Namespace + "/" + wlName
				if replicas, ok := replicaMap[key]; ok && replicas <= 1 {
					entry.SingleReplicaWL++
					wlSet[wlName+" (1 replica)"] = true
				} else {
					wlSet[wlName] = true
				}
			}

			// Can reschedule: check if other nodes can accept this pod
			if nfsCanReschedule(pod, nodeName, nodeInfos) {
				entry.CanReschedule++
			} else {
				entry.Unschedulable++
			}
		}

		entry.CPURequests = fmt.Sprintf("%dm", totalCPUMilli)
		entry.MemRequestsMB = totalMemMB

		// Top 5 workloads
		for wl := range wlSet {
			entry.WorkloadList = append(entry.WorkloadList, wl)
		}
		sort.Strings(entry.WorkloadList)
		if len(entry.WorkloadList) > 5 {
			entry.WorkloadList = entry.WorkloadList[:5]
		}

		entry.RiskLevel = nfsAssessRisk(entry)

		if entry.AffectedPods > 10 {
			result.Summary.CriticalNodes++
			result.CriticalNodes = append(result.CriticalNodes, entry)
			result.Issues = append(result.Issues, NFSIssue{
				Severity: "warning", Type: "high-impact-node",
				Resource: nodeName,
				Message:  fmt.Sprintf("Node %s failure would affect %d pods (%d cannot reschedule)", nodeName, entry.AffectedPods, entry.Unschedulable),
			})
		}
		if entry.SingleReplicaWL > 0 {
			result.Summary.SingleReplicaNodes++
			result.SingleReplica = append(result.SingleReplica, entry)
			result.Issues = append(result.Issues, NFSIssue{
				Severity: "critical", Type: "single-replica-on-node",
				Resource: nodeName,
				Message:  fmt.Sprintf("Node %s hosts %d single-replica workload(s) — failure causes permanent downtime", nodeName, entry.SingleReplicaWL),
			})
		}

		result.ByNode = append(result.ByNode, entry)
	}

	// Calculate averages
	if len(result.ByNode) > 0 {
		totalAffected := 0
		totalUnsched := 0
		for _, e := range result.ByNode {
			totalAffected += e.AffectedPods
			totalUnsched += e.Unschedulable
			if e.AffectedPods > result.Summary.MaxAffected {
				result.Summary.MaxAffected = e.AffectedPods
			}
		}
		result.Summary.AffectedPerNode = totalAffected / len(result.ByNode)
		result.Summary.UnschedulableAvg = totalUnsched / len(result.ByNode)
	}

	// Sort
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].AffectedPods > result.ByNode[j].AffectedPods
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return nfsIssueRank(result.Issues[i].Severity) < nfsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ResilienceScore = nfsScore(result.Summary)
	result.Recommendations = nfsGenRecs(result.Summary, result.CriticalNodes, result.SingleReplica)

	writeJSON(w, result)
}

// nfsCanReschedule checks if a pod could be scheduled on any other node.
func nfsCanReschedule(pod corev1.Pod, failedNode string, nodeInfos map[string]*nfsNodeInfo) bool {
	podCPUReq := int64(0)
	podMemReqMB := 0.0
	for _, c := range pod.Spec.Containers {
		if req := c.Resources.Requests.Cpu(); req != nil {
			podCPUReq += req.MilliValue()
		}
		if req := c.Resources.Requests.Memory(); req != nil {
			podMemReqMB += float64(req.Value()) / (1024 * 1024)
		}
	}

	// Check node selector
	nodeSelector := pod.Spec.NodeSelector

	for nodeName, ni := range nodeInfos {
		if nodeName == failedNode {
			continue
		}
		if !ni.ready {
			continue
		}
		// Check resource capacity (simplified: does the node have enough allocatable?)
		if ni.cpuCap < podCPUReq {
			continue
		}
		if ni.memCapMB < podMemReqMB {
			continue
		}
		// Check node selector
		if len(nodeSelector) > 0 {
			match := true
			for k, v := range nodeSelector {
				if ni.labels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		// Check taints (simplified: if node has NoSchedule taint and pod has no toleration, skip)
		hasBlockingTaint := false
		for _, taint := range ni.taints {
			if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
				tolerated := false
				for _, tol := range pod.Spec.Tolerations {
					if tol.Key == taint.Key && (tol.Effect == taint.Effect || tol.Effect == "") {
						tolerated = true
						break
					}
				}
				if !tolerated {
					hasBlockingTaint = true
					break
				}
			}
		}
		if hasBlockingTaint {
			continue
		}
		// Found at least one node that can accept this pod
		return true
	}
	return false
}

// isDaemonSetPod checks if a pod is owned by a DaemonSet.
func isDaemonSetPod(pod corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// nfsAssessRisk determines node failure risk level.
func nfsAssessRisk(entry NFSEntry) string {
	if entry.SingleReplicaWL > 0 {
		return "critical"
	}
	if entry.Unschedulable > 5 {
		return "critical"
	}
	if entry.AffectedPods > 10 {
		return "high"
	}
	if entry.Unschedulable > 0 {
		return "medium"
	}
	return "low"
}

// nfsScore computes resilience 0-100.
func nfsScore(s NFSSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	score := 100
	score -= s.SingleReplicaNodes * 12
	score -= s.CriticalNodes * 6
	score -= s.UnschedulableAvg * 4
	if score < 0 {
		score = 0
	}
	return score
}

// nfsGenRecs produces actionable advice.
func nfsGenRecs(s NFSSummary, critical []NFSEntry, singleReplica []NFSEntry) []string {
	var recs []string

	if s.SingleReplicaNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) host single-replica workloads — node failure causes permanent downtime, scale to 2+ replicas", s.SingleReplicaNodes))
	}
	if s.CriticalNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) would affect >10 pods on failure — distribute workloads with pod anti-affinity", s.CriticalNodes))
	}
	if s.MaxAffected > 15 {
		recs = append(recs, fmt.Sprintf("Worst-case: %d pods lost in a single node failure — add nodes or spread pods", s.MaxAffected))
	}
	if s.UnschedulableAvg > 2 {
		recs = append(recs, fmt.Sprintf("Average %d pod(s) per node cannot reschedule after failure — check affinity/taints/resource constraints", s.UnschedulableAvg))
	}
	if s.ResilienceScore < 60 {
		recs = append(recs, fmt.Sprintf("Cluster resilience score is %d/100 — multiple nodes pose single points of failure", s.ResilienceScore))
	}
	if s.SingleReplicaNodes == 0 && s.CriticalNodes == 0 {
		recs = append(recs, "No critical node failure risks detected — good resilience posture")
	}

	return recs
}

func nfsIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
