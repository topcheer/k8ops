package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChangeIntelResult analyzes recent cluster changes (deployments, config updates,
// scaling events) and correlates them with health signals to identify change-induced
// degradation. Provides blast radius analysis for each change.
type ChangeIntelResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ChangeIntelSummary  `json:"summary"`
	RecentChanges   []RecentChange      `json:"recentChanges"`
	RiskyChanges    []RiskyChange       `json:"riskyChanges"`
	ByHour          []HourlyChangeRate  `json:"changeVelocity"`
	ChangeFreeze    ChangeFreezeStatus  `json:"changeFreeze"`
	Recommendations []string            `json:"recommendations"`
}

// ChangeIntelSummary aggregates change statistics.
type ChangeIntelSummary struct {
	TotalChanges      int     `json:"totalChanges"`      // changes in last 24h
	DeploymentChanges int     `json:"deploymentChanges"`
	ConfigChanges     int     `json:"configChanges"`
	ScalingEvents     int     `json:"scalingEvents"`
	RiskyChangeCount  int     `json:"riskyChangeCount"`  // changes correlated with health issues
	AvgChangesPerHour float64 `json:"avgChangesPerHour"`
	ChangeStability   float64 `json:"changeStability"`   // % of changes without issues
	LargestBlastRadius int    `json:"largestBlastRadius"` // max pods affected by single change
}

// RecentChange describes one cluster change event.
type RecentChange struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Kind         string    `json:"kind"` // deployment-update, config-change, scaling
	Age          string    `json:"age"`
	Timestamp    time.Time `json:"timestamp"`
	BlastRadius  int       `json:"blastRadius"` // estimated pods affected
	Replicas     int32     `json:"replicas"`
	HealthImpact string    `json:"healthImpact"` // none, low, moderate, high
	RiskScore    int       `json:"riskScore"`
}

// RiskyChange is a change correlated with health degradation.
type RiskyChange struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	ChangeType   string `json:"changeType"`
	Age          string `json:"age"`
	BlastRadius  int    `json:"blastRadius"`
	Issue        string `json:"issue"`
	Severity     string `json:"severity"`
}

// HourlyChangeRate shows change velocity over 24 hours.
type HourlyChangeRate struct {
	Hour       string `json:"hour"`       // HH:00
	HourOffset int    `json:"hourOffset"` // -23 to 0
	ChangeCount int   `json:"changeCount"`
}

// ChangeFreezeStatus indicates if a change freeze should be active.
type ChangeFreezeStatus struct {
	Recommended     bool   `json:"recommended"`
	Reason          string `json:"reason"`
	Window          string `json:"window"` // e.g., "next 2 hours"
	RiskLevel       string `json:"riskLevel"`
}

