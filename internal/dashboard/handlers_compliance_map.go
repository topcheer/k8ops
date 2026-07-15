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

// ComplianceMapResult maps cluster configuration to SOC2/PCI-DSS/HIPAA controls.
type ComplianceMapResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Frameworks      []FrameworkResult    `json:"frameworks"`
	Summary         ComplianceMapSummary `json:"summary"`
	FailingControls []ControlFinding     `json:"failingControls"`
	ByControl       []ControlFinding     `json:"byControl"`
	Recommendations []string             `json:"recommendations"`
	OverallScore    int                  `json:"overallScore"`
}

// ComplianceMapSummary aggregates cross-framework compliance stats.
type ComplianceMapSummary struct {
	FrameworksAssessed int     `json:"frameworksAssessed"`
	TotalControls      int     `json:"totalControls"`
	PassingControls    int     `json:"passingControls"`
	FailingControls    int     `json:"failingControls"`
	WarningControls    int     `json:"warningControls"`
	OverallPassRate    float64 `json:"overallPassRate"`
}

// FrameworkResult shows compliance status for one framework.
type FrameworkResult struct {
	Name          string          `json:"name"` // SOC2, PCI-DSS, HIPAA
	Description   string          `json:"description"`
	TotalControls int             `json:"totalControls"`
	Passing       int             `json:"passingControls"`
	Failing       int             `json:"failingControls"`
	Warnings      int             `json:"warningControls"`
	PassRate      float64         `json:"passRate"`
	Status        string          `json:"status"` // compliant, partial, non-compliant
	Controls      []ControlResult `json:"controls"`
}

// ControlResult shows one compliance control check.
type ControlResult struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pass, fail, warn
	Description string `json:"description"`
	Remediation string `json:"remediation,omitempty"`
}

// ControlFinding is a failing/warning control with context.
type ControlFinding struct {
	Framework   string `json:"framework"`
	ControlID   string `json:"controlId"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation"`
}

