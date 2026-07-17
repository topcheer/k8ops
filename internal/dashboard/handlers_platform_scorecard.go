package dashboard

import (
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformScorecardResult is the unified platform engineering scorecard.
// It aggregates signals from health, DORA, reliability, security, cost,
// and maturity dimensions into a single executive-level platform score.
type PlatformScorecardResult struct {
	ScannedAt          time.Time              `json:"scannedAt"`
	OverallScore       int                    `json:"overallScore"`
	Grade              string                 `json:"grade"`
	Level              string                 `json:"level"` // elite, advanced, intermediate, developing, initial
	Dimensions         []ScorecardDim         `json:"dimensions"`
	Strengths          []ScorecardStrength    `json:"strengths"`
	Weaknesses         []ScorecardWeakness    `json:"weaknesses"`
	ImprovementRoadmap []ScorecardRoadmapItem `json:"improvementRoadmap"`
	TrendDirection     string                 `json:"trendDirection"` // improving, stable, declining
	ExecutiveSummary   string                 `json:"executiveSummary"`
	Recommendations    []string               `json:"recommendations"`
}

// ScorecardDim is one dimension of the platform scorecard.
type ScorecardDim struct {
	Name       string               `json:"name"`
	Score      int                  `json:"score"`
	Weight     float64              `json:"weight"`
	Status     string               `json:"status"`
	Detail     string               `json:"detail"`
	SubMetrics []ScorecardSubMetric `json:"subMetrics,omitempty"`
}

// ScorecardSubMetric is a granular metric within a dimension.
type ScorecardSubMetric struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Value string `json:"value"`
}

// ScorecardStrength describes a platform strong point.
type ScorecardStrength struct {
	Dimension string `json:"dimension"`
	Detail    string `json:"detail"`
}

// ScorecardWeakness describes a platform gap.
type ScorecardWeakness struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"`
	Detail    string `json:"detail"`
}

// RoadmapItem is a prioritized improvement action.
type ScorecardRoadmapItem struct {
	Priority  int    `json:"priority"`
	Dimension string `json:"dimension"`
	Action    string `json:"action"`
	Impact    string `json:"impact"`   // high, medium, low
	Effort    string `json:"effort"`   // high, medium, low
	Timeline  string `json:"timeline"` // quick-win, short-term, long-term
}

