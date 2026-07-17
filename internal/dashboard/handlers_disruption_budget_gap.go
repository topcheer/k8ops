package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DisruptionBudgetGapResult identifies workloads lacking PodDisruptionBudgets
// and exposes their risk to voluntary and involuntary disruptions.
type DisruptionBudgetGapResult struct {
	ScannedAt            time.Time            `json:"scannedAt"`
	Summary              DisruptionGapSummary `json:"summary"`
	UnprotectedWorkloads []DisruptionGapEntry `json:"unprotectedWorkloads"`
	ProtectedCount       int                  `json:"protectedCount"`
	RiskScore            int                  `json:"riskScore"`
	Grade                string               `json:"grade"`
	Recommendations      []string             `json:"recommendations"`
}

type DisruptionGapSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	WithPDB         int `json:"withPDB"`
	WithoutPDB      int `json:"withoutPDB"`
	SingleReplica   int `json:"singleReplica"`
	CriticalExposed int `json:"criticalExposed"`
}

type DisruptionGapEntry struct {
	Workload       string `json:"workload"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	Replicas       int    `json:"replicas"`
	HasPDB         bool   `json:"hasPDB"`
	CriticalLabels bool   `json:"criticalLabels"`
	RiskLevel      string `json:"riskLevel"`
	Issue          string `json:"issue"`
}

// handleDisruptionBudgetGap handles GET /api/security/disruption-budget-gap
func (s *Server) handleDisruptionBudgetGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DisruptionBudgetGapResult{ScannedAt: time.Now()}

	// Collect existing PDB selectors (namespace -> set of workload names matched)
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbNamespaces := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		pdbNamespaces[pdb.Namespace+"/"+pdb.Name] = true
	}

	// Helper to check if a workload likely has a PDB
	hasPDB := func(ns, wlName string) bool {
		return pdbNamespaces[ns+"/"+wlName]
	}

	// Scan Deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := 1
		if d.Spec.Replicas != nil {
			replicas = int(*d.Spec.Replicas)
		}
		result.Summary.TotalWorkloads++
		hasBudget := hasPDB(d.Namespace, d.Name)
		critical := d.Labels["critical"] == "true" || d.Labels["app.kubernetes.io/managed-by"] != ""
		if hasBudget {
			result.ProtectedCount++
			result.Summary.WithPDB++
		} else {
			result.Summary.WithoutPDB++
			entry := DisruptionGapEntry{
				Workload:       d.Name,
				Namespace:      d.Namespace,
				Kind:           "Deployment",
				Replicas:       replicas,
				HasPDB:         false,
				CriticalLabels: critical,
			}
			if replicas <= 1 {
				entry.RiskLevel = "high"
				entry.Issue = "Single replica without PDB - full downtime on node drain"
				result.Summary.SingleReplica++
				if critical {
					result.Summary.CriticalExposed++
				}
			} else {
				entry.RiskLevel = "medium"
				entry.Issue = fmt.Sprintf("%d replicas but no PDB protecting voluntary eviction", replicas)
			}
			result.UnprotectedWorkloads = append(result.UnprotectedWorkloads, entry)
		}
	}

	// Scan StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, s := range sts.Items {
		if isSystemNamespace(s.Namespace) {
			continue
		}
		replicas := 1
		if s.Spec.Replicas != nil {
			replicas = int(*s.Spec.Replicas)
		}
		result.Summary.TotalWorkloads++
		hasBudget := hasPDB(s.Namespace, s.Name)
		critical := s.Labels["critical"] == "true"
		if hasBudget {
			result.ProtectedCount++
			result.Summary.WithPDB++
		} else {
			result.Summary.WithoutPDB++
			entry := DisruptionGapEntry{
				Workload:       s.Name,
				Namespace:      s.Namespace,
				Kind:           "StatefulSet",
				Replicas:       replicas,
				HasPDB:         false,
				CriticalLabels: critical,
			}
			if replicas <= 1 {
				entry.RiskLevel = "high"
				entry.Issue = "Single replica StatefulSet without PDB"
				result.Summary.SingleReplica++
				if critical {
					result.Summary.CriticalExposed++
				}
			} else {
				entry.RiskLevel = "medium"
				entry.Issue = fmt.Sprintf("%d replicas, no PDB for ordered eviction", replicas)
			}
			result.UnprotectedWorkloads = append(result.UnprotectedWorkloads, entry)
		}
	}

	// Calculate risk score
	if result.Summary.TotalWorkloads > 0 {
		gapRatio := float64(result.Summary.WithoutPDB) / float64(result.Summary.TotalWorkloads)
		result.RiskScore = int(gapRatio * 100)
	}

	switch {
	case result.RiskScore < 10:
		result.Grade = "A"
	case result.RiskScore < 25:
		result.Grade = "B"
	case result.RiskScore < 50:
		result.Grade = "C"
	case result.RiskScore < 75:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	// Sort by risk level (high first)
	sort.Slice(result.UnprotectedWorkloads, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.UnprotectedWorkloads[i].RiskLevel] < rank[result.UnprotectedWorkloads[j].RiskLevel]
	})

	result.Recommendations = buildDisruptionGapRecs(&result)
	writeJSON(w, result)
}

func buildDisruptionGapRecs(r *DisruptionBudgetGapResult) []string {
	recs := []string{
		fmt.Sprintf("PDB 覆盖: %d/%d 工作负载受保护", r.Summary.WithPDB, r.Summary.TotalWorkloads),
	}
	if r.Summary.CriticalExposed > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个关键工作负载暴露于中断风险", r.Summary.CriticalExposed))
	}
	if r.Summary.SingleReplica > 0 {
		recs = append(recs, fmt.Sprintf("%d 个单副本工作负载无 PDB 保护", r.Summary.SingleReplica))
	}
	if r.RiskScore >= 50 {
		recs = append(recs, "建议: 为所有多副本工作负载创建 PDB，最低可用数设为 N-1")
	}
	return recs
}

// keep appsv1 import
var _ appsv1.DeploymentList
