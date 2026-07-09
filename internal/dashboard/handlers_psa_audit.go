package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementLevel represents a Pod Security Admission enforcement level.
type PSAEnforcementLevel string

const (
	PSALevelPrivileged PSAEnforcementLevel = "privileged"
	PSALevelBaseline   PSAEnforcementLevel = "baseline"
	PSALevelRestricted PSAEnforcementLevel = "restricted"
	PSALevelNone       PSAEnforcementLevel = "none" // no PSA label set
)

// PSAAuditResult is the full Pod Security Admission enforcement analysis.
type PSAAuditResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         PSAAuditSummary `json:"summary"`
	Namespaces      []PSANamespace  `json:"namespaces"`
	Violations      []PSAViolation  `json:"violations"`
	Recommendations []string        `json:"recommendations"`
}

// PSAAuditSummary aggregates cluster-wide PSA statistics.
type PSAAuditSummary struct {
	TotalNamespaces    int `json:"totalNamespaces"`
	UserNamespaces     int `json:"userNamespaces"`     // non-system namespaces
	Enforced           int `json:"enforced"`           // have enforce label
	NotEnforced        int `json:"notEnforced"`        // missing enforce label
	BaselineEnforced   int `json:"baselineEnforced"`   // enforce=baseline
	RestrictedEnforced int `json:"restrictedEnforced"` // enforce=restricted
	PrivilegedAllowed  int `json:"privilegedAllowed"`  // enforce=privileged or none
	HasAuditMode       int `json:"hasAuditMode"`       // have audit label
	HasWarnMode        int `json:"hasWarnMode"`        // have warn label
	ViolationCount     int `json:"violationCount"`     // pods violating their namespace PSA
	EnforcementScore   int `json:"enforcementScore"`   // 0-100
}

// PSANamespace describes the PSA configuration for one namespace.
type PSANamespace struct {
	Name           string              `json:"name"`
	IsSystem       bool                `json:"isSystem"`
	EnforceLevel   PSAEnforcementLevel `json:"enforceLevel"`
	AuditLevel     PSAEnforcementLevel `json:"auditLevel"`
	WarnLevel      PSAEnforcementLevel `json:"warnLevel"`
	EnforceVersion string              `json:"enforceVersion,omitempty"`
	PodCount       int                 `json:"podCount"`
	ViolatingPods  int                 `json:"violatingPods"`
	RiskLevel      string              `json:"riskLevel"` // critical, high, medium, low
}

// PSAViolation describes a pod that violates its namespace PSA policy.
type PSAViolation struct {
	Namespace      string              `json:"namespace"`
	PodName        string              `json:"podName"`
	Container      string              `json:"container,omitempty"`
	NamespaceLevel PSAEnforcementLevel `json:"namespaceLevel"`
	Violation      string              `json:"violation"` // privileged, hostNetwork, etc.
	Detail         string              `json:"detail"`
	Severity       string              `json:"severity"`
}

// PSA annotation key prefixes
const (
	psaEnforcePrefix = "pod-security.kubernetes.io/enforce"
	psaAuditPrefix   = "pod-security.kubernetes.io/audit"
	psaWarnPrefix    = "pod-security.kubernetes.io/warn"
)

