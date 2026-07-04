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
)

// ProbeAuditResult is the full health probe effectiveness analysis.
type ProbeAuditResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         ProbeAuditSummary `json:"summary"`
	Workloads       []ProbeWorkload   `json:"workloads"`
	TopFindings     []ProbeFindingAgg `json:"topFindings"`
	Recommendations []string          `json:"recommendations"`
}

// ProbeAuditSummary aggregates cluster-wide probe metrics.
type ProbeAuditSummary struct {
	TotalContainers      int `json:"totalContainers"`
	HasLiveness          int `json:"hasLiveness"`
	HasReadiness         int `json:"hasReadiness"`
	HasStartup           int `json:"hasStartup"`
	MissingLiveness      int `json:"missingLiveness"`
	MissingReadiness     int `json:"missingReadiness"`
	AggressiveProbes     int `json:"aggressiveProbes"`     // interval < 10s
	SlowProbes           int `json:"slowProbes"`           // readiness interval > 60s
	ShortTimeout         int `json:"shortTimeout"`         // timeout < 2s
	LowFailureThreshold  int `json:"lowFailureThreshold"`  // failureThreshold < 3
	HighFailureThreshold int `json:"highFailureThreshold"` // failureThreshold > 10
	Score                int `json:"score"`                // 0-100 (100 = all good)
}

// ProbeWorkload describes probe audit for a single workload.
type ProbeWorkload struct {
	Name       string           `json:"name"`
	Namespace  string           `json:"namespace"`
	Kind       string           `json:"kind"`
	Containers []ProbeContainer `json:"containers"`
	RiskScore  int              `json:"riskScore"`
	WorstIssue string           `json:"worstIssue"`
}

// ProbeContainer describes probe config for one container.
type ProbeContainer struct {
	Name      string       `json:"name"`
	Image     string       `json:"image"`
	Liveness  *ProbeDetail `json:"liveness"`
	Readiness *ProbeDetail `json:"readiness"`
	Startup   *ProbeDetail `json:"startup"`
	Findings  []ProbeIssue `json:"findings"`
}

// ProbeDetail describes a configured probe.
type ProbeDetail struct {
	Type             string `json:"type"` // httpGet, tcpSocket, exec
	Path             string `json:"path,omitempty"`
	Port             int32  `json:"port,omitempty"`
	InitialDelaySec  int32  `json:"initialDelaySeconds"`
	PeriodSec        int32  `json:"periodSeconds"`
	TimeoutSec       int32  `json:"timeoutSeconds"`
	SuccessThreshold int32  `json:"successThreshold"`
	FailureThreshold int32  `json:"failureThreshold"`
}

// ProbeIssue describes a single probe configuration problem.
type ProbeIssue struct {
	Severity   string `json:"severity"` // critical / warning / info
	Check      string `json:"check"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

// ProbeFindingAgg aggregates findings by check type.
type ProbeFindingAgg struct {
	Check    string `json:"check"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
}

// handleProbeAudit analyzes health probe effectiveness across all workloads.
// GET /api/operations/probes?namespace=xxx
func (s *Server) handleProbeAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	// Fetch all workload types
	deployments, _ := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})

	result := ProbeAuditResult{ScannedAt: time.Now()}
	findingAgg := make(map[string]*ProbeFindingAgg)
	totalScore := 0

	// Process deployments
	for i := range deployments.Items {
		pw := auditWorkloadProbes("Deployment", &deployments.Items[i].ObjectMeta, &deployments.Items[i].Spec.Template.Spec)
		if len(pw.Containers) > 0 {
			result.Workloads = append(result.Workloads, pw)
			aggregateFindings(findingAgg, pw)
			totalScore += pw.RiskScore
			updateSummary(&result.Summary, pw)
		}
	}

	// Process statefulsets
	for i := range statefulsets.Items {
		pw := auditWorkloadProbes("StatefulSet", &statefulsets.Items[i].ObjectMeta, &statefulsets.Items[i].Spec.Template.Spec)
		if len(pw.Containers) > 0 {
			result.Workloads = append(result.Workloads, pw)
			aggregateFindings(findingAgg, pw)
			totalScore += pw.RiskScore
			updateSummary(&result.Summary, pw)
		}
	}

	// Process daemonsets
	for i := range daemonsets.Items {
		pw := auditWorkloadProbes("DaemonSet", &daemonsets.Items[i].ObjectMeta, &daemonsets.Items[i].Spec.Template.Spec)
		if len(pw.Containers) > 0 {
			result.Workloads = append(result.Workloads, pw)
			aggregateFindings(findingAgg, pw)
			totalScore += pw.RiskScore
			updateSummary(&result.Summary, pw)
		}
	}

	// Calculate overall score
	if len(result.Workloads) > 0 {
		avgRisk := totalScore / len(result.Workloads)
		result.Summary.Score = 100 - avgRisk
		if result.Summary.Score < 0 {
			result.Summary.Score = 0
		}
	}

	// Build top findings
	for _, agg := range findingAgg {
		result.TopFindings = append(result.TopFindings, *agg)
	}
	sort.Slice(result.TopFindings, func(i, j int) bool {
		if result.TopFindings[i].Severity != result.TopFindings[j].Severity {
			return probeSevRank(result.TopFindings[i].Severity) < probeSevRank(result.TopFindings[j].Severity)
		}
		return result.TopFindings[i].Count > result.TopFindings[j].Count
	})

	// Sort workloads by risk
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].RiskScore > result.Workloads[j].RiskScore
	})

	// Recommendations
	result.Recommendations = generateProbeRecommendations(result)

	writeJSON(w, result)
}

