package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.48 Product: Pod Spec OS Name, Container Resource Request vs Limit, Service Type Summary
type PodOSNameResult2548 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByOS      map[string]int `json:"byOSName"`
	}
}

func (s *Server) handlePodOSName2548(w http.ResponseWriter, r *http.Request) {
	result := PodOSNameResult2548{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.ByOS[string(pod.Spec.OS.Name)]++
		} else {
			result.Summary.ByOS["linux"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ReqVsLimitResult2548 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReq"`
		TotalCPULim     float64 `json:"totalCPULim"`
	}
}

func (s *Server) handleReqVsLimit2548(w http.ResponseWriter, r *http.Request) {
	result := ReqVsLimitResult2548{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalCPULim += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ServiceTypeSummaryResult2548 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByType    map[string]int `json:"byType"`
	}
}

func (s *Server) handleServiceTypeSummary2548(w http.ResponseWriter, r *http.Request) {
	result := ServiceTypeSummaryResult2548{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByType[string(svc.Spec.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
