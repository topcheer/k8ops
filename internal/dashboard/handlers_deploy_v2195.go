package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.95 — Deployment Dimension (Round 52)
// 1. Deployment Strategy Distribution
// 2. StatefulSet Service Name Binding
// 3. DaemonSet Tolerations Coverage
// ============================================================

type StrategyDistResult2195 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int            `json:"totalDeployments"`
		ByStrategy   map[string]int `json:"byStrategy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleStrategyDist2195(w http.ResponseWriter, r *http.Request) {
	result := StrategyDistResult2195{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByStrategy = make(map[string]int)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		result.Summary.ByStrategy[string(dep.Spec.Strategy.Type)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Service Name Binding
type STSSvcBindingResult2195 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalStatefulSets"`
		WithSvc  int `json:"withServiceName"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSSvcBinding2195(w http.ResponseWriter, r *http.Request) {
	result := STSSvcBindingResult2195{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName != "" {
			result.Summary.WithSvc++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Tolerations Coverage
type DSTolCovResult2195 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDaemonSets"`
		WithTol     int `json:"withTolerations"`
		MaxTolPerDS int `json:"maxTolerationsPerDS"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSTolCov2195(w http.ResponseWriter, r *http.Request) {
	result := DSTolCovResult2195{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	maxTol := 0
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		tolCnt := len(ds.Spec.Template.Spec.Tolerations)
		if tolCnt > 0 {
			result.Summary.WithTol++
		}
		if tolCnt > maxTol {
			maxTol = tolCnt
		}
	}
	result.Summary.MaxTolPerDS = maxTol
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
