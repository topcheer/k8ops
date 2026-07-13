package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// SurgeRiskResult is the rolling update risk & surge configuration analysis.
type SurgeRiskResult struct {
	ScannedAt         time.Time        `json:"scannedAt"`
	Summary           SurgeRiskSummary `json:"summary"`
	ByWorkload        []SurgeRiskEntry `json:"byWorkload"`
	HighRiskWorkloads []SurgeRiskEntry `json:"highRiskWorkloads"`
	ByStrategy        []StrategyStat   `json:"byStrategy"`
	Recommendations   []string         `json:"recommendations"`
}

// SurgeRiskSummary aggregates surge risk statistics.
type SurgeRiskSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	MaxSurgeTooHigh   int `json:"maxSurgeTooHigh"`   // surge > 50% replicas
	MaxUnavailable100 int `json:"maxUnavailable100"` // maxUnavailable=100% (no availability guarantee)
	NoSurgeConfig     int `json:"noSurgeConfig"`     // default surge (25%)
	RollingStrategy   int `json:"rollingStrategy"`
	RecreateStrategy  int `json:"recreateStrategy"`
	HighRisk          int `json:"highRisk"`
	HealthScore       int `json:"healthScore"`
}

// SurgeRiskEntry describes one workload's surge configuration.
type SurgeRiskEntry struct {
	Name            string  `json:"name"`
	Namespace       string  `json:"namespace"`
	WorkloadType    string  `json:"workloadType"`
	Strategy        string  `json:"strategy"` // RollingUpdate, Recreate
	MaxSurge        string  `json:"maxSurge,omitempty"`
	MaxUnavailable  string  `json:"maxUnavailable,omitempty"`
	Replicas        int32   `json:"replicas"`
	MaxSurgeNum     int     `json:"maxSurgeNum,omitempty"`
	MaxUnavailNum   int     `json:"maxUnavailNum,omitempty"`
	MinAvailable    int     `json:"minAvailable,omitempty"`
	MinAvailablePct float64 `json:"minAvailablePct,omitempty"`
	RiskLevel       string  `json:"riskLevel"`
	Issue           string  `json:"issue,omitempty"`
}

// StrategyStat shows workload count by update strategy.
type StrategyStat struct {
	Strategy string `json:"strategy"`
	Count    int    `json:"count"`
}

