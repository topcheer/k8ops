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
// v20.26 — Operations Dimension (Round 24)
// 1. Pod CPU Throttling Estimator — container CPU limit throttling risk
// 2. Namespace Resource Pressure — namespace-level resource consumption
// 3. Event Age Decay Tracker — stale event cleanup tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Pod CPU Throttling Estimator
// ---------------------------------------------------------------

type CPUThrotResult2026 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CPUThrotSummary2026 `json:"summary"`
	AtRisk          []CPUThrotEntry2026 `json:"atRiskContainers"`
	Recommendations []string            `json:"recommendations"`
}

type CPUThrotSummary2026 struct {
	TotalContainers int     `json:"totalContainers"`
	WithCPULimit    int     `json:"withCPULimit"`
	AtRiskThrottle  int     `json:"atRiskThrottle"`
	AvgCPULimit     float64 `json:"avgCPULimitCores"`
}

type CPUThrotEntry2026 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Container string  `json:"container"`
	CPULimit  float64 `json:"cpuLimitCores"`
}

func (s *Server) handleCPUThrotEst(w http.ResponseWriter, r *http.Request) {
	result := CPUThrotResult2026{ScannedAt: time.Now()}
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

			if c.Resources.Limits.Cpu().IsZero() {
				continue
			}
			cpuLimit := c.Resources.Limits.Cpu().AsApproximateFloat64()
			result.Summary.WithCPULimit++
			totalLimit += cpuLimit
			limitCount++

			// Containers with very low CPU limit (<100m) are at high risk
			if cpuLimit < 0.1 {
				result.Summary.AtRiskThrottle++
				result.AtRisk = append(result.AtRisk, CPUThrotEntry2026{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, CPULimit: cpuLimit,
				})
				score -= 1
			}
		}
	}

	if limitCount > 0 {
		result.Summary.AvgCPULimit = totalLimit / float64(limitCount)
	}

	sort.Slice(result.AtRisk, func(i, j int) bool {
		return result.AtRisk[i].CPULimit < result.AtRisk[j].CPULimit
	})
	if len(result.AtRisk) > 20 {
		result.AtRisk = result.AtRisk[:20]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with CPU limit, %d at-risk, avg %.2f cores", result.Summary.TotalContainers, result.Summary.WithCPULimit, result.Summary.AtRiskThrottle, result.Summary.AvgCPULimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Namespace Resource Pressure
// ---------------------------------------------------------------

type NSPressureResult2026 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NSPressureSummary2026 `json:"summary"`
	PerNS           []NSPressureEntry2026 `json:"perNamespace"`
	Recommendations []string              `json:"recommendations"`
}

type NSPressureSummary2026 struct {
	TotalNS        int `json:"totalNamespaces"`
	HighPressureNS int `json:"highPressureNamespaces"`
	TotalPods      int `json:"totalPods"`
}

type NSPressureEntry2026 struct {
	Namespace     string  `json:"namespace"`
	PodCount      int     `json:"podCount"`
	CPURequest    float64 `json:"cpuRequestCores"`
	MemRequest    float64 `json:"memRequestGB"`
	PressureScore float64 `json:"pressureScore"`
}

func (s *Server) handleNSPressure(w http.ResponseWriter, r *http.Request) {
	result := NSPressureResult2026{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*NSPressureEntry2026)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		entry, ok := nsStats[pod.Namespace]
		if !ok {
			entry = &NSPressureEntry2026{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = entry
		}
		entry.PodCount++

		for _, c := range pod.Spec.Containers {
			entry.CPURequest += c.Resources.Requests.Cpu().AsApproximateFloat64()
			entry.MemRequest += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}

	result.Summary.TotalNS = len(nsStats)

	// Calculate pressure score: weighted sum of pods + CPU + Mem
	maxScore := 0.0
	for _, entry := range nsStats {
		entry.PressureScore = float64(entry.PodCount)*0.5 + entry.CPURequest*10 + entry.MemRequest*5
		if entry.PressureScore > maxScore {
			maxScore = entry.PressureScore
		}
	}

	// Normalize: >70pct of max = high pressure
	threshold := maxScore * 0.7
	if threshold < 10 {
		threshold = 10
	}
	for _, entry := range nsStats {
		if entry.PressureScore > threshold {
			result.Summary.HighPressureNS++
		}
		result.PerNS = append(result.PerNS, *entry)
	}

	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].PressureScore > result.PerNS[j].PressureScore
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d NS, %d high-pressure, %d total pods", result.Summary.TotalNS, result.Summary.HighPressureNS, result.Summary.TotalPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event Age Decay Tracker
// ---------------------------------------------------------------

type EvtAgeResult2026 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EvtAgeSummary2026  `json:"summary"`
	PerBucket       []EvtAgeBucket2026 `json:"ageDistribution"`
	Recommendations []string           `json:"recommendations"`
}

type EvtAgeSummary2026 struct {
	TotalEvents int `json:"totalEvents"`
	StaleEvents int `json:"staleEventsOver1h"`
	OldEvents   int `json:"oldEventsOver1d"`
}

type EvtAgeBucket2026 struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

func (s *Server) handleEvtAgeDecay(w http.ResponseWriter, r *http.Request) {
	result := EvtAgeResult2026{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	buckets := map[string]int{
		"<1m": 0, "1-10m": 0, "10-60m": 0, "1-24h": 0, ">24h": 0,
	}
	now := time.Now()

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		var evtTime time.Time
		if evt.LastTimestamp.IsZero() {
			evtTime = evt.CreationTimestamp.Time
		} else {
			evtTime = evt.LastTimestamp.Time
		}

		ageMin := now.Sub(evtTime).Minutes()
		switch {
		case ageMin < 1:
			buckets["<1m"]++
		case ageMin < 10:
			buckets["1-10m"]++
		case ageMin < 60:
			buckets["10-60m"]++
		case ageMin < 1440:
			buckets["1-24h"]++
			result.Summary.StaleEvents++
		default:
			buckets[">24h"]++
			result.Summary.OldEvents++
		}
	}

	order := []string{"<1m", "1-10m", "10-60m", "1-24h", ">24h"}
	for _, b := range order {
		result.PerBucket = append(result.PerBucket, EvtAgeBucket2026{Bucket: b, Count: buckets[b]})
	}

	if result.Summary.OldEvents > 100 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events: %d stale (>1h), %d old (>1d)", result.Summary.TotalEvents, result.Summary.StaleEvents, result.Summary.OldEvents))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
