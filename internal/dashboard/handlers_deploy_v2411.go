package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.11 Deployment: STS Service Name Audit, Job Parallelism Config, CronJob Concurrency Allow
type STSSvcNameResult2411 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		WithSvcName int `json:"withServiceName"`
	} `json:"summary"`
}

func (s *Server) handleSTSSvcName2411(w http.ResponseWriter, r *http.Request) {
	result := STSSvcNameResult2411{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName != "" {
			result.Summary.WithSvcName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobParallelismResult2411 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs     int   `json:"totalJobs"`
		TotalParallel int32 `json:"totalParallelism"`
	} `json:"summary"`
}

func (s *Server) handleJobParallelism2411(w http.ResponseWriter, r *http.Request) {
	result := JobParallelismResult2411{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.Parallelism != nil {
			result.Summary.TotalParallel += *job.Spec.Parallelism
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobConcurResult2411 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		ByConcur      map[string]int `json:"byConcurrencyPolicy"`
	} `json:"summary"`
}

func (s *Server) handleCronJobConcur2411(w http.ResponseWriter, r *http.Request) {
	result := CronJobConcurResult2411{ScannedAt: time.Now()}
	result.Summary.ByConcur = make(map[string]int)
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.ByConcur[string(cj.Spec.ConcurrencyPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
