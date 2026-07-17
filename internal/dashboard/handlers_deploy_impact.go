package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployImpactResult is the deployment impact simulator.
// It answers: "what will happen if I change/deploy workload X?"
type DeployImpactResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	Summary          ImpactSummary      `json:"summary"`
	TargetWorkload   string             `json:"targetWorkload,omitempty"`
	TargetNamespace  string             `json:"targetNamespace,omitempty"`
	Simulations      []ImpactSimulation `json:"simulations"`
	RankedWorkloads  []RankedWorkload   `json:"rankedWorkloads"` // workloads ranked by deployment risk
	CascadeRisks     []CascadeRisk      `json:"cascadeRisks"`
	Recommendations  []string           `json:"recommendations"`
	ClusterRiskLevel string             `json:"clusterRiskLevel"` // low, medium, high
}

// ImpactSummary aggregates impact simulation stats.
type ImpactSummary struct {
	TotalWorkloads         int `json:"totalWorkloads"`
	HighImpactWorkloads    int `json:"highImpactWorkloads"` // deploying them risks cascading
	MediumImpactWorkloads  int `json:"mediumImpactWorkloads"`
	LowImpactWorkloads     int `json:"lowImpactWorkloads"`
	CriticalDependencies   int `json:"criticalDependencies"` // workloads others depend on
	SingleReplicaWorkloads int `json:"singleReplicaWorkloads"`
	NoPDBWorkloads         int `json:"noPDBWorkloads"`
}

