package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.27 Deployment: RS Deletion Grace, STS Status AvailableReplicas Detail, DS Status DesiredNumberScheduled
type RSDeletionGrace2627Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS   int `json:"totalRS"`
		WithGrace int `json:"withDeletionGracePeriod"`
	} `json:"summary"`
}

func (s *Server) handleRSDeletionGrace2627(w http.ResponseWriter, r *http.Request) {
	result := RSDeletionGrace2627Result{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.DeletionGracePeriodSeconds != nil {
			result.Summary.WithGrace++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSAvailDetail2627Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		TotalAvail int `json:"totalAvailableReplicas"`
		TotalReady int `json:"totalReadyReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSAvailDetail2627(w http.ResponseWriter, r *http.Request) {
	result := STSAvailDetail2627Result{ScannedAt: time.Now()}
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

type DSDesiredScheduled2627Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		TotalDesired int `json:"totalDesiredNumberScheduled"`
	} `json:"summary"`
}

func (s *Server) handleDSDesiredScheduled2627(w http.ResponseWriter, r *http.Request) {
	result := DSDesiredScheduled2627Result{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalDesired += int(ds.Status.DesiredNumberScheduled)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
