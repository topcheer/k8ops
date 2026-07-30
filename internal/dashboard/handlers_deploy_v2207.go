package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.07 — Deployment Dimension (Round 54)
// 1. Deployment Observed Generation Lag
// 2. StatefulSet Status Current Revision Tracker
// 3. DaemonSet Number Available Gap
// ============================================================

type GenLagResult2207 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithLag      int `json:"withGenerationLag"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleGenLag2207(w http.ResponseWriter, r *http.Request) {
	result := GenLagResult2207{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Generation != dep.Status.ObservedGeneration {
			result.Summary.WithLag++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Current Revision Tracker
type STSRevTrackerResult2207 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS           int `json:"totalStatefulSets"`
		WithUpdate         int `json:"withCurrentRevision"`
		WithCollisionCount int `json:"withCollisionCount"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSRevTracker2207(w http.ResponseWriter, r *http.Request) {
	result := STSRevTrackerResult2207{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CurrentRevision != "" {
			result.Summary.WithUpdate++
		}
		if sts.Status.CollisionCount != nil && *sts.Status.CollisionCount > 0 {
			result.Summary.WithCollisionCount++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Number Available Gap
type DSAvailGapResult2207 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int   `json:"totalDaemonSets"`
		TotalDesired int32 `json:"totalDesiredScheduled"`
		TotalAvail   int32 `json:"totalNumberAvailable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSAvailGap2207(w http.ResponseWriter, r *http.Request) {
	result := DSAvailGapResult2207{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalDesired += ds.Status.DesiredNumberScheduled
		result.Summary.TotalAvail += ds.Status.NumberAvailable
	}
	if result.Summary.TotalDesired > 0 && result.Summary.TotalAvail < result.Summary.TotalDesired {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
