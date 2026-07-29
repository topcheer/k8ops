package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.31 — Deployment Dimension (Round 25)
// 1. Deployment Revision Tracker — rollout revision history depth
// 2. Container Lifecycle Hook Audit — preStop/postStart hook coverage
// 3. Pod Topology Constraint Validator — nodeSelector/affinity validation
// ============================================================

// ---------------------------------------------------------------
// 1. Deployment Revision Tracker
// ---------------------------------------------------------------

type DeployRevResult2031 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DeployRevSummary2031 `json:"summary"`
	DeepHistory     []DeployRevEntry2031 `json:"deepHistory"`
	Recommendations []string             `json:"recommendations"`
}

type DeployRevSummary2031 struct {
	TotalDeploys    int `json:"totalDeployments"`
	WithDeepHistory int `json:"withDeepHistory"`
	NoHistoryLimit  int `json:"noHistoryLimit"`
	AvgRevisions    int `json:"avgRevisions"`
}

type DeployRevEntry2031 struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Revisions    int    `json:"revisions"`
	HistoryLimit *int32 `json:"historyLimit"`
}

func (s *Server) handleDeployRevTracker(w http.ResponseWriter, r *http.Request) {
	result := DeployRevResult2031{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	var totalRev int
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		// Get replica sets for this deployment to count revisions
		rsList, _ := s.clientset.AppsV1().ReplicaSets(dep.Namespace).List(r.Context(), metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
		})

		revisions := len(rsList.Items)
		totalRev += revisions

		entry := DeployRevEntry2031{
			Name: dep.Name, Namespace: dep.Namespace,
			Revisions: revisions,
		}

		if dep.Spec.RevisionHistoryLimit != nil {
			entry.HistoryLimit = dep.Spec.RevisionHistoryLimit
		}

		if revisions > 5 {
			result.Summary.WithDeepHistory++
			result.DeepHistory = append(result.DeepHistory, entry)
			score -= 1
		}

		if dep.Spec.RevisionHistoryLimit == nil {
			result.Summary.NoHistoryLimit++
		}
	}

	if result.Summary.TotalDeploys > 0 {
		result.Summary.AvgRevisions = totalRev / result.Summary.TotalDeploys
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.DeepHistory, func(i, j int) bool {
		return result.DeepHistory[i].Revisions > result.DeepHistory[j].Revisions
	})

	if result.Summary.WithDeepHistory > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have >5 old revisions — consider cleanup or lower revisionHistoryLimit", result.Summary.WithDeepHistory))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Lifecycle Hook Audit
// ---------------------------------------------------------------

type LifecycleHookResult2031 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         LifecycleHookSummary2031 `json:"summary"`
	MissingHooks    []LifecycleHookEntry2031 `json:"missingHooks"`
	Recommendations []string                 `json:"recommendations"`
}

type LifecycleHookSummary2031 struct {
	TotalContainers int `json:"totalContainers"`
	WithPreStop     int `json:"withPreStop"`
	WithPostStart   int `json:"withPostStart"`
	NoLifecycle     int `json:"noLifecycle"`
}

type LifecycleHookEntry2031 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleLifecycleHookAudit(w http.ResponseWriter, r *http.Request) {
	result := LifecycleHookResult2031{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			hasPreStop := false
			hasPostStart := false

			if c.Lifecycle != nil {
				if c.Lifecycle.PreStop != nil {
					hasPreStop = true
					result.Summary.WithPreStop++
				}
				if c.Lifecycle.PostStart != nil {
					hasPostStart = true
					result.Summary.WithPostStart++
				}
			}

			if !hasPreStop && !hasPostStart {
				result.Summary.NoLifecycle++
				result.MissingHooks = append(result.MissingHooks, LifecycleHookEntry2031{
					Pod: dep.Name, Namespace: dep.Namespace, Container: c.Name,
				})
				if len(dep.Spec.Template.Spec.Containers) > 1 {
					score -= 1
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.MissingHooks, func(i, j int) bool {
		return result.MissingHooks[i].Namespace < result.MissingHooks[j].Namespace
	})

	if result.Summary.NoLifecycle > 0 && result.Summary.TotalContainers > 0 {
		totalC := result.Summary.TotalContainers
		if totalC < 1 {
			totalC = 1
		}
		pct := result.Summary.NoLifecycle * 100 / totalC
		if pct > 80 {
			result.Recommendations = append(result.Recommendations,
				fmt.Sprintf("%d containers have no lifecycle hooks — add preStop for graceful shutdown", result.Summary.NoLifecycle))
		}
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Topology Constraint Validator
// ---------------------------------------------------------------

type TopoConstraintResult2031 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         TopoConstraintSummary2031 `json:"summary"`
	OverConstrained []TopoConstraintEntry2031 `json:"overConstrained"`
	Recommendations []string                  `json:"recommendations"`
}

type TopoConstraintSummary2031 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithNodeSelector int `json:"withNodeSelector"`
	WithNodeAffinity int `json:"withNodeAffinity"`
	OverConstrained  int `json:"overConstrained"`
}

type TopoConstraintEntry2031 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
}

func (s *Server) handleTopoConstraintValidator(w http.ResponseWriter, r *http.Request) {
	result := TopoConstraintResult2031{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeployments++

		tmpl := dep.Spec.Template.Spec

		if len(tmpl.NodeSelector) > 0 {
			result.Summary.WithNodeSelector++
		}
		if tmpl.Affinity != nil && tmpl.Affinity.NodeAffinity != nil {
			result.Summary.WithNodeAffinity++
		}

		// Check over-constrained: multiple nodeSelector + nodeAffinity required + tolerations
		constraintCount := len(tmpl.NodeSelector)
		if tmpl.Affinity != nil && tmpl.Affinity.NodeAffinity != nil &&
			tmpl.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			constraintCount++
		}

		nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

		// If single node cluster and deployment has constraints, it's over-constrained
		if len(nodeList.Items) <= 1 && constraintCount > 0 {
			result.Summary.OverConstrained++
			result.OverConstrained = append(result.OverConstrained, TopoConstraintEntry2031{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue: "constraints on single-node cluster",
			})
			score -= 2
		}

		// Excessive constraints on multi-node
		if len(nodeList.Items) > 1 && constraintCount >= 3 {
			result.Summary.OverConstrained++
			result.OverConstrained = append(result.OverConstrained, TopoConstraintEntry2031{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue: "excessive topology constraints",
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OverConstrained, func(i, j int) bool {
		return result.OverConstrained[i].Namespace < result.OverConstrained[j].Namespace
	})

	if result.Summary.OverConstrained > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments are over-constrained — relax scheduling constraints for better availability", result.Summary.OverConstrained))
	}

	writeJSON(w, result)
}

// keep import
var _ = corev1.Pod{}
