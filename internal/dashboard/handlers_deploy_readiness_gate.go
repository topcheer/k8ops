package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployReadinessGateResult evaluates deployment readiness gates: are all
// pre-conditions met for a safe deployment? Checks probes, resources, PDB,
// HPA, and rollback capability in a single composite score.
type DeployReadinessGateResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	Summary          DeployGateSummary `json:"summary"`
	ByWorkload       []DeployGateEntry `json:"byWorkload"`
	BlockedWorkloads []DeployGateEntry `json:"blockedWorkloads"`
	GateChecks       []DeployGateCheck `json:"gateChecks"`
	HealthScore      int               `json:"healthScore"`
	Grade            string            `json:"grade"`
	Recommendations  []string          `json:"recommendations"`
}

type DeployGateSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	ReadyToDeploy  int `json:"readyToDeploy"`
	Blocked        int `json:"blocked"`
}

type DeployGateEntry struct {
	Workload  string   `json:"workload"`
	Namespace string   `json:"namespace"`
	Score     int      `json:"score"`
	Ready     bool     `json:"ready"`
	Blockers  []string `json:"blockers"`
}

type DeployGateCheck struct {
	Name        string `json:"name"`
	PassRate    int    `json:"passRate"`
	Description string `json:"description"`
}

// handleDeployReadinessGate handles GET /api/deployment/readiness-gate
func (s *Server) handleDeployReadinessGate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployReadinessGateResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	hpaMap := make(map[string]bool)
	for _, hpa := range hpas.Items {
		hpaMap[hpa.Namespace+"/"+hpa.Spec.ScaleTargetRef.Name] = true
	}
	pdbNS := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		pdbNS[pdb.Namespace] = true
	}

	checkCounts := map[string]int{"Probes": 0, "Resources": 0, "Multi-Replica": 0, "PDB": 0, "HPA": 0, "RollingUpdate": 0}
	checkTotal := map[string]int{"Probes": 0, "Resources": 0, "Multi-Replica": 0, "PDB": 0, "HPA": 0, "RollingUpdate": 0}

	var entries []DeployGateEntry
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := DeployGateEntry{Workload: d.Name, Namespace: d.Namespace}
		var blockers []string
		score := 100

		replicas := int(ptrInt32(d.Spec.Replicas))
		key := d.Namespace + "/" + d.Name

		// Check probes
		checkTotal["Probes"]++
		missingProbes := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.LivenessProbe == nil || c.ReadinessProbe == nil {
				missingProbes = true
			}
		}
		if !missingProbes {
			checkCounts["Probes"]++
		} else {
			blockers = append(blockers, "Missing probes")
			score -= 25
		}

		// Check resources
		checkTotal["Resources"]++
		missingRes := false
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				missingRes = true
			}
		}
		if !missingRes {
			checkCounts["Resources"]++
		} else {
			blockers = append(blockers, "Missing resource limits")
			score -= 20
		}

		// Check multi-replica
		checkTotal["Multi-Replica"]++
		if replicas >= 2 {
			checkCounts["Multi-Replica"]++
		} else {
			blockers = append(blockers, "Single replica (no HA)")
			score -= 15
		}

		// Check PDB
		checkTotal["PDB"]++
		if pdbNS[d.Namespace] {
			checkCounts["PDB"]++
		} else {
			blockers = append(blockers, "No PDB")
			score -= 10
		}

		// Check HPA
		checkTotal["HPA"]++
		if hpaMap[key] {
			checkCounts["HPA"]++
		} else {
			blockers = append(blockers, "No HPA")
			score -= 10
		}

		// Check RollingUpdate
		checkTotal["RollingUpdate"]++
		if d.Spec.Strategy.Type == "" || d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			checkCounts["RollingUpdate"]++
		} else {
			blockers = append(blockers, "Non-rolling strategy")
			score -= 15
		}

		entry.Score = score
		entry.Blockers = blockers
		entry.Ready = len(blockers) == 0

		if entry.Ready {
			result.Summary.ReadyToDeploy++
		} else {
			result.Summary.Blocked++
			result.BlockedWorkloads = append(result.BlockedWorkloads, entry)
		}

		entries = append(entries, entry)
	}

	// Gate checks
	for _, name := range []string{"Probes", "Resources", "Multi-Replica", "PDB", "HPA", "RollingUpdate"} {
		rate := 0
		if checkTotal[name] > 0 {
			rate = checkCounts[name] * 100 / checkTotal[name]
		}
		result.GateChecks = append(result.GateChecks, DeployGateCheck{
			Name: name, PassRate: rate,
			Description: fmt.Sprintf("%d/%d workloads pass", checkCounts[name], checkTotal[name]),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Score < entries[j].Score
	})
	result.ByWorkload = entries

	if result.Summary.TotalWorkloads > 0 {
		result.HealthScore = result.Summary.ReadyToDeploy * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildReadinessGateRecs(&result)
	writeJSON(w, result)
}

func buildReadinessGateRecs(r *DeployReadinessGateResult) []string {
	recs := []string{
		fmt.Sprintf("部署就绪: %d/%d 工作负载通过所有检查", r.Summary.ReadyToDeploy, r.Summary.TotalWorkloads),
	}
	if r.Summary.Blocked > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载被阻塞", r.Summary.Blocked))
	}
	for _, gc := range r.GateChecks {
		if gc.PassRate < 50 {
			recs = append(recs, fmt.Sprintf("[%s] 仅 %d%% 通过", gc.Name, gc.PassRate))
		}
	}
	return recs
}
