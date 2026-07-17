package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutoscalerGapResult analyzes whether Cluster Autoscaler or Karpenter is
// properly configured, identifies pending pods (unschedulable), and
// calculates scaling gaps and recommendations.
type AutoscalerGapResult struct {
	ScannedAt        time.Time            `json:"scannedAt"`
	Summary          AutoscalerGapSummary `json:"summary"`
	PendingPods      []PendingPodEntry    `json:"pendingPods"`
	NodePoolGaps     []NodePoolGapEntry   `json:"nodePoolGaps"`
	AutoscalerStatus AutoscalerStatus     `json:"autoscalerStatus"`
	GapScore         int                  `json:"gapScore"`
	Grade            string               `json:"grade"`
	Recommendations  []string             `json:"recommendations"`
}

type AutoscalerGapSummary struct {
	TotalNodes           int     `json:"totalNodes"`
	WorkerNodes          int     `json:"workerNodes"`
	PendingPods          int     `json:"pendingPods"`
	UnschedulablePods    int     `json:"unschedulablePods"`
	TotalCPURequested    float64 `json:"totalCPURequested"`
	TotalMemRequested    float64 `json:"totalMemRequestedGB"`
	HasAutoscaler        bool    `json:"hasAutoscaler"`
	HasKarpenter         bool    `json:"hasKarpenter"`
	MaxNodeProvisionTime int     `json:"maxNodeProvisionTimeMin"`
}

type PendingPodEntry struct {
	PodName      string   `json:"podName"`
	Namespace    string   `json:"namespace"`
	Workload     string   `json:"workload"`
	PendingTime  string   `json:"pendingDuration"`
	Reason       string   `json:"reason"`
	CPURequest   float64  `json:"cpuRequest"`
	MemRequestGB float64  `json:"memRequestGB"`
	NodeSelector []string `json:"nodeSelector"`
}

type NodePoolGapEntry struct {
	NodeLabel    string `json:"nodeLabel"`
	CurrentValue string `json:"currentValue"`
	DesiredValue string `json:"desiredValue"`
	GapType      string `json:"gapType"`
	Impact       string `json:"impact"`
}

type AutoscalerStatus struct {
	Detected         string   `json:"detected"`
	ConfigNS         string   `json:"configNamespace"`
	MinNodes         int      `json:"minNodes"`
	MaxNodes         int      `json:"maxNodes"`
	ScaleDownEnabled bool     `json:"scaleDownEnabled"`
	ExpanderType     string   `json:"expanderType"`
	Issues           []string `json:"issues"`
}

