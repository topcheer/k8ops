package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.08 — Security Dimension (Round 4)
// 1. Linux Capability Audit
// 2. Host Namespace Access Audit
// 3. Pod Security Standard Compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Linux Capability Audit — high-risk capabilities checker
// ---------------------------------------------------------------

type CapAuditResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         CapAuditSummary1908   `json:"summary"`
	HighRiskCaps    []CapAuditEntry1908   `json:"highRiskCaps"`
	DroppedCaps     []CapAuditEntry1908   `json:"droppedCapsInfo"`
	ByNamespace     []CapAuditNSEntry1908 `json:"byNamespace"`
	Recommendations []string              `json:"recommendations"`
}

type CapAuditSummary1908 struct {
	TotalContainers   int `json:"totalContainers"`
	WithCapAdd        int `json:"withCapAdd"`
	WithCapDrop       int `json:"withCapDrop"`
	WithAllDropped    int `json:"withAllDropped"`
	HighRiskCaps      int `json:"highRiskCaps"`
	Privileged        int `json:"privilegedContainers"`
	WithoutSecContext int `json:"withoutSecurityContext"`
}

type CapAuditEntry1908 struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Container    string   `json:"container"`
	CapsAdded    []string `json:"capsAdded"`
	CapsDropped  []string `json:"capsDropped"`
	IsPrivileged bool     `json:"isPrivileged"`
	RiskLevel    string   `json:"riskLevel"`
	Issue        string   `json:"issue"`
}

type CapAuditNSEntry1908 struct {
	Namespace  string `json:"namespace"`
	Containers int    `json:"containers"`
	HighRisk   int    `json:"highRisk"`
}

// Known high-risk capabilities
var highRiskCaps1908 = map[string]string{
	"CAP_SYS_ADMIN":       "broad system administration - nearly equivalent to root",
	"CAP_NET_ADMIN":       "network configuration - can intercept/redirect traffic",
	"CAP_SYS_MODULE":      "load kernel modules - kernel-level code execution",
	"CAP_SYS_PTRACE":      "trace processes - can inject code into other processes",
	"CAP_SYS_RAWIO":       "raw I/O - bypass filesystem permissions",
	"CAP_DAC_OVERRIDE":    "bypass file read/write/execute checks",
	"CAP_NET_RAW":         "raw network packets - can forge packets",
	"CAP_LINUX_IMMUTABLE": "set immutable file attribute",
	"CAP_SYS_BOOT":        "reboot the system",
}

func (s *Server) handleCapAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CapAuditResult{ScannedAt: time.Now()}

	nsMap := map[string]*CapAuditNSEntry1908{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			nsE, ok := nsMap[dep.Namespace]
			if !ok {
				nsE = &CapAuditNSEntry1908{Namespace: dep.Namespace}
				nsMap[dep.Namespace] = nsE
			}
			nsE.Containers++

			entry := CapAuditEntry1908{
				Name: dep.Name, Namespace: dep.Namespace, Container: c.Name,
				CapsAdded: []string{}, CapsDropped: []string{},
			}

			sc := c.SecurityContext
			if sc == nil {
				result.Summary.WithoutSecContext++
				entry.Issue = "no securityContext defined"
				entry.RiskLevel = "medium"
				result.HighRiskCaps = append(result.HighRiskCaps, entry)
				continue
			}

			// Check privileged
			if sc.Privileged != nil && *sc.Privileged {
				entry.IsPrivileged = true
				result.Summary.Privileged++
				entry.RiskLevel = "critical"
				entry.Issue = "privileged container - full host access"
				nsE.HighRisk++
				result.HighRiskCaps = append(result.HighRiskCaps, entry)
				continue
			}

			// Check capabilities
			if sc.Capabilities != nil {
				if len(sc.Capabilities.Add) > 0 {
					result.Summary.WithCapAdd++
					for _, cap := range sc.Capabilities.Add {
						capStr := string(cap)
						entry.CapsAdded = append(entry.CapsAdded, capStr)
						if desc, isHigh := highRiskCaps1908[capStr]; isHigh {
							result.Summary.HighRiskCaps++
							if entry.RiskLevel == "" {
								entry.RiskLevel = "high"
								entry.Issue = fmt.Sprintf("%s: %s", capStr, desc)
								nsE.HighRisk++
							}
						}
					}
					if entry.RiskLevel == "high" {
						result.HighRiskCaps = append(result.HighRiskCaps, entry)
					}
				}
				if len(sc.Capabilities.Drop) > 0 {
					result.Summary.WithCapDrop++
					for _, cap := range sc.Capabilities.Drop {
						entry.CapsDropped = append(entry.CapsDropped, string(cap))
					}
					// Check if ALL is dropped (best practice)
					for _, cap := range sc.Capabilities.Drop {
						if string(cap) == "ALL" {
							result.Summary.WithAllDropped++
							break
						}
					}
				}
			}

			if entry.RiskLevel == "" {
				if len(entry.CapsDropped) == 0 && len(entry.CapsAdded) == 0 {
					entry.RiskLevel = "low"
					entry.Issue = "no capabilities explicitly managed"
				} else {
					entry.RiskLevel = "low"
				}
				result.DroppedCaps = append(result.DroppedCaps, entry)
			}
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].HighRisk > result.ByNamespace[j].HighRisk
	})

	// Score: fewer high-risk caps = better
	if result.Summary.TotalContainers > 0 {
		safePct := (result.Summary.TotalContainers - result.Summary.Privileged - result.Summary.HighRiskCaps) * 100 / result.Summary.TotalContainers
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildCapAuditRecs1908(&result)
	writeJSON(w, result)
}

