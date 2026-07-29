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
// v20.90 — Scalability & HA Dimension (Round 34)
// 1. Pod Scheduling Latency Score — pending pod wait time
// 2. Resource Overcommit Ratio — requested vs allocatable
// 3. Node Pod Density Distribution — pods per node histogram
// ============================================================

type SchedLatResult2090 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SchedLatSummary2090 `json:"summary"`
	PendingPods     []SchedLatEntry2090 `json:"pendingPods"`
	Recommendations []string            `json:"recommendations"`
}

type SchedLatSummary2090 struct {
	TotalPods   int `json:"totalPods"`
	PendingPods int `json:"pendingPods"`
}

type SchedLatEntry2090 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	WaitMins  int    `json:"waitMinutes"`
}

func (s *Server) handleSchedLat2090(w http.ResponseWriter, r *http.Request) {
	result := SchedLatResult2090{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			waitMin := int(now.Sub(pod.CreationTimestamp.Time).Minutes())
			result.PendingPods = append(result.PendingPods, SchedLatEntry2090{
				Pod: pod.Name, Namespace: pod.Namespace, WaitMins: waitMin,
			})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PendingPods, func(i, j int) bool { return result.PendingPods[i].WaitMins > result.PendingPods[j].WaitMins })

	if result.Summary.PendingPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods pending scheduling", result.Summary.PendingPods))
	}
	writeJSON(w, result)
}

// 2. Resource Overcommit Ratio
type OvercommitResult2090 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         OvercommitSummary2090 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type OvercommitSummary2090 struct {
	AllocatableCPU float64 `json:"allocatableCPU"`
	RequestedCPU   float64 `json:"requestedCPU"`
	CPUOvercommit  int     `json:"cpuOvercommitPct"`
	AllocatableMem float64 `json:"allocatableMemGB"`
	RequestedMem   float64 `json:"requestedMemGB"`
	MemOvercommit  int     `json:"memOvercommitPct"`
}

func (s *Server) handleOvercommit2090(w http.ResponseWriter, r *http.Request) {
	result := OvercommitResult2090{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.AllocatableMem += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.RequestedCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.RequestedMem += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.AllocatableCPU > 0 {
		result.Summary.CPUOvercommit = int(result.Summary.RequestedCPU / result.Summary.AllocatableCPU * 100)
	}
	if result.Summary.AllocatableMem > 0 {
		result.Summary.MemOvercommit = int(result.Summary.RequestedMem / result.Summary.AllocatableMem * 100)
	}
	if result.Summary.CPUOvercommit > 100 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Node Pod Density Distribution
type PodDensResult2090 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodDensSummary2090 `json:"summary"`
	Nodes           []PodDensEntry2090 `json:"nodes"`
	Recommendations []string           `json:"recommendations"`
}

type PodDensSummary2090 struct {
	TotalNodes int `json:"totalNodes"`
	TotalPods  int `json:"totalPods"`
	AvgPerNode int `json:"avgPerNode"`
}

type PodDensEntry2090 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

func (s *Server) handlePodDens2090(w http.ResponseWriter, r *http.Request) {
	result := PodDensResult2090{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}

	for _, node := range nodeList.Items {
		cnt := podsPerNode[node.Name]
		result.Nodes = append(result.Nodes, PodDensEntry2090{Node: node.Name, PodCount: cnt})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].PodCount > result.Nodes[j].PodCount })
	writeJSON(w, result)
}
