package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.03 Deployment: STS Status Replicas Audit, DS Scheduled vs Misscheduled, Job Parallelism Config
type STSStatusResult2303 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS       int   `json:"totalSTS"`
		TotalReplicas  int32 `json:"totalReplicas"`
		TotalReady     int32 `json:"totalReady"`
		TotalUpdated   int32 `json:"totalUpdated"`
		TotalAvailable int32 `json:"totalAvailable"`
	} `json:"summary"`
}

func (s *Server) handleSTSStatus2303(w http.ResponseWriter, r *http.Request) {
	result := STSStatusResult2303{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalReplicas += sts.Status.Replicas
		result.Summary.TotalReady += sts.Status.ReadyReplicas
		result.Summary.TotalUpdated += sts.Status.UpdatedReplicas
		result.Summary.TotalAvailable += sts.Status.AvailableReplicas
	}
	score := 100
	if result.Summary.TotalReplicas > 0 {
		score = int(result.Summary.TotalReady * 100 / result.Summary.TotalReplicas)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DSMisScheduleResult2303 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS         int `json:"totalDS"`
		DesiredNum      int `json:"totalDesired"`
		ScheduledNum    int `json:"totalScheduled"`
		MisscheduledNum int `json:"totalMisscheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSMisSchedule2303(w http.ResponseWriter, r *http.Request) {
	result := DSMisScheduleResult2303{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.DesiredNum += int(ds.Status.DesiredNumberScheduled)
		result.Summary.ScheduledNum += int(ds.Status.CurrentNumberScheduled)
		result.Summary.MisscheduledNum += int(ds.Status.NumberMisscheduled)
	}
	score := 100
	if result.Summary.DesiredNum > 0 {
		score = 100 - (result.Summary.MisscheduledNum*100)/result.Summary.DesiredNum
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type JobParallelResult2303 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs       int   `json:"totalJobs"`
		WithParallelism int   `json:"withParallelism"`
		TotalParallel   int32 `json:"totalParallelism"`
	} `json:"summary"`
}

func (s *Server) handleJobParallel2303(w http.ResponseWriter, r *http.Request) {
	result := JobParallelResult2303{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.Parallelism != nil && *job.Spec.Parallelism > 1 {
			result.Summary.WithParallelism++
			result.Summary.TotalParallel += *job.Spec.Parallelism
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
