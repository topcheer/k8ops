package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecurityFixPlanResult generates actionable YAML patch commands for
// security issues found across the cluster. Instead of just reporting
// problems, it provides copy-paste-ready kubectl commands and structured
// fix priorities ranked by impact and effort.
type SecurityFixPlanResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         SecFixSummary   `json:"summary"`
	FixPlans        []SecFixPlan    `json:"fixPlans"`
	QuickWins       []SecFixAction  `json:"quickWins"`
	CriticalPatches []SecFixAction  `json:"criticalPatches"`
	BatchCommands   []SecBatchCmd   `json:"batchCommands"`
	HealthScore     int             `json:"healthScore"`
	EstEffort       string          `json:"estimatedEffort"`
	Recommendations []string        `json:"recommendations"`
}

type SecFixSummary struct {
	TotalIssues        int `json:"totalIssues"`
	CriticalCount      int `json:"criticalCount"`
	HighCount          int `json:"highCount"`
	MediumCount        int `json:"mediumCount"`
	AffectedWorkloads  int `json:"affectedWorkloads"`
	AutoFixable        int `json:"autoFixable"`
	ManualFix          int `json:"manualFix"`
}

type SecFixPlan struct {
	Category    string         `json:"category"`
	Priority    string         `json:"priority"`
	Description string         `json:"description"`
	Actions     []SecFixAction `json:"actions"`
	Impact      string         `json:"impact"`
	Effort      string         `json:"effort"`
}

