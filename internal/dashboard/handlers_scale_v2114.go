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
// v21.14 — Scalability & HA Dimension (Round 38)
// 1. CPU Burst Headroom — limit vs allocatable burst capacity
// 2. Pod Node Spread Evenness — Gini coefficient of pod distribution
// 3. Namespace Resource Footprint — CPU+Mem footprint per NS
// ============================================================

type BurstHRResult2114 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         BurstHRSummary2114 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type BurstHRSummary2114 struct {
	AllocatableCPU float64 `json:"allocatableCPU"`
	LimitedCPU     float64 `json:"limitedCPU"`
	BurstHeadroom  float64 `json:"burstHeadroomCPU"`
}

func (s *Server) handleBurstHR2114(w http.ResponseWriter, r *http.Request) {
	result := BurstHRResult2114{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.LimitedCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.BurstHeadroom = result.Summary.AllocatableCPU - result.Summary.LimitedCPU
	if result.Summary.BurstHeadroom < 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.BurstHeadroom < 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("CPU limits exceed allocatable by %.1f cores — overcommitted", -result.Summary.BurstHeadroom))
	}
	writeJSON(w, result)
}

// 2. Pod Spread Evenness
type SpreadEvenResult2114 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SpreadEvenSummary2114 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type SpreadEvenSummary2114 struct {
	TotalNodes int `json:"totalNodes"`
	TotalPods  int `json:"totalPods"`
	MaxPerNode int `json:"maxPerNode"`
	MinPerNode int `json:"minPerNode"`
}

func (s *Server) handleSpreadEven2114(w http.ResponseWriter, r *http.Request) {
	result := SpreadEvenResult2114{ScannedAt: time.Now()}
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
	maxP, minP := 0, 999999
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

// 3. NS Resource Footprint
type NSFootprintResult2114 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         NSFootprintSummary2114 `json:"summary"`
	TopNS           []NSFootprintEntry2114 `json:"topNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

type NSFootprintSummary2114 struct {
	TotalNS int `json:"totalNamespaces"`
}

type NSFootprintEntry2114 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequest"`
	MemReq    float64 `json:"memRequestGB"`
}

func (s *Server) handleNSFootprint2114(w http.ResponseWriter, r *http.Request) {
	result := NSFootprintResult2114{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsFoot := make(map[string]*NSFootprintEntry2114)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nsFoot[pod.Namespace] == nil {
			nsFoot[pod.Namespace] = &NSFootprintEntry2114{Namespace: pod.Namespace}
		}
		for _, c := range pod.Spec.Containers {
			nsFoot[pod.Namespace].CPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			nsFoot[pod.Namespace].MemReq += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsFoot)
	for _, entry := range nsFoot {
		result.TopNS = append(result.TopNS, *entry)
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