// handleAutoscalerGap handles GET /api/scalability/autoscaler-gap
func (s *Server) handleAutoscalerGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AutoscalerGapResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Detect Cluster Autoscaler (deployments in kube-system with "cluster-autoscaler" in name)
	autoscalerStatus := AutoscalerStatus{
		Detected: "none",
	}

	// Check for CA pods
	for _, pod := range pods.Items {
		if pod.Namespace == "kube-system" {
			podName := pod.Name
			if contains(podName, "cluster-autoscaler") {
				autoscalerStatus.Detected = "cluster-autoscaler"
				result.Summary.HasAutoscaler = true
			}
			if contains(podName, "karpenter") {
				autoscalerStatus.Detected = "karpenter"
				result.Summary.HasKarpenter = true
			}
		}
	}

	// Also check deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("kube-system").List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		if contains(d.Name, "cluster-autoscaler") {
			autoscalerStatus.Detected = "cluster-autoscaler"
			result.Summary.HasAutoscaler = true
			// Try to extract min/max nodes from args
			for _, c := range d.Spec.Template.Spec.Containers {
				for _, arg := range c.Args {
					if startsWith(arg, "--scale-down-enabled") {
						autoscalerStatus.ScaleDownEnabled = !contains(arg, "false")
					}
					if startsWith(arg, "--expander") {
						autoscalerStatus.ExpanderType = arg
					}
					if startsWith(arg, "--min-nodes") || startsWith(arg, "--nodes") {
						autoscalerStatus.Issues = append(autoscalerStatus.Issues, "CA min/max from args: "+arg)
					}
				}
			}
		}
	}

	result.AutoscalerStatus = autoscalerStatus

	// Analyze nodes
	workerCount := 0
	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		workerCount++
	}
	result.Summary.WorkerNodes = workerCount

	// Analyze pending/unschedulable pods
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		isPending := pod.Status.Phase == corev1.PodPending
		isUnschedulable := false
		pendingReason := ""

		for _, cond := range pod.Status.Conditions {
			if cond.Reason == "Unschedulable" || cond.Message != "" && contains(cond.Message, "Insufficient") {
				isUnschedulable = true
				pendingReason = cond.Message
				break
			}
		}

		if !isPending && !isUnschedulable {
			continue
		}

		if isPending {
			result.Summary.PendingPods++
		}
		if isUnschedulable {
			result.Summary.UnschedulablePods++
		}

		entry := PendingPodEntry{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Reason:    pendingReason,
		}

		// Get workload
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				entry.Workload = ref.Name
				break
			}
		}

		// Calculate pending duration
		if !pod.CreationTimestamp.IsZero() {
			dur := time.Since(pod.CreationTimestamp.Time)
			entry.PendingTime = fmt.Sprintf("%.0fm", dur.Minutes())
		}

		// Get resource requests
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				entry.CPURequest += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemRequestGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
		result.Summary.TotalCPURequested += entry.CPURequest
		result.Summary.TotalMemRequested += entry.MemRequestGB

		// Node selector
		for k, v := range pod.Spec.NodeSelector {
			entry.NodeSelector = append(entry.NodeSelector, k+"="+v)
		}

		result.PendingPods = append(result.PendingPods, entry)
	}

	// Sort pending pods by CPU request descending
	sort.Slice(result.PendingPods, func(i, j int) bool {
		return result.PendingPods[i].CPURequest > result.PendingPods[j].CPURequest
	})

	// Identify node pool gaps
	if !result.Summary.HasAutoscaler && !result.Summary.HasKarpenter {
		result.NodePoolGaps = append(result.NodePoolGaps, NodePoolGapEntry{
			NodeLabel:    "autoscaler",
			CurrentValue: "none",
			DesiredValue: "Cluster Autoscaler or Karpenter",
			GapType:      "missing",
			Impact:       "Pending pods cannot trigger node provisioning",
		})
	}

	if workerCount == 1 {
		result.NodePoolGaps = append(result.NodePoolGaps, NodePoolGapEntry{
			NodeLabel:    "worker-count",
			CurrentValue: fmt.Sprintf("%d", workerCount),
			DesiredValue: ">= 3 (HA minimum)",
			GapType:      "insufficient",
			Impact:       "Single node = single point of failure, no fault tolerance",
		})
	}

	if result.Summary.UnschedulablePods > 0 {
		result.NodePoolGaps = append(result.NodePoolGaps, NodePoolGapEntry{
			NodeLabel:    "pending-capacity",
			CurrentValue: fmt.Sprintf("%d unschedulable", result.Summary.UnschedulablePods),
			DesiredValue: "0 pending pods",
			GapType:      "capacity",
			Impact:       fmt.Sprintf("%d pods stuck pending due to insufficient resources", result.Summary.UnschedulablePods),
		})
	}

	// Gap score
	result.GapScore = 100
	if !result.Summary.HasAutoscaler && !result.Summary.HasKarpenter {
		result.GapScore -= 30
	}
	result.GapScore -= result.Summary.UnschedulablePods * 10
	if workerCount < 3 {
		result.GapScore -= 20
	}
	if result.GapScore < 0 {
		result.GapScore = 0
	}

	switch {
	case result.GapScore >= 80:
		result.Grade = "A"
	case result.GapScore >= 60:
		result.Grade = "B"
	case result.GapScore >= 40:
		result.Grade = "C"
	case result.GapScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildAutoscalerGapRecs(&result)
	writeJSON(w, result)
}

func buildAutoscalerGapRecs(r *AutoscalerGapResult) []string {
	recs := []string{
		fmt.Sprintf("Autoscaler 缺口: %d worker 节点, %d pending Pod, %d unschedulable",
			r.Summary.WorkerNodes, r.Summary.PendingPods, r.Summary.UnschedulablePods),
	}
	if !r.Summary.HasAutoscaler && !r.Summary.HasKarpenter {
		recs = append(recs, "紧急: 未检测到 Cluster Autoscaler 或 Karpenter, pending Pod 无法自动扩展")
	} else {
		recs = append(recs, fmt.Sprintf("已检测到: %s", r.AutoscalerStatus.Detected))
	}
	if r.Summary.WorkerNodes < 3 {
		recs = append(recs, fmt.Sprintf("警告: 仅 %d worker 节点, 生产环境建议 >= 3 (HA)", r.Summary.WorkerNodes))
	}
	if r.Summary.UnschedulablePods > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 因资源不足无法调度", r.Summary.UnschedulablePods))
	}
	if r.GapScore < 60 {
		recs = append(recs, "建议: 部署 Cluster Autoscaler, 设置 min=3/max=10, 启用 scale-down-utilization-threshold")
	}
	return recs
}