type SecFixAction struct {
	Workload    string `json:"workload"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Issue       string `json:"issue"`
	Severity    string `json:"severity"`
	FixCommand  string `json:"fixCommand"`
	PatchJSON   string `json:"patchJSON"`
	Category    string `json:"category"`
	AutoFixable bool   `json:"autoFixable"`
}

type SecBatchCmd struct {
	Title         string   `json:"title"`
	Category      string   `json:"category"`
	Commands      []string `json:"commands"`
	AffectedCount int      `json:"affectedCount"`
}

// handleSecurityFixPlan handles GET /api/security/fix-plan
func (s *Server) handleSecurityFixPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecurityFixPlanResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	affectedWorkloads := make(map[string]bool)
	var allActions []SecFixAction

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for i := range d.Spec.Template.Spec.Containers {
			c := &d.Spec.Template.Spec.Containers[i]
			actions := assessContainerSecurity(d.Name, d.Namespace, "Deployment", c)
			for range actions {
				affectedWorkloads[d.Namespace+"/"+d.Name] = true
			}
			allActions = append(allActions, actions...)
		}
	}
	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		for i := range ss.Spec.Template.Spec.Containers {
			c := &ss.Spec.Template.Spec.Containers[i]
			actions := assessContainerSecurity(ss.Name, ss.Namespace, "StatefulSet", c)
			for range actions {
				affectedWorkloads[ss.Namespace+"/"+ss.Name] = true
			}
			allActions = append(allActions, actions...)
		}
	}

	// Namespace-level fixes
	netpolNS := make(map[string]bool)
	for _, np := range netpols.Items {
		netpolNS[np.Namespace] = true
	}
	nsActions := assessNSFixPlan(namespaces.Items, netpolNS)
	allActions = append(allActions, nsActions...)

	// Build summary
	result.Summary.AffectedWorkloads = len(affectedWorkloads)
	result.Summary.TotalIssues = len(allActions)
	for _, a := range allActions {
		switch a.Severity {
		case "critical":
			result.Summary.CriticalCount++
		case "high":
			result.Summary.HighCount++
		case "medium":
			result.Summary.MediumCount++
		}
		if a.AutoFixable {
			result.Summary.AutoFixable++
		} else {
			result.Summary.ManualFix++
		}
	}

	result.FixPlans = groupFixPlans(allActions)

	for _, a := range allActions {
		if a.AutoFixable && a.Severity != "low" {
			result.QuickWins = append(result.QuickWins, a)
		}
		if a.Severity == "critical" {
			result.CriticalPatches = append(result.CriticalPatches, a)
		}
	}

	result.BatchCommands = buildSecBatchCmds(allActions)

	if result.Summary.TotalIssues == 0 {
		result.HealthScore = 100
	} else {
		score := 100 - result.Summary.CriticalCount*15 - result.Summary.HighCount*8 - result.Summary.MediumCount*3
		if score < 0 {
			score = 0
		}
		result.HealthScore = score
	}

	autoFixable := result.Summary.AutoFixable
	manualFix := result.Summary.ManualFix
	switch {
	case result.Summary.CriticalCount > 10:
		result.EstEffort = "2-3 天（需要系统性加固）"
	case manualFix > 20:
		result.EstEffort = "1-2 天（部分需要手动配置）"
	case autoFixable > 10:
		result.EstEffort = "2-4 小时（大部分可自动修复）"
	default:
		result.EstEffort = "1-2 小时"
	}

	result.Recommendations = buildSecFixRecs(&result)
	writeJSON(w, result)
}

func assessContainerSecurity(name, ns, kind string, c *corev1.Container) []SecFixAction {
	var actions []SecFixAction
	patchBase := func(patch string) string {
		return fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"%s","securityContext":%s}]}}}}`, c.Name, patch)
	}
	cmd := func() string {
		return fmt.Sprintf("kubectl patch %s %s -n %s --type=strategic", strings.ToLower(kind), name, ns)
	}

	if c.SecurityContext == nil {
		actions = append(actions, SecFixAction{
			Workload: name, Namespace: ns, Kind: kind,
			Issue:       "容器缺少 securityContext",
			Severity:    "high", Category: "PSS",
			PatchJSON:   patchBase(`{"runAsNonRoot":true,"allowPrivilegeEscalation":false}`),
			FixCommand:  cmd(),
			AutoFixable: true,
		})
	} else {
		sc := c.SecurityContext
		if sc.Privileged != nil && *sc.Privileged {
			actions = append(actions, SecFixAction{
				Workload: name, Namespace: ns, Kind: kind,
				Issue: "特权容器运行（可逃逸隔离）", Severity: "critical", Category: "PSS",
				PatchJSON: patchBase(`{"privileged":false}`), FixCommand: cmd(), AutoFixable: true,
			})
		}
		if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
			actions = append(actions, SecFixAction{
				Workload: name, Namespace: ns, Kind: kind,
				Issue: "以 root 用户运行", Severity: "medium", Category: "PSS",
				PatchJSON: patchBase(`{"runAsNonRoot":true}`), FixCommand: cmd(), AutoFixable: true,
			})
		}
		if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
			actions = append(actions, SecFixAction{
				Workload: name, Namespace: ns, Kind: kind,
				Issue: "允许权限提升", Severity: "medium", Category: "PSS",
				PatchJSON: patchBase(`{"allowPrivilegeEscalation":false}`), FixCommand: cmd(), AutoFixable: true,
			})
		}
		if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
			actions = append(actions, SecFixAction{
				Workload: name, Namespace: ns, Kind: kind,
				Issue: "根文件系统可写", Severity: "low", Category: "PSS",
				PatchJSON: patchBase(`{"readOnlyRootFilesystem":true}`), FixCommand: cmd(), AutoFixable: true,
			})
		}
	}

	if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
		actions = append(actions, SecFixAction{
			Workload: name, Namespace: ns, Kind: kind,
			Issue: "缺少资源限制", Severity: "medium", Category: "Resources",
			AutoFixable: false,
		})
	}

	return actions
}

func assessNSFixPlan(namespaces []corev1.Namespace, netpolNS map[string]bool) []SecFixAction {
	var actions []SecFixAction
	for _, ns := range namespaces {
		if isSystemNamespace(ns.Name) {
			continue
		}
		if ns.Labels["pod-security.kubernetes.io/enforce"] == "" {
			actions = append(actions, SecFixAction{
				Workload: ns.Name, Namespace: ns.Name, Kind: "Namespace",
				Issue:    "未启用 Pod Security Admission",
				Severity: "high", Category: "Admission",
				FixCommand: fmt.Sprintf("kubectl label namespace %s pod-security.kubernetes.io/enforce=restricted", ns.Name),
				PatchJSON:  `{"metadata":{"labels":{"pod-security.kubernetes.io/enforce":"restricted"}}}`,
				AutoFixable: true,
			})
		}
		if !netpolNS[ns.Name] {
			actions = append(actions, SecFixAction{
				Workload: ns.Name, Namespace: ns.Name, Kind: "Namespace",
				Issue:    "缺少默认网络策略",
				Severity: "high", Category: "Network",
				AutoFixable: false,
			})
		}
	}
	return actions
}