// handlePlatformScorecard handles GET /api/docs/platform-scorecard
func (s *Server) handlePlatformScorecard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PlatformScorecardResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// === 1. Infrastructure Health (weight: 25%) ===
	infraScore, infraDetail := computeInfraScore(nodes.Items, pods.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Infrastructure Health", Score: infraScore, Weight: 0.25,
		Status: scoreToStatusDim(infraScore), Detail: infraDetail,
	})

	// === 2. Workload Reliability (weight: 20%) ===
	relScore, relDetail, relSubs := computeReliabilityScore(deployments.Items, statefulsets.Items, daemonsets.Items, hpas.Items, pdbs.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Workload Reliability", Score: relScore, Weight: 0.20,
		Status: scoreToStatusDim(relScore), Detail: relDetail, SubMetrics: relSubs,
	})

	// === 3. Security Posture (weight: 20%) ===
	secScore, secDetail := computeSecurityScoreQuick(pods.Items, namespaces.Items, deployments.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Security Posture", Score: secScore, Weight: 0.20,
		Status: scoreToStatusDim(secScore), Detail: secDetail,
	})

	// === 4. Cost & Resource Efficiency (weight: 15%) ===
	costScore, costDetail := computeCostScoreQuick(pods.Items, nodes.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Cost Efficiency", Score: costScore, Weight: 0.15,
		Status: scoreToStatusDim(costScore), Detail: costDetail,
	})

	// === 5. Operational Maturity (weight: 10%) ===
	matScore, matDetail := computeMaturityScoreQuick(services.Items, namespaces.Items, hpas.Items, pdbs.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Operational Maturity", Score: matScore, Weight: 0.10,
		Status: scoreToStatusDim(matScore), Detail: matDetail,
	})

	// === 6. Service Connectivity (weight: 10%) ===
	connScore, connDetail := computeConnectivityScore(services.Items, deployments.Items, pods.Items)
	result.Dimensions = append(result.Dimensions, ScorecardDim{
		Name: "Service Connectivity", Score: connScore, Weight: 0.10,
		Status: scoreToStatusDim(connScore), Detail: connDetail,
	})

	// Compute weighted overall score
	weightedSum := 0.0
	for _, d := range result.Dimensions {
		weightedSum += float64(d.Score) * d.Weight
	}
	result.OverallScore = int(weightedSum)
	result.Grade = scoreToGrade(result.OverallScore)
	result.Level = scoreToLevel(result.OverallScore)

	// Identify strengths and weaknesses
	for _, d := range result.Dimensions {
		if d.Score >= 80 {
			result.Strengths = append(result.Strengths, ScorecardStrength{Dimension: d.Name, Detail: d.Detail})
		}
		if d.Score < 60 {
			result.Weaknesses = append(result.Weaknesses, ScorecardWeakness{Dimension: d.Name, Score: d.Score, Detail: d.Detail})
		}
	}

	// Generate roadmap
	result.ImprovementRoadmap = generateScorecardRoadmap(result.Dimensions)

	// Determine trend (simplified: based on overall score)
	switch {
	case result.OverallScore >= 80:
		result.TrendDirection = "improving"
	case result.OverallScore >= 60:
		result.TrendDirection = "stable"
	default:
		result.TrendDirection = "declining"
	}

	// Executive summary
	result.ExecutiveSummary = fmt.Sprintf(
		"Platform score: %d/100 (Grade %s, %s). %d strengths, %d weaknesses identified. %s.",
		result.OverallScore, result.Grade, result.Level,
		len(result.Strengths), len(result.Weaknesses), result.TrendDirection)

	// Recommendations
	result.Recommendations = generatePlatformScorecardRecs(result)

	writeJSON(w, result)
}

// computeInfraScore evaluates infrastructure health.
func computeInfraScore(nodes []corev1.Node, pods []corev1.Pod) (int, string) {
	score := 100
	readyNodes := 0
	for _, n := range nodes {
		isReady := false
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if isReady {
			readyNodes++
		}
		// Penalize node pressure conditions
		for _, c := range n.Status.Conditions {
			if (c.Type == corev1.NodeMemoryPressure || c.Type == corev1.NodeDiskPressure ||
				c.Type == corev1.NodePIDPressure) && c.Status == corev1.ConditionTrue {
				score -= 10
			}
		}
	}
	if len(nodes) > 0 && readyNodes < len(nodes) {
		badRatio := float64(len(nodes)-readyNodes) / float64(len(nodes))
		score -= int(badRatio * 30)
	}
	crashLoop := 0
	pending := 0
	for _, p := range pods {
		if p.Status.Phase == corev1.PodPending {
			pending++
		}
		for _, cs := range p.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoop++
			}
		}
	}
	score -= minInt(crashLoop*3, 15)
	score -= minInt(pending, 10)
	if score < 0 {
		score = 0
	}
	return score, fmt.Sprintf("%d/%d nodes ready, %d crash-loop pods, %d pending", readyNodes, len(nodes), crashLoop, pending)
}

