package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.21 Deployment: RS Annotations Detail, STS AvailableReplicas Total, DS NumberUnavailable
type RSAnnotDetail2621Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS   int `json:"totalRS"`
		WithAnnot int `json:"withAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleRSAnnotDetail2621(w http.ResponseWriter, r *http.Request) {
	result := RSAnnotDetail2621Result{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.Annotations) > 0 {
			result.Summary.WithAnnot++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSAvailRep2621Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		TotalAvail int `json:"totalAvailableReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSAvailRep2621(w http.ResponseWriter, r *http.Request) {
	result := STSAvailRep2621Result{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalAvail += int(sts.Status.AvailableReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNumUnavail2621Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		Unavailable int `json:"totalNumberUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleDSNumUnavail2621(w http.ResponseWriter, r *http.Request) {
	result := DSNumUnavail2621Result{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Unavailable += int(ds.Status.NumberUnavailable)
	}
	score := 100
	if result.Summary.Unavailable > 0 {
		score = 100 - result.Summary.Unavailable*20
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
