package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.98 — Deployment Dimension (Round 36)
// 1. Deployment Paused Status — paused rollout detection
// 2. Pod Topology Spread Constraint Validator
// 3. Container Security Context Completeness
// ============================================================

type PausedResult2098 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PausedSummary2098 `json:"summary"`
	PausedDeploys   []PausedEntry2098 `json:"pausedDeployments"`
	Recommendations []string          `json:"recommendations"`
}

type PausedSummary2098 struct {
	TotalDeploys int `json:"totalDeployments"`
	Paused       int `json:"pausedDeployments"`
}

type PausedEntry2098 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePaused2098(w http.ResponseWriter, r *http.Request) {
	result := PausedResult2098{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Paused {
			result.Summary.Paused++
			result.PausedDeploys = append(result.PausedDeploys, PausedEntry2098{Name: dep.Name, Namespace: dep.Namespace})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PausedDeploys, func(i, j int) bool { return result.PausedDeploys[i].Namespace < result.PausedDeploys[j].Namespace })

	if result.Summary.Paused > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments paused — resume for updates", result.Summary.Paused))
	}
	writeJSON(w, result)
}

// 2. Topology Spread Validator
type TopoSpreadResult2098 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         TopoSpreadSummary2098 `json:"summary"`
	Missing         []TopoSpreadEntry2098 `json:"missingSpread"`
	Recommendations []string              `json:"recommendations"`
}

type TopoSpreadSummary2098 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithSpread        int `json:"withTopologySpread"`
}

type TopoSpreadEntry2098 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleTopoSpread2098(w http.ResponseWriter, r *http.Request) {
	result := TopoSpreadResult2098{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			continue
		}
		result.Summary.TotalMultiReplica++

		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithSpread++
		} else {
			result.Missing = append(result.Missing, TopoSpreadEntry2098{Name: dep.Name, Namespace: dep.Namespace})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Missing, func(i, j int) bool { return result.Missing[i].Namespace < result.Missing[j].Namespace })
	writeJSON(w, result)
}

// 3. Security Context Completeness
type SecCtxResult2098 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SecCtxSummary2098 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type SecCtxSummary2098 struct {
	TotalContainers int `json:"totalContainers"`
	WithSecCtx      int `json:"withSecurityContext"`
}

func (s *Server) handleSecCtx2098(w http.ResponseWriter, r *http.Request) {
	result := SecCtxResult2098{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil {
				result.Summary.WithSecCtx++
			} else {
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// keep import
var _ = appsv1.Deployment{}
