package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBACEffectiveResult is the RBAC effective permissions and escalation analysis.
type RBACEffectiveResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RBACEffectiveSummary `json:"summary"`
	Subjects        []RBACSubjectEntry   `json:"subjects"`
	ClusterRoles    []RBACRoleEntry      `json:"clusterRoles"`
	PrivilegedUsers []RBACPrivileged     `json:"privilegedUsers"`
	EscalationRisks []RBACEscalation     `json:"escalationRisks"`
	Issues          []RBACEffectiveIssue `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

// RBACEffectiveSummary aggregates cluster-wide RBAC posture.
type RBACEffectiveSummary struct {
	TotalSubjects            int `json:"totalSubjects"`
	TotalClusterRoleBindings int `json:"totalClusterRoleBindings"`
	TotalRoleBindings        int `json:"totalRoleBindings"`
	ClusterAdmins            int `json:"clusterAdmins"`      // bound to cluster-admin
	PrivilegedSubjects       int `json:"privilegedSubjects"` // with wildcard or dangerous verbs
	EscalationPaths          int `json:"escalationPaths"`    // can escalate privileges
	WithWildcards            int `json:"withWildcards"`      // uses * in resources/verbs
	SecretReaders            int `json:"secretReaders"`      // can read secrets cluster-wide
	ExecAccess               int `json:"execAccess"`         // can exec into pods
	NodeAccess               int `json:"nodeAccess"`         // can access nodes
	SecurityScore            int `json:"securityScore"`      // 0-100
}

// RBACSubjectEntry describes one subject's effective permissions.
type RBACSubjectEntry struct {
	Name           string          `json:"name"`
	Kind           string          `json:"kind"` // User / Group / ServiceAccount
	Namespace      string          `json:"namespace,omitempty"`
	Roles          []RBACRoleEntry `json:"roles"`
	IsClusterAdmin bool            `json:"isClusterAdmin"`
	HasWildcard    bool            `json:"hasWildcard"`
	CanReadSecrets bool            `json:"canReadSecrets"`
	CanExec        bool            `json:"canExec"`
	CanEscalate    bool            `json:"canEscalate"`
	RiskLevel      string          `json:"riskLevel"` // critical / high / medium / low
}

// RBACRoleEntry describes a role/binding.
type RBACRoleEntry struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace,omitempty"`
	IsCluster   bool     `json:"isCluster"`
	Verbs       []string `json:"verbs"`
	Resources   []string `json:"resources"`
	HasWildcard bool     `json:"hasWildcard"`
}

// RBACPrivileged highlights a subject with cluster-admin.
type RBACPrivileged struct {
	Subject   string `json:"subject"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Reason    string `json:"reason"`
}

// RBACEscalation describes a privilege escalation path.
type RBACEscalation struct {
	Subject   string `json:"subject"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Path      string `json:"path"`
	Severity  string `json:"severity"`
}