func buildCapAuditRecs1908(r *CapAuditResult) []string {
	recs := []string{fmt.Sprintf("Capability audit: %d containers, %d privileged, %d with cap-add, %d high-risk caps, %d drop ALL",
		r.Summary.TotalContainers, r.Summary.Privileged, r.Summary.WithCapAdd,
		r.Summary.HighRiskCaps, r.Summary.WithAllDropped)}
	if r.Summary.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged containers - remove privileged flag and add specific capabilities only", r.Summary.Privileged))
	}
	if r.Summary.HighRiskCaps > 0 {
		recs = append(recs, fmt.Sprintf("%d high-risk capabilities granted - audit each and remove if not strictly needed", r.Summary.HighRiskCaps))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Host Namespace Access Audit
// ---------------------------------------------------------------

type HostNSAuditResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HostNSAuditSummary  `json:"summary"`
	Violations      []HostNSAuditEntry  `json:"violations"`
	HostPathMounts  []HostPathEntry1908 `json:"hostPathMounts"`
	ByNamespace     []HostNSAuditNS     `json:"byNamespace"`
	Recommendations []string            `json:"recommendations"`
}

type HostNSAuditSummary struct {
	TotalWorkloads       int `json:"totalWorkloads"`
	HostPID              int `json:"hostPID"`
	HostNetwork          int `json:"hostNetwork"`
	HostIPC              int `json:"hostIPC"`
	HostPathMounts       int `json:"hostPathMounts"`
	HostPathUnrestricted int `json:"hostPathUnrestricted"`
	Violations           int `json:"violations"`
	CleanWorkloads       int `json:"cleanWorkloads"`
}

type HostNSAuditEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

type HostPathEntry1908 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	VolumeName string `json:"volumeName"`
	HostPath   string `json:"hostPath"`
	ReadOnly   bool   `json:"readOnly"`
	RiskLevel  string `json:"riskLevel"`
}

type HostNSAuditNS struct {
	Namespace  string `json:"namespace"`
	Workloads  int    `json:"workloads"`
	Violations int    `json:"violations"`
}

