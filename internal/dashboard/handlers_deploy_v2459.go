package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.59 Deployment: RS OwnerRef Kind, STS VolumeClaimTemplates Count, DS Template Generation
type RSOwnerRefResult2459 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS int            `json:"totalRS"`
		ByOwner map[string]int `json:"byOwnerKind"`
	} `json:"summary"`
}

func (s *Server) handleRSOwnerRef2459(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerRefResult2459{ScannedAt: time.Now()}
	result.Summary.ByOwner = make(map[string]int)
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		for _, ref := range rs.OwnerReferences {
			result.Summary.ByOwner[ref.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSVolClaimResult2459 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		TotalVCT int `json:"totalVolumeClaimTemplates"`
	} `json:"summary"`
}

func (s *Server) handleSTSVolClaim2459(w http.ResponseWriter, r *http.Request) {
	result := STSVolClaimResult2459{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalVCT += len(sts.Spec.VolumeClaimTemplates)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSTemplateGenResult2459 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS  int `json:"totalDS"`
		TotalGen int `json:"totalTemplateGeneration"`
	} `json:"summary"`
}

func (s *Server) handleDSTemplateGen2459(w http.ResponseWriter, r *http.Request) {
	result := DSTemplateGenResult2459{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalGen += int(ds.Generation)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
