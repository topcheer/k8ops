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
// v19.66 — Operations Dimension (Round 14)
// 1. Pod Restart Cost — downtime & resource waste from restarts
// 2. Node Disk I/O Health — disk pressure & I/O bottleneck detection
// 3. Event QPS Analyzer — event generation rate & API server pressure
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Restart Cost Estimator
// ---------------------------------------------------------------

type RestartCostResult1966 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         RestartCostSummary1966 `json:"summary"`
	HighCostPods    []RestartCostEntry1966 `json:"highCostPods"`
	Recommendations []string               `json:"recommendations"`
}

type RestartCostSummary1966 struct {
	TotalPods            int     `json:"totalPods"`
	TotalRestarts        int     `json:"totalRestarts"`
	HighRestartPods      int     `json:"highRestartPods"`
	EstimatedDowntimeMin float64 `json:"estimatedDowntimeMin"`
	EstimatedWasteCPU    float64 `json:"estimatedWasteCPUCores"`
	EstimatedWasteMemGB  float64 `json:"estimatedWasteMemGB"`
	TotalCostImpactUSD   float64 `json:"estimatedCostImpactUSD"`
}

type RestartCostEntry1966 struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Restarts    int     `json:"restarts"`
	DowntimeMin float64 `json:"estimatedDowntimeMin"`
	WasteCPU    float64 `json:"wasteCPU"`
	WasteMem    float64 `json:"wasteMemGB"`
	CostUSD     float64 `json:"costImpactUSD"`
}

