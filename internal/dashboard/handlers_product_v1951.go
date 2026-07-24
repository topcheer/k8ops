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
// v19.51 — Product Dimension (Round 11)
// 1. Helm Release Audit — release health & drift detection
// 2. Ingress Rule Consolidation — duplicate/conflicting rule analysis
// 3. Namespace Lifecycle Tracker — active vs dormant namespaces
// ============================================================

// ---------------------------------------------------------------
// 1. Helm Release Audit
// ---------------------------------------------------------------

type HelmReleaseResult1951 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         HelmReleaseSummary1951 `json:"summary"`
	Releases        []HelmReleaseEntry1951 `json:"releases"`
	Issues          []HelmReleaseIssue1951 `json:"issues"`
	Recommendations []string               `json:"recommendations"`
}

type HelmReleaseSummary1951 struct {
	TotalReleases    int `json:"totalReleases"`
	DeployedReleases int `json:"deployedReleases"`
	FailedReleases   int `json:"failedReleases"`
	PendingReleases  int `json:"pendingReleases"`
	SecretBackends   int `json:"secretBackends"`
	ConfigBackends   int `json:"configBackends"`
}

type HelmReleaseEntry1951 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	Backend   string `json:"storageBackend"`
}

type HelmReleaseIssue1951 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleHelmReleaseAuditV2(w http.ResponseWriter, r *http.Request) {
	result := HelmReleaseResult1951{ScannedAt: time.Now()}
	score := 100

	// Helm stores releases as secrets or configmaps with labels
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{
		LabelSelector: "owner=helm",
	})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{
		LabelSelector: "owner=helm",
	})

	// Track latest release per name
	releaseMap := make(map[string]*HelmReleaseEntry1951)
	for _, sec := range secretList.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		result.Summary.SecretBackends++
		name := sec.Labels["name"]
		if name == "" {
			continue
		}
		status := sec.Labels["status"]
		version := sec.Labels["version"]
		key := fmt.Sprintf("%s/%s", sec.Namespace, name)
		if releaseMap[key] == nil || version > releaseMap[key].Version {
			releaseMap[key] = &HelmReleaseEntry1951{
				Name: name, Namespace: sec.Namespace,
				Version: version, Status: status, Backend: "secret",
			}
		}
	}
	for _, cm := range cmList.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		result.Summary.ConfigBackends++
		name := cm.Labels["name"]
		if name == "" {
			continue
		}
		status := cm.Labels["status"]
		version := cm.Labels["version"]
		key := fmt.Sprintf("%s/%s", cm.Namespace, name)
		if releaseMap[key] == nil || version > releaseMap[key].Version {
			releaseMap[key] = &HelmReleaseEntry1951{
				Name: name, Namespace: cm.Namespace,
				Version: version, Status: status, Backend: "configmap",
			}
		}
	}

	for _, rel := range releaseMap {
		result.Summary.TotalReleases++
		result.Releases = append(result.Releases, *rel)

		switch strings.ToLower(rel.Status) {
		case "deployed":
			result.Summary.DeployedReleases++
		case "failed":
			result.Summary.FailedReleases++
			result.Issues = append(result.Issues, HelmReleaseIssue1951{
				Name: rel.Name, Namespace: rel.Namespace,
				IssueType: "failed-release", Severity: "high",
				Detail: fmt.Sprintf("Release %s in failed state", rel.Name),
			})
			score -= 5
		case "pending-install", "pending-upgrade", "pending-rollback":
			result.Summary.PendingReleases++
			result.Issues = append(result.Issues, HelmReleaseIssue1951{
				Name: rel.Name, Namespace: rel.Namespace,
				IssueType: "pending-release", Severity: "medium",
				Detail: fmt.Sprintf("Release %s stuck in %s", rel.Name, rel.Status),
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.FailedReleases > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d failed Helm releases — rollback or fix", result.Summary.FailedReleases))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d releases (%d deployed, %d failed)", result.Summary.TotalReleases, result.Summary.DeployedReleases, result.Summary.FailedReleases))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Ingress Rule Consolidation
// ---------------------------------------------------------------

type IngressConsolidResult1951 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         IngressConsolidSummary1951 `json:"summary"`
	Ingresses       []IngressConsolidEntry1951 `json:"ingresses"`
	Duplicates      []IngressDuplicate1951     `json:"duplicates"`
	Recommendations []string                   `json:"recommendations"`
}

type IngressConsolidSummary1951 struct {
	TotalIngresses int `json:"totalIngresses"`
	WithTLS        int `json:"withTLS"`
	WithoutTLS     int `json:"withoutTLS"`
	HostCount      int `json:"totalHosts"`
	PathCount      int `json:"totalPaths"`
	DuplicateHosts int `json:"duplicateHosts"`
}

type IngressConsolidEntry1951 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	HostCount int    `json:"hostCount"`
	PathCount int    `json:"pathCount"`
	HasTLS    bool   `json:"hasTLS"`
}

