package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterNarrativeResult generates a human-readable story of the cluster's
// current state, designed for executive reporting and onboarding docs.
// It translates raw metrics into natural language paragraphs.
type ClusterNarrativeResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	Title            string             `json:"title"`
	ExecutiveSummary string             `json:"executiveSummary"`
	Sections         []NarrativeSection `json:"sections"`
	KeyMetrics       []NarrativeMetric  `json:"keyMetrics"`
	ActionItems      []string           `json:"actionItems"`
}

type NarrativeSection struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Grade   string `json:"grade"`
}

type NarrativeMetric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Trend string `json:"trend"`
}

// handleClusterNarrative handles GET /api/docs/cluster-narrative
func (s *Server) handleClusterNarrative(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ClusterNarrativeResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Counts
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}
	deployCount := 0
	missingProbes := 0
	missingLimits := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes++
			}
			if c.Resources.Limits.Cpu().IsZero() {
				missingLimits++
			}
		}
	}
	podCount := 0
	crashPods := 0
	for _, p := range pods.Items {
		if !isSystemNamespace(p.Namespace) {
			podCount++
		}
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashPods++
			}
		}
	}
	secretCount := 0
	for _, sec := range secrets.Items {
		if !isSystemNamespace(sec.Namespace) {
			secretCount++
		}
	}

	// Title and exec summary
	result.Title = fmt.Sprintf("k8ops 集群运维报告 (%s)", result.ScannedAt.Format("2006-01-02"))

	highRisk := 0
	if workerCount < 2 {
		highRisk++
	}
	if crashPods > 0 {
		highRisk++
	}
	if missingLimits > 10 {
		highRisk++
	}

	riskLevel := "低"
	if highRisk > 2 {
		riskLevel = "高"
	} else if highRisk > 0 {
		riskLevel = "中"
	}

	result.ExecutiveSummary = fmt.Sprintf(
		"集群当前运行 %d 个节点、%d 个命名空间、%d 个 Deployment 和 %d 个 Pod。"+
			"风险等级: %s。%d 个 Pod 处于 CrashLoopBackOff，%d 个容器缺少探针，%d 个容器缺少资源限制。",
		workerCount, nsCount, deployCount, podCount, riskLevel, crashPods, missingProbes, missingLimits,
	)

	// Sections
	result.Sections = []NarrativeSection{
		{
			Title: "高可用性",
			Content: func() string {
				if workerCount < 2 {
					return fmt.Sprintf("当前仅 %d 个工作节点，无法容忍节点故障。建议至少 3 个节点。", workerCount)
				}
				return fmt.Sprintf("%d 个工作节点，具备 HA 能力。", workerCount)
			}(),
			Grade: func() string {
				if workerCount >= 3 {
					return "A"
				} else if workerCount >= 2 {
					return "B"
				}
				return "D"
			}(),
		},
		{
			Title: "稳定性",
			Content: fmt.Sprintf("%d 个 Pod 运行中，%d 个崩溃。%d 个容器缺少探针配置。",
				podCount, crashPods, missingProbes),
			Grade: func() string {
				if crashPods == 0 && missingProbes < 5 {
					return "A"
				} else if crashPods < 3 {
					return "B"
				}
				return "D"
			}(),
		},
		{
			Title: "资源治理",
			Content: fmt.Sprintf("%d/%d 命名空间有 ResourceQuota。%d 个容器缺少资源限制。",
				len(quotas.Items), nsCount, missingLimits),
			Grade: func() string {
				if nsCount > 0 && len(quotas.Items) == 0 {
					return "D"
				}
				return "C"
			}(),
		},
		{
			Title: "网络安全",
			Content: fmt.Sprintf("%d 个 NetworkPolicy 覆盖部分命名空间。建议为所有命名空间添加默认拒绝策略。",
				len(netpols.Items)),
			Grade: func() string {
				if len(netpols.Items) > nsCount/2 {
					return "B"
				}
				return "D"
			}(),
		},
		{
			Title: "可靠性",
			Content: fmt.Sprintf("%d 个 PDB / %d 个 Deployment。%d 个 HPA 配置。",
				len(pdbs.Items), deployCount, len(hpas.Items)),
			Grade: func() string {
				if len(pdbs.Items) >= deployCount/2 && len(hpas.Items) > 0 {
					return "B"
				}
				return "D"
			}(),
		},
		{
			Title: "安全态势",
			Content: fmt.Sprintf("%d 个 Secret 在使用。建议清理未引用的 Secret，并使用 cert-manager 管理 TLS。",
				secretCount),
			Grade: func() string {
				if secretCount < 50 {
					return "B"
				}
				return "C"
			}(),
		},
	}

	// Key metrics
	result.KeyMetrics = []NarrativeMetric{
		{Label: "Worker Nodes", Value: fmt.Sprintf("%d", workerCount)},
		{Label: "Namespaces", Value: fmt.Sprintf("%d", nsCount)},
		{Label: "Deployments", Value: fmt.Sprintf("%d", deployCount)},
		{Label: "Running Pods", Value: fmt.Sprintf("%d", podCount)},
		{Label: "CrashLoopBackOff", Value: fmt.Sprintf("%d", crashPods)},
		{Label: "PDBs", Value: fmt.Sprintf("%d", len(pdbs.Items))},
		{Label: "HPAs", Value: fmt.Sprintf("%d", len(hpas.Items))},
		{Label: "ResourceQuotas", Value: fmt.Sprintf("%d", len(quotas.Items))},
		{Label: "NetworkPolicies", Value: fmt.Sprintf("%d", len(netpols.Items))},
		{Label: "Secrets", Value: fmt.Sprintf("%d", secretCount)},
	}

	// Action items
	result.ActionItems = []string{}
	if workerCount < 2 {
		result.ActionItems = append(result.ActionItems, "添加工作节点实现高可用")
	}
	if crashPods > 0 {
		result.ActionItems = append(result.ActionItems, fmt.Sprintf("修复 %d 个 CrashLoopBackOff Pod", crashPods))
	}
	if missingProbes > 0 {
		result.ActionItems = append(result.ActionItems, fmt.Sprintf("为 %d 个容器添加健康探针", missingProbes))
	}
	if missingLimits > 0 {
		result.ActionItems = append(result.ActionItems, fmt.Sprintf("为 %d 个容器设置资源限制", missingLimits))
	}
	if len(quotas.Items) == 0 && nsCount > 0 {
		result.ActionItems = append(result.ActionItems, "为所有命名空间创建 ResourceQuota")
	}
	if len(netpols.Items) == 0 {
		result.ActionItems = append(result.ActionItems, "部署 NetworkPolicy 实现网络隔离")
	}

	writeJSON(w, result)
}
