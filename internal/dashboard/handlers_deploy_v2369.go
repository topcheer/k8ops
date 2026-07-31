package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.69 Deployment: Deployment MinReadySeconds, STS MinReadySeconds, DS MinReadySeconds
type DepMinReadyResult2369 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithCustom   int `json:"withCustomMinReady"`
	} `json:"summary"`
}

func (s *Server) handleDepMinReady2369(w http.ResponseWriter, r *http.Request) {
	result := DepMinReadyResult2369{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.MinReadySeconds > 0 {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSMinReadyResult2369 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int `json:"totalSTS"`
		WithCustom int `json:"withCustomMinReady"`
	} `json:"summary"`
}

func (s *Server) handleSTSMinReady2369(w http.ResponseWriter, r *http.Request) {
	result := STSMinReadyResult2369{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.MinReadySeconds > 0 {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSMinReadyResult2369 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS    int `json:"totalDS"`
		WithCustom int `json:"withCustomMinReady"`
	} `json:"summary"`
}

func (s *Server) handleDSMinReady2369(w http.ResponseWriter, r *http.Request) {
	result := DSMinReadyResult2369{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.MinReadySeconds > 0 {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
