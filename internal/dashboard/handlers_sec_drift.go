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

// SecDriftResult is the security context drift & runtime policy compliance audit.
type SecDriftResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SecDriftSummary     `json:"summary"`
	ByNamespace     []SecDriftNSEntry   `json:"byNamespace"`
	Violations      []SecDriftViolation `json:"violations"`
	ByWorkload      []SecDriftWorkload  `json:"byWorkload"`
	Recommendations []string            `json:"recommendations"`
}

// SecDriftSummary aggregates security context drift statistics.
type SecDriftSummary struct {
	TotalPods       int `json:"totalPods"`
	TotalContainers int `json:"totalContainers"`
	NoNonRoot       int `json:"noNonRoot"`    // missing runAsNonRoot=true
	NoReadOnlyFS    int `json:"noReadOnlyFS"` // missing readOnlyRootFilesystem=true
	AllowPrivEsc    int `json:"allowPrivEsc"` // allowPrivilegeEscalation not false
	NoCapDrop       int `json:"noCapDrop"`    // no capabilities dropped
	HasAllCaps      int `json:"hasAllCaps"`   // ADD ALL capabilities
	Privileged      int `json:"privileged"`   // privileged=true
	NoRunAsUser     int `json:"noRunAsUser"`  // missing runAsUser (runs as root by default)
	HighRiskPods    int `json:"highRiskPods"`
	HealthScore     int `json:"healthScore"`
}

// SecDriftNSEntry shows security context compliance per namespace.
type SecDriftNSEntry struct {
	Namespace      string  `json:"namespace"`
	TotalPods      int     `json:"totalPods"`
	ViolationCount int     `json:"violationCount"`
	CompliancePct  float64 `json:"compliancePct"`
	RiskLevel      string  `json:"riskLevel"`
}

// SecDriftViolation is a single security context violation.
type SecDriftViolation struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

// SecDriftWorkload shows security compliance per workload.
type SecDriftWorkload struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	WorkloadType   string `json:"workloadType"`
	ContainerCount int    `json:"containerCount"`
	ViolationCount int    `json:"violationCount"`
	RiskLevel      string `json:"riskLevel"`
}

// secDriftAuditCore performs the security context drift audit on pods (testable).
func secDriftAuditCore(pods []corev1.Pod) SecDriftResult {
	result := SecDriftResult{
		ScannedAt: time.Now(),
	}

	nsStats := make(map[string]*SecDriftNSEntry)
	wlStats := make(map[string]*SecDriftWorkload)

	// Dangerous capabilities that should never be added
	dangerousCaps := map[string]bool{
		"SYS_ADMIN": true, "NET_ADMIN": true, "SYS_PTRACE": true,
		"SYS_MODULE": true, "DAC_READ_SEARCH": true, "SYS_RAWIO": true,
		"PERFMON": true, "CHECKPOINT_RESTORE": true,
	}

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &SecDriftNSEntry{Namespace: ns}
		}
		nsStats[ns].TotalPods++

		wlName, wlType := podOwnerInfo(pod)
		wlKey := fmt.Sprintf("%s/%s", ns, wlName)
		if _, ok := wlStats[wlKey]; !ok {
			wlStats[wlKey] = &SecDriftWorkload{
				Name:         wlName,
				Namespace:    ns,
				WorkloadType: wlType,
			}
		}

		podViolations := 0
		podHighRisk := false

		// Check pod-level security context
		podSC := pod.Spec.SecurityContext
		podRunAsNonRoot := false
		podRunAsUser := int64(0)
		podHasRunAsUser := false
		if podSC != nil {
			if podSC.RunAsNonRoot != nil && *podSC.RunAsNonRoot {
				podRunAsNonRoot = true
			}
			if podSC.RunAsUser != nil {
				podRunAsUser = *podSC.RunAsUser
				podHasRunAsUser = true
			}
		}

		// Check all containers (init + regular + ephemeral)
		allContainers := append([]corev1.Container{}, pod.Spec.Containers...)
		allContainers = append(allContainers, pod.Spec.InitContainers...)

		for _, c := range allContainers {
			result.Summary.TotalContainers++
			wlStats[wlKey].ContainerCount++

			sc := c.SecurityContext
			violations := []SecDriftViolation{}

			// 1. runAsNonRoot not set
			containerRunAsNonRoot := podRunAsNonRoot
			if sc != nil && sc.RunAsNonRoot != nil {
				containerRunAsNonRoot = *sc.RunAsNonRoot
			}
			if !containerRunAsNonRoot {
				result.Summary.NoNonRoot++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "runAsNonRoot not set to true — container may run as root",
					Severity:  "medium",
				})
			}

			// 2. readOnlyRootFilesystem not set
			if sc == nil || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				result.Summary.NoReadOnlyFS++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "readOnlyRootFilesystem not set — writable root filesystem allows tampering",
					Severity:  "low",
				})
			}

			// 3. allowPrivilegeEscalation not false
			if sc == nil || sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
				result.Summary.AllowPrivEsc++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "allowPrivilegeEscalation not set to false — child processes can gain more privileges",
					Severity:  "high",
				})
				podHighRisk = true
			}

			// 4. No capabilities dropped
			if sc == nil || sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 {
				result.Summary.NoCapDrop++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "no Linux capabilities dropped — container retains all default capabilities",
					Severity:  "medium",
				})
			}

			// 5. ADD ALL capabilities
			if sc != nil && sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Add {
					if string(cap) == "ALL" {
						result.Summary.HasAllCaps++
						violations = append(violations, SecDriftViolation{
							PodName: pod.Name, Namespace: ns, Container: c.Name,
							Violation: "ALL capabilities added — container has full kernel access",
							Severity:  "critical",
						})
						podHighRisk = true
					}
					if dangerousCaps[string(cap)] {
						violations = append(violations, SecDriftViolation{
							PodName: pod.Name, Namespace: ns, Container: c.Name,
							Violation: fmt.Sprintf("dangerous capability %s added", string(cap)),
							Severity:  "high",
						})
						podHighRisk = true
					}
				}
			}

			// 6. Privileged container
			if sc != nil && sc.Privileged != nil && *sc.Privileged {
				result.Summary.Privileged++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "privileged=true — container has full host access",
					Severity:  "critical",
				})
				podHighRisk = true
			}

			// 7. No runAsUser (runs as root by default)
			containerHasRunAsUser := podHasRunAsUser
			if sc != nil && sc.RunAsUser != nil {
				containerHasRunAsUser = true
				podRunAsUser = *sc.RunAsUser
			}
			if !containerHasRunAsUser {
				result.Summary.NoRunAsUser++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "runAsUser not set — defaults to root (UID 0)",
					Severity:  "medium",
				})
			} else if podRunAsUser == 0 {
				result.Summary.NoRunAsUser++
				violations = append(violations, SecDriftViolation{
					PodName: pod.Name, Namespace: ns, Container: c.Name,
					Violation: "runAsUser=0 — explicitly running as root",
					Severity:  "high",
				})
				podHighRisk = true
			}

			for _, v := range violations {
				result.Violations = append(result.Violations, v)
				podViolations++
				nsStats[ns].ViolationCount++
				wlStats[wlKey].ViolationCount++
			}
		}

		result.Summary.TotalPods++
		if podHighRisk {
			result.Summary.HighRiskPods++
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.TotalPods > 0 {
			stat.CompliancePct = (1 - float64(stat.ViolationCount)/float64(stat.TotalPods*7)) * 100
			if stat.CompliancePct < 0 {
				stat.CompliancePct = 0
			}
		}
		switch {
		case stat.ViolationCount > 20:
			stat.RiskLevel = "critical"
		case stat.ViolationCount > 10:
			stat.RiskLevel = "high"
		case stat.ViolationCount > 3:
			stat.RiskLevel = "medium"
		default:
			stat.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ViolationCount > result.ByNamespace[j].ViolationCount
	})

	// Build workload stats
	for _, wl := range wlStats {
		switch {
		case wl.ViolationCount > 10:
			wl.RiskLevel = "critical"
		case wl.ViolationCount > 5:
			wl.RiskLevel = "high"
		case wl.ViolationCount > 2:
			wl.RiskLevel = "medium"
		default:
			wl.RiskLevel = "low"
		}
		result.ByWorkload = append(result.ByWorkload, *wl)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].ViolationCount > result.ByWorkload[j].ViolationCount
	})

	result.Summary.HealthScore = secDriftScore(result.Summary)
	result.Recommendations = secDriftRecommendations(result.Summary)

	return result
}

