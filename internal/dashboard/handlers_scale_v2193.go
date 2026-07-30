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
// v21.93 — Scalability & HA Dimension (Round 51)
// 1. Node Memory Limit Headroom
// 2. Namespace Resource Density Score
// 3. Cluster Scheduling Bin Pack Efficiency
// ============================================================

type MemLimHRResult2193 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
		TotalLimGB   float64 `json:"totalLimitedGB"`
		HeadroomGB   float64 `json:"headroomGB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMemLimHR2193(w http.ResponseWriter, r *http.Request) {
	result := MemLimHRResult2193{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLimGB += c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.HeadroomGB = result.Summary.TotalAllocGB - result.Summary.TotalLimGB
	if result.Summary.HeadroomGB < 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if result.Summary.HeadroomGB < 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Memory limits exceed allocatable by %.1fGB", -result.Summary.HeadroomGB))
	}
	writeJSON(w, result)
}

// 2. NS Resource Density Score
type NSDensityResult2193 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuRequest"`
		MemReqGB  float64 `json:"memRequestGB"`
		PodCount  int     `json:"podCount"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSDensity2193(w http.ResponseWriter, r *http.Request) {
	result := NSDensityData{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsData := make(map[string]*struct {
		Namespace string
		CPUReq    float64
		MemReqGB  float64
		PodCount  int
	})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nsData[pod.Namespace] == nil {
			nsData[pod.Namespace] = &struct {
				Namespace string
				CPUReq    float64
				MemReqGB  float64
				PodCount  int
			}{pod.Namespace, 0, 0, 0}
		}
		nsData[pod.Namespace].PodCount++
		for _, c := range pod.Spec.Containers {
			nsData[pod.Namespace].CPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			nsData[pod.Namespace].MemReqGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsData)
	for _, d := range nsData {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuRequest"`
			MemReqGB  float64 `json:"memRequestGB"`
			PodCount  int     `json:"podCount"`
		}{d.Namespace, d.CPUReq, d.MemReqGB, d.PodCount})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// NSDensityData is the response type (aliased)
type NSDensityData = NSDensityResult2193

// 3. Bin Pack Efficiency
type BinPackEffResult2193 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		TotalPods     int `json:"totalPods"`
		EfficiencyPct int `json:"efficiencyPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleBinPackEff2193(w http.ResponseWriter, r *http.Request) {
	result := BinPackEffResult2193{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodeList.Items)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	var totalCap int
	for _, node := range nodeList.Items {
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			totalCap += int(pods.AsApproximateFloat64())
		}
	}
	if totalCap > 0 {
		result.Summary.EfficiencyPct = result.Summary.TotalPods * 100 / totalCap
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
