package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.14 Operations: Node Allocatable CPU, Pod Completed Count, Container Restart Total
type NodeAllocCPUResult2514 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPUCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocCPU2514(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocCPUResult2514{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodCompletedCountResult2514 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Completed int `json:"completedPods"`
	} `json:"summary"`
}

func (s *Server) handlePodCompletedCount2514(w http.ResponseWriter, r *http.Request) {
	result := PodCompletedCountResult2514{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodSucceeded {
			result.Summary.Completed++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrRestartTotalResult2514 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalRestarts   int `json:"totalRestarts"`
	} `json:"summary"`
}

func (s *Server) handleCtnrRestartTotal2514(w http.ResponseWriter, r *http.Request) {
	result := CtnrRestartTotalResult2514{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			result.Summary.TotalRestarts += int(cs.RestartCount)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
