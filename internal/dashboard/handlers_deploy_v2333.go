package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.33 Deployment: STS VolumeClaimTemplates Size, Deployment MaxUnavailable Custom, CronJob History Limits
type STSPVCSizeResult2333 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int     `json:"totalSTS"`
		TotalSizeGB float64 `json:"totalPVCSizeGB"`
	} `json:"summary"`
}

func (s *Server) handleSTSPVCSize2333(w http.ResponseWriter, r *http.Request) {
	result := STSPVCSizeResult2333{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		for _, tplt := range sts.Spec.VolumeClaimTemplates {
			result.Summary.TotalSizeGB += tplt.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MaxUnavailResult2333 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys  int `json:"totalDeployments"`
		WithCustomMax int `json:"withCustomMaxUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleMaxUnavail2333(w http.ResponseWriter, r *http.Request) {
	result := MaxUnavailResult2333{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			result.Summary.WithCustomMax++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobHistLimitResult2333 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs  int `json:"totalCronJobs"`
		WithHistoryLim int `json:"withHistoryLimits"`
	} `json:"summary"`
}

func (s *Server) handleCronJobHistLimit2333(w http.ResponseWriter, r *http.Request) {
	result := CronJobHistLimitResult2333{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Spec.SuccessfulJobsHistoryLimit != nil || cj.Spec.FailedJobsHistoryLimit != nil {
			result.Summary.WithHistoryLim++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
