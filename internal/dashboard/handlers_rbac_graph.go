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

// RBACPermGraphResult is the RBAC permission graph & escalation path analyzer.
type RBACPermGraphResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RBACPermGraphSummary `json:"summary"`
	Subjects        []RBACPermSubject    `json:"subjects"`
	CriticalRoles   []RBACPermRole       `json:"criticalRoles"`
	EscalationPaths []EscalationPath     `json:"escalationPaths,omitempty"`
	Overprivileged  []OverprivEntry      `json:"overprivileged,omitempty"`
	WildcardUsers   []RBACPermSubject    `json:"wildcardUsers,omitempty"`
	Recommendations []string             `json:"recommendations"`
	RiskScore       int                  `json:"riskScore"`
}

// RBACPermGraphSummary aggregates RBAC graph statistics.
type RBACPermGraphSummary struct {
	TotalSubjects      int `json:"totalSubjects"`
	TotalClusterRoles  int `json:"totalClusterRoles"`
	TotalRoleBindings  int `json:"totalRoleBindings"`
	TotalCRBs          int `json:"totalClusterRoleBindings"`
	ClusterAdmins      int `json:"clusterAdmins"`
	PrivilegedSubjects int `json:"privilegedSubjects"`
	EscalationPaths    int `json:"escalationPaths"`
	WildcardPerms      int `json:"wildcardPermissions"`
	UnusedBindings     int `json:"unusedBindings"`
}

// RBACPermSubject describes one subject in the RBAC graph.
type RBACPermSubject struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // User, Group, ServiceAccount
	Namespace    string   `json:"namespace,omitempty"`
	Roles        []string `json:"roles"`
	IsAdmin      bool     `json:"isAdmin"`
	IsPrivileged bool     `json:"isPrivileged"`
	HasWildcard  bool     `json:"hasWildcard"`
	BindingCount int      `json:"bindingCount"`
}

// RBACPermRole describes a role with dangerous permissions.
type RBACPermRole struct {
	Name      string   `json:"name"`
	IsCluster bool     `json:"isClusterRole"`
	Namespace string   `json:"namespace,omitempty"`
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	Danger    string   `json:"danger"`   // cluster-admin, privileged-escalation, secret-access, node-access
	Subjects  []string `json:"subjects"` // who has this role
}

// EscalationPath describes a path from low-privilege to high-privilege.
type EscalationPath struct {
	Subject   string `json:"subject"`
	Path      string `json:"path"` // human-readable chain
	StartRole string `json:"startRole"`
	EndRole   string `json:"endRole"`
	Severity  string `json:"severity"`
	Method    string `json:"method"` // impersonate, bind, escalate, pods/exec
}