// auditWorkloadProbes analyzes probe config for one workload.
func auditWorkloadProbes(kind string, meta *metav1.ObjectMeta, spec *corev1.PodSpec) ProbeWorkload {
	pw := ProbeWorkload{
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Kind:      kind,
	}

	for _, c := range spec.Containers {
		pc := ProbeContainer{
			Name:  c.Name,
			Image: c.Image,
		}

		// Liveness
		if c.LivenessProbe != nil {
			pc.Liveness = extractProbeDetail(c.LivenessProbe)
		} else {
			pc.Findings = append(pc.Findings, ProbeIssue{
				Severity:   "warning",
				Check:      "missing-liveness",
				Message:    fmt.Sprintf("Container %q has no liveness probe", c.Name),
				Suggestion: "Add a liveness probe to detect and restart hung containers",
			})
		}

		// Readiness
		if c.ReadinessProbe != nil {
			pc.Readiness = extractProbeDetail(c.ReadinessProbe)
		} else {
			pc.Findings = append(pc.Findings, ProbeIssue{
				Severity:   "warning",
				Check:      "missing-readiness",
				Message:    fmt.Sprintf("Container %q has no readiness probe — traffic may reach unready pods", c.Name),
				Suggestion: "Add a readiness probe to prevent traffic to containers that aren't ready",
			})
		}

		// Startup
		if c.StartupProbe != nil {
			pc.Startup = extractProbeDetail(c.StartupProbe)
		}

		// Analyze probe configurations
		if c.LivenessProbe != nil {
			pc.Findings = append(pc.Findings, analyzeProbeConfig("liveness", c.LivenessProbe, c.Name)...)
		}
		if c.ReadinessProbe != nil {
			pc.Findings = append(pc.Findings, analyzeProbeConfig("readiness", c.ReadinessProbe, c.Name)...)
		}
		if c.StartupProbe != nil {
			pc.Findings = append(pc.Findings, analyzeProbeConfig("startup", c.StartupProbe, c.Name)...)
		}

		// Check for slow-starting apps without startup probe
		if c.StartupProbe == nil && c.LivenessProbe != nil {
			delay := c.LivenessProbe.InitialDelaySeconds
			if delay > 60 {
				pc.Findings = append(pc.Findings, ProbeIssue{
					Severity:   "info",
					Check:      "slow-start-no-startup",
					Message:    fmt.Sprintf("Container %q has liveness initialDelay=%ds but no startup probe — consider adding one for slow-starting apps", c.Name, delay),
					Suggestion: "Startup probes prevent liveness probes from killing slow-to-start containers",
				})
			}
		}

		// Check same path for liveness and readiness (common anti-pattern)
		if c.LivenessProbe != nil && c.ReadinessProbe != nil {
			if probesIdentical(c.LivenessProbe, c.ReadinessProbe) {
				pc.Findings = append(pc.Findings, ProbeIssue{
					Severity:   "info",
					Check:      "identical-probes",
					Message:    fmt.Sprintf("Container %q uses identical liveness and readiness probes — consider differentiating them", c.Name),
					Suggestion: "Use a broader check for liveness (is process alive?) and a narrower check for readiness (is app ready to serve?)",
				})
			}
		}

		pw.Containers = append(pw.Containers, pc)
	}

	// Calculate risk score
	for _, c := range pw.Containers {
		for _, f := range c.Findings {
			switch f.Severity {
			case "critical":
				pw.RiskScore += 20
			case "warning":
				pw.RiskScore += 8
			case "info":
				pw.RiskScore += 2
			}
		}
	}

	// Determine worst issue
	for _, c := range pw.Containers {
		for _, f := range c.Findings {
			if f.Severity == "critical" {
				pw.WorstIssue = f.Check
				return pw
			}
		}
	}
	for _, c := range pw.Containers {
		for _, f := range c.Findings {
			if f.Severity == "warning" && pw.WorstIssue == "" {
				pw.WorstIssue = f.Check
			}
		}
	}

	return pw
}

