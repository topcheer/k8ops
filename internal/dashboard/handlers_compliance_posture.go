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

// ComplianceFrameworkResult maps cluster security state against multiple
// compliance frameworks (SOC2, PCI-DSS, HIPAA, NIST 800-53, GDPR).
// It cross-references existing k8ops findings with framework control families.
type ComplianceFrameworkResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	OverallScore    int                   `json:"overallScore"`
	OverallGrade    string                `json:"overallGrade"`
	Frameworks      []ComplianceFwPosture    `json:"frameworks"`
	ControlResults  []ComplianceCtrlResult       `json:"controlResults"`
	CrossFramework  []CrossFWGap   `json:"crossFrameworkGaps"`
	RemediationPlan []ComplianceRemediation `json:"remediationPlan"`
	Recommendations []string              `json:"recommendations"`
}

// ComplianceFwPosture shows compliance status for one framework.
type ComplianceFwPosture struct {
	Framework      string  `json:"framework"`
	FullName       string  `json:"fullName"`
	Score          int     `json:"score"`
	Grade          string  `json:"grade"`
	Status         string  `json:"status"`     // compliant, partial, non-compliant
	TotalControls  int     `json:"totalControls"`
	PassedControls int     `json:"passedControls"`
	FailedControls int     `json:"failedControls"`
	WaivedControls int     `json:"waivedControls"`
	CoveragePct    float64 `json:"coveragePct"`
	CriticalGaps   int     `json:"criticalGaps"`
}

// ComplianceCtrlResult is the assessment of one compliance control.
type ComplianceCtrlResult struct {
	ID          string   `json:"id"`
	Framework   string   `json:"framework"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`     // pass, fail, warn, na
	Severity    string   `json:"severity"`
	Evidence    string   `json:"evidence"`
	Remediation string   `json:"remediation"`
}

// CrossFWGap identifies controls that fail across multiple frameworks.
type CrossFWGap struct {
	Gap           string   `json:"gap"`
	Severity      string   `json:"severity"`
	Frameworks    []string `json:"frameworks"`
	Impact        string   `json:"impact"`
}

// ComplianceRemediation is a prioritized fix item.
type ComplianceRemediation struct {
	Priority     int    `json:"priority"`
	ControlIDs   []string `json:"controlIds"`
	Action       string `json:"action"`
	Effort       string `json:"effort"`
	Frameworks   []string `json:"frameworksAffected"`
}

