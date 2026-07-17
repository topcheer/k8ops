package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ActionPriorityMatrixResult aggregates all findings from across the platform
// into a single prioritized action queue. Each action has impact, effort,
// and urgency scoring so operators know exactly what to fix first.
type ActionPriorityMatrixResult struct {
	ScannedAt    time.Time        `json:"scannedAt"`
	TotalActions int              `json:"totalActions"`
	Critical     int              `json:"criticalCount"`
	High         int              `json:"highCount"`
	Medium       int              `json:"mediumCount"`
	Actions      []PriorityAction `json:"actions"`
	QuickWins    []PriorityAction `json:"quickWins"`
	BatchPlan    []ActionBatch    `json:"batchPlan"`
	Score        int              `json:"platformScore"`
}

type PriorityAction struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Impact   int    `json:"impact"`   // 1-10
	Effort   int    `json:"effort"`   // 1-10 (lower=easier)
	Urgency  int    `json:"urgency"`  // 1-10
	Priority int    `json:"priority"` // calculated score
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Action   string `json:"action"`
	Affected int    `json:"affectedCount"`
}

type ActionBatch struct {
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Count    int      `json:"count"`
	EstTime  string   `json:"estimatedTime"`
	Actions  []string `json:"actionSummary"`
}

// handleActionPriorityMatrix handles GET /api/docs/action-priority-matrix
func (s *Server) handleActionPriorityMatrix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ActionPriorityMatrixResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	var actions []PriorityAction

	// === Availability ===
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	if workerCount < 2 {
		actions = append(actions, PriorityAction{
			Title: "添加工作节点实现高可用", Category: "Availability",
			Impact: 10, Effort: 7, Urgency: 9, Severity: "critical",
			Detail: fmt.Sprintf("当前仅 %d 个工作节点，节点故障=全站中断", workerCount),
			Action: "添加至少 2 个工作节点",
		})
	}

	// === Pod Health ===
	highRestart := 0
	crashPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount >= 5 {
				highRestart++
			}
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashPods++
			}
		}
	}
	if crashPods > 0 {
		actions = append(actions, PriorityAction{
			Title: "修复 CrashLoopBackOff Pod", Category: "Stability",
			Impact: 8, Effort: 4, Urgency: 10, Severity: "critical",
			Detail:   fmt.Sprintf("%d 个 Pod 处于 CrashLoopBackOff", crashPods),
			Action:   "检查应用日志，修复启动失败原因",
			Affected: crashPods,
		})
	}
	if highRestart > 0 {
		actions = append(actions, PriorityAction{
			Title: "调查高频重启 Pod", Category: "Stability",
			Impact: 6, Effort: 5, Urgency: 7, Severity: "high",
			Detail:   fmt.Sprintf("%d 个 Pod 重启 >= 5 次", highRestart),
			Action:   "分析重启模式，检查 OOM 和资源限制",
			Affected: highRestart,
		})
	}

	// === Resource Governance ===
	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}
	if nsCount > 0 && len(quotas.Items) == 0 {
		actions = append(actions, PriorityAction{
			Title: "创建 ResourceQuota", Category: "Governance",
			Impact: 7, Effort: 2, Urgency: 6, Severity: "high",
			Detail:   fmt.Sprintf("0/%d 命名空间有 ResourceQuota", nsCount),
			Action:   "使用 /api/scalability/quota-generator 生成的 YAML 批量创建",
			Affected: nsCount,
		})
	}

	// === Network Security ===
	netpolNS := 0
	npSet := make(map[string]bool)
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			npSet[np.Namespace] = true
		}
	}
	netpolNS = len(npSet)
	if nsCount > 0 && netpolNS < nsCount/2 {
		actions = append(actions, PriorityAction{
			Title: "部署 NetworkPolicy", Category: "Security",
			Impact: 7, Effort: 3, Urgency: 5, Severity: "high",
			Detail:   fmt.Sprintf("仅 %d/%d 命名空间有 NetworkPolicy", netpolNS, nsCount),
			Action:   "使用 /api/security/netpol-generator 批量部署",
			Affected: nsCount - netpolNS,
		})
	}

	// === PDB ===
	deployCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
	}
	if deployCount > 0 && len(pdbs.Items) < deployCount/3 {
		actions = append(actions, PriorityAction{
			Title: "创建 PodDisruptionBudget", Category: "Reliability",
			Impact: 6, Effort: 2, Urgency: 5, Severity: "medium",
			Detail:   fmt.Sprintf("%d PDB / %d Deployment", len(pdbs.Items), deployCount),
			Action:   "使用 /api/operations/pdb-generator 生成的 YAML",
			Affected: deployCount - len(pdbs.Items),
		})
	}

	// === Probes ===
	missingProbes := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes++
			}
		}
	}
	if missingProbes > 0 {
		actions = append(actions, PriorityAction{
			Title: "添加健康探针", Category: "Reliability",
			Impact: 5, Effort: 3, Urgency: 5, Severity: "medium",
			Detail:   fmt.Sprintf("%d 个容器缺少探针", missingProbes),
			Action:   "使用 /api/deployment/probe-generator 生成 patch",
			Affected: missingProbes,
		})
	}

	// === Secrets cleanup ===
	orphanSecrets := 0
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) || sec.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}
		orphanSecrets++
	}
	if orphanSecrets > 50 {
		actions = append(actions, PriorityAction{
			Title: "清理孤立 Secret", Category: "Security",
			Impact: 5, Effort: 2, Urgency: 4, Severity: "medium",
			Detail:   fmt.Sprintf("%d 个 Secret 可能孤立", orphanSecrets),
			Action:   "使用 /api/security/secret-exposure 识别并清理",
			Affected: orphanSecrets,
		})
	}

	// === Security Context ===
	missingSC := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.SecurityContext == nil {
				missingSC++
			}
		}
	}
	if missingSC > 5 {
		actions = append(actions, PriorityAction{
			Title: "添加 SecurityContext", Category: "Security",
			Impact: 6, Effort: 2, Urgency: 6, Severity: "high",
			Detail:   fmt.Sprintf("%d 个容器缺少 securityContext", missingSC),
			Action:   "使用 /api/security/fix-plan 生成 patch 命令",
			Affected: missingSC,
		})
	}

	// Calculate priority score for each action
	for i := range actions {
		a := &actions[i]
		a.Priority = a.Impact*3 + a.Urgency*2 + (10 - a.Effort)
		switch {
		case a.Priority >= 45:
			a.Severity = "critical"
			result.Critical++
		case a.Priority >= 35:
			if a.Severity != "critical" {
				a.Severity = "high"
			}
			result.High++
		default:
			result.Medium++
		}
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Priority > actions[j].Priority
	})
	result.Actions = actions
	result.TotalActions = len(actions)

	// Quick wins: high impact, low effort
	for _, a := range actions {
		if a.Impact >= 6 && a.Effort <= 3 {
			result.QuickWins = append(result.QuickWins, a)
		}
	}

	// Batch plan
	result.BatchPlan = buildActionBatches(actions)

	// Platform score
	if result.TotalActions == 0 {
		result.Score = 100
	} else {
		score := 100 - result.Critical*15 - result.High*8
		if score < 0 {
			score = 0
		}
		result.Score = score
	}

	writeJSON(w, result)
}

func buildActionBatches(actions []PriorityAction) []ActionBatch {
	catMap := make(map[string][]PriorityAction)
	for _, a := range actions {
		catMap[a.Category] = append(catMap[a.Category], a)
	}

	var batches []ActionBatch
	for cat, acts := range catMap {
		summaries := []string{}
		for _, a := range acts {
			summaries = append(summaries, a.Title)
		}
		estTime := "30 分钟"
		totalEffort := 0
		for _, a := range acts {
			totalEffort += a.Effort
		}
		if totalEffort > 20 {
			estTime = "2-4 小时"
		} else if totalEffort > 10 {
			estTime = "1-2 小时"
		}

		batches = append(batches, ActionBatch{
			Title: cat, Category: cat, Count: len(acts),
			EstTime: estTime, Actions: summaries,
		})
	}
	sort.Slice(batches, func(i, j int) bool {
		return batches[i].Count > batches[j].Count
	})
	return batches
}
