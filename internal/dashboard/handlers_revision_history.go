package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RevHResult is the deployment revision history analysis.
type RevHResult struct {
	ScannedAt        time.Time   `json:"scannedAt"`
	Summary          RevHSummary `json:"summary"`
	ByWorkload       []RevHEntry `json:"byWorkload"`
	LowHistory       []RevHEntry `json:"lowHistory"`       // revisionHistoryLimit < 5
	NoHistory        []RevHEntry `json:"noHistory"`        // revisionHistoryLimit = 0
	HighChurn        []RevHEntry `json:"highChurn"`        // many ReplicaSets = frequent deploys
	StaleReplicaSets []RevHEntry `json:"staleReplicaSets"` // old revisions still running
	Issues           []RevHIssue `json:"issues"`
	Recommendations  []string    `json:"recommendations"`
}

// RevHSummary aggregates revision history stats.
type RevHSummary struct {
	TotalDeployments   int     `json:"totalDeployments"`
	LowHistoryLimit    int     `json:"lowHistoryLimit"`    // revisionHistoryLimit < 5
	NoHistoryLimit     int     `json:"noHistoryLimit"`     // revisionHistoryLimit = 0
	HighChurnWorkloads int     `json:"highChurnWorkloads"` // >10 ReplicaSets
	StaleRevisions     int     `json:"staleRevisions"`     // ReplicaSets older than 30 days
	AvgRevisionHistory float64 `json:"avgRevisionHistory"`
	RollbackReadiness  int     `json:"rollbackReadiness"` // 0-100
}

// RevHEntry describes one deployment's revision history.
type RevHEntry struct {
	Name                 string  `json:"name"`
	Namespace            string  `json:"namespace"`
	RevisionHistoryLimit *int32  `json:"revisionHistoryLimit,omitempty"`
	ReplicaSetCount      int     `json:"replicaSetCount"`
	CurrentReplicas      int32   `json:"currentReplicas"`
	UpdatedReplicas      int32   `json:"updatedReplicas"`
	OldestRSAgeHours     float64 `json:"oldestRSAgeHours"`
	RiskLevel            string  `json:"riskLevel"`
}

// RevHIssue is a detected revision history problem.
type RevHIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleRevisionHistory analyzes deployment revision history depth and rollback readiness.
// GET /api/deployment/revision-history
func (s *Server) handleRevisionHistory(w http.ResponseWriter, r *http.Request) {
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

	// Get ReplicaSets to count revisions per deployment
	rsList, err := rc.clientset.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build RS map: ns/deploymentName → []ReplicaSet
	rsMap := make(map[string][]appsv1.ReplicaSet)
	if rsList != nil {
		for _, rs := range rsList.Items {
			for _, ref := range rs.OwnerReferences {
				if ref.Kind == "Deployment" {
					key := rs.Namespace + "/" + ref.Name
					rsMap[key] = append(rsMap[key], rs)
					break
				}
			}
		}
	}

	result := RevHResult{ScannedAt: time.Now()}
	var totalRevHistory float64

	for _, dep := range deployments.Items {
		result.Summary.TotalDeployments++

		entry := RevHEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
		}

		// Revision history limit
		if dep.Spec.RevisionHistoryLimit != nil {
			entry.RevisionHistoryLimit = dep.Spec.RevisionHistoryLimit
			totalRevHistory += float64(*dep.Spec.RevisionHistoryLimit)
		} else {
			// Default is 10 in Kubernetes, but nil means unlimited in practice
			defaultLimit := int32(10)
			entry.RevisionHistoryLimit = &defaultLimit
			totalRevHistory += 10
		}

		// Current/updated replicas
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		entry.CurrentReplicas = replicas
		entry.UpdatedReplicas = dep.Status.UpdatedReplicas

		// ReplicaSet count
		key := dep.Namespace + "/" + dep.Name
		rsSets := rsMap[key]
		entry.ReplicaSetCount = len(rsSets)

		// Find oldest RS
		var oldestAge float64
		now := time.Now()
		for _, rs := range rsSets {
			if rs.CreationTimestamp.Time.Before(now) {
				age := now.Sub(rs.CreationTimestamp.Time).Hours()
				if age > oldestAge {
					oldestAge = age
				}
			}
		}
		entry.OldestRSAgeHours = oldestAge

		// Check for issues
		rhl := int32(10)
		if entry.RevisionHistoryLimit != nil {
			rhl = *entry.RevisionHistoryLimit
		}

		if rhl == 0 {
			result.Summary.NoHistoryLimit++
			result.NoHistory = append(result.NoHistory, entry)
			result.Issues = append(result.Issues, RevHIssue{
				Severity: "critical", Type: "no-revision-history",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has revisionHistoryLimit=0 — cannot rollback to previous version", dep.Namespace, dep.Name),
			})
		} else if rhl < 5 {
			result.Summary.LowHistoryLimit++
			result.LowHistory = append(result.LowHistory, entry)
			result.Issues = append(result.Issues, RevHIssue{
				Severity: "warning", Type: "low-revision-history",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has revisionHistoryLimit=%d — limited rollback options, recommend ≥10", dep.Namespace, dep.Name, rhl),
			})
		}

		if entry.ReplicaSetCount > 10 {
			result.Summary.HighChurnWorkloads++
			result.HighChurn = append(result.HighChurn, entry)
			result.Issues = append(result.Issues, RevHIssue{
				Severity: "info", Type: "high-churn",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s has %d ReplicaSets — high deployment churn, consider reducing revisionHistoryLimit", dep.Namespace, dep.Name, entry.ReplicaSetCount),
			})
		}

		// Stale ReplicaSets (>30 days old, not current)
		if oldestAge > 30*24 { // 30 days in hours
			result.Summary.StaleRevisions++
			result.StaleReplicaSets = append(result.StaleReplicaSets, entry)
		}

		entry.RiskLevel = revHAssessRisk(entry, rhl)
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Calculate average revision history
	if result.Summary.TotalDeployments > 0 {
		result.Summary.AvgRevisionHistory = totalRevHistory / float64(result.Summary.TotalDeployments)
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return revHRiskRank(result.ByWorkload[i].RiskLevel) < revHRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return revHIssueRank(result.Issues[i].Severity) < revHIssueRank(result.Issues[j].Severity)
	})

	result.Summary.RollbackReadiness = revHScore(result.Summary)
	result.Recommendations = revHGenRecs(result.Summary, result.NoHistory, result.LowHistory)

	writeJSON(w, result)
}

