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

// SecPostureResult is the cluster-wide security posture scorecard.
type SecPostureResult struct {
	ScannedAt         time.Time          `json:"scannedAt"`
	Summary           PostureSummary     `json:"summary"`
	ClusterGrade      string             `json:"clusterGrade"`
	ClusterScore      int                `json:"clusterScore"`
	Dimensions        []PostureDimension `json:"dimensions"`
	HighRiskWorkloads []PostureWorkload  `json:"highRiskWorkloads"`
	AttackSurface     AttackSurface      `json:"attackSurface"`
	Risks             []PostureRisk      `json:"risks"`
	Recommendations   []string           `json:"recommendations"`
}

// PostureSummary aggregates security posture statistics.
type PostureSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	CriticalRisk    int `json:"criticalRisk"` // immediate exploitation risk
	HighRisk        int `json:"highRisk"`
	MediumRisk      int `json:"mediumRisk"`
	LowRisk         int `json:"lowRisk"`
	PrivilegedPods  int `json:"privilegedPods"`
	HostNetworkPods int `json:"hostNetworkPods"`
	NoNetworkPolicy int `json:"noNetworkPolicy"` // workloads without NSP
	RootContainers  int `json:"rootContainers"`  // running as root
	NoResourceLimit int `json:"noResourceLimit"`
}

// PostureDimension scores one security dimension.
type PostureDimension struct {
	Name        string `json:"name"`
	Score       int    `json:"score"`
	Status      string `json:"status"` // good, warning, critical
	Description string `json:"description"`
	Detail      string `json:"detail,omitempty"`
}

// PostureWorkload describes a high-risk workload.
type PostureWorkload struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Kind       string   `json:"kind"`
	RiskLevel  string   `json:"riskLevel"`
	Score      int      `json:"score"`
	Violations []string `json:"violations"`
}

// AttackSurface summarizes the cluster attack surface.
type AttackSurface struct {
	HostAccessPaths    int `json:"hostAccessPaths"`    // privileged + hostPath + hostNS
	CapEscalationPaths int `json:"capEscalationPaths"` // dangerous capabilities
	SATokenExposed     int `json:"saTokenExposed"`     // SA with automount + wide RBAC
	EgressUnrestricted int `json:"egressUnrestricted"` // pods without egress NSP
}

