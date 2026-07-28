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
// v20.24 — Scalability & HA Dimension (Round 23 Final)
// 1. Cluster Label Cardinality — label key cardinality for API scaling
// 2. ConfigMap Size Budget — large ConfigMap detection for etcd pressure
// 3. Endpoint Slice Address Budget — endpoint address count for mesh scaling
// ============================================================

// ---------------------------------------------------------------
// 1. Cluster Label Cardinality
// ---------------------------------------------------------------

type LabelCardResult2024 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         LabelCardSummary2024 `json:"summary"`
	TopLabels       []LabelCardEntry2024 `json:"topLabels"`
	Recommendations []string             `json:"recommendations"`
}

type LabelCardSummary2024 struct {
	TotalObjects    int `json:"totalObjects"`
	UniqueLabelKeys int `json:"uniqueLabelKeys"`
	HighCardinality int `json:"highCardinalityKeys"`
}

type LabelCardEntry2024 struct {
	LabelKey string `json:"labelKey"`
	Count    int    `json:"objectCount"`
}

func (s *Server) handleLabelCard(w http.ResponseWriter, r *http.Request) {
	result := LabelCardResult2024{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	labelCounts := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalObjects++
		for k := range pod.Labels {
			labelCounts[k]++
		}
	}

	result.Summary.UniqueLabelKeys = len(labelCounts)

	for k, c := range labelCounts {
		if c > 100 {
			result.Summary.HighCardinality++
			score -= 1
		}
		result.TopLabels = append(result.TopLabels, LabelCardEntry2024{LabelKey: k, Count: c})
	}
	sort.Slice(result.TopLabels, func(i, j int) bool {
		return result.TopLabels[i].Count > result.TopLabels[j].Count
	})
	if len(result.TopLabels) > 15 {
		result.TopLabels = result.TopLabels[:15]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d objects, %d unique label keys, %d high-cardinality", result.Summary.TotalObjects, result.Summary.UniqueLabelKeys, result.Summary.HighCardinality))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. ConfigMap Size Budget
// ---------------------------------------------------------------

type CMSizeResult2024 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         CMSizeSummary2024 `json:"summary"`
	LargeCMs        []CMSizeEntry2024 `json:"largeConfigMaps"`
	Recommendations []string          `json:"recommendations"`
}

type CMSizeSummary2024 struct {
	TotalCMs      int `json:"totalConfigMaps"`
	LargeCMs      int `json:"largeConfigMaps"`
	VeryLargeCMs  int `json:"veryLargeConfigMaps"`
	TotalDataKeys int `json:"totalDataKeys"`
}

type CMSizeEntry2024 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	DataKeys  int    `json:"dataKeyCount"`
	EstSizeKB int    `json:"estSizeKB"`
}

func (s *Server) handleCMSizeBudget(w http.ResponseWriter, r *http.Request) {
	result := CMSizeResult2024{ScannedAt: time.Now()}
	score := 100

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++

		keyCount := len(cm.Data) + len(cm.BinaryData)
		result.Summary.TotalDataKeys += keyCount

		// Estimate size: sum of string lengths
		estKB := 0
		for _, v := range cm.Data {
			estKB += len(v)
		}
		estKB = estKB / 1024

		if estKB > 100 || keyCount > 50 {
			result.Summary.LargeCMs++
			result.LargeCMs = append(result.LargeCMs, CMSizeEntry2024{
				Name: cm.Name, Namespace: cm.Namespace,
				DataKeys: keyCount, EstSizeKB: estKB,
			})
			if estKB > 500 {
				result.Summary.VeryLargeCMs++
				score -= 2
			}
		}
	}

	sort.Slice(result.LargeCMs, func(i, j int) bool {
		return result.LargeCMs[i].EstSizeKB > result.LargeCMs[j].EstSizeKB
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ConfigMaps: %d large, %d very-large, %d total data keys", result.Summary.TotalCMs, result.Summary.LargeCMs, result.Summary.VeryLargeCMs, result.Summary.TotalDataKeys))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Endpoint Slice Address Budget
// ---------------------------------------------------------------

type EPSAddrResult2024 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EPSAddrSummary2024 `json:"summary"`
	PerNS           []EPSAddrEntry2024 `json:"perNamespace"`
	Recommendations []string           `json:"recommendations"`
}

type EPSAddrSummary2024 struct {
	TotalSlices    int     `json:"totalSlices"`
	TotalAddresses int     `json:"totalAddresses"`
	AvgPerSlice    float64 `json:"avgAddressesPerSlice"`
	MaxPerSlice    int     `json:"maxAddressesPerSlice"`
}

type EPSAddrEntry2024 struct {
	Namespace    string `json:"namespace"`
	SliceCount   int    `json:"sliceCount"`
	AddressCount int    `json:"addressCount"`
}

func (s *Server) handleEPSAddrBudget(w http.ResponseWriter, r *http.Request) {
	result := EPSAddrResult2024{ScannedAt: time.Now()}
	score := 100

	epList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*EPSAddrEntry2024)
	maxAddr := 0

	for _, ep := range epList.Items {
		result.Summary.TotalSlices++

		addrCount := 0
		for _, endpoint := range ep.Endpoints {
			addrCount += len(endpoint.Addresses)
		}
		result.Summary.TotalAddresses += addrCount
		if addrCount > maxAddr {
			maxAddr = addrCount
		}

		entry, ok := nsStats[ep.Namespace]
		if !ok {
			entry = &EPSAddrEntry2024{Namespace: ep.Namespace}
			nsStats[ep.Namespace] = entry
		}
		entry.SliceCount++
		entry.AddressCount += addrCount
	}

	result.Summary.MaxPerSlice = maxAddr
	if result.Summary.TotalSlices > 0 {
		result.Summary.AvgPerSlice = float64(result.Summary.TotalAddresses) / float64(result.Summary.TotalSlices)
	}

	for _, e := range nsStats {
		result.PerNS = append(result.PerNS, *e)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].AddressCount > result.PerNS[j].AddressCount
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	if maxAddr > 100 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d slices, %d addresses, avg %.1f/slice, max %d", result.Summary.TotalSlices, result.Summary.TotalAddresses, result.Summary.AvgPerSlice, maxAddr))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
