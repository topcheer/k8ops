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
// v19.93 — Product Dimension (Round 18)
// 1. CPU Throttle Estimator — CFS quota throttling risk from limit ratios
// 2. Image Layer Dedup — cross-pod image layer sharing potential
// 3. Pod Scheduling Latency — time from creation to running
// ============================================================

// ---------------------------------------------------------------
// 1. CPU Throttle Estimator
// ---------------------------------------------------------------

type CPUThrottleResult1993 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CPUThrottleSummary1993 `json:"summary"`
	AtRisk          []CPUThrottleEntry1993 `json:"atRiskContainers"`
	Recommendations []string               `json:"recommendations"`
}

type CPUThrottleSummary1993 struct {
	TotalContainers int     `json:"totalContainers"`
	WithCPULimit    int     `json:"withCPULimit"`
	AtRiskCount     int     `json:"atRiskCount"`
	AvgLimitCPU     float64 `json:"avgLimitCPU"`
	ThrottleRisk    string  `json:"throttleRiskLevel"`
}

type CPUThrottleEntry1993 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Container string  `json:"container"`
	LimitCPU  float64 `json:"limitCPU"`
	RiskLevel string  `json:"riskLevel"`
}

func (s *Server) handleCPUThrottleEst(w http.ResponseWriter, r *http.Request) {
	result := CPUThrottleResult1993{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalLimit float64
	var limitCount int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			limCPU := c.Resources.Limits.Cpu().AsApproximateFloat64()
			if limCPU > 0 {
				result.Summary.WithCPULimit++
				totalLimit += limCPU
				limitCount++

				// CPU throttling risk: limits < 1 core with multi-threaded workloads are high risk
				// Also, limit = 0.1 or less is very risky
				riskLevel := "low"
				if limCPU < 0.1 {
					riskLevel = "critical"
					result.Summary.AtRiskCount++
					score -= 5
					result.AtRisk = append(result.AtRisk, CPUThrottleEntry1993{
						Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
						LimitCPU: limCPU, RiskLevel: riskLevel,
					})
				} else if limCPU < 0.5 {
					riskLevel = "medium"
				} else if limCPU > 8 {
					riskLevel = "medium" // potential waste
				}
			}
		}
	}

	if limitCount > 0 {
		result.Summary.AvgLimitCPU = totalLimit / float64(limitCount)
	}

	if result.Summary.AtRiskCount > 5 {
		result.Summary.ThrottleRisk = "high"
	} else if result.Summary.AtRiskCount > 0 {
		result.Summary.ThrottleRisk = "medium"
	} else {
		result.Summary.ThrottleRisk = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers, %d with CPU limit, %d at-risk (avg %.2f cores), risk: %s", result.Summary.TotalContainers, result.Summary.WithCPULimit, result.Summary.AtRiskCount, result.Summary.AvgLimitCPU, result.Summary.ThrottleRisk))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Layer Dedup
// ---------------------------------------------------------------

type ImgDedupResult1993 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ImgDedupSummary1993 `json:"summary"`
	TopBaseImages   []ImgDedupEntry1993 `json:"topBaseImages"`
	Recommendations []string            `json:"recommendations"`
}

type ImgDedupSummary1993 struct {
	TotalImages    int     `json:"totalUniqueImages"`
	BaseImageCount int     `json:"estimatedBaseImageCount"`
	DedupPotential float64 `json:"dedupPotentialPct"`
	TotalRefs      int     `json:"totalImageRefs"`
}

type ImgDedupEntry1993 struct {
	BaseImage string `json:"baseImage"`
	RefCount  int    `json:"refCount"`
}

func (s *Server) handleImageLayerDedup(w http.ResponseWriter, r *http.Request) {
	result := ImgDedupResult1993{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Group images by base image (extract from image name)
	baseImageMap := make(map[string]int)
	imageSet := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			imageSet[img] = true
			result.Summary.TotalRefs++

			// Extract base image (repository/name without tag)
			base := img
			// Strip tag
			for i := len(img) - 1; i >= 0; i-- {
				if img[i] == ':' && img[i-1] != '/' {
					base = img[:i]
					break
				}
			}
			baseImageMap[base]++
		}
	}

	result.Summary.TotalImages = len(imageSet)
	result.Summary.BaseImageCount = len(baseImageMap)

	if result.Summary.TotalImages > 0 {
		// Dedup potential: if many images share same base, layers are shared
		shared := 0
		for _, count := range baseImageMap {
			if count > 1 {
				shared += count - 1
			}
		}
		result.Summary.DedupPotential = float64(shared) / float64(result.Summary.TotalRefs) * 100
	}

	for base, count := range baseImageMap {
		result.TopBaseImages = append(result.TopBaseImages, ImgDedupEntry1993{
			BaseImage: base, RefCount: count,
		})
	}
	sort.Slice(result.TopBaseImages, func(i, j int) bool {
		return result.TopBaseImages[i].RefCount > result.TopBaseImages[j].RefCount
	})
	if len(result.TopBaseImages) > 15 {
		result.TopBaseImages = result.TopBaseImages[:15]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unique images, %d base images, %.0f%% dedup potential", result.Summary.TotalImages, result.Summary.BaseImageCount, result.Summary.DedupPotential))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Scheduling Latency
// ---------------------------------------------------------------

type SchedLatResult1993 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SchedLatSummary1993 `json:"summary"`
	SlowPods        []SchedLatEntry1993 `json:"slowPods"`
	Recommendations []string            `json:"recommendations"`
}

type SchedLatSummary1993 struct {
	TotalPods     int     `json:"totalPods"`
	AvgLatencySec float64 `json:"avgSchedulingLatencySec"`
	MaxLatencySec float64 `json:"maxSchedulingLatencySec"`
	SlowPods      int     `json:"slowPodsOver30s"`
	FastPods      int     `json:"fastPodsUnder5s"`
}

type SchedLatEntry1993 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	LatencySec float64 `json:"schedulingLatencySec"`
}

func (s *Server) handlePodSchedLatency(w http.ResponseWriter, r *http.Request) {
	result := SchedLatResult1993{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalLatency float64
	var maxLatency float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Find scheduled condition
		var createdTime, scheduledTime time.Time
		if !pod.CreationTimestamp.IsZero() {
			createdTime = pod.CreationTimestamp.Time
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionTrue {
				scheduledTime = cond.LastTransitionTime.Time
				break
			}
		}

		if createdTime.IsZero() || scheduledTime.IsZero() {
			continue
		}

		latency := scheduledTime.Sub(createdTime).Seconds()
		if latency < 0 {
			latency = 0
		}

		result.Summary.TotalPods++
		count++
		totalLatency += latency
		if latency > maxLatency {
			maxLatency = latency
		}

		if latency > 30 {
			result.Summary.SlowPods++
			result.SlowPods = append(result.SlowPods, SchedLatEntry1993{
				Name: pod.Name, Namespace: pod.Namespace, LatencySec: latency,
			})
			score -= 2
		} else if latency < 5 {
			result.Summary.FastPods++
		}
	}

	if count > 0 {
		result.Summary.AvgLatencySec = totalLatency / float64(count)
	}
	result.Summary.MaxLatencySec = maxLatency

	sort.Slice(result.SlowPods, func(i, j int) bool {
		return result.SlowPods[i].LatencySec > result.SlowPods[j].LatencySec
	})
	if len(result.SlowPods) > 20 {
		result.SlowPods = result.SlowPods[:20]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg scheduling %.1fs, max %.1fs, %d slow (>30s)", result.Summary.TotalPods, result.Summary.AvgLatencySec, result.Summary.MaxLatencySec, result.Summary.SlowPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
