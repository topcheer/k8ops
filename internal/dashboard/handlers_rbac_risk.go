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

// RBACRiskLevel rates the overall risk of a subject's permissions.
type RBACRiskLevel string

const (
	RBACRiskCritical RBACRiskLevel = "critical" // cluster-admin or equivalent
	RBACRiskHigh     RBACRiskLevel = "high"     // cluster-wide write, privilege escalation
	RBACRiskMedium   RBACRiskLevel = "medium"   // namespace-wide write, sensitive resources
	RBACRiskLow      RBACRiskLevel = "low"      // read-only, limited scope
)

// RBACSubjectKind describes the type of RBAC subject.
type RBACSubjectKind string

const (
	SubjectKindUser           RBACSubjectKind = "User"
	SubjectKindGroup          RBACSubjectKind = "Group"
	SubjectKindServiceAccount RBACSubjectKind = "ServiceAccount"
)

// SubjectRisk summarizes the effective permissions and risk of a single subject.
type SubjectRisk struct {
	Kind                RBACSubjectKind  `json:"kind"`
	Name                string           `json:"name"`
	Namespace           string           `json:"namespace,omitempty"`
	DisplayName         string           `json:"displayName"`
	RiskLevel           RBACRiskLevel    `json:"riskLevel"`
	RiskScore           int              `json:"riskScore"`
	IsClusterScoped     bool             `json:"isClusterScoped"`
	Bindings            []BindingSummary `json:"bindings"`
	EffectiveRules      []EffectiveRule  `json:"effectiveRules"`
	PrivilegeEscalation bool             `json:"privilegeEscalation"`
	WildcardAccess      bool             `json:"wildcardAccess"`
	Issues              []string         `json:"issues,omitempty"`
}

// BindingSummary describes a RoleBinding/ClusterRoleBinding.
type BindingSummary struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind"`
	RoleRefKind string `json:"roleRefKind"`
	RoleRefName string `json:"roleRefName"`
}

// EffectiveRule is a flattened permission rule.
type EffectiveRule struct {
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	APIGroups []string `json:"apiGroups"`
	Scope     string   `json:"scope"`
}

// RBACRiskResult is the full scan output.
type RBACRiskResult struct {
	ScannedAt time.Time     `json:"scannedAt"`
	Summary   RBACSummary   `json:"summary"`
	Subjects  []SubjectRisk `json:"subjects"`
}

// RBACSummary aggregates risk statistics.
type RBACSummary struct {
	TotalSubjects       int            `json:"totalSubjects"`
	ByRiskLevel         map[string]int `json:"byRiskLevel"`
	ClusterScoped       int            `json:"clusterScoped"`
	PrivilegeEscalation int            `json:"privilegeEscalation"`
	WildcardAccess      int            `json:"wildcardAccess"`
	TotalBindings       int            `json:"totalBindings"`
	UnusedBindings      int            `json:"unusedBindings"`
}

