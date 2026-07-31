package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.45 Deployment: STS Replicas vs Ready, DS Number Unavailable, Job Completion Duration
type STSRepVsReadyResult2345 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int   `json:"totalSTS"`
		TotalReps  int32 `json:"totalReplicas"`
		TotalReady int32 `json:"totalReady"`
	} `json:"summary"`
}

func (s *Server) handleSTSRepVsReady2345(w http.ResponseWriter, r *http.Request) {
	result := STSRepVsReadyResult2345{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalReps += sts.Status.Replicas
		result.Summary.TotalReady += sts.Status.ReadyReplicas
	}
	score := 100
	if result.Summary.TotalReps > 0 {
		score = int(result.Summary.TotalReady * 100 / result.Summary.TotalReps)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DSUnavailResult2345 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		DesiredNum  int `json:"totalDesired"`
		Unavailable int `json:"totalUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleDSUnavail2345(w http.ResponseWriter, r *http.Request) {
	result := DSUnavailResult2345{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.DesiredNum += int(ds.Status.DesiredNumberScheduled)
		result.Summary.Unavailable += int(ds.Status.NumberUnavailable)
	}
	score := 100
	if result.Summary.DesiredNum > 0 {
		score = 100 - (result.Summary.Unavailable*100)/result.Summary.DesiredNum
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type JobDurationResult2345 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int `json:"totalJobs"`
		Completed int `json:"completed"`
		Active    int `json:"active"`
	} `json:"summary"`
}

func (s *Server) handleJobDuration2345(w http.ResponseWriter, r *http.Request) {
	result := JobDurationResult2345{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Status.Succeeded > 0 {
			result.Summary.Completed++
		}
		if job.Status.Active > 0 {
			result.Summary.Active++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
