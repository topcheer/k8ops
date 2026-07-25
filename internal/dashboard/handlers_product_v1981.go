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
// v19.81 — Product Dimension (Round 16)
// 1. Pod Density Score — pods-per-node efficiency metric
// 2. Image Cache Efficiency — image reuse & deduplication analysis
// 3. Node Bin Packing — resource bin packing efficiency per node
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Density Score
// ---------------------------------------------------------------

type PodDensityResult1981 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         PodDensitySummary1981     `json:"summary"`
	PerNode         []PodDensityNodeEntry1981 `json:"perNode"`
	Recommendations []string                  `json:"recommendations"`
}

type PodDensitySummary1981 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	MaxPodsPerNode int     `json:"maxPodsPerNode"`
	DensityPct     float64 `json:"densityPct"`
}

type PodDensityNodeEntry1981 struct {
	Name     string  `json:"name"`
	PodCount int     `json:"podCount"`
	CPUAlloc float64 `json:"cpuAllocatable"`
	Density  float64 `json:"densityPct"`
}

func (s *Server) handlePodDensityScore(w http.ResponseWriter, r *http.Request) {
	result := PodDensityResult1981{ScannedAt: time.Now()}
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
	result.Summary.TotalPods = len(podList.Items)

	maxPods := 0
	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]
		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()

		// Default pod limit per node is 110
		density := float64(podCount) / 110 * 100

		entry := PodDensityNodeEntry1981{
			Name: node.Name, PodCount: podCount,
			CPUAlloc: cpuAlloc, Density: density,
		}
		result.PerNode = append(result.PerNode, entry)

		if podCount > maxPods {
			maxPods = podCount
		}
	}

	result.Summary.MaxPodsPerNode = maxPods
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPodsPerNode = float64(result.Summary.TotalPods) / float64(result.Summary.TotalNodes)
		result.Summary.DensityPct = result.Summary.AvgPodsPerNode / 110 * 100
	}

	sort.Slice(result.PerNode, func(i, j int) bool {
		return result.PerNode[i].Density > result.PerNode[j].Density
	})

	if result.Summary.DensityPct > 80 {
		score -= 10
	} else if result.Summary.DensityPct > 60 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods / %d nodes, avg %.1f pods/node (%.1f%% density), max %d", result.Summary.TotalPods, result.Summary.TotalNodes, result.Summary.AvgPodsPerNode, result.Summary.DensityPct, result.Summary.MaxPodsPerNode))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Cache Efficiency
// ---------------------------------------------------------------

type ImageCacheResult1981 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         ImageCacheSummary1981     `json:"summary"`
	PerNode         []ImageCacheNodeEntry1981 `json:"perNode"`
	TopImages       []ImageCacheEntry1981     `json:"topImages"`
	Recommendations []string                  `json:"recommendations"`
}

type ImageCacheSummary1981 struct {
	TotalImages    int     `json:"totalUniqueImages"`
	TotalImageRefs int     `json:"totalImageRefs"`
	ReuseRatio     float64 `json:"reuseRatio"`
	CacheHitEst    float64 `json:"estCacheHitPct"`
}

type ImageCacheNodeEntry1981 struct {
	Name       string `json:"name"`
	ImageCount int    `json:"imagesOnNode"`
}

type ImageCacheEntry1981 struct {
	Image    string `json:"image"`
	UseCount int    `json:"useCount"`
}

