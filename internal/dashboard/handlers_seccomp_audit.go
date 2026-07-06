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

// SPAResult is the seccomp profile & PSS restricted compliance analysis.
type SPAResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         SPASummary `json:"summary"`
	ByWorkload      []SPAEntry `json:"byWorkload"`
	NonCompliant    []SPAEntry `json:"nonCompliant"` // fails restricted PSS
	NoSeccomp       []SPAEntry `json:"noSeccomp"`    // missing seccomp profile
	HasAllCaps      []SPAEntry `json:"hasAllCaps"`   // didn't drop ALL capabilities
	CanEscalate     []SPAEntry `json:"canEscalate"`  // allowPrivilegeEscalation not false
	RunsAsRoot      []SPAEntry `json:"runsAsRoot"`   // no runAsNonRoot or runAsUser=0
	Issues          []SPIssue  `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// SPASummary aggregates seccomp/PSS statistics.
type SPASummary struct {
	TotalContainers int `json:"totalContainers"`
	HasSeccomp      int `json:"hasSeccomp"`      // RuntimeDefault or Localhost
	NoSeccomp       int `json:"noSeccomp"`       // missing or Unconfined
	DroppedAllCaps  int `json:"droppedAllCaps"`  // drop: ALL
	NotDroppedAll   int `json:"notDroppedAll"`   // didn't drop ALL
	CanEscalate     int `json:"canEscalate"`     // allowPrivilegeEscalation != false
	RunsAsRoot      int `json:"runsAsRoot"`      // runAsNonRoot != true or runAsUser=0
	ReadOnlyRootfs  int `json:"readOnlyRootfs"`  // readOnlyRootFilesystem = true
	PSSRestrictedOK int `json:"pssRestrictedOK"` // passes all restricted checks
	PSSBaselineFail int `json:"pssBaselineFail"` // fails baseline (e.g., privileged)
	HardeningScore  int `json:"hardeningScore"`  // 0-100
}

// SPAEntry describes one container's seccomp & PSS compliance.
type SPAEntry struct {
	Workload          string   `json:"workload"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	Container         string   `json:"container"`
	SeccompProfile    string   `json:"seccompProfile"` // RuntimeDefault / Localhost / Unconfined / unset
	CapabilitiesDrop  []string `json:"capabilitiesDrop,omitempty"`
	CapabilitiesAdd   []string `json:"capabilitiesAdd,omitempty"`
	DroppedAll        bool     `json:"droppedAll"`
	AllowPrivEscalate *bool    `json:"allowPrivilegeEscalation"`
	RunAsNonRoot      *bool    `json:"runAsNonRoot"`
	RunAsUser         *int64   `json:"runAsUser,omitempty"`
	ReadOnlyRootfs    *bool    `json:"readOnlyRootFilesystem"`
	IsPrivileged      bool     `json:"isPrivileged"`
	Violations        []string `json:"violations,omitempty"`
	PSSLevel          string   `json:"pssLevel"` // restricted / baseline / privileged
	RiskLevel         string   `json:"riskLevel"`
}

