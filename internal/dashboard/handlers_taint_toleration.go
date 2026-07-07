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

// TTResult is the taint & tolerance impact analysis.
type TTResult struct {
	ScannedAt        time.Time      `json:"scannedAt"`
	Summary          TTSummary      `json:"summary"`
	ByNode           []TTNodeEntry  `json:"byNode"`
	BlockedNodes     []TTNodeEntry  `json:"blockedNodes"`     // nodes with NoSchedule taints
	CordonedNodes    []TTNodeEntry  `json:"cordonedNodes"`    // nodes with node.kubernetes.io/unschedulable
	BroadTolerations []TTPodEntry   `json:"broadTolerations"` // pods tolerating everything
	ByTaint          []TTTaintEntry `json:"byTaint"`          // taint summary
	Issues           []TTIssue      `json:"issues"`
	Recommendations  []string       `json:"recommendations"`
}

// TTSummary aggregates taint statistics.
type TTSummary struct {
	TotalNodes       int `json:"totalNodes"`
	NodesWithTaints  int `json:"nodesWithTaints"`
	NoScheduleNodes  int `json:"noScheduleNodes"`  // blocked for new pods
	NoExecuteNodes   int `json:"noExecuteNodes"`   // evicting existing pods
	CordonedNodes    int `json:"cordonedNodes"`    // unschedulable
	PodsWithBroadTol int `json:"podsWithBroadTol"` // tolerate all taints
	ImpactScore      int `json:"impactScore"`      // 0-100
}

// TTNodeEntry describes one node's taint status.
type TTNodeEntry struct {
	NodeName      string         `json:"nodeName"`
	Ready         bool           `json:"ready"`
	Unschedulable bool           `json:"unschedulable"`
	Taints        []corev1.Taint `json:"taints"`
	TaintCount    int            `json:"taintCount"`
	HasNoSchedule bool           `json:"hasNoSchedule"`
	HasNoExecute  bool           `json:"hasNoExecute"`
	RiskLevel     string         `json:"riskLevel"`
}

// TTPodEntry describes a pod with broad tolerations.
type TTPodEntry struct {
	PodName     string   `json:"podName"`
	Namespace   string   `json:"namespace"`
	Workload    string   `json:"workload,omitempty"`
	Tolerations []string `json:"tolerations"`
}

// TTTaintEntry summarizes a taint across the cluster.
type TTTaintEntry struct {
	Key       string   `json:"key"`
	Effect    string   `json:"effect"`
	NodeCount int      `json:"nodeCount"`
	NodeNames []string `json:"nodeNames"`
}

