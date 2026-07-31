package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.71 Scalability: Node Allocatable vs Capacity, StorageClass Usage, PVC Size Quartile
type NodeAllocVsCapResult2271 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes      int     `json:"totalNodes"`
		TotalCapCPU     float64 `json:"totalCapacityCPU"`
		TotalAllocCPU   float64 `json:"totalAllocatableCPU"`
		TotalCapMemGB   float64 `json:"totalCapacityMemGB"`
		TotalAllocMemGB float64 `json:"totalAllocatableMemGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocVsCap2271(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocVsCapResult2271{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.TotalCapMemGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocMemGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SCUsageResult2271 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs      int            `json:"totalPVCs"`
		ByStorageClass map[string]int `json:"byStorageClass"`
	} `json:"summary"`
}

func (s *Server) handleSCUsage2271(w http.ResponseWriter, r *http.Request) {
	result := SCUsageResult2271{ScannedAt: time.Now()}
	result.Summary.ByStorageClass = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		result.Summary.ByStorageClass[sc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCQuartileResult2271 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs  int            `json:"totalPVCs"`
		ByQuartile map[string]int `json:"byQuartile"`
	} `json:"summary"`
}

func (s *Server) handlePVCQuartile2271(w http.ResponseWriter, r *http.Request) {
	result := PVCQuartileResult2271{ScannedAt: time.Now()}
	result.Summary.ByQuartile = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sizeGB := pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
		var q string
		switch {
		case sizeGB < 5:
			q = "Q1(<5GB)"
		case sizeGB < 20:
			q = "Q2(5-20GB)"
		case sizeGB < 100:
			q = "Q3(20-100GB)"
		default:
			q = "Q4(100GB+)"
		}
		result.Summary.ByQuartile[q]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
