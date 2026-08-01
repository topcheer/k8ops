package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.55 Deployment: RS OwnerRef Detail, STS Spec Replicas Total, DS Generation Summary
type RSOwnerDetailResult2555 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int            `json:"totalRS"`
		ByOwnerKind map[string]int `json:"byOwnerKind"`
	}
}

func (s *Server) handleRSOwnerDetail2555(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerDetailResult2555{ScannedAt: time.Now()}
	result.Summary.ByOwnerKind = make(map[string]int)
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.OwnerReferences) == 0 {
			result.Summary.ByOwnerKind["<none>"]++
		}
		for _, ref := range rs.OwnerReferences {
			result.Summary.ByOwnerKind[ref.Kind]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSSpecRepTotalResult2555 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS      int `json:"totalSTS"`
		TotalReplicas int `json:"totalSpecReplicas"`
	}
}

func (s *Server) handleSTSSpecRepTotal2555(w http.ResponseWriter, r *http.Request) {
	result := STSSpecRepTotalResult2555{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReplicas += int(*sts.Spec.Replicas)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSGenSummaryResult2555 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS  int `json:"totalDS"`
		TotalGen int `json:"totalGeneration"`
	}
}

func (s *Server) handleDSGenSummary2555(w http.ResponseWriter, r *http.Request) {
	result := DSGenSummaryResult2555{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalGen += int(ds.Generation)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
