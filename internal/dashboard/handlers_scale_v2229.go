package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.29 — Scalability & HA Dimension (Round 57)
// 1. Namespace Memory Efficiency Ratio
// 2. Node Storage Allocatable Headroom
// 3. Cluster Image Cache Hit Ratio Estimate
// ============================================================

type NSMemEffResult2229 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		MemReqMB  float64 `json:"memReqMB"`
		PodCount  int     `json:"podCount"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSMemEff2229(w http.ResponseWriter, r *http.Request) {
	result := NSMemEffResult2229{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsMem := make(map[string]float64)
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPods[pod.Namespace]++
		for _, c := range pod.Spec.Containers {
			nsMem[pod.Namespace] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e6
		}
	}
	result.Summary.TotalNS = len(nsMem)
	for ns := range nsMem {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			MemReqMB  float64 `json:"memReqMB"`
			PodCount  int     `json:"podCount"`
		}{ns, nsMem[ns], nsPods[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReqMB > result.TopNS[j].MemReqMB })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Storage Allocatable Headroom
type NodeStorageHRResult2229 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes      int     `json:"totalNodes"`
		TotalCapEphGB   float64 `json:"totalCapacityEphemeralGB"`
		TotalAllocEphGB float64 `json:"totalAllocatableEphemeralGB"`
		HeadroomEphGB   float64 `json:"headroomEphemeralGB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeStorageHR2229(w http.ResponseWriter, r *http.Request) {
	result := NodeStorageHRResult2229{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapEphGB += node.Status.Capacity.StorageEphemeral().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocEphGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / 1e9
	}
	result.Summary.HeadroomEphGB = result.Summary.TotalCapEphGB - result.Summary.TotalAllocEphGB
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Image Cache Hit Ratio
type ImgCacheHitResult2229 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		TotalImageRefs int `json:"totalImageRefs"`
		UniqueImages   int `json:"uniqueImages"`
		CacheHitRatio  int `json:"cacheHitRatioPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgCacheHit2229(w http.ResponseWriter, r *http.Request) {
	result := ImgCacheHitResult2229{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImageRefs++
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.UniqueImages++
			}
		}
	}
	if result.Summary.TotalImageRefs > 0 {
		dupes := result.Summary.TotalImageRefs - result.Summary.UniqueImages
		result.Summary.CacheHitRatio = dupes * 100 / result.Summary.TotalImageRefs
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