// extractProbeDetail converts a Kubernetes probe to our detail struct.
func extractProbeDetail(probe *corev1.Probe) *ProbeDetail {
	d := &ProbeDetail{
		InitialDelaySec:  probe.InitialDelaySeconds,
		PeriodSec:        probe.PeriodSeconds,
		TimeoutSec:       probe.TimeoutSeconds,
		SuccessThreshold: probe.SuccessThreshold,
		FailureThreshold: probe.FailureThreshold,
	}

	// Apply defaults for display
	if d.PeriodSec == 0 {
		d.PeriodSec = 10
	}
	if d.TimeoutSec == 0 {
		d.TimeoutSec = 1
	}
	if d.SuccessThreshold == 0 {
		d.SuccessThreshold = 1
	}
	if d.FailureThreshold == 0 {
		d.FailureThreshold = 3
	}

	if probe.HTTPGet != nil {
		d.Type = "httpGet"
		d.Path = probe.HTTPGet.Path
		if probe.HTTPGet.Port.IntValue() > 0 {
			d.Port = int32(probe.HTTPGet.Port.IntValue())
		}
	} else if probe.TCPSocket != nil {
		d.Type = "tcpSocket"
		if probe.TCPSocket.Port.IntValue() > 0 {
			d.Port = int32(probe.TCPSocket.Port.IntValue())
		}
	} else if probe.Exec != nil {
		d.Type = "exec"
		if len(probe.Exec.Command) > 0 {
			d.Path = strings.Join(probe.Exec.Command, " ")
		}
	} else {
		d.Type = "grpc"
	}

	return d
}

// analyzeProbeConfig checks a probe's timing configuration for issues.
func analyzeProbeConfig(probeName string, probe *corev1.Probe, containerName string) []ProbeIssue {
	var findings []ProbeIssue

	period := probe.PeriodSeconds
	if period == 0 {
		period = 10 // default
	}
	timeout := probe.TimeoutSeconds
	if timeout == 0 {
		timeout = 1 // default
	}
	failure := probe.FailureThreshold
	if failure == 0 {
		failure = 3 // default
	}

	// Aggressive probe: period < 5s
	if period < 5 {
		findings = append(findings, ProbeIssue{
			Severity:   "warning",
			Check:      "aggressive-probe",
			Message:    fmt.Sprintf("%s probe for %q has period=%ds (< 5s) — may cause excessive load", probeName, containerName, period),
			Suggestion: "Increase periodSeconds to at least 10s to reduce API server and endpoint load",
		})
	}

	// Short timeout: < 2s
	if timeout < 2 {
		findings = append(findings, ProbeIssue{
			Severity:   "warning",
			Check:      "short-timeout",
			Message:    fmt.Sprintf("%s probe for %q has timeout=%ds (< 2s) — may fail under latency spikes", probeName, containerName, timeout),
			Suggestion: "Increase timeoutSeconds to at least 2-3s to avoid false negatives",
		})
	}

	// Low failure threshold: < 3
	if failure < 3 {
		findings = append(findings, ProbeIssue{
			Severity:   "warning",
			Check:      "low-failure-threshold",
			Message:    fmt.Sprintf("%s probe for %q has failureThreshold=%d (< 3) — too sensitive to transient errors", probeName, containerName, failure),
			Suggestion: "Increase failureThreshold to 3+ to tolerate transient network issues",
		})
	}

	// Slow readiness: period > 60s
	if probeName == "readiness" && period > 60 {
		findings = append(findings, ProbeIssue{
			Severity:   "info",
			Check:      "slow-readiness",
			Message:    fmt.Sprintf("Readiness probe for %q has period=%ds (> 60s) — slow to detect ready pods", containerName, period),
			Suggestion: "Decrease readiness periodSeconds to 10-30s for faster traffic routing",
		})
	}

	// High failure threshold on liveness: > 10 (slow to restart)
	if probeName == "liveness" && failure > 10 {
		findings = append(findings, ProbeIssue{
			Severity:   "info",
			Check:      "high-failure-threshold",
			Message:    fmt.Sprintf("Liveness probe for %q has failureThreshold=%d (> 10) — slow to restart unhealthy containers", containerName, failure),
			Suggestion: "Decrease failureThreshold to 3-6 for faster recovery from failures",
		})
	}

	return findings
}

