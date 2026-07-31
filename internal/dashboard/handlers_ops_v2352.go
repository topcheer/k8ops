package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.52 Operations: Pod Waiting Reason, Node Memory Allocatable GB, Container Resources Limit CPU
type WaitingReasonResult2352 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalWaiting int            `json:"totalWaiting"`
		ByReason     map[string]int `json:"byReason"`
	} `json:"summary"`
}

func (s *Server) handleWaitingReason2352(w http.ResponseWriter, r *http.Request) {
	result := WaitingReasonResult2352{ScannedAt: time.Now()}
	result.Summary.ByReason = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				result.Summary.TotalWaiting++
				result.Summary.ByReason[cs.State.Waiting.Reason]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemAllocResult2352 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalAllocGB float64 `json:"totalAllocatableMemGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemAlloc2352(w http.ResponseWriter, r *http.Request) {
	result := NodeMemAllocResult2352{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LimitCPUResult2352 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalLimitCPU   float64 `json:"totalLimitedCPU"`
	} `json:"summary"`
}

func (s *Server) handleLimitCPU2352(w http.ResponseWriter, r *http.Request) {
	result := LimitCPUResult2352{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalLimitCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
