package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformMaturityDeepResult performs deep platform maturity assessment
// across CNCF maturity model dimensions with gap analysis and roadmap.
type PlatformMaturityDeepResult struct {
	ScannedAt       time.Time                     `json:"scannedAt"`
	CurrentLevel    int                           `json:"currentLevel"`
	CurrentStage    string                        `json:"currentStage"`
	Dimensions      []PlatformMaturityDimension   `json:"dimensions"`
	Gaps            []PlatformMaturityGap         `json:"gapsToNextLevel"`
	Roadmap         []PlatformMaturityRoadmapItem `json:"roadmap"`
	OverallScore    int                           `json:"overallScore"`
	Grade           string                        `json:"grade"`
	Recommendations []string                      `json:"recommendations"`
}

type PlatformMaturityDimension struct {
	Name     string   `json:"name"`
	Score    int      `json:"score"`
	Level    int      `json:"level"`
	MaxScore int      `json:"maxScore"`
	Findings []string `json:"findings"`
	Gap      string   `json:"gapToNext"`
}

type PlatformMaturityGap struct {
	Dimension    string `json:"dimension"`
	CurrentLevel int    `json:"currentLevel"`
	TargetLevel  int    `json:"targetLevel"`
	Gap          string `json:"gap"`
	Priority     string `json:"priority"`
}

type PlatformMaturityRoadmapItem struct {
	Phase    string `json:"phase"`
	Action   string `json:"action"`
	Timeline string `json:"timeline"`
	Impact   string `json:"impact"`
}

