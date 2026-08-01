package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.29 Deployment: Deployment Status Replicas, RS Status Available, CronJob Status Active Count
type DepStatusRepsResult2429 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int   `json:"totalDeployments"`
		TotalReps    int32 `json:"totalStatusReplicas"`
	} `json:"summary"`
}

func (s *Server) handleDepStatusReps2429(w http.ResponseWriter, r *http.Request) {
	result := DepStatusRepsResult2429{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.TotalReps += dep.Status.Replicas
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RSAvailableResult2429 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int   `json:"totalRS"`
		TotalAvail int32 `json:"totalAvailableReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSAvailable2429(w http.ResponseWriter, r *http.Request) {
	result := RSAvailableResult2429{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalAvail += rs.Status.AvailableReplicas
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobActiveCountResult2429 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		TotalActive   int `json:"totalActiveJobs"`
	} `json:"summary"`
}

func (s *Server) handleCronJobActiveCount2429(w http.ResponseWriter, r *http.Request) {
	result := CronJobActiveCountResult2429{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.TotalActive += len(cj.Status.Active)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
