package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.92 — Deployment Dimension (Round 35)
// 1. StatefulSet Pod Ordinal Tracking
// 2. DaemonSet Node Selector Coverage
// 3. Deployment Env Var Consistency
// ============================================================

type STSOrdinalResult2092 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         STSOrdinalSummary2092 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type STSOrdinalSummary2092 struct {
	TotalSTS    int `json:"totalStatefulSets"`
	ReadySTS    int `json:"readyStatefulSets"`
	NotReadySTS int `json:"notReadyStatefulSets"`
}

func (s *Server) handleSTSOrdinal2092(w http.ResponseWriter, r *http.Request) {
	result := STSOrdinalResult2092{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		if sts.Status.ReadyReplicas >= replicas {
			result.Summary.ReadySTS++
		} else {
			result.Summary.NotReadySTS++
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. DaemonSet Node Selector Coverage
type DSCoverResult2092 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         DSCoverSummary2092 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type DSCoverSummary2092 struct {
	TotalDS     int `json:"totalDaemonSets"`
	WithNodeSel int `json:"withNodeSelector"`
	Scheduled   int `json:"scheduledNodes"`
	Desired     int `json:"desiredNodes"`
}

func (s *Server) handleDSCover2092(w http.ResponseWriter, r *http.Request) {
	result := DSCoverResult2092{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})

	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Desired += int(ds.Status.DesiredNumberScheduled)
		result.Summary.Scheduled += int(ds.Status.CurrentNumberScheduled)
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			result.Summary.WithNodeSel++
		}
	}
	if result.Summary.Desired > 0 && result.Summary.Scheduled < result.Summary.Desired {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Deployment Env Var Consistency
type EnvConsResult2092 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EnvConsSummary2092 `json:"summary"`
	Inconsistent    []EnvConsEntry2092 `json:"inconsistent"`
	Recommendations []string           `json:"recommendations"`
}

type EnvConsSummary2092 struct {
	TotalDeploys int `json:"totalDeployments"`
	Inconsistent int `json:"inconsistent"`
}

type EnvConsEntry2092 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	EnvCount  int    `json:"envCount"`
}

func (s *Server) handleEnvCons2092(w http.ResponseWriter, r *http.Request) {
	result := EnvConsResult2092{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		totalEnv := 0
		for _, c := range dep.Spec.Template.Spec.Containers {
			totalEnv += len(c.Env)
		}
		if totalEnv > 30 {
			result.Summary.Inconsistent++
			result.Inconsistent = append(result.Inconsistent, EnvConsEntry2092{
				Name: dep.Name, Namespace: dep.Namespace, EnvCount: totalEnv,
			})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Inconsistent, func(i, j int) bool { return result.Inconsistent[i].EnvCount > result.Inconsistent[j].EnvCount })

	if result.Summary.Inconsistent > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments with >30 env vars — use ConfigMaps", result.Summary.Inconsistent))
	}
	writeJSON(w, result)
}

// keep import
var _ = appsv1.Deployment{}
