package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.97 Deployment: RS OwnerReferences Count, STS PodMinReadySeconds, DS UpdateStrategy RollingUpdate
type RSOwnerRefCountResult2597 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS        int `json:"totalRS"`
		TotalOwnerRefs int `json:"totalOwnerRefs"`
	}
}

func (s *Server) handleRSOwnerRefCount2597(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerRefCountResult2597{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		result.Summary.TotalOwnerRefs += len(rs.OwnerReferences)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSMinReadyResult2597 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		CustomReady int `json:"customMinReady"`
	}
}

func (s *Server) handleSTSMinReady2597(w http.ResponseWriter, r *http.Request) {
	result := STSMinReadyResult2597{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.MinReadySeconds > 0 {
			result.Summary.CustomReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSRollingUpdateResult2597 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		WithRolling int `json:"withRollingUpdate"`
	}
}

func (s *Server) handleDSRollingUpdate2597(w http.ResponseWriter, r *http.Request) {
	result := DSRollingUpdateResult2597{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.UpdateStrategy.RollingUpdate != nil {
			result.Summary.WithRolling++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
