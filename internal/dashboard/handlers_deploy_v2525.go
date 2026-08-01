package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.25 Deployment: RS AvailableReplicas, STS UpdatedReplicas, DS NumberMisscheduled Detail
type RSAvailableRepResult2525 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS    int `json:"totalRS"`
		TotalAvail int `json:"totalAvailableReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSAvailableRep2525(w http.ResponseWriter, r *http.Request) {
	result := RSAvailableRepResult2525{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalAvail += int(rs.Status.AvailableReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSUpdatedRepResult2525 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS     int `json:"totalSTS"`
		TotalUpdated int `json:"totalUpdatedReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSUpdatedRep2525(w http.ResponseWriter, r *http.Request) {
	result := STSUpdatedRepResult2525{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalUpdated += int(sts.Status.UpdatedReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSMisscheduledDetailResult2525 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		Misscheduled int `json:"totalNumberMisscheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSMisscheduledDetail2525(w http.ResponseWriter, r *http.Request) {
	result := DSMisscheduledDetailResult2525{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Misscheduled += int(ds.Status.NumberMisscheduled)
	}
	score := 100
	if result.Summary.Misscheduled > 0 {
		score = 100 - result.Summary.Misscheduled*20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