func (s *Server) handleHostNSAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := HostNSAuditResult{ScannedAt: time.Now()}

	nsMap := map[string]*HostNSAuditNS{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &HostNSAuditNS{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.Workloads++

		spec := dep.Spec.Template.Spec
		hasViolation := false

		// Check hostPID
		if spec.HostPID {
			result.Summary.HostPID++
			result.Summary.Violations++
			result.Violations = append(result.Violations, HostNSAuditEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Violation: "hostPID enabled - can see host processes",
				Severity:  "high",
			})
			nsE.Violations++
			hasViolation = true
		}

		// Check hostNetwork
		if spec.HostNetwork {
			result.Summary.HostNetwork++
			result.Summary.Violations++
			result.Violations = append(result.Violations, HostNSAuditEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Violation: "hostNetwork enabled - uses host network namespace",
				Severity:  "high",
			})
			nsE.Violations++
			hasViolation = true
		}

		// Check hostIPC
		if spec.HostIPC {
			result.Summary.HostIPC++
			result.Summary.Violations++
			result.Violations = append(result.Violations, HostNSAuditEntry{
				Name: dep.Name, Namespace: dep.Namespace,
				Violation: "hostIPC enabled - shared memory with host",
				Severity:  "high",
			})
			nsE.Violations++
			hasViolation = true
		}

		// Check hostPath volumes
		for _, vol := range spec.Volumes {
			if vol.HostPath != nil {
				result.Summary.HostPathMounts++
				readOnly := false
				if vol.HostPath.Type != nil {
					readOnly = *vol.HostPath.Type == corev1.HostPathDirectoryOrCreate ||
						*vol.HostPath.Type == corev1.HostPathFileOrCreate
				}
				riskLevel := "high"
				if readOnly {
					riskLevel = "medium"
				} else {
					result.Summary.HostPathUnrestricted++
				}
				result.HostPathMounts = append(result.HostPathMounts, HostPathEntry1908{
					Name: dep.Name, Namespace: dep.Namespace,
					VolumeName: vol.Name, HostPath: vol.HostPath.Path,
					ReadOnly: readOnly, RiskLevel: riskLevel,
				})
				hasViolation = true
			}
		}

		if !hasViolation {
			result.Summary.CleanWorkloads++
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Violations > result.ByNamespace[j].Violations
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		cleanPct := result.Summary.CleanWorkloads * 100 / result.Summary.TotalWorkloads
		result.HealthScore = cleanPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildHostNSRecs1908(&result)
	writeJSON(w, result)
}