// surgeRiskAuditCore performs the audit on deployments (testable).
func surgeRiskAuditCore(deployments []appsv1.Deployment) SurgeRiskResult {
	result := SurgeRiskResult{
		ScannedAt: time.Now(),
	}

	strategyCounts := make(map[string]int)

	for i := range deployments {
		d := &deployments[i]
		ns := d.Namespace
		_, wlType := podOwnerInfo(deploymentToPod(d))

		result.Summary.TotalWorkloads++

		strategy := string(d.Spec.Strategy.Type)
		strategyCounts[strategy]++

		entry := SurgeRiskEntry{
			Name:         d.Name,
			Namespace:    ns,
			WorkloadType: wlType,
			Strategy:     strategy,
		}

		if d.Spec.Replicas != nil {
			entry.Replicas = *d.Spec.Replicas
		}

		if d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
			result.Summary.RollingStrategy++
			ru := d.Spec.Strategy.RollingUpdate

			surgeNum := 1
			unavailNum := 1
			surgeStr := "25%"
			unavailStr := "25%"

			if ru != nil {
				if ru.MaxSurge != nil {
					surgeStr = ru.MaxSurge.String()
					surgeNum = intstrValue(ru.MaxSurge, int(entry.Replicas))
				}
				if ru.MaxUnavailable != nil {
					unavailStr = ru.MaxUnavailable.String()
					unavailNum = intstrValue(ru.MaxUnavailable, int(entry.Replicas))
				}
			}

			entry.MaxSurge = surgeStr
			entry.MaxUnavailable = unavailStr
			entry.MaxSurgeNum = surgeNum
			entry.MaxUnavailNum = unavailNum

			// Calculate minimum available pods during update
			minAvail := int(entry.Replicas) - unavailNum
			if minAvail < 0 {
				minAvail = 0
			}
			entry.MinAvailable = minAvail
			if entry.Replicas > 0 {
				entry.MinAvailablePct = float64(minAvail) / float64(entry.Replicas) * 100
			}

			risk := "low"
			var issue string

			// Check: maxUnavailable=100% means no availability guarantee
			if unavailNum >= int(entry.Replicas) && entry.Replicas > 0 {
				result.Summary.MaxUnavailable100++
				risk = "high"
				issue = "maxUnavailable=100% — all pods can be unavailable during update (downtime)"
			}

			// Check: maxSurge too high (>50% of replicas)
			if entry.Replicas > 0 && surgeNum > int(entry.Replicas)/2 {
				result.Summary.MaxSurgeTooHigh++
				if risk == "low" {
					risk = "medium"
					issue = fmt.Sprintf("maxSurge=%s is high (>50%% of %d replicas) — may strain resources during update", surgeStr, entry.Replicas)
				}
			}

			// Check: default surge (no explicit config)
			if ru == nil || (ru.MaxSurge == nil && ru.MaxUnavailable == nil) {
				result.Summary.NoSurgeConfig++
				if risk == "low" {
					risk = "low"
					issue = "using default surge config (25%/25%) — consider tuning for your workload"
				}
			}

			entry.RiskLevel = risk
			entry.Issue = issue

			if risk == "high" {
				result.Summary.HighRisk++
				result.HighRiskWorkloads = append(result.HighRiskWorkloads, entry)
			}
		} else if d.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			result.Summary.RecreateStrategy++
			entry.RiskLevel = "high"
			entry.Issue = "Recreate strategy — all pods are killed before new ones are created (guaranteed downtime)"
			result.Summary.HighRisk++
			result.HighRiskWorkloads = append(result.HighRiskWorkloads, entry)
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Build strategy stats
	for strat, count := range strategyCounts {
		result.ByStrategy = append(result.ByStrategy, StrategyStat{Strategy: strat, Count: count})
	}
	sort.Slice(result.ByStrategy, func(i, j int) bool {
		return result.ByStrategy[i].Count > result.ByStrategy[j].Count
	})

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		riskOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return riskOrder[result.ByWorkload[i].RiskLevel] < riskOrder[result.ByWorkload[j].RiskLevel]
	})

	result.Summary.HealthScore = surgeRiskScore(result.Summary)
	result.Recommendations = surgeRiskRecommendations(result.Summary)

	return result
}

// intstrValue converts intstr.IntOrString to int value based on total replicas.
func intstrValue(is *intstr.IntOrString, total int) int {
	if is == nil {
		return total / 4 // default 25%
	}
	if is.Type == intstr.Int {
		return is.IntValue()
	}
	// Percentage
	pct, err := intstr.GetValueFromIntOrPercent(is, total, true)
	if err != nil {
		return total / 4
	}
	return pct
}

// deploymentToPod converts a Deployment to a minimal Pod for podOwnerInfo.
func deploymentToPod(d *appsv1.Deployment) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            d.Name,
			Namespace:       d.Namespace,
			OwnerReferences: d.OwnerReferences,
		},
	}
}

// surgeRiskScore calculates health score.
func surgeRiskScore(s SurgeRiskSummary) int {
	base := 100
	base -= s.HighRisk * 15
	base -= s.MaxSurgeTooHigh * 5
	base -= s.RecreateStrategy * 8
	if base < 0 {
		base = 0
	}
	return base
}

// surgeRiskRecommendations generates recommendations.
func surgeRiskRecommendations(s SurgeRiskSummary) []string {
	var recs []string
	if s.MaxUnavailable100 > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads have maxUnavailable=100%% — set to 0 or 25%% to maintain availability during updates", s.MaxUnavailable100))
	}
	if s.RecreateStrategy > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads use Recreate strategy — switch to RollingUpdate for zero-downtime deployments", s.RecreateStrategy))
	}
	if s.MaxSurgeTooHigh > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads have maxSurge >50%% of replicas — reduce to 25%% to avoid resource strain", s.MaxSurgeTooHigh))
	}
	if s.NoSurgeConfig > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads use default surge config — tune maxSurge/maxUnavailable for your workload requirements", s.NoSurgeConfig))
	}
	if s.HighRisk == 0 {
		recs = append(recs, "rolling update configurations are well-tuned — no high-risk surge settings detected")
	}
	return recs
}

// handleSurgeRisk audits rolling update risk and surge configuration.
// GET /api/deployment/surge-risk
func (s *Server) handleSurgeRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := surgeRiskAuditCore(deployments.Items)
	writeJSON(w, result)
}

// Suppress unused import
var _ = strings.Contains
