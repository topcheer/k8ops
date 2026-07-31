package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.75 Deployment: Deployment MaxSurge Config, STS serviceName Empty Audit, CronJob Suspend Status
type MaxSurgeResult2375 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithCustom   int `json:"withCustomMaxSurge"`
	} `json:"summary"`
}

func (s *Server) handleMaxSurge2375(w http.ResponseWriter, r *http.Request) {
	result := MaxSurgeResult2375{ScannedAt: time.Now()}
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range depList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			result.Summary.WithCustom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSSvcNameEmptyResult2375 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS  int `json:"totalSTS"`
		EmptyName int `json:"withEmptyServiceName"`
	} `json:"summary"`
}

func (s *Server) handleSTSSvcNameEmpty2375(w http.ResponseWriter, r *http.Request) {
	result := STSSvcNameEmptyResult2375{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if sts.Spec.ServiceName == "" {
			result.Summary.EmptyName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobSuspendResult2375 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int `json:"totalCronJobs"`
		Suspended     int `json:"suspended"`
	} `json:"summary"`
}

func (s *Server) handleCronJobSuspend2375(w http.ResponseWriter, r *http.Request) {
	result := CronJobSuspendResult2375{ScannedAt: time.Now()}
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			result.Summary.Suspended++
		}
	}
	score := 100
	if result.Summary.TotalCronJobs > 0 && result.Summary.Suspended > 0 {
		score = 100 - (result.Summary.Suspended*30)/result.Summary.TotalCronJobs
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
