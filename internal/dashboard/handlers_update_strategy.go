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

// USResult is the deployment update strategy & rollback readiness analysis.
type USResult struct {
	ScannedAt          time.Time `json:"scannedAt"`
	Summary            USSummary `json:"summary"`
	ByWorkload         []USEntry `json:"byWorkload"`
	RecreateStrat      []USEntry `json:"recreateStrategy"`   // Recreate = downtime
	NoRevHistory       []USEntry `json:"noRevisionHistory"`  // revisionHistoryLimit too low
	NoProgressDeadline []USEntry `json:"noProgressDeadline"` // no progressDeadlineSeconds
	BadMaxSurge        []USEntry `json:"badMaxSurge"`        // maxSurge=0 or maxUnavailable=100%
	Issues             []USIssue `json:"issues"`
	Recommendations    []string  `json:"recommendations"`
}

// USSummary aggregates update strategy statistics.
type USSummary struct {
	TotalWorkloads      int `json:"totalWorkloads"`
	RollingUpdate       int `json:"rollingUpdate"`
	Recreate            int `json:"recreate"` // Recreate = downtime on deploy
	HasProgressDeadline int `json:"hasProgressDeadline"`
	NoProgressDeadline  int `json:"noProgressDeadline"`
	RevHistoryLow       int `json:"revHistoryLow"`     // revisionHistoryLimit < 3
	RevHistoryDefault   int `json:"revHistoryDefault"` // revisionHistoryLimit = 10 (default)
	RevHistoryHigh      int `json:"revHistoryHigh"`    // > 10
	MaxSurgeZero        int `json:"maxSurgeZero"`      // maxSurge=0 (can't add before removing)
	MaxUnavailable100   int `json:"maxUnavailable100"` // maxUnavailable=100% (removes all at once)
	ReadinessScore      int `json:"readinessScore"`    // 0-100
}

// USEntry describes one deployment's update strategy.
type USEntry struct {
	Name                 string   `json:"name"`
	Namespace            string   `json:"namespace"`
	Strategy             string   `json:"strategy"` // RollingUpdate / Recreate
	MaxSurge             string   `json:"maxSurge,omitempty"`
	MaxUnavailable       string   `json:"maxUnavailable,omitempty"`
	RevisionHistoryLimit *int32   `json:"revisionHistoryLimit,omitempty"`
	ProgressDeadline     *int32   `json:"progressDeadlineSeconds,omitempty"`
	Replicas             int32    `json:"replicas"`
	Violations           []string `json:"violations,omitempty"`
	RiskLevel            string   `json:"riskLevel"`
}

