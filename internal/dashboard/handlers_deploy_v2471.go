package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.71 Deployment: Deployment ProgressDeadline, STS ParallelPodManagement, DS HasTolerations
type DepProgressDeadlineResult2471 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDep     int `json:"totalDeployments"`
		WithDeadline int `json:"withProgressDeadlineSeconds"`
	} `json:"summary"`
}

func (s *Server) handleDepProgressDeadline2471(w http.ResponseWriter, r *http.Request) {
	result := DepProgressDeadlineResult2471{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDep++
		if dep.Spec.ProgressDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSParallelResult2471 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		Parallel int `json:"parallelPodManagement"`
	} `json:"summary"`
}

func (s *Server) handleSTSParallel2471(w http.ResponseWriter, r *http.Request) {
	result := STSParallelResult2471{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.PodManagementPolicy == "Parallel" {
			result.Summary.Parallel++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSTolerationsResult2471 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS   int `json:"totalDS"`
		WithToler int `json:"withTolerations"`
	} `json:"summary"`
}

func (s *Server) handleDSTolerations2471(w http.ResponseWriter, r *http.Request) {
	result := DSTolerationsResult2471{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if len(ds.Spec.Template.Spec.Tolerations) > 0 {
			result.Summary.WithToler++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
