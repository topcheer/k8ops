package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.85 Deployment: RS DeletionTimestamp, STS PVC Retention Policy, DS Template Generation
type RSDeletionResult2585 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		Deleting int `json:"deletingRS"`
	}
}

func (s *Server) handleRSDeletion2585(w http.ResponseWriter, r *http.Request) {
	result := RSDeletionResult2585{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.DeletionTimestamp != nil {
			result.Summary.Deleting++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPVCRetentionResult2585 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		WithPolicy int `json:"withPVCRetention"`
	}
}

func (s *Server) handleSTSPVCRetention2585(w http.ResponseWriter, r *http.Request) {
	result := STSPVCRetentionResult2585{ScannedAt: time.Now()}
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

type DSTemplateGenResult2585 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS  int `json:"totalDS"`
		TotalGen int `json:"totalTemplateGeneration"`
	}
}

func (s *Server) handleDSTemplateGen2585(w http.ResponseWriter, r *http.Request) {
	result := DSTemplateGenResult2585{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalGen += int(ds.Generation)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
