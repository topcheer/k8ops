package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.49 — Documentation Dimension (Round 44)
// 1. Node Capacity Allocatable Gap
// 2. PVC Volume Name Catalog
// 3. Pod Hostname Inventory
// ============================================================

type CapGapResult2149 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         CapGapSummary2149 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type CapGapSummary2149 struct {
	TotalNodes    int     `json:"totalNodes"`
	TotalCapCPU   float64 `json:"totalCapacityCPU"`
	TotalAllocCPU float64 `json:"totalAllocatableCPU"`
	GapPct        int     `json:"gapPct"`
}

func (s *Server) handleCapGap2149(w http.ResponseWriter, r *http.Request) {
	result := CapGapResult2149{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalAllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	if result.Summary.TotalCapCPU > 0 {
		result.Summary.GapPct = int((1 - result.Summary.TotalAllocCPU/result.Summary.TotalCapCPU) * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Volume Name Catalog
type PVCVolNameResult2149 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PVCVolNameSummary2149 `json:"summary"`
	TopVolumes      []PVCVolNameEntry2149 `json:"topVolumes"`
	Recommendations []string              `json:"recommendations"`
}

type PVCVolNameSummary2149 struct {
	TotalPVCs int `json:"totalPVCs"`
}

type PVCVolNameEntry2149 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	VolName   string `json:"volumeName"`
}

func (s *Server) handlePVCVolName2149(w http.ResponseWriter, r *http.Request) {
	result := PVCVolNameResult2149{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.TopVolumes = append(result.TopVolumes, PVCVolNameEntry2149{Name: pvc.Name, Namespace: pvc.Namespace, VolName: pvc.Spec.VolumeName})
	}
	sort.Slice(result.TopVolumes, func(i, j int) bool { return result.TopVolumes[i].Namespace < result.TopVolumes[j].Namespace })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Hostname Inventory
type PodHostResult2149 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodHostSummary2149 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type PodHostSummary2149 struct {
	TotalPods    int `json:"totalPods"`
	WithHostname int `json:"withHostname"`
}

func (s *Server) handlePodHost2149(w http.ResponseWriter, r *http.Request) {
	result := PodHostResult2149{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Hostname != "" {
			result.Summary.WithHostname++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
