package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBACAuditResult is the RBAC overprivilege & wildcard permission audit.
type RBACAuditResult struct {
	ScannedAt         time.Time          `json:"scannedAt"`
	Summary           RBACAuditSummary   `json:"summary"`
	Overprivileged    []RBACAuditEntry   `json:"overprivilegedRoles"`
	WildcardVerbs     []RBACAuditEntry   `json:"wildcardVerbs"`
	WildcardRes       []RBACAuditEntry   `json:"wildcardResources"`
	ExcessiveBindings []RBACAuditBinding `json:"excessiveBindings"`
	Recommendations   []string           `json:"recommendations"`
}

// RBACAuditSummary aggregates RBAC overprivilege statistics.
type RBACAuditSummary struct {
	TotalClusterRoles     int `json:"totalClusterRoles"`
	TotalRoles            int `json:"totalRoles"`
	OverprivilegedCount   int `json:"overprivilegedCount"`   // roles with wildcard * verbs or resources
	WildcardVerbCount     int `json:"wildcardVerbCount"`     // verbs: ["*"]
	WildcardResourceCount int `json:"wildcardResourceCount"` // resources: ["*"]
	ClusterAdminBindings  int `json:"clusterAdminBindings"`  // non-system cluster-admin bindings
	SystemRoles           int `json:"systemRoles"`
	UserRoles             int `json:"userRoles"`
	HealthScore           int `json:"healthScore"`
}

// RBACAuditEntry describes an overprivileged role.
type RBACAuditEntry struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace,omitempty"`
	IsCluster bool     `json:"isClusterRole"`
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	Issue     string   `json:"issue"`
	Severity  string   `json:"severity"`
}

// RBACAuditBinding describes an excessive binding.
type RBACAuditBinding struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	RoleName    string `json:"roleName"`
	IsCluster   bool   `json:"isClusterBinding"`
	SubjectKind string `json:"subjectKind"` // User, Group, ServiceAccount
	SubjectName string `json:"subjectName"`
	Issue       string `json:"issue"`
	Severity    string `json:"severity"`
}

// handleRBACOverprivilege analyzes RBAC roles for overprivilege and wildcard permissions.
// GET /api/security/rbac-audit
func (s *Server) handleRBACOverprivilege(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	clusterRoles, err := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	roles, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	clusterBindings, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	bindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	result := RBACAuditResult{ScannedAt: now}
	result.Summary.TotalClusterRoles = len(clusterRoles.Items)
	result.Summary.TotalRoles = len(roles.Items)

	// Audit ClusterRoles
	for _, role := range clusterRoles.Items {
		isSystem := isSystemRole(role.Name)
		if isSystem {
			result.Summary.SystemRoles++
			continue
		}
		result.Summary.UserRoles++

		entry := analyzeRoleRules(role.Name, "", true, role.Rules)
		if entry != nil {
			result.Summary.OverprivilegedCount++
			if hasWildcardVerb(role.Rules) {
				result.Summary.WildcardVerbCount++
				result.WildcardVerbs = append(result.WildcardVerbs, *entry)
			}
			if hasWildcardResource(role.Rules) {
				result.Summary.WildcardResourceCount++
				result.WildcardRes = append(result.WildcardRes, *entry)
			}
			result.Overprivileged = append(result.Overprivileged, *entry)
		}
	}

	// Audit namespaced Roles
	for _, role := range roles.Items {
		if isSystemRole(role.Name) {
			continue
		}
		entry := analyzeRoleRules(role.Name, role.Namespace, false, role.Rules)
		if entry != nil {
			result.Summary.OverprivilegedCount++
			result.Overprivileged = append(result.Overprivileged, *entry)
			if hasWildcardVerb(role.Rules) {
				result.Summary.WildcardVerbCount++
			}
			if hasWildcardResource(role.Rules) {
				result.Summary.WildcardResourceCount++
			}
		}
	}

	// Audit ClusterRoleBindings for cluster-admin
	for _, crb := range clusterBindings.Items {
		if crb.RoleRef.Name == "cluster-admin" && !isSystemBinding(crb.Name) {
			result.Summary.ClusterAdminBindings++
			for _, subj := range crb.Subjects {
				result.ExcessiveBindings = append(result.ExcessiveBindings, RBACAuditBinding{
					Name:        crb.Name,
					RoleName:    crb.RoleRef.Name,
					IsCluster:   true,
					SubjectKind: subj.Kind,
					SubjectName: subj.Name,
					Issue:       fmt.Sprintf("Subject %s/%s is bound to cluster-admin — use least-privilege roles instead", subj.Kind, subj.Name),
					Severity:    "critical",
				})
			}
		}
	}

	// Audit RoleBindings for excessive permissions
	for _, rb := range bindings.Items {
		if isSystemBinding(rb.Name) {
			continue
		}
		// Check if binding references a wildcard role
		for _, subj := range rb.Subjects {
			if rb.RoleRef.Kind == "ClusterRole" && rb.RoleRef.Name != "" {
				// Check if the referenced role has wildcard permissions
				for _, cr := range clusterRoles.Items {
					if cr.Name == rb.RoleRef.Name && !isSystemRole(cr.Name) && (hasWildcardVerb(cr.Rules) || hasWildcardResource(cr.Rules)) {
						result.ExcessiveBindings = append(result.ExcessiveBindings, RBACAuditBinding{
							Name:        rb.Name,
							Namespace:   rb.Namespace,
							RoleName:    rb.RoleRef.Name,
							IsCluster:   false,
							SubjectKind: subj.Kind,
							SubjectName: subj.Name,
							Issue:       fmt.Sprintf("Subject %s/%s bound to wildcard role %s in %s", subj.Kind, subj.Name, rb.RoleRef.Name, rb.Namespace),
							Severity:    "high",
						})
						break
					}
				}
			}
		}
	}

	// Sort
	sort.Slice(result.Overprivileged, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Overprivileged[i].Severity] < sevOrder[result.Overprivileged[j].Severity]
	})
	sort.Slice(result.ExcessiveBindings, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.ExcessiveBindings[i].Severity] < sevOrder[result.ExcessiveBindings[j].Severity]
	})
	if len(result.Overprivileged) > 30 {
		result.Overprivileged = result.Overprivileged[:30]
	}
	if len(result.ExcessiveBindings) > 30 {
		result.ExcessiveBindings = result.ExcessiveBindings[:30]
	}

	result.Summary.HealthScore = rbacAuditScore(result.Summary)
	result.Recommendations = rbacAuditRecs(&result)

	writeJSON(w, result)
}

