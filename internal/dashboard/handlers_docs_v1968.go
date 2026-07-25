package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.68 — Documentation Dimension (Round 14)
// 1. Ingress Catalog — all ingress rules, TLS coverage & host mapping
// 2. Network Policy Catalog — policy inventory & isolation coverage
// 3. Label Inventory — all labels used across cluster resources
// ============================================================

// ---------------------------------------------------------------
// 1. Ingress Catalog
// ---------------------------------------------------------------

type IngressCatalogResult1968 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         IngressCatalogSummary1968 `json:"summary"`
	Ingresses       []IngressCatalogEntry1968 `json:"ingresses"`
	Recommendations []string                  `json:"recommendations"`
}

type IngressCatalogSummary1968 struct {
	TotalIngresses int `json:"totalIngresses"`
	TotalRules     int `json:"totalRules"`
	TotalHosts     int `json:"totalHosts"`
	WithTLS        int `json:"withTLS"`
	WithoutTLS     int `json:"withoutTLS"`
}

type IngressCatalogEntry1968 struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Hosts          []string `json:"hosts"`
	BackendService string   `json:"backendService"`
	HasTLS         bool     `json:"hasTLS"`
	RuleCount      int      `json:"ruleCount"`
}

func (s *Server) handleIngressCatalogDoc(w http.ResponseWriter, r *http.Request) {
	result := IngressCatalogResult1968{ScannedAt: time.Now()}
	score := 100

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	hostSet := make(map[string]bool)
	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++
		entry := IngressCatalogEntry1968{
			Name: ing.Name, Namespace: ing.Namespace,
			Hosts: []string{}, HasTLS: len(ing.Spec.TLS) > 0,
		}

		entry.RuleCount = len(ing.Spec.Rules)
		result.Summary.TotalRules += entry.RuleCount

		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				entry.Hosts = append(entry.Hosts, rule.Host)
				hostSet[rule.Host] = true
			}
			if rule.HTTP != nil && len(rule.HTTP.Paths) > 0 {
				path := rule.HTTP.Paths[0]
				if path.Backend.Service != nil {
					entry.BackendService = path.Backend.Service.Name
				}
			}
		}

		if entry.HasTLS {
			result.Summary.WithTLS++
		} else {
			result.Summary.WithoutTLS++
			score -= 3
		}

		result.Ingresses = append(result.Ingresses, entry)
	}
	result.Summary.TotalHosts = len(hostSet)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses, %d rules, %d unique hosts", result.Summary.TotalIngresses, result.Summary.TotalRules, result.Summary.TotalHosts))
	if result.Summary.WithoutTLS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses without TLS — add certificates for encryption", result.Summary.WithoutTLS))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Network Policy Catalog
// ---------------------------------------------------------------

type NetPolCatalogResult1968 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         NetPolCatalogSummary1968 `json:"summary"`
	Policies        []NetPolCatalogEntry1968 `json:"policies"`
	IsolatedNS      []string                 `json:"isolatedNamespaces"`
	Recommendations []string                 `json:"recommendations"`
}

type NetPolCatalogSummary1968 struct {
	TotalPolicies     int `json:"totalPolicies"`
	WithIngress       int `json:"withIngressRules"`
	WithEgress        int `json:"withEgressRules"`
	NamespacesCovered int `json:"namespacesCovered"`
	DenyAllIngress    int `json:"denyAllIngressPolicies"`
	DenyAllEgress     int `json:"denyAllEgressPolicies"`
}

type NetPolCatalogEntry1968 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	HasIngress  bool   `json:"hasIngress"`
	HasEgress   bool   `json:"hasEgress"`
	PodSelector string `json:"podSelector"`
}

