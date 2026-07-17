package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployFreqTrendResult analyzes deployment frequency patterns using
// ReplicaSet creation timestamps and revision history to compute DORA metrics.
type DeployFreqTrendResult struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	LookbackDays    int                       `json:"lookbackDays"`
	Summary         DeployFreqTrendSummary    `json:"summary"`
	ByDay           []DeployFreqTrendDayEntry `json:"byDay"`
	ByWorkload      []DeployFreqTrendWLEntry  `json:"byWorkload"`
	DoraLevel       string                    `json:"doraLevel"`
	FrequencyScore  int                       `json:"frequencyScore"`
	Grade           string                    `json:"grade"`
	Recommendations []string                  `json:"recommendations"`
}

type DeployFreqTrendSummary struct {
	TotalDeploys    int     `json:"totalDeploys"`
	AvgPerDay       float64 `json:"avgPerDay"`
	AvgPerWeek      float64 `json:"avgPerWeek"`
	PeakDay         string  `json:"peakDay"`
	PeakDayCount    int     `json:"peakDayCount"`
	ActiveWorkloads int     `json:"activeWorkloads"`
	UniqueDeployers int     `json:"uniqueDeployers"`
	TimeSpan        int     `json:"timeSpanDays"`
}

type DeployFreqTrendDayEntry struct {
	Date    time.Time `json:"date"`
	Count   int       `json:"count"`
	DayName string    `json:"dayName"`
}

type DeployFreqTrendWLEntry struct {
	Workload      string    `json:"workload"`
	Namespace     string    `json:"namespace"`
	DeployCount   int       `json:"deployCount"`
	LastDeploy    time.Time `json:"lastDeploy"`
	DaysSinceLast int       `json:"daysSinceLast"`
	AvgInterval   float64   `json:"avgIntervalHours"`
}