// handleCompliancePosture maps cluster security against compliance frameworks.
// GET /api/security/compliance-posture
func (s *Server) handleCompliancePosture(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ComplianceFrameworkResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	// Collect security signals from the cluster
	deploys, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// === Gather security signals ===
	privilegedContainers := 0
	runAsRoot := 0
	noReadOnlyFS := 0
	noSecurityContext := 0
	noResourceLimits := 0
	imagesFromUntrustedReg := 0
	noImagePullPolicy := 0
	latestTagImages := 0
	plainTextSecrets := 0
	oldSecrets := 0

	totalWorkloads := 0
	for _, dep := range deploys.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		totalWorkloads++
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					privilegedContainers++
				}
				if c.SecurityContext.RunAsNonRoot == nil || (c.SecurityContext.RunAsNonRoot != nil && !*c.SecurityContext.RunAsNonRoot) {
					runAsRoot++
				}
				if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
					noReadOnlyFS++
				}
			} else {
				noSecurityContext++
			}

			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
				noResourceLimits++
			}

			// Image checks
			imageRef := c.Image
			if strings.Contains(imageRef, "latest") {
				latestTagImages++
			}
			if !strings.Contains(imageRef, ".") && !strings.HasPrefix(imageRef, "docker.io/") {
				imagesFromUntrustedReg++
			}
			if c.ImagePullPolicy == "" || c.ImagePullPolicy == corev1.PullNever {
				noImagePullPolicy++
			}
		}
	}

	// Secret checks
	now := time.Now()
	for _, sec := range secrets.Items {
		if systemNS[sec.Namespace] {
			continue
		}
		if sec.Type == corev1.SecretTypeOpaque {
			plainTextSecrets++
		}
		if now.Sub(sec.CreationTimestamp.Time) > 90*24*time.Hour {
			oldSecrets++
		}
	}

	// Pod security checks (running pods)
	hostNetworkPods := 0
	hostPIDPods := 0
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		if pod.Spec.HostNetwork {
			hostNetworkPods++
		}
		if pod.Spec.HostPID {
			hostPIDPods++
		}
	}

	totalNS := 0
	for _, ns := range nsList.Items {
		if !systemNS[ns.Name] {
			totalNS++
		}
	}

	// === Define compliance controls and assess ===
	// Each control maps to multiple frameworks
	type ctrlDef struct {
		ID, Title, Category, Description, Remediation string
		Severity                                      string
		PassFunc                                      func() (bool, string)
	}

	controls := []ctrlDef{
		// Access Control
		{"AC-1", "Privileged containers", "Access Control",
			"No containers should run with privileged=true",
			"Remove privileged flag or use least-privilege security context",
			"critical",
			func() (bool, string) {
				return privilegedContainers == 0, fmt.Sprintf("%d privileged containers detected", privilegedContainers)
			}},
		{"AC-2", "Root user execution", "Access Control",
			"Containers should not run as root user",
			"Set runAsNonRoot: true in securityContext",
			"high",
			func() (bool, string) {
				return runAsRoot == 0, fmt.Sprintf("%d containers run as root", runAsRoot)
			}},
		{"AC-3", "Host namespace access", "Access Control",
			"Pods should not use hostNetwork or hostPID",
			"Remove hostNetwork and hostPID from pod spec",
			"critical",
			func() (bool, string) {
				return hostNetworkPods == 0 && hostPIDPods == 0,
					fmt.Sprintf("hostNetwork: %d, hostPID: %d", hostNetworkPods, hostPIDPods)
			}},

		// Data Protection
		{"DP-1", "Secret encryption", "Data Protection",
			"Secrets should not be stored in plain Kubernetes Secrets without encryption-at-rest",
			"Enable etcd encryption-at-rest or use external secret management",
			"high",
			func() (bool, string) {
				return plainTextSecrets == 0, fmt.Sprintf("%d opaque secrets without external management", plainTextSecrets)
			}},
		{"DP-2", "Secret rotation", "Data Protection",
			"Secrets older than 90 days should be rotated",
			"Implement automated secret rotation policy",
			"medium",
			func() (bool, string) {
				return oldSecrets == 0, fmt.Sprintf("%d secrets older than 90 days", oldSecrets)
			}},
		{"DP-3", "Read-only filesystem", "Data Protection",
			"Containers should use read-only root filesystem",
			"Set readOnlyRootFilesystem: true in securityContext",
			"medium",
			func() (bool, string) {
				return noReadOnlyFS == 0, fmt.Sprintf("%d containers with writable root FS", noReadOnlyFS)
			}},

		// Supply Chain
		{"SC-1", "Image immutability", "Supply Chain",
			"Containers should not use 'latest' tag images",
			"Pin image versions with specific digests",
			"medium",
			func() (bool, string) {
				return latestTagImages == 0, fmt.Sprintf("%d containers using latest tag", latestTagImages)
			}},
		{"SC-2", "Image registry trust", "Supply Chain",
			"Images should come from trusted, private registries",
			"Use private registry with image signing (cosign)",
			"high",
			func() (bool, string) {
				return imagesFromUntrustedReg == 0, fmt.Sprintf("%d images from untrusted registries", imagesFromUntrustedReg)
			}},
		{"SC-3", "Image pull policy", "Supply Chain",
			"Image pull policy should be Always or IfNotPresent (not Never)",
			"Set imagePullPolicy explicitly",
			"low",
			func() (bool, string) {
				return noImagePullPolicy == 0, fmt.Sprintf("%d containers without explicit pull policy", noImagePullPolicy)
			}},

		// Resource Governance
		{"RG-1", "Resource limits", "Resource Governance",
			"All containers should have CPU and memory limits",
			"Add resource limits to all container specs",
			"medium",
			func() (bool, string) {
				return noResourceLimits == 0, fmt.Sprintf("%d containers without limits", noResourceLimits)
			}},
		{"RG-2", "Security context", "Resource Governance",
			"All containers should have explicit security context",
			"Add securityContext to all pod templates",
			"medium",
			func() (bool, string) {
				return noSecurityContext == 0, fmt.Sprintf("%d containers without securityContext", noSecurityContext)
			}},
	}

	// Framework mappings (which controls apply to which frameworks)
	frameworkMap := map[string][]string{
		"SOC2":   {"AC-1", "AC-2", "AC-3", "DP-1", "DP-2", "SC-1", "SC-2", "RG-1", "RG-2"},
		"PCI-DSS": {"AC-1", "AC-2", "AC-3", "DP-1", "DP-2", "DP-3", "SC-1", "SC-2", "SC-3", "RG-1"},
		"HIPAA":  {"AC-1", "AC-2", "DP-1", "DP-2", "DP-3", "SC-2", "RG-1"},
		"NIST":   {"AC-1", "AC-2", "AC-3", "DP-1", "DP-3", "SC-1", "SC-2", "SC-3", "RG-1", "RG-2"},
		"GDPR":   {"DP-1", "DP-2", "DP-3", "SC-2", "RG-1"},
	}

	// Assess all controls
	controlResults := make(map[string]bool)
	for _, ctrl := range controls {
		passed, evidence := ctrl.PassFunc()
		status := "pass"
		if !passed {
			status = "fail"
		}
		result.ControlResults = append(result.ControlResults, ComplianceCtrlResult{
			ID:          ctrl.ID,
			Framework:   "all",
			Category:    ctrl.Category,
			Title:       ctrl.Title,
			Description: ctrl.Description,
			Status:      status,
			Severity:    ctrl.Severity,
			Evidence:    evidence,
			Remediation: ctrl.Remediation,
		})
		controlResults[ctrl.ID] = passed
	}

	// Sort controls: failures first, by severity
	sort.Slice(result.ControlResults, func(i, j int) bool {
		if result.ControlResults[i].Status != result.ControlResults[j].Status {
			return result.ControlResults[i].Status == "fail"
		}
		return severityRankMap(result.ControlResults[i].Severity) > severityRankMap(result.ControlResults[j].Severity)
	})

	// === Build framework postures ===
	totalScore := 0
	for fwName, controlIDs := range frameworkMap {
		passed := 0
		failed := 0
		critGaps := 0
		for _, cid := range controlIDs {
			if controlResults[cid] {
				passed++
			} else {
				failed++
				// Check if this failure is critical
				for _, ctrl := range controls {
					if ctrl.ID == cid && ctrl.Severity == "critical" {
						critGaps++
					}
				}
			}
		}
		total := len(controlIDs)
		coverage := float64(passed) / float64(total) * 100
		status := "compliant"
		if coverage < 50 {
			status = "non-compliant"
		} else if coverage < 100 {
			status = "partial"
		}

		score := int(coverage)
		totalScore += score

		result.Frameworks = append(result.Frameworks, ComplianceFwPosture{
			Framework:      fwName,
			FullName:       getFrameworkFullName(fwName),
			Score:          score,
			Grade:          goldenScoreToGrade(score),
			Status:         status,
			TotalControls:  total,
			PassedControls: passed,
			FailedControls: failed,
			CoveragePct:    coverage,
			CriticalGaps:   critGaps,
		})
	}

	// Overall score
	if len(frameworkMap) > 0 {
		result.OverallScore = totalScore / len(frameworkMap)
	}
	result.OverallGrade = goldenScoreToGrade(result.OverallScore)

	// Sort frameworks by score (worst first)
	sort.Slice(result.Frameworks, func(i, j int) bool {
		return result.Frameworks[i].Score < result.Frameworks[j].Score
	})

	// Cross-framework gaps: controls that fail and affect multiple frameworks
	controlFrameworkCount := make(map[string]int)
	for _, ids := range frameworkMap {
		for _, cid := range ids {
			controlFrameworkCount[cid]++
		}
	}

	for _, cr := range result.ControlResults {
		if cr.Status == "fail" {
			fwCount := controlFrameworkCount[cr.ID]
			if fwCount >= 3 {
				affectedFWs := []string{}
				for fwName, ids := range frameworkMap {
					for _, cid := range ids {
						if cid == cr.ID {
							affectedFWs = append(affectedFWs, fwName)
						}
					}
				}
				result.CrossFramework = append(result.CrossFramework, CrossFWGap{
					Gap:        cr.Title,
					Severity:   cr.Severity,
					Frameworks: affectedFWs,
					Impact:     fmt.Sprintf("Affects %d compliance frameworks: %s", fwCount, strings.Join(affectedFWs, ", ")),
				})
			}
		}
	}

	// Remediation plan
	result.RemediationPlan = generateComplianceRemediation(result)
	result.Recommendations = generateComplianceRecs(result)

	writeJSON(w, result)
}