// handleRBACRiskScan performs a comprehensive RBAC permission risk analysis.
// GET /api/security/rbac-risk
func (s *Server) handleRBACRiskScan(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	crbList, err := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	rbList, err := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	crList, err := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	roleList, err := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	saList, err := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build lookup maps
	clusterRoleMap := make(map[string]*rbacv1.ClusterRole)
	for i := range crList.Items {
		clusterRoleMap[crList.Items[i].Name] = &crList.Items[i]
	}
	roleMap := make(map[string]map[string]*rbacv1.Role)
	for i := range roleList.Items {
		ns := roleList.Items[i].Namespace
		if roleMap[ns] == nil {
			roleMap[ns] = make(map[string]*rbacv1.Role)
		}
		roleMap[ns][roleList.Items[i].Name] = &roleList.Items[i]
	}
	saExists := make(map[string]bool)
	for _, sa := range saList.Items {
		saExists[fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)] = true
	}

	subjectMap := make(map[string]*SubjectRisk)
	unusedBindings := 0

	// Process ClusterRoleBindings
	for i := range crbList.Items {
		crb := &crbList.Items[i]
		rules := resolveClusterRoleRef(crb.RoleRef, clusterRoleMap)

		for _, subj := range crb.Subjects {
			key := subjectKey(subj)
			if subj.Kind == "ServiceAccount" && !saExists[fmt.Sprintf("%s/%s", subj.Namespace, subj.Name)] {
				unusedBindings++
			}
			sr := getOrCreateSubject(subjectMap, key, subj)
			sr.Bindings = append(sr.Bindings, BindingSummary{
				Name: crb.Name, Kind: "ClusterRoleBinding",
				RoleRefKind: crb.RoleRef.Kind, RoleRefName: crb.RoleRef.Name,
			})
			sr.IsClusterScoped = true
			mergeRules(sr, rules, "cluster")
		}
	}

	// Process RoleBindings
	for i := range rbList.Items {
		rb := &rbList.Items[i]
		rules := resolveRoleRef(rb.Namespace, rb.RoleRef, clusterRoleMap, roleMap)

		for _, subj := range rb.Subjects {
			key := subjectKey(subj)
			if subj.Kind == "ServiceAccount" && !saExists[fmt.Sprintf("%s/%s", subj.Namespace, subj.Name)] {
				unusedBindings++
			}
			sr := getOrCreateSubject(subjectMap, key, subj)
			sr.Bindings = append(sr.Bindings, BindingSummary{
				Name: rb.Name, Namespace: rb.Namespace, Kind: "RoleBinding",
				RoleRefKind: rb.RoleRef.Kind, RoleRefName: rb.RoleRef.Name,
			})
			mergeRules(sr, rules, rb.Namespace)
		}
	}

	// Analyze each subject
	var subjects []SubjectRisk
	for _, sr := range subjectMap {
		analyzeSubjectRisk(sr)
		subjects = append(subjects, *sr)
	}

	// Sort by risk score descending
	sort.Slice(subjects, func(i, j int) bool {
		if subjects[i].RiskScore != subjects[j].RiskScore {
			return subjects[i].RiskScore > subjects[j].RiskScore
		}
		return subjects[i].DisplayName < subjects[j].DisplayName
	})

	// Build summary (exclude system subjects)
	totalBindings := len(crbList.Items) + len(rbList.Items)
	summary := RBACSummary{
		ByRiskLevel:    make(map[string]int),
		TotalBindings:  totalBindings,
		UnusedBindings: unusedBindings,
	}
	for _, s := range subjects {
		if strings.HasPrefix(s.Name, "system:") {
			continue
		}
		summary.TotalSubjects++
		summary.ByRiskLevel[string(s.RiskLevel)]++
		if s.IsClusterScoped {
			summary.ClusterScoped++
		}
		if s.PrivilegeEscalation {
			summary.PrivilegeEscalation++
		}
		if s.WildcardAccess {
			summary.WildcardAccess++
		}
	}

	writeJSON(w, RBACRiskResult{
		ScannedAt: time.Now(),
		Summary:   summary,
		Subjects:  subjects,
	})
}

func subjectKey(subj rbacv1.Subject) string {
	return fmt.Sprintf("%s:%s:%s", subj.Kind, subj.Namespace, subj.Name)
}

func getOrCreateSubject(m map[string]*SubjectRisk, key string, subj rbacv1.Subject) *SubjectRisk {
	if sr, ok := m[key]; ok {
		return sr
	}
	displayName := subj.Name
	if subj.Kind == "ServiceAccount" && subj.Namespace != "" {
		displayName = fmt.Sprintf("%s/%s", subj.Namespace, subj.Name)
	}
	sr := &SubjectRisk{
		Kind:        RBACSubjectKind(subj.Kind),
		Name:        subj.Name,
		Namespace:   subj.Namespace,
		DisplayName: displayName,
	}
	m[key] = sr
	return sr
}

func resolveClusterRoleRef(roleRef rbacv1.RoleRef, crMap map[string]*rbacv1.ClusterRole) []rbacv1.PolicyRule {
	if roleRef.Kind == "ClusterRole" {
		if cr, ok := crMap[roleRef.Name]; ok {
			return cr.Rules
		}
	}
	return nil
}

