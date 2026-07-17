package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployHeatmapResult creates a deployment activity heatmap showing which
// namespaces and time periods have the most rollout activity. Useful for
// identifying deployment bottlenecks and planning change windows.
type DeployHeatmapResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         HeatmapSummary   `json:"summary"`
	ByNamespace     []HeatmapNS      `json:"byNamespace"`
	ByHour          []HeatmapHour    `json:"byHour"`
	ByWeekday       []HeatmapWeekday `json:"byWeekday"`
	Hotspots        []HeatmapHotspot `json:"hotspots"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type HeatmapSummary struct {
	TotalUpdates int    `json:"totalUpdates"`
	ActiveNS     int    `json:"activeNamespaces"`
	PeakHour     int    `json:"peakHour"`
	PeakWeekday  string `json:"peakWeekday"`
	LullDetected bool   `json:"lullDetected"`
}

type HeatmapNS struct {
	Namespace string  `json:"namespace"`
	Updates   int     `json:"updates"`
	Percent   float64 `json:"percent"`
}

type HeatmapHour struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type HeatmapWeekday struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type HeatmapHotspot struct {
	Namespace string `json:"namespace"`
	Detail    string `json:"detail"`
	Severity  string `json:"severity"`
}

// handleDeployHeatmap handles GET /api/deployment/deploy-heatmap
func (s *Server) handleDeployHeatmap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployHeatmapResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	nsMap := make(map[string]int)
	hourMap := [24]int{}
	weekdayMap := [7]int{}
	totalUpdates := 0

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}

		// Use last progressing condition update as deployment time
		deployTime := d.CreationTimestamp.Time
		for _, cond := range d.Status.Conditions {
			if cond.Type == "Progressing" && cond.LastUpdateTime.Time.After(deployTime) {
				deployTime = cond.LastUpdateTime.Time
			}
		}

		// Only count updates in last 7 days
		if now.Sub(deployTime) > 7*24*time.Hour {
			continue
		}

		totalUpdates++
		nsMap[d.Namespace]++
		hourMap[deployTime.Hour()]++
		weekdayMap[deployTime.Weekday()]++
	}

	result.Summary.TotalUpdates = totalUpdates
	result.Summary.ActiveNS = len(nsMap)

	// Peak hour
	peakH := 0
	peakCount := 0
	for h, c := range hourMap {
		if c > peakCount {
			peakCount = c
			peakH = h
		}
	}
	result.Summary.PeakHour = peakH

	// Peak weekday
	wdNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	peakWd := 0
	peakWdCount := 0
	for wd, c := range weekdayMap {
		if c > peakWdCount {
			peakWdCount = c
			peakWd = wd
		}
	}
	result.Summary.PeakWeekday = wdNames[peakWd]
	result.Summary.LullDetected = totalUpdates == 0

	// NS breakdown
	for ns, count := range nsMap {
		pct := 0.0
		if totalUpdates > 0 {
			pct = float64(count) / float64(totalUpdates) * 100
		}
		result.ByNamespace = append(result.ByNamespace, HeatmapNS{
			Namespace: ns, Updates: count, Percent: pct,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Updates > result.ByNamespace[j].Updates
	})

	// Hour breakdown
	for h, c := range hourMap {
		result.ByHour = append(result.ByHour, HeatmapHour{Hour: h, Count: c})
	}
	sort.Slice(result.ByHour, func(i, j int) bool {
		return result.ByHour[i].Hour < result.ByHour[j].Hour
	})

	// Weekday breakdown
	for wd, c := range weekdayMap {
		result.ByWeekday = append(result.ByWeekday, HeatmapWeekday{Day: wdNames[wd], Count: c})
	}

	// Hotspots
	if peakCount > 5 {
		result.Hotspots = append(result.Hotspots, HeatmapHotspot{
			Namespace: "*",
			Detail:    fmt.Sprintf("部署高峰时段 %02d:00 (%d 次)", peakH, peakCount),
			Severity:  "info",
		})
	}
	for _, ns := range result.ByNamespace {
		if ns.Updates > 10 {
			result.Hotspots = append(result.Hotspots, HeatmapHotspot{
				Namespace: ns.Namespace,
				Detail:    fmt.Sprintf("%s: %d 次部署 (%.0f%%)", ns.Namespace, ns.Updates, ns.Percent),
				Severity:  "medium",
			})
		}
		if ns.Updates > 30 {
			result.Hotspots[len(result.Hotspots)-1].Severity = "high"
		}
	}

	// Score
	if totalUpdates == 0 {
		result.HealthScore = 20
	} else if totalUpdates < 5 {
		result.HealthScore = 40
	} else if totalUpdates < 20 {
		result.HealthScore = 60
	} else if totalUpdates < 50 {
		result.HealthScore = 80
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildHeatmapRecs(&result)
	writeJSON(w, result)
}

func buildHeatmapRecs(r *DeployHeatmapResult) []string {
	recs := []string{
		fmt.Sprintf("近7天部署: %d 次, %d 个命名空间活跃", r.Summary.TotalUpdates, r.Summary.ActiveNS),
	}
	if r.Summary.LullDetected {
		recs = append(recs, "近7天无部署活动，集群处于停滞状态")
	}
	if r.Summary.PeakHour > 0 {
		recs = append(recs, fmt.Sprintf("部署高峰: %02d:00, %s", r.Summary.PeakHour, r.Summary.PeakWeekday))
	}
	for _, hs := range r.Hotspots {
		if hs.Severity == "high" {
			recs = append(recs, fmt.Sprintf("高频部署命名空间: %s", hs.Detail))
		}
	}
	return recs
}
