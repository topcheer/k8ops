package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.19 Deployment: RS Conditions, STS CurrentReplicas, DS Conditions
type RSConditionsResult2519 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int            `json:"totalRS"`
		TotalConds int            `json:"totalConditions"`
		ByType     map[string]int `json:"byConditionType"`
	} `json:"summary"`
}

func (s *Server) handleRSConditions2519(w http.ResponseWriter, r *http.Request) {
	result := RSConditionsResult2519{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		for _, cond := range rs.Status.Conditions {
			result.Summary.TotalConds++
			result.Summary.ByType[string(cond.Type)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSCurrentRepResult2519 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS     int `json:"totalSTS"`
		TotalCurrent int `json:"totalCurrentReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSCurrentRep2519(w http.ResponseWriter, r *http.Request) {
	result := STSCurrentRepResult2519{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalCurrent += int(sts.Status.CurrentReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSConditionsResult2519 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int            `json:"totalDS"`
		TotalConds int            `json:"totalConditions"`
		ByType     map[string]int `json:"byConditionType"`
	} `json:"summary"`
}

func (s *Server) handleDSConditions2519(w http.ResponseWriter, r *http.Request) {
	result := DSConditionsResult2519{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		for _, cond := range ds.Status.Conditions {
			result.Summary.TotalConds++
			result.Summary.ByType[string(cond.Type)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
