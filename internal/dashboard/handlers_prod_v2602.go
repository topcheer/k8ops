package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.02 Product: Pod Spec Overhead, Container Resource Limit CPU Detail, Service PublishNotReadyAddresses
type PodOverheadResult2602 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithOverhead int `json:"withOverhead"`
	} `json:"summary"`
}

func (s *Server) handlePodOverhead2602(w http.ResponseWriter, r *http.Request) {
	result := PodOverheadResult2602{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Overhead != nil && len(pod.Spec.Overhead) > 0 {
			result.Summary.WithOverhead++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPULimitDetail2Result2602 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPULim     float64 `json:"totalCPULimit"`
	} `json:"summary"`
}

func (s *Server) handleCPULimitDetail2Result2602(w http.ResponseWriter, r *http.Request) {
	result := CPULimitDetail2Result2602{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPULim += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcPublishNotReadyResult2602 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		Publish   int `json:"publishNotReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handleSvcPublishNotReady2602(w http.ResponseWriter, r *http.Request) {
	result := SvcPublishNotReadyResult2602{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.Publish++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
