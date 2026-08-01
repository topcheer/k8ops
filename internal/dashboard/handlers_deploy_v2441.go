package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.41 Deployment: STS Revision History, Deployment MaxSurge, DS MaxUnavailable
type STSRevHistoryResult2441 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		TotalRev int `json:"totalRevisionHistoryLimit"`
	} `json:"summary"`
}

func (s *Server) handleSTSRevHistory2441(w http.ResponseWriter, r *http.Request) {
	result := STSRevHistoryResult2441{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.RevisionHistoryLimit != nil {
			result.Summary.TotalRev += int(*sts.Spec.RevisionHistoryLimit)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DepMaxSurgeResult2441 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDep  int `json:"totalDeployments"`
		WithSurge int `json:"withMaxSurge"`
	} `json:"summary"`
}

func (s *Server) handleDepMaxSurge2441(w http.ResponseWriter, r *http.Request) {
	result := DepMaxSurgeResult2441{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDep++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			result.Summary.WithSurge++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSMaxUnavailResult2441 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS        int `json:"totalDS"`
		WithMaxUnavail int `json:"withMaxUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleDSMaxUnavail2441(w http.ResponseWriter, r *http.Request) {
	result := DSMaxUnavailResult2441{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.UpdateStrategy.RollingUpdate != nil && ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
			result.Summary.WithMaxUnavail++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