// OverprivEntry identifies an overprivileged subject.
type OverprivEntry struct {
	Subject  string `json:"subject"`
	Role     string `json:"role"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleRBACGraph builds a cluster-wide RBAC permission graph and finds
// escalation paths.
// GET /api/security/rbac-graph
func (s *Server) handleRBACGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RBACPermGraphResult{ScannedAt: time.Now()}

	// 1. Collect all ClusterRoles and find dangerous ones
	dangerousRoles := map[string]*RBACPermRole{} // name → entry
	clusterRoles, err := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalClusterRoles = len(clusterRoles.Items)
		for _, cr := range clusterRoles.Items {
			entry := classifyRoleDanger(cr.Name, cr.Rules, true, "")
			if entry != nil {
				dangerousRoles[cr.Name] = entry
			}
		}
	}

	// 2. Collect Roles (namespaced)
	roles, err := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, role := range roles.Items {
			entry := classifyRoleDanger(role.Name, role.Rules, false, role.Namespace)
			if entry != nil {
				key := fmt.Sprintf("%s/%s", role.Namespace, role.Name)
				dangerousRoles[key] = entry
			}
		}
	}

	// 3. Collect ClusterRoleBindings
	subjectRoles := map[string]*RBACPermSubject{} // "kind/name/ns" → entry
	crbSubjectMap := map[string][]string{}        // subjectKey → role names

	crbs, err := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalCRBs = len(crbs.Items)
		for _, crb := range crbs.Items {
			roleName := crb.RoleRef.Name
			if crb.RoleRef.Kind == "ClusterRole" {
				for _, subj := range crb.Subjects {
					key := rbacPermSubjectKey(subj)
					if subjectRoles[key] == nil {
						subjectRoles[key] = &RBACPermSubject{
							Name: subj.Name, Kind: subj.Kind, Namespace: subj.Namespace,
						}
					}
					subjectRoles[key].Roles = append(subjectRoles[key].Roles, roleName)
					subjectRoles[key].BindingCount++
					crbSubjectMap[key] = append(crbSubjectMap[key], roleName)

					if roleName == "cluster-admin" {
						subjectRoles[key].IsAdmin = true
						result.Summary.ClusterAdmins++
					}
					if dr, ok := dangerousRoles[roleName]; ok {
						subjectRoles[key].IsPrivileged = true
						dr.Subjects = append(dr.Subjects, subj.Name)
					}
				}
			}
		}
	}

	// 4. Collect RoleBindings
	rbs, err := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalRoleBindings = len(rbs.Items)
		for _, rb := range rbs.Items {
			roleKey := rb.RoleRef.Name
			if rb.RoleRef.Kind == "ClusterRole" {
				roleKey = rb.RoleRef.Name
			} else {
				roleKey = fmt.Sprintf("%s/%s", rb.Namespace, rb.RoleRef.Name)
			}

			for _, subj := range rb.Subjects {
				key := rbacPermSubjectKey(subj)
				if subjectRoles[key] == nil {
					subjectRoles[key] = &RBACPermSubject{
						Name: subj.Name, Kind: subj.Kind, Namespace: subj.Namespace,
					}
				}
				subjectRoles[key].Roles = append(subjectRoles[key].Roles, roleKey)
				subjectRoles[key].BindingCount++

				if dr, ok := dangerousRoles[roleKey]; ok {
					subjectRoles[key].IsPrivileged = true
					dr.Subjects = append(dr.Subjects, subj.Name)
				}
			}
		}
	}

	// 5. Detect wildcard permissions
	for _, subj := range subjectRoles {
		for _, roleName := range subj.Roles {
			if dr, ok := dangerousRoles[roleName]; ok {
				for _, res := range dr.Resources {
					if res == "*" || res == "" {
						subj.HasWildcard = true
					}
				}
				for _, verb := range dr.Verbs {
					if verb == "*" {
						subj.HasWildcard = true
					}
				}
			}
		}
		if subj.HasWildcard {
			result.Summary.WildcardPerms++
		}
		if subj.IsPrivileged {
			result.Summary.PrivilegedSubjects++
		}
	}

	// 6. Build subject list
	var subjects []RBACPermSubject
	var wildcardUsers []RBACPermSubject
	for _, subj := range subjectRoles {
		entry := *subj
		// Deduplicate roles
		entry.Roles = dedupStrings(entry.Roles)
		subjects = append(subjects, entry)
		if subj.HasWildcard || subj.IsAdmin {
			wildcardUsers = append(wildcardUsers, entry)
		}
	}
	sort.Slice(subjects, func(i, j int) bool {
		if subjects[i].IsAdmin != subjects[j].IsAdmin {
			return subjects[i].IsAdmin
		}
		return subjects[i].BindingCount > subjects[j].BindingCount
	})
	result.Subjects = subjects
	result.WildcardUsers = wildcardUsers
	result.Summary.TotalSubjects = len(subjects)

	// 7. Build critical roles list
	var criticalRoles []RBACPermRole
	for _, dr := range dangerousRoles {
		if len(dr.Subjects) > 0 {
			dr.Subjects = dedupStrings(dr.Subjects)
			criticalRoles = append(criticalRoles, *dr)
		}
	}
	sort.Slice(criticalRoles, func(i, j int) bool {
		return dangerOrder(criticalRoles[i].Danger) < dangerOrder(criticalRoles[j].Danger)
	})
	result.CriticalRoles = criticalRoles

	// 8. Find escalation paths
	result.EscalationPaths = findEscalationPaths(subjectRoles, dangerousRoles)
	result.Summary.EscalationPaths = len(result.EscalationPaths)

	// 9. Find overprivileged subjects
	result.Overprivileged = findOverprivileged(subjectRoles, dangerousRoles)

	// 10. Risk score
	score := 100
	score -= result.Summary.ClusterAdmins * 5
	score -= result.Summary.WildcardPerms * 8
	score -= len(result.EscalationPaths) * 10
	score -= len(result.Overprivileged) * 3
	if score < 0 {
		score = 0
	}
	result.RiskScore = score

	// 11. Recommendations
	result.Recommendations = generateRBACGraphRecs(result)

	writeJSON(w, result)
}

// classifyRoleDanger checks if a role has dangerous permissions.
func classifyRoleDanger(name string, rules []rbacv1.PolicyRule, isCluster bool, namespace string) *RBACPermRole {
	// Skip system roles
	if strings.HasPrefix(name, "system:") {
		return nil
	}

	entry := &RBACPermRole{
		Name:      name,
		IsCluster: isCluster,
		Namespace: namespace,
	}

	dangerLevel := ""
	for _, rule := range rules {
		entry.Verbs = append(entry.Verbs, ruleVerbs(rule)...)
		entry.Resources = append(entry.Resources, rule.Resources...)

		// Check for cluster-admin equivalent
		if containsAny(rule.Resources, "*") && containsAny(rule.Verbs, "*") {
			dangerLevel = "cluster-admin"
		}
		// Check for privilege escalation verb
		if containsAny(rule.Verbs, "escalate", "bind", "impersonate") {
			if dangerLevel == "" {
				dangerLevel = "privileged-escalation"
			}
		}
		// Check for secret access
		if containsAny(rule.Resources, "secrets") && !containsAny(rule.Verbs, "get", "list", "watch", "*") {
			// write access to secrets
			if dangerLevel == "" {
				dangerLevel = "secret-access"
			}
		}
		// Check for pods/exec or pods/portforward
		if containsAny(rule.Resources, "pods/exec", "pods", "pods/portforward") {
			if containsAny(rule.Verbs, "create", "*") {
				if dangerLevel == "" {
					dangerLevel = "pods-exec"
				}
			}
		}
		// Check for node access
		if containsAny(rule.Resources, "nodes") && containsAny(rule.Verbs, "*", "delete", "update", "patch") {
			if dangerLevel == "" {
				dangerLevel = "node-access"
			}
		}
	}

	if dangerLevel == "" {
		return nil
	}
	entry.Danger = dangerLevel
	entry.Verbs = dedupStrings(entry.Verbs)
	entry.Resources = dedupStrings(entry.Resources)
	return entry
}

// findEscalationPaths identifies subjects that can escalate privileges.
func findEscalationPaths(subjects map[string]*RBACPermSubject, dangerousRoles map[string]*RBACPermRole) []EscalationPath {
	var paths []EscalationPath

	for _, subj := range subjects {
		if subj.IsAdmin {
			continue // Already admin, no escalation needed
		}

		for _, roleName := range subj.Roles {
			dr, ok := dangerousRoles[roleName]
			if !ok {
				continue
			}

			switch dr.Danger {
			case "privileged-escalation":
				// Subject can use escalate/bind verb to gain more privileges
				paths = append(paths, EscalationPath{
					Subject:   subj.Name,
					Path:      fmt.Sprintf("%s → (%s: escalate/bind) → cluster-admin", subj.Name, roleName),
					StartRole: roleName,
					EndRole:   "cluster-admin (potential)",
					Severity:  "critical",
					Method:    "escalate/bind verb",
				})
			case "pods-exec":
				// Subject can exec into pods and potentially steal SA tokens
				paths = append(paths, EscalationPath{
					Subject:   subj.Name,
					Path:      fmt.Sprintf("%s → (%s: pods/exec) → pod SA token → cluster resources", subj.Name, roleName),
					StartRole: roleName,
					EndRole:   "pod service account",
					Severity:  "high",
					Method:    "pods/exec to steal SA token",
				})
			case "secret-access":
				// Subject can read secrets including SA tokens
				paths = append(paths, EscalationPath{
					Subject:   subj.Name,
					Path:      fmt.Sprintf("%s → (%s: secrets) → SA tokens → privilege escalation", subj.Name, roleName),
					StartRole: roleName,
					EndRole:   "any SA via token",
					Severity:  "high",
					Method:    "secret read to steal SA tokens",
				})
			}
		}
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Severity > paths[j].Severity
	})
	if len(paths) > 50 {
		paths = paths[:50]
	}
	return paths
}

// findOverprivileged identifies subjects with more permissions than likely needed.
func findOverprivileged(subjects map[string]*RBACPermSubject, dangerousRoles map[string]*RBACPermRole) []OverprivEntry {
	var entries []OverprivEntry

	for _, subj := range subjects {
		if subj.IsAdmin && subj.Kind == "ServiceAccount" {
			entries = append(entries, OverprivEntry{
				Subject:  fmt.Sprintf("%s/%s", subj.Namespace, subj.Name),
				Role:     "cluster-admin",
				Issue:    "ServiceAccount has cluster-admin — use least-privilege role instead",
				Severity: "critical",
			})
		}
		if subj.HasWildcard {
			entries = append(entries, OverprivEntry{
				Subject:  subj.Name,
				Role:     "wildcard",
				Issue:    "Subject has wildcard (*) permissions — restrict to specific resources and verbs",
				Severity: "high",
			})
		}
	}

	if len(entries) > 50 {
		entries = entries[:50]
	}
	return entries
}

// generateRBACGraphRecs produces recommendations.
func generateRBACGraphRecs(result RBACPermGraphResult) []string {
	var recs []string

	if result.Summary.ClusterAdmins > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) have cluster-admin — audit and replace with least-privilege roles", result.Summary.ClusterAdmins))
	}

	if result.Summary.WildcardPerms > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) have wildcard (*) permissions — replace with specific resource/verb grants", result.Summary.WildcardPerms))
	}

	if len(result.EscalationPaths) > 0 {
		recs = append(recs, fmt.Sprintf("%d escalation path(s) detected — remove escalate/bind verbs and restrict pods/exec access", len(result.EscalationPaths)))
	}

	if len(result.CriticalRoles) > 0 {
		recs = append(recs, fmt.Sprintf("%d role(s) have dangerous permissions — review and tighten rules", len(result.CriticalRoles)))
	}

	if len(result.Overprivileged) > 0 {
		recs = append(recs, fmt.Sprintf("%d overprivileged subject(s) — apply least-privilege principle", len(result.Overprivileged)))
	}

	if result.RiskScore < 50 {
		recs = append(recs, fmt.Sprintf("RBAC risk score is %d/100 — significant privilege escalation risk", result.RiskScore))
	}

	if len(recs) == 0 {
		recs = append(recs, "RBAC configuration is well-controlled — no escalation paths or overprivileged subjects detected")
	}

	return recs
}

// subjectKey creates a unique key for a subject.
func rbacPermSubjectKey(subj rbacv1.Subject) string {
	return fmt.Sprintf("%s/%s/%s", subj.Kind, subj.Namespace, subj.Name)
}

// ruleVerbs extracts verbs from a policy rule.
func ruleVerbs(rule rbacv1.PolicyRule) []string {
	verbs := make([]string, len(rule.Verbs))
	copy(verbs, rule.Verbs)
	return verbs
}

// containsAny checks if slice contains any of the values.
func containsAny(slice []string, values ...string) bool {
	for _, s := range slice {
		for _, v := range values {
			if s == v {
				return true
			}
		}
	}
	return false
}

// dedupStrings removes duplicates from a string slice.
func dedupStrings(s []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// dangerOrder returns sort order for danger levels.
func dangerOrder(d string) int {
	switch d {
	case "cluster-admin":
		return 0
	case "privileged-escalation":
		return 1
	case "secret-access":
		return 2
	case "pods-exec":
		return 3
	case "node-access":
		return 4
	default:
		return 5
	}
}