// getFrameworkFullName returns the full framework name.
func getFrameworkFullName(fw string) string {
	names := map[string]string{
		"SOC2":    "SOC 2 Type II",
		"PCI-DSS": "PCI-DSS v4.0",
		"HIPAA":   "HIPAA Security Rule",
		"NIST":    "NIST 800-53 Rev 5",
		"GDPR":    "GDPR (EU)",
	}
	if name, ok := names[fw]; ok {
		return name
	}
	return fw
}

// generateComplianceRemediation creates prioritized remediation items.
func generateComplianceRemediation(result ComplianceFrameworkResult) []ComplianceRemediation {
	var items []ComplianceRemediation
	priority := 1

	// Group failed controls by severity
	critFails := []ComplianceCtrlResult{}
	highFails := []ComplianceCtrlResult{}
	for _, cr := range result.ControlResults {
		if cr.Status == "fail" {
			switch cr.Severity {
			case "critical":
				critFails = append(critFails, cr)
			case "high":
				highFails = append(highFails, cr)
			}
		}
	}

	// Critical first
	for _, cr := range critFails {
		items = append(items, ComplianceRemediation{
			Priority:     priority,
			ControlIDs:   []string{cr.ID},
			Action:       cr.Remediation,
			Effort:       "low",
			Frameworks:   getAllFrameworksWithControl(cr.ID),
		})
		priority++
	}

	// High severity
	for _, cr := range highFails {
		items = append(items, ComplianceRemediation{
			Priority:     priority,
			ControlIDs:   []string{cr.ID},
			Action:       cr.Remediation,
			Effort:       "medium",
			Frameworks:   getAllFrameworksWithControl(cr.ID),
		})
		priority++
	}

	return items
}

