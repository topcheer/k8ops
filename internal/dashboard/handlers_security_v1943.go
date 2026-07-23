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
// v19.43 — Security Dimension (Round 10)
// 1. Network Policy Coverage Auditor — namespace & pod-level policy gaps
// 2. Container syscall exposure — seccomp profile analysis
// 3. Kubernetes API Discovery Exposure — anonymous access & RBAC gaps
// ============================================================

// ---------------------------------------------------------------
// 1. Network Policy Coverage Auditor
// ---------------------------------------------------------------

type NetPolCoverageResult1943 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         NetPolCoverageSummary1943 `json:"summary"`
	UncoveredNS     []NetPolUncoveredNS1943   `json:"uncoveredNamespaces"`
	Coverage        []NetPolCoverageEntry1943 `json:"coverageByNamespace"`
	Recommendations []string                  `json:"recommendations"`
}

type NetPolCoverageSummary1943 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithNetPol      int `json:"namespacesWithNetPol"`
	WithoutNetPol   int `json:"namespacesWithoutNetPol"`
	TotalNetPols    int `json:"totalNetworkPolicies"`
	DefaultDenyNS   int `json:"defaultDenyNamespaces"`
	IsolatedPods    int `json:"isolatedPods"`
	UnisolatedPods  int `json:"unisolatedPods"`
}

type NetPolUncoveredNS1943 struct {
	Namespace    string `json:"namespace"`
	PodCount     int    `json:"podCount"`
	ServiceCount int    `json:"serviceCount"`
	Severity     string `json:"severity"`
}

type NetPolCoverageEntry1943 struct {
	Namespace      string `json:"namespace"`
	NetPolCount    int    `json:"netPolCount"`
	HasDefaultDeny bool   `json:"hasDefaultDeny"`
	PodCount       int    `json:"podCount"`
}

func (s *Server) handleNetPolCoverage(w http.ResponseWriter, r *http.Request) {
	result := NetPolCoverageResult1943{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	// Count NetPols and default-deny per namespace
	npCount := make(map[string]int)
	hasDefaultDeny := make(map[string]bool)
	for _, np := range npList.Items {
		npCount[np.Namespace]++
		// Default deny = policy with empty podSelector and empty ingress/egress
		if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
			if len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0 {
				hasDefaultDeny[np.Namespace] = true
			}
		}
	}

	// Count pods per namespace
	podCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCount[pod.Namespace]++
		}
	}
	svcCount := make(map[string]int)
	for _, svc := range svcList.Items {
		svcCount[svc.Namespace]++
	}

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		count := npCount[ns.Name]
		pods := podCount[ns.Name]
		isDefaultDeny := hasDefaultDeny[ns.Name]

		entry := NetPolCoverageEntry1943{
			Namespace: ns.Name, NetPolCount: count,
			HasDefaultDeny: isDefaultDeny, PodCount: pods,
		}
		result.Coverage = append(result.Coverage, entry)

		if count > 0 {
			result.Summary.WithNetPol++
			result.Summary.TotalNetPols += count
			if isDefaultDeny {
				result.Summary.DefaultDenyNS++
				result.Summary.IsolatedPods += pods
			}
		} else {
			result.Summary.WithoutNetPol++
			result.Summary.UnisolatedPods += pods
			severity := "medium"
			if pods > 10 {
				severity = "high"
			}
			result.UncoveredNS = append(result.UncoveredNS, NetPolUncoveredNS1943{
				Namespace: ns.Name, PodCount: pods,
				ServiceCount: svcCount[ns.Name], Severity: severity,
			})
			if severity == "high" {
				score -= 5
			} else {
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutNetPol > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without NetworkPolicy — add default-deny", result.Summary.WithoutNetPol))
	}
	if result.Summary.DefaultDenyNS == 0 {
		result.Recommendations = append(result.Recommendations, "No namespace has default-deny — all traffic is unrestricted")
	}
	if result.Summary.UnisolatedPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods without network isolation — exposed to all traffic", result.Summary.UnisolatedPods))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Syscall Exposure — seccomp profile analysis
// ---------------------------------------------------------------

type SeccompResult1943 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SeccompSummary1943 `json:"summary"`
	Containers      []SeccompEntry1943 `json:"containers"`
	Risks           []SeccompRisk1943  `json:"risks"`
	Recommendations []string           `json:"recommendations"`
}

type SeccompSummary1943 struct {
	TotalContainers int `json:"totalContainers"`
	WithSeccomp     int `json:"withSeccomp"`
	WithoutSeccomp  int `json:"withoutSeccomp"`
	Unconfined      int `json:"unconfinedCount"`
	RuntimeDefault  int `json:"runtimeDefaultCount"`
	CustomProfile   int `json:"customProfileCount"`
	WithCapAdd      int `json:"withCapAdd"`
	WithCapDropAll  int `json:"withCapDropAll"`
}

type SeccompEntry1943 struct {
	PodName        string `json:"podName"`
	Namespace      string `json:"namespace"`
	Container      string `json:"container"`
	SeccompProfile string `json:"seccompProfile"`
	HasCapAdd      bool   `json:"hasCapAdd"`
	CapDropAll     bool   `json:"capDropAll"`
}

