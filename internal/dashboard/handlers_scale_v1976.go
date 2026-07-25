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
// v19.76 — Scalability & HA Dimension (Round 15 Final)
// 1. Node Allocatable Gap — kubelet reserved vs allocatable resource analysis
// 2. Pod Overhead Ratio — resource request vs actual allocation efficiency
// 3. API Server QPS Estimator — estimated API server request load
// ============================================================

// ---------------------------------------------------------------
// 1. Node Allocatable Gap
// ---------------------------------------------------------------

type NodeAllocGapResult1976 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         NodeAllocGapSummary1976 `json:"summary"`
	PerNode         []NodeAllocGapEntry1976 `json:"perNode"`
	Recommendations []string                `json:"recommendations"`
}

type NodeAllocGapSummary1976 struct {
	TotalNodes       int     `json:"totalNodes"`
	TotalCapacity    float64 `json:"totalCapacityCPU"`
	TotalAllocatable float64 `json:"totalAllocatableCPU"`
	TotalReserved    float64 `json:"totalReservedCPU"`
	ReservationPct   float64 `json:"reservationPct"`
}

type NodeAllocGapEntry1976 struct {
	Name        string  `json:"name"`
	Capacity    float64 `json:"capacityCPU"`
	Allocatable float64 `json:"allocatableCPU"`
	Reserved    float64 `json:"reservedCPU"`
	ReservedPct float64 `json:"reservedPct"`
}

func (s *Server) handleNodeAllocatableGap(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocGapResult1976{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		cap := node.Status.Capacity.Cpu().AsApproximateFloat64()
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		reserved := cap - alloc

		entry := NodeAllocGapEntry1976{
			Name: node.Name, Capacity: cap,
			Allocatable: alloc, Reserved: reserved,
		}
		if cap > 0 {
			entry.ReservedPct = reserved / cap * 100
		}

		result.Summary.TotalCapacity += cap
		result.Summary.TotalAllocatable += alloc
		result.Summary.TotalReserved += reserved

		if entry.ReservedPct > 15 {
			score -= 2
		}

		result.PerNode = append(result.PerNode, entry)
	}

	if result.Summary.TotalCapacity > 0 {
		result.Summary.ReservationPct = result.Summary.TotalReserved / result.Summary.TotalCapacity * 100
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, cap %.1f CPU, allocatable %.1f, reserved %.1f%%", result.Summary.TotalNodes, result.Summary.TotalCapacity, result.Summary.TotalAllocatable, result.Summary.ReservationPct))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Overhead Ratio
// ---------------------------------------------------------------

type PodOverheadResult1976 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PodOverheadSummary1976 `json:"summary"`
	HighOverhead    []PodOverheadEntry1976 `json:"highOverheadPods"`
	Recommendations []string               `json:"recommendations"`
}

type PodOverheadSummary1976 struct {
	TotalPods         int     `json:"totalPods"`
	AvgOverheadPct    float64 `json:"avgOverheadPct"`
	HighOverheadCount int     `json:"highOverheadPods"`
	TotalRequestedCPU float64 `json:"totalRequestedCPU"`
	TotalRequestedMem float64 `json:"totalRequestedMemGB"`
}

type PodOverheadEntry1976 struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	CPUReq         float64 `json:"cpuRequest"`
	MemReq         float64 `json:"memRequestGB"`
	ContainerCount int     `json:"containerCount"`
	OverheadPct    float64 `json:"overheadPct"`
}

