package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.39 Deployment: STS CollisionCount, DS Updated Desired, Job TTL Seconds
type STSCollisionResult2339 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS        int   `json:"totalSTS"`
		TotalCollisions int32 `json:"totalCollisionCount"`
	} `json:"summary"`
}

func (s *Server) handleSTSCollision2339(w http.ResponseWriter, r *http.Request) {
	result := STSCollisionResult2339{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CollisionCount != nil {
			result.Summary.TotalCollisions += *sts.Status.CollisionCount
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSUpdatedDesiredResult2339 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		UpdatedNum int `json:"totalUpdated"`
		DesiredNum int `json:"totalDesired"`
	} `json:"summary"`
}

func (s *Server) handleDSUpdatedDesired2339(w http.ResponseWriter, r *http.Request) {
	result := DSUpdatedDesiredResult2339{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.UpdatedNum += int(ds.Status.UpdatedNumberScheduled)
		result.Summary.DesiredNum += int(ds.Status.DesiredNumberScheduled)
	}
	score := 100
	if result.Summary.DesiredNum > 0 {
		score = result.Summary.UpdatedNum * 100 / result.Summary.DesiredNum
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type JobTTLResult2339 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int `json:"totalJobs"`
		WithTTL   int `json:"withTTLCleanup"`
	} `json:"summary"`
}

func (s *Server) handleJobTTL2339(w http.ResponseWriter, r *http.Request) {
	result := JobTTLResult2339{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.TTLSecondsAfterFinished != nil {
			result.Summary.WithTTL++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
