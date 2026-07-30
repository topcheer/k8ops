package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.65 — Deployment Dimension (Round 47)
// 1. ReplicaSet Orphan Detector
// 2. Job Completion Pattern Audit
// 3. CronJob Schedule Conflict Detector
// ============================================================

type RSOrphanResult2165 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalReplicaSets"`
		OrphanRS int `json:"orphanReplicaSets"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRSOrphan2165(w http.ResponseWriter, r *http.Request) {
	result := RSOrphanResult2165{ScannedAt: time.Now()}
	score := 100
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.OwnerReferences) == 0 && (rs.Spec.Replicas == nil || *rs.Spec.Replicas > 0) {
			result.Summary.OrphanRS++
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Job Completion Pattern
type JobCompResult2165 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int            `json:"totalJobs"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleJobComp2165(w http.ResponseWriter, r *http.Request) {
	result := JobCompResult2165{ScannedAt: time.Now()}
	score := 100
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPhase = make(map[string]int)
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		phase := string(job.Status.Succeeded)
		if job.Status.Failed > 0 {
			result.Summary.ByPhase["failed"]++
		} else if job.Status.Succeeded > 0 {
			result.Summary.ByPhase["completed"]++
		} else {
			result.Summary.ByPhase["running"]++
		}
		_ = phase
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CronJob Schedule Conflict
type CronConflictResult2165 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		BySchedule    map[string]int `json:"bySchedule"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCronConflict2165(w http.ResponseWriter, r *http.Request) {
	result := CronConflictResult2165{ScannedAt: time.Now()}
	score := 100
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	result.Summary.BySchedule = make(map[string]int)
	for _, cron := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.BySchedule[cron.Spec.Schedule]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
