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
// v21.20 — Scalability & HA Dimension (Round 39)
// 1. Memory Limit vs Allocatable Ratio
// 2. Pod Distribution by Namespace Quartile
// 3. Node Failure Blast Radius
// ============================================================

type MemLimitAllocResult2120 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         MemLimitAllocSummary2120 `json:"summary"`
	Recommendations []string                 `json:"recommendations"`
}

type MemLimitAllocSummary2120 struct {
	AllocatableMem float64 `json:"allocatableMemGB"`
	LimitedMem     float64 `json:"limitedMemGB"`
	RatioPct       int     `json:"ratioPct"`
}

func (s *Server) handleMemLimitAlloc2120(w http.ResponseWriter, r *http.Request) {
	result := MemLimitAllocResult2120{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableMem += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.LimitedMem += c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.AllocatableMem > 0 {
		result.Summary.RatioPct = int(result.Summary.LimitedMem / result.Summary.AllocatableMem * 100)
	}
	if result.Summary.RatioPct > 100 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Distribution by NS Quartile
type NSQuartileResult2120 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NSQuartileSummary2120 `json:"summary"`
	TopNS           []NSQuartileEntry2120 `json:"topNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type NSQuartileSummary2120 struct {
	TotalNS int `json:"totalNamespaces"`
}

type NSQuartileEntry2120 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleNSQuartile2120(w http.ResponseWriter, r *http.Request) {
	result := NSQuartileResult2120{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podsPerNS[pod.Namespace]++
		}
	}
	result.Summary.TotalNS = len(nsList.Items)
	for ns, cnt := range podsPerNS {
		result.TopNS = append(result.TopNS, NSQuartileEntry2120{Namespace: ns, PodCount: cnt})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PodCount > result.TopNS[j].PodCount })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Node Failure Blast Radius
type BlastRadiusResult2120 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         BlastRadiusSummary2120 `json:"summary"`
	Nodes           []BlastRadiusEntry2120 `json:"nodes"`
	Recommendations []string               `json:"recommendations"`
}

type BlastRadiusSummary2120 struct {
	TotalNodes    int `json:"totalNodes"`
	MaxPodsOnNode int `json:"maxPodsOnSingleNode"`
}

type BlastRadiusEntry2120 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

func (s *Server) handleBlastRadius2120(w http.ResponseWriter, r *http.Request) {
	result := BlastRadiusResult2120{ScannedAt: time.Now()}
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
	for _, node := range nodeList.Items {
		cnt := podsPerNode[node.Name]
		if cnt > result.Summary.MaxPodsOnNode {
			result.Summary.MaxPodsOnNode = cnt
		}
		result.Nodes = append(result.Nodes, BlastRadiusEntry2120{Node: node.Name, PodCount: cnt})
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].PodCount > result.Nodes[j].PodCount })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.MaxPodsOnNode > 50 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Max %d pods on single node — high blast radius", result.Summary.MaxPodsOnNode))
	}
	writeJSON(w, result)
}