// SPIssue is a detected hardening problem.
type SPIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleSeccompAudit audits seccomp profiles and PSS restricted compliance.
// GET /api/security/seccomp-audit
func (s *Server) handleSeccompAudit(w http.ResponseWriter, r *http.Request) {
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

	result := SPAResult{ScannedAt: time.Now()}

	// Known dangerous capabilities (should never be added)
	dangerousCaps := map[string]bool{
		"SYS_ADMIN": true, "SYS_MODULE": true, "SYS_PTRACE": true,
		"SYS_RAWIO": true, "NET_ADMIN": true, "NET_RAW": true,
		"DAC_OVERRIDE": true, "SETUID": true, "SETGID": true,
		"CHOWN": true, "FOWNER": true,
	}

	for _, dep := range deployments.Items {
		podSC := dep.Spec.Template.Spec.SecurityContext
		podRunAsNonRoot := false
		podRunAsUser := int64(-1)
		podSeccompType := ""

		if podSC != nil {
			if podSC.RunAsNonRoot != nil && *podSC.RunAsNonRoot {
				podRunAsNonRoot = true
			}
			if podSC.RunAsUser != nil {
				podRunAsUser = *podSC.RunAsUser
			}
			if podSC.SeccompProfile != nil {
				podSeccompType = string(podSC.SeccompProfile.Type)
			}
		}

		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			entry := SPAEntry{
				Workload:  dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Container: c.Name,
			}

			// Seccomp profile (container-level overrides pod-level)
			seccompType := podSeccompType
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				seccompType = string(c.SecurityContext.SeccompProfile.Type)
			}
			entry.SeccompProfile = seccompType
			if seccompType == "" {
				entry.SeccompProfile = "unset"
				result.Summary.NoSeccomp++
			} else if seccompType == "Unconfined" {
				result.Summary.NoSeccomp++
			} else {
				result.Summary.HasSeccomp++
			}

			// Capabilities
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Drop {
					entry.CapabilitiesDrop = append(entry.CapabilitiesDrop, string(cap))
					if string(cap) == "ALL" {
						entry.DroppedAll = true
					}
				}
				for _, cap := range c.SecurityContext.Capabilities.Add {
					entry.CapabilitiesAdd = append(entry.CapabilitiesAdd, string(cap))
				}
				if entry.DroppedAll {
					result.Summary.DroppedAllCaps++
				} else {
					result.Summary.NotDroppedAll++
				}
			} else {
				result.Summary.NotDroppedAll++
			}

			// AllowPrivilegeEscalation
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil {
				entry.AllowPrivEscalate = c.SecurityContext.AllowPrivilegeEscalation
				if *c.SecurityContext.AllowPrivilegeEscalation {
					result.Summary.CanEscalate++
				}
			} else {
				t := true // default is true if not explicitly set to false
				entry.AllowPrivEscalate = &t
				result.Summary.CanEscalate++
			}

			// Privileged
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil {
				entry.IsPrivileged = *c.SecurityContext.Privileged
			}

			// RunAsNonRoot / RunAsUser
			containerRunAsNonRoot := podRunAsNonRoot
			containerRunAsUser := podRunAsUser
			if c.SecurityContext != nil {
				if c.SecurityContext.RunAsNonRoot != nil {
					entry.RunAsNonRoot = c.SecurityContext.RunAsNonRoot
					containerRunAsNonRoot = *c.SecurityContext.RunAsNonRoot
				} else {
					entry.RunAsNonRoot = &podRunAsNonRoot
				}
				if c.SecurityContext.RunAsUser != nil {
					entry.RunAsUser = c.SecurityContext.RunAsUser
					containerRunAsUser = *c.SecurityContext.RunAsUser
				}
			} else {
				entry.RunAsNonRoot = &podRunAsNonRoot
			}

			if !containerRunAsNonRoot || containerRunAsUser == 0 {
				result.Summary.RunsAsRoot++
			}

			// ReadOnlyRootFilesystem
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil {
				entry.ReadOnlyRootfs = c.SecurityContext.ReadOnlyRootFilesystem
				if *c.SecurityContext.ReadOnlyRootFilesystem {
					result.Summary.ReadOnlyRootfs++
				}
			}

			// Check for dangerous added capabilities
			for _, cap := range entry.CapabilitiesAdd {
				upperCap := strings.ToUpper(string(cap))
				if dangerousCaps[upperCap] {
					entry.Violations = append(entry.Violations, fmt.Sprintf("Adds dangerous capability %s", upperCap))
					result.Issues = append(result.Issues, SPIssue{
						Severity: "warning", Type: "dangerous-capability",
						Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
						Message:  fmt.Sprintf("Container %s in %s/%s adds %s — grants excessive kernel access", c.Name, dep.Namespace, dep.Name, upperCap),
					})
				}
			}

			// Build violations list for PSS compliance
			if entry.IsPrivileged {
				entry.Violations = append(entry.Violations, "Privileged container")
				result.Summary.PSSBaselineFail++
				result.Issues = append(result.Issues, SPIssue{
					Severity: "critical", Type: "privileged",
					Resource: fmt.Sprintf("%s/%s/%s", dep.Namespace, dep.Name, c.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s is privileged — full host access", c.Name, dep.Namespace, dep.Name),
				})
			}
			if entry.SeccompProfile == "unset" || entry.SeccompProfile == "Unconfined" {
				entry.Violations = append(entry.Violations, "Missing seccomp profile")
				result.NoSeccomp = append(result.NoSeccomp, entry)
			}
			if !entry.DroppedAll {
				entry.Violations = append(entry.Violations, "Did not drop ALL capabilities")
				result.HasAllCaps = append(result.HasAllCaps, entry)
			}
			if entry.AllowPrivEscalate != nil && *entry.AllowPrivEscalate {
				entry.Violations = append(entry.Violations, "allowPrivilegeEscalation not set to false")
				result.CanEscalate = append(result.CanEscalate, entry)
			}
			if !containerRunAsNonRoot || containerRunAsUser == 0 {
				entry.Violations = append(entry.Violations, "Runs as root (no runAsNonRoot=true or runAsUser=0)")
				result.RunsAsRoot = append(result.RunsAsRoot, entry)
			}

			// Determine PSS level
			entry.PSSLevel, entry.RiskLevel = spAssessPSS(entry)

			if entry.PSSLevel == "restricted" && len(entry.Violations) == 0 {
				result.Summary.PSSRestrictedOK++
			} else {
				result.NonCompliant = append(result.NonCompliant, entry)
			}

			result.ByWorkload = append(result.ByWorkload, entry)
		}
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return spRiskRank(result.ByWorkload[i].RiskLevel) < spRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.NonCompliant, func(i, j int) bool {
		return len(result.NonCompliant[i].Violations) > len(result.NonCompliant[j].Violations)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return spIssueRank(result.Issues[i].Severity) < spIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HardeningScore = spScore(result.Summary)
	result.Recommendations = spGenRecs(result.Summary, result.NoSeccomp, result.CanEscalate)

	writeJSON(w, result)
}

