package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.63 Deployment: Deployment Strategy Type, STS Update Strategy, DS Revision Count
type DepStrategyResult2363 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int            `json:"totalDeployments"`
		ByStrategy   map[string]int `json:"byStrategy"`
	} `json:"summary"`
}

func (s *Server) handleDepStrategy2363(w http.ResponseWriter, r *http.Request) {
	result := DepStrategyResult2363{ScannedAt: time.Now()}
	result.Summary.ByStrategy = make(map[string]int)
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.ByStrategy[string(dep.Spec.Strategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSUpdateStratResult2363 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS   int            `json:"totalSTS"`
		ByStrategy map[string]int `json:"byUpdateStrategy"`
	} `json:"summary"`
}

func (s *Server) handleSTSUpdateStrat2363(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateStratResult2363{ScannedAt: time.Now()}
	result.Summary.ByStrategy = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByStrategy[string(sts.Spec.UpdateStrategy.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSRevisionResult2363 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS        int   `json:"totalDS"`
		TotalRevisions int32 `json:"totalRevision"`
	} `json:"summary"`
}

func (s *Server) handleDSRevision2363(w http.ResponseWriter, r *http.Request) {
	result := DSRevisionResult2363{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		result.Summary.TotalRevisions += int32(ds.Status.ObservedGeneration)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
