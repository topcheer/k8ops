package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.91 Deployment: RS Label Count, STS Status CollisionCount Detail, DS NodeSelector Count
type RSLabelCountResult2591 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int `json:"totalRS"`
		TotalLabels int `json:"totalLabels"`
	}
}

func (s *Server) handleRSLabelCount2591(w http.ResponseWriter, r *http.Request) {
	result := RSLabelCountResult2591{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalLabels += len(rs.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSCollisionDetailResult2591 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS  int `json:"totalSTS"`
		TotalColl int `json:"totalCollisionCount"`
	}
}

func (s *Server) handleSTSCollisionDetail2591(w http.ResponseWriter, r *http.Request) {
	result := STSCollisionDetailResult2591{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Status.CollisionCount != nil {
			result.Summary.TotalColl += int(*sts.Status.CollisionCount)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNodeSelectorCountResult2591 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		WithSelector int `json:"withNodeSelector"`
	}
}

func (s *Server) handleDSNodeSelectorCount2591(w http.ResponseWriter, r *http.Request) {
	result := DSNodeSelectorCountResult2591{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			result.Summary.WithSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
