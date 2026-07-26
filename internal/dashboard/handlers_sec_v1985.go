package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.85 — Security Dimension (Round 17)
// 1. RBAC Wildcard Verb — wildcard (*) verb usage in roles
// 2. Anonymous Auth Risk — pods/services exposing unauthenticated endpoints
// 3. Node Restriction Label — node label tamper surface analysis
// ============================================================

// ---------------------------------------------------------------
// 1. RBAC Wildcard Verb
// ---------------------------------------------------------------

type RBACWildcardResult1985 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RBACWildcardSummary1985 `json:"summary"`
	WildcardRoles   []RBACWildcardEntry1985 `json:"wildcardRoles"`
	Recommendations []string                `json:"recommendations"`
}

type RBACWildcardSummary1985 struct {
	TotalRoles           int `json:"totalRoles"`
	WithWildcardVerb     int `json:"withWildcardVerb"`
	WithWildcardResource int `json:"withWildcardResource"`
	WithWildcardAPIGroup int `json:"withWildcardAPIGroup"`
	HighRiskRoles        int `json:"highRiskRoles"`
}

type RBACWildcardEntry1985 struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	Verbs     []string `json:"verbs"`
	Resources []string `json:"resources"`
	RiskLevel string   `json:"riskLevel"`
}

func (s *Server) handleRBACWildcardVerb(w http.ResponseWriter, r *http.Request) {
	result := RBACWildcardResult1985{ScannedAt: time.Now()}
	score := 100

	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	roleList, _ := s.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})

	checkWildcard := func(name, ns, kind string, rules interface{}) {
		// Use reflection-free approach: iterate rules
	}

	checkWildcard = func(name, ns, kind string, rulesVal interface{}) {}

	// ClusterRoles
	for _, role := range crList.Items {
		if strings.HasPrefix(role.Name, "system:") {
			continue
		}
		result.Summary.TotalRoles++

		for _, rule := range role.Rules {
			hasWildVerb := containsInList1985(rule.Verbs, "*")
			hasWildResource := containsInList1985(rule.Resources, "*")
			hasWildGroup := containsInList1985(rule.APIGroups, "*")

			if hasWildVerb {
				result.Summary.WithWildcardVerb++
			}
			if hasWildResource {
				result.Summary.WithWildcardResource++
			}
			if hasWildGroup {
				result.Summary.WithWildcardAPIGroup++
			}

			risk := "low"
			if hasWildVerb && hasWildResource {
				risk = "critical"
				result.Summary.HighRiskRoles++
				score -= 5
			} else if hasWildVerb {
				risk = "high"
				score -= 2
			} else if hasWildResource {
				risk = "medium"
			}

			if risk == "critical" || risk == "high" {
				result.WildcardRoles = append(result.WildcardRoles, RBACWildcardEntry1985{
					Name: role.Name, Namespace: "cluster", Kind: "ClusterRole",
					Verbs: rule.Verbs, Resources: rule.Resources, RiskLevel: risk,
				})
			}
		}
	}

	// Roles
	for _, role := range roleList.Items {
		result.Summary.TotalRoles++

		for _, rule := range role.Rules {
			hasWildVerb := containsInList1985(rule.Verbs, "*")
			hasWildResource := containsInList1985(rule.Resources, "*")

			if hasWildVerb {
				result.Summary.WithWildcardVerb++
			}
			if hasWildResource {
				result.Summary.WithWildcardResource++
			}

			risk := "low"
			if hasWildVerb && hasWildResource {
				risk = "critical"
				result.Summary.HighRiskRoles++
				score -= 3
			} else if hasWildVerb {
				risk = "high"
				score -= 1
			}

			if risk == "critical" || risk == "high" {
				result.WildcardRoles = append(result.WildcardRoles, RBACWildcardEntry1985{
					Name: role.Name, Namespace: role.Namespace, Kind: "Role",
					Verbs: rule.Verbs, Resources: rule.Resources, RiskLevel: risk,
				})
			}
		}
	}

	_ = checkWildcard // suppress unused

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d roles: %d wildcard verbs, %d wildcard resources, %d high-risk", result.Summary.TotalRoles, result.Summary.WithWildcardVerb, result.Summary.WithWildcardResource, result.Summary.HighRiskRoles))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsInList1985(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 2. Anonymous Auth Risk
// ---------------------------------------------------------------

