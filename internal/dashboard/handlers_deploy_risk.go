package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployRiskResult provides a comprehensive pre-deployment risk assessment
// with weighted scoring across multiple dimensions.
type DeployRiskResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	OverallRisk     int                `json:"overallRisk"`
	Verdict         string             `json:"verdict"`
	RiskFactors     []DeployRiskFactor `json:"riskFactors"`
	RiskMatrix      []DeployRiskMatrix `json:"riskMatrix"`
	Recommendations []string           `json:"recommendations"`
}

type DeployRiskFactor struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	Risk       int    `json:"risk"`
	Weight     int    `json:"weight"`
	Detail     string `json:"detail"`
	Mitigation string `json:"mitigation"`
}

type DeployRiskMatrix struct {
	Category string `json:"category"`
	Risk     int    `json:"risk"`
	Status   string `json:"status"`
}

// handleDeployRisk handles GET /api/deployment/deploy-risk
func (s *Server) handleDeployRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployRiskResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	var factors []DeployRiskFactor

	// 1. Single node risk
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	nodeRisk := 0
	if workerCount < 2 {
		nodeRisk = 90
	} else if workerCount < 3 {
		nodeRisk = 30
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Single Node", Category: "Availability", Risk: nodeRisk, Weight: 25,
		Detail:     fmt.Sprintf("%d worker nodes", workerCount),
		Mitigation: "Deploy at least 3 worker nodes",
	})

	// 2. Pod crash rate
	totalPods := 0
	crashPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		totalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashPods++
				break
			}
		}
	}
	crashRisk := 0
	if totalPods > 0 {
		crashRisk = crashPods * 100 / totalPods
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Pod Crash Rate", Category: "Stability", Risk: crashRisk, Weight: 20,
		Detail:     fmt.Sprintf("%d/%d pods crashing", crashPods, totalPods),
		Mitigation: "Fix CrashLoopBackOff workloads",
	})

	// 3. High restart rate
	highRestart := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		totalR := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalR += int(cs.RestartCount)
		}
		if totalR >= 5 {
			highRestart++
		}
	}
	restartRisk := 0
	if totalPods > 0 {
		restartRisk = highRestart * 100 / totalPods
	}
	factors = append(factors, DeployRiskFactor{
		Name: "High Restart Rate", Category: "Stability", Risk: restartRisk, Weight: 15,
		Detail:     fmt.Sprintf("%d pods with >= 5 restarts", highRestart),
		Mitigation: "Investigate restart causes",
	})

	// 4. Missing PDB
	deployCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			deployCount++
		}
	}
	pdbRisk := 50
	if deployCount > 0 {
		if len(pdbs.Items) >= deployCount/2 {
			pdbRisk = 10
		} else if len(pdbs.Items) > 0 {
			pdbRisk = 40
		}
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Missing PDBs", Category: "Reliability", Risk: pdbRisk, Weight: 15,
		Detail:     fmt.Sprintf("%d PDBs / %d deployments", len(pdbs.Items), deployCount),
		Mitigation: "Create PDBs for multi-replica workloads",
	})

	// 5. Missing probes
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
	probeRisk := 30
	if missingProbes > 10 {
		probeRisk = 60
	} else if missingProbes > 5 {
		probeRisk = 40
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Missing Probes", Category: "Reliability", Risk: probeRisk, Weight: 10,
		Detail:     fmt.Sprintf("%d containers without probes", missingProbes),
		Mitigation: "Add liveness/readiness probes",
	})

	// 6. Missing resource limits
	noLimits := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				noLimits++
			}
		}
	}
	limitRisk := 20
	if noLimits > 15 {
		limitRisk = 50
	} else if noLimits > 5 {
		limitRisk = 30
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Missing Resource Limits", Category: "Resources", Risk: limitRisk, Weight: 10,
		Detail:     fmt.Sprintf("%d containers without limits", noLimits),
		Mitigation: "Set CPU/memory limits on all containers",
	})

	// 7. Recreate strategy
	recreateCount := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType && ptrInt32(d.Spec.Replicas) > 1 {
			recreateCount++
		}
	}
	stratRisk := 5
	if recreateCount > 0 {
		stratRisk = 40
	}
	factors = append(factors, DeployRiskFactor{
		Name: "Recreate Strategy", Category: "Deployment", Risk: stratRisk, Weight: 5,
		Detail:     fmt.Sprintf("%d multi-replica with Recreate", recreateCount),
		Mitigation: "Switch to RollingUpdate",
	})

	// Calculate weighted risk
	weightedSum := 0
	totalWeight := 0
	for _, f := range factors {
		weightedSum += f.Risk * f.Weight
		totalWeight += f.Weight
	}
	if totalWeight > 0 {
		result.OverallRisk = weightedSum / totalWeight
	}

	switch {
	case result.OverallRisk >= 60:
		result.Verdict = "risky"
	case result.OverallRisk >= 35:
		result.Verdict = "cautious"
	default:
		result.Verdict = "safe"
	}

	// Risk matrix by category
	catMap := make(map[string]*DeployRiskMatrix)
	for _, f := range factors {
		if _, ok := catMap[f.Category]; !ok {
			catMap[f.Category] = &DeployRiskMatrix{Category: f.Category}
		}
		if f.Risk > catMap[f.Category].Risk {
			catMap[f.Category].Risk = f.Risk
		}
	}
	for _, m := range catMap {
		if m.Risk >= 60 {
			m.Status = "critical"
		} else if m.Risk >= 40 {
			m.Status = "high"
		} else if m.Risk >= 20 {
			m.Status = "moderate"
		} else {
			m.Status = "safe"
		}
		result.RiskMatrix = append(result.RiskMatrix, *m)
	}
	sort.Slice(result.RiskMatrix, func(i, j int) bool {
		return result.RiskMatrix[i].Risk > result.RiskMatrix[j].Risk
	})

	sort.Slice(factors, func(i, j int) bool {
		return factors[i].Risk > factors[j].Risk
	})
	result.RiskFactors = factors

	result.Recommendations = buildDeployRiskRecs(&result)
	writeJSON(w, result)
}

func buildDeployRiskRecs(r *DeployRiskResult) []string {
	recs := []string{
		fmt.Sprintf("Overall deployment risk: %d/100 (%s)", r.OverallRisk, r.Verdict),
	}
	for _, f := range r.RiskFactors {
		if f.Risk >= 50 {
			recs = append(recs, fmt.Sprintf("[%s] Risk %d: %s -> %s", f.Name, f.Risk, f.Detail, f.Mitigation))
		}
	}
	if len(recs) <= 1 {
		recs = append(recs, "Deployment risk is manageable, proceed with standard process")
	}
	return recs
}
