package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.16 Product: Pod SchedulerName Audit, Container Resources Request Memory, Service ClusterIPs Count
type SchedulerNameResult2416 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByScheduler map[string]int `json:"bySchedulerName"`
	} `json:"summary"`
}

func (s *Server) handleSchedulerName2416(w http.ResponseWriter, r *http.Request) {
	result := SchedulerNameResult2416{ScannedAt: time.Now()}
	result.Summary.ByScheduler = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sn := pod.Spec.SchedulerName
		if sn == "" {
			sn = "<default-scheduler>"
		}
		result.Summary.ByScheduler[sn]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ReqMemResult2416 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalReqMemGB   float64 `json:"totalRequestedMemGB"`
	} `json:"summary"`
}

func (s *Server) handleReqMem2416(w http.ResponseWriter, r *http.Request) {
	result := ReqMemResult2416{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalReqMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterIPsResult2416 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices   int `json:"totalServices"`
		TotalClusterIPs int `json:"totalClusterIPs"`
	} `json:"summary"`
}

func (s *Server) handleClusterIPs2416(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPsResult2416{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.TotalClusterIPs += len(svc.Spec.ClusterIPs)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
