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
// v21.50 — Scalability & HA Dimension (Round 44)
// 1. Node Memory Pressure Forecast
// 2. Pod Anti-Affinity Topology Key Coverage
// 3. Namespace CPU Quota Utilization
// ============================================================

type MemForecastResult2150 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         MemForecastSummary2150 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type MemForecastSummary2150 struct {
	AllocatableMemGB float64 `json:"allocatableMemGB"`
	RequestedMemGB   float64 `json:"requestedMemGB"`
	ForecastPct      int     `json:"forecastPct"`
}

func (s *Server) handleMemForecast2150(w http.ResponseWriter, r *http.Request) {
	result := MemForecastResult2150{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableMemGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.RequestedMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.AllocatableMemGB > 0 {
		result.Summary.ForecastPct = int(result.Summary.RequestedMemGB / result.Summary.AllocatableMemGB * 100)
	}
	if result.Summary.ForecastPct > 80 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.ForecastPct > 80 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Memory at %d%% capacity — add nodes", result.Summary.ForecastPct))
	}
	writeJSON(w, result)
}

// 2. Anti-Affinity Topology Key Coverage
type TopoKeyResult2150 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         TopoKeySummary2150 `json:"summary"`
	TopKeys         []TopoKeyEntry2150 `json:"topTopologyKeys"`
	Recommendations []string           `json:"recommendations"`
}

type TopoKeySummary2150 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithAntiAff  int `json:"withAntiAffinity"`
}

type TopoKeyEntry2150 struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func (s *Server) handleTopoKey2150(w http.ResponseWriter, r *http.Request) {
	result := TopoKeyResult2150{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	keyCount := make(map[string]int)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		aff := dep.Spec.Template.Spec.Affinity
		if aff == nil || aff.PodAntiAffinity == nil {
			continue
		}
		result.Summary.WithAntiAff++
		for _, term := range aff.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution {
			keyCount[term.PodAffinityTerm.TopologyKey]++
		}
		for _, term := range aff.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution {
			keyCount[term.TopologyKey]++
		}
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range keyCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for i, s2 := range sorted {
		if i >= 10 {
			break
		}
		result.TopKeys = append(result.TopKeys, TopoKeyEntry2150{Key: s2.key, Count: s2.count})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NS CPU Quota Utilization
type NSCPUQuotaResult2150 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NSCPUQuotaSummary2150 `json:"summary"`
	TopNS           []NSCPUQuotaEntry2150 `json:"topNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type NSCPUQuotaSummary2150 struct {
	TotalNS int `json:"totalNamespaces"`
}

type NSCPUQuotaEntry2150 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequest"`
}

func (s *Server) handleNSCPUQuota2150(w http.ResponseWriter, r *http.Request) {
	result := NSCPUQuotaResult2150{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns, cpu := range nsCPU {
		result.TopNS = append(result.TopNS, NSCPUQuotaEntry2150{Namespace: ns, CPUReq: cpu})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
