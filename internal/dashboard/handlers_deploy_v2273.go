package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.73 Deployment: CronJob Schedule Catalog, Deployment Revision History, STS Ordinal Status
type CronJobCatalogResult2273 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		Suspended     int `json:"suspended"`
		Active        int `json:"active"`
	} `json:"summary"`
	Items []struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Schedule  string `json:"schedule"`
	} `json:"items"`
}

func (s *Server) handleCronJobCatalog2273(w http.ResponseWriter, r *http.Request) {
	result := CronJobCatalogResult2273{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			result.Summary.Suspended++
		}
		if len(cj.Status.Active) > 0 {
			result.Summary.Active++
		}
		result.Items = append(result.Items, struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Schedule  string `json:"schedule"`
		}{cj.Name, cj.Namespace, cj.Spec.Schedule})
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RevisionHistoryResult2273 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeployments int `json:"totalDeployments"`
		TotalHistory     int `json:"totalHistoryLimit"`
		AvgHistory       int `json:"avgHistoryLimit"`
	} `json:"summary"`
}

func (s *Server) handleRevisionHistory2273(w http.ResponseWriter, r *http.Request) {
	result := RevisionHistoryResult2273{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++
		limit := 10
		if dep.Spec.RevisionHistoryLimit != nil {
			limit = int(*dep.Spec.RevisionHistoryLimit)
		}
		result.Summary.TotalHistory += limit
	}
	if result.Summary.TotalDeployments > 0 {
		result.Summary.AvgHistory = result.Summary.TotalHistory / result.Summary.TotalDeployments
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSOrdinalResult2273 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS         int `json:"totalSTS"`
		WithOrderedReady int `json:"withOrderedReady"`
		WithParallel     int `json:"withParallelPoll"`
	} `json:"summary"`
}

func (s *Server) handleSTSOrdinal2273(w http.ResponseWriter, r *http.Request) {
	result := STSOrdinalResult2273{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.PodManagementPolicy == "OrderedReady" {
			result.Summary.WithOrderedReady++
		} else if sts.Spec.PodManagementPolicy == "Parallel" {
			result.Summary.WithParallel++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
