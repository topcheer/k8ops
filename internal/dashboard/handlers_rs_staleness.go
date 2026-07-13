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

// RSStalenessResult is the ReplicaSet staleness & rollout history analysis.
type RSStalenessResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          RSSummary           `json:"summary"`
	ByNamespace      []RSNSStat          `json:"byNamespace"`
	StaleReplicaSets []RSEntry           `json:"staleReplicaSets"`
	Deployments      []RSDeploymentEntry `json:"deployments"`
	Issues           []RSIssue           `json:"issues"`
	Recommendations  []string            `json:"recommendations"`
	HealthScore      int                 `json:"healthScore"`
}

// RSSummary aggregates ReplicaSet staleness statistics.
type RSSummary struct {
	TotalDeployments    int `json:"totalDeployments"`
	TotalReplicaSets    int `json:"totalReplicaSets"`
	ActiveReplicaSets   int `json:"activeReplicaSets"`
	StaleReplicaSets    int `json:"staleReplicaSets"`
	NoRevisionHistory   int `json:"noRevisionHistoryLimit"`
	LowRevisionHistory  int `json:"lowRevisionHistoryLimit"`
	HighRevisionHistory int `json:"highRevisionHistoryCount"`
}

// RSNSStat per-namespace stats.
type RSNSStat struct {
	Namespace       string `json:"namespace"`
	DeploymentCount int    `json:"deploymentCount"`
	StaleRSCount    int    `json:"staleRSCount"`
}

// RSEntry describes one stale ReplicaSet.
type RSEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	OwnerDeployment string `json:"ownerDeployment"`
	Replicas        int    `json:"replicas"`
	Age             string `json:"age"`
	Revision        string `json:"revision"`
	RiskLevel       string `json:"riskLevel"`
}

// RSDeploymentEntry describes one deployment's rollout history posture.
type RSDeploymentEntry struct {
	Name                 string `json:"name"`
	Namespace            string `json:"namespace"`
	RevisionHistoryLimit int    `json:"revisionHistoryLimit"`
	ReplicaSetCount      int    `json:"replicaSetCount"`
	ActiveRS             string `json:"activeRS"`
	StaleRSCount         int    `json:"staleRSCount"`
	RiskLevel            string `json:"riskLevel"`
}