// RBACEffectiveIssue is a detected problem.
type RBACEffectiveIssue struct {
	Subject  string `json:"subject"`
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

// handleRBACEffective analyzes effective RBAC permissions and escalation risks.
// GET /api/security/rbac-effective
func (s *Server) handleRBACEffective(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	clusterRoles, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	clusterRoleBindings, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	roles, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	// Build role lookup: name → rules
	clusterRoleMap := make(map[string][]rbacv1.PolicyRule)
	for _, cr := range clusterRoles.Items {
		clusterRoleMap[cr.Name] = cr.Rules
	}
	roleMap := make(map[string]map[string][]rbacv1.PolicyRule) // ns → name → rules
	for _, role := range roles.Items {
		if roleMap[role.Namespace] == nil {
			roleMap[role.Namespace] = make(map[string][]rbacv1.PolicyRule)
		}
		roleMap[role.Namespace][role.Name] = role.Rules
	}

	// Aggregate subjects: key → effective rules
	type subjectKey struct {
		Name      string
		Kind      string
		Namespace string
	}
	subjectRules := make(map[subjectKey][]rbacv1.PolicyRule)

	// From ClusterRoleBindings
	for _, crb := range clusterRoleBindings.Items {
		rules := clusterRoleMap[crb.RoleRef.Name]
		for _, subj := range crb.Subjects {
			key := subjectKey{Name: subj.Name, Kind: subj.Kind, Namespace: subj.Namespace}
			subjectRules[key] = append(subjectRules[key], rules...)
		}
	}

	// From RoleBindings
	for _, rb := range roleBindings.Items {
		var rules []rbacv1.PolicyRule
		if rb.RoleRef.Kind == "ClusterRole" {
			rules = clusterRoleMap[rb.RoleRef.Name]
		} else {
			if nsMap, ok := roleMap[rb.Namespace]; ok {
				rules = nsMap[rb.RoleRef.Name]
			}
		}
		for _, subj := range rb.Subjects {
			key := subjectKey{Name: subj.Name, Kind: subj.Kind, Namespace: subj.Namespace}
			subjectRules[key] = append(subjectRules[key], rules...)
		}
	}

	result := RBACEffectiveResult{ScannedAt: time.Now()}
	result.Summary.TotalClusterRoleBindings = len(clusterRoleBindings.Items)
	result.Summary.TotalRoleBindings = len(roleBindings.Items)

	for key, rules := range subjectRules {
		entry := RBACSubjectEntry{
			Name:      key.Name,
			Kind:      key.Kind,
			Namespace: key.Namespace,
		}

		isClusterAdmin := false
		hasWildcard := false
		canReadSecrets := false
		canExec := false
		canEscalate := false

		var entryRoles []RBACRoleEntry

		for _, rule := range rules {
			roleEntry := RBACRoleEntry{
				Verbs:     rule.Verbs,
				Resources: rule.Resources,
			}

			// Check for wildcards
			for _, v := range rule.Verbs {
				if v == "*" {
					hasWildcard = true
					roleEntry.HasWildcard = true
				}
			}
			for _, res := range rule.Resources {
				if res == "*" {
					hasWildcard = true
					roleEntry.HasWildcard = true
				}
			}

			// Cluster-admin detection (very broad permissions)
			if hasVerb(rule.Verbs, "*") && hasResource(rule.Resources, "*") {
				isClusterAdmin = true
			}

			// Secret access
			if hasResource(rule.Resources, "secrets") || hasResource(rule.Resources, "*") {
				if hasVerb(rule.Verbs, "get") || hasVerb(rule.Verbs, "list") || hasVerb(rule.Verbs, "watch") || hasVerb(rule.Verbs, "*") {
					canReadSecrets = true
				}
			}

			// Exec access
			if hasResource(rule.Resources, "pods/exec") || hasResource(rule.Resources, "pods") || hasResource(rule.Resources, "*") {
				if hasVerb(rule.Verbs, "create") || hasVerb(rule.Verbs, "*") {
					canExec = true
				}
			}

			// Escalation: can create/update RBAC
			if hasResource(rule.Resources, "clusterroles") || hasResource(rule.Resources, "roles") || hasResource(rule.Resources, "*") {
				if hasVerb(rule.Verbs, "create") || hasVerb(rule.Verbs, "update") || hasVerb(rule.Verbs, "patch") || hasVerb(rule.Verbs, "*") {
					canEscalate = true
				}
			}

			entryRoles = append(entryRoles, roleEntry)
		}

		entry.Roles = entryRoles
		entry.IsClusterAdmin = isClusterAdmin
		entry.HasWildcard = hasWildcard
		entry.CanReadSecrets = canReadSecrets
		entry.CanExec = canExec
		entry.CanEscalate = canEscalate
		entry.RiskLevel = assessRBACSubjectRisk(entry)

		result.Summary.TotalSubjects++

		if isClusterAdmin {
			result.Summary.ClusterAdmins++
			result.PrivilegedUsers = append(result.PrivilegedUsers, RBACPrivileged{
				Subject: key.Name, Kind: key.Kind, Namespace: key.Namespace,
				Reason: "Bound to cluster-admin or has wildcard permissions on all resources",
			})
		}
		if hasWildcard {
			result.Summary.WithWildcards++
		}
		if canReadSecrets {
			result.Summary.SecretReaders++
		}
		if canExec {
			result.Summary.ExecAccess++
		}
		if canEscalate {
			result.Summary.EscalationPaths++
			result.EscalationRisks = append(result.EscalationRisks, RBACEscalation{
				Subject: key.Name, Kind: key.Kind, Namespace: key.Namespace,
				Path:     "Can create/modify RBAC roles — self-escalation possible",
				Severity: "critical",
			})
		}

		// Issues for high-risk subjects
		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			if isClusterAdmin && key.Kind != "ServiceAccount" {
				result.Issues = append(result.Issues, RBACEffectiveIssue{
					Subject: key.Name, Severity: "critical", Type: "cluster-admin",
					Message: fmt.Sprintf("%s %s has cluster-admin equivalent permissions", key.Kind, key.Name),
				})
			}
			if canEscalate {
				result.Issues = append(result.Issues, RBACEffectiveIssue{
					Subject: key.Name, Severity: "critical", Type: "escalation",
					Message: fmt.Sprintf("%s %s can modify RBAC — privilege escalation risk", key.Kind, key.Name),
				})
			}
			if hasWildcard {
				result.Issues = append(result.Issues, RBACEffectiveIssue{
					Subject: key.Name, Severity: "warning", Type: "wildcard",
					Message: fmt.Sprintf("%s %s has wildcard (*) permissions", key.Kind, key.Name),
				})
			}
		}

		result.Subjects = append(result.Subjects, entry)
	}

	// Sort subjects by risk
	sort.Slice(result.Subjects, func(i, j int) bool {
		return rbacSubjectRiskRank(result.Subjects[i].RiskLevel) < rbacSubjectRiskRank(result.Subjects[j].RiskLevel)
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return rbacIssueRank(result.Issues[i].Severity) < rbacIssueRank(result.Issues[j].Severity)
	})

	result.Summary.SecurityScore = calculateRBACEffectiveScore(result.Summary)
	result.Recommendations = generateRBACEffectiveRecs(result.Summary)

	writeJSON(w, result)
}

