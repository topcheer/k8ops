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
// v21.69 — Scalability & HA Dimension (Round 47)
// 1. Node Resource Fragmentation Score
// 2. Namespace Workload Spread
// 3. Cluster Pod Density Health
// ============================================================

type FragScoreResult2169 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalFrag  float64 `json:"totalFragmentationScore"`
		AvgFragPct int     `json:"avgFragmentationPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleFragScore2169(w http.ResponseWriter, r *http.Request) {
	result := FragScoreResult2169{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	reqPerNode := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			reqPerNode[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}

	var totalFragPct int
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		req := reqPerNode[node.Name]
		if alloc > 0 {
			frag := int((1 - req/alloc) * 100)
			totalFragPct += frag
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgFragPct = totalFragPct / result.Summary.TotalNodes
	}
	if result.Summary.AvgFragPct > 70 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if result.Summary.AvgFragPct > 70 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Avg %d%% CPU fragmentation — consolidate workloads", result.Summary.AvgFragPct))
	}
	writeJSON(w, result)
}

// 2. NS Workload Spread
type NSWkSpreadResult2169 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuRequest"`
		PodCount  int     `json:"podCount"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSWkSpread2169(w http.ResponseWriter, r *http.Request) {
	result := NSWkSpreadResult2169{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsCPU := make(map[string]float64)
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPods[pod.Namespace]++
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuRequest"`
			PodCount  int     `json:"podCount"`
		}{ns, nsCPU[ns], nsPods[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Cluster Pod Density Health
type ClusterDensityResult2169 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalPods  int `json:"totalPods"`
		AvgPerNode int `json:"avgPodsPerNode"`
		MaxPerNode int `json:"maxPodsPerNode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleClusterDensity2169(w http.ResponseWriter, r *http.Request) {
	result := ClusterDensityResult2169{ScannedAt: time.Now()}
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
	maxP := 0
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
		if cnt > maxP {
			maxP = cnt
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}
	result.Summary.MaxPerNode = maxP
	if maxP > 100 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