type IngressDuplicate1951 struct {
	Host     string `json:"host"`
	IngA     string `json:"ingressA"`
	IngB     string `json:"ingressB"`
	Severity string `json:"severity"`
}

func (s *Server) handleIngressConsolidation(w http.ResponseWriter, r *http.Request) {
	result := IngressConsolidResult1951{ScannedAt: time.Now()}
	score := 100

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	hostOwners := make(map[string][]string) // host -> list of "ns/ingress"
	for _, ing := range ingList.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		result.Summary.TotalIngresses++

		hasTLS := len(ing.Spec.TLS) > 0
		hostCount := len(ing.Spec.Rules)
		pathCount := 0
		for _, rule := range ing.Spec.Rules {
			if rule.IngressRuleValue.HTTP != nil {
				pathCount += len(rule.IngressRuleValue.HTTP.Paths)
			}
			host := rule.Host
			if host != "" {
				hostOwners[host] = append(hostOwners[host], fmt.Sprintf("%s/%s", ing.Namespace, ing.Name))
				result.Summary.HostCount++
			}
		}
		result.Summary.PathCount += pathCount

		if hasTLS {
			result.Summary.WithTLS++
		} else {
			result.Summary.WithoutTLS++
			score -= 2
		}

		result.Ingresses = append(result.Ingresses, IngressConsolidEntry1951{
			Name: ing.Name, Namespace: ing.Namespace,
			HostCount: hostCount, PathCount: pathCount, HasTLS: hasTLS,
		})
	}

	// Detect duplicate hosts
	for host, owners := range hostOwners {
		if len(owners) > 1 {
			result.Summary.DuplicateHosts++
			severity := "medium"
			if len(owners) > 2 {
				severity = "high"
			}
			result.Duplicates = append(result.Duplicates, IngressDuplicate1951{
				Host: host, IngA: owners[0], IngB: owners[1], Severity: severity,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutTLS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses without TLS — add cert-manager", result.Summary.WithoutTLS))
	}
	if result.Summary.DuplicateHosts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d duplicate hosts — consolidate ingress rules", result.Summary.DuplicateHosts))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ingresses, %d hosts, %d paths", result.Summary.TotalIngresses, result.Summary.HostCount, result.Summary.PathCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Lifecycle Tracker
// ---------------------------------------------------------------

type NSLifecycleResult1951 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         NSLifecycleSummary1951 `json:"summary"`
	DormantNS       []NSLifecycleEntry1951 `json:"dormantNamespaces"`
	ActiveNS        []NSLifecycleEntry1951 `json:"activeNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

type NSLifecycleSummary1951 struct {
	TotalNamespaces int    `json:"totalNamespaces"`
	ActiveNS        int    `json:"activeNamespaces"`
	DormantNS       int    `json:"dormantNamespaces"`
	NewestNSAge     string `json:"newestNSAge"`
	OldestNSAge     string `json:"oldestNSAge"`
}

type NSLifecycleEntry1951 struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	PodCount int    `json:"podCount"`
	Age      string `json:"age"`
	Category string `json:"category"`
}

func (s *Server) handleNSLifecycleTracker(w http.ResponseWriter, r *http.Request) {
	result := NSLifecycleResult1951{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podsPerNS[pod.Namespace]++
		}
	}

	var oldestTime, newestTime time.Time
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		podCount := podsPerNS[ns.Name]
		age := time.Since(ns.CreationTimestamp.Time)
		ageStr := fmt.Sprintf("%.0fd", age.Hours()/24)

		if oldestTime.IsZero() || ns.CreationTimestamp.Time.Before(oldestTime) {
			oldestTime = ns.CreationTimestamp.Time
		}
		if newestTime.IsZero() || ns.CreationTimestamp.Time.After(newestTime) {
			newestTime = ns.CreationTimestamp.Time
		}

		entry := NSLifecycleEntry1951{
			Name: ns.Name, Status: string(ns.Status.Phase),
			PodCount: podCount, Age: ageStr,
		}

		if podCount == 0 {
			entry.Category = "dormant"
			result.Summary.DormantNS++
			result.DormantNS = append(result.DormantNS, entry)
			score -= 2
		} else {
			entry.Category = "active"
			result.Summary.ActiveNS++
			result.ActiveNS = append(result.ActiveNS, entry)
		}
	}

	if !oldestTime.IsZero() {
		result.Summary.OldestNSAge = fmt.Sprintf("%.0fd", time.Since(oldestTime).Hours()/24)
	}
	if !newestTime.IsZero() {
		result.Summary.NewestNSAge = fmt.Sprintf("%.0fd", time.Since(newestTime).Hours()/24)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DormantNS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d dormant namespaces (0 pods) — archive or delete", result.Summary.DormantNS))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces (%d active, %d dormant)", result.Summary.TotalNamespaces, result.Summary.ActiveNS, result.Summary.DormantNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
