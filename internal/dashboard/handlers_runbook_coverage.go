package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunbookCoverageResult scans workloads for documentation annotations and
// identifies undocumented critical services. Checks for common runbook
// annotation patterns: runbook, docs, wiki, oncall, sop.
type RunbookCoverageResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         RunbookSummary `json:"summary"`
	Documented      []RunbookEntry `json:"documented"`
	Undocumented    []RunbookEntry `json:"undocumented"`
	CoverageScore   int            `json:"coverageScore"`
	Grade           string         `json:"grade"`
	Recommendations []string       `json:"recommendations"`
}

type RunbookSummary struct {
	TotalWorkloads      int `json:"totalWorkloads"`
	WithRunbook         int `json:"withRunbook"`
	WithoutRunbook      int `json:"withoutRunbook"`
	CriticalMissing     int `json:"criticalMissing"`
	HighPriorityMissing int `json:"highPriorityMissing"`
}

type RunbookEntry struct {
	Workload      string `json:"workload"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	HasRunbook    bool   `json:"hasRunbook"`
	RunbookURL    string `json:"runbookUrl"`
	RunbookSource string `json:"runbookSource"`
	Priority      string `json:"priority"`
	Issue         string `json:"issue"`
}

// Common runbook-related annotation keys
var runbookAnnotations = []string{
	"runbook",
	"runbook-url",
	"docs",
	"documentation",
	"wiki",
	"wiki-url",
	"oncall",
	"oncall-url",
	"sop",
	"playbook",
	"incident.io/runbook",
	"grafana/dashboard",
}

// handleRunbookCoverage handles GET /api/docs/runbook-coverage
func (s *Server) handleRunbookCoverage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RunbookCoverageResult{ScannedAt: time.Now()}

	// Helper to check annotations for runbook references
	findRunbook := func(annotations map[string]string) (bool, string, string) {
		for _, key := range runbookAnnotations {
			if val, ok := annotations[key]; ok && val != "" {
				return true, val, key
			}
			// Also check with app.kubernetes.io prefix
			fullKey := "app.kubernetes.io/" + key
			if val, ok := annotations[fullKey]; ok && val != "" {
				return true, val, fullKey
			}
		}
		return false, "", ""
	}

	// Determine priority based on labels
	getPriority := func(labels map[string]string) string {
		if labels["critical"] == "true" || labels["priority"] == "critical" {
			return "critical"
		}
		if labels["app.kubernetes.io/name"] != "" {
			return "standard"
		}
		return "low"
	}

	// Scan Deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		entry := RunbookEntry{
			Workload:  d.Name,
			Namespace: d.Namespace,
			Kind:      "Deployment",
			Priority:  getPriority(d.Labels),
		}

		hasRb, url, source := findRunbook(d.Annotations)
		entry.HasRunbook = hasRb
		entry.RunbookURL = url
		entry.RunbookSource = source

		if hasRb {
			result.Summary.WithRunbook++
			result.Documented = append(result.Documented, entry)
		} else {
			result.Summary.WithoutRunbook++
			entry.Issue = "No runbook annotation found"
			result.Undocumented = append(result.Undocumented, entry)
			if entry.Priority == "critical" {
				result.Summary.CriticalMissing++
			}
		}
	}

	// Scan StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, s := range sts.Items {
		if isSystemNamespace(s.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		entry := RunbookEntry{
			Workload:  s.Name,
			Namespace: s.Namespace,
			Kind:      "StatefulSet",
			Priority:  getPriority(s.Labels),
		}

		hasRb, url, source := findRunbook(s.Annotations)
		entry.HasRunbook = hasRb
		entry.RunbookURL = url
		entry.RunbookSource = source

		if hasRb {
			result.Summary.WithRunbook++
			result.Documented = append(result.Documented, entry)
		} else {
			result.Summary.WithoutRunbook++
			entry.Issue = "No runbook annotation found"
			result.Undocumented = append(result.Undocumented, entry)
			if entry.Priority == "critical" {
				result.Summary.CriticalMissing++
			}
		}
	}

	// Sort undocumented by priority
	priorityRank := map[string]int{"critical": 0, "high": 1, "standard": 2, "low": 3}
	sort.Slice(result.Undocumented, func(i, j int) bool {
		return priorityRank[result.Undocumented[i].Priority] < priorityRank[result.Undocumented[j].Priority]
	})

	// Coverage score
	if result.Summary.TotalWorkloads > 0 {
		result.CoverageScore = result.Summary.WithRunbook * 100 / result.Summary.TotalWorkloads
	}

	switch {
	case result.CoverageScore >= 80:
		result.Grade = "A"
	case result.CoverageScore >= 60:
		result.Grade = "B"
	case result.CoverageScore >= 40:
		result.Grade = "C"
	case result.CoverageScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildRunbookRecs(&result)
	writeJSON(w, result)
}

func buildRunbookRecs(r *RunbookCoverageResult) []string {
	recs := []string{
		fmt.Sprintf("Runbook 覆盖率: %d/%d 工作负载 (%d%%)", r.Summary.WithRunbook, r.Summary.TotalWorkloads, r.CoverageScore),
	}
	if r.Summary.CriticalMissing > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个关键工作负载缺少 Runbook 文档", r.Summary.CriticalMissing))
	}
	if r.CoverageScore < 50 {
		recs = append(recs, "建议: 为每个工作负载添加 'runbook' 注解指向运维手册 URL")
	}
	if len(r.Undocumented) > 0 {
		top := r.Undocumented[0]
		recs = append(recs, fmt.Sprintf("最高优先级缺失: %s/%s (%s)", top.Namespace, top.Workload, top.Priority))
	}
	// List supported annotation patterns
	recs = append(recs, fmt.Sprintf("支持的注解格式: %s", strings.Join(runbookAnnotations[:5], ", ")))
	return recs
}
