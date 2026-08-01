package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.09 Deployment: RS Finalizers Count, STS VolumeClaimTemplates Count, DS RevisionHistoryLimit
type RSFinalizers2609Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS   int `json:"totalRS"`
		WithFinal int `json:"withFinalizers"`
	} `json:"summary"`
}

func (s *Server) handleRSFinalizers2609(w http.ResponseWriter, r *http.Request) {
	result := RSFinalizers2609Result{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.Finalizers) > 0 {
			result.Summary.WithFinal++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSVolClaim2609Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithVCT  int `json:"withVolumeClaimTemplates"`
	} `json:"summary"`
}

func (s *Server) handleSTSVolClaim2609(w http.ResponseWriter, r *http.Request) {
	result := STSVolClaim2609Result{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			result.Summary.WithVCT++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSRevHistory2609Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS        int `json:"totalDS"`
		WithRevHistory int `json:"withRevisionHistoryLimit"`
	} `json:"summary"`
}

func (s *Server) handleDSRevHistory2609(w http.ResponseWriter, r *http.Request) {
	result := DSRevHistory2609Result{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.RevisionHistoryLimit != nil {
			result.Summary.WithRevHistory++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
