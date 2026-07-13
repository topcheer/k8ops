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

// PSSScorecardResult is the Pod Security Standards compliance scorecard.
type PSSScorecardResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PSSSummary          `json:"summary"`
	ByNamespace     []PSSNSStat         `json:"byNamespace"`
	NonCompliant    []PSSContainerEntry `json:"nonCompliant"`
	Issues          []PSSIssue          `json:"issues"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// PSSSummary aggregates PSS compliance statistics.
type PSSSummary struct {
	TotalContainers     int `json:"totalContainers"`
	RestrictedCompliant int `json:"restrictedCompliant"`
	BaselineCompliant   int `json:"baselineCompliant"`
	Privileged          int `json:"privileged"`
	NoRunAsNonRoot      int `json:"noRunAsNonRoot"`
	NoSeccompProfile    int `json:"noSeccompProfile"`
	AllowPrivEscal      int `json:"allowPrivilegeEscalation"`
	NoCapDropAll        int `json:"noCapabilitiesDropAll"`
	NoReadOnlyRootFS    int `json:"noReadOnlyRootFilesystem"`
	HostNetwork         int `json:"hostNetwork"`
	HostPID             int `json:"hostPID"`
	HostIPC             int `json:"hostIPC"`
}

// PSSNSStat per-namespace PSS compliance stats.
type PSSNSStat struct {
	Namespace      string  `json:"namespace"`
	ContainerCount int     `json:"containerCount"`
	NonCompliant   int     `json:"nonCompliant"`
	ComplianceRate float64 `json:"complianceRate"`
}

// PSSContainerEntry describes one container's PSS compliance.
type PSSContainerEntry struct {
	PodName         string   `json:"podName"`
	Namespace       string   `json:"namespace"`
	Container       string   `json:"container"`
	HasRunAsNonRoot bool     `json:"hasRunAsNonRoot"`
	HasSeccomp      bool     `json:"hasSeccompProfile"`
	PrivEscalFalse  bool     `json:"privilegeEscalationFalse"`
	CapsDropAll     bool     `json:"capabilitiesDropAll"`
	ReadOnlyRootFS  bool     `json:"readOnlyRootFilesystem"`
	IsPrivileged    bool     `json:"isPrivileged"`
	Violations      []string `json:"violations"`
	RiskLevel       string   `json:"riskLevel"`
}

// PSSIssue is a detected PSS compliance problem.
type PSSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handlePSSScorecard audits Pod Security Standards compliance across all containers.
// GET /api/security/pss-scorecard
func (s *Server) handlePSSScorecard(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &PSSScorecardResult{
		ScannedAt: time.Now(),
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var nonCompliant []PSSContainerEntry
	var issues []PSSIssue
	nsStats := make(map[string]*PSSNSStat)

	totalContainers := 0
	restrictedCompliant := 0
	baselineCompliant := 0
	privileged := 0
	noRunAsNonRoot := 0
	noSeccomp := 0
	allowPrivEscal := 0
	noCapDropAll := 0
	noReadOnlyRootFS := 0
	hostNetwork := 0
	hostPID := 0
	hostIPC := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		// Check pod-level security context
		podSC := pod.Spec.SecurityContext

		for _, c := range pod.Spec.Containers {
			totalContainers++

			entry := PSSContainerEntry{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Container: c.Name,
			}

			var violations []string
			isPrivileged := false

			// 1. runAsNonRoot (from container or pod level)
			runAsNonRoot := false
			if c.SecurityContext != nil && c.SecurityContext.RunAsNonRoot != nil {
				runAsNonRoot = *c.SecurityContext.RunAsNonRoot
			} else if podSC != nil && podSC.RunAsNonRoot != nil {
				runAsNonRoot = *podSC.RunAsNonRoot
			}
			entry.HasRunAsNonRoot = runAsNonRoot
			if !runAsNonRoot {
				noRunAsNonRoot++
				violations = append(violations, "runAsNonRoot not set")
			}

			// 2. seccompProfile
			hasSeccomp := false
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				hasSeccomp = true
			} else if podSC != nil && podSC.SeccompProfile != nil {
				hasSeccomp = true
			}
			entry.HasSeccomp = hasSeccomp
			if !hasSeccomp {
				noSeccomp++
				violations = append(violations, "seccompProfile not set")
			}

			// 3. allowPrivilegeEscalation must be false
			privEscalFalse := false
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil {
				privEscalFalse = !*c.SecurityContext.AllowPrivilegeEscalation
			}
			entry.PrivEscalFalse = privEscalFalse
			if !privEscalFalse {
				allowPrivEscal++
				violations = append(violations, "allowPrivilegeEscalation not set to false")
			}

			// 4. capabilities.drop ALL
			capsDropAll := false
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, drop := range c.SecurityContext.Capabilities.Drop {
					if string(drop) == "ALL" {
						capsDropAll = true
						break
					}
				}
			}
			entry.CapsDropAll = capsDropAll
			if !capsDropAll {
				noCapDropAll++
				violations = append(violations, "capabilities.drop ALL not set")
			}

			// 5. readOnlyRootFilesystem
			readOnlyRootFS := false
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil {
				readOnlyRootFS = *c.SecurityContext.ReadOnlyRootFilesystem
			}
			entry.ReadOnlyRootFS = readOnlyRootFS
			if !readOnlyRootFS {
				noReadOnlyRootFS++
				violations = append(violations, "readOnlyRootFilesystem not set")
			}

			// 6. privileged
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				isPrivileged = true
				privileged++
				entry.IsPrivileged = true
				violations = append(violations, "container is privileged")
			}

			// 7. Host network/PID/IPC
			if pod.Spec.HostNetwork {
				hostNetwork++
				violations = append(violations, "hostNetwork enabled")
			}
			if pod.Spec.HostPID {
				hostPID++
				violations = append(violations, "hostPID enabled")
			}
			if pod.Spec.HostIPC {
				hostIPC++
				violations = append(violations, "hostIPC enabled")
			}

			// Determine compliance level
			if isPrivileged {
				// Not even baseline compliant
			} else if len(violations) == 0 {
				restrictedCompliant++
			} else if !isPrivileged && hostNetwork == 0 && hostPID == 0 && hostIPC == 0 {
				baselineCompliant++
			}

			entry.Violations = violations
			entry.RiskLevel = assessPSSRisk(entry)

			// Add to non-compliant list if has violations
			if len(violations) > 0 {
				nonCompliant = append(nonCompliant, entry)
			}

			// Critical issues
			if isPrivileged {
				issues = append(issues, PSSIssue{
					Severity: "critical",
					Type:     "privileged-container",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Container %s is privileged — full host access, violates PSS restricted profile", c.Name),
				})
			}
			if !privEscalFalse && !isPrivileged {
				issues = append(issues, PSSIssue{
					Severity: "warning",
					Type:     "allow-priv-escalation",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name),
					Message:  fmt.Sprintf("Container %s does not set allowPrivilegeEscalation=false — may allow privilege escalation", c.Name),
				})
			}

			// Update namespace stats
			if _, ok := nsStats[pod.Namespace]; !ok {
				nsStats[pod.Namespace] = &PSSNSStat{Namespace: pod.Namespace}
			}
			ns := nsStats[pod.Namespace]
			ns.ContainerCount++
			if len(violations) > 0 {
				ns.NonCompliant++
			}
		}
	}

	// Calculate compliance rates and convert to slice
	for _, ns := range nsStats {
		if ns.ContainerCount > 0 {
			ns.ComplianceRate = float64(ns.ContainerCount-ns.NonCompliant) / float64(ns.ContainerCount)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ComplianceRate < result.ByNamespace[j].ComplianceRate
	})

	// Sort non-compliant by risk
	sort.Slice(nonCompliant, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[nonCompliant[i].RiskLevel] < riskOrder[nonCompliant[j].RiskLevel]
	})
	if len(nonCompliant) > 50 {
		nonCompliant = nonCompliant[:50]
	}

	// Recommendations
	var recommendations []string
	if privileged > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d privileged container(s) — remove privileged flag and use securityContext capabilities instead", privileged))
	}
	if noRunAsNonRoot > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) missing runAsNonRoot — set to true to prevent running as root", noRunAsNonRoot))
	}
	if noSeccomp > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) missing seccompProfile — set to RuntimeDefault for syscall restriction", noSeccomp))
	}
	if allowPrivEscal > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) allow privilege escalation — set allowPrivilegeEscalation=false", allowPrivEscal))
	}
	if noCapDropAll > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) don't drop ALL capabilities — add capabilities.drop: [ALL]", noCapDropAll))
	}
	if noReadOnlyRootFS > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d container(s) don't use readOnlyRootFilesystem — enable for filesystem isolation", noReadOnlyRootFS))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "All containers are PSS restricted profile compliant — excellent security posture")
	}

	result.NonCompliant = nonCompliant
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = PSSSummary{
		TotalContainers:     totalContainers,
		RestrictedCompliant: restrictedCompliant,
		BaselineCompliant:   baselineCompliant,
		Privileged:          privileged,
		NoRunAsNonRoot:      noRunAsNonRoot,
		NoSeccompProfile:    noSeccomp,
		AllowPrivEscal:      allowPrivEscal,
		NoCapDropAll:        noCapDropAll,
		NoReadOnlyRootFS:    noReadOnlyRootFS,
		HostNetwork:         hostNetwork,
		HostPID:             hostPID,
		HostIPC:             hostIPC,
	}
	result.HealthScore = computePSSScore(result.Summary)

	writeJSON(w, result)
}

// assessPSSRisk determines risk level based on PSS violations.
func assessPSSRisk(entry PSSContainerEntry) string {
	if entry.IsPrivileged {
		return "critical"
	}
	violationCount := len(entry.Violations)
	switch {
	case violationCount >= 4:
		return "critical"
	case violationCount >= 2:
		return "warning"
	case violationCount >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computePSSScore computes a 0-100 health score.
func computePSSScore(s PSSSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	// Privileged is most critical
	score -= s.Privileged * 15
	// Host namespaces
	score -= s.HostNetwork * 5
	score -= (s.HostPID + s.HostIPC) * 5
	// Missing security settings
	score -= s.AllowPrivEscal * 3
	score -= s.NoRunAsNonRoot * 2
	score -= s.NoSeccompProfile * 2
	score -= s.NoCapDropAll * 1
	score -= s.NoReadOnlyRootFS * 1
	// Compliance ratio bonus
	compliantRatio := float64(s.RestrictedCompliant) / float64(s.TotalContainers)
	if compliantRatio > 0.8 {
		score += 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
var _ = corev1.PodSecurityContext{}