func resolveRoleRef(ns string, roleRef rbacv1.RoleRef, crMap map[string]*rbacv1.ClusterRole, roleMap map[string]map[string]*rbacv1.Role) []rbacv1.PolicyRule {
	if roleRef.Kind == "ClusterRole" {
		return resolveClusterRoleRef(roleRef, crMap)
	}
	if roleRef.Kind == "Role" {
		if nsRoles, ok := roleMap[ns]; ok {
			if role, ok := nsRoles[roleRef.Name]; ok {
				return role.Rules
			}
		}
	}
	return nil
}

func mergeRules(sr *SubjectRisk, rules []rbacv1.PolicyRule, scope string) {
	for _, rule := range rules {
		sr.EffectiveRules = append(sr.EffectiveRules, EffectiveRule{
			Verbs:     append([]string{}, rule.Verbs...),
			Resources: append([]string{}, rule.Resources...),
			APIGroups: append([]string{}, rule.APIGroups...),
			Scope:     scope,
		})
	}
}

func analyzeSubjectRisk(sr *SubjectRisk) {
	// Check for cluster-admin
	for _, b := range sr.Bindings {
		if b.RoleRefName == "cluster-admin" {
			sr.RiskLevel = RBACRiskCritical
			sr.RiskScore = 100
			if !strings.HasPrefix(sr.Name, "system:") {
				sr.Issues = append(sr.Issues, "Has cluster-admin privileges — full cluster control")
			}
			return
		}
	}

	score := 0
	hasWrite := false
	hasWildcardVerbs := false
	hasWildcardResources := false
	hasEscalation := false
	hasSensitiveResources := false
	clusterWrite := false

	sensitiveResources := map[string]bool{
		"secrets": true, "pods/exec": true, "serviceaccounts": true,
		"rolebindings": true, "clusterrolebindings": true,
		"roles": true, "clusterroles": true, "nodes": true,
		"persistentvolumes": true, "podsecuritypolicies": true,
	}

	for _, rule := range sr.EffectiveRules {
		for _, v := range rule.Verbs {
			if v == "*" {
				hasWildcardVerbs = true
				hasWrite = true // * implies write
				if rule.Scope == "cluster" {
					clusterWrite = true
				}
			}
			if v == "create" || v == "update" || v == "patch" || v == "delete" {
				hasWrite = true
				if rule.Scope == "cluster" {
					clusterWrite = true
				}
			}
		}
		for _, res := range rule.Resources {
			if res == "*" {
				hasWildcardResources = true
			}
			if sensitiveResources[res] {
				hasSensitiveResources = true
			}
			if res == "clusterrolebindings" || res == "rolebindings" || res == "clusterroles" || res == "roles" {
				for _, v := range rule.Verbs {
					if v == "*" || v == "create" || v == "update" || v == "patch" || v == "bind" || v == "escalate" {
						hasEscalation = true
					}
				}
			}
		}
	}

	sr.WildcardAccess = hasWildcardVerbs || hasWildcardResources
	sr.PrivilegeEscalation = hasEscalation

	if clusterWrite {
		score += 30
	}
	if sr.IsClusterScoped {
		score += 15
	}
	if hasWildcardVerbs {
		score += 25
	}
	if hasWildcardResources {
		score += 20
	}
	if hasEscalation {
		score += 25
		sr.Issues = append(sr.Issues, "Can modify RBAC bindings — potential privilege escalation path")
	}
	if hasSensitiveResources {
		score += 15
		sr.Issues = append(sr.Issues, "Has access to sensitive resources (secrets, pods/exec, serviceaccounts)")
	}
	if hasWrite && !clusterWrite {
		score += 10
	}

	// Default SA with write
	if sr.Kind == SubjectKindServiceAccount && sr.Name == "default" && hasWrite {
		score += 15
		sr.Issues = append(sr.Issues, "Default service account has write permissions — should be restricted")
	}

	if score > 99 {
		score = 99
	}
	sr.RiskScore = score

	switch {
	case score >= 70:
		sr.RiskLevel = RBACRiskHigh
	case score >= 40:
		sr.RiskLevel = RBACRiskMedium
	default:
		sr.RiskLevel = RBACRiskLow
	}
}
