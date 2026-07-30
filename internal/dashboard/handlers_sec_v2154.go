package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.54 — Security Dimension (Round 45)
// 1. Pod Seccomp Type Distribution
// 2. Namespace NetworkPolicy Default Deny Check
// 3. ClusterRole Verbs Wildcard Detector
// ============================================================

type SeccompTypeResult2154 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SeccompTypeSummary2154 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type SeccompTypeSummary2154 struct {
	TotalPods int            `json:"totalPods"`
	ByType    map[string]int `json:"bySeccompType"`
}

func (s *Server) handleSeccompType2154(w http.ResponseWriter, r *http.Request) {
	result := SeccompTypeResult2154{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byT := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		seccomp := "Unconfined"
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			sp := pod.Spec.SecurityContext.SeccompProfile
			if sp.Type == corev1.SeccompProfileTypeRuntimeDefault {
				seccomp = "RuntimeDefault"
			} else if sp.Type == corev1.SeccompProfileTypeLocalhost {
				seccomp = "Localhost"
			}
		}
		byT[seccomp]++
	}
	result.Summary.ByType = byT
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Default Deny Check
type DefaultDenyResult2154 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         DefaultDenySummary2154 `json:"summary"`
	OpenNS          []DefaultDenyEntry2154 `json:"openNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

type DefaultDenySummary2154 struct {
	TotalNS     int `json:"totalNamespaces"`
	DefaultDeny int `json:"defaultDeny"`
}

type DefaultDenyEntry2154 struct {
	Namespace string `json:"namespace"`
}

func (s *Server) handleDefaultDeny2154(w http.ResponseWriter, r *http.Request) {
	result := DefaultDenyResult2154{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	nsDeny := make(map[string]bool)
	for _, np := range npList.Items {
		if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
			hasDenyAll := false
			for _, pt := range np.Spec.PolicyTypes {
				if pt == "Ingress" {
					hasDenyAll = true
				}
			}
			if hasDenyAll {
				nsDeny[np.Namespace] = true
			}
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsDeny[ns.Name] {
			result.Summary.DefaultDeny++
		} else {
			result.OpenNS = append(result.OpenNS, DefaultDenyEntry2154{Namespace: ns.Name})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OpenNS, func(i, j int) bool { return result.OpenNS[i].Namespace < result.OpenNS[j].Namespace })

	if len(result.OpenNS) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces without default deny NetworkPolicy", len(result.OpenNS)))
	}
	writeJSON(w, result)
}

// 3. CR Wildcard Verbs
type CRWildVerbResult2154 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         CRWildVerbSummary2154 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type CRWildVerbSummary2154 struct {
	TotalCR    int `json:"totalClusterRoles"`
	WildVerbCR int `json:"withWildcardVerbs"`
}

func (s *Server) handleCRWildVerb2154(w http.ResponseWriter, r *http.Request) {
	result := CRWildVerbResult2154{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WildVerbCR++
					break
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
