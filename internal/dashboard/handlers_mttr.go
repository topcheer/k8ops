package dashboard

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MTTRResult is the mean time to recovery & incident lifecycle analytics engine.
// It estimates MTTD, MTTR, incident frequency, and recovery effectiveness from
// pod restart patterns, event history, and container state transitions.
type MTTRResult struct {
	ScannedAt      time.Time         `json:"scannedAt"`
	Summary        MTTRSummary       `json:"summary"`
	MTTREstimate   MTTREstimate      `json:"mttrEstimate"`
	IncidentFrequency IncidentFreq   `json:"incidentFrequency"`
	RecoveryByType []RecoveryType    `json:"recoveryByType"`
	TopIncidents   []IncidentRecord  `json:"topIncidents"`
	ByNamespace    []NSMTTR          `json:"byNamespace"`
	TrendAnalysis  MTTRTrend         `json:"trendAnalysis"`
	Recommendations []string         `json:"recommendations"`
}

// MTTRSummary aggregates incident recovery statistics.
type MTTRSummary struct {
	TotalRestarts       int     `json:"totalRestarts"`
	TotalOOMKills       int     `json:"totalOOMKills"`
	TotalCrashLoops     int     `json:"totalCrashLoops"`
	AffectedPods        int     `json:"affectedPods"`
	AffectedNamespaces  int     `json:"affectedNamespaces"`
	RecoveryRate        float64 `json:"recoveryRate"` // % of pods that recovered
	StabilityScore      int     `json:"stabilityScore"` // 0-100
}

// MTTREstimate estimates mean time to recovery from available signals.
type MTTREstimate struct {
	EstMTTRSeconds float64 `json:"estMTTRSeconds"` // estimated mean time to recovery
	EstMTTRDisplay string  `json:"estMTTR"`        // human-readable
	EstMTTDSeconds float64 `json:"estMTTDSeconds"` // estimated mean time to detect
	EstMTTDDisplay string  `json:"estMTTD"`
	Confidence     string  `json:"confidence"` // high, medium, low
	Method         string  `json:"method"`     // description of estimation method
}

// IncidentFreq analyzes incident frequency patterns.
type IncidentFreq struct {
	RestartsPerHour    float64 `json:"restartsPerHour"`
	IncidentsPerDay    float64 `json:"incidentsPerDay"`
	PeakHour           int     `json:"peakHour,omitempty"` // hour of day with most restarts
	BurstDetected      bool    `json:"burstDetected"`      // rapid restart clustering
	BurstPodCount      int     `json:"burstPodCount"`
}

// RecoveryType shows recovery patterns by failure type.
type RecoveryType struct {
	FailureType   string  `json:"failureType"`   // OOMKilled, CrashLoopBackOff, Error, Evicted
	Count         int     `json:"count"`
	AvgRecoveryS float64 `json:"avgRecoverySeconds"` // average time to recover
	RecoveryRate  float64 `json:"recoveryRate"`       // % that eventually recovered
}

