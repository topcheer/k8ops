package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivilegeEscalationPathResult detects potential privilege escalation paths via RBAC.
type PrivilegeEscalationPathResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          PrivEscSummary        `json:"summary"`
	Paths            []PrivEscPathEntry    `json:"escalationPaths"`
	HighRiskSubjects []PrivEscSubjectEntry `json:"highRiskSubjects"`
	HealthScore      int                   `json:"healthScore"`
	Grade            string                `json:"grade"`
	Recommendations  []string              `json:"recommendations"`
}

type PrivEscSummary struct {
	TotalSubjects      int `json:"totalSubjects"`
	ClusterAdmins      int `json:"clusterAdmins"`
	PrivEscalationRisk int `json:"privEscalationRisk"`
	WildcardUsers      int `json:"wildcardUsers"`
	ImpersonateUsers   int `json:"impersonateUsers"`
	EscalateUsers      int `json:"escalateUsers"`
	BindUsers          int `json:"bindUsers"`
}

type PrivEscPathEntry struct {
	Subject        string   `json:"subject"`
	Kind           string   `json:"kind"`
	Namespace      string   `json:"namespace"`
	RiskPath       string   `json:"riskPath"`
	DangerousPerms []string `json:"dangerousPermissions"`
	RiskLevel      string   `json:"riskLevel"`
}

type PrivEscSubjectEntry struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Bindings  int      `json:"bindingCount"`
	RiskScore int      `json:"riskScore"`
	RiskPaths []string `json:"riskPaths"`
}

// Dangerous verbs that enable privilege escalation
var dangerousVerbs = map[string]bool{
	"escalate":    true,
	"bind":        true,
	"impersonate": true,
	"*":           true,
}

// handlePrivilegeEscalationPath handles GET /api/security/privilege-escalation-path
func (s *Server) handlePrivilegeEscalationPath(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PrivilegeEscalationPathResult{ScannedAt: time.Now()}

	crbList, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	crList, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	rbList, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	// Build ClusterRole rules map
	crRules := make(map[string][]rbacv1.PolicyRule)
	for _, cr := range crList.Items {
		crRules[cr.Name] = cr.Rules
	}

	// Subject risk map
	subjectRisk := make(map[string]*PrivEscSubjectEntry)

	// Analyze ClusterRoleBindings
	for _, crb := range crbList.Items {
		for _, subject := range crb.Subjects {
			result.Summary.TotalSubjects++
			key := subject.Kind + "/" + subject.Namespace + "/" + subject.Name
			if subjectRisk[key] == nil {
				subjectRisk[key] = &PrivEscSubjectEntry{
					Name: subject.Name, Kind: subject.Kind, Namespace: subject.Namespace,
				}
			}
			subjectRisk[key].Bindings++

			// Check referenced ClusterRole for dangerous permissions
			rules, ok := crRules[crb.RoleRef.Name]
			if !ok {
				continue
			}

			isClusterAdmin := crb.RoleRef.Name == "cluster-admin"
			if isClusterAdmin {
				result.Summary.ClusterAdmins++
				result.Paths = append(result.Paths, PrivEscPathEntry{
					Subject: subject.Name, Kind: subject.Kind, Namespace: subject.Namespace,
					RiskPath:       fmt.Sprintf("%s -> cluster-admin (full access)", subject.Name),
					DangerousPerms: []string{"cluster-admin"},
					RiskLevel:      "critical",
				})
				subjectRisk[key].RiskPaths = append(subjectRisk[key].RiskPaths, "cluster-admin")
				subjectRisk[key].RiskScore += 50
			}

			for _, rule := range rules {
				for _, verb := range rule.Verbs {
					if dangerousVerbs[verb] {
						perm := fmt.Sprintf("%s on %s", verb, rule.Resources)
						result.Paths = append(result.Paths, PrivEscPathEntry{
							Subject: subject.Name, Kind: subject.Kind, Namespace: subject.Namespace,
							RiskPath:       fmt.Sprintf("%s -> %s -> %s", crb.RoleRef.Name, verb, rule.Resources),
							DangerousPerms: []string{perm},
							RiskLevel:      "high",
						})
						subjectRisk[key].RiskPaths = append(subjectRisk[key].RiskPaths, perm)
						subjectRisk[key].RiskScore += 20

						switch verb {
						case "escalate":
							result.Summary.EscalateUsers++
						case "impersonate":
							result.Summary.ImpersonateUsers++
						case "bind":
							result.Summary.BindUsers++
						case "*":
							result.Summary.WildcardUsers++
						}
					}
				}
			}
		}
	}

	// Build high-risk subjects
	for _, entry := range subjectRisk {
		if entry.RiskScore >= 50 {
			result.HighRiskSubjects = append(result.HighRiskSubjects, *entry)
		}
	}
	sort.Slice(result.HighRiskSubjects, func(i, j int) bool {
		return result.HighRiskSubjects[i].RiskScore > result.HighRiskSubjects[j].RiskScore
	})

	// Count unique subjects
	result.Summary.TotalSubjects = len(subjectRisk)
	result.Summary.PrivEscalationRisk = len(result.Paths)

	// Score: penalize for escalation paths
	riskRatio := 0
	if result.Summary.TotalSubjects > 0 {
		riskCount := result.Summary.ClusterAdmins + result.Summary.EscalateUsers + result.Summary.ImpersonateUsers
		riskRatio = riskCount * 100 / result.Summary.TotalSubjects
	}
	result.HealthScore = 100 - riskRatio
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("权限提升路径: %d 主体, %d cluster-admin, %d wildcard, %d escalate, %d impersonate, %d bind",
			result.Summary.TotalSubjects, result.Summary.ClusterAdmins, result.Summary.WildcardUsers,
			result.Summary.EscalateUsers, result.Summary.ImpersonateUsers, result.Summary.BindUsers),
	}
	if result.Summary.ClusterAdmins > 3 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 cluster-admin 绑定过多, 建议用最小权限角色替代", result.Summary.ClusterAdmins))
	}
	if result.Summary.EscalateUsers > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个主体有 escalate 权限, 可自行提升权限", result.Summary.EscalateUsers))
	}
	if result.HealthScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 实施 RBAC 最小权限原则, 移除不必要的 escalate/bind/impersonate 权限")
	}

	// Use rbList to avoid unused variable
	_ = rbList
	writeJSON(w, result)
}
