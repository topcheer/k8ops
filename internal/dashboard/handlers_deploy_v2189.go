package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.89 — Deployment Dimension (Round 51)
// 1. Deployment Paused Status Tracker
// 2. StatefulSet Ready vs Available Gap
// 3. ReplicaSet Owner Kind Distribution
// ============================================================

type PausedStatusResult2189 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Paused       int `json:"pausedDeployments"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePausedStatus2189(w http.ResponseWriter, r *http.Request) {
	result := PausedStatusResult2189{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Paused {
			result.Summary.Paused++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Ready vs Available
type STSReadyAvailResult2189 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int   `json:"totalStatefulSets"`
		TotalReady int32 `json:"totalReadyReplicas"`
		TotalAvail int32 `json:"totalAvailableReplicas"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSReadyAvail2189(w http.ResponseWriter, r *http.Request) {
	result := STSReadyAvailResult2189{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalReady += sts.Status.ReadyReplicas
		result.Summary.TotalAvail += sts.Status.AvailableReplicas
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. RS Owner Kind Distribution
type RSOwnerKindResult2189 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int            `json:"totalReplicaSets"`
		ByOwnerKind map[string]int `json:"byOwnerKind"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSOwnerKind2189(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerKindResult2189{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByOwnerKind = make(map[string]int)
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.OwnerReferences) > 0 {
			result.Summary.ByOwnerKind[rs.OwnerReferences[0].Kind]++
		} else {
			result.Summary.ByOwnerKind["standalone"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