// getAllFrameworksWithControl returns frameworks that include a given control.
func getAllFrameworksWithControl(ctrlID string) []string {
	frameworkMap := map[string][]string{
		"SOC2":    {"AC-1", "AC-2", "AC-3", "DP-1", "DP-2", "SC-1", "SC-2", "RG-1", "RG-2"},
		"PCI-DSS": {"AC-1", "AC-2", "AC-3", "DP-1", "DP-2", "DP-3", "SC-1", "SC-2", "SC-3", "RG-1"},
		"HIPAA":   {"AC-1", "AC-2", "DP-1", "DP-2", "DP-3", "SC-2", "RG-1"},
		"NIST":    {"AC-1", "AC-2", "AC-3", "DP-1", "DP-3", "SC-1", "SC-2", "SC-3", "RG-1", "RG-2"},
		"GDPR":    {"DP-1", "DP-2", "DP-3", "SC-2", "RG-1"},
	}
	var result []string
	for fwName, ids := range frameworkMap {
		for _, cid := range ids {
			if cid == ctrlID {
				result = append(result, fwName)
				break
			}
		}
	}
	return result
}

// generateComplianceRecs produces actionable recommendations.
func generateComplianceRecs(result ComplianceFrameworkResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Overall compliance posture: %d/100 (grade %s)", result.OverallScore, result.OverallGrade))

	totalFails := 0
	for _, cr := range result.ControlResults {
		if cr.Status == "fail" {
			totalFails++
		}
	}
	if totalFails > 0 {
		recs = append(recs, fmt.Sprintf("%d compliance controls failing across frameworks", totalFails))
	}

	// Worst framework
	if len(result.Frameworks) > 0 {
		worst := result.Frameworks[0]
		recs = append(recs, fmt.Sprintf("Lowest scoring framework: %s (%d/100, %s)", worst.FullName, worst.Score, worst.Status))
	}

	// Cross-framework gaps
	critCross := 0
	for _, gap := range result.CrossFramework {
		if gap.Severity == "critical" {
			critCross++
		}
	}
	if critCross > 0 {
		recs = append(recs, fmt.Sprintf("%d critical gaps affect 3+ frameworks — fixing these has maximum compliance ROI", critCross))
	}

	// Remediation plan top item
	if len(result.RemediationPlan) > 0 {
		top := result.RemediationPlan[0]
		recs = append(recs, fmt.Sprintf("Priority #1: %s (affects %d frameworks)", top.Action, len(top.Frameworks)))
	}

	if len(recs) == 1 {
		recs = append(recs, "All compliance frameworks are passing — maintain current controls")
	}

	return recs
}
