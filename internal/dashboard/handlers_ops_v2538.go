package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.38 Operations: Node Allocatable Pods Detail, Pod Spec CPU Limit Total, Container State Running Detail
type NodeAllocPodsDetailResult2538 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalPods  int `json:"totalAllocatablePods"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocPodsDetail2538(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocPodsDetailResult2538{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalPods += int(node.Status.Allocatable.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodCPULimitTotalResult2538 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int     `json:"totalPods"`
		TotalCPULim float64 `json:"totalCPULimitCores"`
	} `json:"summary"`
}

func (s *Server) handlePodCPULimitTotal2538(w http.ResponseWriter, r *http.Request) {
	result := PodCPULimitTotalResult2538{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalCPULim += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RunningStateDetailResult2538 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Running         int `json:"running"`
	} `json:"summary"`
}

func (s *Server) handleRunningStateDetail2538(w http.ResponseWriter, r *http.Request) {
	result := RunningStateDetailResult2538{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Running != nil {
				result.Summary.Running++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.Running * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
