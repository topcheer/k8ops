package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.42 Operations: Pod Restart Total, Node Memory Pressure, Event Last Timestamp Spread
type PodRestartTotalResult2442 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		TotalRestarts int `json:"totalRestarts"`
		HighRestart   int `json:"highRestartPods"`
	} `json:"summary"`
}

func (s *Server) handlePodRestartTotal2442(w http.ResponseWriter, r *http.Request) {
	result := PodRestartTotalResult2442{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalRestarts += int(cs.RestartCount)
			if cs.RestartCount > 5 {
				result.Summary.HighRestart++
			}
		}
	}
	score := 100
	if result.Summary.HighRestart > 0 {
		score = 100 - result.Summary.HighRestart*5
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeMemPressureResult2442 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		MemPressure int `json:"memoryPressure"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemPressure2442(w http.ResponseWriter, r *http.Request) {
	result := NodeMemPressureResult2442{ScannedAt: time.Now()}
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

type EventTimestampSpreadResult2442 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int `json:"totalEvents"`
		OldestAgeHr int `json:"oldestAgeHours"`
	} `json:"summary"`
}

func (s *Server) handleEventTimestampSpread2442(w http.ResponseWriter, r *http.Request) {
	result := EventTimestampSpreadResult2442{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var oldest time.Time
	for _, ev := range eventList.Items {
		result.Summary.TotalEvents++
		if ev.LastTimestamp.Time.After(oldest) {
			// track oldest = first event seen
			if oldest.IsZero() || ev.LastTimestamp.Time.Before(oldest) {
				oldest = ev.LastTimestamp.Time
			}
		}
	}
	if !oldest.IsZero() {
		result.Summary.OldestAgeHr = int(now.Sub(oldest).Hours())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
