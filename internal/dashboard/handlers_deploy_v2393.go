package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.93 Deployment: Deployment Paused Status, STS Ordinal, Job BackoffLimit
type DepPausedResult2393 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Paused       int `json:"paused"`
	} `json:"summary"`
}

func (s *Server) handleDepPaused2393(w http.ResponseWriter, r *http.Request) {
	result := DepPausedResult2393{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Paused {
			result.Summary.Paused++
		}
	}
	score := 100
	if result.Summary.TotalDeploys > 0 && result.Summary.Paused > 0 {
		score = 100 - (result.Summary.Paused*20)/result.Summary.TotalDeploys
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type STSOrdinalResult2393 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS  int   `json:"totalSTS"`
		TotalReps int32 `json:"totalReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSOrdinal2393(w http.ResponseWriter, r *http.Request) {
	result := STSOrdinalResult2393{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReps += *sts.Spec.Replicas
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobBackoffResult2393 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs   int `json:"totalJobs"`
		WithBackoff int `json:"withBackoffLimit"`
	} `json:"summary"`
}

func (s *Server) handleJobBackoff2393(w http.ResponseWriter, r *http.Request) {
	result := JobBackoffResult2393{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.BackoffLimit != nil {
			result.Summary.WithBackoff++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
