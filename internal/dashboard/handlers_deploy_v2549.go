package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.49 Deployment: RS Spec Replicas Summary, STS Spec UpdateStrategy RollingUpdate, DS Status NumberAvailable
type RSSpecReplicasResult2549 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		TotalRep int `json:"totalSpecReplicas"`
	}
}

func (s *Server) handleRSSpecReplicas2549(w http.ResponseWriter, r *http.Request) {
	result := RSSpecReplicasResult2549{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Spec.Replicas != nil {
			result.Summary.TotalRep += int(*rs.Spec.Replicas)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSRollingUpdateResult2549 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		RollingUpd int `json:"rollingUpdateCount"`
	}
}

func (s *Server) handleSTSRollingUpdate2549(w http.ResponseWriter, r *http.Request) {
	result := STSRollingUpdateResult2549{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.UpdateStrategy.RollingUpdate != nil {
			result.Summary.RollingUpd++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSNumberAvailResult2549 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int `json:"totalDS"`
		Avail   int `json:"totalNumberAvailable"`
	}
}

func (s *Server) handleDSNumberAvail2549(w http.ResponseWriter, r *http.Request) {
	result := DSNumberAvailResult2549{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.Avail += int(ds.Status.NumberAvailable)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
