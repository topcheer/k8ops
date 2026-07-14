package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PolicyDriftResult is the security policy drift & baseline configuration audit.
type PolicyDriftResult struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	Summary         PolicyDriftSummary       `json:"summary"`
	PSALabelGaps    []PolicyDriftPSAGap      `json:"psaLabelGaps"`
	DefaultRoleRisk []PolicyDriftRoleRisk    `json:"defaultRoleRisk"`
	APIFlags        []PolicyDriftAPIFlag     `json:"apiFlags"`
	NetworkBaseline []PolicyDriftNetBaseline `json:"networkBaseline"`
	Recommendations []string                 `json:"recommendations"`
	HealthScore     int                      `json:"healthScore"`
}

// PolicyDriftSummary aggregates policy drift statistics.
type PolicyDriftSummary struct {
	TotalNamespaces     int `json:"totalNamespaces"`
	MissingPSALabels    int `json:"missingPSALabels"`    // no PSA enforce labels
	InconsistentPSA     int `json:"inconsistentPSA"`     // PSA labels mismatch across namespace
	DefaultRoleBindings int `json:"defaultRoleBindings"` // cluster-admin or privileged bindings to default SAs
	UnsafeAPIFlags      int `json:"unsafeAPIFlags"`      // API server flags that disable security
	NamespacesNoNetPol  int `json:"namespacesNoNetPol"`  // no default deny network policy
	DriftDetected       int `json:"driftDetected"`       // total drift items
}

// PolicyDriftPSAGap describes a namespace missing PSA labels.
type PolicyDriftPSAGap struct {
	Namespace     string `json:"namespace"`
	CurrentLevel  string `json:"currentLevel"` // privileged, baseline, restricted, or empty
	ExpectedLevel string `json:"expectedLevel"`
	HasEnforce    bool   `json:"hasEnforce"`
	HasAudit      bool   `json:"hasAudit"`
	HasWarn       bool   `json:"hasWarn"`
	Severity      string `json:"severity"`
}

// PolicyDriftRoleRisk describes risky default role bindings.
type PolicyDriftRoleRisk struct {
	Namespace   string `json:"namespace"`
	RoleName    string `json:"roleName"`
	RoleKind    string `json:"roleKind"` // ClusterRole or Role
	SubjectName string `json:"subjectName"`
	SubjectKind string `json:"subjectKind"` // ServiceAccount, User, Group
	Verb        string `json:"verb"`        // cluster-admin, privileged, etc.
	Severity    string `json:"severity"`
}

// PolicyDriftAPIFlag describes an API server security flag drift.
type PolicyDriftAPIFlag struct {
	Flag          string `json:"flag"`
	CurrentValue  string `json:"currentValue"`
	ExpectedValue string `json:"expectedValue"`
	Safe          bool   `json:"safe"`
	Description   string `json:"description"`
	Severity      string `json:"severity"`
}

// PolicyDriftNetBaseline describes network policy baseline drift.
type PolicyDriftNetBaseline struct {
	Namespace      string `json:"namespace"`
	HasDefaultDeny bool   `json:"hasDefaultDeny"`
	NetPolCount    int    `json:"netPolCount"`
	WorkloadCount  int    `json:"workloadCount"`
	Severity       string `json:"severity"`
}

