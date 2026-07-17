package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KnowledgeBaseResult generates a human-readable knowledge base from the
// live cluster state. It documents best practices, common pitfalls, and
// operational runbooks derived from actual cluster configuration findings.
type KnowledgeBaseResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	Summary         KBSummary   `json:"summary"`
	Sections        []KBSection `json:"sections"`
	Runbooks        []KBRunbook `json:"runbooks"`
	FAQ             []KBFAQ     `json:"faq"`
	Recommendations []string    `json:"recommendations"`
}

type KBSummary struct {
	TotalArticles int `json:"totalArticles"`
	Sections      int `json:"sections"`
	Runbooks      int `json:"runbooks"`
	FAQCount      int `json:"faqCount"`
}

type KBSection struct {
	Title    string      `json:"title"`
	Category string      `json:"category"`
	Articles []KBArticle `json:"articles"`
}

type KBArticle struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Severity   string `json:"severity"`
	Actionable bool   `json:"actionable"`
}

type KBRunbook struct {
	Title    string   `json:"title"`
	Scenario string   `json:"scenario"`
	Steps    []string `json:"steps"`
	Commands []string `json:"commands"`
}

type KBFAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// handleKnowledgeBase handles GET /api/docs/knowledge-base
func (s *Server) handleKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := KnowledgeBaseResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

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
		if isSystemNamespace(d.Namespace) {
			continue
		}
		deployCount++
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes++
			}
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				missingLimits++
			}
		}
	}

	crashPods := 0
	highRestart := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashPods++
			}
			if cs.RestartCount >= 5 {
				highRestart++
			}
		}
	}

	// Build knowledge base sections
	var sections []KBSection

	// Section 1: Availability
	availArticles := []KBArticle{}
	if workerCount < 2 {
		availArticles = append(availArticles, KBArticle{
			Title:    "单节点集群风险",
			Content:  fmt.Sprintf("当前仅 %d 个工作节点。单节点集群无法容忍节点故障，建议至少 3 个工作节点实现 HA。", workerCount),
			Severity: "critical", Actionable: true,
		})
	}
	availArticles = append(availArticles, KBArticle{
		Title:    "节点维护流程",
		Content:  "1. Drain 节点: kubectl drain <node> --ignore-daemonsets\n2. 维护完成后: kubectl uncordon <node>\n3. 验证 Pod 重新调度",
		Severity: "info", Actionable: true,
	})
	sections = append(sections, KBSection{Title: "高可用", Category: "Availability", Articles: availArticles})

	// Section 2: Stability
	stabArticles := []KBArticle{}
	if crashPods > 0 {
		stabArticles = append(stabArticles, KBArticle{
			Title:    "CrashLoopBackOff 排查",
			Content:  fmt.Sprintf("当前有 %d 个 Pod 处于 CrashLoopBackOff。排查: kubectl logs --previous, 检查 OOM, 检查依赖服务", crashPods),
			Severity: "critical", Actionable: true,
		})
	}
	if highRestart > 0 {
		stabArticles = append(stabArticles, KBArticle{
			Title:    "高频重启分析",
			Content:  fmt.Sprintf("%d 个 Pod 重启 >= 5 次。常见原因: OOM Kill, 探针错误, 资源不足, panic", highRestart),
			Severity: "high", Actionable: true,
		})
	}
	stabArticles = append(stabArticles, KBArticle{
		Title:    "资源限制最佳实践",
		Content:  "CPU request: 生产环境建议 100m-500m 起步。Memory request/limit: 建议 limit/request 比率 <= 2。",
		Severity: "info", Actionable: false,
	})
	sections = append(sections, KBSection{Title: "稳定性", Category: "Stability", Articles: stabArticles})

	// Section 3: Configuration
	configArticles := []KBArticle{}
	if missingProbes > 0 {
		configArticles = append(configArticles, KBArticle{
			Title:    "健康探针配置",
			Content:  fmt.Sprintf("%d 个容器缺少探针。使用 /api/deployment/probe-generator 生成探针 patch。\nlivenessProbe: 检测应用是否存活\nreadinessProbe: 检测应用是否就绪接流量", missingProbes),
			Severity: "high", Actionable: true,
		})
	}
	if missingLimits > 0 {
		configArticles = append(configArticles, KBArticle{
			Title:    "资源限制配置",
			Content:  fmt.Sprintf("%d 个容器缺少资源限制。没有限制的容器可能消耗节点所有资源。\n使用 /api/scalability/right-size-engine 获取建议。", missingLimits),
			Severity: "high", Actionable: true,
		})
	}
	sections = append(sections, KBSection{Title: "配置管理", Category: "Configuration", Articles: configArticles})

	// Runbooks
	result.Runbooks = []KBRunbook{
		{
			Title:    "Pod CrashLoopBackOff 应急响应",
			Scenario: "Pod 持续崩溃重启",
			Steps:    []string{"确认崩溃原因", "检查应用日志", "检查资源配置", "修复并重新部署"},
			Commands: []string{
				"kubectl get pods -n <ns> --field-selector=status.phase=Running",
				"kubectl logs <pod> -n <ns> --previous --tail=50",
				"kubectl describe pod <pod> -n <ns>",
			},
		},
		{
			Title:    "节点资源耗尽应急响应",
			Scenario: "节点 CPU/内存/Pod 数接近上限",
			Steps:    []string{"识别资源消耗大户", "cordon 节点防止新 Pod 调度", "扩容或优化工作负载"},
			Commands: []string{
				"kubectl describe node <node>",
				"kubectl get pods --all-namespaces --field-selector=spec.nodeName=<node> --sort-by=.metadata.creationTimestamp",
				"kubectl cordon <node>",
			},
		},
		{
			Title:    "命名空间资源配额设置",
			Scenario: "命名空间缺少资源限制",
			Steps:    []string{"评估命名空间资源需求", "生成 ResourceQuota YAML", "创建 LimitRange 设置默认值", "验证配额生效"},
			Commands: []string{
				fmt.Sprintf("# 使用 /api/scalability/quota-generator 生成 YAML"),
				"kubectl apply -f quota.yaml -n <ns>",
				"kubectl get resourcequota -n <ns>",
			},
		},
	}

	// FAQ
	result.FAQ = []KBFAQ{
		{Question: "为什么我的 Pod 一直 Pending?", Answer: "通常是资源不足、调度约束（nodeSelector/affinity）不匹配、或 PVC 未绑定。使用 kubectl describe pod 查看 Events。"},
		{Question: "如何减少 Pod 重启?", Answer: "1. 检查 OOM Kill 2. 调整 livenessProbe 3. 增加资源 limit 4. 检查依赖服务稳定性"},
		{Question: "HPA 为什么不工作?", Answer: "HPA 需要 CPU requests 已设置且 metrics-server 正常运行。检查 kubectl describe hpa。"},
		{Question: "如何安全清理孤立资源?", Answer: "使用 /api/scalability/orphan-cleanup 识别孤立 ConfigMap/Secret/PVC，确认后批量删除。"},
		{Question: "PDB 有什么作用?", Answer: "PodDisruptionBudget 确保自愿驱逐（如节点维护）时保持最小可用副本数。使用 /api/operations/pdb-generator 生成。"},
	}

	// Summary counts
	result.Sections = sections
	result.Summary.Sections = len(sections)
	result.Summary.Runbooks = len(result.Runbooks)
	result.Summary.FAQCount = len(result.FAQ)
	for _, sec := range sections {
		result.Summary.TotalArticles += len(sec.Articles)
	}

	result.Recommendations = []string{
		fmt.Sprintf("知识库: %d 篇文章, %d 个运维手册, %d 个 FAQ", result.Summary.TotalArticles, result.Summary.Runbooks, result.Summary.FAQCount),
		"知识库内容基于集群实际配置生成，定期刷新获取最新建议",
	}

	writeJSON(w, result)
}
