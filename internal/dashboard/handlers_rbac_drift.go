package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBACDriftResult detects changes and anomalies in RBAC configuration:
// over-permissive roles, wildcard permissions, stale bindings, and
// security-sensitive cluster role bindings that need review.
type RBACDriftResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         RBACDriftSummary `json:"summary"`
	OverPermissive  []RBACDriftEntry `json:"overPermissive"`
	WildcardPerms   []RBACDriftEntry `json:"wildcardPerms"`
	StaleBindings   []RBACDriftEntry `json:"staleBindings"`
	ByNS            []RBACDriftNS    `json:"byNamespace"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type RBACDriftSummary struct {
	TotalRoles     int `json:"totalRoles"`
	TotalBindings  int `json:"totalBindings"`
	OverPermissive int `json:"overPermissiveCount"`
	WildcardPerms  int `json:"wildcardPermCount"`
	StaleBindings  int `json:"staleBindingCount"`
	ClusterAdmins  int `json:"clusterAdminBindings"`
	SystemBindings int `json:"systemBindings"`
}

type RBACDriftEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // Role, ClusterRole, RoleBinding, ClusterRoleBinding
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type RBACDriftNS struct {
	Namespace string `json:"namespace"`
	Roles     int    `json:"roles"`
	Bindings  int    `json:"bindings"`
	Issues    int    `json:"issues"`
}

// handleRBACDrift handles GET /api/security/rbac-drift
func (s *Server) handleRBACDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RBACDriftResult{ScannedAt: time.Now()}

	clusterRoles, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	clusterRoleBindings, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	roles, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*RBACDriftNS)
	var overPermissive, wildcardPerms, staleBindings []RBACDriftEntry

	// Check ClusterRoles for wildcard permissions
	for _, cr := range clusterRoles.Items {
		if cr.Name == "cluster-admin" || startsWith(cr.Name, "system:") {
			result.Summary.SystemBindings++
			continue
		}
		result.Summary.TotalRoles++

		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					for _, res := range rule.Resources {
						if res == "*" || res == "secrets" || res == "pods/exec" {
							wildcardPerms = append(wildcardPerms, RBACDriftEntry{
								Name: cr.Name, Kind: "ClusterRole",
								Issue:    "Wildcard permission on sensitive resource",
								Severity: "critical",
								Detail:   fmt.Sprintf("verb=* on resource=%s", res),
							})
							result.Summary.WildcardPerms++
						}
					}
				}
			}
			// Check for overly broad permissions
			if len(rule.Resources) == 1 && rule.Resources[0] == "*" && len(rule.Verbs) == 1 && rule.Verbs[0] == "*" {
				overPermissive = append(overPermissive, RBACDriftEntry{
					Name: cr.Name, Kind: "ClusterRole",
					Issue:    "Full cluster access (*/*)",
					Severity: "critical",
					Detail:   "resources=*, verbs=* grants unrestricted access",
				})
				result.Summary.OverPermissive++
			}
		}
	}

	// Check ClusterRoleBindings for cluster-admin usage
	for _, crb := range clusterRoleBindings.Items {
		result.Summary.TotalBindings++
		if crb.RoleRef.Name == "cluster-admin" {
			result.Summary.ClusterAdmins++
			overPermissive = append(overPermissive, RBACDriftEntry{
				Name: crb.Name, Kind: "ClusterRoleBinding",
				Issue:    "cluster-admin binding",
				Severity: "high",
				Detail:   fmt.Sprintf("Subject: %s/%s", crb.Subjects[0].Kind, crb.Subjects[0].Name),
			})
		}
	}

	// Check namespace-scoped Roles and Bindings
	for _, role := range roles.Items {
		if isSystemNamespace(role.Namespace) {
			continue
		}
		result.Summary.TotalRoles++
		if _, ok := nsMap[role.Namespace]; !ok {
			nsMap[role.Namespace] = &RBACDriftNS{Namespace: role.Namespace}
		}
		nsMap[role.Namespace].Roles++

		for _, rule := range role.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					for _, res := range rule.Resources {
						if res == "secrets" || res == "*" {
							overPermissive = append(overPermissive, RBACDriftEntry{
								Name: role.Name, Namespace: role.Namespace, Kind: "Role",
								Issue:    "Overly permissive Role",
								Severity: "high",
								Detail:   fmt.Sprintf("verb=* resource=%s", res),
							})
							nsMap[role.Namespace].Issues++
							result.Summary.OverPermissive++
						}
					}
				}
			}
		}
	}

	for _, rb := range roleBindings.Items {
		if isSystemNamespace(rb.Namespace) {
			continue
		}
		result.Summary.TotalBindings++
		if _, ok := nsMap[rb.Namespace]; !ok {
			nsMap[rb.Namespace] = &RBACDriftNS{Namespace: rb.Namespace}
		}
		nsMap[rb.Namespace].Bindings++
	}

	// NS stats
	for _, ns := range nsMap {
		result.ByNS = append(result.ByNS, *ns)
	}
	sort.Slice(result.ByNS, func(i, j int) bool {
		return result.ByNS[i].Issues > result.ByNS[j].Issues
	})

	// Sort findings
	sortAll := func(entries []RBACDriftEntry) {
		sort.Slice(entries, func(i, j int) bool {
			sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2}
			return sevOrder[entries[i].Severity] < sevOrder[entries[j].Severity]
		})
	}
	sortAll(overPermissive)
	sortAll(wildcardPerms)
	sortAll(staleBindings)

	result.OverPermissive = overPermissive
	result.WildcardPerms = wildcardPerms
	result.StaleBindings = staleBindings

	// Score
	totalChecks := result.Summary.TotalRoles + result.Summary.TotalBindings
	if totalChecks > 0 {
		issues := result.Summary.OverPermissive + result.Summary.WildcardPerms
		result.HealthScore = (totalChecks - issues) * 100 / totalChecks
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

	result.Recommendations = buildRBACDriftRecs(&result)
	writeJSON(w, result)
}

func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func buildRBACDriftRecs(r *RBACDriftResult) []string {
	recs := []string{}
	if r.Summary.ClusterAdmins > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 cluster-admin 绑定，建议最小化权限", r.Summary.ClusterAdmins))
	}
	if r.Summary.WildcardPerms > 0 {
		recs = append(recs, fmt.Sprintf("%d 个通配符权限 (*/*)，存在安全风险", r.Summary.WildcardPerms))
	}
	if r.Summary.OverPermissive > 0 {
		recs = append(recs, fmt.Sprintf("%d 个过度宽松的 Role/ClusterRole", r.Summary.OverPermissive))
	}
	if len(recs) == 0 {
		recs = append(recs, "RBAC 配置合理，未发现过度权限")
	}
	return recs
}

var _ rbacv1.ClusterRole
