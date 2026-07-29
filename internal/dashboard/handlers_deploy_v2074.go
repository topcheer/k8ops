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
// v20.74 — Deployment Dimension (Round 32)
// 1. Deployment Progress Deadline Audit — progressDeadlineSeconds config
// 2. Container Working Dir Validator — workingDir consistency
// 3. ReplicaSet Orphan Detector — RS without deployment owner
// ============================================================

type ProgressDeadlineResult2074 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         ProgressDeadlineSummary2074 `json:"summary"`
	NoDeadline      []ProgressDeadlineEntry2074 `json:"noDeadlineDeploys"`
	Recommendations []string                    `json:"recommendations"`
}

type ProgressDeadlineSummary2074 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithDeadline int `json:"withProgressDeadline"`
	NoDeadline   int `json:"noProgressDeadline"`
}

type ProgressDeadlineEntry2074 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleProgressDeadlineAudit(w http.ResponseWriter, r *http.Request) {
	result := ProgressDeadlineResult2074{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.ProgressDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		} else {
			result.Summary.NoDeadline++
			result.NoDeadline = append(result.NoDeadline, ProgressDeadlineEntry2074{
				Name: dep.Name, Namespace: dep.Namespace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoDeadline, func(i, j int) bool { return result.NoDeadline[i].Namespace < result.NoDeadline[j].Namespace })

	if result.Summary.NoDeadline > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments without progressDeadlineSeconds — set for rollout failure detection", result.Summary.NoDeadline))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Working Dir Validator
// ---------------------------------------------------------------

type WorkDirResult2074 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         WorkDirSummary2074 `json:"summary"`
	CustomWorkDir   []WorkDirEntry2074 `json:"customWorkDirContainers"`
	Recommendations []string           `json:"recommendations"`
}

type WorkDirSummary2074 struct {
	TotalContainers int `json:"totalContainers"`
	CustomWorkDir   int `json:"customWorkDir"`
}

type WorkDirEntry2074 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	WorkDir   string `json:"workingDir"`
}

func (s *Server) handleWorkDirValidator(w http.ResponseWriter, r *http.Request) {
	result := WorkDirResult2074{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.WorkingDir != "" {
				result.Summary.CustomWorkDir++
				result.CustomWorkDir = append(result.CustomWorkDir, WorkDirEntry2074{
					Pod: pod.Name, Namespace: pod.Namespace, WorkDir: c.WorkingDir,
				})
			}
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.CustomWorkDir, func(i, j int) bool { return result.CustomWorkDir[i].WorkDir < result.CustomWorkDir[j].WorkDir })

	if result.Summary.CustomWorkDir > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with custom workingDir — verify consistency", result.Summary.CustomWorkDir))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. ReplicaSet Orphan Detector
// ---------------------------------------------------------------

type RSOrphanResult2074 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         RSOrphanSummary2074 `json:"summary"`
	OrphanRS        []RSOrphanEntry2074 `json:"orphanReplicaSets"`
	Recommendations []string            `json:"recommendations"`
}

type RSOrphanSummary2074 struct {
	TotalRS  int `json:"totalReplicaSets"`
	OrphanRS int `json:"orphanReplicaSets"`
}

type RSOrphanEntry2074 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleRSOrphanDetect(w http.ResponseWriter, r *http.Request) {
	result := RSOrphanResult2074{ScannedAt: time.Now()}
	score := 100

	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})

	for _, rs := range rsList.Items {
		result.Summary.TotalRS++

		hasOwner := false
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" || ref.Kind == "StatefulSet" {
				hasOwner = true
				break
			}
		}

		if !hasOwner && rs.Status.Replicas == 0 {
			result.Summary.OrphanRS++
			result.OrphanRS = append(result.OrphanRS, RSOrphanEntry2074{
				Name: rs.Name, Namespace: rs.Namespace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OrphanRS, func(i, j int) bool { return result.OrphanRS[i].Namespace < result.OrphanRS[j].Namespace })

	if result.Summary.OrphanRS > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d orphaned ReplicaSets — clean up via kubectl delete", result.Summary.OrphanRS))
	}
	writeJSON(w, result)
}
