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
// v20.62 — Deployment Dimension (Round 30)
// 1. Deployment Generation Tracker — rollout generation depth
// 2. Container Stdin Toggle Audit — stdin/stderr config compliance
// 3. Pod Spread Constraint Audit — topology spread constraint enforcement
// ============================================================

type GenTrackResult2062 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         GenTrackSummary2062 `json:"summary"`
	HighGen         []GenTrackEntry2062 `json:"highGenerationDeploys"`
	Recommendations []string            `json:"recommendations"`
}

type GenTrackSummary2062 struct {
	TotalDeploys int `json:"totalDeployments"`
	HighGenCount int `json:"highGenerationCount"`
	AvgGen       int `json:"avgGeneration"`
}

type GenTrackEntry2062 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Generation int64  `json:"generation"`
}

func (s *Server) handleGenTracker(w http.ResponseWriter, r *http.Request) {
	result := GenTrackResult2062{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	var totalGen int64
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		totalGen += dep.Generation

		if dep.Generation > 50 {
			result.Summary.HighGenCount++
			result.HighGen = append(result.HighGen, GenTrackEntry2062{
				Name: dep.Name, Namespace: dep.Namespace, Generation: dep.Generation,
			})
			score -= 1
		}
	}

	if result.Summary.TotalDeploys > 0 {
		result.Summary.AvgGen = int(totalGen) / result.Summary.TotalDeploys
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.HighGen, func(i, j int) bool { return result.HighGen[i].Generation > result.HighGen[j].Generation })

	if result.Summary.HighGenCount > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments with >50 generations — frequent rollouts", result.Summary.HighGenCount))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Stdin Toggle Audit
// ---------------------------------------------------------------

type StdinResult2062 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         StdinSummary2062 `json:"summary"`
	StdinEnabled    []StdinEntry2062 `json:"stdinEnabledContainers"`
	Recommendations []string         `json:"recommendations"`
}

type StdinSummary2062 struct {
	TotalContainers int `json:"totalContainers"`
	StdinEnabled    int `json:"stdinEnabled"`
	TTYEnabled      int `json:"ttyEnabled"`
}

type StdinEntry2062 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleStdinAudit(w http.ResponseWriter, r *http.Request) {
	result := StdinResult2062{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.StdinEnabled++
				result.StdinEnabled = append(result.StdinEnabled, StdinEntry2062{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
				score -= 1
			}
			if c.TTY {
				result.Summary.TTYEnabled++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.StdinEnabled > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have stdin enabled — disable for production", result.Summary.StdinEnabled))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Spread Constraint Audit
// ---------------------------------------------------------------

type SpreadResult2062 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SpreadSummary2062 `json:"summary"`
	MissingSpread   []SpreadEntry2062 `json:"missingSpreadDeployments"`
	Recommendations []string          `json:"recommendations"`
}

type SpreadSummary2062 struct {
	TotalMultiReplica int `json:"totalMultiReplica"`
	WithSpread        int `json:"withTopologySpread"`
	MissingSpread     int `json:"missingTopologySpread"`
}

type SpreadEntry2062 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSpreadAudit2062(w http.ResponseWriter, r *http.Request) {
	result := SpreadResult2062{ScannedAt: time.Now()}
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

		hasSpread := len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0
		if hasSpread {
			result.Summary.WithSpread++
		} else {
			result.Summary.MissingSpread++
			result.MissingSpread = append(result.MissingSpread, SpreadEntry2062{
				Name: dep.Name, Namespace: dep.Namespace,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.MissingSpread, func(i, j int) bool { return result.MissingSpread[i].Namespace < result.MissingSpread[j].Namespace })

	if result.Summary.MissingSpread > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica deployments without topology spread", result.Summary.MissingSpread))
	}
	writeJSON(w, result)
}

// keep import
var _ = appsv1.Deployment{}
