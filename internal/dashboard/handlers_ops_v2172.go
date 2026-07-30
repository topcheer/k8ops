package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.72 — Operations Dimension (Round 48)
// 1. Pod Image Version Freshness
// 2. Node Allocatable CPU Ratio
// 3. Event Type Distribution
// ============================================================

type ImgFreshnessResult2172 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByTag       map[string]int `json:"byTag"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgFreshness2172(w http.ResponseWriter, r *http.Request) {
	result := ImgFreshnessResult2172{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByTag = make(map[string]int)
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seen[c.Image] {
				continue
			}
			seen[c.Image] = true
			result.Summary.TotalImages++
			tag := "latest"
			for i := len(c.Image) - 1; i >= 0; i-- {
				if c.Image[i] == ':' {
					tag = c.Image[i+1:]
					break
				}
				if c.Image[i] == '/' {
					break
				}
			}
			result.Summary.ByTag[tag]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Allocatable CPU Ratio
type AllocCPURatioResult2172 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int     `json:"totalNodes"`
		TotalCapCPU   float64 `json:"totalCapacityCPU"`
		TotalAllocCPU float64 `json:"totalAllocatableCPU"`
		RatioPct      int     `json:"ratioPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAllocCPURatio2172(w http.ResponseWriter, r *http.Request) {
	result := AllocCPURatioResult2172{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	if result.Summary.TotalCapCPU > 0 {
		result.Summary.RatioPct = int(result.Summary.TotalAllocCPU / result.Summary.TotalCapCPU * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Type Distribution
type EvtTypeDistResult2172 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByType      map[string]int `json:"byType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtTypeDist2172(w http.ResponseWriter, r *http.Request) {
	result := EvtTypeDistResult2172{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByType = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		t := string(evt.Type)
		if t == "" {
			t = "Normal"
		}
		result.Summary.ByType[t]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