// PostureRisk describes a security risk finding.
type PostureRisk struct {
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Resource   string `json:"resource"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// handleSecurityPosture generates a comprehensive security posture scorecard.
// GET /api/security/posture-scorecard
func (s *Server) handleSecurityPosture(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SecPostureResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulSets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonSets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	networkPolicies, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	// Build NS coverage map
	nsHasPolicy := map[string]bool{}
	if networkPolicies != nil {
		for _, np := range networkPolicies.Items {
			nsHasPolicy[np.Namespace] = true
		}
	}

	var dims []PostureDimension
	var highRiskWLs []PostureWorkload
	var risks []PostureRisk
	summary := PostureSummary{}

	// Danger capabilities set
	dangerCaps := map[string]bool{
		"SYS_ADMIN": true, "SYS_PTRACE": true, "SYS_MODULE": true,
		"NET_ADMIN": true, "NET_RAW": true, "DAC_OVERRIDE": true,
		"CHOWN": true, "FOWNER": true, "SETUID": true, "SETGID": true,
	}

	attackSurface := AttackSurface{}
	totalContainers := 0
	checkedWorkloads := 0

	// Analyze deployments
	if deployments != nil {
		for _, d := range deployments.Items {
			if isSystemNSReliability(d.Namespace) {
				continue
			}
			checkedWorkloads++
			wl := analyzeSecPosture(d.Name, d.Namespace, "Deployment", d.Spec.Template.Spec, nsHasPolicy, dangerCaps)
			updatePostureStats(&summary, &attackSurface, &wl, &totalContainers)
			if wl.RiskLevel == "critical" || wl.RiskLevel == "high" {
				highRiskWLs = append(highRiskWLs, wl)
				if len(risks) < 50 {
					risks = append(risks, wlToRisks(wl)...)
				}
			}
		}
	}
	if statefulSets != nil {
		for _, ss := range statefulSets.Items {
			if isSystemNSReliability(ss.Namespace) {
				continue
			}
			checkedWorkloads++
			wl := analyzeSecPosture(ss.Name, ss.Namespace, "StatefulSet", ss.Spec.Template.Spec, nsHasPolicy, dangerCaps)
			updatePostureStats(&summary, &attackSurface, &wl, &totalContainers)
			if wl.RiskLevel == "critical" || wl.RiskLevel == "high" {
				highRiskWLs = append(highRiskWLs, wl)
			}
		}
	}
	if daemonSets != nil {
		for _, ds := range daemonSets.Items {
			if isSystemNSReliability(ds.Namespace) {
				continue
			}
			checkedWorkloads++
			wl := analyzeSecPosture(ds.Name, ds.Namespace, "DaemonSet", ds.Spec.Template.Spec, nsHasPolicy, dangerCaps)
			updatePostureStats(&summary, &attackSurface, &wl, &totalContainers)
			if wl.RiskLevel == "critical" || wl.RiskLevel == "high" {
				highRiskWLs = append(highRiskWLs, wl)
			}
		}
	}

	summary.TotalWorkloads = checkedWorkloads
	sort.Slice(highRiskWLs, func(i, j int) bool {
		return highRiskWLs[i].Score < highRiskWLs[j].Score
	})
	if len(highRiskWLs) > 30 {
		highRiskWLs = highRiskWLs[:30]
	}

	// ========================================
	// Compute dimension scores
	// ========================================

	// 1. Pod Security (privileged, root, capabilities)
	podSecScore := 100
	if summary.PrivilegedPods > 0 {
		podSecScore -= summary.PrivilegedPods * 15
	}
	if summary.RootContainers > 0 && totalContainers > 0 {
		rootRatio := float64(summary.RootContainers) / float64(totalContainers)
		podSecScore -= int(rootRatio * 30)
	}
	podSecScore = clampScore(podSecScore)
	dims = append(dims, PostureDimension{
		Name: "pod-security", Score: podSecScore, Status: dimStatus(podSecScore),
		Description: fmt.Sprintf("%d privileged, %d root containers of %d total", summary.PrivilegedPods, summary.RootContainers, totalContainers),
	})

	// 2. Host Access (hostNetwork, hostPID, hostIPC, hostPath)
	hostScore := 100
	hostAccess := summary.HostNetworkPods + attackSurface.HostAccessPaths
	if hostAccess > 0 {
		hostScore -= hostAccess * 10
	}
	hostScore = clampScore(hostScore)
	dims = append(dims, PostureDimension{
		Name: "host-access", Score: hostScore, Status: dimStatus(hostScore),
		Description: fmt.Sprintf("%d pods with host access (network/pid/ipc/path)", hostAccess),
	})

	// 3. Network Isolation
	netScore := 100
	if summary.NoNetworkPolicy > 0 && checkedWorkloads > 0 {
		noPolRatio := float64(summary.NoNetworkPolicy) / float64(checkedWorkloads)
		netScore -= int(noPolRatio * 50)
	}
	netScore = clampScore(netScore)
	dims = append(dims, PostureDimension{
		Name: "network-isolation", Score: netScore, Status: dimStatus(netScore),
		Description: fmt.Sprintf("%d/%d workloads without NetworkPolicy", summary.NoNetworkPolicy, checkedWorkloads),
	})

	// 4. Resource Boundaries
	resScore := 100
	if summary.NoResourceLimit > 0 && totalContainers > 0 {
		noLimRatio := float64(summary.NoResourceLimit) / float64(totalContainers)
		resScore -= int(noLimRatio * 40)
	}
	resScore = clampScore(resScore)
	dims = append(dims, PostureDimension{
		Name: "resource-boundaries", Score: resScore, Status: dimStatus(resScore),
		Description: fmt.Sprintf("%d containers without limits of %d total", summary.NoResourceLimit, totalContainers),
	})

	// 5. Attack Surface
	attackScore := 100
	attackScore -= attackSurface.HostAccessPaths * 5
	attackScore -= attackSurface.CapEscalationPaths * 8
	attackScore -= attackSurface.SATokenExposed * 5
	attackScore -= attackSurface.EgressUnrestricted * 3
	attackScore = clampScore(attackScore)
	dims = append(dims, PostureDimension{
		Name: "attack-surface", Score: attackScore, Status: dimStatus(attackScore),
		Description: fmt.Sprintf("%d host paths, %d cap escalation, %d SA token exposed, %d unrestricted egress",
			attackSurface.HostAccessPaths, attackSurface.CapEscalationPaths, attackSurface.SATokenExposed, attackSurface.EgressUnrestricted),
	})

	// Compute cluster score
	totalDimScore := 0
	for _, d := range dims {
		totalDimScore += d.Score
	}
	if len(dims) > 0 {
		result.ClusterScore = totalDimScore / len(dims)
	} else {
		result.ClusterScore = 100
	}
	result.ClusterGrade = scoreToGradeReliability(result.ClusterScore)

	result.Summary = summary
	result.Dimensions = dims
	result.HighRiskWorkloads = highRiskWLs
	result.AttackSurface = attackSurface
	result.Risks = risks
	result.Recommendations = generatePostureRecs(result)

	writeJSON(w, result)
}

// analyzeSecPosture analyzes a single workload's security posture.
func analyzeSecPosture(name, namespace, kind string, spec corev1.PodSpec, nsHasPolicy map[string]bool, dangerCaps map[string]bool) PostureWorkload {
	wl := PostureWorkload{
		Name: name, Namespace: namespace, Kind: kind,
		Score: 100, RiskLevel: "low",
	}

	var violations []string
	score := 100

	// Pod-level checks
	if spec.HostNetwork {
		score -= 25
		violations = append(violations, "hostNetwork")
	}
	if spec.HostPID {
		score -= 20
		violations = append(violations, "hostPID")
	}
	if spec.HostIPC {
		score -= 20
		violations = append(violations, "hostIPC")
	}

	// ServiceAccount token automount
	autoMount := true
	if spec.AutomountServiceAccountToken != nil {
		autoMount = *spec.AutomountServiceAccountToken
	}
	if !autoMount {
		// Good practice - no deduction
	} else if spec.ServiceAccountName == "" || spec.ServiceAccountName == "default" {
		score -= 10
		violations = append(violations, "uses default ServiceAccount")
	}

	// Container checks
	for _, c := range spec.Containers {
		// Privileged
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			score -= 30
			violations = append(violations, fmt.Sprintf("%s: privileged", c.Name))
		}

		// Running as root
		if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
			if c.SecurityContext == nil || c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser == 0 {
				violations = append(violations, fmt.Sprintf("%s: runs as root", c.Name))
			}
		}

		// Capabilities
		if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
			for _, cap := range c.SecurityContext.Capabilities.Add {
				if dangerCaps[string(cap)] {
					score -= 12
					violations = append(violations, fmt.Sprintf("%s: %s capability", c.Name, cap))
				}
			}
		}

		// No resource limits
		if c.Resources.Limits.Cpu() == nil && c.Resources.Limits.Memory() == nil {
			violations = append(violations, fmt.Sprintf("%s: no resource limits", c.Name))
		}
	}

	// hostPath volumes
	for _, v := range spec.Volumes {
		if v.HostPath != nil {
			score -= 15
			violations = append(violations, fmt.Sprintf("hostPath: %s", v.HostPath.Path))
		}
	}

	// NetworkPolicy coverage
	if !nsHasPolicy[namespace] {
		violations = append(violations, "no NetworkPolicy in namespace")
	}

	score = clampScore(score)
	wl.Score = score
	wl.Violations = violations

	switch {
	case score < 40:
		wl.RiskLevel = "critical"
	case score < 60:
		wl.RiskLevel = "high"
	case score < 80:
		wl.RiskLevel = "medium"
	default:
		wl.RiskLevel = "low"
	}

	return wl
}

// updatePostureStats updates summary and attack surface from a workload analysis.
func updatePostureStats(summary *PostureSummary, as *AttackSurface, wl *PostureWorkload, totalContainers *int) {
	switch wl.RiskLevel {
	case "critical":
		summary.CriticalRisk++
	case "high":
		summary.HighRisk++
	case "medium":
		summary.MediumRisk++
	default:
		summary.LowRisk++
	}

	for _, v := range wl.Violations {
		lv := strings.ToLower(v)
		if strings.Contains(lv, "privileged") {
			summary.PrivilegedPods++
		}
		if strings.Contains(lv, "hostnetwork") {
			summary.HostNetworkPods++
			as.HostAccessPaths++
		}
		if strings.Contains(lv, "hostpid") || strings.Contains(lv, "hostipc") || strings.Contains(lv, "hostpath") {
			as.HostAccessPaths++
		}
		if strings.Contains(lv, "capability") {
			as.CapEscalationPaths++
		}
		if strings.Contains(lv, "default serviceaccount") {
			as.SATokenExposed++
		}
		if strings.Contains(lv, "no networkpolicy") {
			summary.NoNetworkPolicy++
		}
		if strings.Contains(lv, "no resource limits") {
			summary.NoResourceLimit++
		}
		if strings.Contains(lv, "runs as root") {
			summary.RootContainers++
		}
		*totalContainers++
	}
}

// wlToRisks converts a workload's violations to risk entries.
func wlToRisks(wl PostureWorkload) []PostureRisk {
	var risks []PostureRisk
	sev := "medium"
	if wl.RiskLevel == "critical" {
		sev = "critical"
	} else if wl.RiskLevel == "high" {
		sev = "high"
	}
	for _, v := range wl.Violations {
		risks = append(risks, PostureRisk{
			Severity:   sev,
			Category:   categorizeViolation(v),
			Resource:   fmt.Sprintf("%s/%s", wl.Kind, wl.Name),
			Issue:      v,
			Suggestion: suggestFix(v),
		})
	}
	return risks
}

// categorizeViolation maps a violation string to a category.
func categorizeViolation(v string) string {
	v = strings.ToLower(v)
	switch {
	case strings.Contains(v, "privileged"):
		return "pod-security"
	case strings.Contains(v, "host"):
		return "host-access"
	case strings.Contains(v, "capability"):
		return "capabilities"
	case strings.Contains(v, "networkpolicy"):
		return "network-isolation"
	case strings.Contains(v, "resource"):
		return "resource-boundaries"
	case strings.Contains(v, "root"):
		return "pod-security"
	default:
		return "misc"
	}
}

// suggestFix returns a remediation suggestion for a violation.
func suggestFix(v string) string {
	v = strings.ToLower(v)
	switch {
	case strings.Contains(v, "privileged"):
		return "Remove privileged: true; use specific Linux capabilities instead"
	case strings.Contains(v, "hostnetwork"):
		return "Use ClusterIP/NodePort services instead of hostNetwork"
	case strings.Contains(v, "hostpid"), strings.Contains(v, "hostipc"):
		return "Remove hostPID/hostIPC unless absolutely required for system components"
	case strings.Contains(v, "hostpath"):
		return "Use PersistentVolumeClaim or projected volumes instead of hostPath"
	case strings.Contains(v, "capability"):
		return "Drop dangerous capabilities; use Pod Security Admission restricted level"
	case strings.Contains(v, "default serviceaccount"):
		return "Create dedicated ServiceAccount per workload; disable automount if unused"
	case strings.Contains(v, "networkpolicy"):
		return "Add default-deny NetworkPolicy in namespace; only allow required traffic"
	case strings.Contains(v, "resource"):
		return "Set CPU and memory limits on all containers"
	case strings.Contains(v, "root"):
		return "Set runAsNonRoot: true or specify runAsUser > 0"
	default:
		return "Review and remediate security configuration"
	}
}

// clampScore ensures score is 0-100.
func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

// generatePostureRecs produces recommendations.
func generatePostureRecs(result SecPostureResult) []string {
	var recs []string

	if result.Summary.CriticalRisk > 0 {
		recs = append(recs, fmt.Sprintf("URGENT: %d workload(s) at critical security risk — immediate remediation needed", result.Summary.CriticalRisk))
	}
	if result.Summary.PrivilegedPods > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged pod(s) — enforce Pod Security Admission restricted level", result.Summary.PrivilegedPods))
	}
	if result.Summary.HostNetworkPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with hostNetwork — restrict to system components only", result.Summary.HostNetworkPods))
	}
	if result.Summary.NoNetworkPolicy > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) without NetworkPolicy — add default-deny policies", result.Summary.NoNetworkPolicy))
	}
	if result.AttackSurface.CapEscalationPaths > 0 {
		recs = append(recs, fmt.Sprintf("%d dangerous capability grants — drop all and add only required ones", result.AttackSurface.CapEscalationPaths))
	}

	if result.ClusterGrade == "A" || result.ClusterGrade == "B" {
		recs = append(recs, fmt.Sprintf("Security posture grade %s (%d/100) — well-hardened cluster", result.ClusterGrade, result.ClusterScore))
	}

	if len(recs) == 0 {
		recs = append(recs, fmt.Sprintf("Security posture grade %s (%d/100) — no critical findings", result.ClusterGrade, result.ClusterScore))
	}

	return recs
}