func (s *Server) handleImageCacheEff(w http.ResponseWriter, r *http.Request) {
	result := ImageCacheResult1981{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imageMap := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			imageMap[c.Image]++
			result.Summary.TotalImageRefs++
		}
	}
	result.Summary.TotalImages = len(imageMap)

	// Reuse ratio = refs / unique images
	if result.Summary.TotalImages > 0 {
		result.Summary.ReuseRatio = float64(result.Summary.TotalImageRefs) / float64(result.Summary.TotalImages)
	}

	// Cache hit estimate: higher reuse = higher cache hit
	result.Summary.CacheHitEst = result.Summary.ReuseRatio * 20 // rough estimate
	if result.Summary.CacheHitEst > 95 {
		result.Summary.CacheHitEst = 95
	}

	for img, count := range imageMap {
		result.TopImages = append(result.TopImages, ImageCacheEntry1981{Image: img, UseCount: count})
	}
	sort.Slice(result.TopImages, func(i, j int) bool {
		return result.TopImages[i].UseCount > result.TopImages[j].UseCount
	})
	if len(result.TopImages) > 15 {
		result.TopImages = result.TopImages[:15]
	}

	for _, node := range nodeList.Items {
		result.PerNode = append(result.PerNode, ImageCacheNodeEntry1981{
			Name: node.Name, ImageCount: len(node.Status.Images),
		})
	}

	if result.Summary.ReuseRatio < 1.5 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unique images, %d refs, reuse ratio %.1f, est cache hit %.0f%%", result.Summary.TotalImages, result.Summary.TotalImageRefs, result.Summary.ReuseRatio, result.Summary.CacheHitEst))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Bin Packing
// ---------------------------------------------------------------

type BinPackResult1981 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         BinPackSummary1981     `json:"summary"`
	PerNode         []BinPackNodeEntry1981 `json:"perNode"`
	Recommendations []string               `json:"recommendations"`
}

type BinPackSummary1981 struct {
	TotalNodes    int     `json:"totalNodes"`
	AvgBinPackPct float64 `json:"avgBinPackPct"`
	BestNode      string  `json:"bestNode"`
	BestPct       float64 `json:"bestBinPackPct"`
	WorstNode     string  `json:"worstNode"`
	WorstPct      float64 `json:"worstBinPackPct"`
}

type BinPackNodeEntry1981 struct {
	Name       string  `json:"name"`
	CPUPackPct float64 `json:"cpuBinPackPct"`
	MemPackPct float64 `json:"memBinPackPct"`
	OverallPct float64 `json:"overallBinPackPct"`
}

func (s *Server) handleNodeBinPacking(w http.ResponseWriter, r *http.Request) {
	result := BinPackResult1981{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build per-node resource requests
	nodeReq := make(map[string]struct{ cpu, mem float64 })
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		req, ok := nodeReq[pod.Spec.NodeName]
		if !ok {
			req = struct{ cpu, mem float64 }{}
		}
		for _, c := range pod.Spec.Containers {
			req.cpu += c.Resources.Requests.Cpu().AsApproximateFloat64()
			req.mem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
		nodeReq[pod.Spec.NodeName] = req
	}

	var totalPack float64
	bestNode, worstNode := "", ""
	bestPct, worstPct := -1.0, 999.0

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocMem := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)

		req := nodeReq[node.Name]
		cpuPct := 0.0
		memPct := 0.0
		if allocCPU > 0 {
			cpuPct = req.cpu / allocCPU * 100
		}
		if allocMem > 0 {
			memPct = req.mem / allocMem * 100
		}
		overall := (cpuPct + memPct) / 2

		entry := BinPackNodeEntry1981{
			Name: node.Name, CPUPackPct: cpuPct,
			MemPackPct: memPct, OverallPct: overall,
		}
		result.PerNode = append(result.PerNode, entry)

		totalPack += overall
		if overall > bestPct {
			bestPct = overall
			bestNode = node.Name
		}
		if overall < worstPct {
			worstPct = overall
			worstNode = node.Name
		}
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgBinPackPct = totalPack / float64(result.Summary.TotalNodes)
	}
	result.Summary.BestNode = bestNode
	result.Summary.BestPct = bestPct
	result.Summary.WorstNode = worstNode
	result.Summary.WorstPct = worstPct

	sort.Slice(result.PerNode, func(i, j int) bool {
		return result.PerNode[i].OverallPct > result.PerNode[j].OverallPct
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Bin packing: avg %.1f%%, best %s (%.1f%%), worst %s (%.1f%%)", result.Summary.AvgBinPackPct, bestNode, bestPct, worstNode, worstPct))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
