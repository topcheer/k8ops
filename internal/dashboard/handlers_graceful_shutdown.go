package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GSResult is the graceful shutdown & termination compliance analysis.
type GSResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         GSSummary `json:"summary"`
	ByWorkload      []GSEntry `json:"byWorkload"`
	NoPreStop       []GSEntry `json:"noPreStop"`        // missing preStop hook
	ShortGrace      []GSEntry `json:"shortGracePeriod"` // <10s termination grace
	NoReadinessGate []GSEntry `json:"noReadinessGate"`  // no readiness = can't drain
	NoSIGTERM       []GSEntry `json:"noSIGTERM"`        // likely ignores SIGTERM (no preStop, short grace)
	Issues          []GSIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// GSSummary aggregates graceful shutdown statistics.
type GSSummary struct {
	TotalContainers int `json:"totalContainers"`
	HasPreStop      int `json:"hasPreStop"` // containers with preStop hook
	NoPreStop       int `json:"noPreStop"`
	HasReadiness    int `json:"hasReadiness"` // containers with readiness probe
	NoReadiness     int `json:"noReadiness"`
	GraceShort      int `json:"gracePeriodShort"`   // <10s
	GraceDefault    int `json:"gracePeriodDefault"` // exactly 30s (default)
	GraceLong       int `json:"gracePeriodLong"`    // >60s
	GraceCustom     int `json:"gracePeriodCustom"`  // explicitly set (non-default)
	LikelyDropReqs  int `json:"likelyDropRequests"` // no preStop + no readiness = dropped
	ShutdownScore   int `json:"shutdownScore"`      // 0-100
}

// GSEntry describes one container's termination config.
type GSEntry struct {
	Workload      string   `json:"workload"`
	Namespace     string   `json:"namespace"`
	Kind          string   `json:"kind"`
	Container     string   `json:"container"`
	HasPreStop    bool     `json:"hasPreStop"`
	PreStopAction string   `json:"preStopAction,omitempty"` // httpGet/exec description
	HasReadiness  bool     `json:"hasReadiness"`
	GracePeriod   int64    `json:"gracePeriodSeconds"`
	GraceCategory string   `json:"graceCategory"` // short/default/long/custom
	Violations    []string `json:"violations,omitempty"`
	RiskLevel     string   `json:"riskLevel"`
}

// GSIssue is a detected shutdown problem.
type GSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleGracefulShutdown audits graceful shutdown and termination compliance.
// GET /api/deployment/graceful-shutdown
func (s *Server) handleGracefulShutdown(w http.ResponseWriter, r *http.Request) {
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

	result := GSResult{ScannedAt: time.Now()}

	for _, dep := range deployments.Items {
		// Pod-level grace period
		podGrace := int64(30) // Kubernetes default
		if dep.Spec.Template.Spec.TerminationGracePeriodSeconds != nil {
			podGrace = *dep.Spec.Template.Spec.TerminationGracePeriodSeconds
		}
		graceCat := gsGraceCategory(podGrace)

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			entry := GSEntry{
				Workload:      dep.Name,
				Namespace:     dep.Namespace,
				Kind:          "Deployment",
				Container:     c.Name,
				GracePeriod:   podGrace,
				GraceCategory: graceCat,
			}

			// Check preStop lifecycle hook
			if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
				entry.HasPreStop = true
				if c.Lifecycle.PreStop.HTTPGet != nil {
					entry.PreStopAction = fmt.Sprintf("httpGet: %s%s", c.Lifecycle.PreStop.HTTPGet.Path, gsPortStr(c.Lifecycle.PreStop.HTTPGet.Port))
				} else if c.Lifecycle.PreStop.Exec != nil {
					entry.PreStopAction = fmt.Sprintf("exec: %s", gsJoinCmd(c.Lifecycle.PreStop.Exec.Command))
				}
				result.Summary.HasPreStop++
			} else {
				result.Summary.NoPreStop++
				entry.Violations = append(entry.Violations, "No preStop hook — SIGTERM sent immediately, in-flight requests may be dropped")
				result.NoPreStop = append(result.NoPreStop, entry)
			}

			// Check readiness probe (needed for connection draining)
			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
				result.Summary.HasReadiness++
			} else {
				result.Summary.NoReadiness++
				entry.Violations = append(entry.Violations, "No readiness probe — endpoints not removed before termination, traffic to dying pod")
				result.NoReadinessGate = append(result.NoReadinessGate, entry)
			}

			// Grace period categorization
			switch graceCat {
			case "short":
				result.Summary.GraceShort++
				if !entry.HasPreStop {
					entry.Violations = append(entry.Violations, fmt.Sprintf("Grace period %ds is very short (<10s) — insufficient for graceful shutdown", podGrace))
					result.ShortGrace = append(result.ShortGrace, entry)
				}
			case "default":
				result.Summary.GraceDefault++
			case "long":
				result.Summary.GraceLong++
			}
			if podGrace != 30 {
				result.Summary.GraceCustom++
			}

			// Likely drops requests: no preStop + no readiness
			if !entry.HasPreStop && !entry.HasReadiness {
				result.Summary.LikelyDropReqs++
				result.NoSIGTERM = append(result.NoSIGTERM, entry)
				result.Issues = append(result.Issues, GSIssue{
					Severity: "critical", Type: "dropped-requests",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s has no preStop hook AND no readiness probe — in-flight requests WILL be dropped during rolling update", c.Name, dep.Namespace, dep.Name),
				})
			} else if !entry.HasPreStop {
				result.Issues = append(result.Issues, GSIssue{
					Severity: "warning", Type: "no-prestop",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s lacks preStop hook — add 'sleep 5' or drain endpoint for graceful connection termination", c.Name, dep.Namespace, dep.Name),
				})
			}

			entry.RiskLevel = gsAssessRisk(entry)
			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return gsRiskRank(result.ByWorkload[i].RiskLevel) < gsRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return gsIssueRank(result.Issues[i].Severity) < gsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ShutdownScore = gsScore(result.Summary)
	result.Recommendations = gsGenRecs(result.Summary, result.NoPreStop, result.NoReadinessGate)

	writeJSON(w, result)
}