// revHAssessRisk determines risk level.
func revHAssessRisk(entry RevHEntry, rhl int32) string {
	if rhl == 0 {
		return "critical"
	}
	if rhl < 5 {
		return "high"
	}
	if entry.ReplicaSetCount > 15 {
		return "medium"
	}
	return "low"
}

// revHScore computes rollback readiness 0-100.
func revHScore(s RevHSummary) int {
	if s.TotalDeployments == 0 {
		return 100
	}
	score := 100
	score -= s.NoHistoryLimit * 15
	score -= s.LowHistoryLimit * 5
	if score < 0 {
		score = 0
	}
	return score
}

// revHGenRecs produces actionable advice.
func revHGenRecs(s RevHSummary, noHistory []RevHEntry, lowHistory []RevHEntry) []string {
	var recs []string

	if s.NoHistoryLimit > 0 {
		top := ""
		if len(noHistory) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", noHistory[0].Namespace, noHistory[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d deployment(s) have revisionHistoryLimit=0%s — cannot rollback, set to ≥10", s.NoHistoryLimit, top))
	}
	if s.LowHistoryLimit > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have revisionHistoryLimit<5 — limited rollback options, increase to ≥10", s.LowHistoryLimit))
	}
	if s.HighChurnWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have >10 ReplicaSets — high deployment churn, review CI/CD pipeline", s.HighChurnWorkloads))
	}
	if s.StaleRevisions > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have ReplicaSets older than 30 days — old revisions consuming etcd space", s.StaleRevisions))
	}
	if s.AvgRevisionHistory < 5 {
		recs = append(recs, fmt.Sprintf("Average revisionHistoryLimit is %.1f — low rollback capability across cluster", s.AvgRevisionHistory))
	}
	if s.RollbackReadiness < 70 {
		recs = append(recs, fmt.Sprintf("Rollback readiness score is %d/100 — review revision history settings", s.RollbackReadiness))
	}
	if s.NoHistoryLimit == 0 && s.LowHistoryLimit == 0 {
		recs = append(recs, fmt.Sprintf("All deployments have adequate revision history (avg: %.1f) — good rollback posture", s.AvgRevisionHistory))
	}

	return recs
}

func revHRiskRank(level string) int {
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

func revHIssueRank(s string) int {
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
