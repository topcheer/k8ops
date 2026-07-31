package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.34 Operations: Pod FailedScheduling Census, Node Cond Network, Container Resources Summary
type FailedSchedResult2334 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		FailedSched int `json:"failedScheduling"`
	} `json:"summary"`
}

func (s *Server) handleFailedSched2334(w http.ResponseWriter, r *http.Request) {
	result := FailedSchedResult2334{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodPending {
			for _, cond := range pod.Status.Conditions {
				if string(cond.Reason) == "Unschedulable" {
					result.Summary.FailedSched++
					break
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.FailedSched > 0 {
		score = 100 - (result.Summary.FailedSched*50)/result.Summary.TotalPods
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeCondNetResult2334 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		NetUnavailable int `json:"networkUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondNet2334(w http.ResponseWriter, r *http.Request) {
	result := NodeCondNetResult2334{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
				result.Summary.NetUnavailable++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.NetUnavailable*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ResSummaryResult2334 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalReqCPU     float64 `json:"totalRequestedCPU"`
		TotalReqMemGB   float64 `json:"totalRequestedMemGB"`
		TotalLimitCPU   float64 `json:"totalLimitedCPU"`
		TotalLimitMemGB float64 `json:"totalLimitedMemGB"`
	} `json:"summary"`
}

func (s *Server) handleResSummary2334(w http.ResponseWriter, r *http.Request) {
	result := ResSummaryResult2334{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalReqMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			result.Summary.TotalLimitCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
			result.Summary.TotalLimitMemGB += c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