func (s *Server) handlePodOverheadRatio(w http.ResponseWriter, r *http.Request) {
	result := PodOverheadResult1976{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalOverhead float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		count++

		cpuReq := 0.0
		memReq := 0.0
		containerCount := len(pod.Spec.Containers)

		for _, c := range pod.Spec.Containers {
			cpuReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			memReq += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}

		// Overhead estimation: init containers + sidecar overhead
		// More containers = higher coordination overhead
		overheadPct := float64(containerCount-1) * 5.0
		if containerCount == 1 {
			overheadPct = 0
		}
		if len(pod.Spec.InitContainers) > 0 {
			overheadPct += float64(len(pod.Spec.InitContainers)) * 3.0
		}

		// Large requests also add overhead (memory page tables, etc.)
		if cpuReq > 4 {
			overheadPct += 5
		}
		if memReq > 8 {
			overheadPct += 5
		}

		result.Summary.TotalRequestedCPU += cpuReq
		result.Summary.TotalRequestedMem += memReq
		totalOverhead += overheadPct

		if overheadPct > 15 {
			result.Summary.HighOverheadCount++
			result.HighOverhead = append(result.HighOverhead, PodOverheadEntry1976{
				Name: pod.Name, Namespace: pod.Namespace,
				CPUReq: cpuReq, MemReq: memReq,
				ContainerCount: containerCount, OverheadPct: overheadPct,
			})
			score -= 1
		}
	}

	if count > 0 {
		result.Summary.AvgOverheadPct = totalOverhead / float64(count)
	}

	sort.Slice(result.HighOverhead, func(i, j int) bool {
		return result.HighOverhead[i].OverheadPct > result.HighOverhead[j].OverheadPct
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg overhead %.1f%%, %d high overhead", result.Summary.TotalPods, result.Summary.AvgOverheadPct, result.Summary.HighOverheadCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. API Server QPS Estimator
// ------------------------------------------------===============

type APIQPSResult1976 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         APIQPSSummary1976   `json:"summary"`
	PerNS           []APIQPENSEntry1976 `json:"perNamespace"`
	Recommendations []string            `json:"recommendations"`
}

type APIQPSSummary1976 struct {
	TotalPods        int     `json:"totalPods"`
	TotalControllers int     `json:"estimatedControllers"`
	EstQPS           float64 `json:"estimatedQPS"`
	PressureLevel    string  `json:"pressureLevel"`
	RecommendedLimit int     `json:"recommendedAPIQPSLimit"`
}

type APIQPENSEntry1976 struct {
	Namespace string  `json:"namespace"`
	PodCount  int     `json:"podCount"`
	EstQPS    float64 `json:"estQPS"`
}

func (s *Server) handleAPIServerQPSEst(w http.ResponseWriter, r *http.Request) {
	result := APIQPSResult1976{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	// Estimate QPS: each pod generates ~0.5 QPS (watch + periodic)
	// Each controller generates ~5 QPS
	const qpsPerPod = 0.5
	const qpsPerController = 5.0
	const baseQPS = 10.0

	nsStats := make(map[string]int)
	totalPods := 0
	controllerPods := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		totalPods++
		nsStats[pod.Namespace]++

		// Detect controller pods (heuristic)
		for _, label := range []string{"control-plane", "app.kubernetes.io/component"} {
			v := pod.Labels[label]
			if v == "controller-manager" || v == "operator" {
				controllerPods++
				break
			}
		}
	}

	// Estimate controllers
	estControllers := controllerPods + len(nsList.Items)/5 + 5 // base system controllers

	estQPS := baseQPS + float64(totalPods)*qpsPerPod + float64(estControllers)*qpsPerController

	result.Summary.TotalPods = totalPods
	result.Summary.TotalControllers = estControllers
	result.Summary.EstQPS = estQPS

	// Recommended API QPS limit (usually 5x estimated)
	result.Summary.RecommendedLimit = int(estQPS * 5)

	// Pressure level
	if estQPS > 500 {
		result.Summary.PressureLevel = "critical"
		score -= 10
	} else if estQPS > 200 {
		result.Summary.PressureLevel = "high"
		score -= 5
	} else if estQPS > 100 {
		result.Summary.PressureLevel = "medium"
	} else {
		result.Summary.PressureLevel = "low"
	}

	for ns, count := range nsStats {
		result.PerNS = append(result.PerNS, APIQPENSEntry1976{
			Namespace: ns, PodCount: count,
			EstQPS: float64(count) * qpsPerPod,
		})
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EstQPS > result.PerNS[j].EstQPS
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Est QPS: %.0f (%s), ~%d controllers, recommended limit: %d", estQPS, result.Summary.PressureLevel, estControllers, result.Summary.RecommendedLimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