// RSIssue is a detected rollout history problem.
type RSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleRSStaleness audits ReplicaSet staleness and rollout history.
// GET /api/deployment/rs-staleness
func (s *Server) handleRSStaleness(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &RSStalenessResult{
		ScannedAt: time.Now(),
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	replicaSets, err := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build RS map: namespace/ownerDeployment -> []ReplicaSet
	rsMap := make(map[string][]appsv1.ReplicaSet)
	for i := range replicaSets.Items {
		rs := &replicaSets.Items[i]
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				key := fmt.Sprintf("%s/%s", rs.Namespace, owner.Name)
				rsMap[key] = append(rsMap[key], *rs)
				break
			}
		}
	}

	var staleRS []RSEntry
	var depEntries []RSDeploymentEntry
	var issues []RSIssue
	nsStats := make(map[string]*RSNSStat)

	noRevisionHistory := 0
	lowRevisionHistory := 0
	highRevisionCount := 0
	totalRS := 0
	activeRS := 0
	staleCount := 0

	for i := range deployments.Items {
		dep := &deployments.Items[i]
		if isSystemNamespace(dep.Namespace) {
			continue
		}

		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		depRS := rsMap[key]

		// Revision history limit
		revHistoryLimit := 10 // default
		if dep.Spec.RevisionHistoryLimit != nil {
			revHistoryLimit = int(*dep.Spec.RevisionHistoryLimit)
		} else {
			noRevisionHistory++
		}

		if revHistoryLimit < 2 {
			lowRevisionHistory++
			issues = append(issues, RSIssue{
				Severity: "warning",
				Type:     "low-revision-history",
				Resource: key,
				Message:  fmt.Sprintf("revisionHistoryLimit=%d — too few revisions for rollback capability", revHistoryLimit),
			})
		}

		depEntry := RSDeploymentEntry{
			Name:                 dep.Name,
			Namespace:            dep.Namespace,
			RevisionHistoryLimit: revHistoryLimit,
			ReplicaSetCount:      len(depRS),
		}

		// Find active and stale ReplicaSets
		depStaleCount := 0
		for _, rs := range depRS {
			totalRS++
			isActive := false
			if rs.Annotations != nil {
				if rev, ok := rs.Annotations["deployment.kubernetes.io/revision"]; ok {
					_ = rev
				}
			}
			// Active RS has replicas > 0
			if rs.Spec.Replicas != nil && *rs.Spec.Replicas > 0 {
				isActive = true
				activeRS++
				depEntry.ActiveRS = rs.Name
			}

			if !isActive {
				staleCount++
				depStaleCount++
				age := time.Since(rs.CreationTimestamp.Time).Round(time.Hour * 24).String()

				revision := ""
				if rs.Annotations != nil {
					if rev, ok := rs.Annotations["deployment.kubernetes.io/revision"]; ok {
						revision = rev
					}
				}

				riskLevel := "info"
				if depStaleCount > 5 {
					riskLevel = "warning"
				}

				staleRS = append(staleRS, RSEntry{
					Name:            rs.Name,
					Namespace:       rs.Namespace,
					OwnerDeployment: dep.Name,
					Replicas:        0,
					Age:             age,
					Revision:        revision,
					RiskLevel:       riskLevel,
				})

				// Stale RS older than 30 days
				if time.Since(rs.CreationTimestamp.Time) > 30*24*time.Hour {
					issues = append(issues, RSIssue{
						Severity: "info",
						Type:     "old-stale-rs",
						Resource: fmt.Sprintf("%s/%s", rs.Namespace, rs.Name),
						Message:  fmt.Sprintf("Stale ReplicaSet from deployment '%s' is %s old — consuming etcd storage", dep.Name, age),
					})
				}
			}
		}

		depEntry.StaleRSCount = depStaleCount
		depEntry.RiskLevel = assessRSRisk(depEntry)

		if len(depRS) > revHistoryLimit+2 {
			highRevisionCount++
			issues = append(issues, RSIssue{
				Severity: "warning",
				Type:     "excess-revisions",
				Resource: key,
				Message:  fmt.Sprintf("Deployment has %d ReplicaSets but revisionHistoryLimit=%d — old ReplicaSets should be garbage collected", len(depRS), revHistoryLimit),
			})
		}

		depEntries = append(depEntries, depEntry)

		// Namespace stats
		if _, ok := nsStats[dep.Namespace]; !ok {
			nsStats[dep.Namespace] = &RSNSStat{Namespace: dep.Namespace}
		}
		nsStats[dep.Namespace].DeploymentCount++
		nsStats[dep.Namespace].StaleRSCount += depStaleCount
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].StaleRSCount > result.ByNamespace[j].StaleRSCount
	})

	sort.Slice(staleRS, func(i, j int) bool {
		return staleRS[i].RiskLevel > staleRS[j].RiskLevel
	})
	if len(staleRS) > 50 {
		staleRS = staleRS[:50]
	}

	// Recommendations
	var recommendations []string
	if noRevisionHistory > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) have no explicit revisionHistoryLimit — set it to control rollback history retention", noRevisionHistory))
	}
	if lowRevisionHistory > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) have very low revisionHistoryLimit — increase to at least 3-5 for rollback capability", lowRevisionHistory))
	}
	if highRevisionCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) have excess ReplicaSets beyond revisionHistoryLimit — check garbage collection settings", highRevisionCount))
	}
	if staleCount > 20 {
		recommendations = append(recommendations, fmt.Sprintf("%d stale ReplicaSets found — consider reducing revisionHistoryLimit to save etcd storage", staleCount))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Rollout history is well-managed — ReplicaSet retention is appropriate")
	}

	result.StaleReplicaSets = staleRS
	result.Deployments = depEntries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = RSSummary{
		TotalDeployments:    len(depEntries),
		TotalReplicaSets:    totalRS,
		ActiveReplicaSets:   activeRS,
		StaleReplicaSets:    staleCount,
		NoRevisionHistory:   noRevisionHistory,
		LowRevisionHistory:  lowRevisionHistory,
		HighRevisionHistory: highRevisionCount,
	}
	result.HealthScore = computeRSStalenessScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessRSRisk determines risk level.
func assessRSRisk(entry RSDeploymentEntry) string {
	risk := 0
	if entry.RevisionHistoryLimit < 2 {
		risk += 2
	}
	if entry.StaleRSCount > 10 {
		risk += 2
	}
	if entry.ReplicaSetCount > entry.RevisionHistoryLimit+2 {
		risk += 1
	}
	switch {
	case risk >= 3:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computeRSStalenessScore computes a 0-100 health score.
func computeRSStalenessScore(s RSSummary, issueCount int) int {
	if s.TotalDeployments == 0 {
		return 100
	}
	score := 100
	score -= s.LowRevisionHistory * 5
	score -= s.HighRevisionHistory * 3
	score -= s.NoRevisionHistory * 2
	if s.StaleReplicaSets > 50 {
		score -= 5
	}
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
