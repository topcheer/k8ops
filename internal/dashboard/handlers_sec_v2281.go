package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.81 Security: Seccomp Profile Audit, NetPol Default Deny Check, ClusterRole Wildcard Audit
type SeccompResult2281 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		WithSeccomp int `json:"withSeccompProfile"`
	} `json:"summary"`
}

func (s *Server) handleSeccomp2281(w http.ResponseWriter, r *http.Request) {
	result := SeccompResult2281{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			result.Summary.WithSeccomp++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		unprotected := result.Summary.TotalPods - result.Summary.WithSeccomp
		score = 100 - (unprotected*50)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DefaultDenyResult2281 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS         int `json:"totalNS"`
		WithDefaultDeny int `json:"withDefaultDeny"`
	} `json:"summary"`
}

func (s *Server) handleDefaultDeny2281(w http.ResponseWriter, r *http.Request) {
	result := DefaultDenyResult2281{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	denyNS := make(map[string]bool)
	for _, np := range npList.Items {
		if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
			if len(np.Spec.Ingress) == 0 || len(np.Spec.Egress) == 0 {
				denyNS[np.Namespace] = true
			}
		}
	}
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		if denyNS[ns.Name] {
			result.Summary.WithDefaultDeny++
		}
	}
	score := 100
	if result.Summary.TotalNS > 0 {
		score = result.Summary.WithDefaultDeny * 100 / result.Summary.TotalNS
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type WildcardRoleResult2281 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalClusterRoles int `json:"totalClusterRoles"`
		WithWildcard      int `json:"withWildcardVerb"`
	} `json:"summary"`
}

func (s *Server) handleWildcardRole2281(w http.ResponseWriter, r *http.Request) {
	result := WildcardRoleResult2281{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalClusterRoles++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WithWildcard++
					break
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalClusterRoles > 0 {
		wildcardPct := result.Summary.WithWildcard * 100 / result.Summary.TotalClusterRoles
		score = 100 - wildcardPct/3
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