// USIssue is a detected update strategy problem.
type USIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleUpdateStrategy audits deployment update strategy and rollback readiness.
// GET /api/deployment/update-strategy
func (s *Server) handleUpdateStrategy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := USResult{ScannedAt: time.Now()}

	for _, dep := range deployments.Items {
		result.Summary.TotalWorkloads++

		entry := USEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Strategy:  string(dep.Spec.Strategy.Type),
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.Replicas = replicas

		// Revision history limit
		if dep.Spec.RevisionHistoryLimit != nil {
			entry.RevisionHistoryLimit = dep.Spec.RevisionHistoryLimit
			limit := *dep.Spec.RevisionHistoryLimit
			if limit < 3 {
				result.Summary.RevHistoryLow++
				entry.Violations = append(entry.Violations, fmt.Sprintf("revisionHistoryLimit=%d — too few revisions for safe rollback", limit))
				result.NoRevHistory = append(result.NoRevHistory, entry)
				result.Issues = append(result.Issues, USIssue{
					Severity: "warning", Type: "low-rev-history",
					Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
					Message:  fmt.Sprintf("Deployment %s/%s has revisionHistoryLimit=%d — insufficient rollback history, set to 5-10", dep.Namespace, dep.Name, limit),
				})
			} else if limit == 10 {
				result.Summary.RevHistoryDefault++
			} else if limit > 10 {
				result.Summary.RevHistoryHigh++
			}
		} else {
			// Default is 10 per Kubernetes spec
			result.Summary.RevHistoryDefault++
		}

		// Progress deadline
		if dep.Spec.ProgressDeadlineSeconds != nil {
			entry.ProgressDeadline = dep.Spec.ProgressDeadlineSeconds
			result.Summary.HasProgressDeadline++
		} else {
			result.Summary.NoProgressDeadline++
			entry.Violations = append(entry.Violations, "No progressDeadlineSeconds — failed deploys may hang indefinitely")
			result.NoProgressDeadline = append(result.NoProgressDeadline, entry)
		}

		// Strategy analysis
		switch dep.Spec.Strategy.Type {
		case appsv1.RecreateDeploymentStrategyType:
			result.Summary.Recreate++
			entry.Violations = append(entry.Violations, "Recreate strategy — all pods killed before new ones start, causes downtime")
			result.RecreateStrat = append(result.RecreateStrat, entry)
			result.Issues = append(result.Issues, USIssue{
				Severity: "critical", Type: "recreate-strategy",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s uses Recreate strategy — causes downtime during updates, switch to RollingUpdate", dep.Namespace, dep.Name),
			})
		case appsv1.RollingUpdateDeploymentStrategyType:
			result.Summary.RollingUpdate++

			// Check rolling update params
			ru := dep.Spec.Strategy.RollingUpdate
			if ru != nil {
				if ru.MaxSurge != nil {
					entry.MaxSurge = ru.MaxSurge.String()
					// maxSurge=0 means no extra pods during update — slow and risky
					if ru.MaxSurge.IntVal == 0 && ru.MaxSurge.Type == 0 {
						result.Summary.MaxSurgeZero++
						entry.Violations = append(entry.Violations, "maxSurge=0 — no extra capacity during rollout, slower updates")
						result.BadMaxSurge = append(result.BadMaxSurge, entry)
					}
				}
				if ru.MaxUnavailable != nil {
					entry.MaxUnavailable = ru.MaxUnavailable.String()
					// maxUnavailable=100% means all pods can be unavailable at once
					s := ru.MaxUnavailable.String()
					if strings.Contains(s, "100%") {
						result.Summary.MaxUnavailable100++
						entry.Violations = append(entry.Violations, "maxUnavailable=100% — all pods can be down simultaneously during update")
						result.BadMaxSurge = append(result.BadMaxSurge, entry)
						result.Issues = append(result.Issues, USIssue{
							Severity: "warning", Type: "bad-max-unavailable",
							Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
							Message:  fmt.Sprintf("Deployment %s/%s has maxUnavailable=100%% — all pods down during rollout, defeats zero-downtime", dep.Namespace, dep.Name),
						})
					}
				}
			}
		default:
			entry.Violations = append(entry.Violations, fmt.Sprintf("Unknown strategy: %s", dep.Spec.Strategy.Type))
		}

		entry.RiskLevel = usAssessRisk(entry)
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return usRiskRank(result.ByWorkload[i].RiskLevel) < usRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return usIssueRank(result.Issues[i].Severity) < usIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ReadinessScore = usScore(result.Summary)
	result.Recommendations = usGenRecs(result.Summary, result.RecreateStrat, result.NoRevHistory)

	writeJSON(w, result)
}

// usAssessRisk determines risk level.
func usAssessRisk(entry USEntry) string {
	if entry.Strategy == "Recreate" {
		return "critical"
	}
	for _, v := range entry.Violations {
		if strings.Contains(v, "maxUnavailable=100%") {
			return "high"
		}
	}
	if len(entry.Violations) >= 2 {
		return "medium"
	}
	if len(entry.Violations) >= 1 {
		return "medium"
	}
	return "low"
}

// usScore computes 0-100.
func usScore(s USSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.Recreate * 15
	score -= s.MaxUnavailable100 * 10
	score -= s.MaxSurgeZero * 5
	score -= s.RevHistoryLow * 4
	score -= s.NoProgressDeadline * 3
	if score < 0 {
		score = 0
	}
	return score
}

// usGenRecs produces actionable advice.
func usGenRecs(s USSummary, recreate []USEntry, noRevHistory []USEntry) []string {
	var recs []string

	if s.Recreate > 0 {
		top := ""
		if len(recreate) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", recreate[0].Namespace, recreate[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d deployment(s) use Recreate strategy%s — switch to RollingUpdate for zero-downtime updates", s.Recreate, top))
	}
	if s.MaxUnavailable100 > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have maxUnavailable=100%% — reduce to 25%% or 1 for safe rolling updates", s.MaxUnavailable100))
	}
	if s.MaxSurgeZero > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have maxSurge=0 — set to 25%% or 1 to allow extra capacity during rollout", s.MaxSurgeZero))
	}
	if s.RevHistoryLow > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have low revisionHistoryLimit — set to 5-10 to ensure rollback capability", s.RevHistoryLow))
	}
	if s.NoProgressDeadline > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) lack progressDeadlineSeconds — set to 300-600s to auto-fail stuck rollouts", s.NoProgressDeadline))
	}
	if s.RevHistoryHigh > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have revisionHistoryLimit > 10 — reduces ReplicaSet clutter if lowered to 10", s.RevHistoryHigh))
	}
	if s.ReadinessScore < 70 {
		recs = append(recs, fmt.Sprintf("Deployment readiness score is %d/100 — update strategies need improvement", s.ReadinessScore))
	}
	if s.Recreate == 0 && s.MaxUnavailable100 == 0 && s.RevHistoryLow == 0 {
		recs = append(recs, "All deployments have safe update strategies — good rollout posture")
	}

	return recs
}

func usRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func usIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

var _ = appsv1.DeploymentSpec{}
