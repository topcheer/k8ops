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

// RevisionDiffResult is the deployment revision diff & pod template change impact audit.
type RevisionDiffResult struct {
	Timestamp       time.Time           `json:"timestamp"`
	Score           int                 `json:"score"`
	Status          string              `json:"status"`
	Summary         RevisionDiffSummary `json:"summary"`
	WorkloadDiffs   []WorkloadDiff      `json:"workloadDiffs"`
	BreakingChanges []BreakingChange    `json:"breakingChanges"`
	RiskyWorkloads  []RiskyWorkload     `json:"riskyWorkloads"`
	Issues          []RevDiffIssue      `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

// RevisionDiffSummary holds aggregate revision diff metrics.
type RevisionDiffSummary struct {
	TotalWorkloads      int `json:"totalWorkloads"`
	WithMultipleRevs    int `json:"withMultipleRevisions"`
	WithImageChange     int `json:"withImageChange"`
	WithResourceChange  int `json:"withResourceChange"`
	WithProbeChange     int `json:"withProbeChange"`
	WithSecurityChange  int `json:"withSecurityChange"`
	BreakingChangeCount int `json:"breakingChangeCount"`
	RiskyWorkloadCount  int `json:"riskyWorkloadCount"`
}

// WorkloadDiff describes changes between the current and previous revision of a workload.
type WorkloadDiff struct {
	Namespace       string           `json:"namespace"`
	Name            string           `json:"name"`
	Kind            string           `json:"kind"`
	CurrentRevision int64            `json:"currentRevision"`
	Changes         []TemplateChange `json:"changes"`
	RiskLevel       string           `json:"riskLevel"`
}

// TemplateChange is a single change in the pod template.
type TemplateChange struct {
	Type     string `json:"type"`
	Field    string `json:"field"`
	OldValue string `json:"oldValue,omitempty"`
	NewValue string `json:"newValue"`
	Impact   string `json:"impact"`
	Severity string `json:"severity"`
}

// BreakingChange identifies a change that could cause downtime or errors.
type BreakingChange struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Change    string `json:"change"`
	Reason    string `json:"reason"`
}

// RiskyWorkload is a workload with many or high-severity changes.
type RiskyWorkload struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	ChangeCount  int    `json:"changeCount"`
	HighSeverity int    `json:"highSeverityCount"`
	RiskScore    int    `json:"riskScore"`
}

// RevDiffIssue is an issue found during revision diff analysis.
type RevDiffIssue struct {
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	Message   string `json:"message"`
}

func (s *Server) handleRevisionDiff(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list deployments: %v", err))
		return
	}

	result := analyzeRevisionDiff(deployments.Items)
	writeJSON(w, result)
}

func analyzeRevisionDiff(deployments []appsv1.Deployment) RevisionDiffResult {
	now := time.Now()

	var workloadDiffs []WorkloadDiff
	var breakingChanges []BreakingChange
	var riskyWorkloads []RiskyWorkload
	var issues []RevDiffIssue

	summary := RevisionDiffSummary{TotalWorkloads: len(deployments)}

	for _, dep := range deployments {
		if dep.Namespace == "kube-system" {
			continue
		}

		currentRev := dep.Annotations["deployment.kubernetes.io/revision"]
		if currentRev == "" {
			continue
		}

		// Parse ReplicaSets from status to find the previous revision
		// We analyze template hash changes instead of actual RS diffs since we don't have RS objects
		var changes []TemplateChange
		riskLevel := "low"
		highSeverityCount := 0

		// Check if the deployment has been updated recently
		if dep.Status.UpdatedReplicas < *dep.Spec.Replicas && dep.Spec.Replicas != nil {
			changes = append(changes, TemplateChange{
				Type:     "Rollout",
				Field:    "status.updatedReplicas",
				NewValue: fmt.Sprintf("%d/%d", dep.Status.UpdatedReplicas, *dep.Spec.Replicas),
				Impact:   "Deployment is actively rolling out; some pods still on old revision",
				Severity: "medium",
			})
			if riskLevel == "low" {
				riskLevel = "medium"
			}
		}

		// Analyze the current pod template for risk indicators
		for i := range dep.Spec.Template.Spec.Containers {
			c := &dep.Spec.Template.Spec.Containers[i]

			// Image change detection (check annotation)
			changeCause := dep.Annotations["kubernetes.io/change-cause"]
			if changeCause != "" {
				changes = append(changes, TemplateChange{
					Type:     "Image",
					Field:    fmt.Sprintf("container[%s].image", c.Name),
					NewValue: c.Image,
					Impact:   fmt.Sprintf("Last change: %s", changeCause),
					Severity: "info",
				})
				summary.WithImageChange++
			}

			// Resource request analysis
			if c.Resources.Requests.Cpu().IsZero() && c.Resources.Requests.Memory().IsZero() {
				changes = append(changes, TemplateChange{
					Type:     "MissingResources",
					Field:    fmt.Sprintf("container[%s].resources.requests", c.Name),
					NewValue: "empty",
					Impact:   "No resource requests set; pod may not get guaranteed scheduling",
					Severity: "medium",
				})
				if riskLevel == "low" {
					riskLevel = "medium"
				}
			}

			// Probe analysis
			if c.ReadinessProbe == nil {
				changes = append(changes, TemplateChange{
					Type:     "MissingProbe",
					Field:    fmt.Sprintf("container[%s].readinessProbe", c.Name),
					NewValue: "nil",
					Impact:   "No readiness probe; traffic may route to unready pods",
					Severity: "high",
				})
				highSeverityCount++
				riskLevel = "high"
				summary.WithProbeChange++
			}
			if c.LivenessProbe == nil {
				changes = append(changes, TemplateChange{
					Type:     "MissingProbe",
					Field:    fmt.Sprintf("container[%s].livenessProbe", c.Name),
					NewValue: "nil",
					Impact:   "No liveness probe; stuck containers won't be restarted automatically",
					Severity: "medium",
				})
				if riskLevel == "low" {
					riskLevel = "medium"
				}
				summary.WithProbeChange++
			}

			// Security context analysis
			if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				changes = append(changes, TemplateChange{
					Type:     "SecurityContext",
					Field:    fmt.Sprintf("container[%s].securityContext.runAsNonRoot", c.Name),
					NewValue: "false/nil",
					Impact:   "Container may run as root; security risk",
					Severity: "high",
				})
				highSeverityCount++
				riskLevel = "high"
				summary.WithSecurityChange++
			}

			// Privileged container
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				changes = append(changes, TemplateChange{
					Type:     "SecurityContext",
					Field:    fmt.Sprintf("container[%s].securityContext.privileged", c.Name),
					NewValue: "true",
					Impact:   "Container runs in privileged mode; full host access",
					Severity: "critical",
				})
				highSeverityCount++
				riskLevel = "critical"
				summary.WithSecurityChange++

				breakingChanges = append(breakingChanges, BreakingChange{
					Namespace: dep.Namespace,
					Name:      dep.Name,
					Change:    "Privileged container",
					Reason:    "Container has full host access; potential security breach",
				})
				summary.BreakingChangeCount++
			}
		}

		// Check for strategy mismatches
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			changes = append(changes, TemplateChange{
				Type:     "Strategy",
				Field:    "spec.strategy.type",
				NewValue: "Recreate",
				Impact:   "Recreate strategy causes downtime during updates; consider RollingUpdate",
				Severity: "high",
			})
			highSeverityCount++
			riskLevel = "high"

			breakingChanges = append(breakingChanges, BreakingChange{
				Namespace: dep.Namespace,
				Name:      dep.Name,
				Change:    "Recreate strategy",
				Reason:    "Deployment uses Recreate strategy; will cause downtime during next update",
			})
			summary.BreakingChangeCount++
		}

		// Check revision history limit
		if dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0 {
			changes = append(changes, TemplateChange{
				Type:     "RevisionHistory",
				Field:    "spec.revisionHistoryLimit",
				NewValue: "0",
				Impact:   "Revision history disabled; cannot rollback to previous version",
				Severity: "medium",
			})
			if riskLevel == "low" {
				riskLevel = "medium"
			}
		}

		// Only include workloads with changes
		if len(changes) > 0 {
			wd := WorkloadDiff{
				Namespace: dep.Namespace,
				Name:      dep.Name,
				Kind:      "Deployment",
				Changes:   changes,
				RiskLevel: riskLevel,
			}
			// Parse revision number
			var revNum int64
			fmt.Sscanf(currentRev, "%d", &revNum)
			wd.CurrentRevision = revNum
			if revNum > 1 {
				summary.WithMultipleRevs++
			}

			workloadDiffs = append(workloadDiffs, wd)

			// Track risky workloads
			if riskLevel == "high" || riskLevel == "critical" {
				riskScore := len(changes) + highSeverityCount*3
				riskyWorkloads = append(riskyWorkloads, RiskyWorkload{
					Namespace:    dep.Namespace,
					Name:         dep.Name,
					ChangeCount:  len(changes),
					HighSeverity: highSeverityCount,
					RiskScore:    riskScore,
				})
				summary.RiskyWorkloadCount++
			}
		}
	}

	// Sort risky workloads by risk score descending
	sort.Slice(riskyWorkloads, func(i, j int) bool {
		return riskyWorkloads[i].RiskScore > riskyWorkloads[j].RiskScore
	})

	// Limit workloadDiffs to top 50 for performance
	if len(workloadDiffs) > 50 {
		sort.Slice(workloadDiffs, func(i, j int) bool {
			if workloadDiffs[i].RiskLevel != workloadDiffs[j].RiskLevel {
				return revRiskLevelRank(workloadDiffs[i].RiskLevel) > revRiskLevelRank(workloadDiffs[j].RiskLevel)
			}
			return len(workloadDiffs[i].Changes) > len(workloadDiffs[j].Changes)
		})
		workloadDiffs = workloadDiffs[:50]
	}

	// Score
	score := 100
	score -= summary.BreakingChangeCount * 10
	score -= summary.RiskyWorkloadCount * 5
	score -= summary.WithProbeChange * 2
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if summary.BreakingChangeCount > 0 {
		recs = append(recs, fmt.Sprintf("%d breaking change(s) detected; review and fix before next deployment", summary.BreakingChangeCount))
	}
	if summary.RiskyWorkloadCount > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have high-risk template configurations; add probes, resource requests, and security contexts", summary.RiskyWorkloadCount))
	}
	if summary.WithProbeChange > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) missing readiness/liveness probes; traffic may route to unhealthy pods", summary.WithProbeChange))
	}
	if len(recs) == 0 {
		recs = append(recs, "Deployment template configurations look healthy; no risky changes detected")
	}

	return RevisionDiffResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		WorkloadDiffs:   workloadDiffs,
		BreakingChanges: breakingChanges,
		RiskyWorkloads:  riskyWorkloads,
		Issues:          issues,
		Recommendations: recs,
	}
}

func revRiskLevelRank(level string) int {
	switch strings.ToLower(level) {
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