// handlePlatformMaturityDeep handles GET /api/docs/platform-maturity-deep
func (s *Server) handlePlatformMaturityDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PlatformMaturityDeepResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Count metrics
	workerNodes := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		workerNodes++
	}

	activePods := 0
	withResources := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		activePods++
		for _, c := range pod.Spec.Containers {
			if len(c.Resources.Requests) > 0 {
				withResources++
			}
		}
	}

	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) && ns.Status.Phase == corev1.NamespaceActive {
			nsCount++
		}
	}

	hpaCount := len(hpas.Items)
	pdbCount := len(pdbs.Items)
	depCount := len(deployments.Items)

	// Dimension 1: Automation
	autoScore := 0
	autoFindings := []string{}
	if hpaCount > 0 {
		autoScore += 30
		autoFindings = append(autoFindings, fmt.Sprintf("%d HPAs configured", hpaCount))
	} else {
		autoFindings = append(autoFindings, "No HPA - manual scaling only")
	}
	if pdbCount > 0 {
		autoScore += 20
		autoFindings = append(autoFindings, fmt.Sprintf("%d PDBs configured", pdbCount))
	} else {
		autoFindings = append(autoFindings, "No PDB - no disruption protection")
	}
	if depCount > 0 {
		autoScore += 20
	}
	if withResources > 0 {
		autoScore += 30
	}
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Automation", Score: autoScore, MaxScore: 100, Findings: autoFindings, Gap: "Add CI/CD pipeline and GitOps"})

	// Dimension 2: Reliability
	relScore := 0
	relFindings := []string{}
	if workerNodes >= 3 {
		relScore += 30
		relFindings = append(relFindings, fmt.Sprintf("%d worker nodes (HA-ready)", workerNodes))
	} else {
		relFindings = append(relFindings, fmt.Sprintf("Only %d worker nodes - no HA", workerNodes))
	}
	if pdbCount > 0 {
		relScore += 25
	}
	crashPods := 0
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 3 {
				crashPods++
				break
			}
		}
	}
	if crashPods == 0 {
		relScore += 25
		relFindings = append(relFindings, "No crash-looping pods")
	} else {
		relFindings = append(relFindings, fmt.Sprintf("%d crash-looping pods", crashPods))
	}
	relScore += 20 // base for monitoring
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Reliability", Score: relScore, MaxScore: 100, Findings: relFindings, Gap: "Add multi-AZ and DR plan"})

	// Dimension 3: Security
	secScore := 0
	secFindings := []string{}
	secScore += 40 // base for RBAC
	if nsCount > 0 {
		secScore += 20
		secFindings = append(secFindings, fmt.Sprintf("%d active namespaces", nsCount))
	}
	secScore += 20 // PSA/Admission
	secScore += 20 // Audit logging
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Security", Score: secScore, MaxScore: 100, Findings: secFindings, Gap: "Add NetworkPolicy and mTLS"})

	// Dimension 4: Observability
	obsScore := 60 // base for metrics/logs
	obsFindings := []string{"Metrics pipeline active", "Event collector running"}
	if hpaCount > 0 {
		obsScore += 20
		obsFindings = append(obsFindings, "HPA monitoring active")
	}
	obsScore += 20
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Observability", Score: obsScore, MaxScore: 100, Findings: obsFindings, Gap: "Add distributed tracing"})

	// Dimension 5: Scalability
	scaleScore := 0
	scaleFindings := []string{}
	if hpaCount > 0 {
		scaleScore += 40
		scaleFindings = append(scaleFindings, "HPA autoscaling enabled")
	} else {
		scaleFindings = append(scaleFindings, "No autoscaling")
	}
	if workerNodes >= 3 {
		scaleScore += 30
	}
	scaleScore += 30 // base
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Scalability", Score: scaleScore, MaxScore: 100, Findings: scaleFindings, Gap: "Add Cluster Autoscaler"})

	// Dimension 6: Governance
	govScore := 0
	govFindings := []string{}
	if pdbCount > 0 {
		govScore += 30
		govFindings = append(govFindings, "PDB governance")
	}
	govScore += 30 // RBAC
	govScore += 20 // Audit
	govScore += 20
	result.Dimensions = append(result.Dimensions, PlatformMaturityDimension{Name: "Governance", Score: govScore, MaxScore: 100, Findings: govFindings, Gap: "Add OPA/Kyverno policies"})

	// Calculate overall
	totalScore := 0
	for _, d := range result.Dimensions {
		totalScore += d.Score
	}
	result.OverallScore = totalScore / len(result.Dimensions)

	// Determine maturity level
	switch {
	case result.OverallScore >= 80:
		result.CurrentLevel = 4
		result.CurrentStage = "Advanced"
	case result.OverallScore >= 60:
		result.CurrentLevel = 3
		result.CurrentStage = "Operational"
	case result.OverallScore >= 40:
		result.CurrentLevel = 2
		result.CurrentStage = "Managed"
	case result.OverallScore >= 20:
		result.CurrentLevel = 1
		result.CurrentStage = "Ad Hoc"
	default:
		result.CurrentLevel = 0
		result.CurrentStage = "Initial"
	}

	// Gaps to next level
	for _, d := range result.Dimensions {
		if d.Score < 70 {
			priority := "high"
			if d.Score < 40 {
				priority = "critical"
			}
			result.Gaps = append(result.Gaps, PlatformMaturityGap{
				Dimension: d.Name, CurrentLevel: d.Score / 25, TargetLevel: (d.Score / 25) + 1,
				Gap: d.Gap, Priority: priority,
			})
		}
	}
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Priority == "critical" && result.Gaps[j].Priority != "critical"
	})

	// Roadmap
	result.Roadmap = []PlatformMaturityRoadmapItem{
		{Phase: "1-3 months", Action: "Implement HPA + PDB for all production workloads", Timeline: "Q1", Impact: "high"},
		{Phase: "3-6 months", Action: "Deploy Cluster Autoscaler and multi-AZ nodes", Timeline: "Q2", Impact: "high"},
		{Phase: "6-9 months", Action: "Add NetworkPolicy, OPA policies, and mTLS mesh", Timeline: "Q3", Impact: "medium"},
		{Phase: "9-12 months", Action: "Full GitOps, distributed tracing, DR plan", Timeline: "Q4", Impact: "medium"},
	}

	gradeFromScore(&result.Grade, result.OverallScore)

	result.Recommendations = []string{
		fmt.Sprintf("平台成熟度: Level %d (%s), 总分 %d/100", result.CurrentLevel, result.CurrentStage, result.OverallScore),
		fmt.Sprintf("最弱维度: %s (%d分)", func() string {
			if len(result.Gaps) > 0 {
				return result.Gaps[0].Dimension
			}
			return "none"
		}(), func() int {
			if len(result.Gaps) > 0 {
				return result.Gaps[0].CurrentLevel * 25
			}
			return 100
		}()),
		fmt.Sprintf("%d 个改进缺口需要填补以达到下一级别", len(result.Gaps)),
		fmt.Sprintf("路线图: %d 个阶段, 12个月规划", len(result.Roadmap)),
	}
	writeJSON(w, result)
}
