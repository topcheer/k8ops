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
// v21.24 — Security Dimension (Round 40)
// 1. Pod Sysctl Validator
// 2. Namespace PSA Level Audit
// 3. ClusterRole Verbs Per Resource Audit
// ============================================================

type SysctlResult2124 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SysctlSummary2124 `json:"summary"`
	AtRisk          []SysctlEntry2124 `json:"atRiskPods"`
	Recommendations []string          `json:"recommendations"`
}

type SysctlSummary2124 struct {
	TotalPods int `json:"totalPods"`
	AtRisk    int `json:"atRiskPods"`
}

type SysctlEntry2124 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSysctl2124(w http.ResponseWriter, r *http.Request) {
	result := SysctlResult2124{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.SecurityContext.Sysctls) > 0 {
			for _, sysctl := range pod.Spec.SecurityContext.Sysctls {
				if containsStr2039(sysctl.Name, "kernel.shm") || containsStr2039(sysctl.Name, "kernel.msg") {
					result.Summary.AtRisk++
					result.AtRisk = append(result.AtRisk, SysctlEntry2124{Pod: pod.Name, Namespace: pod.Namespace})
					score -= 3
					break
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.AtRisk, func(i, j int) bool { return result.AtRisk[i].Namespace < result.AtRisk[j].Namespace })
	writeJSON(w, result)
}

// 2. PSA Level Audit
type PSAResult2124 struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	HealthScore     int            `json:"healthScore"`
	Grade           string         `json:"grade"`
	Summary         PSASummary2124 `json:"summary"`
	Recommendations []string       `json:"recommendations"`
}

type PSASummary2124 struct {
	TotalNS     int `json:"totalNamespaces"`
	Enforced    int `json:"psaEnforced"`
	NotEnforced int `json:"psaNotEnforced"`
}

func (s *Server) handlePSA2124(w http.ResponseWriter, r *http.Request) {
	result := PSAResult2124{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if ns.Labels["pod-security.kubernetes.io/enforce"] != "" {
			result.Summary.Enforced++
		} else {
			result.Summary.NotEnforced++
		}
	}
	if result.Summary.NotEnforced > result.Summary.TotalNS/2 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NotEnforced > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces without PSA enforce", result.Summary.NotEnforced))
	}
	writeJSON(w, result)
}

// 3. CR Verbs Per Resource
type CRVerbsResult2124 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CRVerbsSummary2124 `json:"summary"`
	BroadCR         []CRVerbsEntry2124 `json:"broadClusterRoles"`
	Recommendations []string           `json:"recommendations"`
}

type CRVerbsSummary2124 struct {
	TotalCR int `json:"totalClusterRoles"`
	BroadCR int `json:"broadClusterRoles"`
}

type CRVerbsEntry2124 struct {
	Name string `json:"name"`
}

func (s *Server) handleCRVerbs2124(w http.ResponseWriter, r *http.Request) {
	result := CRVerbsResult2124{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		for _, rule := range cr.Rules {
			for _, res := range rule.Resources {
				if res == "*" || res == "pods" || res == "secrets" {
					for _, verb := range rule.Verbs {
						if verb == "*" || verb == "delete" || verb == "create" {
							result.Summary.BroadCR++
							result.BroadCR = append(result.BroadCR, CRVerbsEntry2124{Name: cr.Name})
							score -= 1
							break
						}
					}
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.BroadCR, func(i, j int) bool { return result.BroadCR[i].Name < result.BroadCR[j].Name })
	writeJSON(w, result)
}
