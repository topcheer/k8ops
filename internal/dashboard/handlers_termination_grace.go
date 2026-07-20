package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminationGraceResult audits terminationGracePeriodSeconds and preStop hooks.
type TerminationGraceResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         TermGraceSummary `json:"summary"`
	ByWorkload      []TermGraceEntry `json:"byWorkload"`
	Issues          []TermGraceIssue `json:"issues"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type TermGraceSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	DefaultGrace   int `json:"defaultGraceSeconds"`
	ShortGrace     int `json:"shortGraceWorkloads"` // < 10s
	LongGrace      int `json:"longGraceWorkloads"`  // > 60s
	WithPreStop    int `json:"withPreStopHook"`
	NoPreStop      int `json:"withoutPreStopHook"`
	ZeroGrace      int `json:"zeroGraceWorkloads"`
}

type TermGraceEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	GraceSeconds int64    `json:"gracePeriodSeconds"`
	HasPreStop   bool     `json:"hasPreStopHook"`
	PreStopCmd   string   `json:"preStopCommand"`
	RiskLevel    string   `json:"riskLevel"`
	Issues       []string `json:"issues"`
}

type TermGraceIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleTerminationGraceAudit handles GET /api/deployment/termination-grace-audit
func (s *Server) handleTerminationGraceAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TerminationGraceResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		grace := int64(30) // default
		if dep.Spec.Template.Spec.TerminationGracePeriodSeconds != nil {
			grace = *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
		}

		entry := TermGraceEntry{
			Name: dep.Name, Namespace: dep.Namespace, GraceSeconds: grace,
		}

		// Check preStop hooks
		hasPreStop := false
		preStopCmd := ""
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
				hasPreStop = true
				if c.Lifecycle.PreStop.Exec != nil {
					preStopCmd = string(c.Lifecycle.PreStop.Exec.Command[0])
				} else if c.Lifecycle.PreStop.HTTPGet != nil {
					preStopCmd = "http-get"
				}
			}
		}
		entry.HasPreStop = hasPreStop
		entry.PreStopCmd = preStopCmd

		var issues []string
		switch {
		case grace == 0:
			result.Summary.ZeroGrace++
			issues = append(issues, "zero grace period - SIGKILL immediately")
			result.Issues = append(result.Issues, TermGraceIssue{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue: "terminationGracePeriodSeconds=0", Severity: "critical",
			})
		case grace < 10:
			result.Summary.ShortGrace++
			issues = append(issues, fmt.Sprintf("short grace %ds", grace))
		case grace > 60:
			result.Summary.LongGrace++
		}

		if hasPreStop {
			result.Summary.WithPreStop++
		} else {
			result.Summary.NoPreStop++
			if grace < 15 {
				issues = append(issues, "no preStop hook + short grace")
				result.Issues = append(result.Issues, TermGraceIssue{
					Name: dep.Name, Namespace: dep.Namespace,
					Issue: "no preStop hook, in-flight requests may be dropped", Severity: "high",
				})
			}
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 2:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	if result.Summary.TotalWorkloads == 0 {
		result.Summary.DefaultGrace = 30
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].GraceSeconds < result.ByWorkload[j].GraceSeconds
	})

	if result.Summary.TotalWorkloads > 0 {
		protected := result.Summary.WithPreStop + (result.Summary.TotalWorkloads - result.Summary.ZeroGrace - result.Summary.ShortGrace)
		result.HealthScore = protected * 50 / result.Summary.TotalWorkloads
		if result.Summary.NoPreStop == 0 {
			result.HealthScore += 50
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("终止宽限期审计: %d 部署, %d 短(<10s), %d 长(>60s), %d 零, %d 有 preStop",
			result.Summary.TotalWorkloads, result.Summary.ShortGrace,
			result.Summary.LongGrace, result.Summary.ZeroGrace,
			result.Summary.WithPreStop),
	}
	if result.Summary.NoPreStop > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个部署无 preStop 钩子, 优雅终止可能不完整", result.Summary.NoPreStop))
	}
	writeJSON(w, result)
}

var _ appsv1.DeploymentSpec
