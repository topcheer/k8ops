package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.37 — Deployment Dimension (Round 59)
// 1. Deployment Spec Replicas vs Status Ratio
// 2. StatefulSet Replicas vs Updated Gap
// 3. DaemonSet Misscheduled Count Tracker
// ============================================================

type SpecStatusRatioResult2237 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int   `json:"totalDeployments"`
		TotalSpec    int32 `json:"totalSpecReplicas"`
		TotalStatus  int32 `json:"totalStatusReplicas"`
		RatioPct     int   `json:"ratioPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSpecStatusRatio2237(w http.ResponseWriter, r *http.Request) {
	result := SpecStatusRatioResult2237{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Replicas != nil {
			result.Summary.TotalSpec += *dep.Spec.Replicas
		}
		result.Summary.TotalStatus += dep.Status.Replicas
	}
	if result.Summary.TotalSpec > 0 {
		result.Summary.RatioPct = int(result.Summary.TotalStatus) * 100 / int(result.Summary.TotalSpec)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. STS Replicas vs Updated Gap
type STSReplicasUpdatedResult2237 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int   `json:"totalStatefulSets"`
		TotalReplicas int32 `json:"totalReplicas"`
		TotalUpdated  int32 `json:"totalUpdatedReplicas"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSTSReplicasUpdated2237(w http.ResponseWriter, r *http.Request) {
	result := STSReplicasUpdatedResult2237{ScannedAt: time.Now()}
	score := 100
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReplicas += *sts.Spec.Replicas
		}
		result.Summary.TotalUpdated += sts.Status.UpdatedReplicas
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. DS Misscheduled Count
type DSMisscheduledResult2237 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int   `json:"totalDaemonSets"`
		Misscheduled int32 `json:"misscheduledNumber"`
		Desired      int32 `json:"desiredNumber"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDSMisscheduled2237(w http.ResponseWriter, r *http.Request) {
	result := DSMisscheduledResult2237{ScannedAt: time.Now()}
	score := 100
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Misscheduled += ds.Status.NumberMisscheduled
		result.Summary.Desired += ds.Status.DesiredNumberScheduled
	}
	if result.Summary.Misscheduled > 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