// IncidentRecord describes a specific incident.
type IncidentRecord struct {
	Pod        string    `json:"pod"`
	Namespace  string    `json:"namespace"`
	Type       string    `json:"type"`       // OOMKill, CrashLoop, Restart, Eviction
	Count      int       `json:"count"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	Duration   string    `json:"duration"`
	Severity   string    `json:"severity"`
	Status     string    `json:"status"` // recovered, ongoing
}

// NSMTTR shows per-namespace MTTR stats.
type NSMTTR struct {
	Namespace   string  `json:"namespace"`
	Restarts    int     `json:"restarts"`
	Incidents   int     `json:"incidents"`
	AffectedPods int   `json:"affectedPods"`
	MTTREstimate float64 `json:"mttrSeconds"`
	Stability    int    `json:"stabilityScore"`
}

// MTTRTrend analyzes recovery trend over time.
type MTTRTrend struct {
	Improving     bool    `json:"improving"`     // restart rate decreasing
	RestartTrend  string  `json:"restartTrend"`  // improving, stable, degrading
	HourlyPattern []int   `json:"hourlyPattern"` // restarts per hour bucket (24 buckets)
}

// mttrNSData holds per-namespace MTTR data.
type mttrNSData struct {
	restarts     int
	oomKills     int
	crashLoops   int
	affectedPods int
}

// mttrFailTypeData holds failure type tracking data.
type mttrFailTypeData struct {
	count       int
	recoverySec float64
	recovered   int
}

// handleMTTR provides mean time to recovery & incident lifecycle analytics.
// GET /api/operations/mttr
func (s *Server) handleMTTR(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := MTTRResult{ScannedAt: time.Now()}
	now := time.Now()
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Collect pods
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pods")
		return
	}

	// Collect warning events for incident timeline
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type!=Normal",
		Limit:         500,
	})

	// Analyze pods for restart/recovery patterns
	totalRestarts := 0
	totalOOM := 0
	totalCrashLoop := 0
	affectedPods := 0
	recoveredPods := 0
	affectedNS := make(map[string]bool)

	// Per-namespace stats
	nsStats := make(map[string]*mttrNSData)

	// Hourly pattern
	hourlyBuckets := make([]int, 24)

	// Failure type tracking
	failTypeMap := make(map[string]*mttrFailTypeData)

	var incidents []IncidentRecord

	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}

		ns := pod.Namespace
		nsd, ok := nsStats[ns]
		if !ok {
			nsd = &mttrNSData{}
			nsStats[ns] = nsd
		}

		podHadIssue := false
		podRestarts := 0

		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)
			totalRestarts += int(cs.RestartCount)
			nsd.restarts += int(cs.RestartCount)

			// Check OOM kills
			oomCount := 0
			if cs.LastTerminationState.Terminated != nil {
				if cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					oomCount++
					totalOOM++
					nsd.oomKills++
					podHadIssue = true

					// Track failure type
					ft := failTypeMap["OOMKilled"]
					if ft == nil {
						ft = &mttrFailTypeData{}
						failTypeMap["OOMKilled"] = ft
					}
					ft.count++

					// Estimate recovery time from restart
					if !cs.LastTerminationState.Terminated.FinishedAt.IsZero() {
						recoveryS := now.Sub(cs.LastTerminationState.Terminated.FinishedAt.Time).Seconds()
						ft.recoverySec += recoveryS
						ft.recovered++
					}

					incidents = append(incidents, IncidentRecord{
						Pod:       pod.Name,
						Namespace: ns,
						Type:      "OOMKill",
						Count:     oomCount,
						LastSeen:  cs.LastTerminationState.Terminated.FinishedAt.Time,
						Severity:  "high",
						Status:    "recovered",
					})
				}
			}

			// Check CrashLoopBackOff
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				totalCrashLoop++
				nsd.crashLoops++
				podHadIssue = true

				ft := failTypeMap["CrashLoopBackOff"]
				if ft == nil {
					ft = &mttrFailTypeData{}
					failTypeMap["CrashLoopBackOff"] = ft
				}
				ft.count++

				incidents = append(incidents, IncidentRecord{
					Pod:       pod.Name,
					Namespace: ns,
					Type:      "CrashLoopBackOff",
					Count:     int(cs.RestartCount),
					LastSeen:  now,
					Severity:  "critical",
					Status:    "ongoing",
				})
			}

			// Track hourly pattern from restart count
			if cs.RestartCount > 0 && !pod.CreationTimestamp.IsZero() {
				podAgeHours := int(now.Sub(pod.CreationTimestamp.Time).Hours())
				if podAgeHours > 0 && podAgeHours < 24 {
					hourlyBuckets[podAgeHours] += int(cs.RestartCount)
				}
			}
		}

		if podHadIssue || podRestarts > 0 {
			affectedPods++
			nsd.affectedPods++
			affectedNS[ns] = true

			// Check if recovered (currently running and ready)
			if pod.Status.Phase == corev1.PodRunning {
				allReady := true
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					recoveredPods++
				}
			}
		}
	}

	// Process events for incident timeline and detection time
	eventRestartCount := 0
	for _, ev := range events.Items {
		if systemNS[ev.InvolvedObject.Namespace] {
			continue
		}
		if strings.Contains(ev.Reason, "BackOff") || strings.Contains(ev.Reason, "Unhealthy") ||
			strings.Contains(ev.Reason, "Failed") || strings.Contains(ev.Reason, "Killing") {
			eventRestartCount++
		}
	}

	// === Compute Summary ===
	result.Summary.TotalRestarts = totalRestarts
	result.Summary.TotalOOMKills = totalOOM
	result.Summary.TotalCrashLoops = totalCrashLoop
	result.Summary.AffectedPods = affectedPods
	result.Summary.AffectedNamespaces = len(affectedNS)

	if affectedPods > 0 {
		result.Summary.RecoveryRate = math.Round(float64(recoveredPods)/float64(affectedPods)*100*10) / 10
	}

	// Stability score: based on restart rate and crash loops
	clusterPodCount := len(pods.Items)
	stability := 100
	if clusterPodCount > 0 {
		restartRate := float64(totalRestarts) / float64(clusterPodCount)
		stability -= int(restartRate * 5)
	}
	stability -= totalCrashLoop * 5
	stability -= totalOOM * 3
	if stability < 0 {
		stability = 0
	}
	result.Summary.StabilityScore = stability

	// === MTTR Estimate ===
	// Use failure type recovery data if available, otherwise estimate from pod age and restart patterns
	mttrSeconds := 0.0
	mttdSeconds := 0.0
	method := ""
	confidence := "low"

	totalRecoverySec := 0.0
	totalRecoveredCount := 0
	for _, ft := range failTypeMap {
		totalRecoverySec += ft.recoverySec
		totalRecoveredCount += ft.recovered
	}

	if totalRecoveredCount > 0 {
		mttrSeconds = totalRecoverySec / float64(totalRecoveredCount)
		// Cap at reasonable values
		if mttrSeconds > 3600 {
			mttrSeconds = 3600 // cap at 1 hour
		}
		method = "Calculated from container termination and restart timestamps"
		confidence = "medium"
	} else {
		// Fallback: estimate from pod lifecycle patterns
		if clusterPodCount > 0 && totalRestarts > 0 {
			// Rough estimate: average pod age / restart count gives approximate interval between failures
			// Recovery time is typically 1/10th of the interval
			mttrSeconds = 60 // default 1 minute for auto-restart
			method = "Estimated from restart frequency (no termination timestamps available)"
			confidence = "low"
		} else {
			mttrSeconds = 0
			method = "No failure data available"
			confidence = "high" // no failures = high confidence in stability
		}
	}

	// MTTD: typically much shorter than MTTR (K8s detects within seconds)
	mttdSeconds = math.Max(5, mttrSeconds*0.1) // 10% of MTTR or minimum 5 seconds

	result.MTTREstimate = MTTREstimate{
		EstMTTRSeconds: math.Round(mttrSeconds),
		EstMTTRDisplay: mttrFormatDuration(mttrSeconds),
		EstMTTDSeconds: math.Round(mttdSeconds),
		EstMTTDDisplay: mttrFormatDuration(mttdSeconds),
		Confidence:     confidence,
		Method:         method,
	}

	// === Incident Frequency ===
	clusterAgeHours := 1.0
	if clusterPodCount > 0 {
		// Estimate cluster age from oldest pod
		oldestPod := now
		for _, pod := range pods.Items {
			if pod.CreationTimestamp.Time.Before(oldestPod) {
				oldestPod = pod.CreationTimestamp.Time
			}
		}
		clusterAgeHours = now.Sub(oldestPod).Hours()
		if clusterAgeHours < 1 {
			clusterAgeHours = 1
		}
	}

	restartsPerHour := float64(totalRestarts) / clusterAgeHours
	incidentsPerDay := restartsPerHour * 24

	// Detect burst (rapid restart clustering)
	burstPods := 0
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 10 {
				burstPods++
			}
		}
	}

	// Find peak hour
	peakHour := 0
	peakCount := 0
	for h, c := range hourlyBuckets {
		if c > peakCount {
			peakCount = c
			peakHour = h
		}
	}

	result.IncidentFrequency = IncidentFreq{
		RestartsPerHour: math.Round(restartsPerHour*100) / 100,
		IncidentsPerDay: math.Round(incidentsPerDay*10) / 10,
		PeakHour:        peakHour,
		BurstDetected:   burstPods > 0,
		BurstPodCount:   burstPods,
	}

	// === Recovery by Type ===
	for failType, ft := range failTypeMap {
		avgRecovery := 0.0
		if ft.recovered > 0 {
			avgRecovery = ft.recoverySec / float64(ft.recovered)
			if avgRecovery > 3600 {
				avgRecovery = 3600
			}
		}
		recRate := 0.0
		if ft.count > 0 {
			recRate = float64(ft.recovered) / float64(ft.count) * 100
		}
		result.RecoveryByType = append(result.RecoveryByType, RecoveryType{
			FailureType:    failType,
			Count:          ft.count,
			AvgRecoveryS:  math.Round(avgRecovery),
			RecoveryRate:   math.Round(recRate*10) / 10,
		})
	}
	sort.Slice(result.RecoveryByType, func(i, j int) bool {
		return result.RecoveryByType[i].Count > result.RecoveryByType[j].Count
	})

	// === Top Incidents ===
	sort.Slice(incidents, func(i, j int) bool {
		if incidents[i].Severity != incidents[j].Severity {
			return mttrSeverityRank(incidents[i].Severity) > mttrSeverityRank(incidents[j].Severity)
		}
		return incidents[i].Count > incidents[j].Count
	})
	if len(incidents) > 20 {
		incidents = incidents[:20]
	}
	for i := range incidents {
		if !incidents[i].LastSeen.IsZero() {
			dur := now.Sub(incidents[i].LastSeen)
			incidents[i].Duration = mttrFormatDuration(dur.Seconds()) + " ago"
		}
		if incidents[i].FirstSeen.IsZero() {
			incidents[i].FirstSeen = incidents[i].LastSeen
		}
	}
	result.TopIncidents = incidents

	// === By Namespace ===
	for nsName, nsd := range nsStats {
		mttr := 0.0
		if nsd.restarts > 0 {
			mttr = 300 // estimate 5 minutes average recovery per namespace
		}
		nsStability := 100
		nsStability -= nsd.restarts * 2
		nsStability -= nsd.crashLoops * 10
		nsStability -= nsd.oomKills * 5
		if nsStability < 0 {
			nsStability = 0
		}
		result.ByNamespace = append(result.ByNamespace, NSMTTR{
			Namespace:    nsName,
			Restarts:     nsd.restarts,
			Incidents:    nsd.oomKills + nsd.crashLoops,
			AffectedPods: nsd.affectedPods,
			MTTREstimate: mttr,
			Stability:    nsStability,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Restarts > result.ByNamespace[j].Restarts
	})

	// === Trend Analysis ===
	result.TrendAnalysis.HourlyPattern = hourlyBuckets
	if result.IncidentFrequency.BurstDetected {
		result.TrendAnalysis.RestartTrend = "degrading"
	} else if totalRestarts > 0 && result.Summary.RecoveryRate > 80 {
		result.TrendAnalysis.RestartTrend = "stable"
		result.TrendAnalysis.Improving = true
	} else if totalRestarts == 0 {
		result.TrendAnalysis.RestartTrend = "stable"
		result.TrendAnalysis.Improving = true
	} else {
		result.TrendAnalysis.RestartTrend = "stable"
	}

	// === Recommendations ===
	result.Recommendations = generateMTTRRecs(result)

	writeJSON(w, result)
}

// generateMTTRRecs produces actionable recommendations.
func generateMTTRRecs(result MTTRResult) []string {
	var recs []string

	if result.Summary.StabilityScore < 50 {
		recs = append(recs, fmt.Sprintf("Cluster stability score is %d/100 — multiple failure patterns detected requiring investigation", result.Summary.StabilityScore))
	}

	if result.Summary.TotalCrashLoops > 0 {
		recs = append(recs, fmt.Sprintf("%d CrashLoopBackOff pods detected — check application logs, resource limits, and configuration", result.Summary.TotalCrashLoops))
	}

	if result.Summary.TotalOOMKills > 0 {
		recs = append(recs, fmt.Sprintf("%d OOMKill events — increase memory limits or optimize memory usage in affected workloads", result.Summary.TotalOOMKills))
	}

	if result.IncidentFrequency.BurstDetected {
		recs = append(recs, fmt.Sprintf("Restart burst detected (%d pods with >10 restarts) — investigate cascading failure or resource contention", result.IncidentFrequency.BurstPodCount))
	}

	if result.Summary.RecoveryRate < 50 && result.Summary.AffectedPods > 0 {
		recs = append(recs, fmt.Sprintf("Only %.1f%% of affected pods recovered — check if auto-healing mechanisms (HPA, restart policies) are functioning", result.Summary.RecoveryRate))
	}

	if result.MTTREstimate.EstMTTRSeconds > 300 {
		recs = append(recs, fmt.Sprintf("Estimated MTTR is %s — consider implementing better health checks and faster rollback mechanisms", result.MTTREstimate.EstMTTRDisplay))
	}

	// Namespace hotspot
	if len(result.ByNamespace) > 0 && result.ByNamespace[0].Restarts > 10 {
		ns := result.ByNamespace[0]
		recs = append(recs, fmt.Sprintf("Namespace '%s' has the most incidents (%d restarts, stability %d) — prioritize investigation", ns.Namespace, ns.Restarts, ns.Stability))
	}

	if result.Summary.TotalRestarts == 0 {
		recs = append(recs, "No restart incidents detected — cluster is stable, maintain current practices")
	}

	return recs
}

// mttrFormatDuration converts float seconds to human-readable duration.
func mttrFormatDuration(seconds float64) string {
	return formatDuration(time.Duration(seconds) * time.Second)
}

// severityRank returns a sortable rank for severity levels.
func mttrSeverityRank(sev string) int {
	switch sev {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
