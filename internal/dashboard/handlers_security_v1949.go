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
// v19.49 — Security Dimension (Round 11)
// 1. Pod Forensics Snapshot — suspicious pod indicator collection
// 2. Egress Traffic Exposure — external destination audit
// 3. Service Account Token Age — stale token rotation tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Forensics Snapshot
// ---------------------------------------------------------------

type PodForensicsResult1949 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PodForensicsSummary1949 `json:"summary"`
	SuspiciousPods  []PodForensicsEntry1949 `json:"suspiciousPods"`
	Recommendations []string                `json:"recommendations"`
}

type PodForensicsSummary1949 struct {
	TotalPods        int `json:"totalPods"`
	SuspiciousCount  int `json:"suspiciousCount"`
	WithHostNetwork  int `json:"withHostNetwork"`
	WithHostPID      int `json:"withHostPID"`
	WithHostPath     int `json:"withHostPath"`
	WithPrivileged   int `json:"withPrivileged"`
	WithCapSysAdmin  int `json:"withCapSysAdmin"`
	WithWritableRoot int `json:"withWritableRootFS"`
}

type PodForensicsEntry1949 struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Indicators []string `json:"indicators"`
	Severity   string   `json:"severity"`
}

func (s *Server) handlePodForensicsSnap(w http.ResponseWriter, r *http.Request) {
	result := PodForensicsResult1949{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		var indicators []string
		severity := "low"

		if pod.Spec.HostNetwork {
			indicators = append(indicators, "hostNetwork")
			result.Summary.WithHostNetwork++
			severity = "high"
		}
		if pod.Spec.HostPID {
			indicators = append(indicators, "hostPID")
			result.Summary.WithHostPID++
			severity = "high"
		}

		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				indicators = append(indicators, fmt.Sprintf("hostPath:%s", vol.HostPath.Path))
				result.Summary.WithHostPath++
				if severity != "high" {
					severity = "medium"
				}
			}
		}

		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					indicators = append(indicators, fmt.Sprintf("privileged:%s", c.Name))
					result.Summary.WithPrivileged++
					severity = "critical"
				}
				if c.SecurityContext.Capabilities != nil {
					for _, cap := range c.SecurityContext.Capabilities.Add {
						if string(cap) == "SYS_ADMIN" {
							indicators = append(indicators, fmt.Sprintf("CAP_SYS_ADMIN:%s", c.Name))
							result.Summary.WithCapSysAdmin++
							if severity != "critical" {
								severity = "high"
							}
						}
					}
				}
				if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
					result.Summary.WithWritableRoot++
				}
			} else {
				result.Summary.WithWritableRoot++
			}
		}

		if len(indicators) > 0 {
			result.Summary.SuspiciousCount++
			result.SuspiciousPods = append(result.SuspiciousPods, PodForensicsEntry1949{
				Name: pod.Name, Namespace: pod.Namespace,
				Indicators: indicators, Severity: severity,
			})
			switch severity {
			case "critical":
				score -= 10
			case "high":
				score -= 5
			case "medium":
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithPrivileged > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d privileged containers — remove immediately", result.Summary.WithPrivileged))
	}
	if result.Summary.WithCapSysAdmin > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with CAP_SYS_ADMIN — equivalent to root", result.Summary.WithCapSysAdmin))
	}
	if result.Summary.WithHostNetwork > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with hostNetwork — network isolation bypass", result.Summary.WithHostNetwork))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Egress Traffic Exposure
// ---------------------------------------------------------------

type EgressResult1949 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         EgressSummary1949        `json:"summary"`
	Destinations    []EgressEntry1949        `json:"destinations"`
	Unrestricted    []EgressUnrestricted1949 `json:"unrestrictedNS"`
	Recommendations []string                 `json:"recommendations"`
}

type EgressSummary1949 struct {
	TotalNamespaces      int `json:"totalNamespaces"`
	WithEgressPolicy     int `json:"namespacesWithEgressPolicy"`
	WithoutEgress        int `json:"namespacesWithoutEgress"`
	TotalEgressRules     int `json:"totalEgressRules"`
	DenyAllEgress        int `json:"denyAllEgressCount"`
	ExternalDestinations int `json:"externalDestinations"`
}

type EgressEntry1949 struct {
	Namespace  string `json:"namespace"`
	NetPolName string `json:"netPolName"`
	HasCIDR    bool   `json:"hasCIDRBlock"`
	AllowsAll  bool   `json:"allowsAllEgress"`
}

type EgressUnrestricted1949 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Severity  string `json:"severity"`
}

