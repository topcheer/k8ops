package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DeployFrequencyResult analyzes deployment rollout history to measure
// deployment frequency, success rate, and rollback patterns. This is a
// key DORA metric for platform engineering maturity.
type DeployFrequencyResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         DeployFreqSummary `json:"summary"`
	ByDay           []DeployDayStat   `json:"byDay"`
	ByNamespace     []DeployFreqNS    `json:"byNamespace"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type DeployFreqSummary struct {
	TotalDeployments  int     `json:"totalDeployments"`
	ActiveReplicaSets int     `json:"activeReplicaSets"`
	OldReplicaSets    int     `json:"oldReplicaSets"`
	RolloutRate       float64 `json:"rolloutRatePerDay"`
	AvgAge            string  `json:"avgAgeDays"`
	RecentlyUpdated   int     `json:"recentlyUpdated24h"`
	RecentlyUpdated7d int     `json:"recentlyUpdated7d"`
}

type DeployDayStat struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type DeployFreqNS struct {
	Namespace    string  `json:"namespace"`
	Count        int     `json:"deploymentCount"`
	Updated24h   int     `json:"updated24h"`
	RolloutScore float64 `json:"rolloutScore"`
}

// handleDeployFrequency handles GET /api/deployment/deploy-frequency
func (s *Server) handleDeployFrequency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployFrequencyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	replicaSets, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	nsMap := make(map[string]*DeployFreqNS)
	dayMap := make(map[string]int)
	totalAgeHours := 0.0

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		if _, ok := nsMap[d.Namespace]; !ok {
			nsMap[d.Namespace] = &DeployFreqNS{Namespace: d.Namespace}
		}
		nsMap[d.Namespace].Count++

		// Check last update time
		updateTime := d.Status.Conditions
		lastUpdate := d.CreationTimestamp.Time
		for _, cond := range updateTime {
			if cond.Type == appsv1.DeploymentProgressing && cond.LastUpdateTime.Time.After(lastUpdate) {
				lastUpdate = cond.LastUpdateTime.Time
			}
		}

		ageHours := now.Sub(d.CreationTimestamp.Time).Hours()
		totalAgeHours += ageHours

		if now.Sub(lastUpdate) < 24*time.Hour {
			result.Summary.RecentlyUpdated++
			nsMap[d.Namespace].Updated24h++
			dayKey := lastUpdate.Format("2006-01-02")
			dayMap[dayKey]++
		}
		if now.Sub(lastUpdate) < 7*24*time.Hour {
			result.Summary.RecentlyUpdated7d++
		}
	}

	// ReplicaSet stats
	for _, rs := range replicaSets.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		if *rs.Spec.Replicas > 0 {
			result.Summary.ActiveReplicaSets++
		} else {
			result.Summary.OldReplicaSets++
		}
	}

	// Avg age
	if result.Summary.TotalDeployments > 0 {
		avgDays := totalAgeHours / float64(result.Summary.TotalDeployments) / 24
		result.Summary.AvgAge = fmt.Sprintf("%.0fd", avgDays)
		result.Summary.RolloutRate = float64(result.Summary.RecentlyUpdated7d) / 7.0
	}

	// Day breakdown (last 14 days)
	for i := 13; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		result.ByDay = append(result.ByDay, DeployDayStat{
			Day: day, Count: dayMap[day],
		})
	}

	// NS breakdown
	for _, ns := range nsMap {
		if ns.Count > 0 {
			ns.RolloutScore = float64(ns.Updated24h) / float64(ns.Count) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Updated24h > result.ByNamespace[j].Updated24h
	})

	// Score: higher deployment frequency = better DORA score
	result.HealthScore = 0
	if result.Summary.RecentlyUpdated7d > 0 {
		result.HealthScore = 40 // Base for having any deploys
	}
	if result.Summary.RolloutRate >= 1 {
		result.HealthScore = 60 // Daily deploys
	}
	if result.Summary.RolloutRate >= 3 {
		result.HealthScore = 80 // Multiple per day
	}
	if result.Summary.RecentlyUpdated7d == 0 && result.Summary.TotalDeployments > 0 {
		result.HealthScore = 20 // Stale
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

	result.Recommendations = buildDeployFreqRecs(&result)
	writeJSON(w, result)
}

func buildDeployFreqRecs(r *DeployFrequencyResult) []string {
	recs := []string{
		fmt.Sprintf("部署频率: %.1f 次/天 (近7天 %d 次)", r.Summary.RolloutRate, r.Summary.RecentlyUpdated7d),
	}
	if r.Summary.RecentlyUpdated7d == 0 {
		recs = append(recs, "近 7 天无部署，工作负载可能处于停滞状态")
	}
	if r.Summary.OldReplicaSets > 20 {
		recs = append(recs, fmt.Sprintf("%d 个旧 ReplicaSet，建议设置 revisionHistoryLimit", r.Summary.OldReplicaSets))
	}
	if len(r.ByNamespace) > 0 && r.ByNamespace[0].Updated24h > 0 {
		recs = append(recs, fmt.Sprintf("最活跃命名空间: %s (%d 个部署, %d 个近期更新)",
			r.ByNamespace[0].Namespace, r.ByNamespace[0].Count, r.ByNamespace[0].Updated24h))
	}
	return recs
}
