package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.30 Operations: Pod Pending Phase Count, Node Cond Ready, Container Resources Limit Memory
type PodPendingResult2430 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Pending   int `json:"pendingPhase"`
	} `json:"summary"`
}

func (s *Server) handlePodPending2430(w http.ResponseWriter, r *http.Request) {
	result := PodPendingResult2430{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.Pending++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = 100 - (result.Summary.Pending*50)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeCondReadyResult2430 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		ReadyNodes int `json:"readyNodes"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondReady2430(w http.ResponseWriter, r *http.Request) {
	result := NodeCondReadyResult2430{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				result.Summary.ReadyNodes++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = result.Summary.ReadyNodes * 100 / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type LimitMemResult2430 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalLimitMemGB float64 `json:"totalLimitedMemGB"`
	} `json:"summary"`
}

func (s *Server) handleLimitMem2430(w http.ResponseWriter, r *http.Request) {
	result := LimitMemResult2430{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalLimitMemGB += c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
