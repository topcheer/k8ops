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
	"k8s.io/apimachinery/pkg/labels"
)

// LHResult is the label & annotation hygiene analysis.
type LHResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	Summary         LHSummary   `json:"summary"`
	ByWorkload      []LHEntry   `json:"byWorkload"`
	MissingStandard []LHEntry   `json:"missingStandard"` // missing app.kubernetes.io/name
	MissingTeam     []LHEntry   `json:"missingTeam"`     // missing team/owner labels
	MalformedKeys   []LHEntry   `json:"malformedKeys"`   // invalid key format
	NoLabels        []LHEntry   `json:"noLabels"`        // zero labels at all
	ByNamespace     []LHNSEntry `json:"byNamespace"`
	Issues          []LHIssue   `json:"issues"`
	Recommendations []string    `json:"recommendations"`
}

// LHSummary aggregates label hygiene statistics.
type LHSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	HasStandardLabel int `json:"hasStandardLabel"` // app.kubernetes.io/name
	HasTeamLabel     int `json:"hasTeamLabel"`     // team or owner label
	HasVersionLabel  int `json:"hasVersionLabel"`  // app.kubernetes.io/version
	NoLabels         int `json:"noLabels"`
	MalformedKeys    int `json:"malformedKeys"`
	ExcessiveLabels  int `json:"excessiveLabels"` // >20 labels
	HasAnnotations   int `json:"hasAnnotations"`
	HealthScore      int `json:"healthScore"` // 0-100
}

// LHEntry describes label hygiene for one workload.
type LHEntry struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Kind            string            `json:"kind"`
	LabelCount      int               `json:"labelCount"`
	AnnotationCount int               `json:"annotationCount"`
	Labels          map[string]string `json:"labels,omitempty"`
	MissingStandard bool              `json:"missingStandard"`
	MissingTeam     bool              `json:"missingTeam"`
	MissingVersion  bool              `json:"missingVersion"`
	HasMalformed    bool              `json:"hasMalformed"`
	MalformedKeys   []string          `json:"malformedKeys,omitempty"`
	IsExcessive     bool              `json:"isExcessive"`
	RiskLevel       string            `json:"riskLevel"`
}

// LHNSEntry per-namespace label stats.
type LHNSEntry struct {
	Namespace       string `json:"namespace"`
	WorkloadCount   int    `json:"workloadCount"`
	MissingStandard int    `json:"missingStandard"`
	MissingTeam     int    `json:"missingTeam"`
	NoLabels        int    `json:"noLabels"`
	HealthScore     int    `json:"healthScore"`
}

// LHIssue is a detected label problem.
type LHIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleLabelHygiene audits label and annotation hygiene across workloads.
// GET /api/product/label-hygiene
func (s *Server) handleLabelHygiene(w http.ResponseWriter, r *http.Request) {
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

	result := LHResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*LHNSEntry)

	for _, dep := range deployments.Items {
		entry := LHEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
		}

		lbls := dep.Spec.Template.Labels
		if len(lbls) == 0 {
			lbls = dep.Labels // fallback to metadata labels
		}

		entry.LabelCount = len(lbls)
		entry.AnnotationCount = len(dep.Annotations)
		entry.Labels = lbls

		result.Summary.TotalWorkloads++

		// Check standard labels (app.kubernetes.io/*)
		hasName := false
		hasVersion := false
		for k := range lbls {
			if k == "app.kubernetes.io/name" || k == "app" || k == "app.kubernetes.io/app" {
				hasName = true
			}
			if k == "app.kubernetes.io/version" || k == "version" {
				hasVersion = true
			}
		}
		if !hasName {
			entry.MissingStandard = true
			result.MissingStandard = append(result.MissingStandard, entry)
		} else {
			result.Summary.HasStandardLabel++
		}
		if !hasVersion {
			entry.MissingVersion = true
		} else {
			result.Summary.HasVersionLabel++
		}

		// Check team/owner labels
		hasTeam := false
		for _, teamKey := range []string{"team", "owner", "app.kubernetes.io/managed-by", "maintainer"} {
			if _, ok := lbls[teamKey]; ok {
				hasTeam = true
				break
			}
		}
		if !hasTeam {
			entry.MissingTeam = true
			result.MissingTeam = append(result.MissingTeam, entry)
		} else {
			result.Summary.HasTeamLabel++
		}

		// Check for malformed label keys
		for k := range lbls {
			if !isValidLabelKey(k) {
				entry.HasMalformed = true
				entry.MalformedKeys = append(entry.MalformedKeys, k)
			}
		}
		if entry.HasMalformed {
			result.Summary.MalformedKeys++
			result.MalformedKeys = append(result.MalformedKeys, entry)
			result.Issues = append(result.Issues, LHIssue{
				Severity: "warning", Type: "malformed-label-key",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Workload %s/%s has malformed label keys: %s", dep.Namespace, dep.Name, strings.Join(entry.MalformedKeys, ", ")),
			})
		}

		// No labels at all
		if entry.LabelCount == 0 {
			result.Summary.NoLabels++
			result.NoLabels = append(result.NoLabels, entry)
			result.Issues = append(result.Issues, LHIssue{
				Severity: "critical", Type: "no-labels",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Workload %s/%s has ZERO labels — Service selector, monitoring, and NetworkPolicy matching will fail", dep.Namespace, dep.Name),
			})
		}

		// Excessive labels
		if entry.LabelCount > 20 {
			entry.IsExcessive = true
			result.Summary.ExcessiveLabels++
			result.Issues = append(result.Issues, LHIssue{
				Severity: "info", Type: "excessive-labels",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Workload %s/%s has %d labels — consider cleaning up unused labels", dep.Namespace, dep.Name, entry.LabelCount),
			})
		}

		if entry.AnnotationCount > 0 {
			result.Summary.HasAnnotations++
		}

		entry.RiskLevel = lhAssessRisk(entry)

		// Namespace tracking
		nsStat := lhGetOrCreateNS(nsMap, dep.Namespace)
		nsStat.WorkloadCount++
		if entry.MissingStandard {
			nsStat.MissingStandard++
		}
		if entry.MissingTeam {
			nsStat.MissingTeam++
		}
		if entry.LabelCount == 0 {
			nsStat.NoLabels++
		}

		// Generate issues for missing standard/team
		if entry.MissingStandard && entry.LabelCount > 0 {
			result.Issues = append(result.Issues, LHIssue{
				Severity: "warning", Type: "missing-standard-label",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Workload %s/%s missing 'app.kubernetes.io/name' label — hinders discovery and management", dep.Namespace, dep.Name),
			})
		}
		if entry.MissingTeam && entry.LabelCount > 0 {
			result.Issues = append(result.Issues, LHIssue{
				Severity: "info", Type: "missing-team-label",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Workload %s/%s missing team/owner label — hinders ownership tracking and cost attribution", dep.Namespace, dep.Name),
			})
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		nsStat.HealthScore = lhNSScore(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return lhRiskRank(result.ByWorkload[i].RiskLevel) < lhRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].HealthScore < result.ByNamespace[j].HealthScore
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return lhIssueRank(result.Issues[i].Severity) < lhIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = lhScore(result.Summary)
	result.Recommendations = lhGenRecs(result.Summary, result.NoLabels, result.MissingStandard, result.MissingTeam)

	writeJSON(w, result)
}