func (s *Server) handlePodRestartCost(w http.ResponseWriter, r *http.Request) {
	result := RestartCostResult1966{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Cost model: each restart wastes ~30s of startup + resource overhead
	// CPU cost: ~$0.03/core/hour, Memory: ~$0.004/GB/hour (rough cloud pricing)
	const restartDowntimeSec = 30.0
	const cpuCostPerCoreHour = 0.03
	const memCostPerGBHour = 0.004

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
		}

		if totalRestarts == 0 {
			continue
		}

		result.Summary.TotalRestarts += totalRestarts

		// Calculate wasted resources during restarts
		podCPU := 0.0
		podMem := 0.0
		for _, c := range pod.Spec.Containers {
			podCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			podMem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}

		downtimeMin := float64(totalRestarts) * restartDowntimeSec / 60.0
		wasteCPU := podCPU * downtimeMin / 60.0
		wasteMem := podMem * downtimeMin / 60.0
		costUSD := wasteCPU*cpuCostPerCoreHour + wasteMem*memCostPerGBHour

		result.Summary.EstimatedDowntimeMin += downtimeMin
		result.Summary.EstimatedWasteCPU += wasteCPU
		result.Summary.EstimatedWasteMemGB += wasteMem
		result.Summary.TotalCostImpactUSD += costUSD

		if totalRestarts >= 5 {
			result.Summary.HighRestartPods++
			result.HighCostPods = append(result.HighCostPods, RestartCostEntry1966{
				Name: pod.Name, Namespace: pod.Namespace,
				Restarts: totalRestarts, DowntimeMin: downtimeMin,
				WasteCPU: wasteCPU, WasteMem: wasteMem, CostUSD: costUSD,
			})
			if totalRestarts >= 20 {
				score -= 5
			} else {
				score -= 2
			}
		}
	}

	sort.Slice(result.HighCostPods, func(i, j int) bool {
		return result.HighCostPods[i].Restarts > result.HighCostPods[j].Restarts
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total restarts across %d pods — est. %.1f min downtime, $%.2f cost impact", result.Summary.TotalRestarts, result.Summary.TotalPods, result.Summary.EstimatedDowntimeMin, result.Summary.TotalCostImpactUSD))
	if result.Summary.HighRestartPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with 5+ restarts — investigate crash patterns", result.Summary.HighRestartPods))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Disk I/O Health
// ---------------------------------------------------------------

type NodeDiskIOResult1966 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeDiskIOSummary1966 `json:"summary"`
	AtRiskNodes     []NodeDiskIOEntry1966 `json:"atRiskNodes"`
	Recommendations []string              `json:"recommendations"`
}

type NodeDiskIOSummary1966 struct {
	TotalNodes        int `json:"totalNodes"`
	HealthyNodes      int `json:"healthyNodes"`
	DiskPressureNodes int `json:"diskPressureNodes"`
	HighImageCount    int `json:"nodesWithHighImageCount"`
}

type NodeDiskIOEntry1966 struct {
	Name            string `json:"name"`
	HasDiskPressure bool   `json:"hasDiskPressure"`
	PressureAge     string `json:"pressureAge"`
	ImagesOnNode    int    `json:"imagesOnNode"`
	Status          string `json:"status"`
}

func (s *Server) handleNodeDiskIOHealth(w http.ResponseWriter, r *http.Request) {
	result := NodeDiskIOResult1966{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		entry := NodeDiskIOEntry1966{
			Name:   node.Name,
			Status: "healthy",
		}

		// Check for DiskPressure condition
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeDiskPressure {
				if cond.Status == corev1.ConditionTrue {
					entry.HasDiskPressure = true
					entry.Status = "disk-pressure"
					entry.PressureAge = fmt.Sprintf("%.0fh", time.Since(cond.LastTransitionTime.Time).Hours())
					result.Summary.DiskPressureNodes++
					score -= 10
				}
			}
		}

		// Check images on node (high count indicates GC pressure)
		entry.ImagesOnNode = len(node.Status.Images)
		if entry.ImagesOnNode > 50 {
			result.Summary.HighImageCount++
			if entry.Status == "healthy" {
				entry.Status = "high-image-count"
			}
			score -= 2
		}

		if entry.Status != "healthy" {
			result.AtRiskNodes = append(result.AtRiskNodes, entry)
		} else {
			result.Summary.HealthyNodes++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d healthy, %d disk pressure, %d high image count", result.Summary.TotalNodes, result.Summary.HealthyNodes, result.Summary.DiskPressureNodes, result.Summary.HighImageCount))
	if result.Summary.DiskPressureNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with disk pressure — run kubelet GC or increase disk size", result.Summary.DiskPressureNodes))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event QPS Analyzer
// ---------------------------------------------------------------

type EventQPSResult1966 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EventQPSSummary1966   `json:"summary"`
	TopEventSources []EventQPSEntry1966   `json:"topEventSources"`
	NoisyNS         []EventQPSNSEntry1966 `json:"noisyNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type EventQPSSummary1966 struct {
	TotalEvents   int     `json:"totalEvents"`
	EventsPerMin  float64 `json:"eventsPerMinute"`
	HighVolumeNS  int     `json:"highVolumeNamespaces"`
	WarningEvents int     `json:"warningEvents"`
	NormalEvents  int     `json:"normalEvents"`
	PressureLevel string  `json:"pressureLevel"`
}

type EventQPSEntry1966 struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type EventQPSNSEntry1966 struct {
	Namespace    string  `json:"namespace"`
	EventCount   int     `json:"eventCount"`
	EventsPerMin float64 `json:"eventsPerMin"`
}

func (s *Server) handleEventQPSAnalyzer(w http.ResponseWriter, r *http.Request) {
	result := EventQPSResult1966{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	// Track events per source and namespace
	sourceMap := make(map[string]*EventQPSEntry1966)
	nsMap := make(map[string]int)

	// Determine time window from most recent event
	var newestTime time.Time
	for _, evt := range eventList.Items {
		var evtTime time.Time
		if !evt.LastTimestamp.IsZero() {
			evtTime = evt.LastTimestamp.Time
		} else if !evt.CreationTimestamp.IsZero() {
			evtTime = evt.CreationTimestamp.Time
		}
		if evtTime.After(newestTime) {
			newestTime = evtTime
		}
	}

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		if evt.Type == "Warning" {
			result.Summary.WarningEvents++
		} else {
			result.Summary.NormalEvents++
		}

		// Track by source+reason
		key := evt.Source.Component + "/" + evt.Reason
		if entry, ok := sourceMap[key]; ok {
			entry.Count++
		} else {
			sourceMap[key] = &EventQPSEntry1966{
				Source: evt.Source.Component, Reason: evt.Reason, Count: 1,
			}
		}

		// Track by namespace
		nsMap[evt.Namespace]++
	}

	// Calculate events per minute (last hour window)
	timeWindow := time.Hour
	if !newestTime.IsZero() {
		// Use actual span if events exist
		oldestTime := newestTime
		for _, evt := range eventList.Items {
			var evtTime time.Time
			if !evt.LastTimestamp.IsZero() {
				evtTime = evt.LastTimestamp.Time
			} else if !evt.CreationTimestamp.IsZero() {
				evtTime = evt.CreationTimestamp.Time
			}
			if evtTime.Before(oldestTime) && !evtTime.IsZero() {
				oldestTime = evtTime
			}
		}
		span := newestTime.Sub(oldestTime)
		if span > 0 {
			timeWindow = span
		}
	}

	result.Summary.EventsPerMin = float64(result.Summary.TotalEvents) / timeWindow.Minutes()

	// Sort sources by count
	for _, entry := range sourceMap {
		result.TopEventSources = append(result.TopEventSources, *entry)
	}
	sort.Slice(result.TopEventSources, func(i, j int) bool {
		return result.TopEventSources[i].Count > result.TopEventSources[j].Count
	})
	if len(result.TopEventSources) > 20 {
		result.TopEventSources = result.TopEventSources[:20]
	}

	// Namespace noise
	for ns, count := range nsMap {
		epm := float64(count) / timeWindow.Minutes()
		result.NoisyNS = append(result.NoisyNS, EventQPSNSEntry1966{
			Namespace: ns, EventCount: count, EventsPerMin: epm,
		})
		if count > 100 {
			result.Summary.HighVolumeNS++
			score -= 2
		}
	}
	sort.Slice(result.NoisyNS, func(i, j int) bool {
		return result.NoisyNS[i].EventCount > result.NoisyNS[j].EventCount
	})

	// Pressure level
	if result.Summary.EventsPerMin > 100 {
		result.Summary.PressureLevel = "critical"
		score -= 10
	} else if result.Summary.EventsPerMin > 50 {
		result.Summary.PressureLevel = "high"
		score -= 5
	} else if result.Summary.EventsPerMin > 10 {
		result.Summary.PressureLevel = "medium"
	} else {
		result.Summary.PressureLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events (%.1f/min), pressure: %s", result.Summary.TotalEvents, result.Summary.EventsPerMin, result.Summary.PressureLevel))
	if result.Summary.HighVolumeNS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces with 100+ events — investigate noisy controllers", result.Summary.HighVolumeNS))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