// handlePSAAudit analyzes Pod Security Admission enforcement across all namespaces.
// GET /api/security/psa-audit
func (s *Server) handlePSAAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	namespaces, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build pod-per-namespace map
	podsByNs := map[string][]corev1.Pod{}
	for _, pod := range pods.Items {
		podsByNs[pod.Namespace] = append(podsByNs[pod.Namespace], pod)
	}

	now := time.Now()
	result := PSAAuditResult{ScannedAt: now}
	result.Summary.TotalNamespaces = len(namespaces.Items)

	for _, ns := range namespaces.Items {
		isSys := isSystemNamespace(ns.Name)
		enforce := getPSALevel(ns.Annotations, psaEnforcePrefix)
		audit := getPSALevel(ns.Annotations, psaAuditPrefix)
		warn := getPSALevel(ns.Annotations, psaWarnPrefix)
		enforceVer := ns.Annotations[psaEnforcePrefix+"-version"]

		nsPods := podsByNs[ns.Name]
		podCount := len(nsPods)
		violating := 0

		// Check for violations only if enforce level is baseline or restricted
		if enforce == PSALevelBaseline || enforce == PSALevelRestricted {
			for _, pod := range nsPods {
				if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
					continue
				}
				violations := checkPSAViolations(&pod, enforce)
				if len(violations) > 0 {
					violating++
					result.Summary.ViolationCount++
					for _, v := range violations {
						result.Violations = append(result.Violations, PSAViolation{
							Namespace:      ns.Name,
							PodName:        pod.Name,
							Container:      v.container,
							NamespaceLevel: enforce,
							Violation:      v.category,
							Detail:         v.detail,
							Severity:       v.severity,
						})
					}
				}
			}
		}

		// Risk assessment
		risk := assessPSARisk(enforce, isSys, violating)

		nsEntry := PSANamespace{
			Name:           ns.Name,
			IsSystem:       isSys,
			EnforceLevel:   enforce,
			AuditLevel:     audit,
			WarnLevel:      warn,
			EnforceVersion: enforceVer,
			PodCount:       podCount,
			ViolatingPods:  violating,
			RiskLevel:      risk,
		}

		// Update summary
		if !isSys {
			result.Summary.UserNamespaces++
		}
		if enforce != PSALevelNone {
			result.Summary.Enforced++
		} else {
			result.Summary.NotEnforced++
		}
		switch enforce {
		case PSALevelBaseline:
			result.Summary.BaselineEnforced++
		case PSALevelRestricted:
			result.Summary.RestrictedEnforced++
		case PSALevelPrivileged, PSALevelNone:
			result.Summary.PrivilegedAllowed++
		}
		if audit != PSALevelNone {
			result.Summary.HasAuditMode++
		}
		if warn != PSALevelNone {
			result.Summary.HasWarnMode++
		}

		result.Namespaces = append(result.Namespaces, nsEntry)
	}

	// Sort namespaces: non-enforced first, then by violation count
	sort.Slice(result.Namespaces, func(i, j int) bool {
		a, b := result.Namespaces[i], result.Namespaces[j]
		if a.IsSystem != b.IsSystem {
			return !a.IsSystem // user namespaces first
		}
		if a.EnforceLevel != b.EnforceLevel {
			return a.EnforceLevel == PSALevelNone // unenforced first
		}
		return a.ViolatingPods > b.ViolatingPods
	})

	// Sort violations by severity
	sort.Slice(result.Violations, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Violations[i].Severity] < sevOrder[result.Violations[j].Severity]
	})
	if len(result.Violations) > 50 {
		result.Violations = result.Violations[:50]
	}

	result.Summary.EnforcementScore = psaScore(result.Summary)
	result.Recommendations = psaRecommendations(&result)

	writeJSON(w, result)
}

// psaViolationDetail describes a single violation found in a pod.
type psaViolationDetail struct {
	category  string
	container string
	detail    string
	severity  string
}

// getPSALevel extracts the PSA enforcement level from namespace annotations.
func getPSALevel(annotations map[string]string, prefix string) PSAEnforcementLevel {
	val, ok := annotations[prefix]
	if !ok || val == "" {
		return PSALevelNone
	}
	switch PSAEnforcementLevel(val) {
	case PSALevelPrivileged, PSALevelBaseline, PSALevelRestricted:
		return PSAEnforcementLevel(val)
	default:
		return PSALevelNone
	}
}