// assessRBACSubjectRisk determines risk level for a subject.
func assessRBACSubjectRisk(entry RBACSubjectEntry) string {
	if entry.IsClusterAdmin {
		return "critical"
	}
	risk := 0
	if entry.HasWildcard {
		risk += 20
	}
	if entry.CanEscalate {
		risk += 25
	}
	if entry.CanReadSecrets {
		risk += 10
	}
	if entry.CanExec {
		risk += 10
	}

	switch {
	case risk >= 25:
		return "critical"
	case risk >= 15:
		return "high"
	case risk >= 5:
		return "medium"
	default:
		return "low"
	}
}

// calculateRBACEffectiveScore computes 0-100.
func calculateRBACEffectiveScore(s RBACEffectiveSummary) int {
	if s.TotalSubjects == 0 {
		return 100
	}
	score := 100
	score -= s.ClusterAdmins * 8
	score -= s.WithWildcards * 5
	score -= s.EscalationPaths * 10
	score -= s.SecretReaders * 2
	score -= s.ExecAccess * 2
	if score < 0 {
		score = 0
	}
	return score
}

// generateRBACEffectiveRecs produces actionable advice.
func generateRBACEffectiveRecs(s RBACEffectiveSummary) []string {
	var recs []string

	if s.ClusterAdmins > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) have cluster-admin level permissions — apply least privilege principle", s.ClusterAdmins))
	}
	if s.EscalationPaths > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) can modify RBAC — restrict role/clusterrole creation to break escalation chains", s.EscalationPaths))
	}
	if s.WithWildcards > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) use wildcard (*) permissions — replace with explicit resource/verb lists", s.WithWildcards))
	}
	if s.SecretReaders > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) can read Secrets — audit secret access and restrict to specific namespaces", s.SecretReaders))
	}
	if s.ExecAccess > 0 {
		recs = append(recs, fmt.Sprintf("%d subject(s) can exec into pods — restrict for production environments", s.ExecAccess))
	}
	if s.SecurityScore < 50 {
		recs = append(recs, fmt.Sprintf("RBAC security score is %d/100 — review and tighten permissions", s.SecurityScore))
	}

	return recs
}

func hasVerb(verbs []string, target string) bool {
	for _, v := range verbs {
		if v == target {
			return true
		}
	}
	return false
}

func hasResource(resources []string, target string) bool {
	for _, r := range resources {
		if r == target {
			return true
		}
	}
	return false
}

func rbacSubjectRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func rbacIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
