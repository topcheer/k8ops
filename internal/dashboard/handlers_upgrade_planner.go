package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradePlannerResult generates a Kubernetes upgrade plan by analyzing
// current cluster version, API deprecations, and workload compatibility.
type UpgradePlannerResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         UpgradePlanSummary      `json:"summary"`
	CurrentVersion  string                  `json:"currentVersion"`
	TargetVersion   string                  `json:"targetVersion"`
	Blockers        []UpgradePlannerBlocker `json:"blockers"`
	Checklist       []UpgradeChecklistItem  `json:"checklist"`
	ReadinessScore  int                     `json:"readinessScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type UpgradePlanSummary struct {
	TotalWorkloads      int `json:"totalWorkloads"`
	CompatibleWorkloads int `json:"compatibleWorkloads"`
	AtRiskWorkloads     int `json:"atRiskWorkloads"`
	DeprecatedAPIs      int `json:"deprecatedAPIs"`
	BlockerCount        int `json:"blockerCount"`
}

type UpgradePlannerBlocker struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Action   string `json:"action"`
}

type UpgradeChecklistItem struct {
	Step     string `json:"step"`
	Category string `json:"category"`
	Status   string `json:"status"` // ready, action-needed, blocked
	Command  string `json:"command"`
}

// handleUpgradePlanner handles GET /api/docs/upgrade-planner
func (s *Server) handleUpgradePlanner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := UpgradePlannerResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Get current version
	currentVer := "unknown"
	if len(nodes.Items) > 0 {
		currentVer = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}
	result.CurrentVersion = currentVer
	result.TargetVersion = "next-minor"

	// Analyze workloads
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		// Check API version compatibility
		apiVer := d.APIVersion
		if apiVer == "extensions/v1beta1" || apiVer == "apps/v1beta1" || apiVer == "apps/v1beta2" {
			result.Summary.AtRiskWorkloads++
			result.Summary.DeprecatedAPIs++
		} else {
			result.Summary.CompatibleWorkloads++
		}
	}

	// Build blockers
	var blockers []UpgradePlannerBlocker

	// Single node = upgrade risk
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	if workerCount < 2 {
		blockers = append(blockers, UpgradePlannerBlocker{
			Category: "HA", Severity: "high",
			Detail: "单节点集群升级会导致服务中断",
			Action: "升级前添加工作节点",
		})
	}

	// PVC count (drain risk)
	pvcCount := 0
	for _, pvc := range pvcs.Items {
		if !isSystemNamespace(pvc.Namespace) {
			pvcCount++
		}
	}
	if pvcCount > 10 {
		blockers = append(blockers, UpgradePlannerBlocker{
			Category: "Storage", Severity: "medium",
			Detail: fmt.Sprintf("%d 个 PVC，节点 drain 可能缓慢", pvcCount),
			Action: "提前规划 drain 顺序，预留维护窗口",
		})
	}

	// Deprecated APIs
	if result.Summary.DeprecatedAPIs > 0 {
		blockers = append(blockers, UpgradePlannerBlocker{
			Category: "API", Severity: "critical",
			Detail: fmt.Sprintf("%d 个工作负载使用已废弃的 API 版本", result.Summary.DeprecatedAPIs),
			Action: "迁移到 apps/v1 API 版本",
		})
	}

	result.Blockers = blockers
	result.Summary.BlockerCount = len(blockers)

	// Checklist
	result.Checklist = []UpgradeChecklistItem{
		{Step: "备份 etcd", Category: "Pre-Upgrade", Status: "action-needed",
			Command: "ETCDCTL_API=3 etcdctl snapshot save /backup/etcd-snap.db"},
		{Step: "检查 API 废弃", Category: "Pre-Upgrade", Status: func() string {
			if result.Summary.DeprecatedAPIs > 0 {
				return "blocked"
			}
			return "ready"
		}(), Command: "kubectl get deployments --all-namespaces -o jsonpath='{range .items[*]}{.metadata.name}:{.apiVersion}{\"\\n\"}{end}'"},
		{Step: "验证节点容量", Category: "Pre-Upgrade", Status: func() string {
			if workerCount < 2 {
				return "blocked"
			}
			return "ready"
		}(), Command: "kubectl top nodes"},
		{Step: "检查 CRD 兼容性", Category: "Pre-Upgrade", Status: "action-needed",
			Command: "kubectl get crds"},
		{Step: "Drain 第一个节点", Category: "During-Upgrade", Status: "action-needed",
			Command: "kubectl drain <node> --ignore-daemonsets --delete-emptydir-data"},
		{Step: "升级节点", Category: "During-Upgrade", Status: "action-needed",
			Command: "# Follow your infrastructure-specific upgrade procedure"},
		{Step: "Uncordon 节点", Category: "During-Upgrade", Status: "action-needed",
			Command: "kubectl uncordon <node>"},
		{Step: "验证 Pod 恢复", Category: "Post-Upgrade", Status: "action-needed",
			Command: "kubectl get pods --all-namespaces --field-selector=status.phase!=Running"},
		{Step: "验证服务健康", Category: "Post-Upgrade", Status: "action-needed",
			Command: "curl -s https://k8ops.iot2.win/api/cluster/overview"},
	}

	// Score
	score := 100
	for _, b := range blockers {
		switch b.Severity {
		case "critical":
			score -= 30
		case "high":
			score -= 20
		case "medium":
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	result.ReadinessScore = score

	switch {
	case score >= 80:
		result.Grade = "A"
	case score >= 60:
		result.Grade = "B"
	case score >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = []string{
		fmt.Sprintf("升级就绪度: %d/100 (%s), 当前版本: %s", score, result.Grade, currentVer),
	}
	if result.Summary.BlockerCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个升级阻碍需要先解决", result.Summary.BlockerCount))
	}
	for _, b := range blockers {
		if b.Severity == "critical" {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("[CRITICAL] %s: %s", b.Category, b.Action))
		}
	}

	writeJSON(w, result)
}
