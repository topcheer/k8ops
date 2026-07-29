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
// v20.29 — Product Dimension (Round 24)
// 1. Cost Anomaly Detector — workload cost spike detection
// 2. Pod Right-Size Recommender — resource request vs actual usage
// 3. Image Layer Dedup Report — container image sharing analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Cost Anomaly Detector
// ---------------------------------------------------------------

type CostAnomalyResult2029 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CostAnomalySummary2029 `json:"summary"`
	Anomalies       []CostAnomalyEntry2029 `json:"anomalies"`
	Recommendations []string               `json:"recommendations"`
}

type CostAnomalySummary2029 struct {
	TotalWorkloads int     `json:"totalWorkloads"`
	HighCost       int     `json:"highCost"`
	AvgCPUCost     float64 `json:"avgCPUCostPerMonth"`
	AvgMemCost     float64 `json:"avgMemCostPerMonth"`
}

type CostAnomalyEntry2029 struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	MonthlyCost float64 `json:"monthlyCost"`
	Reason      string  `json:"reason"`
}

func (s *Server) handleCostAnomalyDet(w http.ResponseWriter, r *http.Request) {
	result := CostAnomalyResult2029{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Aggregate resource requests per namespace
	type nsCost struct {
		cpuReq, memReq float64
		podCount       int
	}
	nsCosts := make(map[string]*nsCost)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nsCosts[pod.Namespace] == nil {
			nsCosts[pod.Namespace] = &nsCost{}
		}
		nsCosts[pod.Namespace].podCount++
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				nsCosts[pod.Namespace].cpuReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			}
			if !c.Resources.Requests.Memory().IsZero() {
				nsCosts[pod.Namespace].memReq += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9 // GB
			}
		}
	}

	// Cost model: $25/core/month, $3/GB/month
	const cpuPrice = 25.0
	const memPrice = 3.0

	var totalCPUCost, totalMemCost float64
	var nsCount int

	for ns, nc := range nsCosts {
		cpuCost := nc.cpuReq * cpuPrice
		memCost := nc.memReq * memPrice
		totalCPUCost += cpuCost
		totalMemCost += memCost
		nsCount++
		monthlyCost := cpuCost + memCost

		result.Summary.TotalWorkloads += nc.podCount
		if monthlyCost > 500 {
			result.Summary.HighCost++
			reason := fmt.Sprintf("%.0f CPU cores + %.1f GB memory", nc.cpuReq, nc.memReq)
			result.Anomalies = append(result.Anomalies, CostAnomalyEntry2029{
				Name: ns, Namespace: ns,
				MonthlyCost: monthlyCost, Reason: reason,
			})
			if monthlyCost > 1000 {
				score -= 5
			} else {
				score -= 2
			}
		}
	}

	if nsCount > 0 {
		result.Summary.AvgCPUCost = totalCPUCost / float64(nsCount)
		result.Summary.AvgMemCost = totalMemCost / float64(nsCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Anomalies, func(i, j int) bool {
		return result.Anomalies[i].MonthlyCost > result.Anomalies[j].MonthlyCost
	})

	if result.Summary.HighCost > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces exceed $500/month — review resource requests for right-sizing", result.Summary.HighCost))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Right-Size Recommender
// ---------------------------------------------------------------

type RightSizeResult2029 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         RightSizeSummary2029 `json:"summary"`
	OverProvisioned []RightSizeEntry2029 `json:"overProvisioned"`
	Recommendations []string             `json:"recommendations"`
}

type RightSizeSummary2029 struct {
	TotalContainers int     `json:"totalContainers"`
	OverProvisioned int     `json:"overProvisioned"`
	NoLimits        int     `json:"noLimits"`
	AvgCPURequest   float64 `json:"avgCPURequestCores"`
	AvgMemRequest   float64 `json:"avgMemRequestMB"`
}

type RightSizeEntry2029 struct {
	Pod        string  `json:"pod"`
	Namespace  string  `json:"namespace"`
	Container  string  `json:"container"`
	CPURequest float64 `json:"cpuRequestCores"`
	MemRequest float64 `json:"memRequestMB"`
}

func (s *Server) handleRightSizeRecommender(w http.ResponseWriter, r *http.Request) {
	result := RightSizeResult2029{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalCPUReq, totalMemReq float64
	var reqCount int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
				result.Summary.NoLimits++
				continue
			}

			cpuReq := c.Resources.Requests.Cpu().AsApproximateFloat64()
			memReqMB := c.Resources.Requests.Memory().AsApproximateFloat64() / 1e6

			totalCPUReq += cpuReq
			totalMemReq += memReqMB
			reqCount++

			// Flag over-provisioned: CPU > 2 cores or Mem > 4GB
			if cpuReq > 2.0 || memReqMB > 4096 {
				result.Summary.OverProvisioned++
				result.OverProvisioned = append(result.OverProvisioned, RightSizeEntry2029{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					CPURequest: cpuReq, MemRequest: memReqMB,
				})
				score -= 1
			}
		}
	}

	if reqCount > 0 {
		result.Summary.AvgCPURequest = totalCPUReq / float64(reqCount)
		result.Summary.AvgMemRequest = totalMemReq / float64(reqCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OverProvisioned, func(i, j int) bool {
		return result.OverProvisioned[i].CPURequest > result.OverProvisioned[j].CPURequest
	})

	if result.Summary.OverProvisioned > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers over-provisioned — use VPA or metrics to right-size", result.Summary.OverProvisioned))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Image Layer Dedup Report
// ---------------------------------------------------------------

type ImageDedupResult2029 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ImageDedupSummary2029 `json:"summary"`
	SharedImages    []ImageDedupEntry2029 `json:"sharedImages"`
	Recommendations []string              `json:"recommendations"`
}

type ImageDedupSummary2029 struct {
	TotalPods        int     `json:"totalPods"`
	UniqueImages     int     `json:"uniqueImages"`
	SharedImages     int     `json:"sharedImages"`
	DuplicationRatio float64 `json:"duplicationRatio"`
}

type ImageDedupEntry2029 struct {
	Image      string   `json:"image"`
	PodCount   int      `json:"podCount"`
	Namespaces []string `json:"namespaces"`
}

func (s *Server) handleImageDedupReport(w http.ResponseWriter, r *http.Request) {
	result := ImageDedupResult2029{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imageMap := make(map[string]*ImageDedupEntry2029)
	nsSet := make(map[string]map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			img := c.Image
			if imageMap[img] == nil {
				imageMap[img] = &ImageDedupEntry2029{Image: img, PodCount: 0}
				nsSet[img] = make(map[string]bool)
			}
			imageMap[img].PodCount++
			nsSet[img][pod.Namespace] = true
		}
	}

	result.Summary.UniqueImages = len(imageMap)
	for img, entry := range imageMap {
		for ns := range nsSet[img] {
			entry.Namespaces = append(entry.Namespaces, ns)
		}
		sort.Strings(entry.Namespaces)
		if entry.PodCount > 3 {
			result.Summary.SharedImages++
			result.SharedImages = append(result.SharedImages, *entry)
		}
	}

	if result.Summary.UniqueImages > 0 {
		result.Summary.DuplicationRatio = float64(result.Summary.SharedImages) / float64(result.Summary.UniqueImages)
	}

	// High dedup is good (efficient), not a risk
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.SharedImages, func(i, j int) bool {
		return result.SharedImages[i].PodCount > result.SharedImages[j].PodCount
	})

	if result.Summary.UniqueImages > 50 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unique images — consider consolidating base images to reduce storage", result.Summary.UniqueImages))
	}

	writeJSON(w, result)
}