func (s *Server) handleEgressExposure(w http.ResponseWriter, r *http.Request) {
	result := EgressResult1949{ScannedAt: time.Now()}
	score := 100

	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count egress policies per namespace
	hasEgress := make(map[string]bool)
	podCountNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCountNS[pod.Namespace]++
		}
	}

	for _, np := range npList.Items {
		if isSystemNamespace(np.Namespace) {
			continue
		}
		hasEgress[np.Namespace] = true
		result.Summary.TotalEgressRules++

		allowsAll := len(np.Spec.Egress) == 0 && np.Spec.PolicyTypes != nil
		hasCIDR := false
		for _, egr := range np.Spec.Egress {
			for _, dst := range egr.To {
				if dst.IPBlock != nil {
					hasCIDR = true
					result.Summary.ExternalDestinations++
				}
			}
		}

		if allowsAll {
			result.Summary.DenyAllEgress++
		}

		result.Destinations = append(result.Destinations, EgressEntry1949{
			Namespace: np.Namespace, NetPolName: np.Name,
			HasCIDR: hasCIDR, AllowsAll: allowsAll,
		})
	}

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		if hasEgress[ns.Name] {
			result.Summary.WithEgressPolicy++
		} else {
			result.Summary.WithoutEgress++
			pods := podCountNS[ns.Name]
			severity := "low"
			if pods > 10 {
				severity = "medium"
			}
			result.Unrestricted = append(result.Unrestricted, EgressUnrestricted1949{
				Namespace: ns.Name, PodCount: pods, Severity: severity,
			})
			if severity == "medium" {
				score -= 3
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

	if result.Summary.WithoutEgress > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without egress restrictions — add default-deny egress", result.Summary.WithoutEgress))
	}
	if result.Summary.ExternalDestinations > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d external CIDR destinations — audit for data exfiltration risk", result.Summary.ExternalDestinations))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Service Account Token Age
// -----------------------------------------------------------

type SATokenAgeResult1949 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SATokenAgeSummary1949 `json:"summary"`
	OldTokens       []SATokenEntry1949    `json:"oldTokens"`
	ByNS            []SATokenNS1949       `json:"byNamespace"`
	Recommendations []string              `json:"recommendations"`
}

type SATokenAgeSummary1949 struct {
	TotalSAs        int     `json:"totalServiceAccounts"`
	WithTokenSecret int     `json:"withTokenSecret"`
	OlderThan90d    int     `json:"olderThan90Days"`
	OlderThan180d   int     `json:"olderThan180Days"`
	MaxAgeDays      float64 `json:"maxAgeDays"`
	ProjectedTokens int     `json:"projectedTokenCount"`
}

type SATokenEntry1949 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	AgeDays   float64 `json:"ageDays"`
	Severity  string  `json:"severity"`
}

type SATokenNS1949 struct {
	Namespace string `json:"namespace"`
	SACount   int    `json:"saCount"`
	OldCount  int    `json:"oldTokenCount"`
}

func (s *Server) handleSATokenAgeV2(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult1949{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]*SATokenNS1949)

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		if isSystemNamespace(sa.Namespace) {
			continue
		}
		result.Summary.TotalSAs++

		ageDays := time.Since(sa.CreationTimestamp.Time).Hours() / 24
		if ageDays > result.Summary.MaxAgeDays {
			result.Summary.MaxAgeDays = ageDays
		}

		hasToken := len(sa.Secrets) > 0
		if hasToken {
			result.Summary.WithTokenSecret++
		}
		// Check if using projected tokens (no secret-based token)
		if !hasToken {
			result.Summary.ProjectedTokens++
		}

		if ageDays > 90 {
			result.Summary.OlderThan90d++
			severity := "low"
			if ageDays > 365 {
				severity = "high"
			} else if ageDays > 180 {
				severity = "medium"
			}
			if ageDays > 180 {
				result.Summary.OlderThan180d++
				result.OldTokens = append(result.OldTokens, SATokenEntry1949{
					Name: sa.Name, Namespace: sa.Namespace,
					AgeDays: ageDays, Severity: severity,
				})
				if severity == "high" {
					score -= 3
				} else {
					score -= 1
				}
			}
		}

		if nsStats[sa.Namespace] == nil {
			nsStats[sa.Namespace] = &SATokenNS1949{Namespace: sa.Namespace}
		}
		nsStats[sa.Namespace].SACount++
		if ageDays > 180 {
			nsStats[sa.Namespace].OldCount++
		}
	}

	for _, ns := range nsStats {
		result.ByNS = append(result.ByNS, *ns)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OlderThan180d > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs older than 180 days — rotate or use projected tokens", result.Summary.OlderThan180d))
	}
	if result.Summary.WithTokenSecret > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs with secret-based tokens — migrate to projected volume tokens", result.Summary.WithTokenSecret))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs (%d projected, %d secret-based)", result.Summary.TotalSAs, result.Summary.ProjectedTokens, result.Summary.WithTokenSecret))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// helper to suppress unused import
var _ = strings.Contains
