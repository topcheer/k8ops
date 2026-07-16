package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpotReadinessResult analyzes spot/preemptible instance readiness:
// interruption handling, PDB coverage, node labels for spot,
// and pod disruption budget adequacy for spot workloads.
type SpotReadinessDeepResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SpotDeepSummary     `json:"summary"`
	SpotWorkloads   []SpotWorkload      `json:"spotWorkloads"`
	DisruptionGaps  []DisruptionGap     `json:"disruptionGaps"`
	ReadinessScore  int                 `json:"readinessScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type SpotDeepSummary struct {
	TotalNodes      int  `json:"totalNodes"`
	SpotNodes       int  `json:"spotNodes"`
	HasSpotLabel    bool `json:"hasSpotLabel"`
	WorkloadsWithToleration int `json:"workloadsWithToleration"`
	WorkloadsWithNodeSelector int `json:"workloadsWithNodeSelector"`
	PDBCoverage     int  `json:"pdbCoverage"`
}

type SpotWorkload struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	HasSpotToleration bool `json:"hasSpotToleration"`
	HasNodeAffinity   bool `json:"hasNodeAffinity"`
}

type DisruptionGap struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Gap       string `json:"gap"`
	Severity  string `json:"severity"`
}

// handleSpotReadiness analyzes spot/preemptible instance readiness.
// GET /api/scalability/spot-readiness-deep
func (s *Server) handleSpotReadinessDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SpotReadinessDeepResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Detect spot nodes via labels
	spotNodeLabels := []string{"spot", "preemptible", "karpenter.sh/capacity-type"}
	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		for lk, lv := range node.Labels {
			for _, spot := range spotNodeLabels {
				if fmt.Sprintf("%s=%s", lk, lv) != "" && (strings.Contains(strings.ToLower(lk), spot) || strings.Contains(strings.ToLower(lv), spot)) {
					result.Summary.SpotNodes++
					result.Summary.HasSpotLabel = true
					break
				}
			}
		}
	}

	// Build PDB namespace map
	pdbNS := map[string]bool{}
	for _, pdb := range pdbs.Items {
		pdbNS[pdb.Namespace] = true
		result.Summary.PDBCoverage++
	}

	// Check deployment spot readiness
	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] { continue }
		hasSpotTol := false
		hasNodeAff := false

		for _, tol := range dep.Spec.Template.Spec.Tolerations {
			if tol.Key == "spot" || tol.Key == "karpenter.sh/capacity-type" || strings.Contains(strings.ToLower(tol.Key), "spot") {
				hasSpotTol = true
				result.Summary.WorkloadsWithToleration++
			}
		}
		if dep.Spec.Template.Spec.NodeSelector != nil {
			for k, v := range dep.Spec.Template.Spec.NodeSelector {
				if strings.Contains(strings.ToLower(k), "spot") || strings.Contains(strings.ToLower(v), "spot") {
					hasNodeAff = true
					result.Summary.WorkloadsWithNodeSelector++
				}
			}
		}

		result.SpotWorkloads = append(result.SpotWorkloads, SpotWorkload{
			Name: dep.Name, Namespace: dep.Namespace,
			HasSpotToleration: hasSpotTol, HasNodeAffinity: hasNodeAff,
		})

		// PDB gap
		if !pdbNS[dep.Namespace] {
			replicas := int32(1)
			if dep.Spec.Replicas != nil { replicas = *dep.Spec.Replicas }
			if replicas > 1 {
				result.DisruptionGaps = append(result.DisruptionGaps, DisruptionGap{
					Workload: dep.Name, Namespace: dep.Namespace,
					Gap: "No PDB — pods can be evicted without quorum guarantee",
					Severity: "high",
				})
			}
		}
	}

	// Score
	score := 50
	if result.Summary.HasSpotLabel { score += 20 }
	if result.Summary.WorkloadsWithToleration > 0 { score += 15 }
	if result.Summary.PDBCoverage > 0 { score += 15 }
	result.ReadinessScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.ReadinessScore)

	sort.Slice(result.DisruptionGaps, func(i, j int) bool {
		return result.DisruptionGaps[i].Severity > result.DisruptionGaps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Spot readiness: %d/100 (grade %s) — %d spot nodes, %d PDBs", result.ReadinessScore, result.Grade, result.Summary.SpotNodes, result.Summary.PDBCoverage))
	if !result.Summary.HasSpotLabel {
		recs = append(recs, "No spot/preemptible nodes detected — consider spot instances for cost savings (50-90% cheaper)")
	}
	if len(result.DisruptionGaps) > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without PDB — add PodDisruptionBudget for graceful eviction", len(result.DisruptionGaps)))
	}
	if len(recs) == 1 {
		recs = append(recs, "Spot readiness is good — workloads tolerate disruption")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