// handleComplianceMap maps cluster configuration to SOC2/PCI-DSS/HIPAA controls.
// GET /api/security/compliance-map
func (s *Server) handleComplianceMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ComplianceMapResult{ScannedAt: time.Now()}

	// Collect cluster state
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{Limit: 500})

	// Detect encryption/audit/tls state
	hasAuditLog := false
	hasRBAC := false
	hasNetworkPolicy := false
	hasPrivilegedPods := false
	hasHostPathPods := false
	hasLatestImages := false

	// Check pods for security posture
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				hasPrivilegedPods = true
			}
			if len(c.Resources.Requests) == 0 {
			}
			if strings.Contains(strings.ToLower(c.Image), ":latest") {
				hasLatestImages = true
			}
		}
		for _, v := range pod.Spec.Volumes {
			if v.HostPath != nil {
				hasHostPathPods = true
			}
		}
	}

	// Check namespaces for default namespace usage
	for _, ns := range namespaces.Items {
		if ns.Name == "default" {
			for _, pod := range pods.Items {
				if pod.Namespace == "default" && pod.Status.Phase == corev1.PodRunning {
				}
			}
		}
	}

	// Check for audit logging (API server audit policy)
	_, auditErr := rc.clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "audit-policy", metav1.GetOptions{})
	if auditErr == nil {
		hasAuditLog = true
	}

	// Check for RBAC
	crbs, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if crbs != nil && len(crbs.Items) > 0 {
		hasRBAC = true
	}

	// Check for network policies
	nps, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if nps != nil && len(nps.Items) > 0 {
		hasNetworkPolicy = true
	}

	// Check for encryption at rest (approximate: check if etcd encryption secret exists)
	_, encErr := rc.clientset.CoreV1().Secrets("kube-system").Get(ctx, "encryption-config", metav1.GetOptions{})
	hasEncryptionAtRest := encErr == nil

	// Check for TLS on services
	hasPublicSvcNoTLS := false
	if services != nil {
		for _, svc := range services.Items {
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				hasPublicSvcNoTLS = true
			}
		}
	}

	// Check for secret rotation (stale secrets)
	hasStaleSecrets := false
	if secrets != nil {
		for _, secret := range secrets.Items {
			age := time.Since(secret.CreationTimestamp.Time)
			if age > 90*24*time.Hour && secret.Type == corev1.SecretTypeOpaque {
				hasStaleSecrets = true
				break
			}
		}
	}

	// Check node versions (patch status - approximate from kubelet version)
	hasNodeVersionDrift := false
	if nodes != nil && len(nodes.Items) > 1 {
		versions := map[string]bool{}
		for _, node := range nodes.Items {
			ver := node.Status.NodeInfo.KubeletVersion
			versions[ver] = true
		}
		if len(versions) > 1 {
			hasNodeVersionDrift = true
		}
	}

	// Check for image scanning (approximate: check if gatekeeper/kyverno exists)
	hasPolicyEnforcement := false
	for _, pod := range pods.Items {
		nameLower := strings.ToLower(pod.Name)
		if strings.Contains(nameLower, "gatekeeper") || strings.Contains(nameLower, "kyverno") {
			hasPolicyEnforcement = true
			break
		}
	}

	// === SOC2 Controls ===
	soc2Checks := []ControlResult{
		makeControl("SOC2-CC6.1", "Access Control", "RBAC enabled and enforced",
			hasRBAC, "Kubernetes RBAC is not configured", "Enable RBAC and define least-privilege roles"),
		makeControl("SOC2-CC6.6", "Access Control", "Network policies restrict pod-to-pod communication",
			hasNetworkPolicy, "No NetworkPolicy found — all pods can communicate freely", "Define default-deny NetworkPolicies per namespace"),
		makeControl("SOC2-CC6.7", "Access Control", "No privileged containers running",
			!hasPrivilegedPods, "Privileged containers detected — host-level access risk", "Remove privileged:true or use Pod Security Admission restricted"),
		makeControl("SOC2-CC7.1", "Monitoring", "Audit logging configured",
			hasAuditLog, "No audit-policy ConfigMap found in kube-system", "Configure API server audit logging with appropriate policy"),
		makeControl("SOC2-CC7.2", "Monitoring", "Policy enforcement (OPA/Kyverno) deployed",
			hasPolicyEnforcement, "No policy engine detected — workloads not validated at admission", "Deploy OPA Gatekeeper or Kyverno for admission control"),
		makeControl("SOC2-CC8.1", "Change Management", "No :latest images in production",
			!hasLatestImages, ":latest image tags detected — non-reproducible deployments", "Use versioned image tags for all containers"),
		makeControl("SOC2-CC8.2", "Change Management", "Node versions consistent (no drift)",
			!hasNodeVersionDrift, "Multiple kubelet versions detected — patch level inconsistency", "Ensure all nodes run the same Kubernetes patch version"),
	}

	// === PCI-DSS Controls ===
	pciChecks := []ControlResult{
		makeControl("PCI-3.4", "Data Protection", "Encryption at rest for secrets",
			hasEncryptionAtRest, "No encryption-config Secret found in kube-system", "Enable etcd encryption at rest for Secret resources"),
		makeControl("PCI-4.1", "Network Security", "Network policies for segmentation",
			hasNetworkPolicy, "No NetworkPolicy — cardholder data environment not isolated", "Define NetworkPolicies to isolate PCI workloads"),
		makeControl("PCI-6.2", "Vulnerability Mgmt", "Policy enforcement for image scanning",
			hasPolicyEnforcement, "No admission policy engine for image validation", "Deploy admission controller that validates image signatures and scans"),
		makeControl("PCI-7.1", "Access Control", "No privileged containers",
			!hasPrivilegedPods, "Privileged containers can bypass PCI isolation controls", "Enforce restricted Pod Security Standards"),
		makeControl("PCI-7.2", "Access Control", "RBAC least-privilege",
			hasRBAC, "RBAC not configured — no access control enforcement", "Enable RBAC with namespace-scoped roles for PCI workloads"),
		makeControl("PCI-8.2", "Auth & Identity", "Secret rotation (no stale credentials >90d)",
			!hasStaleSecrets, "Secrets older than 90 days detected — credential rotation overdue", "Implement automated secret rotation (e.g., External Secrets Operator)"),
		makeControl("PCI-10.1", "Logging", "Audit logging enabled",
			hasAuditLog, "No audit logging — PCI activity not recorded", "Configure comprehensive audit logging for all API operations"),
	}

	// === HIPAA Controls ===
	hipaaChecks := []ControlResult{
		makeControl("HIPAA-164.312(a)", "Access Control", "RBAC + NetworkPolicy isolation",
			hasRBAC && hasNetworkPolicy, "PHI workloads not properly isolated", "Enable RBAC and NetworkPolicies for HIPAA-scoped namespaces"),
		makeControl("HIPAA-164.312(b)", "Audit Control", "Audit logging configured",
			hasAuditLog, "No audit logging — PHI access not tracked", "Enable API server audit logging with PHI-relevant events"),
		makeControl("HIPAA-164.312(c)", "Integrity", "No hostPath volume mounts",
			!hasHostPathPods, "hostPath volumes detected — can modify host files (integrity risk)", "Remove hostPath mounts and use PVCs"),
		makeControl("HIPAA-164.312(e)", "Transmission", "TLS for all external services",
			!hasPublicSvcNoTLS, "LoadBalancer services without TLS detected", "Route all external traffic through Ingress with TLS"),
	}

	soc2Result := buildFramework("SOC2 Type II", "Service Organization Control 2 — security, availability, confidentiality", soc2Checks)
	pciResult := buildFramework("PCI-DSS 4.0", "Payment Card Industry Data Security Standard", pciChecks)
	hipaaResult := buildFramework("HIPAA", "Health Insurance Portability and Accountability Act", hipaaChecks)

	result.Frameworks = []FrameworkResult{soc2Result, pciResult, hipaaResult}

	// Summary
	totalControls := 0
	totalPass := 0
	totalFail := 0
	totalWarn := 0
	for _, fw := range result.Frameworks {
		totalControls += fw.TotalControls
		totalPass += fw.Passing
		totalFail += fw.Failing
		totalWarn += fw.Warnings
	}
	result.Summary = ComplianceMapSummary{
		FrameworksAssessed: len(result.Frameworks),
		TotalControls:      totalControls,
		PassingControls:    totalPass,
		FailingControls:    totalFail,
		WarningControls:    totalWarn,
	}
	if totalControls > 0 {
		result.Summary.OverallPassRate = float64(totalPass) / float64(totalControls) * 100
	}

	// Failing controls detail
	for _, fw := range result.Frameworks {
		for _, c := range fw.Controls {
			if c.Status != "pass" {
				severity := "medium"
				if c.Status == "fail" {
					severity = "high"
				}
				result.FailingControls = append(result.FailingControls, ControlFinding{
					Framework:   fw.Name,
					ControlID:   c.ID,
					Title:       c.Title,
					Status:      c.Status,
					Severity:    severity,
					Detail:      c.Description,
					Remediation: c.Remediation,
				})
			}
		}
	}
	sort.Slice(result.FailingControls, func(i, j int) bool {
		return result.FailingControls[i].Severity > result.FailingControls[j].Severity
	})

	// All controls flat list
	for _, fw := range result.Frameworks {
		for _, c := range fw.Controls {
			result.ByControl = append(result.ByControl, ControlFinding{
				Framework:   fw.Name,
				ControlID:   c.ID,
				Title:       c.Title,
				Status:      c.Status,
				Severity:    "info",
				Detail:      c.Description,
				Remediation: c.Remediation,
			})
		}
	}

	// Overall score
	result.OverallScore = int(result.Summary.OverallPassRate)

	// Recommendations
	result.Recommendations = generateComplianceMapRecs(result)

	writeJSON(w, result)
}