// secDriftScore calculates health score.
func secDriftScore(s SecDriftSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	base := 100
	// Critical issues weighted heavily
	base -= s.Privileged * 15
	base -= s.HasAllCaps * 12
	// High severity
	base -= s.AllowPrivEsc * 8
	base -= s.HighRiskPods * 5
	// Medium severity
	base -= s.NoNonRoot * 3
	base -= s.NoCapDrop * 3
	base -= s.NoRunAsUser * 3
	// Low severity
	base -= s.NoReadOnlyFS * 1

	if base < 0 {
		base = 0
	}
	return base
}

// secDriftRecommendations generates recommendations.
func secDriftRecommendations(s SecDriftSummary) []string {
	var recs []string
	if s.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged containers detected — remove privileged flag and use granular capabilities instead", s.Privileged))
	}
	if s.HasAllCaps > 0 {
		recs = append(recs, fmt.Sprintf("%d containers with ALL capabilities — drop unnecessary capabilities and only add what is required", s.HasAllCaps))
	}
	if s.AllowPrivEsc > 0 {
		recs = append(recs, fmt.Sprintf("%d containers allow privilege escalation — set allowPrivilegeEscalation=false", s.AllowPrivEsc))
	}
	if s.NoCapDrop > 0 {
		recs = append(recs, fmt.Sprintf("%d containers without capability drops — add 'ALL' to capabilities.drop and only add back what is needed", s.NoCapDrop))
	}
	if s.NoNonRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d containers may run as root — set runAsNonRoot=true and runAsUser to non-zero UID", s.NoNonRoot))
	}
	if s.NoReadOnlyFS > 0 {
		recs = append(recs, fmt.Sprintf("%d containers with writable root filesystem — set readOnlyRootFilesystem=true for immutability", s.NoReadOnlyFS))
	}
	if s.Privileged == 0 && s.HasAllCaps == 0 && s.AllowPrivEsc == 0 {
		recs = append(recs, "no critical security context violations detected — runtime policy is well enforced")
	}
	return recs
}

// handleSecDrift audits security context drift and runtime policy compliance.
// GET /api/security/sec-drift
func (s *Server) handleSecDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := secDriftAuditCore(pods.Items)
	writeJSON(w, result)
}

// Suppress unused import
var _ = strings.Contains
