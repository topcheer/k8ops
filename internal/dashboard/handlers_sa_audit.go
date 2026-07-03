package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SASeverity describes the risk level of a ServiceAccount finding.
type SASeverity string

const (
	SASevCritical SASeverity = "critical"
	SASevHigh     SASeverity = "high"
	SASevMedium   SASeverity = "medium"
	SASevLow      SASeverity = "low"
	SASevInfo     SASeverity = "info"
)

// SAAuditResult is the full scan output.
type SAAuditResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         SAAuditSummary `json:"summary"`
	ServiceAccounts []SAHealth     `json:"serviceAccounts"`
	Issues          []SAIssue      `json:"issues"`
}

// SAAuditSummary aggregates SA audit statistics.
type SAAuditSummary struct {
	TotalSAs          int            `json:"totalServiceAccounts"`
	UnusedSAs         int            `json:"unusedServiceAccounts"`
	DefaultSAUsed     int            `json:"defaultSAUsedByPods"`
	TokenAutoMount    int            `json:"tokenAutoMountEnabled"`
	HighRiskSAs       int            `json:"highRiskServiceAccounts"`
	SystemSAs         int            `json:"systemServiceAccounts"`
	BySeverity        map[string]int `json:"bySeverity"`
	TotalBindings     int            `json:"totalBindings"`
	ClusterAdminBound int            `json:"clusterAdminBound"`
}

// SAHealth describes the security posture of a single ServiceAccount.
type SAHealth struct {
	Name                string      `json:"name"`
	Namespace           string      `json:"namespace"`
	IsDefault           bool        `json:"isDefault"`
	IsSystem            bool        `json:"isSystem"`
	AgeDays             float64     `json:"ageDays"`
	AutomountToken      bool        `json:"automountToken"`
	UsedByPods          []string    `json:"usedByPods,omitempty"`
	PodCount            int         `json:"podCount"`
	RoleBindings        []SABinding `json:"roleBindings,omitempty"`
	ClusterRoleBindings []SABinding `json:"clusterRoleBindings,omitempty"`
	HasSecrets          bool        `json:"hasSecrets"`
	SecretCount         int         `json:"secretCount"`
	RiskScore           int         `json:"riskScore"`
	MaxSeverity         SASeverity  `json:"maxSeverity"`
	Issues              []string    `json:"issues,omitempty"`
}

// SABinding describes a RoleBinding or ClusterRoleBinding referencing this SA.
type SABinding struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind"` // RoleBinding or ClusterRoleBinding
	RoleName    string `json:"roleName"`
	RoleKind    string `json:"roleKind"` // Role or ClusterRole
	ClusterWide bool   `json:"clusterWide"`
}

// SAIssue is a standalone finding not tied to a specific SA.
type SAIssue struct {
	Severity  SASeverity `json:"severity"`
	Category  string     `json:"category"`
	Resource  string     `json:"resource"`
	Namespace string     `json:"namespace"`
	Detail    string     `json:"detail"`
	Fix       string     `json:"fix"`
}

