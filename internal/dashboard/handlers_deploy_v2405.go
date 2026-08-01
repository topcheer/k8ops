package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.05 Deployment: Deployment Conditions, STS Status Available, DS Conditions Ready
type DepConditionsResult2405 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys   int `json:"totalDeployments"`
		AvailableConds int `json:"availableConditions"`
	} `json:"summary"`
}

func (s *Server) handleDepConditions2405(w http.ResponseWriter, r *http.Request) {
	result := DepConditionsResult2405{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		for _, cond := range dep.Status.Conditions {
			if string(cond.Type) == "Available" && cond.Status == "True" {
				result.Summary.AvailableConds++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSAvailableResult2405 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int   `json:"totalSTS"`
		TotalAvail int32 `json:"totalAvailableReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSAvailable2405(w http.ResponseWriter, r *http.Request) {
	result := STSAvailableResult2405{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalAvail += sts.Status.AvailableReplicas
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSCondReadyResult2405 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		TotalReady int `json:"totalNumberReady"`
	} `json:"summary"`
}

func (s *Server) handleDSCondReady2405(w http.ResponseWriter, r *http.Request) {
	result := DSCondReadyResult2405{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalReady += int(ds.Status.NumberReady)
	}
	score := 100
	if result.Summary.TotalDS > 0 && result.Summary.TotalReady == 0 {
		score = 50
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
