package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RBACBlastResult analyzes RBAC privilege escalation paths and blast radius.
type RBACBlastResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RBACBlastSummary     `json:"summary"`
	HighRiskRoles   []HighRiskRole       `json:"highRiskRoles"`
	EscalationPaths []RBACEscalationPath `json:"escalationPaths"`
	RiskScore       int                  `json:"riskScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type RBACBlastSummary struct {
	TotalRoles      int `json:"totalRoles"`
	TotalBindings   int `json:"totalBindings"`
	ClusterAdmins   int `json:"clusterAdmins"`
	PrivilegedRoles int `json:"privilegedRoles"`
	WildcardRoles   int `json:"wildcardRoles"`
	SubjectCount    int `json:"subjectCount"`
}

type HighRiskRole struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Subjects  int    `json:"subjects"`
}

type RBACEscalationPath struct {
	Subject  string `json:"subject"`
	Via      string `json:"via"`
	Reaches  string `json:"reaches"`
	Severity string `json:"severity"`
}

// handleRBACBlast analyzes RBAC privilege escalation paths and blast radius.
// GET /api/security/rbac-blast
func (s *Server) handleRBACBlast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RBACBlastResult{ScannedAt: time.Now()}

	roles, _ := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	bindings, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	nsRoles, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	nsBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	result.Summary.TotalBindings = len(bindings.Items) + len(nsBindings.Items)
	result.Summary.SubjectCount = len(bindings.Items)

	// Analyze cluster roles for dangerous permissions
	for _, role := range roles.Items {
		result.Summary.TotalRoles++
		riskType := ""
		severity := "low"

		for _, rule := range role.Rules {
			// Check for wildcard verbs/resources
			for _, v := range rule.Verbs {
				if v == "*" {
					riskType = "wildcard-verb"
					severity = "high"
					result.Summary.WildcardRoles++
				}
			}
			for _, res := range rule.Resources {
				if res == "*" || res == "pods/exec" || res == "nodes" {
					if riskType == "" {
						riskType = fmt.Sprintf("access-to-%s", res)
						severity = "high"
					}
					if res == "*" {
						result.Summary.PrivilegedRoles++
					}
				}
			}
			// Check for escalate permission
			for _, res := range rule.Resources {
				if res == "clusterroles" || res == "roles" {
					for _, v := range rule.Verbs {
						if v == "escalate" || v == "bind" || v == "*" {
							riskType = "privilege-escalation"
							severity = "critical"
						}
					}
				}
			}
		}

		if riskType != "" && severity != "low" {
			// Count subjects bound to this role
			subjCount := 0
			for _, crb := range bindings.Items {
				if crb.RoleRef.Name == role.Name {
					subjCount += len(crb.Subjects)
				}
			}
			if role.Name == "cluster-admin" {
				result.Summary.ClusterAdmins = subjCount
			}
			result.HighRiskRoles = append(result.HighRiskRoles, HighRiskRole{
				Name: role.Name, Kind: "ClusterRole", RiskType: riskType,
				Severity: severity, Subjects: subjCount,
			})
		}
	}

	// Check namespace-level roles too
	result.Summary.TotalRoles += len(nsRoles.Items)

	// Escalation paths: subjects with cluster-admin
	for _, crb := range bindings.Items {
		if crb.RoleRef.Name == "cluster-admin" {
			for _, subj := range crb.Subjects {
				result.EscalationPaths = append(result.EscalationPaths, RBACEscalationPath{
					Subject: subj.Name + " (" + subj.Kind + ")",
					Via:     "cluster-admin binding", Reaches: "full cluster control",
					Severity: "critical",
				})
			}
		}
	}

	// Score
	score := 100
	score -= result.Summary.ClusterAdmins * 10
	score -= result.Summary.PrivilegedRoles * 5
	score -= result.Summary.WildcardRoles * 8
	if score < 0 {
		score = 0
	}
	result.RiskScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.RiskScore)

	sort.Slice(result.HighRiskRoles, func(i, j int) bool {
		return result.HighRiskRoles[i].Severity > result.HighRiskRoles[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("RBAC blast radius: %d/100 (grade %s) — %d cluster-admins, %d privileged roles", result.RiskScore, result.Grade, result.Summary.ClusterAdmins, result.Summary.PrivilegedRoles))
	if result.Summary.ClusterAdmins > 3 {
		recs = append(recs, fmt.Sprintf("%d subjects have cluster-admin — reduce to minimum (2-3 break-glass accounts)", result.Summary.ClusterAdmins))
	}
	if result.Summary.PrivilegedRoles > 0 {
		recs = append(recs, fmt.Sprintf("%d roles with wildcard resource access — scope to specific resources", result.Summary.PrivilegedRoles))
	}
	if len(result.EscalationPaths) > 0 {
		recs = append(recs, fmt.Sprintf("%d critical escalation paths — audit and remove unnecessary cluster-admin bindings", len(result.EscalationPaths)))
	}
	if len(recs) == 1 {
		recs = append(recs, "RBAC posture is well-scoped — minimal blast radius")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}

func init() { _ = strings.ToLower }
