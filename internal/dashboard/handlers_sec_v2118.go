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
// v21.18 — Security Dimension (Round 39)
// 1. Pod Supplementary Group Audit
// 2. Namespace Default SA Privilege
// 3. NetworkPolicy Ingress Rule Complexity
// ============================================================

type SuppGroupResult2118 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SuppGroupSummary2118 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type SuppGroupSummary2118 struct {
	TotalPods   int `json:"totalPods"`
	WithSuppGrp int `json:"withSupplementaryGroups"`
}

func (s *Server) handleSuppGroup2118(w http.ResponseWriter, r *http.Request) {
	result := SuppGroupResult2118{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && len(pod.Spec.SecurityContext.SupplementalGroups) > 0 {
			result.Summary.WithSuppGrp++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Default SA Privilege
type DefaultSAResult2118 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DefaultSASummary2118 `json:"summary"`
	PrivilegedNS    []DefaultSAEntry2118 `json:"privilegedNamespaces"`
	Recommendations []string             `json:"recommendations"`
}

type DefaultSASummary2118 struct {
	TotalNS       int `json:"totalNamespaces"`
	DefaultSAPriv int `json:"defaultSAPrivileged"`
}

type DefaultSAEntry2118 struct {
	Namespace string `json:"namespace"`
}

func (s *Server) handleDefaultSA2118(w http.ResponseWriter, r *http.Request) {
	result := DefaultSAResult2118{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	defaultPriv := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && (pod.Spec.ServiceAccountName == "" || pod.Spec.ServiceAccountName == "default") {
			for _, c := range pod.Spec.Containers {
				if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					defaultPriv[pod.Namespace] = true
				}
			}
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if defaultPriv[ns.Name] {
			result.Summary.DefaultSAPriv++
			result.PrivilegedNS = append(result.PrivilegedNS, DefaultSAEntry2118{Namespace: ns.Name})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PrivilegedNS, func(i, j int) bool { return result.PrivilegedNS[i].Namespace < result.PrivilegedNS[j].Namespace })

	if result.Summary.DefaultSAPriv > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces with privileged default SA pods", result.Summary.DefaultSAPriv))
	}
	writeJSON(w, result)
}

// 3. NP Ingress Complexity
type NPIngCmplxResult2118 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NPIngCmplxSummary2118 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type NPIngCmplxSummary2118 struct {
	TotalNP           int `json:"totalNetworkPolicies"`
	TotalIngressRules int `json:"totalIngressRules"`
}

func (s *Server) handleNPIngCmplx2118(w http.ResponseWriter, r *http.Request) {
	result := NPIngCmplxResult2118{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	for _, np := range npList.Items {
		result.Summary.TotalNP++
		result.Summary.TotalIngressRules += len(np.Spec.Ingress)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