type AnonAuthResult1985 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AnonAuthSummary1985 `json:"summary"`
	ExposedSvcs     []AnonAuthEntry1985 `json:"exposedServices"`
	Recommendations []string            `json:"recommendations"`
}

type AnonAuthSummary1985 struct {
	TotalServices    int `json:"totalServices"`
	LoadBalancerSvcs int `json:"loadBalancerServices"`
	NodePortSvcs     int `json:"nodePortServices"`
	ExternalIPSvcs   int `json:"externalIPServices"`
	WithoutAuth      int `json:"servicesWithoutAuth"`
}

type AnonAuthEntry1985 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	HasAuth   bool   `json:"hasAuthAnnotation"`
}

func (s *Server) handleAnonymousAuthRisk(w http.ResponseWriter, r *http.Request) {
	result := AnonAuthResult1985{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		exposed := false
		entry := AnonAuthEntry1985{
			Name: svc.Name, Namespace: svc.Namespace,
			Type: string(svc.Spec.Type),
		}

		switch svc.Spec.Type {
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancerSvcs++
			exposed = true
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePortSvcs++
			exposed = true
		}
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.ExternalIPSvcs++
			exposed = true
		}

		// Check for auth annotations
		hasAuth := false
		for k := range svc.Annotations {
			if strings.Contains(k, "auth") || strings.Contains(k, "ingress") {
				if strings.Contains(k, "nginx.ingress.kubernetes.io/auth") ||
					strings.Contains(k, "traefik.ingress.kubernetes.io/auth") ||
					strings.Contains(k, "oauth") {
					hasAuth = true
					break
				}
			}
		}
		entry.HasAuth = hasAuth

		if exposed && !hasAuth {
			result.Summary.WithoutAuth++
			result.ExposedSvcs = append(result.ExposedSvcs, entry)
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services: %d LB, %d NodePort, %d without auth annotation", result.Summary.TotalServices, result.Summary.LoadBalancerSvcs, result.Summary.NodePortSvcs, result.Summary.WithoutAuth))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Restriction Label
// ---------------------------------------------------------------

type NodeRestrResult1985 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeRestrSummary1985 `json:"summary"`
	Nodes           []NodeRestrEntry1985 `json:"nodes"`
	Recommendations []string             `json:"recommendations"`
}

type NodeRestrSummary1985 struct {
	TotalNodes       int `json:"totalNodes"`
	WithRestriction  int `json:"nodesWithRestriction"`
	UserLabelCount   int `json:"userManagedLabels"`
	SystemLabelCount int `json:"systemLabelCount"`
}

type NodeRestrEntry1985 struct {
	Name         string   `json:"name"`
	UserLabels   []string `json:"userManagedLabels"`
	SystemLabels int      `json:"systemLabelCount"`
}

func (s *Server) handleNodeRestrictionLabel(w http.ResponseWriter, r *http.Request) {
	result := NodeRestrResult1985{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	systemPrefixes := []string{
		"kubernetes.io/", "k8s.io/", "node.kubernetes.io/",
		"beta.kubernetes.io/", "k3s.io/", "node-role.",
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		entry := NodeRestrEntry1985{Name: node.Name}
		sysCount := 0

		for k := range node.Labels {
			isSystem := false
			for _, prefix := range systemPrefixes {
				if strings.HasPrefix(k, prefix) {
					isSystem = true
					break
				}
			}
			if isSystem {
				sysCount++
			} else {
				entry.UserLabels = append(entry.UserLabels, k)
			}
		}

		entry.SystemLabels = sysCount
		result.Summary.SystemLabelCount += sysCount
		result.Summary.UserLabelCount += len(entry.UserLabels)

		result.Nodes = append(result.Nodes, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d system labels, %d user-managed labels", result.Summary.TotalNodes, result.Summary.SystemLabelCount, result.Summary.UserLabelCount))
	if result.Summary.UserLabelCount > 20 {
		result.Recommendations = append(result.Recommendations, "High user-managed label count — consider NodeRestriction admission for label governance")
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