// analyzeRoleRules checks for overprivilege in role rules.
func analyzeRoleRules(name, ns string, isCluster bool, rules []rbacv1.PolicyRule) *RBACAuditEntry {
	hasWildVerb := false
	hasWildRes := false
	var verbs, resources []string

	for _, rule := range rules {
		for _, v := range rule.Verbs {
			verbs = append(verbs, v)
			if v == "*" {
				hasWildVerb = true
			}
		}
		for _, r := range rule.Resources {
			resources = append(resources, r)
			if r == "*" {
				hasWildRes = true
			}
		}
		// Check for nonResourceURLs wildcard
		for _, u := range rule.NonResourceURLs {
			if u == "*" || u == "/*" {
				hasWildRes = true
			}
		}
	}

	if !hasWildVerb && !hasWildRes {
		return nil
	}

	severity := "medium"
	if hasWildVerb && hasWildRes {
		severity = "critical"
	} else if hasWildVerb || hasWildRes {
		severity = "high"
	}

	issues := []string{}
	if hasWildVerb {
		issues = append(issues, "wildcard verb '*'")
	}
	if hasWildRes {
		issues = append(issues, "wildcard resource '*'")
	}

	return &RBACAuditEntry{
		Name:      name,
		Namespace: ns,
		IsCluster: isCluster,
		Verbs:     verbs,
		Resources: resources,
		Issue:     fmt.Sprintf("Role has %s — grants excessive permissions", strings.Join(issues, " and ")),
		Severity:  severity,
	}
}

// hasWildcardVerb checks if any rule grants verb "*".
func hasWildcardVerb(rules []rbacv1.PolicyRule) bool {
	for _, rule := range rules {
		for _, v := range rule.Verbs {
			if v == "*" {
				return true
			}
		}
	}
	return false
}

// hasWildcardResource checks if any rule grants resource "*".
func hasWildcardResource(rules []rbacv1.PolicyRule) bool {
	for _, rule := range rules {
		for _, r := range rule.Resources {
			if r == "*" {
				return true
			}
		}
		for _, u := range rule.NonResourceURLs {
			if u == "*" || u == "/*" {
				return true
			}
		}
	}
	return false
}

// isSystemRole checks if a role name indicates a system-managed role.
func isSystemRole(name string) bool {
	prefixes := []string{"system:", "kubernetes.io/", "k8s.io/"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	// Common system roles
	systemNames := map[string]bool{
		"cluster-admin": true,
		"admin":         true,
		"edit":          true,
		"view":          true,
	}
	return systemNames[name]
}

// isSystemBinding checks if a binding is system-managed.
func isSystemBinding(name string) bool {
	return strings.HasPrefix(name, "system:") || strings.HasPrefix(name, "kubernetes.io/")
}

// rbacAuditScore computes a 0-100 health score (100 = least privileged).
func rbacAuditScore(s RBACAuditSummary) int {
	score := 100

	if s.ClusterAdminBindings > 0 {
		score -= min(40, s.ClusterAdminBindings*15)
	}

	if s.WildcardVerbCount > 0 {
		score -= min(25, s.WildcardVerbCount*5)
	}

	if s.WildcardResourceCount > 0 {
		score -= min(20, s.WildcardResourceCount*4)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// rbacAuditRecs generates actionable recommendations.
func rbacAuditRecs(r *RBACAuditResult) []string {
	var recs []string

	if r.Summary.ClusterAdminBindings > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d non-system subject(s) have cluster-admin binding — replace with least-privilege roles",
			r.Summary.ClusterAdminBindings,
		))
	}

	if r.Summary.WildcardVerbCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d role(s) use wildcard verb '*' — specify explicit verbs (get, list, watch, create, update, patch, delete)",
			r.Summary.WildcardVerbCount,
		))
	}

	if r.Summary.WildcardResourceCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d role(s) use wildcard resource '*' — specify explicit resource types (pods, services, deployments, etc.)",
			r.Summary.WildcardResourceCount,
		))
	}

	if r.Summary.OverprivilegedCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d total overprivileged role(s) — apply least-privilege principle and use audit tools (e.g., audit2rbac) to right-size",
			r.Summary.OverprivilegedCount,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "RBAC configuration follows least-privilege — no overprivileged roles or excessive bindings detected")
	}

	return recs
}