// makeControl creates a ControlResult from a boolean condition.
func makeControl(id, category, title string, passed bool, failMsg, remediation string) ControlResult {
	if passed {
		return ControlResult{
			ID: id, Category: category, Title: title,
			Status:      "pass",
			Description: fmt.Sprintf("%s: compliant", title),
		}
	}
	return ControlResult{
		ID:          id,
		Category:    category,
		Title:       title,
		Status:      "fail",
		Description: failMsg,
		Remediation: remediation,
	}
}

// buildFramework creates a FrameworkResult from controls.
func buildFramework(name, description string, controls []ControlResult) FrameworkResult {
	passing, failing, warnings := 0, 0, 0
	for _, c := range controls {
		switch c.Status {
		case "pass":
			passing++
		case "fail":
			failing++
		case "warn":
			warnings++
		}
	}
	total := len(controls)
	passRate := 0.0
	if total > 0 {
		passRate = float64(passing) / float64(total) * 100
	}
	status := "compliant"
	if passRate < 50 {
		status = "non-compliant"
	} else if passRate < 100 {
		status = "partial"
	}

	return FrameworkResult{
		Name:          name,
		Description:   description,
		TotalControls: total,
		Passing:       passing,
		Failing:       failing,
		Warnings:      warnings,
		PassRate:      passRate,
		Status:        status,
		Controls:      controls,
	}
}

// generateComplianceMapRecs produces recommendations.
func generateComplianceMapRecs(result ComplianceMapResult) []string {
	var recs []string

	for _, fw := range result.Frameworks {
		recs = append(recs, fmt.Sprintf("%s: %.0f%% compliant (%d/%d controls passing, status: %s)",
			fw.Name, fw.PassRate, fw.Passing, fw.TotalControls, fw.Status))
	}

	if len(result.FailingControls) > 0 {
		highSeverity := 0
		for _, fc := range result.FailingControls {
			if fc.Severity == "high" {
				highSeverity++
			}
		}
		recs = append(recs, fmt.Sprintf("%d failing control(s) (%d high severity) — prioritize remediation of high-severity findings", len(result.FailingControls), highSeverity))
	}

	// Top remediation priorities
	for i, fc := range result.FailingControls {
		if i >= 3 || fc.Severity != "high" {
			break
		}
		recs = append(recs, fmt.Sprintf("Priority: %s %s — %s", fc.Framework, fc.ControlID, fc.Remediation))
	}

	if result.OverallScore >= 90 {
		recs = append(recs, fmt.Sprintf("Overall compliance score: %d/100 — cluster meets most framework requirements", result.OverallScore))
	} else if result.OverallScore < 50 {
		recs = append(recs, fmt.Sprintf("Overall compliance score: %d/100 — significant gaps require immediate attention", result.OverallScore))
	}

	return recs
}
