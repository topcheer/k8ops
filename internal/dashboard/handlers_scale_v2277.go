package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.77 Scalability: Cluster CPU Utilization Ratio, Cluster Memory Utilization Ratio, Node Pod Capacity Usage
type CPUUtilRatioResult2277 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalAllocCPU float64 `json:"totalAllocatableCPU"`
		TotalReqCPU   float64 `json:"totalRequestedCPU"`
		TotalLimitCPU float64 `json:"totalLimitedCPU"`
		UtilPct       int     `json:"utilizationPct"`
	} `json:"summary"`
}

func (s *Server) handleCPUUtilRatio2277(w http.ResponseWriter, r *http.Request) {
	result := CPUUtilRatioResult2277{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalLimitCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	if result.Summary.TotalAllocCPU > 0 {
		result.Summary.UtilPct = int(result.Summary.TotalReqCPU * 100 / result.Summary.TotalAllocCPU)
	}
	score := 100
	if result.Summary.UtilPct > 80 {
		score = 60
	} else if result.Summary.UtilPct > 60 {
		score = 80
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type MemUtilRatioResult2277 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalAllocMemGB float64 `json:"totalAllocatableMemGB"`
		TotalReqMemGB   float64 `json:"totalRequestedMemGB"`
		UtilPct         int     `json:"utilizationPct"`
	} `json:"summary"`
}

func (s *Server) handleMemUtilRatio2277(w http.ResponseWriter, r *http.Request) {
	result := MemUtilRatioResult2277{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalAllocMemGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.TotalAllocMemGB > 0 {
		result.Summary.UtilPct = int(result.Summary.TotalReqMemGB * 100 / result.Summary.TotalAllocMemGB)
	}
	score := 100
	if result.Summary.UtilPct > 80 {
		score = 60
	} else if result.Summary.UtilPct > 60 {
		score = 80
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodePodCapacityResult2277 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCap   int `json:"totalPodCapacity"`
		TotalPods  int `json:"totalRunningPods"`
		UtilPct    int `json:"utilizationPct"`
	} `json:"summary"`
}

func (s *Server) handleNodePodCapacity2277(w http.ResponseWriter, r *http.Request) {
	result := NodePodCapacityResult2277{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Allocatable.Pods().Value()
		result.Summary.TotalCap += int(cap)
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	if result.Summary.TotalCap > 0 {
		result.Summary.UtilPct = result.Summary.TotalPods * 100 / result.Summary.TotalCap
	}
	score := 100
	if result.Summary.UtilPct > 80 {
		score = 60
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
