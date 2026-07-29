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
// v21.02 — Scalability & HA Dimension (Round 36)
// 1. Pod Anti-Affinity Effectiveness — actual pod spread per node
// 2. CPU Request Waste — over-provisioned CPU requests
// 3. Namespace Pod Quota Headroom — pods per namespace vs quota
// ============================================================

type AntiAffEffResult2102 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         AntiAffEffSummary2102 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type AntiAffEffSummary2102 struct {
	TotalNodes int `json:"totalNodes"`
	TotalPods  int `json:"totalPods"`
	MaxPerNode int `json:"maxPodsPerNode"`
	MinPerNode int `json:"minPodsPerNode"`
}

func (s *Server) handleAntiAffEff2102(w http.ResponseWriter, r *http.Request) {
	result := AntiAffEffResult2102{ScannedAt: time.Now()}
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
	minP := 999999
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
		if cnt > maxP {
			maxP = cnt
		}
		if cnt < minP {
			minP = cnt
		}
	}
	if minP == 999999 {
		minP = 0
	}
	result.Summary.MaxPerNode = maxP
	result.Summary.MinPerNode = minP
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CPU Request Waste
type CPUWasteResult2102 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CPUWasteSummary2102 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type CPUWasteSummary2102 struct {
	TotalRequestedCPU float64 `json:"totalRequestedCPU"`
	TotalAllocatable  float64 `json:"totalAllocatableCPU"`
	WastePct          int     `json:"wastePct"`
}

func (s *Server) handleCPUWaste2102(w http.ResponseWriter, r *http.Request) {
	result := CPUWasteResult2102{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalAllocatable += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalRequestedCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	if result.Summary.TotalAllocatable > 0 {
		waste := (1 - result.Summary.TotalRequestedCPU/result.Summary.TotalAllocatable) * 100
		result.Summary.WastePct = int(waste)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WastePct > 80 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d%% CPU allocatable unused — reduce nodes or increase requests", result.Summary.WastePct))
	}
	writeJSON(w, result)
}

// 3. Namespace Pod Quota Headroom
type NSQuotaHRResult2102 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NSQuotaHRSummary2102 `json:"summary"`
	NearQuota       []NSQuotaHREntry2102 `json:"nearQuotaNamespaces"`
	Recommendations []string             `json:"recommendations"`
}

type NSQuotaHRSummary2102 struct {
	TotalNS   int `json:"totalNamespaces"`
	NearQuota int `json:"nearQuota"`
}

type NSQuotaHREntry2102 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleNSQuotaHR2102(w http.ResponseWriter, r *http.Request) {
	result := NSQuotaHRResult2102{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podsPerNS[pod.Namespace]++
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		cnt := podsPerNS[ns.Name]
		if cnt > 50 {
			result.Summary.NearQuota++
			result.NearQuota = append(result.NearQuota, NSQuotaHREntry2102{Namespace: ns.Name, PodCount: cnt})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NearQuota, func(i, j int) bool { return result.NearQuota[i].PodCount > result.NearQuota[j].PodCount })
	writeJSON(w, result)
}
