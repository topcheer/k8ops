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

// PCResult is the health probe compliance analysis.
type PCResult struct {
	ScannedAt        time.Time `json:"scannedAt"`
	Summary          PCSummary `json:"summary"`
	ByWorkload       []PCEntry `json:"byWorkload"`
	MissingReadiness []PCEntry `json:"missingReadiness"`
	MissingLiveness  []PCEntry `json:"missingLiveness"`
	Misconfigured    []PCEntry `json:"misconfigured"`
	Issues           []PCIssue `json:"issues"`
	Recommendations  []string  `json:"recommendations"`
}

// PCSummary aggregates probe compliance.
type PCSummary struct {
	TotalContainers    int `json:"totalContainers"`
	HasLiveness        int `json:"hasLiveness"`
	HasReadiness       int `json:"hasReadiness"`
	HasStartup         int `json:"hasStartup"`
	MissingLiveness    int `json:"missingLiveness"`
	MissingReadiness   int `json:"missingReadiness"`
	TCPSocketProbes    int `json:"tcpSocketProbes"`
	NoProbeAtAll       int `json:"noProbeAtAll"`
	MisconfiguredCount int `json:"misconfiguredCount"`
	HealthScore        int `json:"healthScore"`
}

// PCEntry describes probe compliance for one container.
type PCEntry struct {
	Workload           string    `json:"workload"`
	Namespace          string    `json:"namespace"`
	Kind               string    `json:"kind"`
	Container          string    `json:"container"`
	HasLiveness        bool      `json:"hasLiveness"`
	HasReadiness       bool      `json:"hasReadiness"`
	HasStartup         bool      `json:"hasStartup"`
	LivenessProbeType  string    `json:"livenessProbeType,omitempty"`
	ReadinessProbeType string    `json:"readinessProbeType,omitempty"`
	LivenessProbe      *PCDetail `json:"livenessProbe,omitempty"`
	ReadinessProbe     *PCDetail `json:"readinessProbe,omitempty"`
	StartupProbe       *PCDetail `json:"startupProbe,omitempty"`
	Issues             []string  `json:"issues,omitempty"`
	RiskLevel          string    `json:"riskLevel"`
}

// PCDetail describes a configured probe.
type PCDetail struct {
	Type             string `json:"type"`
	Path             string `json:"path,omitempty"`
	Port             int32  `json:"port"`
	InitialDelay     int32  `json:"initialDelaySeconds"`
	Period           int32  `json:"periodSeconds"`
	Timeout          int32  `json:"timeoutSeconds"`
	SuccessThreshold int32  `json:"successThreshold"`
	FailureThreshold int32  `json:"failureThreshold"`
}

// PCIssue is a detected probe problem.
type PCIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleProbeCompliance audits health probe configuration across workloads.
// GET /api/deployment/probe-compliance
func (s *Server) handleProbeCompliance(w http.ResponseWriter, r *http.Request) {
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

	result := PCResult{ScannedAt: time.Now()}

	for _, dep := range deployments.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			entry := PCEntry{
				Workload:  dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Container: c.Name,
			}

			result.Summary.TotalContainers++

			// Liveness
			if c.LivenessProbe != nil {
				entry.HasLiveness = true
				result.Summary.HasLiveness++
				entry.LivenessProbe = pcProbeToDetail(c.LivenessProbe)
				entry.LivenessProbeType = entry.LivenessProbe.Type
				if entry.LivenessProbe.Type == "tcpSocket" {
					result.Summary.TCPSocketProbes++
				}
			} else {
				result.Summary.MissingLiveness++
				entry.Issues = append(entry.Issues, "Missing liveness probe")
				result.Issues = append(result.Issues, PCIssue{
					Severity: "warning", Type: "missing-liveness",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s has NO liveness probe", c.Name, dep.Namespace, dep.Name),
				})
			}

			// Readiness
			if c.ReadinessProbe != nil {
				entry.HasReadiness = true
				result.Summary.HasReadiness++
				entry.ReadinessProbe = pcProbeToDetail(c.ReadinessProbe)
				entry.ReadinessProbeType = entry.ReadinessProbe.Type
			} else {
				result.Summary.MissingReadiness++
				entry.Issues = append(entry.Issues, "Missing readiness probe")
				result.Issues = append(result.Issues, PCIssue{
					Severity: "critical", Type: "missing-readiness",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s has NO readiness probe — traffic sent to unhealthy pods", c.Name, dep.Namespace, dep.Name),
				})
			}

			// Startup
			if c.StartupProbe != nil {
				entry.HasStartup = true
				result.Summary.HasStartup++
				entry.StartupProbe = pcProbeToDetail(c.StartupProbe)
			}

			// No probe at all
			if !entry.HasLiveness && !entry.HasReadiness {
				result.Summary.NoProbeAtAll++
				result.Issues = append(result.Issues, PCIssue{
					Severity: "critical", Type: "no-probes",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s has ZERO probes", c.Name, dep.Namespace, dep.Name),
				})
			}

			// Misconfiguration check
			misconfigIssues := pcCheckMisconfigured(c)
			entry.Issues = append(entry.Issues, misconfigIssues...)
			if len(misconfigIssues) > 0 {
				result.Summary.MisconfiguredCount++
				result.Misconfigured = append(result.Misconfigured, entry)
			}

			entry.RiskLevel = pcAssessRisk(entry)

			if !entry.HasReadiness {
				result.MissingReadiness = append(result.MissingReadiness, entry)
			}
			if !entry.HasLiveness {
				result.MissingLiveness = append(result.MissingLiveness, entry)
			}

			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return pcRiskRank(result.ByWorkload[i].RiskLevel) < pcRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return pcIssueRank(result.Issues[i].Severity) < pcIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = pcScore(result.Summary)
	result.Recommendations = pcGenRecs(result.Summary, result.MissingReadiness, result.MissingLiveness)

	writeJSON(w, result)
}

