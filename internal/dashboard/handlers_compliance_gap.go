package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ComplianceGapResult performs a comprehensive gap analysis against common
// compliance frameworks (CIS, NIST, SOC2). It maps cluster findings to
// compliance controls and generates a remediation roadmap.
type ComplianceGapResult struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	Summary         ComplianceGapSummary       `json:"summary"`
	ByFramework     []ComplianceFramework      `json:"byFramework"`
	Findings        []ComplianceFinding        `json:"findings"`
	RemediationPlan []ComplianceGapRemediation `json:"remediationPlan"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Recommendations []string                   `json:"recommendations"`
}

type ComplianceGapSummary struct {
	TotalControls  int `json:"totalControls"`
	PassedControls int `json:"passedControls"`
	FailedControls int `json:"failedControls"`
	CriticalGaps   int `json:"criticalGaps"`
	HighGaps       int `json:"highGaps"`
	MediumGaps     int `json:"mediumGaps"`
}

type ComplianceFramework struct {
	Name        string `json:"name"`
	Controls    int    `json:"controls"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	CoveragePct int    `json:"coveragePct"`
}

type ComplianceFinding struct {
	Control     string `json:"control"`
	Framework   string `json:"framework"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation"`
}

type ComplianceGapRemediation struct {
	Priority int    `json:"priority"`
	Control  string `json:"control"`
	Action   string `json:"action"`
	Effort   string `json:"effort"`
	Impact   string `json:"impact"`
}

