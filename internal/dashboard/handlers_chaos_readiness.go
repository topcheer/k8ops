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

// ChaosReadinessResult assesses how well workloads would survive
// chaos engineering experiments such as pod kills, network partitions,
// and node drains. It evaluates resilience primitives like PDB coverage,
// anti-affinity rules, graceful shutdown, health probes, and replica counts.
type ChaosReadinessResult struct {
	ScannedAt        time.Time       `json:"scannedAt"`
	Summary          ChaosSummary    `json:"summary"`
	Workloads        []ChaosWorkload `json:"workloads"`
	FailureScenarios []ChaosScenario `json:"failureScenarios"`
	ByNamespace      []ChaosNS       `json:"byNamespace"`
	HealthScore      int             `json:"healthScore"`
	Grade            string          `json:"grade"`
	Recommendations  []string        `json:"recommendations"`
}

type ChaosSummary struct {
	TotalWorkloads     int `json:"totalWorkloads"`
	ReadyWorkloads     int `json:"readyWorkloads"`
	AtRiskWorkloads    int `json:"atRiskWorkloads"`
	WithPDB            int `json:"withPDB"`
	WithAntiAffinity   int `json:"withAntiAffinity"`
	WithGracefulStop   int `json:"withGracefulShutdown"`
	WithHealthProbe    int `json:"withHealthProbe"`
	WithResourceLimits int `json:"withResourceLimits"`
	MultiReplica       int `json:"multiReplica"`
	AvgReadiness       int `json:"avgReadiness"`
}

type ChaosWorkload struct {
	Name                string   `json:"name"`
	Namespace           string   `json:"namespace"`
	Kind                string   `json:"kind"`
	Replicas            int      `json:"replicas"`
	HasPDB              bool     `json:"hasPDB"`
	HasAntiAffinity     bool     `json:"hasAntiAffinity"`
	GracefulShutdown    bool     `json:"gracefulShutdown"`
	HealthProbeOK       bool     `json:"healthProbeOK"`
	ResourceLimitsOK    bool     `json:"resourceLimitsOK"`
	SurvivePodKill      bool     `json:"survivePodKill"`
	SurviveNodeDrain    bool     `json:"surviveNodeDrain"`
	SurviveNetPartition bool     `json:"surviveNetPartition"`
	ReadinessScore      int      `json:"readinessScore"`
	RiskLevel           string   `json:"riskLevel"`
	Gaps                []string `json:"gaps"`
}

type ChaosScenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ImpactCount int    `json:"impactCount"`
	Severity    string `json:"severity"`
	SafeCount   int    `json:"safeCount"`
}

type ChaosNS struct {
	Namespace   string `json:"namespace"`
	Workloads   int    `json:"workloads"`
	ReadyCount  int    `json:"readyCount"`
	AtRiskCount int    `json:"atRiskCount"`
	AvgScore    int    `json:"avgScore"`
}

