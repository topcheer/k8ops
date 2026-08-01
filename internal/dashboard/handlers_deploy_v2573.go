package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.73 Deployment: RS MinReadySeconds, STS PodManagementPolicy Detail, DS HasHostNetwork
type RSMinReadyResult2573 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS     int `json:"totalRS"`
		CustomReady int `json:"customMinReady"`
	}
}

func (s *Server) handleRSMinReady2573(w http.ResponseWriter, r *http.Request) {
	result := RSMinReadyResult2573{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if rs.Spec.MinReadySeconds > 0 {
			result.Summary.CustomReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPodMgmtResult2573 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByPolicy map[string]int `json:"byPodManagementPolicy"`
	}
}

func (s *Server) handleSTSPodMgmt2573(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtResult2573{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByPolicy[string(sts.Spec.PodManagementPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSHostNetResult2573 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS     int `json:"totalDS"`
		HostNetwork int `json:"hostNetwork"`
	}
}

func (s *Server) handleDSHostNet2573(w http.ResponseWriter, r *http.Request) {
	result := DSHostNetResult2573{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.Template.Spec.HostNetwork {
			result.Summary.HostNetwork++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
