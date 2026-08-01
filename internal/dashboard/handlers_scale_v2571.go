package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.71 Scalability: Top Namespace by Secret v2, Node CPU Request vs Allocatable, Cluster CronJob Schedule
type TopNSBySecret2Result2571 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace   string `json:"namespace"`
		SecretCount int    `json:"secretCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSBySecret2Result2571(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySecret2Result2571{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	nsSecrets := make(map[string]int)
	for _, secret := range secretList.Items {
		nsSecrets[secret.Namespace]++
	}
	result.Summary.TotalNS = len(nsSecrets)
	for ns, count := range nsSecrets {
		result.TopNS = append(result.TopNS, struct {
			Namespace   string `json:"namespace"`
			SecretCount int    `json:"secretCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SecretCount > result.TopNS[j].SecretCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUReqVsAllocResult2571 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalCPUAllocatable"`
		TotalReq   float64 `json:"totalCPURequest"`
	}
}

func (s *Server) handleNodeCPUReqVsAlloc2571(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUReqVsAllocResult2571{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	var totalReq float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			totalReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalReq = totalReq
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CronJobScheduleResult2571 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCronJobs int            `json:"totalCronJobs"`
		BySchedule    map[string]int `json:"bySchedule"`
	}
}

func (s *Server) handleCronJobSchedule2571(w http.ResponseWriter, r *http.Request) {
	result := CronJobScheduleResult2571{ScannedAt: time.Now()}
	result.Summary.BySchedule = make(map[string]int)
	cronList, _ := s.clientset.BatchV1().CronJobs("").List(r.Context(), metav1.ListOptions{})
	for _, cron := range cronList.Items {
		result.Summary.TotalCronJobs++
		result.Summary.BySchedule[cron.Spec.Schedule]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
