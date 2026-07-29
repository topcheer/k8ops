package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.61 — Product Dimension (Round 30)
// 1. Image Layer Cache Hit — repeated image pull efficiency
// 2. Endpoint Health Distribution — endpoint ready addresses spread
// 3. Namespace Cost Allocation — cost distribution per namespace
// ============================================================

// ---------------------------------------------------------------
// 1. Image Layer Cache Hit
// ---------------------------------------------------------------

type ImgCacheResult2061 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ImgCacheSummary2061 `json:"summary"`
	DupImages       []ImgCacheEntry2061 `json:"duplicateImages"`
	Recommendations []string            `json:"recommendations"`
}

type ImgCacheSummary2061 struct {
	TotalContainers int `json:"totalContainers"`
	UniqueImages    int `json:"uniqueImages"`
	DuplicateImages int `json:"duplicateImages"`
}

type ImgCacheEntry2061 struct {
	Image    string `json:"image"`
	UseCount int    `json:"useCount"`
}

func (s *Server) handleImgCacheHit2061(w http.ResponseWriter, r *http.Request) {
	result := ImgCacheResult2061{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imgCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			imgCount[c.Image]++
		}
	}

	result.Summary.UniqueImages = len(imgCount)
	for img, count := range imgCount {
		if count > 1 {
			result.Summary.DuplicateImages++
			result.DupImages = append(result.DupImages, ImgCacheEntry2061{
				Image: img, UseCount: count,
			})
		}
	}

	// High dedup is good (efficient layer caching)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.DupImages, func(i, j int) bool {
		return result.DupImages[i].UseCount > result.DupImages[j].UseCount
	})

	if len(result.DupImages) > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d images used by multiple pods — good cache locality", len(result.DupImages)))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Endpoint Health Distribution
// ---------------------------------------------------------------

type EPHealthResult2061 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EPHealthSummary2061 `json:"summary"`
	UnhealthyEPs    []EPHealthEntry2061 `json:"unhealthyEndpoints"`
	Recommendations []string            `json:"recommendations"`
}

type EPHealthSummary2061 struct {
	TotalEPs     int `json:"totalEndpoints"`
	HealthyEPs   int `json:"healthyEndpoints"`
	UnhealthyEPs int `json:"unhealthyEndpoints"`
}

type EPHealthEntry2061 struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	Addresses int    `json:"addresses"`
}

func (s *Server) handleEPHealthDist(w http.ResponseWriter, r *http.Request) {
	result := EPHealthResult2061{ScannedAt: time.Now()}
	score := 100

	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})

	for _, ep := range epList.Items {
		result.Summary.TotalEPs++

		addrCount := 0
		for _, sub := range ep.Subsets {
			addrCount += len(sub.Addresses)
		}

		if addrCount > 0 {
			result.Summary.HealthyEPs++
		} else {
			result.Summary.UnhealthyEPs++
			result.UnhealthyEPs = append(result.UnhealthyEPs, EPHealthEntry2061{
				Service: ep.Name, Namespace: ep.Namespace, Addresses: 0,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.UnhealthyEPs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d services have no healthy endpoints", result.Summary.UnhealthyEPs))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Cost Allocation
// ---------------------------------------------------------------

type NSCostResult2061 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         NSCostSummary2061 `json:"summary"`
	TopCostNS       []NSCostEntry2061 `json:"topCostNamespaces"`
	Recommendations []string          `json:"recommendations"`
}

type NSCostSummary2061 struct {
	TotalNS      int     `json:"totalNamespaces"`
	TotalCost    float64 `json:"totalMonthlyCost"`
	AvgCostPerNS float64 `json:"avgCostPerNS"`
}

type NSCostEntry2061 struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	MonthlyCost float64 `json:"monthlyCost"`
}

func (s *Server) handleNSCostAlloc(w http.ResponseWriter, r *http.Request) {
	result := NSCostResult2061{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsCost := make(map[string]*NSCostEntry2061)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nsCost[pod.Namespace] == nil {
			nsCost[pod.Namespace] = &NSCostEntry2061{Namespace: pod.Namespace}
		}
		nsCost[pod.Namespace].PodCount++

		for _, c := range pod.Spec.Containers {
			cpu := c.Resources.Requests.Cpu().AsApproximateFloat64()
			mem := c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			nsCost[pod.Namespace].MonthlyCost += cpu*25 + mem*3
		}
	}

	result.Summary.TotalNS = len(nsCost)
	for _, entry := range nsCost {
		result.Summary.TotalCost += entry.MonthlyCost
		result.TopCostNS = append(result.TopCostNS, *entry)
	}
	if result.Summary.TotalNS > 0 {
		result.Summary.AvgCostPerNS = result.Summary.TotalCost / float64(result.Summary.TotalNS)
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.TopCostNS, func(i, j int) bool {
		return result.TopCostNS[i].MonthlyCost > result.TopCostNS[j].MonthlyCost
	})

	if len(result.TopCostNS) > 10 {
		result.Recommendations = append(result.Recommendations,
			"High namespace count — review cost allocation and unused namespaces")
	}

	writeJSON(w, result)
}

// keep import
var _ = strings.Contains