func (s *Server) handleNetPolCatalogDoc(w http.ResponseWriter, r *http.Request) {
	result := NetPolCatalogResult1968{ScannedAt: time.Now()}
	score := 100

	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	nsWithPolicy := make(map[string]bool)

	for _, np := range npList.Items {
		result.Summary.TotalPolicies++
		entry := NetPolCatalogEntry1968{
			Name: np.Name, Namespace: np.Namespace,
			HasIngress: len(np.Spec.Ingress) > 0 || hasPolicyType1968(np.Spec.PolicyTypes, "Ingress"),
			HasEgress:  len(np.Spec.Egress) > 0 || hasPolicyType1968(np.Spec.PolicyTypes, "Egress"),
		}

		if entry.HasIngress {
			result.Summary.WithIngress++
		}
		if entry.HasEgress {
			result.Summary.WithEgress++
		}

		// Check for deny-all (empty ingress/egress with policy type set)
		if hasPolicyType1968(np.Spec.PolicyTypes, "Ingress") && len(np.Spec.Ingress) == 0 {
			result.Summary.DenyAllIngress++
		}
		if hasPolicyType1968(np.Spec.PolicyTypes, "Egress") && len(np.Spec.Egress) == 0 {
			result.Summary.DenyAllEgress++
		}

		nsWithPolicy[np.Namespace] = true
		result.Policies = append(result.Policies, entry)
	}

	result.Summary.NamespacesCovered = len(nsWithPolicy)

	// Check isolated namespaces
	for _, ns := range nsList.Items {
		if nsWithPolicy[ns.Name] {
			result.IsolatedNS = append(result.IsolatedNS, ns.Name)
		}
	}

	// Score: lower if most namespaces lack policies
	totalNS := len(nsList.Items)
	if totalNS > 0 {
		uncovered := totalNS - result.Summary.NamespacesCovered
		if uncovered > totalNS/2 {
			score -= 10
		} else if uncovered > 0 {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d policies across %d/%d namespaces", result.Summary.TotalPolicies, result.Summary.NamespacesCovered, totalNS))
	if totalNS-result.Summary.NamespacesCovered > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without network policies — add default deny for isolation", totalNS-result.Summary.NamespacesCovered))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func hasPolicyType1968(types []networkingv1.PolicyType, check string) bool {
	for _, t := range types {
		if string(t) == check {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 3. Label Inventory
// ---------------------------------------------------------------

type LabelInvResult1968 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LabelInvSummary1968 `json:"summary"`
	TopLabels       []LabelInvEntry1968 `json:"topLabels"`
	NonStandard     []LabelInvEntry1968 `json:"nonStandardLabels"`
	Recommendations []string            `json:"recommendations"`
}

type LabelInvSummary1968 struct {
	TotalLabels    int `json:"totalUniqueLabels"`
	StandardLabels int `json:"standardLabels"`
	NonStandard    int `json:"nonStandardLabels"`
	TotalPods      int `json:"totalPodsLabeled"`
}

type LabelInvEntry1968 struct {
	Key        string `json:"key"`
	Count      int    `json:"count"`
	IsStandard bool   `json:"isStandard"`
}

var standardLabels1968 = map[string]bool{
	"app": true, "app.kubernetes.io/name": true, "app.kubernetes.io/instance": true,
	"app.kubernetes.io/version": true, "app.kubernetes.io/managed-by": true,
	"app.kubernetes.io/component": true, "app.kubernetes.io/part-of": true,
	"tier": true, "env": true, "environment": true, "version": true,
	"name": true, "kind": true, "chart": true, "heritage": true,
	"release": true, "pod-template-hash": true, "controller-revision-hash": true,
}

func (s *Server) handleLabelInventoryDoc(w http.ResponseWriter, r *http.Request) {
	result := LabelInvResult1968{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	labelCount := make(map[string]int)
	labeledPods := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if len(pod.Labels) > 0 {
			labeledPods++
		}
		for k := range pod.Labels {
			labelCount[k]++
		}
	}

	result.Summary.TotalPods = labeledPods

	for key, count := range labelCount {
		entry := LabelInvEntry1968{
			Key: key, Count: count,
			IsStandard: standardLabels1968[key],
		}
		result.Summary.TotalLabels++

		if entry.IsStandard {
			result.Summary.StandardLabels++
		} else {
			result.Summary.NonStandard++
			result.NonStandard = append(result.NonStandard, entry)
		}
		result.TopLabels = append(result.TopLabels, entry)
	}

	sort.Slice(result.TopLabels, func(i, j int) bool {
		return result.TopLabels[i].Count > result.TopLabels[j].Count
	})
	if len(result.TopLabels) > 30 {
		result.TopLabels = result.TopLabels[:30]
	}

	sort.Slice(result.NonStandard, func(i, j int) bool {
		return result.NonStandard[i].Count > result.NonStandard[j].Count
	})

	// Score based on standard label ratio
	if result.Summary.TotalLabels > 0 {
		ratio := float64(result.Summary.StandardLabels) / float64(result.Summary.TotalLabels)
		if ratio < 0.3 {
			score -= 10
		} else if ratio < 0.5 {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unique labels (%d standard, %d non-standard) across %d pods", result.Summary.TotalLabels, result.Summary.StandardLabels, result.Summary.NonStandard, result.Summary.TotalPods))
	if result.Summary.NonStandard > 10 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d non-standard labels — migrate to app.kubernetes.io/* prefix", result.Summary.NonStandard))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
