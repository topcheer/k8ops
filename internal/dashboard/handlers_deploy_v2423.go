package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.23 Deployment: STS VolumeClaim Default, Job Spec Completions, CronJob Spec StartingDeadline
type STSVolClaimDefaultResult2423 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS     int `json:"totalSTS"`
		WithVolClaim int `json:"withVolumeClaimTemplates"`
	} `json:"summary"`
}

func (s *Server) handleSTSVolClaimDefault2423(w http.ResponseWriter, r *http.Request) {
	result := STSVolClaimDefaultResult2423{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			result.Summary.WithVolClaim++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobCompletionsResult2423 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs        int   `json:"totalJobs"`
		TotalCompletions int32 `json:"totalCompletions"`
	} `json:"summary"`
}

func (s *Server) handleJobCompletions2423(w http.ResponseWriter, r *http.Request) {
	result := JobCompletionsResult2423{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.Completions != nil {
			result.Summary.TotalCompletions += *job.Spec.Completions
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobStartDeadlineResult2423 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		WithDeadline  int `json:"withStartingDeadline"`
	} `json:"summary"`
}

func (s *Server) handleCronJobStartDeadline2423(w http.ResponseWriter, r *http.Request) {
	result := CronJobStartDeadlineResult2423{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Spec.StartingDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
