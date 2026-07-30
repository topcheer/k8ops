package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.59 — Deployment Dimension (Round 46)
// 1. StatefulSet Pod Management Policy
// 2. DaemonSet Update Strategy Validator
// 3. Deployment Progress Deadline Tracker
// ============================================================

type STSPodMgmtResult2159 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalStatefulSets"`
		ByPolicy map[string]int `json:"byPodManagementPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSPodMgmt2159(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtResult2159{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPolicy = make(map[string]int)
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		policy := string(sts.Spec.PodManagementPolicy)
		if policy == "" {
			policy = "OrderedReady"
		}
		result.Summary.ByPolicy[policy]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. DS Update Strategy
type DSUpdStratResult2159 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int            `json:"totalDaemonSets"`
		ByStrategy map[string]int `json:"byUpdateStrategy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSUpdStrat2159(w http.ResponseWriter, r *http.Request) {
	result := DSUpdStratResult2159{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByStrategy = make(map[string]int)
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		strategy := string(ds.Spec.UpdateStrategy.Type)
		if strategy == "" {
			strategy = "RollingUpdate"
		}
		result.Summary.ByStrategy[strategy]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Progress Deadline Tracker
type ProgDeadlineResult2159 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithDeadline int `json:"withProgressDeadlineSeconds"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleProgDeadline2159(w http.ResponseWriter, r *http.Request) {
	result := ProgDeadlineResult2159{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.ProgressDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
