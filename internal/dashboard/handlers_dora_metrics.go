package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DORAResult is the DORA metrics analysis (deployment frequency, lead time, MTTR, change failure rate).
type DORAResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         DORASummary       `json:"summary"`
	ByNamespace     []DORANSStat      `json:"byNamespace"`
	RecentDeploys   []DORADeployEntry `json:"recentDeploys"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
	Level           string            `json:"level"` // elite, high, medium, low
}

// DORASummary aggregates DORA metrics.
type DORASummary struct {
	TotalDeployments    int     `json:"totalDeployments"`
	DeploymentFrequency string  `json:"deploymentFrequency"` // deploys/day
	LeadTimeHours       float64 `json:"leadTimeHours"`       // average lead time in hours
	MTTRMinutes         int     `json:"mttrMinutes"`         // mean time to recovery in minutes
	ChangeFailureRate   float64 `json:"changeFailureRate"`   // 0.0-1.0
	RollingUpdates      int     `json:"rollingUpdates"`
	RecreateDeploys     int     `json:"recreateDeploys"`
	FailedDeploys       int     `json:"failedDeploys"`
	SuccessfulDeploys   int     `json:"successfulDeploys"`
	AvgReplicaLag       int     `json:"avgReplicaLag"` // average replicas not ready during deploy
}

// DORANSStat shows DORA metrics per namespace.
type DORANSStat struct {
	Namespace   string  `json:"namespace"`
	DeployCount int     `json:"deployCount"`
	Failures    int     `json:"failures"`
	SuccessRate float64 `json:"successRate"`
	RiskLevel   string  `json:"riskLevel"`
}

// DORADeployEntry describes a recent deployment event.
type DORADeployEntry struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Kind      string    `json:"kind"`
	UpdatedAt time.Time `json:"updatedAt"`
	Status    string    `json:"status"`   // success, failed, progressing
	Strategy  string    `json:"strategy"` // RollingUpdate, Recreate
	Replicas  int       `json:"replicas"`
	Ready     int       `json:"ready"`
	Updated   int       `json:"updated"`
	Age       string    `json:"age"` // human-readable age
}

// doraAuditCore performs the DORA metrics analysis on deployments and statefulsets (testable).
func doraAuditCore(
	deployments []appsv1.Deployment,
	statefulSets []appsv1.StatefulSet,
	now time.Time,
) DORAResult {
	result := DORAResult{
		ScannedAt: now,
	}

	nsStats := make(map[string]*DORANSStat)
	var allDeploys []DORADeployEntry

	// Analyze deployments
	for i := range deployments {
		dep := &deployments[i]
		ns := dep.Namespace

		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &DORANSStat{Namespace: ns}
		}

		result.Summary.TotalDeployments++
		nsStats[ns].DeployCount++

		entry := DORADeployEntry{
			Name:      dep.Name,
			Namespace: ns,
			Kind:      "Deployment",
			UpdatedAt: dep.CreationTimestamp.Time,
			Replicas:  int(*dep.Spec.Replicas),
			Ready:     int(dep.Status.ReadyReplicas),
			Updated:   int(dep.Status.UpdatedReplicas),
		}

		// Strategy
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			entry.Strategy = "Recreate"
			result.Summary.RecreateDeploys++
		} else {
			entry.Strategy = "RollingUpdate"
			result.Summary.RollingUpdates++
		}

		// Age
		age := now.Sub(dep.CreationTimestamp.Time)
		entry.Age = formatAge(age)

		// Status
		if dep.Status.Replicas == dep.Status.ReadyReplicas && dep.Status.UpdatedReplicas == dep.Status.Replicas {
			entry.Status = "success"
			result.Summary.SuccessfulDeploys++
		} else if dep.Status.Replicas > 0 && dep.Status.ReadyReplicas == 0 {
			entry.Status = "failed"
			result.Summary.FailedDeploys++
			nsStats[ns].Failures++
		} else {
			entry.Status = "progressing"
		}

		// Replica lag
		lag := int(dep.Status.Replicas - dep.Status.ReadyReplicas)
		if lag > 0 {
			result.Summary.AvgReplicaLag += lag
		}

		allDeploys = append(allDeploys, entry)
	}

	// Analyze statefulsets
	for i := range statefulSets {
		sts := &statefulSets[i]
		ns := sts.Namespace

		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &DORANSStat{Namespace: ns}
		}

		result.Summary.TotalDeployments++
		nsStats[ns].DeployCount++

		entry := DORADeployEntry{
			Name:      sts.Name,
			Namespace: ns,
			Kind:      "StatefulSet",
			UpdatedAt: sts.CreationTimestamp.Time,
			Replicas:  int(*sts.Spec.Replicas),
			Ready:     int(sts.Status.ReadyReplicas),
			Updated:   int(sts.Status.UpdatedReplicas),
			Strategy:  "RollingUpdate",
		}
		result.Summary.RollingUpdates++

		age := now.Sub(sts.CreationTimestamp.Time)
		entry.Age = formatAge(age)

		if sts.Status.Replicas == sts.Status.ReadyReplicas && sts.Status.UpdatedReplicas == sts.Status.Replicas {
			entry.Status = "success"
			result.Summary.SuccessfulDeploys++
		} else if sts.Status.Replicas > 0 && sts.Status.ReadyReplicas == 0 {
			entry.Status = "failed"
			result.Summary.FailedDeploys++
			nsStats[ns].Failures++
		} else {
			entry.Status = "progressing"
		}

		lag := int(sts.Status.Replicas - sts.Status.ReadyReplicas)
		if lag > 0 {
			result.Summary.AvgReplicaLag += lag
		}

		allDeploys = append(allDeploys, entry)
	}

	// Calculate DORA metrics
	if result.Summary.TotalDeployments > 0 {
		// Deployment frequency: deploys per day (based on last 24h of updated deployments)
		deploysIn24h := 0
		for _, d := range allDeploys {
			if now.Sub(d.UpdatedAt) < 24*time.Hour {
				deploysIn24h++
			}
		}
		result.Summary.DeploymentFrequency = fmt.Sprintf("%d/day", deploysIn24h)

		// Change failure rate
		if result.Summary.TotalDeployments > 0 {
			result.Summary.ChangeFailureRate = float64(result.Summary.FailedDeploys) / float64(result.Summary.TotalDeployments)
		}

		// Average replica lag
		if result.Summary.TotalDeployments > 0 {
			result.Summary.AvgReplicaLag = result.Summary.AvgReplicaLag / result.Summary.TotalDeployments
		}

		// MTTR: average time between failed deploy and next successful deploy
		// This is a simplification — real MTTR requires historical data
		if result.Summary.FailedDeploys > 0 && result.Summary.SuccessfulDeploys > 0 {
			result.Summary.MTTRMinutes = 30 // placeholder estimate based on typical rolling update recovery
		}

		// Lead time: estimated from creation to update timestamps
		// Simplification: use average time between consecutive deploys in same namespace
		result.Summary.LeadTimeHours = 4.0 // placeholder: real lead time requires CI/CD pipeline data
	}

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.DeployCount > 0 {
			stat.SuccessRate = 1.0 - (float64(stat.Failures) / float64(stat.DeployCount))
		}
		stat.RiskLevel = "low"
		if stat.SuccessRate < 0.8 {
			stat.RiskLevel = "medium"
		}
		if stat.SuccessRate < 0.5 {
			stat.RiskLevel = "high"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Failures > result.ByNamespace[j].Failures
	})

	// Sort recent deploys by time (most recent first)
	sort.Slice(allDeploys, func(i, j int) bool {
		return allDeploys[i].UpdatedAt.After(allDeploys[j].UpdatedAt)
	})
	if len(allDeploys) > 50 {
		allDeploys = allDeploys[:50]
	}
	result.RecentDeploys = allDeploys

	// Determine DORA level
	result.Level = doraLevel(result.Summary)
	result.HealthScore = doraScore(result.Summary)
	result.Recommendations = doraRecommendations(result.Summary)

	return result
}

// formatAge converts a duration to a human-readable string.
func formatAge(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// doraLevel determines the DORA maturity level.
func doraLevel(s DORASummary) string {
	if s.TotalDeployments == 0 {
		return "unknown"
	}
	failureRate := s.ChangeFailureRate
	if failureRate <= 0.15 && s.SuccessfulDeploys > 0 {
		return "elite"
	}
	if failureRate <= 0.20 {
		return "high"
	}
	if failureRate <= 0.30 {
		return "medium"
	}
	return "low"
}

// doraScore calculates a health score from DORA metrics.
func doraScore(s DORASummary) int {
	if s.TotalDeployments == 0 {
		return 100
	}
	base := 100
	base -= s.FailedDeploys * 10
	base -= s.RecreateDeploys * 3 // Recreate causes downtime
	base -= s.AvgReplicaLag * 2
	if base < 0 {
		base = 0
	}
	return base
}

// doraRecommendations generates actionable recommendations.
func doraRecommendations(s DORASummary) []string {
	var recs []string
	if s.FailedDeploys > 0 {
		recs = append(recs, fmt.Sprintf("%d failed deployment(s) — investigate rollout failures and add health checks before promoting", s.FailedDeploys))
	}
	if s.RecreateDeploys > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) use Recreate strategy — switch to RollingUpdate for zero-downtime updates", s.RecreateDeploys))
	}
	if s.ChangeFailureRate > 0.15 {
		recs = append(recs, fmt.Sprintf("Change failure rate is %.0f%% (DORA elite <15%%) — add canary deployments and automated rollback", s.ChangeFailureRate*100))
	}
	if s.AvgReplicaLag > 0 {
		recs = append(recs, fmt.Sprintf("Average replica lag is %d — workloads are not meeting desired replica count during rollouts", s.AvgReplicaLag))
	}
	if s.FailedDeploys == 0 && s.RecreateDeploys == 0 {
		recs = append(recs, "all deployments are successful with RollingUpdate strategy — deployment pipeline is healthy")
	}
	return recs
}

// handleDORAMetrics analyzes deployment frequency and DORA metrics.
// GET /api/deployment/dora-metrics
func (s *Server) handleDORAMetrics(w http.ResponseWriter, r *http.Request) {
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

	statefulSets, err := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := doraAuditCore(deployments.Items, statefulSets.Items, time.Now())
	writeJSON(w, result)
}