// handleSAAudit performs a comprehensive ServiceAccount security audit.
// GET /api/security/service-accounts
func (s *Server) handleSAAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	// List ServiceAccounts
	saList, err := rc.clientset.CoreV1().ServiceAccounts(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List Pods for SA usage
	podList, err := rc.clientset.CoreV1().Pods(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List RoleBindings
	rbList, err := rc.clientset.RbacV1().RoleBindings(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List ClusterRoleBindings
	crbList, err := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List ClusterRoles to find cluster-admin references
	crList, err := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build indexes
	clusterAdminSet := make(map[string]bool)
	for _, cr := range crList.Items {
		if cr.Name == "cluster-admin" {
			clusterAdminSet[cr.Name] = true
		}
	}

	// Build SA usage index: ns/saName -> []podName
	saUsage := make(map[string][]string)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		key := fmt.Sprintf("%s/%s", pod.Namespace, saName)
		podRef := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		saUsage[key] = append(saUsage[key], podRef)
	}

	// Build binding indexes
	rbBySA := make(map[string][]SABinding)
	for i := range rbList.Items {
		rb := &rbList.Items[i]
		for _, subj := range rb.Subjects {
			if subj.Kind == "ServiceAccount" {
				key := fmt.Sprintf("%s/%s", subj.Namespace, subj.Name)
				binding := SABinding{
					Name:        rb.Name,
					Namespace:   rb.Namespace,
					Kind:        "RoleBinding",
					RoleName:    rb.RoleRef.Name,
					RoleKind:    rb.RoleRef.Kind,
					ClusterWide: false,
				}
				rbBySA[key] = append(rbBySA[key], binding)
			}
		}
	}

	crbBySA := make(map[string][]SABinding)
	clusterAdminBindings := make(map[string][]SABinding)
	for i := range crbList.Items {
		crb := &crbList.Items[i]
		for _, subj := range crb.Subjects {
			if subj.Kind == "ServiceAccount" {
				key := fmt.Sprintf("%s/%s", subj.Namespace, subj.Name)
				binding := SABinding{
					Name:        crb.Name,
					Kind:        "ClusterRoleBinding",
					RoleName:    crb.RoleRef.Name,
					RoleKind:    crb.RoleRef.Kind,
					ClusterWide: true,
				}
				crbBySA[key] = append(crbBySA[key], binding)
				if crb.RoleRef.Name == "cluster-admin" {
					clusterAdminBindings[key] = append(clusterAdminBindings[key], binding)
				}
			}
		}
	}

	// Analyze each SA
	var saHealths []SAHealth
	var standaloneIssues []SAIssue
	summary := SAAuditSummary{BySeverity: make(map[string]int)}

	for i := range saList.Items {
		sa := &saList.Items[i]
		h := analyzeSAHealth(sa, saUsage, rbBySA, crbBySA, clusterAdminBindings)

		summary.TotalSAs++
		if h.PodCount == 0 {
			summary.UnusedSAs++
		}
		if h.AutomountToken {
			summary.TokenAutoMount++
		}
		if h.IsSystem {
			summary.SystemSAs++
		}
		if h.IsDefault && h.PodCount > 0 {
			summary.DefaultSAUsed++
		}
		if h.MaxSeverity == SASevCritical || h.MaxSeverity == SASevHigh {
			summary.HighRiskSAs++
		}
		summary.BySeverity[string(h.MaxSeverity)]++
		summary.TotalBindings += len(h.RoleBindings) + len(h.ClusterRoleBindings)
		if len(clusterAdminBindings[fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)]) > 0 {
			summary.ClusterAdminBound++
		}

		saHealths = append(saHealths, h)
	}

	// Generate standalone issues for default SA usage by pods
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		if saName == "default" && !isSystemNamespace(pod.Namespace) {
			standaloneIssues = append(standaloneIssues, SAIssue{
				Severity:  SASevLow,
				Category:  "Default SA Usage",
				Resource:  fmt.Sprintf("%s/pod/%s", pod.Namespace, pod.Name),
				Namespace: pod.Namespace,
				Detail:    fmt.Sprintf("Pod %q uses the default service account", pod.Name),
				Fix:       "Create a dedicated service account with least-privilege permissions",
			})
		}
	}

	// Sort: highest risk first
	sort.Slice(saHealths, func(i, j int) bool {
		if saHealths[i].RiskScore != saHealths[j].RiskScore {
			return saHealths[i].RiskScore > saHealths[j].RiskScore
		}
		return saHealths[i].Namespace+"/"+saHealths[i].Name < saHealths[j].Namespace+"/"+saHealths[j].Name
	})

	// Sort issues by severity
	sort.Slice(standaloneIssues, func(i, j int) bool {
		return saSeverityRank(standaloneIssues[i].Severity) < saSeverityRank(standaloneIssues[j].Severity)
	})

	writeJSON(w, SAAuditResult{
		ScannedAt:       time.Now(),
		Summary:         summary,
		ServiceAccounts: saHealths,
		Issues:          standaloneIssues,
	})
}