// handleChangeIntel provides change intelligence & blast radius analysis.
// GET /api/operations/change-intel
func (s *Server) handleChangeIntel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChangeIntelResult{ScannedAt: time.Now()}
	now := time.Now()
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deploys, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Build pod-per-namespace map
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		nsPodCount[pod.Namespace]++
	}

	// Build pod-restart map per namespace
	nsRestarts := make(map[string]int)
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			nsRestarts[pod.Namespace] += int(cs.RestartCount)
		}
	}

	// 24-hour buckets for change velocity
	hourlyBuckets := make([]int, 24) // index 0 = 23h ago, 23 = current hour

	totalChanges := 0

	for _, dep := range deploys.Items {
		if systemNS[dep.Namespace] {
			continue
		}

		// Determine last update time from conditions
		var lastUpdate time.Time
		for _, cond := range dep.Status.Conditions {
			if cond.LastUpdateTime.After(lastUpdate) {
				lastUpdate = cond.LastUpdateTime.Time
			}
		}
		if lastUpdate.IsZero() && dep.CreationTimestamp.Time.After(time.Time{}) {
			lastUpdate = dep.CreationTimestamp.Time
		}

		if lastUpdate.IsZero() {
			continue
		}

		age := now.Sub(lastUpdate)
		if age > 24*time.Hour {
			continue
		}

		// This is a recent change
		hourBucket := int((24*time.Hour - age) / time.Hour)
		if hourBucket >= 0 && hourBucket < 24 {
			hourlyBuckets[hourBucket]++
		}

		replicas := int32(0)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		// Blast radius = replicas in this deployment + namespace pod count
		blastRadius := int(replicas)
		if blastRadius == 0 {
			blastRadius = nsPodCount[dep.Namespace]
		}

		// Health impact: check restarts in this namespace
		nsRest := nsRestarts[dep.Namespace]
		healthImpact := "none"
		riskScore := 0

		if nsRest > 20 {
			healthImpact = "high"
			riskScore = 80
		} else if nsRest > 10 {
			healthImpact = "moderate"
			riskScore = 50
		} else if nsRest > 3 {
			healthImpact = "low"
			riskScore = 25
		}

		change := RecentChange{
			Name:         dep.Name,
			Namespace:    dep.Namespace,
			Kind:         "deployment-update",
			Age:          formatDuration(age),
			Timestamp:    lastUpdate,
			BlastRadius:  blastRadius,
			Replicas:     replicas,
			HealthImpact: healthImpact,
			RiskScore:    riskScore,
		}

		result.RecentChanges = append(result.RecentChanges, change)
		totalChanges++
		result.Summary.DeploymentChanges++

		// Track risky changes
		if healthImpact == "high" || healthImpact == "moderate" {
			severity := "medium"
			if healthImpact == "high" {
				severity = "high"
			}
			result.RiskyChanges = append(result.RiskyChanges, RiskyChange{
				Name:        dep.Name,
				Namespace:   dep.Namespace,
				ChangeType:  "deployment-update",
				Age:         formatDuration(age),
				BlastRadius: blastRadius,
				Issue:       fmt.Sprintf("Recent deployment in namespace with %d pod restarts", nsRest),
				Severity:    severity,
			})
			result.Summary.RiskyChangeCount++
		}

		if blastRadius > result.Summary.LargestBlastRadius {
			result.Summary.LargestBlastRadius = blastRadius
		}
	}

	// ConfigMap changes
	for _, cm := range configmaps.Items {
		if systemNS[cm.Namespace] {
			continue
		}
		age := now.Sub(cm.CreationTimestamp.Time)
		if age > 24*time.Hour || age < 0 {
			continue
		}
		totalChanges++
		result.Summary.ConfigChanges++
		hourBucket := int((24*time.Hour - age) / time.Hour)
		if hourBucket >= 0 && hourBucket < 24 {
			hourlyBuckets[hourBucket]++
		}
	}

	// Build hourly change rate
	for i := 0; i < 24; i++ {
		hourTime := now.Add(time.Duration(i-23) * time.Hour)
		result.ByHour = append(result.ByHour, HourlyChangeRate{
			Hour:        hourTime.Format("15:00"),
			HourOffset:  i - 23,
			ChangeCount: hourlyBuckets[i],
		})
	}

	// Summary
	result.Summary.TotalChanges = totalChanges
	if totalChanges > 0 {
		result.Summary.AvgChangesPerHour = float64(totalChanges) / 24.0
		result.Summary.ChangeStability = float64(totalChanges-result.Summary.RiskyChangeCount) / float64(totalChanges) * 100
	}

	// Sort recent changes by timestamp (most recent first)
	sort.Slice(result.RecentChanges, func(i, j int) bool {
		return result.RecentChanges[i].Timestamp.After(result.RecentChanges[j].Timestamp)
	})
	if len(result.RecentChanges) > 30 {
		result.RecentChanges = result.RecentChanges[:30]
	}

	// Sort risky changes by blast radius
	sort.Slice(result.RiskyChanges, func(i, j int) bool {
		return result.RiskyChanges[i].BlastRadius > result.RiskyChanges[j].BlastRadius
	})
	if len(result.RiskyChanges) > 20 {
		result.RiskyChanges = result.RiskyChanges[:20]
	}

	// Change freeze recommendation
	result.ChangeFreeze = ChangeFreezeStatus{Recommended: false, RiskLevel: "low"}
	if result.Summary.RiskyChangeCount > 3 {
		result.ChangeFreeze = ChangeFreezeStatus{
			Recommended: true,
			Reason:      fmt.Sprintf("%d risky changes detected — recent changes correlated with health issues", result.Summary.RiskyChangeCount),
			Window:      "until restart counts stabilize",
			RiskLevel:   "high",
		}
	} else if result.Summary.AvgChangesPerHour > 5 {
		result.ChangeFreeze = ChangeFreezeStatus{
			Recommended: true,
			Reason:      fmt.Sprintf("High change velocity (%.1f/hour) increases risk of cascading failures", result.Summary.AvgChangesPerHour),
			Window:      "next 2 hours",
			RiskLevel:   "medium",
		}
	}

	result.Recommendations = generateChangeIntelRecs(result)

	writeJSON(w, result)
}

// generateChangeIntelRecs produces actionable recommendations.
func generateChangeIntelRecs(result ChangeIntelResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("%d changes in last 24h (%.1f/hour), %d risky — stability %.0f%%",
		result.Summary.TotalChanges, result.Summary.AvgChangesPerHour,
		result.Summary.RiskyChangeCount, result.Summary.ChangeStability))

	if result.Summary.RiskyChangeCount > 0 {
		recs = append(recs, fmt.Sprintf("%d changes correlated with health degradation — investigate recent deployments in affected namespaces", result.Summary.RiskyChangeCount))
	}

	if result.ChangeFreeze.Recommended {
		recs = append(recs, fmt.Sprintf("CHANGE FREEZE RECOMMENDED: %s (risk: %s)", result.ChangeFreeze.Reason, result.ChangeFreeze.RiskLevel))
	}

	if result.Summary.LargestBlastRadius > 10 {
		recs = append(recs, fmt.Sprintf("Largest blast radius: %d pods — consider canary deployments to reduce change impact", result.Summary.LargestBlastRadius))
	}

	if result.Summary.ChangeStability < 50 {
		recs = append(recs, fmt.Sprintf("Change stability is %.0f%% — improve pre-deployment testing and rollback procedures", result.Summary.ChangeStability))
	}

	if len(recs) == 1 {
		recs = append(recs, "Change patterns are stable — no action needed")
	}

	return recs
}

// Suppress unused imports
var _ appsv1.Deployment
var _ strings.Builder
