package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.08 Operations: Node Capacity CPU, Pod HostNetwork Count, Container VolumeDevice Count
type NodeCapCPUResult2508 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCPU   float64 `json:"totalCPUCapacityCores"`
	} `json:"summary"`
}

func (s *Server) handleNodeCapCPU2508(w http.ResponseWriter, r *http.Request) {
	result := NodeCapCPUResult2508{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodHostNetCountResult2508 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		HostNetwork int `json:"hostNetworkPods"`
	} `json:"summary"`
}

func (s *Server) handlePodHostNetCount2508(w http.ResponseWriter, r *http.Request) {
	result := PodHostNetCountResult2508{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostNetwork {
			result.Summary.HostNetwork++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type VolumeDeviceResult2508 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalDevices    int `json:"totalVolumeDevices"`
	} `json:"summary"`
}

func (s *Server) handleVolumeDevice2508(w http.ResponseWriter, r *http.Request) {
	result := VolumeDeviceResult2508{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalDevices += len(c.VolumeDevices)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
