package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v23.97 Scalability: Top Namespace by CPU Limit, Node Allocatable CPU Summary, Cluster PVC Density
type TopNSCPULimitResult2397 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPULimit  float64 `json:"cpuLimit"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSCPULimit2397(w http.ResponseWriter, r *http.Request) {
	result := TopNSCPULimitResult2397{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCPU := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns, cpu := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPULimit  float64 `json:"cpuLimit"`
		}{ns, cpu})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPULimit > result.TopNS[j].CPULimit })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocCPUSumResult2397 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCPU   float64 `json:"totalAllocatableCPU"`
		AvgPerNode float64 `json:"avgPerNode"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocCPUSum2397(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocCPUSumResult2397{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalCPU / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCDensityResult2397 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByNS      map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handlePVCDensity2397(w http.ResponseWriter, r *http.Request) {
	result := PVCDensityResult2397{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.Summary.ByNS[pvc.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
