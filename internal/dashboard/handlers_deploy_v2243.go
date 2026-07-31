package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.43 — Deployment Dimension (Round 60)
// 1. Deployment Status Replicas vs Ready Replicas
// 2. StatefulSet Status Update Revision Catalog
// 3. DaemonSet NumberUnavailable Tracker
// ============================================================

type ReplicasVsReadyResult2243 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int   `json:"totalDeployments"`
		TotalReplicas int32 `json:"totalReplicas"`
		TotalReady    int32 `json:"totalReadyReplicas"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleReplicasVsReady2243(w http.ResponseWriter, r *http.Request) {
	result := ReplicasVsReadyResult2243{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		result.Summary.TotalReplicas += dep.Status.Replicas
		result.Summary.TotalReady += dep.Status.ReadyReplicas
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Update Revision Catalog
type STSUpdateRevResult2243 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int `json:"totalStatefulSets"`
		WithUpdateRev int `json:"withUpdateRevision"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSUpdateRev2243(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateRevResult2243{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.UpdateRevision != "" {
			result.Summary.WithUpdateRev++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS NumberUnavailable
type DSUnavailResult2243 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int   `json:"totalDaemonSets"`
		Unavailable int32 `json:"numberUnavailable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSUnavail2243(w http.ResponseWriter, r *http.Request) {
	result := DSUnavailResult2243{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Unavailable += ds.Status.NumberUnavailable
	}
	if result.Summary.Unavailable > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