// TTIssue is a detected taint problem.
type TTIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleTaintToleration audits node taints and pod tolerations.
// GET /api/product/taint-toleration
func (s *Server) handleTaintToleration(w http.ResponseWriter, r *http.Request) {
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

	result := TTResult{ScannedAt: time.Now()}

	// Taint map: key=effect/value → nodes
	taintMap := make(map[string]*TTTaintEntry)

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		entry := TTNodeEntry{
			NodeName:      node.Name,
			Ready:         isNodeReady(&node),
			Unschedulable: node.Spec.Unschedulable,
			TaintCount:    len(node.Spec.Taints),
		}

		for _, taint := range node.Spec.Taints {
			entry.Taints = append(entry.Taints, taint)

			if taint.Effect == corev1.TaintEffectNoSchedule {
				entry.HasNoSchedule = true
			}
			if taint.Effect == corev1.TaintEffectNoExecute {
				entry.HasNoExecute = true
			}

			// Build taint summary
			key := fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect)
			if taintMap[key] == nil {
				taintMap[key] = &TTTaintEntry{
					Key:    taint.Key,
					Effect: string(taint.Effect),
				}
			}
			taintMap[key].NodeCount++
			taintMap[key].NodeNames = append(taintMap[key].NodeNames, node.Name)
		}

		if entry.Unschedulable {
			result.Summary.CordonedNodes++
			result.CordonedNodes = append(result.CordonedNodes, entry)
			result.Issues = append(result.Issues, TTIssue{
				Severity: "warning", Type: "node-cordoned",
				Resource: node.Name,
				Message:  fmt.Sprintf("Node %s is cordoned (unschedulable=true) — no new pods will be scheduled", node.Name),
			})
		}

		if entry.HasNoSchedule {
			result.Summary.NoScheduleNodes++
			result.BlockedNodes = append(result.BlockedNodes, entry)
		}

		if entry.HasNoExecute {
			result.Summary.NoExecuteNodes++
			if !entry.HasNoSchedule {
				result.BlockedNodes = append(result.BlockedNodes, entry)
			}
			result.Issues = append(result.Issues, TTIssue{
				Severity: "critical", Type: "no-execute-taint",
				Resource: node.Name,
				Message:  fmt.Sprintf("Node %s has NoExecute taint — pods without matching toleration are being evicted", node.Name),
			})
		}

		if entry.TaintCount > 0 {
			result.Summary.NodesWithTaints++
		}

		entry.RiskLevel = ttAssessNodeRisk(entry)
		result.ByNode = append(result.ByNode, entry)
	}

	// Check pods for broad tolerations
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}

		for _, tol := range pod.Spec.Tolerations {
			// Broad toleration: empty key + Exists operator = tolerates everything
			if tol.Operator == corev1.TolerationOpExists && tol.Key == "" {
				result.Summary.PodsWithBroadTol++
				wlName := ""
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "ReplicaSet" || ref.Kind == "Deployment" ||
						ref.Kind == "StatefulSet" || ref.Kind == "DaemonSet" {
						wlName = ref.Name
						break
					}
				}
				tolStrs := []string{}
				if tol.Effect == "" {
					tolStrs = append(tolStrs, "*:* (all effects)")
				} else {
					tolStrs = append(tolStrs, fmt.Sprintf("*:%s", tol.Effect))
				}
				entry := TTPodEntry{
					PodName:     pod.Name,
					Namespace:   pod.Namespace,
					Workload:    wlName,
					Tolerations: tolStrs,
				}
				result.BroadTolerations = append(result.BroadTolerations, entry)
				result.Issues = append(result.Issues, TTIssue{
					Severity: "warning", Type: "broad-toleration",
					Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
					Message:  fmt.Sprintf("Pod %s/%s has broad toleration (key=Exists, effect=%s) — will run on any tainted node including master/infra", pod.Namespace, pod.Name, ttEffectOrAny(tol.Effect)),
				})
				break // one broad toleration is enough
			}
		}
	}

	// Build taint summary
	for _, te := range taintMap {
		result.ByTaint = append(result.ByTaint, *te)
	}

	// Sort
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].TaintCount > result.ByNode[j].TaintCount
	})
	sort.Slice(result.ByTaint, func(i, j int) bool {
		return result.ByTaint[i].NodeCount > result.ByTaint[j].NodeCount
	})
	if len(result.ByTaint) > 20 {
		result.ByTaint = result.ByTaint[:20]
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		return ttIssueRank(result.Issues[i].Severity) < ttIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ImpactScore = ttScore(result.Summary)
	result.Recommendations = ttGenRecs(result.Summary, result.BlockedNodes, result.CordonedNodes, result.BroadTolerations)

	writeJSON(w, result)
}

func ttEffectOrAny(effect corev1.TaintEffect) string {
	if effect == "" {
		return "any"
	}
	return string(effect)
}

// ttAssessNodeRisk determines risk level.
func ttAssessNodeRisk(entry TTNodeEntry) string {
	if entry.HasNoExecute {
		return "critical"
	}
	if entry.Unschedulable || entry.HasNoSchedule {
		return "high"
	}
	if entry.TaintCount > 0 {
		return "medium"
	}
	return "low"
}

// ttScore computes impact score 0-100.
func ttScore(s TTSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	score := 100
	score -= s.NoExecuteNodes * 15
	score -= s.CordonedNodes * 8
	score -= s.NoScheduleNodes * 5
	score -= s.PodsWithBroadTol * 3
	if score < 0 {
		score = 0
	}
	return score
}

// ttGenRecs produces actionable advice.
func ttGenRecs(s TTSummary, blocked []TTNodeEntry, cordoned []TTNodeEntry, broad []TTPodEntry) []string {
	var recs []string

	if s.NoExecuteNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have NoExecute taints — pods are being actively evicted, check node health", s.NoExecuteNodes))
	}
	if s.CordonedNodes > 0 {
		top := ""
		if len(cordoned) > 0 {
			top = fmt.Sprintf(" (e.g. %s)", cordoned[0].NodeName)
		}
		recs = append(recs, fmt.Sprintf("%d node(s) are cordoned%s — no new pods will be scheduled, uncordon when ready", s.CordonedNodes, top))
	}
	if s.NoScheduleNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have NoSchedule taints — these nodes are blocked for new pod scheduling", s.NoScheduleNodes))
	}
	if s.PodsWithBroadTol > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have broad tolerations (key=Exists) — they may run on tainted nodes including master/infra, restrict tolerations", s.PodsWithBroadTol))
	}
	if s.ImpactScore < 70 {
		recs = append(recs, fmt.Sprintf("Taint impact score is %d/100 — review node taint configuration", s.ImpactScore))
	}
	if s.NoExecuteNodes == 0 && s.CordonedNodes == 0 && s.PodsWithBroadTol == 0 {
		recs = append(recs, "Node taints and pod tolerations are well-configured — good scheduling posture")
	}

	return recs
}

func ttIssueRank(s string) int {
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

// Suppress unused import warning
var _ = strings.Contains
