package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PDBGapAnalysisResult analyzes PodDisruptionBudget coverage gaps.
type PDBGapAnalysisResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         PDBGapSummary   `json:"summary"`
	ByNamespace     []PDBGapNSEntry `json:"byNamespace"`
	UnprotectedDeps []PDBGapEntry   `json:"unprotectedDeployments"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type PDBGapSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	MultiReplicaDeps int `json:"multiReplicaDeployments"`
	WithPDB          int `json:"deploymentsWithPDB"`
	WithoutPDB       int `json:"deploymentsWithoutPDB"`
	MismatchedPDB    int `json:"mismatchedPDB"`
}

type PDBGapNSEntry struct {
	Namespace   string `json:"namespace"`
	Deployments int    `json:"deployments"`
	WithPDB     int    `json:"withPDB"`
	Unprotected int    `json:"unprotected"`
}

type PDBGapEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	HasPDB    bool   `json:"hasPDB"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// handlePDBGapAnalysis handles GET /api/scalability/pdb-gap-analysis
func (s *Server) handlePDBGapAnalysis(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PDBGapAnalysisResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build PDB selector map
	pdbSelectors := make(map[string]bool) // ns + label match
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil {
			for k, v := range pdb.Spec.Selector.MatchLabels {
				pdbSelectors[pdb.Namespace+"/"+k+"="+v] = true
			}
		}
	}

	nsMap := make(map[string]*PDBGapNSEntry)

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if nsMap[dep.Namespace] == nil {
			nsMap[dep.Namespace] = &PDBGapNSEntry{Namespace: dep.Namespace}
		}
		nsMap[dep.Namespace].Deployments++

		if replicas < 2 {
			continue // single replica, PDB less critical
		}
		result.Summary.MultiReplicaDeps++

		// Check if PDB covers this deployment
		hasPDB := false
		for k, v := range dep.Spec.Selector.MatchLabels {
			if pdbSelectors[dep.Namespace+"/"+k+"="+v] {
				hasPDB = true
				break
			}
		}

		entry := PDBGapEntry{
			Name: dep.Name, Namespace: dep.Namespace,
			Replicas: replicas, HasPDB: hasPDB,
		}

		if hasPDB {
			result.Summary.WithPDB++
			nsMap[dep.Namespace].WithPDB++
		} else {
			result.Summary.WithoutPDB++
			nsMap[dep.Namespace].Unprotected++
			entry.Reason = "multi-replica deployment without PDB"
			entry.Severity = "high"
			result.UnprotectedDeps = append(result.UnprotectedDeps, entry)
		}
	}

	for _, e := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Unprotected > result.ByNamespace[j].Unprotected
	})

	if result.Summary.MultiReplicaDeps > 0 {
		result.HealthScore = result.Summary.WithPDB * 100 / result.Summary.MultiReplicaDeps
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("PDB 缺口分析: %d 部署, %d 多副本, %d 有 PDB, %d 无 PDB",
			result.Summary.TotalDeployments, result.Summary.MultiReplicaDeps,
			result.Summary.WithPDB, result.Summary.WithoutPDB),
	}
	if result.Summary.WithoutPDB > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个多副本部署无 PDB, 节点维护时可全部驱逐", result.Summary.WithoutPDB))
	}
	writeJSON(w, result)
}

var _ policyv1.PodDisruptionBudgetSpec
var _ appsv1.DeploymentSpec
