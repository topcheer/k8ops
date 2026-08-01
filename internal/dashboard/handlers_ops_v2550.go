package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.50 Operations: Node Capacity Memory, Pod Spec Priority Summary, Container Resource Summary
type NodeCapMemResult2550 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalMemGB float64 `json:"totalCapacityMemGB"`
	}
}

func (s *Server) handleNodeCapMem2550(w http.ResponseWriter, r *http.Request) {
	result := NodeCapMemResult2550{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalMemGB += node.Status.Capacity.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodPrioritySummaryResult2550 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithPri   int `json:"withPriority"`
	}
}

func (s *Server) handlePodPrioritySummary2550(w http.ResponseWriter, r *http.Request) {
	result := PodPrioritySummaryResult2550{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Priority != nil {
			result.Summary.WithPri++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceSummaryResult2550 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemReqMB   float64 `json:"totalMemReqMB"`
		TotalMemLimMB   float64 `json:"totalMemLimMB"`
	}
}

func (s *Server) handleResourceSummary2550(w http.ResponseWriter, r *http.Request) {
	result := ResourceSummaryResult2550{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
			result.Summary.TotalMemLimMB += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