// computeReliabilityScore evaluates workload HA and reliability.
func computeReliabilityScore(deps []appsv1.Deployment, stss []appsv1.StatefulSet, dss []appsv1.DaemonSet, hpas []autoscalingv2.HorizontalPodAutoscaler, pdbs []policyv1.PodDisruptionBudget) (int, string, []ScorecardSubMetric) {
	score := 100
	totalWorkloads := len(deps) + len(stss) + len(dss)

	// HA check: replicas >= 2
	singleReplica := 0
	for _, d := range deps {
		if d.Spec.Replicas != nil && *d.Spec.Replicas < 2 {
			singleReplica++
		}
	}
	haScore := 100
	if totalWorkloads > 0 {
		badRatio := float64(singleReplica) / float64(totalWorkloads)
		haScore = 100 - int(badRatio*50)
	}

	// PDB coverage
	pdbScore := 100
	if totalWorkloads > 0 {
		pdbRatio := float64(len(pdbs)) / float64(totalWorkloads)
		pdbScore = int(pdbRatio * 100)
		if pdbScore > 100 {
			pdbScore = 100
		}
	}

	// HPA coverage
	hpaScore := 100
	if totalWorkloads > 0 {
		hpaRatio := float64(len(hpas)) / float64(totalWorkloads)
		hpaScore = int(hpaRatio * 100)
		if hpaScore > 100 {
			hpaScore = 100
		}
	}

	// Weighted average
	score = (haScore*40 + pdbScore*30 + hpaScore*30) / 100

	return score, fmt.Sprintf("HA: %d%%, PDB: %d%%, HPA: %d%%", haScore, pdbScore, hpaScore),
		[]ScorecardSubMetric{
			{Name: "HA Coverage", Score: haScore, Value: fmt.Sprintf("%d single-replica of %d", singleReplica, totalWorkloads)},
			{Name: "PDB Coverage", Score: pdbScore, Value: fmt.Sprintf("%d PDBs", len(pdbs))},
			{Name: "HPA Coverage", Score: hpaScore, Value: fmt.Sprintf("%d HPAs", len(hpas))},
		}
}

// computeSecurityScoreQuick evaluates security posture from pod security contexts.
func computeSecurityScoreQuick(pods []corev1.Pod, namespaces []corev1.Namespace, deps []appsv1.Deployment) (int, string) {
	score := 100
	privileged := 0
	runAsRoot := 0
	noSC := 0

	for _, pod := range pods {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext == nil {
				noSC++
				continue
			}
			if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				privileged++
			}
			if c.SecurityContext.RunAsUser == nil || (c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0) {
				runAsRoot++
			}
		}
	}

	score -= minInt(privileged*5, 25)
	score -= minInt(runAsRoot*2, 15)
	score -= minInt(noSC, 10)
	if score < 0 {
		score = 0
	}
	return score, fmt.Sprintf("%d privileged, %d root, %d without security context", privileged, runAsRoot, noSC)
}

// computeCostScoreQuick evaluates resource efficiency.
func computeCostScoreQuick(pods []corev1.Pod, nodes []corev1.Node) (int, string) {
	score := 100
	noRequests := 0
	noLimits := 0
	overProvisioned := 0

	for _, pod := range pods {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests == nil || len(c.Resources.Requests) == 0 {
				noRequests++
			}
			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
				noLimits++
			}
			// Check over-provisioning: limit >> request
			if c.Resources.Requests != nil && c.Resources.Limits != nil {
				reqCPU := c.Resources.Requests.Cpu()
				limCPU := c.Resources.Limits.Cpu()
				if reqCPU != nil && limCPU != nil {
					if limCPU.MilliValue() > reqCPU.MilliValue()*4 {
						overProvisioned++
					}
				}
			}
		}
	}

	score -= minInt(noRequests, 20)
	score -= minInt(noLimits, 10)
	score -= minInt(overProvisioned, 15)
	if score < 0 {
		score = 0
	}
	return score, fmt.Sprintf("%d without requests, %d without limits, %d over-provisioned", noRequests, noLimits, overProvisioned)
}

// computeMaturityScoreQuick evaluates operational maturity indicators.
func computeMaturityScoreQuick(services []corev1.Service, namespaces []corev1.Namespace, hpas []autoscalingv2.HorizontalPodAutoscaler, pdbs []policyv1.PodDisruptionBudget) (int, string) {
	score := 0
	indicators := 0

	// Namespace governance
	if len(namespaces) > 0 {
		nsScore := minInt(len(namespaces)*5, 30)
		score += nsScore
		indicators++
	}

	// Service count indicates platform adoption
	if len(services) > 5 {
		score += 20
	} else if len(services) > 0 {
		score += 10
	}
	indicators++

	// HPA adoption
	if len(hpas) > 0 {
		score += minInt(len(hpas)*5, 25)
	}
	indicators++

	// PDB adoption
	if len(pdbs) > 0 {
		score += minInt(len(pdbs)*5, 25)
	}
	indicators++

	if score > 100 {
		score = 100
	}
	return score, fmt.Sprintf("%d namespaces, %d services, %d HPAs, %d PDBs", len(namespaces), len(services), len(hpas), len(pdbs))
}