func groupFixPlans(actions []SecFixAction) []SecFixPlan {
	catMap := make(map[string][]SecFixAction)
	for _, a := range actions {
		catMap[a.Category] = append(catMap[a.Category], a)
	}

	var plans []SecFixPlan
	for cat, acts := range catMap {
		criticals := 0
		manualCount := 0
		for _, a := range acts {
			if a.Severity == "critical" {
				criticals++
			}
			if !a.AutoFixable {
				manualCount++
			}
		}
		priority := "medium"
		if criticals > 0 {
			priority = "critical"
		} else if len(acts) > 5 {
			priority = "high"
		}
		effort := "低"
		if manualCount > len(acts)/2 {
			effort = "高"
		} else if len(acts) > 10 {
			effort = "中"
		}

		sort.Slice(acts, func(i, j int) bool {
			sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
			return sevOrder[acts[i].Severity] < sevOrder[acts[j].Severity]
		})

		plans = append(plans, SecFixPlan{
			Category: cat, Priority: priority,
			Description: fixPlanDesc(cat), Actions: acts,
			Impact: fixPlanImpact(cat), Effort: effort,
		})
	}

	sort.Slice(plans, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return sevOrder[plans[i].Priority] < sevOrder[plans[j].Priority]
	})

	return plans
}

func fixPlanDesc(cat string) string {
	descs := map[string]string{
		"PSS":        "Pod Security Standards - 安全上下文、特权模式、非 root 运行",
		"Resources":  "资源限制 - CPU/内存 limits 防止资源争抢",
		"Admission":  "准入控制 - Pod Security Admission 策略强制执行",
		"Network":    "网络安全 - 默认拒绝网络策略、流量隔离",
	}
	if d, ok := descs[cat]; ok {
		return d
	}
	return cat
}

func fixPlanImpact(cat string) string {
	impacts := map[string]string{
		"PSS":       "防止容器逃逸、权限提升攻击",
		"Resources": "防止资源耗尽、DoS 攻击",
		"Admission": "强制安全策略执行、阻止不安全 Pod 创建",
		"Network":   "限制爆炸半径、防止横向移动",
	}
	if i, ok := impacts[cat]; ok {
		return i
	}
	return "提升安全姿态"
}

func buildSecBatchCmds(actions []SecFixAction) []SecBatchCmd {
	catCmds := make(map[string][]string)
	catCounts := make(map[string]int)

	for _, a := range actions {
		if !a.AutoFixable || a.FixCommand == "" {
			continue
		}
		catCmds[a.Category] = append(catCmds[a.Category], a.FixCommand)
		catCounts[a.Category]++
	}

	var batches []SecBatchCmd
	for cat, cmds := range catCmds {
		if len(cmds) > 10 {
			cmds = cmds[:10]
		}
		batches = append(batches, SecBatchCmd{
			Title:         fmt.Sprintf("%s 批量修复 (%d 项)", cat, catCounts[cat]),
			Category:      cat,
			Commands:      cmds,
			AffectedCount: catCounts[cat],
		})
	}

	sort.Slice(batches, func(i, j int) bool {
		return batches[i].AffectedCount > batches[j].AffectedCount
	})
	return batches
}

func buildSecFixRecs(r *SecurityFixPlanResult) []string {
	recs := []string{}
	if r.Summary.CriticalCount > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个严重问题，建议立即执行修复", r.Summary.CriticalCount))
	}
	if r.Summary.AutoFixable > 0 {
		recs = append(recs, fmt.Sprintf("%d 个问题可通过 kubectl patch 自动修复，预计耗时 %s", r.Summary.AutoFixable, r.EstEffort))
	}
	if r.Summary.ManualFix > 0 {
		recs = append(recs, fmt.Sprintf("%d 个问题需要手动配置，建议制定修复计划", r.Summary.ManualFix))
	}
	for _, plan := range r.FixPlans {
		if plan.Priority == "critical" || plan.Priority == "high" {
			recs = append(recs, fmt.Sprintf("[%s] %d 项修复，优先级: %s", plan.Category, len(plan.Actions), plan.Priority))
		}
	}
	if len(recs) == 0 {
		recs = append(recs, "安全状态良好，建议持续监控")
	}
	return recs
}

var _ networkingv1.NetworkPolicy