// handleComplianceGap handles GET /api/security/compliance-gap
func (s *Server) handleComplianceGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ComplianceGapResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	// Collect findings
	var findings []ComplianceFinding
	addFinding := func(control, framework, severity, detail, remediation string) {
		findings = append(findings, ComplianceFinding{
			Control: control, Framework: framework,
			Status: "fail", Severity: severity,
			Detail: detail, Remediation: remediation,
		})
	}

	// CIS Benchmark Controls
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			// CIS 5.1.1: Ensure runAsNonRoot
			if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil {
				addFinding("CIS-5.1.1", "CIS", "high",
					fmt.Sprintf("%s/%s: container %s missing runAsNonRoot", d.Namespace, d.Name, c.Name),
					"Set securityContext.runAsNonRoot=true")
			}
			// CIS 5.1.2: Ensure readOnlyRootFilesystem
			if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil {
				addFinding("CIS-5.1.2", "CIS", "medium",
					fmt.Sprintf("%s/%s: container %s missing readOnlyRootFilesystem", d.Namespace, d.Name, c.Name),
					"Set securityContext.readOnlyRootFilesystem=true")
			}
			// CIS 5.1.3: Ensure privilege escalation prevented
			if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
				addFinding("CIS-5.1.3", "CIS", "high",
					fmt.Sprintf("%s/%s: container %s allows privilege escalation", d.Namespace, d.Name, c.Name),
					"Set securityContext.allowPrivilegeEscalation=false")
			}
			// CIS 5.1.5: Ensure resources have limits
			if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
				addFinding("CIS-5.1.5", "CIS", "medium",
					fmt.Sprintf("%s/%s: container %s missing resource limits", d.Namespace, d.Name, c.Name),
					"Set resources.limits.cpu and resources.limits.memory")
			}
		}
	}

	// NIST Controls
	nsCount := 0
	for _, ns := range namespaces.Items {
		if !isSystemNamespace(ns.Name) {
			nsCount++
		}
	}
	// NIST AC-3: Resource quotas enforce access control
	if nsCount > 0 && len(quotas.Items) == 0 {
		addFinding("NIST-AC-3", "NIST", "high",
			"No ResourceQuotas in any namespace",
			"Create ResourceQuota per namespace using /api/scalability/quota-generator")
	}
	// NIST SC-7: Network policies enforce boundary protection
	netpolNS := 0
	npSet := make(map[string]bool)
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			npSet[np.Namespace] = true
		}
	}
	netpolNS = len(npSet)
	if netpolNS < nsCount/2 {
		addFinding("NIST-SC-7", "NIST", "high",
			fmt.Sprintf("Only %d/%d namespaces have NetworkPolicy", netpolNS, nsCount),
			"Deploy NetworkPolicy using /api/security/netpol-generator")
	}

	// SOC2 Controls
	// SOC2 CC-6: Multi-node HA for availability
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerCount++
		}
	}
	if workerCount < 2 {
		addFinding("SOC2-CC6", "SOC2", "critical",
			fmt.Sprintf("Single worker node (%d), no HA", workerCount),
			"Add worker nodes for availability")
	}

	// CIS 1.1: Encrypted storage (PVC encryption is assumed at cloud provider level)
	encryptedPVC := 0
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		// Check for encryption annotation (heuristic)
		if pvc.Annotations["encryption"] != "" || pvc.Spec.StorageClassName != nil {
			encryptedPVC++
		}
	}

	// Build framework stats
	fwMap := make(map[string]*ComplianceFramework)
	for _, f := range findings {
		if _, ok := fwMap[f.Framework]; !ok {
			fwMap[f.Framework] = &ComplianceFramework{Name: f.Framework}
		}
		fwMap[f.Framework].Controls++
		fwMap[f.Framework].Failed++
		switch f.Severity {
		case "critical":
			result.Summary.CriticalGaps++
		case "high":
			result.Summary.HighGaps++
		case "medium":
			result.Summary.MediumGaps++
		}
	}

	// Add passed controls (estimated)
	totalCIS := 15 // CIS has ~15 container-level controls
	totalNIST := 5
	totalSOC2 := 3
	fwPassed := map[string]int{"CIS": totalCIS, "NIST": totalNIST, "SOC2": totalSOC2}

	for fw, passed := range fwPassed {
		if _, ok := fwMap[fw]; !ok {
			fwMap[fw] = &ComplianceFramework{Name: fw}
		}
		fw := fwMap[fw]
		fw.Passed = passed
		fw.Controls = fw.Failed + passed
		if fw.Controls > 0 {
			fw.CoveragePct = fw.Passed * 100 / fw.Controls
		}
	}
	for _, fw := range fwMap {
		result.ByFramework = append(result.ByFramework, *fw)
	}
	sort.Slice(result.ByFramework, func(i, j int) bool {
		return result.ByFramework[i].CoveragePct < result.ByFramework[j].CoveragePct
	})

	// Remediation plan
	var remPlan []ComplianceGapRemediation
	priority := 1
	seen := make(map[string]bool)
	for _, f := range findings {
		if seen[f.Control] {
			continue
		}
		seen[f.Control] = true
		effort := "medium"
		impact := "high"
		if f.Severity == "critical" {
			effort = "high"
			impact = "critical"
		}
		remPlan = append(remPlan, ComplianceGapRemediation{
			Priority: priority, Control: f.Control,
			Action: f.Remediation, Effort: effort, Impact: impact,
		})
		priority++
		if len(remPlan) >= 10 {
			break
		}
	}
	sort.Slice(remPlan, func(i, j int) bool {
		return remPlan[i].Priority < remPlan[j].Priority
	})

	result.Summary.TotalControls = totalCIS + totalNIST + totalSOC2
	result.Summary.PassedControls = result.Summary.TotalControls - len(findings)
	result.Summary.FailedControls = len(findings)
	result.Findings = findings
	result.RemediationPlan = remPlan

	// Score
	if result.Summary.TotalControls > 0 {
		result.HealthScore = result.Summary.PassedControls * 100 / result.Summary.TotalControls
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildComplianceGapRecs(&result)
	writeJSON(w, result)
}

func buildComplianceGapRecs(r *ComplianceGapResult) []string {
	recs := []string{
		fmt.Sprintf("合规覆盖率: %d/%d (%d%%), %s", r.Summary.PassedControls, r.Summary.TotalControls, r.HealthScore, r.Grade),
	}
	if r.Summary.CriticalGaps > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 critical 合规缺口需立即修复", r.Summary.CriticalGaps))
	}
	if r.Summary.HighGaps > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 high 合规缺口", r.Summary.HighGaps))
	}
	for _, fw := range r.ByFramework {
		recs = append(recs, fmt.Sprintf("  %s: %d%% (%d/%d)", fw.Name, fw.CoveragePct, fw.Passed, fw.Controls))
	}
	return recs
}
