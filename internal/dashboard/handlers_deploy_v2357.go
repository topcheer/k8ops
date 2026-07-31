package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.57 Deployment: DS NodeName Target, STS Pod Management Policy, CronJob Time Zone
type DSNodeNameResult2357 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS      int `json:"totalDS"`
		WithNodeName int `json:"withNodeNameTarget"`
	} `json:"summary"`
}

func (s *Server) handleDSNodeName2357(w http.ResponseWriter, r *http.Request) {
	result := DSNodeNameResult2357{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.Template.Spec.NodeName != "" {
			result.Summary.WithNodeName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSPodMgmtResult2357 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int            `json:"totalSTS"`
		ByPolicy map[string]int `json:"byPodManagementPolicy"`
	} `json:"summary"`
}

func (s *Server) handleSTSPodMgmt2357(w http.ResponseWriter, r *http.Request) {
	result := STSPodMgmtResult2357{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		result.Summary.ByPolicy[string(sts.Spec.PodManagementPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobTZResult2357 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		ByTimeZone    map[string]int `json:"byTimeZone"`
	} `json:"summary"`
}

func (s *Server) handleCronJobTZ2357(w http.ResponseWriter, r *http.Request) {
	result := CronJobTZResult2357{ScannedAt: time.Now()}
	result.Summary.ByTimeZone = make(map[string]int)
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cj := range cronList.Items {
		result.Summary.TotalCronJobs++
		tz := "UTC"
		if cj.Spec.TimeZone != nil {
			tz = *cj.Spec.TimeZone
		}
		result.Summary.ByTimeZone[tz]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