// handleChaosReadiness handles GET /api/operations/chaos-readiness
func (s *Server) handleChaosReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChaosReadinessResult{ScannedAt: time.Now()}

	// Collect PDBs
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbMap := make(map[string]bool) // key: namespace/name
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil {
			for _, l := range pdb.Labels {
				_ = l
			}
		}
		// Map by namespace + name pattern from selector matchLabels
		key := pdb.Namespace + "/" + pdb.Name
		pdbMap[key] = true
	}

	// Build PDB namespace coverage map
	pdbNS := make(map[string][]string)
	for _, pdb := range pdbs.Items {
		labels := []string{}
		if pdb.Spec.Selector != nil {
			for k, v := range pdb.Spec.Selector.MatchLabels {
				labels = append(labels, k+"="+v)
			}
		}
		pdbNS[pdb.Namespace] = append(pdbNS[pdb.Namespace], strings.Join(labels, ","))
	}

	// Collect all workloads
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	nodeCount := len(nodes.Items)

	var allWorkloads []ChaosWorkload

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		cw := assessChaosReadiness(
			d.Name, d.Namespace, "Deployment",
			int(ptrInt32(d.Spec.Replicas)), d.Spec.Template, d.Spec.Selector,
			pdbNS[d.Namespace], nodeCount,
		)
		allWorkloads = append(allWorkloads, cw)
	}

	for _, ss := range statefulsets.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		cw := assessChaosReadiness(
			ss.Name, ss.Namespace, "StatefulSet",
			int(ptrInt32(ss.Spec.Replicas)), ss.Spec.Template, ss.Spec.Selector,
			pdbNS[ss.Namespace], nodeCount,
		)
		allWorkloads = append(allWorkloads, cw)
	}

	for _, ds := range daemonsets.Items {
		if isSystemNamespace(ds.Namespace) {
			continue
		}
		cw := assessChaosReadiness(
			ds.Name, ds.Namespace, "DaemonSet",
			nodeCount, ds.Spec.Template, ds.Spec.Selector,
			pdbNS[ds.Namespace], nodeCount,
		)
		// DaemonSet always survives node drain (runs on all nodes)
		cw.SurviveNodeDrain = true
		allWorkloads = append(allWorkloads, cw)
	}

	// Build summary
	result.Workloads = allWorkloads
	nsMap := make(map[string]*ChaosNS)
	totalScore := 0
	for _, cw := range allWorkloads {
		result.Summary.TotalWorkloads++
		totalScore += cw.ReadinessScore

		if cw.HasPDB {
			result.Summary.WithPDB++
		}
		if cw.HasAntiAffinity {
			result.Summary.WithAntiAffinity++
		}
		if cw.GracefulShutdown {
			result.Summary.WithGracefulStop++
		}
		if cw.HealthProbeOK {
			result.Summary.WithHealthProbe++
		}
		if cw.ResourceLimitsOK {
			result.Summary.WithResourceLimits++
		}
		if cw.Replicas >= 2 {
			result.Summary.MultiReplica++
		}
		if cw.ReadinessScore >= 70 {
			result.Summary.ReadyWorkloads++
		} else {
			result.Summary.AtRiskWorkloads++
		}

		if _, ok := nsMap[cw.Namespace]; !ok {
			nsMap[cw.Namespace] = &ChaosNS{Namespace: cw.Namespace}
		}
		ns := nsMap[cw.Namespace]
		ns.Workloads++
		ns.AvgScore += cw.ReadinessScore
		if cw.ReadinessScore >= 70 {
			ns.ReadyCount++
		} else {
			ns.AtRiskCount++
		}
	}

	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgReadiness = totalScore / result.Summary.TotalWorkloads
	}
	result.HealthScore = result.Summary.AvgReadiness

	// Grade
	switch {
	case result.HealthScore >= 85:
		result.Grade = "A"
	case result.HealthScore >= 70:
		result.Grade = "B"
	case result.HealthScore >= 55:
		result.Grade = "C"
	case result.HealthScore >= 40:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	// Namespace breakdown
	for _, ns := range nsMap {
		if ns.Workloads > 0 {
			ns.AvgScore = ns.AvgScore / ns.Workloads
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].AtRiskCount > result.ByNamespace[j].AtRiskCount
	})

	// Failure scenarios
	result.FailureScenarios = buildChaosScenarios(allWorkloads)

	// Recommendations
	result.Recommendations = buildChaosRecommendations(&result)

	// Sort workloads by score ascending (worst first)
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].ReadinessScore < result.Workloads[j].ReadinessScore
	})

	writeJSON(w, result)
}

// assessChaosReadiness evaluates a single workload's chaos resilience.
func assessChaosReadiness(name, ns, kind string, replicas int,
	podSpec corev1.PodTemplateSpec, selector *metav1.LabelSelector,
	pdbSelectors []string, nodeCount int) ChaosWorkload {

	cw := ChaosWorkload{
		Name:      name,
		Namespace: ns,
		Kind:      kind,
		Replicas:  replicas,
		Gaps:      []string{},
	}

	// Check PDB (simplified: namespace has any PDB matching)
	cw.HasPDB = len(pdbSelectors) > 0

	// Check anti-affinity / topology spread
	if podSpec.Spec.Affinity != nil && podSpec.Spec.Affinity.PodAntiAffinity != nil {
		hasHard := false
		for _, term := range podSpec.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			if len(term.TopologyKey) > 0 {
				hasHard = true
				break
			}
		}
		if hasHard || len(podSpec.Spec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
			cw.HasAntiAffinity = true
		}
	}
	for _, ts := range podSpec.Spec.TopologySpreadConstraints {
		if ts.MaxSkew > 0 {
			cw.HasAntiAffinity = true
			break
		}
	}

	// Check graceful shutdown
	if podSpec.Spec.TerminationGracePeriodSeconds != nil {
		cw.GracefulShutdown = *podSpec.Spec.TerminationGracePeriodSeconds >= 10
	} else {
		cw.GracefulShutdown = true // default is 30s
	}

	// Check health probes
	cw.HealthProbeOK = true
	for _, c := range podSpec.Spec.Containers {
		if c.LivenessProbe == nil {
			cw.HealthProbeOK = false
			break
		}
	}

	// Check resource limits
	cw.ResourceLimitsOK = true
	for _, c := range podSpec.Spec.Containers {
		if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
			cw.ResourceLimitsOK = false
			break
		}
	}

	// Failure scenario survival assessment
	cw.SurvivePodKill = replicas >= 2 && cw.HasPDB
	cw.SurviveNodeDrain = replicas >= 2 && nodeCount >= 2 && cw.HasAntiAffinity
	cw.SurviveNetPartition = cw.HasAntiAffinity && nodeCount >= 2

	// Score calculation (max 100)
	score := 0
	if cw.HasPDB {
		score += 20
	} else {
		cw.Gaps = append(cw.Gaps, "缺少 PDB: Pod Disruption Budget 未设置，pod 中断无法控制")
	}
	if cw.HasAntiAffinity {
		score += 20
	} else if replicas >= 2 {
		cw.Gaps = append(cw.Gaps, "缺少反亲和性: 多副本集中部署，单节点故障风险高")
	}
	if cw.GracefulShutdown {
		score += 10
	} else {
		cw.Gaps = append(cw.Gaps, "优雅终止时间不足: terminationGracePeriodSeconds < 10s")
	}
	if cw.HealthProbeOK {
		score += 15
	} else {
		cw.Gaps = append(cw.Gaps, "缺少存活探针: 容器无法自动检测和恢复故障")
	}
	if cw.ResourceLimitsOK {
		score += 15
	} else {
		cw.Gaps = append(cw.Gaps, "缺少资源限制: 无 CPU/内存 limit 可能导致资源争抢")
	}
	if replicas >= 2 {
		score += 20
	} else {
		cw.Gaps = append(cw.Gaps, "单副本部署: 无法容忍任何 pod 故障")
	}

	cw.ReadinessScore = score

	switch {
	case score >= 70:
		cw.RiskLevel = "low"
	case score >= 40:
		cw.RiskLevel = "medium"
	default:
		cw.RiskLevel = "high"
	}

	return cw
}