// checkPSAViolations checks if a pod violates the given PSA level.
// Returns a list of violations found.
func checkPSAViolations(pod *corev1.Pod, level PSAEnforcementLevel) []psaViolationDetail {
	var violations []psaViolationDetail

	checkBaseline := level == PSALevelBaseline || level == PSALevelRestricted

	for _, c := range pod.Spec.Containers {
		sc := c.SecurityContext

		// Baseline checks
		if checkBaseline {
			// Privileged
			if sc != nil && sc.Privileged != nil && *sc.Privileged {
				violations = append(violations, psaViolationDetail{
					category:  "privileged",
					container: c.Name,
					detail:    fmt.Sprintf("Container %s is privileged — violates %s policy", c.Name, level),
					severity:  "critical",
				})
			}

			// Host namespaces
			if pod.Spec.HostNetwork {
				violations = append(violations, psaViolationDetail{
					category:  "host-network",
					container: c.Name,
					detail:    fmt.Sprintf("Pod uses hostNetwork — violates %s policy", level),
					severity:  "critical",
				})
			}
			if pod.Spec.HostPID {
				violations = append(violations, psaViolationDetail{
					category:  "host-pid",
					container: c.Name,
					detail:    fmt.Sprintf("Pod uses hostPID — violates %s policy", level),
					severity:  "critical",
				})
			}
			if pod.Spec.HostIPC {
				violations = append(violations, psaViolationDetail{
					category:  "host-ipc",
					container: c.Name,
					detail:    fmt.Sprintf("Pod uses hostIPC — violates %s policy", level),
					severity:  "critical",
				})
			}

			// Host path volumes
			for _, vol := range pod.Spec.Volumes {
				if vol.HostPath != nil {
					violations = append(violations, psaViolationDetail{
						category:  "host-path",
						container: c.Name,
						detail:    fmt.Sprintf("Volume %s uses hostPath — violates %s policy", vol.Name, level),
						severity:  "high",
					})
					break
				}
			}

			// Dangerous capabilities
			if sc != nil && sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Add {
					if isDangerousCapability(string(cap)) {
						violations = append(violations, psaViolationDetail{
							category:  "dangerous-capability",
							container: c.Name,
							detail:    fmt.Sprintf("Container %s adds capability %s — violates %s policy", c.Name, cap, level),
							severity:  "high",
						})
					}
				}
			}

			// Host port
			for _, p := range c.Ports {
				if p.HostPort != 0 {
					violations = append(violations, psaViolationDetail{
						category:  "host-port",
						container: c.Name,
						detail:    fmt.Sprintf("Container %s uses hostPort %d — violates %s policy", c.Name, p.HostPort, level),
						severity:  "medium",
					})
				}
			}
		}

		// Restricted-only checks
		if level == PSALevelRestricted {
			// Must run as non-root
			runsRoot := true
			if sc != nil && sc.RunAsNonRoot != nil && *sc.RunAsNonRoot {
				runsRoot = false
			}
			if sc != nil && sc.RunAsUser != nil && *sc.RunAsUser != 0 {
				runsRoot = false
			}
			if pod.Spec.SecurityContext != nil {
				psc := pod.Spec.SecurityContext
				if psc.RunAsNonRoot != nil && *psc.RunAsNonRoot {
					runsRoot = false
				}
				if psc.RunAsUser != nil && *psc.RunAsUser != 0 {
					runsRoot = false
				}
			}
			if runsRoot {
				violations = append(violations, psaViolationDetail{
					category:  "runs-as-root",
					container: c.Name,
					detail:    fmt.Sprintf("Container %s may run as root — violates restricted policy", c.Name),
					severity:  "high",
				})
			}

			// Must allow privilege escalation = false
			if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
				violations = append(violations, psaViolationDetail{
					category:  "privilege-escalation",
					container: c.Name,
					detail:    fmt.Sprintf("Container %s does not set allowPrivilegeEscalation=false — violates restricted policy", c.Name),
					severity:  "high",
				})
			}

			// Must drop ALL capabilities
			droppedAll := false
			if sc != nil && sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Drop {
					if strings.EqualFold(string(cap), "ALL") {
						droppedAll = true
						break
					}
				}
			}
			if !droppedAll {
				violations = append(violations, psaViolationDetail{
					category:  "capabilities-not-dropped",
					container: c.Name,
					detail:    fmt.Sprintf("Container %s does not drop ALL capabilities — violates restricted policy", c.Name),
					severity:  "medium",
				})
			}

			// Seccomp profile required
			hasSeccomp := false
			if sc != nil && sc.SeccompProfile != nil {
				hasSeccomp = true
			}
			if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
				hasSeccomp = true
			}
			if !hasSeccomp {
				violations = append(violations, psaViolationDetail{
					category:  "missing-seccomp",
					container: c.Name,
					detail:    fmt.Sprintf("Container %s missing seccompProfile — violates restricted policy", c.Name),
					severity:  "medium",
				})
			}
		}
	}

	return violations
}

