package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.77 Deployment: RS Generation Distribution, STS PersistentVolumeClaim Retention, DS HasAffinity
type RSGenerationResult2477 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		TotalGen int `json:"totalGenerations"`
	} `json:"summary"`
}

func (s *Server) handleRSGeneration2477(w http.ResponseWriter, r *http.Request) {
	result := RSGenerationResult2477{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalGen += int(rs.Generation)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPVCRetentionResult2477 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		WithPolicy int `json:"withPVCRetentionPolicy"`
	} `json:"summary"`
}

func (s *Server) handleSTSPVCRetention2477(w http.ResponseWriter, r *http.Request) {
	result := STSPVCRetentionResult2477{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.PersistentVolumeClaimRetentionPolicy != nil {
			result.Summary.WithPolicy++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSAffinityResult2477 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		WithAffinity int `json:"withAffinity"`
	} `json:"summary"`
}

func (s *Server) handleDSAffinity2477(w http.ResponseWriter, r *http.Request) {
	result := DSAffinityResult2477{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.Template.Spec.Affinity != nil {
			result.Summary.WithAffinity++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