// spAssessPSS determines Pod Security Standards level.
func spAssessPSS(entry SPAEntry) (level, risk string) {
	if entry.IsPrivileged {
		return "privileged", "critical"
	}

	violations := len(entry.Violations)
	if violations == 0 {
		return "restricted", "low"
	}

	switch {
	case violations >= 4:
		return "baseline", "high"
	case violations >= 2:
		return "baseline", "medium"
	default:
		return "baseline", "low"
	}
}

// spScore computes 0-100.
func spScore(s SPASummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.NoSeccomp * 8
	score -= s.PSSBaselineFail * 20
	score -= s.NotDroppedAll * 4
	score -= s.CanEscalate * 6
	score -= s.RunsAsRoot * 3
	if score < 0 {
		score = 0
	}
	return score
}

// spGenRecs produces actionable advice.
func spGenRecs(s SPASummary, noSeccomp []SPAEntry, canEscalate []SPAEntry) []string {
	var recs []string

	if s.PSSBaselineFail > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) fail PSS baseline (privileged) — remove privileged flag immediately", s.PSSBaselineFail))
	}
	if s.NoSeccomp > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing seccomp profile — set seccompProfile: type: RuntimeDefault", s.NoSeccomp))
	}
	if s.CanEscalate > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) allow privilege escalation — set allowPrivilegeEscalation: false", s.CanEscalate))
	}
	if s.NotDroppedAll > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) didn't drop ALL capabilities — add capabilities.drop: [ALL], then add back only needed ones", s.NotDroppedAll))
	}
	if s.RunsAsRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) run as root — set runAsNonRoot: true and runAsUser to a non-zero UID", s.RunsAsRoot))
	}
	if s.ReadOnlyRootfs < s.TotalContainers {
		recs = append(recs, fmt.Sprintf("%d of %d container(s) don't use readOnlyRootFilesystem — enable for tamper resistance", s.TotalContainers-s.ReadOnlyRootfs, s.TotalContainers))
	}
	if s.PSSRestrictedOK < s.TotalContainers {
		pct := float64(s.PSSRestrictedOK) / float64(s.TotalContainers) * 100
		recs = append(recs, fmt.Sprintf("Only %.0f%% of containers pass PSS restricted — target 100%% for hardened deployments", pct))
	}
	if s.HardeningScore < 60 {
		recs = append(recs, fmt.Sprintf("Container hardening score is %d/100 — apply Pod Security Admission with restricted level", s.HardeningScore))
	}
	if s.PSSRestrictedOK == s.TotalContainers && s.TotalContainers > 0 {
		recs = append(recs, "All containers pass PSS restricted — excellent security hardening")
	}

	return recs
}

func spRiskRank(level string) int {
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

func spIssueRank(s string) int {
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