// gsGraceCategory classifies the grace period.
func gsGraceCategory(grace int64) string {
	switch {
	case grace < 10:
		return "short"
	case grace == 30:
		return "default"
	case grace > 60:
		return "long"
	default:
		return "custom"
	}
}

// gsAssessRisk determines risk level.
func gsAssessRisk(entry GSEntry) string {
	if !entry.HasPreStop && !entry.HasReadiness {
		return "critical"
	}
	if !entry.HasPreStop {
		return "high"
	}
	if entry.GraceCategory == "short" {
		return "high"
	}
	if !entry.HasReadiness {
		return "medium"
	}
	return "low"
}

// gsScore computes 0-100.
func gsScore(s GSSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.LikelyDropReqs * 15
	score -= s.NoPreStop * 5
	score -= s.NoReadiness * 4
	score -= s.GraceShort * 3
	if score < 0 {
		score = 0
	}
	return score
}

// gsGenRecs produces actionable advice.
func gsGenRecs(s GSSummary, noPreStop []GSEntry, noReadiness []GSEntry) []string {
	var recs []string

	if s.LikelyDropReqs > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) will drop in-flight requests during rolling updates (no preStop + no readiness) — add both immediately", s.LikelyDropReqs))
	}
	if s.NoPreStop > 0 {
		top := ""
		if len(noPreStop) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", noPreStop[0].Namespace, noPreStop[0].Workload)
		}
		recs = append(recs, fmt.Sprintf("%d container(s) lack preStop hook%s — add preStop: exec: ['sleep', '5'] or HTTP drain endpoint", s.NoPreStop, top))
	}
	if s.NoReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) lack readiness probe — needed for endpoint removal before SIGTERM", s.NoReadiness))
	}
	if s.GraceShort > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have grace period <10s — increase terminationGracePeriodSeconds for slow shutdown apps", s.GraceShort))
	}
	if s.GraceLong > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have grace period >60s — verify the app actually needs this long", s.GraceLong))
	}
	if s.ShutdownScore < 60 {
		recs = append(recs, fmt.Sprintf("Graceful shutdown score is %d/100 — rolling updates may cause request drops", s.ShutdownScore))
	}
	if s.LikelyDropReqs == 0 && s.NoPreStop == 0 {
		recs = append(recs, "All containers have preStop hooks — good graceful shutdown posture")
	}

	return recs
}

// gsPortStr extracts port string from intstr.
func gsPortStr(port interface{}) string {
	switch v := port.(type) {
	case int:
		return fmt.Sprintf(":%d", v)
	case int32:
		return fmt.Sprintf(":%d", v)
	case string:
		return fmt.Sprintf(":%s", v)
	default:
		return ""
	}
}

// gsJoinCmd joins command parts for display.
func gsJoinCmd(cmd []string) string {
	if len(cmd) == 0 {
		return ""
	}
	if len(cmd) <= 3 {
		result := ""
		for i, c := range cmd {
			if i > 0 {
				result += " "
			}
			result += c
		}
		return result
	}
	return cmd[0] + " " + cmd[1] + " ..."
}

func gsRiskRank(level string) int {
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

func gsIssueRank(s string) int {
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
var _ = corev1.PodSpec{}
