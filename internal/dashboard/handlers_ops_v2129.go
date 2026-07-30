package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.29 — Operations Dimension (Round 41)
// 1. Node Allocatable Efficiency Ratio
// 2. Pod Container Restart Average
// 3. Service Port Range Distribution
// ============================================================

type AllocEffRatioResult2129 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         AllocEffRatioSummary2129 `json:"summary"`
	Recommendations []string                 `json:"recommendations"`
}

type AllocEffRatioSummary2129 struct {
	TotalNodes     int `json:"totalNodes"`
	AvgCPUAllocPct int `json:"avgCPUAllocatablePct"`
}

func (s *Server) handleAllocEffRatio2129(w http.ResponseWriter, r *http.Request) {
	result := AllocEffRatioResult2129{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	var totalPct int
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Capacity.Cpu().AsApproximateFloat64()
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		if cap > 0 {
			totalPct += int(alloc / cap * 100)
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUAllocPct = totalPct / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Container Restart Average
type RestartAvgResult2129 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         RestartAvgSummary2129 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type RestartAvgSummary2129 struct {
	TotalPods   int     `json:"totalPods"`
	AvgRestarts float64 `json:"avgRestartsPerPod"`
}

func (s *Server) handleRestartAvg2129(w http.ResponseWriter, r *http.Request) {
	result := RestartAvgResult2129{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalRestarts int32
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}
	}
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestarts = float64(totalRestarts) / float64(result.Summary.TotalPods)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Service Port Range Distribution
type PortRangeResult2129 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PortRangeSummary2129 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type PortRangeSummary2129 struct {
	TotalPorts int            `json:"totalPorts"`
	ByRange    map[string]int `json:"byPortRange"`
}

func (s *Server) handlePortRange2129(w http.ResponseWriter, r *http.Request) {
	result := PortRangeResult2129{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	byRange := make(map[string]int)
	for _, svc := range svcList.Items {
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			if p.Port < 1024 {
				byRange["well-known(0-1023)"]++
			} else if p.Port < 49152 {
				byRange["registered(1024-49151)"]++
			} else {
				byRange["dynamic(49152+)"]++
			}
		}
	}
	result.Summary.ByRange = byRange
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
