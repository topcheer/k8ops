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
// v20.36 — Scalability & HA Dimension (Round 25)
// 1. Node Capacity Headroom — remaining allocatable vs requested
// 2. Pod Density Analyzer — pods per node vs kubelet maxPods
// 3. Storage Capacity Forecast — PVC growth vs node storage
// ============================================================

// ---------------------------------------------------------------
// 1. Node Capacity Headroom
// ---------------------------------------------------------------

type NodeHeadroomResult2036 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         NodeHeadroomSummary2036 `json:"summary"`
	TightNodes      []NodeHeadroomEntry2036 `json:"tightNodes"`
	Recommendations []string                `json:"recommendations"`
}

type NodeHeadroomSummary2036 struct {
	TotalNodes     int `json:"totalNodes"`
	LowHeadroom    int `json:"lowHeadroom"`
	AvgCPUHeadroom int `json:"avgCPUHeadroomPct"`
	AvgMemHeadroom int `json:"avgMemHeadroomPct"`
}

type NodeHeadroomEntry2036 struct {
	Node        string `json:"node"`
	CPUHeadroom int    `json:"cpuHeadroomPct"`
	MemHeadroom int    `json:"memHeadroomPct"`
}

func (s *Server) handleNodeCapacityHeadroom(w http.ResponseWriter, r *http.Request) {
	result := NodeHeadroomResult2036{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Calculate requested resources per node
	type nodeReq struct {
		cpu, mem float64
	}
	nodeReqs := make(map[string]*nodeReq)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nodeName := pod.Spec.NodeName
		if nodeReqs[nodeName] == nil {
			nodeReqs[nodeName] = &nodeReq{}
		}
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				nodeReqs[nodeName].cpu += c.Resources.Requests.Cpu().AsApproximateFloat64()
			}
			if !c.Resources.Requests.Memory().IsZero() {
				nodeReqs[nodeName].mem += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			}
		}
	}

	var totalCPUHR, totalMemHR int
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocMem := node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9

		nr := nodeReqs[node.Name]
		var cpuReq, memReq float64
		if nr != nil {
			cpuReq = nr.cpu
			memReq = nr.mem
		}

		var cpuHR, memHR int
		if allocCPU > 0 {
			cpuHR = int((1 - cpuReq/allocCPU) * 100)
		}
		if allocMem > 0 {
			memHR = int((1 - memReq/allocMem) * 100)
		}

		totalCPUHR += cpuHR
		totalMemHR += memHR

		if cpuHR < 20 || memHR < 20 {
			result.Summary.LowHeadroom++
			result.TightNodes = append(result.TightNodes, NodeHeadroomEntry2036{
				Node: node.Name, CPUHeadroom: cpuHR, MemHeadroom: memHR,
			})
			score -= 5
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUHeadroom = totalCPUHR / result.Summary.TotalNodes
		result.Summary.AvgMemHeadroom = totalMemHR / result.Summary.TotalNodes
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.TightNodes, func(i, j int) bool {
		return result.TightNodes[i].CPUHeadroom < result.TightNodes[j].CPUHeadroom
	})

	if result.Summary.LowHeadroom > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have <20%% headroom — add nodes or reduce requests", result.Summary.LowHeadroom))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Density Analyzer
// ---------------------------------------------------------------

type PodDensityResult2036 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PodDensitySummary2036 `json:"summary"`
	DenseNodes      []PodDensityEntry2036 `json:"denseNodes"`
	Recommendations []string              `json:"recommendations"`
}

type PodDensitySummary2036 struct {
	TotalNodes     int `json:"totalNodes"`
	DenseNodes     int `json:"denseNodes"`
	TotalPods      int `json:"totalPods"`
	AvgPodsPerNode int `json:"avgPodsPerNode"`
}

type PodDensityEntry2036 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
	MaxPods  int    `json:"maxPods"`
	Density  int    `json:"densityPct"`
}

func (s *Server) handlePodDensity2036(w http.ResponseWriter, r *http.Request) {
	result := PodDensityResult2036{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count pods per node
	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	result.Summary.TotalPods = len(podList.Items)

	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]

		// Get maxPods from allocatable pods
		maxPods := 110 // default
		if pods := node.Status.Allocatable.Pods(); pods != nil && !pods.IsZero() {
			maxPods = int(pods.AsApproximateFloat64())
		}

		density := 0
		if maxPods > 0 {
			density = podCount * 100 / maxPods
		}

		if density > 80 {
			result.Summary.DenseNodes++
			result.DenseNodes = append(result.DenseNodes, PodDensityEntry2036{
				Node: node.Name, PodCount: podCount, MaxPods: maxPods, Density: density,
			})
			score -= 5
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.DenseNodes, func(i, j int) bool {
		return result.DenseNodes[i].Density > result.DenseNodes[j].Density
	})

	if result.Summary.DenseNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have >80%% pod density — consider increasing maxPods or adding nodes", result.Summary.DenseNodes))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Storage Capacity Forecast
// ---------------------------------------------------------------

type StorageForecastResult2036 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         StorageForecastSummary2036 `json:"summary"`
	LargePVCs       []StorageForecastEntry2036 `json:"largePVCs"`
	Recommendations []string                   `json:"recommendations"`
}

type StorageForecastSummary2036 struct {
	TotalPVCs     int `json:"totalPVCs"`
	BoundPVCs     int `json:"boundPVCs"`
	LargePVCs     int `json:"largePVCs"`
	TotalCapacity int `json:"totalCapacityGB"`
}

type StorageForecastEntry2036 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Size      string `json:"size"`
	SizeGB    int    `json:"sizeGB"`
}

func (s *Server) handleStorageForecast2036(w http.ResponseWriter, r *http.Request) {
	result := StorageForecastResult2036{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	var totalCapGB int

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		}

		storage := pvc.Spec.Resources.Requests.Storage()
		if storage == nil {
			continue
		}

		sizeGB := int(storage.AsApproximateFloat64() / 1e9)
		totalCapGB += sizeGB

		if sizeGB > 100 {
			result.Summary.LargePVCs++
			result.LargePVCs = append(result.LargePVCs, StorageForecastEntry2036{
				Name: pvc.Name, Namespace: pvc.Namespace,
				Size: storage.String(), SizeGB: sizeGB,
			})
		}
	}

	result.Summary.TotalCapacity = totalCapGB
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.LargePVCs, func(i, j int) bool {
		return result.LargePVCs[i].SizeGB > result.LargePVCs[j].SizeGB
	})

	if result.Summary.LargePVCs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVCs >100GB — monitor storage growth and plan capacity", result.Summary.LargePVCs))
	}

	writeJSON(w, result)
}