// pcProbeToDetail converts a corev1.Probe to PCDetail.
func pcProbeToDetail(p *corev1.Probe) *PCDetail {
	d := &PCDetail{
		InitialDelay:     p.InitialDelaySeconds,
		Period:           p.PeriodSeconds,
		Timeout:          p.TimeoutSeconds,
		SuccessThreshold: p.SuccessThreshold,
		FailureThreshold: p.FailureThreshold,
	}

	if p.HTTPGet != nil {
		d.Type = "httpGet"
		d.Path = p.HTTPGet.Path
		if p.HTTPGet.Port.IntValue() > 0 {
			d.Port = int32(p.HTTPGet.Port.IntValue())
		}
	} else if p.TCPSocket != nil {
		d.Type = "tcpSocket"
		if p.TCPSocket.Port.IntValue() > 0 {
			d.Port = int32(p.TCPSocket.Port.IntValue())
		}
	} else if p.Exec != nil {
		d.Type = "exec"
		if len(p.Exec.Command) > 0 {
			d.Path = p.Exec.Command[0]
		}
	}

	if d.Period == 0 {
		d.Period = 10
	}
	if d.Timeout == 0 {
		d.Timeout = 1
	}
	if d.SuccessThreshold == 0 {
		d.SuccessThreshold = 1
	}
	if d.FailureThreshold == 0 {
		d.FailureThreshold = 3
	}

	return d
}

// pcCheckMisconfigured identifies bad probe configurations.
func pcCheckMisconfigured(c corev1.Container) []string {
	var issues []string

	if p := c.LivenessProbe; p != nil {
		if p.InitialDelaySeconds > 120 {
			issues = append(issues, fmt.Sprintf("Liveness initialDelay=%ds (>120s)", p.InitialDelaySeconds))
		}
		if p.PeriodSeconds > 60 {
			issues = append(issues, fmt.Sprintf("Liveness period=%ds (>60s)", p.PeriodSeconds))
		}
		if p.FailureThreshold > 10 {
			issues = append(issues, fmt.Sprintf("Liveness failureThreshold=%d (>10)", p.FailureThreshold))
		}
		if p.TimeoutSeconds > 10 {
			issues = append(issues, fmt.Sprintf("Liveness timeout=%ds (>10s)", p.TimeoutSeconds))
		}
		if p.SuccessThreshold > 1 {
			issues = append(issues, "Liveness successThreshold>1")
		}
	}

	if p := c.ReadinessProbe; p != nil {
		if p.InitialDelaySeconds > 180 {
			issues = append(issues, fmt.Sprintf("Readiness initialDelay=%ds (>180s)", p.InitialDelaySeconds))
		}
		if p.PeriodSeconds > 30 {
			issues = append(issues, fmt.Sprintf("Readiness period=%ds (>30s)", p.PeriodSeconds))
		}
		if p.TimeoutSeconds > 10 {
			issues = append(issues, fmt.Sprintf("Readiness timeout=%ds (>10s)", p.TimeoutSeconds))
		}
	}

	return issues
}

func pcAssessRisk(entry PCEntry) string {
	if !entry.HasReadiness && !entry.HasLiveness {
		return "critical"
	}
	if !entry.HasReadiness {
		return "critical"
	}
	if !entry.HasLiveness {
		return "high"
	}
	if len(entry.Issues) > 2 {
		return "medium"
	}
	return "low"
}

func pcScore(s PCSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.NoProbeAtAll * 20
	score -= s.MissingReadiness * 12
	score -= s.MissingLiveness * 5
	score -= s.MisconfiguredCount * 3
	if score < 0 {
		score = 0
	}
	return score
}

func pcGenRecs(s PCSummary, missingReady []PCEntry, missingLive []PCEntry) []string {
	var recs []string

	if s.NoProbeAtAll > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have ZERO probes — add liveness + readiness immediately", s.NoProbeAtAll))
	}
	if s.MissingReadiness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing readiness probe — traffic sent to unhealthy pods", s.MissingReadiness))
	}
	if s.MissingLiveness > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing liveness probe — stale containers won't restart", s.MissingLiveness))
	}
	if s.TCPSocketProbes > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) use tcpSocket probes — prefer httpGet for app-level health checks", s.TCPSocketProbes))
	}
	if s.HasStartup == 0 && s.HasLiveness > 0 {
		recs = append(recs, "No startup probes detected — add startupProbe for slow-starting apps")
	}
	if s.MisconfiguredCount > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have misconfigured probes — review delays, periods, and thresholds", s.MisconfiguredCount))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Probe compliance health score is %d/100 — multiple containers lack proper health checks", s.HealthScore))
	}
	if s.MissingReadiness == 0 && s.MissingLiveness == 0 && s.NoProbeAtAll == 0 {
		recs = append(recs, "All containers have liveness + readiness probes — excellent health monitoring coverage")
	}

	return recs
}

func pcRiskRank(level string) int {
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

func pcIssueRank(s string) int {
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