// probesIdentical checks if two probes have the same config.
func probesIdentical(a, b *corev1.Probe) bool {
	if a.HTTPGet != nil && b.HTTPGet != nil {
		return a.HTTPGet.Path == b.HTTPGet.Path &&
			a.HTTPGet.Port.String() == b.HTTPGet.Port.String()
	}
	if a.TCPSocket != nil && b.TCPSocket != nil {
		return a.TCPSocket.Port.String() == b.TCPSocket.Port.String()
	}
	if a.Exec != nil && b.Exec != nil {
		return strings.Join(a.Exec.Command, " ") == strings.Join(b.Exec.Command, " ")
	}
	return false
}

// aggregateFindings updates the finding aggregation map.
func aggregateFindings(m map[string]*ProbeFindingAgg, pw ProbeWorkload) {
	for _, c := range pw.Containers {
		for _, f := range c.Findings {
			if agg, ok := m[f.Check]; ok {
				agg.Count++
			} else {
				m[f.Check] = &ProbeFindingAgg{
					Check:    f.Check,
					Count:    1,
					Severity: f.Severity,
				}
			}
		}
	}
}

// updateSummary updates the audit summary from a workload.
func updateSummary(s *ProbeAuditSummary, pw ProbeWorkload) {
	for _, c := range pw.Containers {
		s.TotalContainers++
		if c.Liveness != nil {
			s.HasLiveness++
		} else {
			s.MissingLiveness++
		}
		if c.Readiness != nil {
			s.HasReadiness++
		} else {
			s.MissingReadiness++
		}
		if c.Startup != nil {
			s.HasStartup++
		}

		for _, f := range c.Findings {
			switch f.Check {
			case "aggressive-probe":
				s.AggressiveProbes++
			case "slow-readiness":
				s.SlowProbes++
			case "short-timeout":
				s.ShortTimeout++
			case "low-failure-threshold":
				s.LowFailureThreshold++
			case "high-failure-threshold":
				s.HighFailureThreshold++
			}
		}
	}
}

// generateProbeRecommendations produces actionable advice.
func generateProbeRecommendations(result ProbeAuditResult) []string {
	var recs []string
	s := result.Summary

	if s.MissingReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing readiness probes — traffic may reach unready pods", s.MissingReadiness))
	}
	if s.MissingLiveness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing liveness probes — hung containers won't be restarted", s.MissingLiveness))
	}
	if s.AggressiveProbes > 0 {
		recs = append(recs, fmt.Sprintf("%d probe(s) are too aggressive (period < 5s) — may cause excessive API load", s.AggressiveProbes))
	}
	if s.ShortTimeout > 0 {
		recs = append(recs, fmt.Sprintf("%d probe(s) have short timeout (< 2s) — may fail under latency spikes", s.ShortTimeout))
	}
	if s.LowFailureThreshold > 0 {
		recs = append(recs, fmt.Sprintf("%d probe(s) have low failure threshold (< 3) — too sensitive to transient errors", s.LowFailureThreshold))
	}
	if s.Score < 50 {
		recs = append(recs, fmt.Sprintf("Probe effectiveness score is %d/100 — review probe configurations across workloads", s.Score))
	}

	return recs
}

func probeSevRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

// Ensure appsv1 is imported.
var _ appsv1.DeploymentSpec = appsv1.DeploymentSpec{}
