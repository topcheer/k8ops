package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.44 — Operations Dimension (Round 60)
// 1. Pod Container Exit Code Distribution
// 2. Node CPU Capacity vs Allocatable
// 3. Event Action Top Frequency
// ============================================================

type ExitCodeDistResult2244 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int         `json:"totalContainers"`
		ByExitCode      map[int]int `json:"byExitCode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleExitCodeDist2244(w http.ResponseWriter, r *http.Request) {
	result := ExitCodeDistResult2244{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByExitCode = make(map[int]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.ByExitCode[int(cs.LastTerminationState.Terminated.ExitCode)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node CPU Capacity vs Allocatable
type NodeCPUCapAllocResult2244 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCap   float64 `json:"totalCapacityCPU"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		GapPct     int     `json:"gapPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeCPUCapAlloc2244(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUCapAllocResult2244{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	if result.Summary.TotalCap > 0 {
		result.Summary.GapPct = int((1 - result.Summary.TotalAlloc/result.Summary.TotalCap) * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Action Top Frequency
type EvtActionResult2244 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByAction    map[string]int `json:"byAction"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtAction2244(w http.ResponseWriter, r *http.Request) {
	result := EvtActionResult2244{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByAction = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		action := evt.Action
		if action == "" {
			action = "Unknown"
		}
		result.Summary.ByAction[action]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
