package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.19 — Documentation Dimension (Round 39)
// 1. Node Allocatable Map
// 2. PVC Phase Distribution
// 3. Pod Tolerations Catalog
// ============================================================

type NodeAllocResult2119 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeAllocSummary2119 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type NodeAllocSummary2119 struct {
	TotalNodes  int `json:"totalNodes"`
	TotalPodCap int `json:"totalPodCapacity"`
}

func (s *Server) handleNodeAlloc2119(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocResult2119{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			result.Summary.TotalPodCap += int(pods.AsApproximateFloat64())
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Phase Distribution
type PVCPhaseResult2119 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PVCPhaseSummary2119 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type PVCPhaseSummary2119 struct {
	TotalPVCs int            `json:"totalPVCs"`
	ByPhase   map[string]int `json:"byPhase"`
}

func (s *Server) handlePVCPhase2119(w http.ResponseWriter, r *http.Request) {
	result := PVCPhaseResult2119{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	byPhase := make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		byPhase[string(pvc.Status.Phase)]++
	}
	result.Summary.ByPhase = byPhase
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Tolerations Catalog
type TolCatalogResult2119 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         TolCatalogSummary2119 `json:"summary"`
	TopTolerations  []TolCatalogEntry2119 `json:"topTolerations"`
	Recommendations []string              `json:"recommendations"`
}

type TolCatalogSummary2119 struct {
	TotalPods int `json:"totalPods"`
	WithTol   int `json:"withTolerations"`
}

type TolCatalogEntry2119 struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func (s *Server) handleTolCatalog2119(w http.ResponseWriter, r *http.Request) {
	result := TolCatalogResult2119{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	tolCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.Tolerations) > 0 {
			result.Summary.WithTol++
		}
		for _, tol := range pod.Spec.Tolerations {
			tolCount[tol.Key]++
		}
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range tolCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for i, s2 := range sorted {
		if i >= 10 {
			break
		}
		result.TopTolerations = append(result.TopTolerations, TolCatalogEntry2119{Key: s2.key, Count: s2.count})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