// handleDeployFrequencyTrend handles GET /api/deployment/deploy-frequency-trend
func (s *Server) handleDeployFrequencyTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	lookback := 30 // days
	result := DeployFreqTrendResult{
		ScannedAt:    time.Now(),
		LookbackDays: lookback,
	}

	since := time.Now().Add(-time.Duration(lookback) * 24 * time.Hour)

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Collect ReplicaSet creation timestamps as proxy for deploy events
	// Each new RS = one deployment
	type deployEvent struct {
		timestamp time.Time
		workload  string
		namespace string
	}
	var events []deployEvent
	wlMap := make(map[string]*DeployFreqTrendWLEntry)

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		key := d.Namespace + "/" + d.Name
		if _, ok := wlMap[key]; !ok {
			wlMap[key] = &DeployFreqTrendWLEntry{
				Workload:  d.Name,
				Namespace: d.Namespace,
			}
		}
		entry := wlMap[key]
		entry.DeployCount++
		entry.LastDeploy = d.ObjectMeta.CreationTimestamp.Time

		// Use deployment creation as first deploy event
		if d.CreationTimestamp.Time.After(since) {
			events = append(events, deployEvent{
				timestamp: d.CreationTimestamp.Time,
				workload:  d.Name,
				namespace: d.Namespace,
			})
		}
	}

	// Also scan ReplicaSets (more accurate for multi-deploy workloads)
	rss, err := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, rs := range rss.Items {
			if isSystemNamespace(rs.Namespace) {
				continue
			}
			if !rs.CreationTimestamp.Time.After(since) {
				continue
			}
			// Get parent deployment name
			depName := ""
			for _, ref := range rs.OwnerReferences {
				if ref.Kind == "Deployment" {
					depName = ref.Name
					break
				}
			}
			if depName == "" {
				continue
			}

			events = append(events, deployEvent{
				timestamp: rs.CreationTimestamp.Time,
				workload:  depName,
				namespace: rs.Namespace,
			})

			key := rs.Namespace + "/" + depName
			if _, ok := wlMap[key]; !ok {
				wlMap[key] = &DeployFreqTrendWLEntry{
					Workload:  depName,
					Namespace: rs.Namespace,
				}
			}
		}
	}

	// Re-count per workload from events
	wlDeployCount := make(map[string]int)
	wlLastDeploy := make(map[string]time.Time)
	for _, ev := range events {
		key := ev.namespace + "/" + ev.workload
		wlDeployCount[key]++
		if ev.timestamp.After(wlLastDeploy[key]) {
			wlLastDeploy[key] = ev.timestamp
		}
	}

	// Build per-day counts
	dayMap := make(map[string]int)
	for _, ev := range events {
		dateStr := ev.timestamp.Format("2006-01-02")
		dayMap[dateStr]++
	}

	var dayEntries []DeployFreqTrendDayEntry
	peakCount := 0
	peakDay := ""
	for i := 0; i < lookback; i++ {
		date := time.Now().Add(-time.Duration(lookback-1-i) * 24 * time.Hour)
		dateStr := date.Format("2006-01-02")
		count := dayMap[dateStr]
		dayEntries = append(dayEntries, DeployFreqTrendDayEntry{
			Date:    date.Truncate(24 * time.Hour),
			Count:   count,
			DayName: date.Format("Mon"),
		})
		if count > peakCount {
			peakCount = count
			peakDay = dateStr
		}
	}
	result.ByDay = dayEntries

	// Build per-workload entries
	var wlEntries []DeployFreqTrendWLEntry
	now := time.Now()
	for key, e := range wlMap {
		count := wlDeployCount[key]
		if count == 0 {
			count = e.DeployCount
		}
		lastDeploy := wlLastDeploy[key]
		if lastDeploy.IsZero() {
			lastDeploy = e.LastDeploy
		}
		daysSince := int(now.Sub(lastDeploy).Hours() / 24)
		if daysSince < 0 {
			daysSince = 0
		}
		avgInterval := 0.0
		if count > 1 && daysSince > 0 {
			avgInterval = float64(daysSince*24) / float64(count)
		}
		wlEntries = append(wlEntries, DeployFreqTrendWLEntry{
			Workload:      e.Workload,
			Namespace:     e.Namespace,
			DeployCount:   count,
			LastDeploy:    lastDeploy,
			DaysSinceLast: daysSince,
			AvgInterval:   avgInterval,
		})
	}

	sort.Slice(wlEntries, func(i, j int) bool {
		return wlEntries[i].DeployCount > wlEntries[j].DeployCount
	})
	result.ByWorkload = wlEntries

	// Summary
	result.Summary.TotalDeploys = len(events)
	result.Summary.ActiveWorkloads = len(wlEntries)
	result.Summary.PeakDay = peakDay
	result.Summary.PeakDayCount = peakCount
	result.Summary.TimeSpan = lookback
	if lookback > 0 {
		result.Summary.AvgPerDay = float64(len(events)) / float64(lookback)
		result.Summary.AvgPerWeek = result.Summary.AvgPerDay * 7
	}

	// DORA level classification
	switch {
	case result.Summary.AvgPerDay >= 1:
		result.DoraLevel = "Elite" // Multiple deploys per day
		result.FrequencyScore = 100
	case result.Summary.AvgPerWeek >= 1:
		result.DoraLevel = "High" // Between once per day and once per week
		result.FrequencyScore = 75
	case result.Summary.AvgPerWeek > 0:
		result.DoraLevel = "Medium" // Between once per week and once per month
		result.FrequencyScore = 50
	default:
		result.DoraLevel = "Low" // Less than once per month
		result.FrequencyScore = 25
	}

	switch {
	case result.FrequencyScore >= 90:
		result.Grade = "A"
	case result.FrequencyScore >= 70:
		result.Grade = "B"
	case result.FrequencyScore >= 50:
		result.Grade = "C"
	case result.FrequencyScore >= 30:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildDeployFreqTrendRecs(&result)
	writeJSON(w, result)
}

func buildDeployFreqTrendRecs(r *DeployFreqTrendResult) []string {
	recs := []string{
		fmt.Sprintf("部署频率: %d 次/%d天 (%.1f/天), DORA 等级: %s", r.Summary.TotalDeploys, r.LookbackDays, r.Summary.AvgPerDay, r.DoraLevel),
	}
	if r.Summary.PeakDayCount > 0 {
		recs = append(recs, fmt.Sprintf("峰值日: %s (%d 次部署)", r.Summary.PeakDay, r.Summary.PeakDayCount))
	}
	if r.DoraLevel == "Low" {
		recs = append(recs, "部署频率过低, 建议实施 CI/CD 自动化流水线")
	}
	if r.Summary.ActiveWorkloads > 0 && len(r.ByWorkload) > 0 {
		top := r.ByWorkload[0]
		recs = append(recs, fmt.Sprintf("最活跃: %s/%s (%d 次部署, 最后 %d 天前)", top.Namespace, top.Workload, top.DeployCount, top.DaysSinceLast))
	}
	return recs
}

// keep import
var _ appsv1.Deployment
