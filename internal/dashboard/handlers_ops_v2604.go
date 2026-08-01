package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.04 Operations: Node CPU vs Memory Allocatable Ratio, Pod Spec InitContainer Count, Container StdinOnce
type NodeCPUvsMemResult2604 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgRatio   float64 `json:"avgCPUvsMemRatio"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUvsMem2604(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUvsMemResult2604{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cpu := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memGB := node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		if memGB > 0 {
			totalRatio += cpu / memGB
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgRatio = totalRatio / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type InitContainerCountResult2604 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		TotalInit int `json:"totalInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleInitContainerCount2604(w http.ResponseWriter, r *http.Request) {
	result := InitContainerCountResult2604{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalInit += len(pod.Spec.InitContainers)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StdinOnceResult2604 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdinOnce   int `json:"withStdinOnce"`
	} `json:"summary"`
}

func (s *Server) handleStdinOnce2604(w http.ResponseWriter, r *http.Request) {
	result := StdinOnceResult2604{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StdinOnce {
				result.Summary.WithStdinOnce++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