// isValidLabelKey checks Kubernetes label key format.
func isValidLabelKey(key string) bool {
	if key == "" {
		return false
	}
	// Max length 253 (prefix) + 63 (name)
	if len(key) > 316 {
		return false
	}

	// Optional prefix/name format
	if idx := strings.Index(key, "/"); idx >= 0 {
		prefix := key[:idx]
		name := key[idx+1:]
		return isValidLabelSegment(prefix) && isValidLabelSegment(name)
	}
	return isValidLabelSegment(key)
}

// isValidLabelSegment validates a single DNS-1123 label segment.
func isValidLabelSegment(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, c := range s {
		if i == 0 || i == len(s)-1 {
			if !(isAlphaNum(c)) {
				return false
			}
		} else {
			if !(isAlphaNum(c) || c == '-' || c == '.') {
				return false
			}
		}
	}
	return true
}

func isAlphaNum(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

func lhAssessRisk(entry LHEntry) string {
	if entry.LabelCount == 0 {
		return "critical"
	}
	if entry.HasMalformed {
		return "high"
	}
	if entry.MissingStandard && entry.MissingTeam {
		return "medium"
	}
	if entry.MissingStandard || entry.MissingTeam {
		return "low"
	}
	return "low"
}

func lhScore(s LHSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.NoLabels * 20
	score -= s.MalformedKeys * 10
	missingStdPct := float64(s.TotalWorkloads-s.HasStandardLabel) / float64(s.TotalWorkloads) * 100
	score -= int(missingStdPct * 0.3)
	if score < 0 {
		score = 0
	}
	return score
}

func lhNSScore(ns LHNSEntry) int {
	if ns.WorkloadCount == 0 {
		return 100
	}
	issues := ns.MissingStandard + ns.MissingTeam + ns.NoLabels*3
	score := 100 - (issues*100)/(ns.WorkloadCount*4)
	if score < 0 {
		score = 0
	}
	return score
}

func lhGenRecs(s LHSummary, noLabels []LHEntry, missingStd []LHEntry, missingTeam []LHEntry) []string {
	var recs []string

	if s.NoLabels > 0 {
		top := ""
		if len(noLabels) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", noLabels[0].Namespace, noLabels[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d workload(s) have ZERO labels%s — add app.kubernetes.io/name immediately for Service selector matching", s.NoLabels, top))
	}
	if s.TotalWorkloads-s.HasStandardLabel > 0 {
		recs = append(recs, fmt.Sprintf("%d of %d workload(s) missing 'app.kubernetes.io/name' label — breaks kubectl, Helm, and monitoring discovery", s.TotalWorkloads-s.HasStandardLabel, s.TotalWorkloads))
	}
	if s.TotalWorkloads-s.HasTeamLabel > 0 {
		recs = append(recs, fmt.Sprintf("%d of %d workload(s) missing team/owner label — add 'team' or 'owner' for ownership tracking and FinOps", s.TotalWorkloads-s.HasTeamLabel, s.TotalWorkloads))
	}
	if s.MalformedKeys > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have malformed label keys — fix to DNS-1123 format (lowercase alphanumerics, '-', '.')", s.MalformedKeys))
	}
	if s.ExcessiveLabels > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have >20 labels — consider cleaning up unused labels for performance", s.ExcessiveLabels))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Label hygiene score is %d/100 — multiple workloads lack standard labels", s.HealthScore))
	}
	if s.NoLabels == 0 && s.MalformedKeys == 0 && s.HasStandardLabel == s.TotalWorkloads {
		recs = append(recs, "All workloads have proper labels — excellent discoverability and management")
	}

	return recs
}

func lhGetOrCreateNS(m map[string]*LHNSEntry, ns string) *LHNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &LHNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func lhRiskRank(level string) int {
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

func lhIssueRank(s string) int {
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

// Ensure imports used
var _ = labels.Everything
var _ = appsv1.DeploymentSpec{}
var _ = corev1.PodSpec{}
