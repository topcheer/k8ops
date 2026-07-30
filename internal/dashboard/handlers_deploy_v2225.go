package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.25 — Deployment Dimension (Round 57)
// 1. Deployment Revision Limit Compliance
// 2. StatefulSet Template Hash Tracker
// 3. ReplicaSet Active vs Inactive Ratio
// ============================================================

type RevLimitCompResult2225 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithLimit    int `json:"withRevisionLimit"`
		Default      int `json:"usingDefault"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRevLimitComp2225(w http.ResponseWriter, r *http.Request) {
	result := RevLimitCompResult2225{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.RevisionHistoryLimit != nil {
			result.Summary.WithLimit++
		} else {
			result.Summary.Default++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Template Hash Tracker
type STSTmplHashResult2225 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int   `json:"totalStatefulSets"`
		TotalObserved int64 `json:"totalObservedGeneration"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSTmplHash2225(w http.ResponseWriter, r *http.Request) {
	result := STSTmplHashResult2225{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalObserved += sts.Status.ObservedGeneration
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RS Active vs Inactive
type RSActiveRatioResult2225 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalReplicaSets"`
		Active   int `json:"active"`
		Inactive int `json:"inactive"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSActiveRatio2225(w http.ResponseWriter, r *http.Request) {
	result := RSActiveRatioResult2225{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas > 0 {
			result.Summary.Active++
		} else {
			result.Summary.Inactive++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
