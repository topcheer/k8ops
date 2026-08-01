package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.95 Deployment: RS FullyLabeledReplicas, STS AvailableReplicas, DS NumberReady
type RSFullyLabeledResult2495 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		TotalFLR int `json:"totalFullyLabeledReplicas"`
	} `json:"summary"`
}

func (s *Server) handleRSFullyLabeled2495(w http.ResponseWriter, r *http.Request) {
	result := RSFullyLabeledResult2495{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalFLR += int(rs.Status.FullyLabeledReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSAvailableRepResult2495 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		TotalAvail int `json:"totalAvailableReplicas"`
		TotalReady int `json:"totalReadyReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSAvailableRep2495(w http.ResponseWriter, r *http.Request) {
	result := STSAvailableRepResult2495{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalAvail += int(sts.Status.AvailableReplicas)
		result.Summary.TotalReady += int(sts.Status.ReadyReplicas)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNumberReadyResult2495 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		TotalReady int `json:"totalNumberReady"`
		TotalAvail int `json:"totalNumberAvailable"`
	} `json:"summary"`
}

func (s *Server) handleDSNumberReady2495(w http.ResponseWriter, r *http.Request) {
	result := DSNumberReadyResult2495{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalReady += int(ds.Status.NumberReady)
		result.Summary.TotalAvail += int(ds.Status.NumberAvailable)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
