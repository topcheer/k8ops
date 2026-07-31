package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.97 Deployment: DS Desired vs Ready, Deployment Condition Rollout, CronJob Last Schedule
type DSDeployReadyResult2297 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		DesiredNum int `json:"totalDesiredNumber"`
		ReadyNum   int `json:"totalReadyNumber"`
	} `json:"summary"`
}

func (s *Server) handleDSDeployReady2297(w http.ResponseWriter, r *http.Request) {
	result := DSDeployReadyResult2297{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.DesiredNum += int(ds.Status.DesiredNumberScheduled)
		result.Summary.ReadyNum += int(ds.Status.NumberReady)
	}
	score := 100
	if result.Summary.DesiredNum > 0 {
		score = result.Summary.ReadyNum * 100 / result.Summary.DesiredNum
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RolloutCondResult2297 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		Progressing  int `json:"progressing"`
		Available    int `json:"available"`
		ReplicaFail  int `json:"replicaFailure"`
	} `json:"summary"`
}

func (s *Server) handleRolloutCond2297(w http.ResponseWriter, r *http.Request) {
	result := RolloutCondResult2297{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		for _, cond := range dep.Status.Conditions {
			if cond.Status != "True" {
				continue
			}
			switch string(cond.Type) {
			case "Progressing":
				result.Summary.Progressing++
			case "Available":
				result.Summary.Available++
			case "ReplicaFailure":
				result.Summary.ReplicaFail++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobLastSchedResult2297 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		WithSchedule  int `json:"withLastSchedule"`
	} `json:"summary"`
}

func (s *Server) handleCronJobLastSched2297(w http.ResponseWriter, r *http.Request) {
	result := CronJobLastSchedResult2297{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Status.LastScheduleTime != nil {
			result.Summary.WithSchedule++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