type SeccompRisk1943 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleSeccompExposure(w http.ResponseWriter, r *http.Request) {
	result := SeccompResult1943{ScannedAt: time.Now()}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Pod-level seccomp
		podSeccomp := ""
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			if pod.Spec.SecurityContext.SeccompProfile.Type == "RuntimeDefault" {
				podSeccomp = "RuntimeDefault"
			} else if pod.Spec.SecurityContext.SeccompProfile.Type == "Unconfined" {
				podSeccomp = "Unconfined"
			} else if pod.Spec.SecurityContext.SeccompProfile.Type == "Localhost" {
				podSeccomp = "Localhost"
			}
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			seccompProfile := podSeccomp
			hasCapAdd := false
			capDropAll := false

			if c.SecurityContext != nil {
				if c.SecurityContext.Capabilities != nil {
					hasCapAdd = len(c.SecurityContext.Capabilities.Add) > 0
					for _, drop := range c.SecurityContext.Capabilities.Drop {
						if string(drop) == "ALL" {
							capDropAll = true
						}
					}
				}
				if c.SecurityContext.SeccompProfile != nil {
					if c.SecurityContext.SeccompProfile.Type == "RuntimeDefault" {
						seccompProfile = "RuntimeDefault"
					} else if c.SecurityContext.SeccompProfile.Type == "Unconfined" {
						seccompProfile = "Unconfined"
					} else if c.SecurityContext.SeccompProfile.Type == "Localhost" {
						seccompProfile = "Localhost"
					}
				}
			}

			entry := SeccompEntry1943{
				PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				SeccompProfile: seccompProfile, HasCapAdd: hasCapAdd, CapDropAll: capDropAll,
			}
			if len(result.Containers) < 100 {
				result.Containers = append(result.Containers, entry)
			}

			switch seccompProfile {
			case "RuntimeDefault":
				result.Summary.WithSeccomp++
				result.Summary.RuntimeDefault++
			case "Unconfined":
				result.Summary.Unconfined++
				result.Risks = append(result.Risks, SeccompRisk1943{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					RiskType: "seccomp-unconfined", Severity: "high",
					Detail: "Seccomp Unconfined — all syscalls allowed, high attack surface",
				})
				score -= 5
			case "Localhost":
				result.Summary.WithSeccomp++
				result.Summary.CustomProfile++
			default:
				result.Summary.WithoutSeccomp++
				result.Risks = append(result.Risks, SeccompRisk1943{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					RiskType: "no-seccomp", Severity: "medium",
					Detail: "No seccomp profile — defaults to no syscall filtering",
				})
				score -= 2
			}

			if hasCapAdd {
				result.Summary.WithCapAdd++
				score -= 1
			}
			if capDropAll {
				result.Summary.WithCapDropAll++
			} else {
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutSeccomp > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without seccomp — add RuntimeDefault profile", result.Summary.WithoutSeccomp))
	}
	if result.Summary.Unconfined > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with Unconfined seccomp — restrict immediately", result.Summary.Unconfined))
	}
	if result.Summary.WithCapAdd > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with added capabilities — audit and remove unnecessary caps", result.Summary.WithCapAdd))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Kubernetes API Discovery Exposure
// ---------------------------------------------------------------

type APIDiscoveryResult1943 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         APIDiscoverySummary1943 `json:"summary"`
	Exposures       []APIDiscoveryEntry1943 `json:"exposures"`
	Recommendations []string                `json:"recommendations"`
}

type APIDiscoverySummary1943 struct {
	TotalAPIResources int `json:"totalAPIResources"`
	AnonymousAccess   int `json:"anonymousAccessible"`
	VerbGet           int `json:"verbGetResources"`
	VerbList          int `json:"verbListResources"`
	VerbCreate        int `json:"verbCreateResources"`
	VerbDelete        int `json:"verbDeleteResources"`
	VerbsWildcard     int `json:"verbsWildcard"`
	Subresources      int `json:"subresourceCount"`
}

type APIDiscoveryEntry1943 struct {
	Resource   string `json:"resource"`
	Group      string `json:"group"`
	Version    string `json:"version"`
	Namespaced bool   `json:"namespaced"`
	VerbCount  int    `json:"verbCount"`
}

func (s *Server) handleAPIDiscoveryExposure(w http.ResponseWriter, r *http.Request) {
	result := APIDiscoveryResult1943{ScannedAt: time.Now()}
	score := 100

	_, apiResList, err := s.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, apiGroup := range apiResList {
		gv := apiGroup.GroupVersion
		groupName := "core"
		if parts := strings.SplitN(gv, "/", 2); len(parts) == 2 {
			groupName = parts[0]
		}

		for _, res := range apiGroup.APIResources {
			if strings.Contains(res.Name, "/") {
				result.Summary.Subresources++
				continue
			}

			result.Summary.TotalAPIResources++

			verbCount := len(res.Verbs)
			for _, v := range res.Verbs {
				switch v {
				case "get":
					result.Summary.VerbGet++
				case "list":
					result.Summary.VerbList++
				case "create":
					result.Summary.VerbCreate++
				case "delete":
					result.Summary.VerbDelete++
				case "*":
					result.Summary.VerbsWildcard++
				}
			}

			if len(result.Exposures) < 200 {
				result.Exposures = append(result.Exposures, APIDiscoveryEntry1943{
					Resource: res.Name, Group: groupName, Version: gv,
					Namespaced: res.Namespaced, VerbCount: verbCount,
				})
			}
		}
	}

	// Score based on exposure surface
	if result.Summary.TotalAPIResources > 300 {
		score -= 5
	}
	if result.Summary.VerbsWildcard > 0 {
		score -= 5
	}

	// Check for anonymous access patterns (system:anonymous / system:unauthenticated bindings)
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	for _, crb := range crbList.Items {
		for _, sub := range crb.Subjects {
			if sub.Name == "system:anonymous" || sub.Name == "system:unauthenticated" {
				result.Summary.AnonymousAccess++
				score -= 10
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.AnonymousAccess > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d bindings grant anonymous access — remove immediately", result.Summary.AnonymousAccess))
	}
	if result.Summary.VerbsWildcard > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources with wildcard verbs — restrict to explicit verbs", result.Summary.VerbsWildcard))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total API resources exposed — review RBAC coverage", result.Summary.TotalAPIResources))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
