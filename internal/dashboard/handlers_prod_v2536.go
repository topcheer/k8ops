package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.36 Product: Pod Spec CPU Request Summary, Container Image VolumeMount Detail, Service Ports Summary
type CPUReqSummaryResult2536 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPURequestCores"`
	} `json:"summary"`
}

func (s *Server) handleCPUReqSummary2536(w http.ResponseWriter, r *http.Request) {
	result := CPUReqSummaryResult2536{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type VolumeMountDetailResult2536 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnlyMounts  int `json:"readOnlyMounts"`
	} `json:"summary"`
}

func (s *Server) handleVolumeMountDetail2536(w http.ResponseWriter, r *http.Request) {
	result := VolumeMountDetailResult2536{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, vm := range c.VolumeMounts {
				if vm.ReadOnly {
					result.Summary.ReadOnlyMounts++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ServicePortsSummaryResult2536 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int            `json:"totalServices"`
		ByProtocol map[string]int `json:"byProtocol"`
	} `json:"summary"`
}

func (s *Server) handleServicePortsSummary2536(w http.ResponseWriter, r *http.Request) {
	result := ServicePortsSummaryResult2536{ScannedAt: time.Now()}
	result.Summary.ByProtocol = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		for _, port := range svc.Spec.Ports {
			result.Summary.ByProtocol[string(port.Protocol)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
