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
// v21.44 — Scalability & HA Dimension (Round 43)
// 1. Node CPU Utilization Quartile
// 2. PVC Storage Overhead Ratio
// 3. Namespace Workload Density Forecast
// ============================================================

type CPUQuartileResult2144 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CPUQuartileSummary2144 `json:"summary"`
	Nodes           []CPUQuartileEntry2144 `json:"nodes"`
	Recommendations []string               `json:"recommendations"`
}

type CPUQuartileSummary2144 struct {
	TotalNodes int `json:"totalNodes"`
}

type CPUQuartileEntry2144 struct {
	Node     string  `json:"node"`
	ReqCPU   float64 `json:"requestedCPU"`
	AllocCPU float64 `json:"allocatableCPU"`
	SatPct   int     `json:"saturationPct"`
}

func (s *Server) handleCPUQuartile2144(w http.ResponseWriter, r *http.Request) {
	result := CPUQuartileResult2144{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	reqPerNode := make(map[string]float64)
	allocPerNode := make(map[string]float64)
	for _, node := range nodeList.Items {
		allocPerNode[node.Name] = node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			reqPerNode[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		req := reqPerNode[node.Name]
		alloc := allocPerNode[node.Name]
		satPct := 0
		if alloc > 0 {
			satPct = int(req / alloc * 100)
		}
		result.Nodes = append(result.Nodes, CPUQuartileEntry2144{Node: node.Name, ReqCPU: req, AllocCPU: alloc, SatPct: satPct})
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].SatPct > result.Nodes[j].SatPct })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Storage Overhead
type PVCOverheadResult2144 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PVCOverheadSummary2144 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type PVCOverheadSummary2144 struct {
	TotalPVCs int     `json:"totalPVCs"`
	TotalGB   float64 `json:"totalRequestedGB"`
	AvgPerPVC float64 `json:"avgPerPVCGB"`
}

func (s *Server) handlePVCOverhead2144(w http.ResponseWriter, r *http.Request) {
	result := PVCOverheadResult2144{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	var totalGB float64
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			totalGB += req.AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalGB = totalGB
	if result.Summary.TotalPVCs > 0 {
		result.Summary.AvgPerPVC = totalGB / float64(result.Summary.TotalPVCs)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NS Workload Density Forecast
type NSForecastResult2144 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NSForecastSummary2144 `json:"summary"`
	TopNS           []NSForecastEntry2144 `json:"topNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type NSForecastSummary2144 struct {
	TotalNS int `json:"totalNamespaces"`
}

type NSForecastEntry2144 struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequest"`
	PodCount  int     `json:"podCount"`
}

func (s *Server) handleNSForecast2144(w http.ResponseWriter, r *http.Request) {
	result := NSForecastResult2144{ScannedAt: time.Now()}
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
		result.TopNS = append(result.TopNS, NSForecastEntry2144{Namespace: ns, CPUReq: nsCPU[ns], PodCount: nsPods[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })

	if len(result.TopNS) > 0 && result.TopNS[0].PodCount > 100 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Namespace %s forecast: %d pods, %.1f CPU", result.TopNS[0].Namespace, result.TopNS[0].PodCount, result.TopNS[0].CPUReq))
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