func buildChaosScenarios(workloads []ChaosWorkload) []ChaosScenario {
	scenarios := []ChaosScenario{
		{
			Name:        "random-pod-kill",
			Description: "随机杀死一个 Pod（Chaos Monkey 风格），验证自愈能力",
		},
		{
			Name:        "node-drain",
			Description: "驱逐一个节点上的所有 Pod，模拟节点维护",
		},
		{
			Name:        "network-partition",
			Description: "网络分区注入，验证服务降级和超时处理",
		},
		{
			Name:        "cpu-stress",
			Description: "CPU 压力注入，验证资源限制和自动扩缩容",
		},
	}

	for i := range scenarios {
		impactCount := 0
		safeCount := 0
		for _, cw := range workloads {
			switch scenarios[i].Name {
			case "random-pod-kill":
				if cw.SurvivePodKill {
					safeCount++
				} else {
					impactCount++
				}
			case "node-drain":
				if cw.SurviveNodeDrain {
					safeCount++
				} else {
					impactCount++
				}
			case "network-partition":
				if cw.SurviveNetPartition {
					safeCount++
				} else {
					impactCount++
				}
			case "cpu-stress":
				if cw.ResourceLimitsOK {
					safeCount++
				} else {
					impactCount++
				}
			}
		}

		if impactCount > safeCount {
			scenarios[i].Severity = "critical"
		} else if impactCount > 0 {
			scenarios[i].Severity = "warning"
		} else {
			scenarios[i].Severity = "safe"
		}
		scenarios[i].ImpactCount = impactCount
		scenarios[i].SafeCount = safeCount
	}

	return scenarios
}

func buildChaosRecommendations(r *ChaosReadinessResult) []string {
	recs := []string{}
	if r.Summary.TotalWorkloads == 0 {
		return recs
	}

	pdbPct := pctInt(r.Summary.WithPDB, r.Summary.TotalWorkloads)
	if pdbPct < 50 {
		recs = append(recs, fmt.Sprintf("PDB 覆盖率仅 %.0f%%，建议为所有多副本工作负载创建 PodDisruptionBudget", pdbPct))
	}
	probePct := pctInt(r.Summary.WithHealthProbe, r.Summary.TotalWorkloads)
	if probePct < 80 {
		recs = append(recs, fmt.Sprintf("健康探针覆盖率 %.0f%%，建议为所有容器添加 livenessProbe 和 readinessProbe", probePct))
	}
	limitPct := pctInt(r.Summary.WithResourceLimits, r.Summary.TotalWorkloads)
	if limitPct < 90 {
		recs = append(recs, fmt.Sprintf("资源限制覆盖率 %.0f%%，建议为所有容器设置 CPU 和 memory limits", limitPct))
	}
	affPct := pctInt(r.Summary.WithAntiAffinity, r.Summary.MultiReplica)
	if r.Summary.MultiReplica > 0 && affPct < 50 {
		recs = append(recs, fmt.Sprintf("多副本工作负载中仅 %.0f%% 配置了反亲和性，建议添加 podAntiAffinity 分散风险", affPct))
	}
	if r.Summary.AtRiskWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("有 %d 个工作负载混沌就绪度低于 70 分，建议优先修复高风险工作负载", r.Summary.AtRiskWorkloads))
	}
	if len(recs) == 0 {
		recs = append(recs, "集群混沌就绪度良好，建议定期执行混沌工程实验验证韧性")
	}
	return recs
}

// pctInt returns the percentage n/d*100 as float64.
func pctInt(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d) * 100
}

// (ptrInt32 is defined in handlers_resources.go)