// computeConnectivityScore evaluates service mesh and connectivity posture.
func computeConnectivityScore(services []corev1.Service, deps []appsv1.Deployment, pods []corev1.Pod) (int, string) {
	score := 100

	// Check for services without endpoints
	orphanServices := 0
	for _, svc := range services {
		if isSystemNamespace(svc.Namespace) || len(svc.Spec.Selector) == 0 {
			continue
		}
		matched := false
		for _, pod := range pods {
			if pod.Namespace != svc.Namespace {
				continue
			}
			allMatch := true
			for k, v := range svc.Spec.Selector {
				if pod.Labels[k] != v {
					allMatch = false
					break
				}
			}
			if allMatch {
				matched = true
				break
			}
		}
		if !matched {
			orphanServices++
		}
	}

	score -= minInt(orphanServices*3, 20)

	// Ingress availability check
	ingressCount := 0
	for _, svc := range services {
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
			ingressCount++
		}
	}
	if ingressCount == 0 && len(services) > 3 {
		score -= 10 // No external access configured
	}

	if score < 0 {
		score = 0
	}
	return score, fmt.Sprintf("%d orphan services, %d ingress points", orphanServices, ingressCount)
}

// scoreToStatusDim converts score to status string.
func scoreToStatusDim(score int) string {
	switch {
	case score >= 80:
		return "healthy"
	case score >= 60:
		return "warning"
	default:
		return "critical"
	}
}

// scoreToLevel converts score to maturity level.
func scoreToLevel(score int) string {
	switch {
	case score >= 90:
		return "elite"
	case score >= 75:
		return "advanced"
	case score >= 60:
		return "intermediate"
	case score >= 40:
		return "developing"
	default:
		return "initial"
	}
}

// generateScorecardRoadmap creates prioritized improvement actions.
func generateScorecardRoadmap(dims []ScorecardDim) []ScorecardRoadmapItem {
	var items []ScorecardRoadmapItem
	prio := 1

	// Sort dimensions by score (lowest first)
	sorted := make([]ScorecardDim, len(dims))
	copy(sorted, dims)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score < sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	for _, d := range sorted {
		if d.Score >= 80 {
			continue
		}
		timeline := "short-term"
		effort := "medium"
		if d.Score < 40 {
			timeline = "long-term"
			effort = "high"
		} else if d.Score >= 70 {
			timeline = "quick-win"
			effort = "low"
		}
		impact := "high"
		if d.Weight < 0.15 {
			impact = "medium"
		}
		items = append(items, ScorecardRoadmapItem{
			Priority:  prio,
			Dimension: d.Name,
			Action:    fmt.Sprintf("Improve %s from %d to 80+ — %s", d.Name, d.Score, d.Detail),
			Impact:    impact,
			Effort:    effort,
			Timeline:  timeline,
		})
		prio++
	}
	return items
}

// generateScorecardRecs produces executive recommendations.
func generatePlatformScorecardRecs(r PlatformScorecardResult) []string {
	var recs []string

	recs = append(recs, r.ExecutiveSummary)

	for _, d := range r.Dimensions {
		recs = append(recs, fmt.Sprintf("%s: %d/100 (%.0f%% weight, %s) — %s", d.Name, d.Score, d.Weight*100, d.Status, d.Detail))
	}

	if len(r.Strengths) > 0 {
		recs = append(recs, fmt.Sprintf("Strengths: %d dimensions scoring 80+", len(r.Strengths)))
	}

	if len(r.Weaknesses) > 0 {
		recs = append(recs, fmt.Sprintf("Weaknesses: %d dimensions scoring below 60 — see improvement roadmap", len(r.Weaknesses)))
	}

	if len(r.ImprovementRoadmap) > 0 {
		top := r.ImprovementRoadmap[0]
		recs = append(recs, fmt.Sprintf("Top priority: %s (%s, %s effort, %s)", top.Action, top.Impact, top.Effort, top.Timeline))
	}

	return recs
}
