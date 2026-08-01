package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.17 Deployment: Deployment Selector MatchLabels, RS Owner Ref Controller, CronJob LastScheduleTime
type DepSelectorResult2417 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int `json:"totalDeployments"`
		WithMatchLbls int `json:"withMatchLabels"`
	} `json:"summary"`
}

func (s *Server) handleDepSelector2417(w http.ResponseWriter, r *http.Request) {
	result := DepSelectorResult2417{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Selector != nil && len(dep.Spec.Selector.MatchLabels) > 0 {
			result.Summary.WithMatchLbls++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RSOwnerRefResult2417 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		WithCtrl int `json:"withControllerOwner"`
	} `json:"summary"`
}

func (s *Server) handleRSOwnerRef2417(w http.ResponseWriter, r *http.Request) {
	result := RSOwnerRefResult2417{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		for _, ref := range rs.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				result.Summary.WithCtrl++
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobLastSchedResult2417 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		WithLastSched int `json:"withLastScheduleTime"`
	} `json:"summary"`
}

func (s *Server) handleCronJobLastSched2417(w http.ResponseWriter, r *http.Request) {
	result := CronJobLastSchedResult2417{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Status.LastScheduleTime != nil {
			result.Summary.WithLastSched++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
