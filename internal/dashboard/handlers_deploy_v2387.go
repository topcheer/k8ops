package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.87 Deployment: Deployment Label Count, STS VolumeClaim Count, CronJob Failed Jobs
type DepLabelCountResult2387 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		TotalLabels  int `json:"totalLabels"`
	} `json:"summary"`
}

func (s *Server) handleDepLabelCount2387(w http.ResponseWriter, r *http.Request) {
	result := DepLabelCountResult2387{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		result.Summary.TotalLabels += len(dep.Labels)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSVolClaimResult2387 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS    int `json:"totalSTS"`
		TotalClaims int `json:"totalVolumeClaimTemplates"`
	} `json:"summary"`
}

func (s *Server) handleSTSVolClaim2387(w http.ResponseWriter, r *http.Request) {
	result := STSVolClaimResult2387{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.TotalClaims += len(sts.Spec.VolumeClaimTemplates)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobFailedResult2387 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		TotalFailed   int `json:"totalFailedJobs"`
	} `json:"summary"`
}

func (s *Server) handleCronJobFailed2387(w http.ResponseWriter, r *http.Request) {
	result := CronJobFailedResult2387{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Status.LastSuccessfulTime == nil {
			result.Summary.TotalFailed++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