// handlePolicyDrift audits security policy drift & baseline configuration.
// GET /api/security/policy-drift
func (s *Server) handlePolicyDrift(w http.ResponseWriter, r *http.Request) {
	result := PolicyDriftResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Check PSA labels on all namespaces
	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list namespaces: %v", err))
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":        true,
		"kube-public":        true,
		"kube-node-lease":    true,
		"default":            true,
		"k8ops-system":       true,
		"local-path-storage": true,
		"ingress-nginx":      true,
		"monitoring":         true,
		"metallb-system":     true,
		"cert-manager":       true,
	}

	totalNS := 0
	missingPSA := 0
	inconsistentPSA := 0

	for _, ns := range namespaces.Items {
		if systemNamespaces[ns.Name] {
			continue
		}
		totalNS++

		enforce := ns.Labels["pod-security.kubernetes.io/enforce"]
		audit := ns.Labels["pod-security.kubernetes.io/audit"]
		warn := ns.Labels["pod-security.kubernetes.io/warn"]
		enforceVersion := ns.Labels["pod-security.kubernetes.io/enforce-version"]

		expectedLevel := "baseline"

		if enforce == "" {
			missingPSA++
			result.PSALabelGaps = append(result.PSALabelGaps, PolicyDriftPSAGap{
				Namespace:     ns.Name,
				CurrentLevel:  enforce,
				ExpectedLevel: expectedLevel,
				HasEnforce:    false,
				HasAudit:      audit != "",
				HasWarn:       warn != "",
				Severity:      "high",
			})
		} else if enforce == "privileged" {
			inconsistentPSA++
			result.PSALabelGaps = append(result.PSALabelGaps, PolicyDriftPSAGap{
				Namespace:     ns.Name,
				CurrentLevel:  enforce,
				ExpectedLevel: expectedLevel,
				HasEnforce:    true,
				HasAudit:      audit != "",
				HasWarn:       warn != "",
				Severity:      "critical",
			})
		}

		// Check if enforce-version is outdated
		if enforce != "" && enforceVersion == "" {
			inconsistentPSA++
			result.PSALabelGaps = append(result.PSALabelGaps, PolicyDriftPSAGap{
				Namespace:     ns.Name,
				CurrentLevel:  enforce,
				ExpectedLevel: expectedLevel,
				HasEnforce:    true,
				HasAudit:      audit != "",
				HasWarn:       warn != "",
				Severity:      "low",
			})
		}

		_ = warn
	}

	// 2. Check default role bindings for overprivileged access
	roleBindings, err := rc.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, rb := range roleBindings.Items {
			roleRef := rb.RoleRef.Name
			roleKind := rb.RoleRef.Kind

			isRisky := false
			severity := "medium"
			if roleKind == "ClusterRole" {
				switch roleRef {
				case "cluster-admin":
					isRisky = true
					severity = "critical"
				case "admin":
					isRisky = true
					severity = "high"
				case "edit":
					isRisky = true
					severity = "medium"
				}
			}

			if !isRisky {
				continue
			}

			for _, subj := range rb.Subjects {
				if subj.Kind == "ServiceAccount" && (subj.Name == "default" || strings.HasSuffix(subj.Name, "-default")) {
					result.DefaultRoleRisk = append(result.DefaultRoleRisk, PolicyDriftRoleRisk{
						Namespace:   rb.Namespace,
						RoleName:    roleRef,
						RoleKind:    roleKind,
						SubjectName: subj.Name,
						SubjectKind: subj.Kind,
						Verb:        roleRef,
						Severity:    severity,
					})
				}
			}
		}
	}

	clusterRoleBindings, err := rc.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, crb := range clusterRoleBindings.Items {
			roleRef := crb.RoleRef.Name
			if roleRef != "cluster-admin" {
				continue
			}
			for _, subj := range crb.Subjects {
				if subj.Kind == "ServiceAccount" {
					result.DefaultRoleRisk = append(result.DefaultRoleRisk, PolicyDriftRoleRisk{
						Namespace:   subj.Namespace,
						RoleName:    roleRef,
						RoleKind:    "ClusterRole",
						SubjectName: subj.Name,
						SubjectKind: subj.Kind,
						Verb:        roleRef,
						Severity:    "critical",
					})
				}
			}
		}
	}

	// 3. Check network policy baseline (default deny)
	netpols, err := rc.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsNetPolMap := make(map[string]int)
		nsDefaultDeny := make(map[string]bool)
		for _, np := range netpols.Items {
			nsNetPolMap[np.Namespace]++
			// Check if it's a default deny policy
			if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
				// Empty selector = all pods in namespace
				hasDenyAll := false
				for _, rule := range np.Spec.Ingress {
					if len(rule.From) == 0 {
						hasDenyAll = true
					}
				}
				for _, rule := range np.Spec.Egress {
					if len(rule.To) == 0 {
						hasDenyAll = true
					}
				}
				if hasDenyAll || (len(np.Spec.PolicyTypes) > 0) {
					nsDefaultDeny[np.Namespace] = true
				}
			}
		}

		// Count workloads per namespace
		pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
		nsWorkloadCount := make(map[string]int)
		if err == nil {
			for _, pod := range pods.Items {
				if systemNamespaces[pod.Namespace] {
					continue
				}
				if pod.OwnerReferences != nil && len(pod.OwnerReferences) > 0 {
					if pod.OwnerReferences[0].Kind == "ReplicaSet" || pod.OwnerReferences[0].Kind == "Deployment" || pod.OwnerReferences[0].Kind == "StatefulSet" || pod.OwnerReferences[0].Kind == "DaemonSet" {
						nsWorkloadCount[pod.Namespace]++
					}
				}
			}
		}

		for _, ns := range namespaces.Items {
			if systemNamespaces[ns.Name] {
				continue
			}
			if nsWorkloadCount[ns.Name] == 0 {
				continue
			}
			hasDD := nsDefaultDeny[ns.Name]
			polCount := nsNetPolMap[ns.Name]
			if !hasDD {
				result.NetworkBaseline = append(result.NetworkBaseline, PolicyDriftNetBaseline{
					Namespace:      ns.Name,
					HasDefaultDeny: false,
					NetPolCount:    polCount,
					WorkloadCount:  nsWorkloadCount[ns.Name],
					Severity:       "high",
				})
			} else if polCount < 2 {
				result.NetworkBaseline = append(result.NetworkBaseline, PolicyDriftNetBaseline{
					Namespace:      ns.Name,
					HasDefaultDeny: true,
					NetPolCount:    polCount,
					WorkloadCount:  nsWorkloadCount[ns.Name],
					Severity:       "low",
				})
			}
		}
	}

	// 4. Check API server security flags via kube-system config
	// Check if anonymous auth is enabled via configmaps or known flags
	cm, err := rc.clientset.CoreV1().ConfigMaps("kube-system").Get(r.Context(), "extension-apiserver-authentication", metav1.GetOptions{})
	if err == nil && cm != nil {
		anonAuth := cm.Data["requestheader-allowed-names"]
		if anonAuth == "" {
			result.APIFlags = append(result.APIFlags, PolicyDriftAPIFlag{
				Flag:          "anonymous-auth",
				CurrentValue:  "unknown",
				ExpectedValue: "false",
				Safe:          true,
				Description:   "Anonymous auth status could not be determined from configmap",
				Severity:      "info",
			})
		}
	}

	// Check for kube-system namespaces with privileged PSA
	for _, ns := range namespaces.Items {
		if ns.Name == "kube-system" || ns.Name == "kube-public" || ns.Name == "kube-node-lease" {
			enforce := ns.Labels["pod-security.kubernetes.io/enforce"]
			if enforce != "privileged" && enforce != "" {
				result.APIFlags = append(result.APIFlags, PolicyDriftAPIFlag{
					Flag:          fmt.Sprintf("psa-enforce-%s", ns.Name),
					CurrentValue:  enforce,
					ExpectedValue: "privileged",
					Safe:          true,
					Description:   fmt.Sprintf("System namespace %s should enforce privileged level", ns.Name),
					Severity:      "low",
				})
			}
		}
	}

	// Sort results
	sort.Slice(result.PSALabelGaps, func(i, j int) bool {
		return result.PSALabelGaps[i].Severity > result.PSALabelGaps[j].Severity
	})
	sort.Slice(result.DefaultRoleRisk, func(i, j int) bool {
		return result.DefaultRoleRisk[i].Severity > result.DefaultRoleRisk[j].Severity
	})
	sort.Slice(result.NetworkBaseline, func(i, j int) bool {
		return result.NetworkBaseline[i].Severity > result.NetworkBaseline[j].Severity
	})

	// Build summary
	result.Summary = PolicyDriftSummary{
		TotalNamespaces:     totalNS,
		MissingPSALabels:    missingPSA,
		InconsistentPSA:     inconsistentPSA,
		DefaultRoleBindings: len(result.DefaultRoleRisk),
		UnsafeAPIFlags:      countUnsafeFlags(result.APIFlags),
		NamespacesNoNetPol:  countNoNetPol(result.NetworkBaseline),
		DriftDetected:       len(result.PSALabelGaps) + len(result.DefaultRoleRisk) + len(result.APIFlags) + len(result.NetworkBaseline),
	}

	// Recommendations
	if missingPSA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Add PSA enforce labels to %d namespaces (recommended: baseline or restricted)", missingPSA))
	}
	if inconsistentPSA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Fix inconsistent PSA labels on %d namespaces (avoid privileged enforce)", inconsistentPSA))
	}
	if len(result.DefaultRoleRisk) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Review %d high-privilege role bindings to default ServiceAccounts", len(result.DefaultRoleRisk)))
	}
	if result.Summary.NamespacesNoNetPol > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Add default-deny network policies to %d namespaces", result.Summary.NamespacesNoNetPol))
	}

	// Health score: start from 100, deduct per drift
	score := 100
	score -= missingPSA * 5
	score -= inconsistentPSA * 10
	for _, dr := range result.DefaultRoleRisk {
		switch dr.Severity {
		case "critical":
			score -= 15
		case "high":
			score -= 8
		case "medium":
			score -= 3
		}
	}
	score -= result.Summary.NamespacesNoNetPol * 3
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

func countUnsafeFlags(flags []PolicyDriftAPIFlag) int {
	count := 0
	for _, f := range flags {
		if !f.Safe {
			count++
		}
	}
	return count
}

func countNoNetPol(baselines []PolicyDriftNetBaseline) int {
	count := 0
	for _, b := range baselines {
		if !b.HasDefaultDeny {
			count++
		}
	}
	return count
}
