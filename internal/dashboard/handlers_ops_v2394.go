package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.94 Operations: Pod CrashLoopBackOff, Node Cond Memory, Container Restart Count
type CrashLoopResult2394 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		InCrashLoop     int `json:"inCrashLoopBackOff"`
	} `json:"summary"`
}

func (s *Server) handleCrashLoop2394(w http.ResponseWriter, r *http.Request) {
	result := CrashLoopResult2394{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.InCrashLoop++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.InCrashLoop*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeCondMemResult2394 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		MemPressure int `json:"memoryPressure"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondMem2394(w http.ResponseWriter, r *http.Request) {
	result := NodeCondMemResult2394{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.MemPressure++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.MemPressure*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RestartCountResult2394 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalRestarts   int `json:"totalRestarts"`
		AvgRestarts     int `json:"avgRestarts"`
	} `json:"summary"`
}

func (s *Server) handleRestartCount2394(w http.ResponseWriter, r *http.Request) {
	result := RestartCountResult2394{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			result.Summary.TotalRestarts += int(cs.RestartCount)
		}
	}
	if result.Summary.TotalContainers > 0 {
		result.Summary.AvgRestarts = result.Summary.TotalRestarts / result.Summary.TotalContainers
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
