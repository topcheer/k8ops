package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.64 Operations: Pod Start Time Audit, Node Architecture Census, Event Recent Count
type PodStartTimeResult2364 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithStartTime int `json:"withStartTime"`
	} `json:"summary"`
}

func (s *Server) handlePodStartTime2364(w http.ResponseWriter, r *http.Request) {
	result := PodStartTimeResult2364{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Status.StartTime != nil {
			result.Summary.WithStartTime++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeArchResult2364 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	} `json:"summary"`
}

func (s *Server) handleNodeArch2364(w http.ResponseWriter, r *http.Request) {
	result := NodeArchResult2364{ScannedAt: time.Now()}
	result.Summary.ByArch = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByArch[node.Status.NodeInfo.Architecture]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventRecentResult2364 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents  int `json:"totalEvents"`
		RecentEvents int `json:"recentEvents1h"`
	} `json:"summary"`
}

func (s *Server) handleEventRecent2364(w http.ResponseWriter, r *http.Request) {
	result := EventRecentResult2364{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if evt.LastTimestamp.Time.After(now.Add(-time.Hour)) {
			result.Summary.RecentEvents++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
