package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.35 Deployment: RS Label Template, STS Label Count, DS Label Count
type RSLabelResult2435 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int `json:"totalRS"`
		TotalLabels int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleRSLabel2435(w http.ResponseWriter, r *http.Request) {
	result := RSLabelResult2435{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalLabels += len(rs.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSLabelResult2435 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		TotalLabels int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleSTSLabel2435(w http.ResponseWriter, r *http.Request) {
	result := STSLabelResult2435{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalLabels += len(sts.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSLabelResult2435 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		TotalLabels int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleDSLabel2435(w http.ResponseWriter, r *http.Request) {
	result := DSLabelResult2435{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalLabels += len(ds.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
