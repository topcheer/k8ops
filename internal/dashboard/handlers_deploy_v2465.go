package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.65 Deployment: Deployment Paused Status, STS OrdinalCount, DS DeletionTimestamp
type DepPausedResult2465 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDep int `json:"totalDeployments"`
		Paused   int `json:"pausedCount"`
	} `json:"summary"`
}

func (s *Server) handleDepPaused2465(w http.ResponseWriter, r *http.Request) {
	result := DepPausedResult2465{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDep++
		if dep.Spec.Paused {
			result.Summary.Paused++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSOrdinalResult2465 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS  int `json:"totalSTS"`
		TotalReps int `json:"totalDesiredReplicas"`
	} `json:"summary"`
}

func (s *Server) handleSTSOrdinal2465(w http.ResponseWriter, r *http.Request) {
	result := STSOrdinalResult2465{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.Replicas != nil {
			result.Summary.TotalReps += int(*sts.Spec.Replicas)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSDeletionResult2465 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS  int `json:"totalDS"`
		Deleting int `json:"deletingCount"`
	} `json:"summary"`
}

func (s *Server) handleDSDeletion2465(w http.ResponseWriter, r *http.Request) {
	result := DSDeletionResult2465{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.DeletionTimestamp != nil {
			result.Summary.Deleting++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
