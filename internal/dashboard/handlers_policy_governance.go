package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyGovResult analyzes admission policy governance: OPA Gatekeeper constraints,
// Kyverno policies, PSA levels, and policy enforcement coverage across namespaces.
type PolicyGovResult struct {
	ScannedAt        time.Time        `json:"scannedAt"`
	Summary          PolicyGovSummary `json:"summary"`
	GatekeeperStatus string           `json:"gatekeeperStatus"`
	KyvernoStatus    string           `json:"kyvernoStatus"`
	PSACoverage      PSACoverage      `json:"psaCoverage"`
	PolicyGaps       []PolicyGap      `json:"policyGaps"`
	EnforcementScore int              `json:"enforcementScore"`
	Grade            string           `json:"grade"`
	Recommendations  []string         `json:"recommendations"`
}

type PolicyGovSummary struct {
	TotalNamespaces  int  `json:"totalNamespaces"`
	NSWithEnforcePSA int  `json:"nsWithEnforcePSA"`
	NSWithAuditPSA   int  `json:"nsWithAuditPSA"`
	NSWithNoPSA      int  `json:"nsWithNoPSA"`
	HasGatekeeper    bool `json:"hasGatekeeper"`
	HasKyverno       bool `json:"hasKyverno"`
	ConstraintCount  int  `json:"constraintCount"`
	PolicyCount      int  `json:"policyCount"`
}

type PSACoverage struct {
	EnforceLevel string  `json:"enforceLevel"`
	Score        int     `json:"score"`
	CoveragePct  float64 `json:"coveragePct"`
	GapCount     int     `json:"gapCount"`
}

type PolicyGap struct {
	Namespace string `json:"namespace"`
	Gap       string `json:"gap"`
	Severity  string `json:"severity"`
	Impact    string `json:"impact"`
}

// handlePolicyGovernance analyzes admission policy governance and enforcement.
// GET /api/security/policy-governance
func (s *Server) handlePolicyGovernance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PolicyGovResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	// Check for Gatekeeper installation
	gatekeeperDeployed := false
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deployments.Items {
		if strings.Contains(strings.ToLower(dep.Name), "gatekeeper") || strings.Contains(strings.ToLower(dep.Namespace), "gatekeeper") {
			gatekeeperDeployed = true
		}
		if strings.Contains(strings.ToLower(dep.Name), "kyverno") || strings.Contains(strings.ToLower(dep.Namespace), "kyverno") {
			result.Summary.HasKyverno = true
		}
	}
	result.Summary.HasGatekeeper = gatekeeperDeployed

	if gatekeeperDeployed {
		result.GatekeeperStatus = "installed"
	} else {
		result.GatekeeperStatus = "not-installed"
	}
	if result.Summary.HasKyverno {
		result.KyvernoStatus = "installed"
	} else {
		result.KyvernoStatus = "not-installed"
	}

	// Check for Gatekeeper CRDs via API extension
	crds, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	_ = crds // Gatekeeper runs as DaemonSet
	for _, ds := range crds.Items {
		if strings.Contains(strings.ToLower(ds.Name), "gatekeeper") {
			gatekeeperDeployed = true
			result.Summary.HasGatekeeper = true
			result.GatekeeperStatus = "installed"
		}
	}

	// Analyze PSA (Pod Security Admission) labels per namespace
	enforceCount := 0
	auditCount := 0
	noPSACount := 0
	for _, ns := range namespaces.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNamespaces++

		enforceLevel := ns.Labels["pod-security.kubernetes.io/enforce"]
		auditLevel := ns.Labels["pod-security.kubernetes.io/audit"]

		hasEnforce := enforceLevel != "" && enforceLevel != "privileged"
		hasAudit := auditLevel != "" && auditLevel != "privileged"

		if hasEnforce {
			enforceCount++
			result.Summary.NSWithEnforcePSA++
		} else if hasAudit {
			auditCount++
			result.Summary.NSWithAuditPSA++
		} else {
			noPSACount++
			result.Summary.NSWithNoPSA++
			severity := "high"
			if ns.Status.Phase == "Terminating" {
				severity = "low"
			}
			result.PolicyGaps = append(result.PolicyGaps, PolicyGap{
				Namespace: ns.Name,
				Gap:       "no PSA enforce/audit labels",
				Severity:  severity,
				Impact:    fmt.Sprintf("Namespace '%s' allows privileged pods — no pod security baseline enforced", ns.Name),
			})
		}
	}

	// PSA Coverage calculation
	enforcePct := 0.0
	if result.Summary.TotalNamespaces > 0 {
		enforcePct = float64(enforceCount) / float64(result.Summary.TotalNamespaces) * 100
	}
	psaScore := int(enforcePct)
	enforceLevel := "none"
	if enforcePct > 80 {
		enforceLevel = "comprehensive"
	} else if enforcePct > 50 {
		enforceLevel = "partial"
	} else if enforcePct > 0 {
		enforceLevel = "minimal"
	}
	result.PSACoverage = PSACoverage{
		EnforceLevel: enforceLevel,
		Score:        psaScore,
		CoveragePct:  enforcePct,
		GapCount:     noPSACount,
	}

	// Score
	score := 0
	if result.Summary.HasGatekeeper || result.Summary.HasKyverno {
		score += 40
	}
	score += psaScore * 40 / 100
	if result.Summary.NSWithAuditPSA > 0 && result.Summary.TotalNamespaces > 0 {
		score += result.Summary.NSWithAuditPSA * 20 / result.Summary.TotalNamespaces
	}
	result.EnforcementScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.EnforcementScore)

	// Sort gaps
	sort.Slice(result.PolicyGaps, func(i, j int) bool {
		return result.PolicyGaps[i].Severity > result.PolicyGaps[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Policy governance: %d/100 (grade %s)", result.EnforcementScore, result.Grade))
	if !result.Summary.HasGatekeeper && !result.Summary.HasKyverno {
		recs = append(recs, "No admission policy engine (Gatekeeper/Kyverno) — install one for OPA-based policy enforcement")
	}
	if result.Summary.NSWithNoPSA > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have no PSA labels — add pod-security.kubernetes.io/enforce=baseline", result.Summary.NSWithNoPSA))
	}
	if enforcePct < 50 {
		recs = append(recs, fmt.Sprintf("PSA enforce coverage at %.0f%% — most namespaces allow privileged workloads", enforcePct))
	}
	if len(recs) == 1 {
		recs = append(recs, "Policy governance is comprehensive — maintain current enforcement posture")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
