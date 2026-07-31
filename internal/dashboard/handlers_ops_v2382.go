package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.82 Operations: Pod Phase Census, Node Capacity Pods, Container State Running
type PodPhaseResult2382 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePodPhase2382(w http.ResponseWriter, r *http.Request) {
	result := PodPhaseResult2382{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		result.Summary.ByPhase[string(pod.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCapPodsResult2382 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		TotalCapPods int `json:"totalPodCapacity"`
	} `json:"summary"`
}

func (s *Server) handleNodeCapPods2382(w http.ResponseWriter, r *http.Request) {
	result := NodeCapPodsResult2382{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapPods += int(node.Status.Capacity.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StateRunningResult2382 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Running         int `json:"runningState"`
	} `json:"summary"`
}

func (s *Server) handleStateRunning2382(w http.ResponseWriter, r *http.Request) {
	result := StateRunningResult2382{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
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