// ImpactSimulation simulates deployment impact for one workload.
type ImpactSimulation struct {
	Workload          string   `json:"workload"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	ImpactLevel       string   `json:"impactLevel"` // high, medium, low
	RiskScore         int      `json:"riskScore"`   // 0-100, higher = more risky to deploy
	PodsAffected      int      `json:"podsAffected"`
	EstimatedDowntime string   `json:"estimatedDowntime"`
	DirectDependents  int      `json:"directDependents"` // services selecting this workload
	SharesNode        int      `json:"sharesNode"`       // other workloads sharing node
	CascadeRisk       string   `json:"cascadeRisk"`      // none, low, medium, high
	Blockers          []string `json:"blockers,omitempty"`
	Mitigations       []string `json:"mitigations,omitempty"`
}

// RankedWorkload ranks workloads by deployment risk.
type RankedWorkload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	RiskScore int    `json:"riskScore"`
	Rank      int    `json:"rank"` // 1 = most risky to deploy
	Reason    string `json:"reason"`
}

// CascadeRisk describes a potential cascading failure path.
type CascadeRisk struct {
	From     string   `json:"from"`
	To       []string `json:"to"`
	Chain    string   `json:"chain"` // human-readable chain
	Severity string   `json:"severity"`
}

// handleDeployImpact simulates deployment impact across the cluster.
// GET /api/deployment/impact-simulator
func (s *Server) handleDeployImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployImpactResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulSets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build service selector map: ns -> selector labels -> service name
	type svcSelector struct {
		Name     string
		Selector map[string]string
	}
	svcByNS := map[string][]svcSelector{}
	if services != nil {
		for _, svc := range services.Items {
			if svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
				svcByNS[svc.Namespace] = append(svcByNS[svc.Namespace], svcSelector{
					Name: svc.Name, Selector: svc.Spec.Selector,
				})
			}
		}
	}

	// Build PDB selector list
	pdbSelectors := buildPDBSelectors(pdbs)

	// Build node sharing map: nodeName -> set of unique workloads
	nodeWorkloads := map[string]map[string]bool{}
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
				continue
			}
			if nodeWorkloads[pod.Spec.NodeName] == nil {
				nodeWorkloads[pod.Spec.NodeName] = map[string]bool{}
			}
			// Get workload name from labels
			wlKey := fmt.Sprintf("%s/%s", pod.Namespace, getWorkloadKey(&pod))
			nodeWorkloads[pod.Spec.NodeName][wlKey] = true
		}
	}

	// Analyze each workload
	var simulations []ImpactSimulation
	var ranked []RankedWorkload
	var cascadeRisks []CascadeRisk
	summary := ImpactSummary{}

	analyze := func(name, namespace, kind string, spec corev1.PodSpec, replicas *int32, available int32, labels map[string]string) {
		sim := ImpactSimulation{
			Workload: name, Namespace: namespace, Kind: kind,
		}

		replicaCount := 0
		if replicas != nil {
			replicaCount = int(*replicas)
		}
		sim.PodsAffected = replicaCount

		riskScore := 0
		var blockers, mitigations []string

		// Factor 1: Single replica = high risk
		if replicaCount <= 1 && kind != "DaemonSet" {
			riskScore += 55
			blockers = append(blockers, "single replica — update will cause downtime")
			mitigations = append(mitigations, "scale to >=2 replicas before deploying")
			summary.SingleReplicaWorkloads++
		} else if replicaCount <= 2 && kind != "DaemonSet" {
			riskScore += 15
			mitigations = append(mitigations, "only 2 replicas — rolling update may cause brief degradation")
		}

		// Factor 2: No PDB = higher risk
		hasPDB := false
		for _, sel := range pdbSelectors {
			if labelsMatchSelector(labels, sel) {
				hasPDB = true
				break
			}
		}
		if !hasPDB && replicaCount > 1 {
			riskScore += 20
			mitigations = append(mitigations, "add PDB before deploying")
			summary.NoPDBWorkloads++
		}

		// Factor 3: Direct dependents (services selecting this workload)
		dependentCount := 0
		if selectors, ok := svcByNS[namespace]; ok {
			for _, ss := range selectors {
				if labelsMatchSelector(labels, ss.Selector) {
					dependentCount++
				}
			}
		}
		sim.DirectDependents = dependentCount
		if dependentCount >= 3 {
			riskScore += 25
			summary.CriticalDependencies++
			mitigations = append(mitigations, fmt.Sprintf("%d services depend on this — high blast radius", dependentCount))
		} else if dependentCount >= 1 {
			riskScore += 10
		}

		// Factor 4: Node sharing (co-located workloads affected during drain)
		nodeShareCount := 0
		if pods != nil {
			for _, pod := range pods.Items {
				if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
					podLabels := pod.Labels
					if labelsMatchSelector(labels, podLabels) && pod.Namespace == namespace {
						if workloads, ok := nodeWorkloads[pod.Spec.NodeName]; ok {
							nodeShareCount += len(workloads) - 1 // exclude self
						}
					}
				}
			}
		}
		sim.SharesNode = nodeShareCount
		if nodeShareCount > 10 {
			riskScore += 10
		}

		// Factor 5: Update strategy
		if kind == "Deployment" {
			// Default is RollingUpdate — check template strategy
		}

		// Factor 6: Missing health probes
		hasReadiness := false
		for _, c := range spec.Containers {
			if c.ReadinessProbe != nil {
				hasReadiness = true
				break
			}
		}
		if !hasReadiness {
			riskScore += 15
			blockers = append(blockers, "no readiness probe — traffic may route to unready pods during rollout")
			mitigations = append(mitigations, "add readiness probe before deploying")
		}

		// Compute impact level
		riskScore = clampScore(riskScore)
		sim.RiskScore = riskScore

		switch {
		case riskScore >= 60:
			sim.ImpactLevel = "high"
			sim.CascadeRisk = "high"
			sim.EstimatedDowntime = "likely (30s-5min)"
			summary.HighImpactWorkloads++
		case riskScore >= 30:
			sim.ImpactLevel = "medium"
			sim.CascadeRisk = "medium"
			sim.EstimatedDowntime = "brief (0-30s)"
			summary.MediumImpactWorkloads++
		default:
			sim.ImpactLevel = "low"
			sim.CascadeRisk = "low"
			sim.EstimatedDowntime = "none expected"
			summary.LowImpactWorkloads++
		}

		sim.Blockers = blockers
		sim.Mitigations = mitigations

		simulations = append(simulations, sim)

		// Rank
		ranked = append(ranked, RankedWorkload{
			Name: name, Namespace: namespace, Kind: kind,
			RiskScore: riskScore,
			Reason:    topRiskReason(blockers, riskScore, replicaCount, dependentCount),
		})

		// Cascade risk if has dependents and high risk
		if dependentCount > 0 && riskScore >= 50 {
			depNames := []string{}
			if selectors, ok := svcByNS[namespace]; ok {
				for _, ss := range selectors {
					if labelsMatchSelector(labels, ss.Selector) {
						depNames = append(depNames, ss.Name)
					}
				}
			}
			cascadeRisks = append(cascadeRisks, CascadeRisk{
				From:     fmt.Sprintf("%s/%s", namespace, name),
				To:       depNames,
				Chain:    fmt.Sprintf("%s/%s → [%s]", namespace, name, strings.Join(depNames, ", ")),
				Severity: "high",
			})
		}
	}

	// Process all workload types
	if deployments != nil {
		for _, d := range deployments.Items {
			if isSystemNSReliability(d.Namespace) {
				continue
			}
			analyze(d.Name, d.Namespace, "Deployment", d.Spec.Template.Spec,
				d.Spec.Replicas, d.Status.AvailableReplicas, d.Spec.Selector.MatchLabels)
		}
	}
	if statefulSets != nil {
		for _, ss := range statefulSets.Items {
			if isSystemNSReliability(ss.Namespace) {
				continue
			}
			analyze(ss.Name, ss.Namespace, "StatefulSet", ss.Spec.Template.Spec,
				ss.Spec.Replicas, ss.Status.AvailableReplicas, ss.Spec.Selector.MatchLabels)
		}
	}

	// Rank workloads
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].RiskScore > ranked[j].RiskScore
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	// Limit simulations to top 30 by risk
	sort.Slice(simulations, func(i, j int) bool {
		return simulations[i].RiskScore > simulations[j].RiskScore
	})
	if len(simulations) > 30 {
		simulations = simulations[:30]
	}

	// Summary
	summary.TotalWorkloads = len(ranked)
	result.Summary = summary
	result.Simulations = simulations
	result.RankedWorkloads = ranked[:min(20, len(ranked))]
	result.CascadeRisks = cascadeRisks

	// Cluster risk level
	highCount := summary.HighImpactWorkloads
	switch {
	case highCount > 5:
		result.ClusterRiskLevel = "high"
	case highCount > 0:
		result.ClusterRiskLevel = "medium"
	default:
		result.ClusterRiskLevel = "low"
	}

	result.Recommendations = generateImpactRecs(result)

	writeJSON(w, result)
}

// getWorkloadKey extracts a workload identifier from pod labels.
func getWorkloadKey(pod *corev1.Pod) string {
	for _, key := range []string{"app", "app.kubernetes.io/name", "k8s-app"} {
		if v, ok := pod.Labels[key]; ok {
			return v
		}
	}
	return pod.Name
}

// topRiskReason returns the primary reason for a high risk score.
func topRiskReason(blockers []string, score, replicas, dependents int) string {
	if len(blockers) > 0 {
		return blockers[0]
	}
	if replicas <= 1 {
		return "single replica — no HA"
	}
	if dependents >= 3 {
		return fmt.Sprintf("%d dependent services — high blast radius", dependents)
	}
	if score >= 30 {
		return "moderate deployment risk"
	}
	return "low risk — safe to deploy"
}

// generateImpactRecs produces recommendations.
func generateImpactRecs(result DeployImpactResult) []string {
	var recs []string

	if result.Summary.HighImpactWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d high-impact workload(s) — deploying them risks downtime or cascading failures", result.Summary.HighImpactWorkloads))
	}
	if result.Summary.SingleReplicaWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d single-replica workload(s) — scale up before deploying", result.Summary.SingleReplicaWorkloads))
	}
	if result.Summary.NoPDBWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) without PDB — add PDB before deploying", result.Summary.NoPDBWorkloads))
	}
	if len(result.CascadeRisks) > 0 {
		recs = append(recs, fmt.Sprintf("%d cascade risk path(s) — deploying high-risk workloads can impact dependents", len(result.CascadeRisks)))
	}

	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("Cluster deployment risk is %s — all workloads have proper safeguards", result.ClusterRiskLevel))
	}

	return recs
}

// Ensure imports used
var _ policyv1.PodDisruptionBudget
var _ appsv1.Deployment
