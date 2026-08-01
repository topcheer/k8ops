package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.18 Operations: Pod PodIP Distribution, Node MachineInfo, Event FirstTimestamp Age
type PodIPResult2418 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithPodIP int `json:"withPodIP"`
	} `json:"summary"`
}

func (s *Server) handlePodIP2418(w http.ResponseWriter, r *http.Request) {
	result := PodIPResult2418{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Status.PodIP != "" {
			result.Summary.WithPodIP++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MachineInfoResult2418 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByMachine  map[string]int `json:"byMachineID"`
	} `json:"summary"`
}

func (s *Server) handleMachineInfo2418(w http.ResponseWriter, r *http.Request) {
	result := MachineInfoResult2418{ScannedAt: time.Now()}
	result.Summary.ByMachine = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByMachine[node.Status.NodeInfo.MachineID]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventFirstTSResult2418 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents  int `json:"totalEvents"`
		RecentEvents int `json:"recentFirstTimestamp1h"`
	} `json:"summary"`
}

func (s *Server) handleEventFirstTS2418(w http.ResponseWriter, r *http.Request) {
	result := EventFirstTSResult2418{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if evt.FirstTimestamp.Time.After(now.Add(-time.Hour)) {
			result.Summary.RecentEvents++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