func buildHostNSRecs1908(r *HostNSAuditResult) []string {
	recs := []string{fmt.Sprintf("Host namespace audit: %d workloads, %d hostPID, %d hostNetwork, %d hostIPC, %d hostPath mounts (%d unrestricted)",
		r.Summary.TotalWorkloads, r.Summary.HostPID, r.Summary.HostNetwork,
		r.Summary.HostIPC, r.Summary.HostPathMounts, r.Summary.HostPathUnrestricted)}
	if r.Summary.HostPathUnrestricted > 0 {
		recs = append(recs, fmt.Sprintf("%d unrestricted hostPath mounts - can write to host filesystem", r.Summary.HostPathUnrestricted))
	}
	if r.Summary.HostNetwork > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with hostNetwork - can bind to host ports and intercept traffic", r.Summary.HostNetwork))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Pod Security Standard Compliance
// ---------------------------------------------------------------

type PSSComplianceResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PSSComplianceSummary `json:"summary"`
	Violations      []PSSViolation       `json:"violations"`
	ByLevel         map[string]int       `json:"byLevel"`
	ByNamespace     []PSSNSCompliance    `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type PSSComplianceSummary struct {
	TotalWorkloads int `json:"totalWorkloads"`
	PassBaseline   int `json:"passBaseline"`
	FailBaseline   int `json:"failBaseline"`
	PassRestricted int `json:"passRestricted"`
	FailRestricted int `json:"failRestricted"`
	Privileged     int `json:"privileged"`
	RunAsRoot      int `json:"runAsRoot"`
	NoNonRoot      int `json:"noNonRoot"`
}

type PSSViolation struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Level     string `json:"level"`
	Check     string `json:"check"`
	Severity  string `json:"severity"`
}

type PSSNSCompliance struct {
	Namespace      string `json:"namespace"`
	Workloads      int    `json:"workloads"`
	BaselinePass   int    `json:"baselinePass"`
	RestrictedPass int    `json:"restrictedPass"`
}

func (s *Server) handlePSSCompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PSSComplianceResult{
		ScannedAt: time.Now(),
		ByLevel:   map[string]int{},
	}

	nsMap := map[string]*PSSNSCompliance{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &PSSNSCompliance{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.Workloads++

		podSpec := &dep.Spec.Template.Spec
		passBaseline := true
		passRestricted := true

		// === Baseline checks ===
		// 1. Privileged containers
		for _, c := range podSpec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				passBaseline = false
				passRestricted = false
				result.Summary.Privileged++
				result.Violations = append(result.Violations, PSSViolation{
					Name: dep.Name, Namespace: dep.Namespace,
					Level: "baseline", Check: "privileged container",
					Severity: "critical",
				})
			}
			// 2. hostPath volumes already checked in host-ns audit, skip here

			// 3. hostPID/hostNetwork/hostIPC
			if podSpec.HostPID || podSpec.HostNetwork || podSpec.HostIPC {
				passBaseline = false
				passRestricted = false
			}

			// 4. hostPort
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					capStr := string(cap)
					if _, isHigh := highRiskCaps1908[capStr]; isHigh {
						passBaseline = false
						result.Violations = append(result.Violations, PSSViolation{
							Name: dep.Name, Namespace: dep.Namespace,
							Level: "baseline", Check: fmt.Sprintf("added capability %s", capStr),
							Severity: "high",
						})
					}
				}
			}
		}

		// === Restricted checks ===
		for _, c := range podSpec.Containers {
			sc := c.SecurityContext
			if sc == nil {
				result.Summary.NoNonRoot++
				passRestricted = false
				result.Violations = append(result.Violations, PSSViolation{
					Name: dep.Name, Namespace: dep.Namespace,
					Level: "restricted", Check: "no securityContext",
					Severity: "medium",
				})
				continue
			}

			// runAsNonRoot must be true
			if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				if podSpec.SecurityContext == nil ||
					podSpec.SecurityContext.RunAsNonRoot == nil ||
					!*podSpec.SecurityContext.RunAsNonRoot {
					result.Summary.RunAsRoot++
					passRestricted = false
					result.Violations = append(result.Violations, PSSViolation{
						Name: dep.Name, Namespace: dep.Namespace,
						Level: "restricted", Check: "runAsNonRoot not set to true",
						Severity: "medium",
					})
				}
			}

			// allowPrivilegeEscalation must be false
			if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
				passRestricted = false
				result.Violations = append(result.Violations, PSSViolation{
					Name: dep.Name, Namespace: dep.Namespace,
					Level: "restricted", Check: "allowPrivilegeEscalation not false",
					Severity: "medium",
				})
			}

			// readOnlyRootFilesystem (restricted recommended)
			if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				passRestricted = false
				result.Violations = append(result.Violations, PSSViolation{
					Name: dep.Name, Namespace: dep.Namespace,
					Level: "restricted", Check: "readOnlyRootFilesystem not true",
					Severity: "low",
				})
			}
		}

		if passBaseline {
			result.Summary.PassBaseline++
			nsE.BaselinePass++
		} else {
			result.Summary.FailBaseline++
		}
		if passRestricted {
			result.Summary.PassRestricted++
			nsE.RestrictedPass++
		} else {
			result.Summary.FailRestricted++
		}
	}

	result.ByLevel["baseline-pass"] = result.Summary.PassBaseline
	result.ByLevel["baseline-fail"] = result.Summary.FailBaseline
	result.ByLevel["restricted-pass"] = result.Summary.PassRestricted
	result.ByLevel["restricted-fail"] = result.Summary.FailRestricted

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RestrictedPass < result.ByNamespace[j].RestrictedPass
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		baselinePct := result.Summary.PassBaseline * 100 / result.Summary.TotalWorkloads
		restrictedPct := result.Summary.PassRestricted * 100 / result.Summary.TotalWorkloads
		result.HealthScore = (baselinePct + restrictedPct) / 2
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildPSSRecs1908(&result)
	writeJSON(w, result)
}

func buildPSSRecs1908(r *PSSComplianceResult) []string {
	recs := []string{fmt.Sprintf("PSS compliance: %d workloads, baseline %d/%d pass, restricted %d/%d pass",
		r.Summary.TotalWorkloads,
		r.Summary.PassBaseline, r.Summary.TotalWorkloads,
		r.Summary.PassRestricted, r.Summary.TotalWorkloads)}
	if r.Summary.FailBaseline > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads fail Pod Security Baseline - enforce baseline policy", r.Summary.FailBaseline))
	}
	if r.Summary.RunAsRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads running as root - set runAsNonRoot: true", r.Summary.RunAsRoot))
	}
	return recs
}
