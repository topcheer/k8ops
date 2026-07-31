package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.91 Deployment: Deployment Progress Audit, RS Owner Reference Census, Job Active Deadline
type DepProgressResult2291 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int `json:"totalDeployments"`
		FullyProgress int `json:"fullyProgressed"`
		Stalled       int `json:"stalled"`
	} `json:"summary"`
}

func (s *Server) handleDepProgress2291(w http.ResponseWriter, r *http.Request) {
	result := DepProgressResult2291{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Status.UpdatedReplicas == dep.Status.Replicas && dep.Status.Replicas == dep.Status.AvailableReplicas {
			result.Summary.FullyProgress++
		} else if dep.Status.Replicas > 0 && dep.Status.UpdatedReplicas < dep.Status.Replicas {
			result.Summary.Stalled++
		}
	}
	score := 100
	if result.Summary.TotalDeploys > 0 {
		score = result.Summary.FullyProgress * 100 / result.Summary.TotalDeploys
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RSOwnerResult2291 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS   int `json:"totalRS"`
		WithOwner int `json:"withOwnerRef"`
		Orphaned  int `json:"orphaned"`
	} `json:"summary"`
}

func (s *Server) handleRSOwner2291(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerResult2291{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		if rs.Spec.Replicas != nil && *rs.Spec.Replicas == 0 && len(rs.ObjectMeta.OwnerReferences) == 0 {
			continue // skip old zero-replica orphans
		}
		result.Summary.TotalRS++
		if len(rs.ObjectMeta.OwnerReferences) > 0 {
			result.Summary.WithOwner++
		} else {
			result.Summary.Orphaned++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobDeadlineResult2291 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs    int `json:"totalJobs"`
		WithDeadline int `json:"withActiveDeadline"`
		WithBackoff  int `json:"withBackoffLimit"`
	} `json:"summary"`
}

func (s *Server) handleJobDeadline2291(w http.ResponseWriter, r *http.Request) {
	result := JobDeadlineResult2291{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Spec.ActiveDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
		if job.Spec.BackoffLimit != nil {
			result.Summary.WithBackoff++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
