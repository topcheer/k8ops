package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterFaultToleranceResult evaluates the cluster's ability to survive
// various failure scenarios: node loss, zone outage, control plane failure.
type ClusterFaultToleranceResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         FaultToleranceSummary `json:"summary"`
	Scenarios       []FaultScenario       `json:"scenarios"`
	WeakPoints      []FaultWeakPoint      `json:"weakPoints"`
	ToleranceScore  int                   `json:"toleranceScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type FaultToleranceSummary struct {
	TotalNodes        int  `json:"totalNodes"`
	WorkerNodes       int  `json:"workerNodes"`
	ControlPlaneNodes int  `json:"controlPlaneNodes"`
	Zones             int  `json:"zones"`
	TotalPods         int  `json:"totalPods"`
	SurvivesNodeLoss  bool `json:"survivesNodeLoss"`
	SurvivesZoneLoss  bool `json:"survivesZoneLoss"`
	SurvivesCPFailure bool `json:"survivesControlPlaneFailure"`
}

type FaultScenario struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	AffectedPods int     `json:"affectedPods"`
	TotalPods    int     `json:"totalPods"`
	ImpactPct    float64 `json:"impactPct"`
	Survives     bool    `json:"survives"`
	RecoveryTime string  `json:"recoveryTimeEstimate"`
}

type FaultWeakPoint struct {
	Component  string `json:"component"`
	Risk       string `json:"risk"`
	Severity   string `json:"severity"`
	Mitigation string `json:"mitigation"`
}

// handleClusterFaultTolerance handles GET /api/scalability/cluster-fault-tolerance
func (s *Server) handleClusterFaultTolerance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ClusterFaultToleranceResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	workerNodes := 0
	cpNodes := 0
	zoneSet := make(map[string]bool)
	totalPods := 0

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		if zone, ok := node.Labels[corev1.LabelTopologyZone]; ok && zone != "" {
			zoneSet[zone] = true
		}
		isCP := false
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			isCP = true
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			isCP = true
		}
		if isCP {
			cpNodes++
		} else {
			workerNodes++
		}
	}
	result.Summary.WorkerNodes = workerNodes
	result.Summary.ControlPlaneNodes = cpNodes
	result.Summary.Zones = len(zoneSet)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if pod.Spec.NodeName != "" {
			totalPods++
		}
	}
	result.Summary.TotalPods = totalPods

	// Scenario 1: Single node loss
	nodeLossPods := 0
	if workerNodes > 0 {
		nodeLossPods = totalPods / workerNodes // average per node
	}
	survivesNodeLoss := workerNodes >= 3 && nodeLossPods < totalPods/2
	result.Summary.SurvivesNodeLoss = survivesNodeLoss
	result.Scenarios = append(result.Scenarios, FaultScenario{
		Name: "single-node-loss", Description: "One worker node fails",
		AffectedPods: nodeLossPods, TotalPods: totalPods,
		ImpactPct: safeDivPctFloat(nodeLossPods, totalPods),
		Survives:  survivesNodeLoss, RecoveryTime: estRecovery(workerNodes),
	})

	// Scenario 2: Zone outage
	zoneCount := len(zoneSet)
	if zoneCount == 0 {
		zoneCount = 1
	}
	zoneLossPods := totalPods / zoneCount
	survivesZoneLoss := zoneCount >= 3 && workerNodes >= 3
	result.Summary.SurvivesZoneLoss = survivesZoneLoss
	result.Scenarios = append(result.Scenarios, FaultScenario{
		Name: "zone-outage", Description: "An entire availability zone goes down",
		AffectedPods: zoneLossPods, TotalPods: totalPods,
		ImpactPct: safeDivPctFloat(zoneLossPods, totalPods),
		Survives:  survivesZoneLoss, RecoveryTime: estRecovery(zoneCount),
	})

	// Scenario 3: Control plane failure
	survivesCP := cpNodes >= 3
	result.Summary.SurvivesCPFailure = survivesCP
	result.Scenarios = append(result.Scenarios, FaultScenario{
		Name: "control-plane-failure", Description: "Control plane node(s) fail",
		AffectedPods: 0, TotalPods: totalPods, ImpactPct: 0,
		Survives: survivesCP, RecoveryTime: estRecovery(cpNodes),
	})

	// Weak points
	if workerNodes < 3 {
		result.WeakPoints = append(result.WeakPoints, FaultWeakPoint{
			Component: "worker-nodes", Risk: fmt.Sprintf("Only %d worker nodes", workerNodes),
			Severity: "critical", Mitigation: "Add nodes to reach minimum 3 for HA",
		})
	}
	if zoneCount < 2 {
		result.WeakPoints = append(result.WeakPoints, FaultWeakPoint{
			Component: "topology", Risk: fmt.Sprintf("Only %d zone(s)", zoneCount),
			Severity: "high", Mitigation: "Distribute nodes across multiple zones",
		})
	}
	if cpNodes < 3 {
		result.WeakPoints = append(result.WeakPoints, FaultWeakPoint{
			Component: "control-plane", Risk: fmt.Sprintf("Only %d CP nodes", cpNodes),
			Severity: "high", Mitigation: "Run 3+ control plane nodes for quorum",
		})
	}

	// Score
	result.ToleranceScore = 0
	if survivesNodeLoss {
		result.ToleranceScore += 40
	}
	if survivesZoneLoss {
		result.ToleranceScore += 35
	}
	if survivesCP {
		result.ToleranceScore += 25
	}

	gradeFromScore(&result.Grade, result.ToleranceScore)

	result.Recommendations = []string{
		fmt.Sprintf("容错评估: %d worker, %d CP, %d zones, %d pods", workerNodes, cpNodes, zoneCount, totalPods),
		fmt.Sprintf("存活: 节点故障=%v, Zone故障=%v, CP故障=%v", survivesNodeLoss, survivesZoneLoss, survivesCP),
	}
	if len(result.WeakPoints) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个弱点需修复", len(result.WeakPoints)))
	}
	if result.ToleranceScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 扩展到 >=3 worker 节点, >=3 zones, >=3 CP 节点")
	}

	sort.Slice(result.Scenarios, func(i, j int) bool {
		return result.Scenarios[i].ImpactPct > result.Scenarios[j].ImpactPct
	})

	writeJSON(w, result)
}

func safeDivPctFloat(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

func estRecovery(count int) string {
	switch {
	case count >= 3:
		return "< 2min (auto-recovery)"
	case count >= 1:
		return "5-15min (manual intervention)"
	default:
		return "30min+ (full rebuild)"
	}
}
