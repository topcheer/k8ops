package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.75 — Operations Dimension (Round 32)
// 1. Pod Crash Pattern Detector — crash loop back-off pattern
// 2. Container Image Pull Time — slow image pull estimation
// 3. Node Allocatable Efficiency — allocatable vs capacity ratio
// ============================================================

type CrashPatternResult2075 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         CrashPatternSummary2075 `json:"summary"`
	CrashLoopPods   []CrashPatternEntry2075 `json:"crashLoopPods"`
	Recommendations []string                `json:"recommendations"`
}

type CrashPatternSummary2075 struct {
	TotalPods     int `json:"totalPods"`
	CrashLoopPods int `json:"crashLoopPods"`
}

type CrashPatternEntry2075 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Restarts  int32  `json:"restarts"`
}

func (s *Server) handleCrashPatternDetect(w http.ResponseWriter, r *http.Request) {
	result := CrashPatternResult2075{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		var maxRestarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > maxRestarts {
				maxRestarts = cs.RestartCount
			}
		}
		if maxRestarts >= 5 {
			result.Summary.CrashLoopPods++
			result.CrashLoopPods = append(result.CrashLoopPods, CrashPatternEntry2075{
				Pod: pod.Name, Namespace: pod.Namespace, Restarts: maxRestarts,
			})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.CrashLoopPods, func(i, j int) bool { return result.CrashLoopPods[i].Restarts > result.CrashLoopPods[j].Restarts })

	if result.Summary.CrashLoopPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods in crash loop — investigate app logs", result.Summary.CrashLoopPods))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Image Pull Time
// ---------------------------------------------------------------

type PullTimeResult2075 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PullTimeSummary2075 `json:"summary"`
	SlowPulls       []PullTimeEntry2075 `json:"slowPullImages"`
	Recommendations []string            `json:"recommendations"`
}

type PullTimeSummary2075 struct {
	TotalImages  int `json:"totalImages"`
	LargeImages  int `json:"largeImages"`
	AlwaysPolicy int `json:"alwaysPolicy"`
}

type PullTimeEntry2075 struct {
	Image string `json:"image"`
}

func (s *Server) handlePullTimeEst2075(w http.ResponseWriter, r *http.Request) {
	result := PullTimeResult2075{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	seenImg := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seenImg[c.Image] {
				continue
			}
			seenImg[c.Image] = true
			result.Summary.TotalImages++

			if string(c.ImagePullPolicy) == "Always" {
				result.Summary.AlwaysPolicy++
				result.SlowPulls = append(result.SlowPulls, PullTimeEntry2075{Image: c.Image})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.AlwaysPolicy > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d images with Always pull policy — use IfNotPresent for pinned tags", result.Summary.AlwaysPolicy))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Allocatable Efficiency
// ---------------------------------------------------------------

type AllocEffResult2075 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AllocEffSummary2075 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type AllocEffSummary2075 struct {
	TotalNodes int `json:"totalNodes"`
	AvgCPUEff  int `json:"avgCpuEfficiencyPct"`
	AvgMemEff  int `json:"avgMemEfficiencyPct"`
}

func (s *Server) handleAllocEff2075(w http.ResponseWriter, r *http.Request) {
	result := AllocEffResult2075{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	var totalCPU, totalMem int
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		capCPU := node.Status.Capacity.Cpu().AsApproximateFloat64()
		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		capMem := node.Status.Capacity.Memory().AsApproximateFloat64()
		allocMem := node.Status.Allocatable.Memory().AsApproximateFloat64()

		if capCPU > 0 {
			totalCPU += int(allocCPU / capCPU * 100)
		}
		if capMem > 0 {
			totalMem += int(allocMem / capMem * 100)
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUEff = totalCPU / result.Summary.TotalNodes
		result.Summary.AvgMemEff = totalMem / result.Summary.TotalNodes
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
