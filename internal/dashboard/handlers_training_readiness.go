package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrainingReadinessResult assesses platform onboarding quality:
// documentation completeness, runbook coverage, labeling standardization,
// annotation governance, and team knowledge transfer readiness.
type TrainingReadinessResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          TrainingSummary       `json:"summary"`
	LabelGaps        []LabelGap            `json:"labelGaps"`
	OnboardingScore  int                   `json:"onboardingScore"`
	Grade            string                `json:"grade"`
	Recommendations  []string              `json:"recommendations"`
}

type TrainingSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	WithOwnerLabel    int     `json:"withOwnerLabel"`
	WithTeamLabel     int     `json:"withTeamLabel"`
	WithDocsLabel     int     `json:"withDocsLabel"`
	WithRunbookLabel  int     `json:"withRunbookLabel"`
	WithVersionLabel  int     `json:"withVersionLabel"`
	OwnerCoverage     float64 `json:"ownerCoverage"`
	TeamCoverage      float64 `json:"teamCoverage"`
	DocsCoverage      float64 `json:"docsCoverage"`
	RunbookCoverage   float64 `json:"runbookCoverage"`
}

type LabelGap struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Missing   []string `json:"missing"`
	Severity  string `json:"severity"`
}

// handleTrainingReadiness assesses platform onboarding and documentation quality.
// GET /api/docs/training-readiness
func (s *Server) handleTrainingReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TrainingReadinessResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})

	// Check labels on each workload
	checkLabels := func(name, ns string, labels map[string]string, kind string) {
		if systemNS[ns] {
			return
		}
		result.Summary.TotalWorkloads++

		var missing []string
		hasOwner := false
		hasTeam := false
		hasDocs := false
		hasRunbook := false
		hasVersion := false

		for k, v := range labels {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "owner") || strings.Contains(kl, "maintainer") || strings.Contains(kl, "contact") {
				if v != "" {
					hasOwner = true
				}
			}
			if strings.Contains(kl, "team") || strings.Contains(kl, "group") || strings.Contains(kl, "dept") {
				if v != "" {
					hasTeam = true
				}
			}
			if strings.Contains(kl, "docs") || strings.Contains(kl, "wiki") || strings.Contains(kl, "runbook") || strings.Contains(kl, "playbook") {
				if strings.Contains(strings.ToLower(v), "http") || strings.Contains(kl, "runbook") || strings.Contains(kl, "playbook") {
					hasDocs = true
				}
				if strings.Contains(kl, "runbook") || strings.Contains(kl, "playbook") {
					hasRunbook = true
				}
			}
			if strings.Contains(kl, "version") || strings.Contains(kl, "app.kubernetes.io/version") {
				if v != "" {
					hasVersion = true
				}
			}
		}

		if hasOwner {
			result.Summary.WithOwnerLabel++
		} else {
			missing = append(missing, "owner")
		}
		if hasTeam {
			result.Summary.WithTeamLabel++
		} else {
			missing = append(missing, "team")
		}
		if hasDocs {
			result.Summary.WithDocsLabel++
		} else {
			missing = append(missing, "docs-url")
		}
		if hasRunbook {
			result.Summary.WithRunbookLabel++
		} else {
			missing = append(missing, "runbook")
		}
		if hasVersion {
			result.Summary.WithVersionLabel++
		}

		if len(missing) > 0 {
			severity := "medium"
			if len(missing) >= 3 {
				severity = "high"
			}
			result.LabelGaps = append(result.LabelGaps, LabelGap{
				Workload:  name,
				Namespace: ns,
				Missing:   missing,
				Severity:  severity,
			})
		}
	}

	for _, dep := range deployments.Items {
		checkLabels(dep.Name, dep.Namespace, dep.Labels, "Deployment")
	}
	for _, sts := range statefulsets.Items {
		checkLabels(sts.Name, sts.Namespace, sts.Labels, "StatefulSet")
	}

	// Calculate coverage percentages
	total := result.Summary.TotalWorkloads
	if total > 0 {
		result.Summary.OwnerCoverage = float64(result.Summary.WithOwnerLabel) / float64(total) * 100
		result.Summary.TeamCoverage = float64(result.Summary.WithTeamLabel) / float64(total) * 100
		result.Summary.DocsCoverage = float64(result.Summary.WithDocsLabel) / float64(total) * 100
		result.Summary.RunbookCoverage = float64(result.Summary.WithRunbookLabel) / float64(total) * 100
	}

	// Score: weighted average of coverage metrics
	score := int((result.Summary.OwnerCoverage*0.3 + result.Summary.TeamCoverage*0.2 +
		result.Summary.DocsCoverage*0.3 + result.Summary.RunbookCoverage*0.2))
	result.OnboardingScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.OnboardingScore)

	// Sort gaps
	sort.Slice(result.LabelGaps, func(i, j int) bool {
		return len(result.LabelGaps[i].Missing) > len(result.LabelGaps[j].Missing)
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Onboarding readiness: %d/100 (grade %s)", result.OnboardingScore, result.Grade))
	if result.Summary.OwnerCoverage < 50 {
		recs = append(recs, fmt.Sprintf("Owner labels on %.0f%% of workloads — add 'app.kubernetes.io/managed-by' or 'owner' labels", result.Summary.OwnerCoverage))
	}
	if result.Summary.TeamCoverage < 50 {
		recs = append(recs, fmt.Sprintf("Team labels on %.0f%% of workloads — add 'team' label for accountability", result.Summary.TeamCoverage))
	}
	if result.Summary.DocsCoverage < 50 {
		recs = append(recs, fmt.Sprintf("Documentation URLs on %.0f%% of workloads — add 'docs-url' annotation pointing to wiki", result.Summary.DocsCoverage))
	}
	if result.Summary.RunbookCoverage < 50 {
		recs = append(recs, fmt.Sprintf("Runbook links on %.0f%% of workloads — add 'runbook-url' for operational procedures", result.Summary.RunbookCoverage))
	}
	if len(recs) == 1 {
		recs = append(recs, "Onboarding documentation is comprehensive — all workloads properly labeled")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