// analyzeSAHealth evaluates the security posture of a single ServiceAccount.
func analyzeSAHealth(
	sa *corev1.ServiceAccount,
	saUsage map[string][]string,
	rbBySA map[string][]SABinding,
	crbBySA map[string][]SABinding,
	clusterAdminBindings map[string][]SABinding,
) SAHealth {
	key := fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)
	h := SAHealth{
		Name:                sa.Name,
		Namespace:           sa.Namespace,
		IsDefault:           sa.Name == "default",
		IsSystem:            isSystemNamespace(sa.Namespace),
		AgeDays:             time.Since(sa.CreationTimestamp.Time).Hours() / 24,
		UsedByPods:          saUsage[key],
		PodCount:            len(saUsage[key]),
		RoleBindings:        rbBySA[key],
		ClusterRoleBindings: crbBySA[key],
	}

	// Automount token
	h.AutomountToken = true // default is true
	if sa.AutomountServiceAccountToken != nil {
		h.AutomountToken = *sa.AutomountServiceAccountToken
	}

	// Secrets (legacy token mounting)
	h.SecretCount = len(sa.Secrets)
	h.HasSecrets = h.SecretCount > 0

	// Calculate risk score and issues
	h.RiskScore = 0
	var maxSev SASeverity = SASevInfo

	// Issue: Unused SA (> 7 days old)
	if h.PodCount == 0 && !h.IsSystem && h.AgeDays > 7 {
		score := 20
		h.RiskScore += score
		h.Issues = append(h.Issues, fmt.Sprintf("Unused ServiceAccount (%.0f days old, no pods reference it)", h.AgeDays))
		if score > 0 && saSeverityToInt(SASevMedium) < saSeverityToInt(maxSev) {
			maxSev = SASevMedium
		}
	}

	// Issue: automountServiceAccountToken=true with no API usage indicators
	if h.AutomountToken && !h.IsSystem && h.PodCount > 0 && len(h.RoleBindings) == 0 && len(h.ClusterRoleBindings) == 0 {
		h.RiskScore += 15
		h.Issues = append(h.Issues, "automountServiceAccountToken=true but SA has no RoleBindings (unnecessary token exposure)")
		if saSeverityToInt(SASevMedium) < saSeverityToInt(maxSev) {
			maxSev = SASevMedium
		}
	}

	// Issue: ClusterRoleBinding to this SA
	if len(h.ClusterRoleBindings) > 0 && !h.IsSystem {
		for _, crb := range h.ClusterRoleBindings {
			if crb.RoleName == "cluster-admin" {
				h.RiskScore += 80
				h.Issues = append(h.Issues, fmt.Sprintf("SA bound to cluster-admin via %s — full cluster control", crb.Name))
				maxSev = SASevCritical
			} else {
				h.RiskScore += 30
				h.Issues = append(h.Issues, fmt.Sprintf("SA has cluster-wide permissions via %s (role: %s)", crb.Name, crb.RoleName))
				if saSeverityToInt(SASevHigh) < saSeverityToInt(maxSev) {
					maxSev = SASevHigh
				}
			}
		}
	}

	// Issue: Default SA being used by pods (non-system namespace)
	if h.IsDefault && h.PodCount > 0 && !h.IsSystem {
		h.RiskScore += 25
		h.Issues = append(h.Issues, fmt.Sprintf("Default SA used by %d pod(s) — should use dedicated SA with least privilege", h.PodCount))
		if saSeverityToInt(SASevMedium) < saSeverityToInt(maxSev) {
			maxSev = SASevMedium
		}
	}

	// Issue: Legacy token secrets (K8s < 1.24)
	if h.HasSecrets && !h.IsSystem {
		h.RiskScore += 10
		h.Issues = append(h.Issues, fmt.Sprintf("SA has %d legacy token secret(s) — long-lived tokens should be replaced with projected tokens", h.SecretCount))
		if saSeverityToInt(SASevLow) < saSeverityToInt(maxSev) {
			maxSev = SASevLow
		}
	}

	// Issue: Old SA with bindings but no pod usage (potential stale access)
	if h.PodCount == 0 && (len(h.RoleBindings) > 0 || len(h.ClusterRoleBindings) > 0) && !h.IsSystem && h.AgeDays > 30 {
		h.RiskScore += 35
		h.Issues = append(h.Issues, fmt.Sprintf("SA has active permissions but no pod usage in %.0f days — revoke stale access", h.AgeDays))
		if saSeverityToInt(SASevHigh) < saSeverityToInt(maxSev) {
			maxSev = SASevHigh
		}
	}

	h.MaxSeverity = maxSev
	return h
}

// saSeverityRank returns sort priority (lower = more severe).
func saSeverityRank(sev SASeverity) int {
	return saSeverityToInt(sev)
}

// saSeverityToInt converts severity to numeric for comparison.
func saSeverityToInt(sev SASeverity) int {
	switch sev {
	case SASevCritical:
		return 0
	case SASevHigh:
		return 1
	case SASevMedium:
		return 2
	case SASevLow:
		return 3
	case SASevInfo:
		return 4
	default:
		return 5
	}
}