// isDangerousCapability checks if a Linux capability is considered dangerous per PSA baseline.
func isDangerousCapability(cap string) bool {
	dangerous := []string{
		"CAP_SYS_ADMIN", "CAP_SYS_MODULE", "CAP_SYS_PTRACE", "CAP_SYS_RAWIO",
		"CAP_SYS_BOOT", "CAP_SYS_TIME", "CAP_SYS_TTY_CONFIG",
		"CAP_NET_ADMIN", "CAP_NET_RAW",
		"CAP_DAC_READ_SEARCH", "CAP_LINUX_IMMUTABLE",
		"CAP_IPC_LOCK", "CAP_IPC_OWNER",
		"CAP_BLOCK_SUSPEND", "CAP_WAKE_ALARM",
		"CAP_AUDIT_CONTROL", "CAP_AUDIT_WRITE",
		"CAP_MAC_ADMIN", "CAP_MAC_OVERRIDE",
		"CAP_SETFCAP", "CAP_SETPCAP", "CAP_SETUID", "CAP_SETGID",
	}
	for _, d := range dangerous {
		if strings.EqualFold(cap, d) || strings.EqualFold(cap, strings.TrimPrefix(d, "CAP_")) {
			return true
		}
	}
	return false
}

// assessPSARisk determines the risk level of a namespace based on PSA config.
func assessPSARisk(enforce PSAEnforcementLevel, isSystem bool, violatingPods int) string {
	if isSystem {
		if violatingPods > 0 {
			return "medium"
		}
		return "low"
	}
	switch enforce {
	case PSALevelRestricted:
		if violatingPods > 0 {
			return "medium"
		}
		return "low"
	case PSALevelBaseline:
		if violatingPods > 3 {
			return "high"
		}
		if violatingPods > 0 {
			return "medium"
		}
		return "low"
	case PSALevelPrivileged:
		return "high"
	default: // PSALevelNone
		return "critical"
	}
}

// psaScore computes a 0-100 enforcement score.
func psaScore(s PSAAuditSummary) int {
	if s.UserNamespaces == 0 {
		return 100
	}

	score := 0

	// How many user namespaces have enforcement?
	enforcedRatio := float64(s.Enforced) / float64(s.TotalNamespaces)
	score += int(enforcedRatio * 40)

	// Quality of enforcement (restricted > baseline > privileged/none)
	restrictedRatio := float64(s.RestrictedEnforced) / float64(s.TotalNamespaces)
	score += int(restrictedRatio * 25)

	baselineRatio := float64(s.BaselineEnforced) / float64(s.TotalNamespaces)
	score += int(baselineRatio * 10)

	// Audit/warn mode coverage
	auditRatio := float64(s.HasAuditMode) / float64(s.TotalNamespaces)
	score += int(auditRatio * 10)

	warnRatio := float64(s.HasWarnMode) / float64(s.TotalNamespaces)
	score += int(warnRatio * 5)

	// Penalize violations
	if s.ViolationCount > 0 {
		score -= min(10, s.ViolationCount)
	}

	// Penalize privileged-allowed namespaces
	privRatio := float64(s.PrivilegedAllowed) / float64(s.TotalNamespaces)
	score -= int(privRatio * 10)

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// psaRecommendations generates actionable security recommendations.
func psaRecommendations(r *PSAAuditResult) []string {
	var recs []string

	if r.Summary.NotEnforced > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d namespace(s) have no PSA enforcement — add pod-security.kubernetes.io/enforce=baseline label",
			r.Summary.NotEnforced,
		))
	}

	if r.Summary.PrivilegedAllowed > 0 && r.Summary.RestrictedEnforced == 0 {
		recs = append(recs, "No namespaces enforce restricted policy — consider enforcing restricted for production namespaces")
	}

	if r.Summary.HasAuditMode < r.Summary.Enforced {
		recs = append(recs, "Enable audit mode alongside enforcement for monitoring PSA violations before they're blocked")
	}

	if r.Summary.ViolationCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) violate their namespace PSA policy — fix security contexts or adjust enforcement level",
			r.Summary.ViolationCount,
		))
	}

	if r.Summary.RestrictedEnforced > 0 && r.Summary.RestrictedEnforced < r.Summary.UserNamespaces {
		recs = append(recs, "Some user namespaces enforce restricted but not all — ensure consistent enforcement across similar workloads")
	}

	if len(recs) == 0 {
		recs = append(recs, "PSA enforcement is well configured — all user namespaces have appropriate enforcement levels")
	}

	return recs
}
