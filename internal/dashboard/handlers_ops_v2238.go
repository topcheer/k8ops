package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.38 — Operations Dimension (Round 59)
// 1. Pod Container Restart Reason Catalog
// 2. Node Memory Capacity vs Allocatable Ratio
// 3. Event Reporting Component Distribution
// ============================================================

type RestartReasonResult2238 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		WithRestarts    int            `json:"withRestarts"`
		ByReason        map[string]int `json:"byReason"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRestartReason2238(w http.ResponseWriter, r *http.Request) {
	result := RestartReasonResult2238{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByReason = make(map[string]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.RestartCount > 0 {
				result.Summary.WithRestarts++
				if cs.LastTerminationState.Terminated != nil {
					reason := cs.LastTerminationState.Terminated.Reason
					if reason == "" {
						reason = "Unknown"
					}
					result.Summary.ByReason[reason]++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Memory Cap vs Alloc Ratio
type MemCapAllocRatioResult2238 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
		RatioPct     int     `json:"ratioPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMemCapAllocRatio2238(w http.ResponseWriter, r *http.Request) {
	result := MemCapAllocRatioResult2238{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	if result.Summary.TotalCapGB > 0 {
		result.Summary.RatioPct = int(result.Summary.TotalAllocGB / result.Summary.TotalCapGB * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Reporting Component
type EvtComponentResult2238 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByComponent map[string]int `json:"byReportingComponent"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtComponent2238(w http.ResponseWriter, r *http.Request) {
	result := EvtComponentResult2238{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByComponent = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		comp := evt.ReportingController
		if comp == "" {
			comp = "unknown"
		}
		result.Summary.ByComponent[comp]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
