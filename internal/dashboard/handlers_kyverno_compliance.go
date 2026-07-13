package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// KyvernoComplianceResult is the Kyverno policy compliance & cluster policy audit.
type KyvernoComplianceResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         KyvernoSummary       `json:"summary"`
	Policies        []KyvernoPolicyEntry `json:"policies"`
	Violations      []KyvernoViolation   `json:"violations"`
	ByNamespace     []KyvernoNSStat      `json:"byNamespace"`
	PodHealth       []KyvernoPodHealth   `json:"podHealth"`
	Issues          []KyvernoIssue       `json:"issues"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// KyvernoSummary aggregates Kyverno policy statistics.
type KyvernoSummary struct {
	KyvernoDetected    bool   `json:"kyvernoDetected"`
	KyvernoVersion     string `json:"kyvernoVersion,omitempty"`
	TotalPolicies      int    `json:"totalPolicies"`
	EnforcePolicies    int    `json:"enforcePolicies"`
	AuditPolicies      int    `json:"auditPolicies"`
	ValidateRules      int    `json:"validateRules"`
	MutateRules        int    `json:"mutateRules"`
	GenerateRules      int    `json:"generateRules"`
	ViolationCount     int    `json:"violationCount"`
	NamespacesWithVios int    `json:"namespacesWithViolations"`
	PodCount           int    `json:"podCount"`
	ReadyPods          int    `json:"readyPods"`
}

// KyvernoPolicyEntry describes one Kyverno ClusterPolicy.
type KyvernoPolicyEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace,omitempty"`
	ValidationRules int    `json:"validationRules"`
	MutationRules   int    `json:"mutationRules"`
	GenerationRules int    `json:"generationRules"`
	Enforcement     string `json:"enforcement"` // Enforce or Audit
	BackgroundScan  string `json:"backgroundScan"`
	Violations      int    `json:"violations"`
	RiskLevel       string `json:"riskLevel"`
}

// KyvernoViolation is a policy violation.
type KyvernoViolation struct {
	Policy    string `json:"policy"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
}

// KyvernoNSStat per-namespace violation stats.
type KyvernoNSStat struct {
	Namespace      string `json:"namespace"`
	ViolationCount int    `json:"violationCount"`
	PolicyCount    int    `json:"policyCount"`
}

// KyvernoPodHealth describes a Kyverno pod's health.
type KyvernoPodHealth struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Image     string `json:"image"`
}

// KyvernoIssue is a detected Kyverno problem.
type KyvernoIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleKyvernoCompliance audits Kyverno policy compliance and cluster policy health.
// GET /api/security/kyverno-compliance
func (s *Server) handleKyvernoCompliance(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &KyvernoComplianceResult{
		ScannedAt: time.Now(),
	}

	// 1. Detect Kyverno pods
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	var kyvernoPods []corev1.Pod
	var kyvernoImage string

	for i := range allPods.Items {
		pod := &allPods.Items[i]
		podName := strings.ToLower(pod.Name)
		isKyverno := false
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			if strings.Contains(podName, "kyverno") || strings.Contains(img, "kyverno") {
				isKyverno = true
				if kyvernoImage == "" {
					kyvernoImage = c.Image
				}
			}
		}
		if isKyverno {
			kyvernoPods = append(kyvernoPods, *pod)
		}
	}

	// 2. Check Kyverno pod health
	readyPods := 0
	for _, pod := range kyvernoPods {
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if ready {
			readyPods++
		}
		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}
		result.PodHealth = append(result.PodHealth, KyvernoPodHealth{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Ready:     ready,
			Restarts:  restarts,
			Image:     kyvernoImage,
		})
		if !ready {
			result.Issues = append(result.Issues, KyvernoIssue{
				Severity: "critical",
				Type:     "kyverno-pod-not-ready",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  "Kyverno pod is not ready — policy enforcement may be impaired",
			})
		}
		if restarts > 3 {
			result.Issues = append(result.Issues, KyvernoIssue{
				Severity: "warning",
				Type:     "kyverno-high-restarts",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Kyverno pod has %d restarts — may indicate instability", restarts),
			})
		}
	}

	// 3. Try to list Kyverno ClusterPolicies using dynamic client
	var policies []KyvernoPolicyEntry
	var allViolations []KyvernoViolation
	enforceCount := 0
	auditCount := 0
	validateRules := 0
	mutateRules := 0
	generateRules := 0
	nsStats := make(map[string]*KyvernoNSStat)

	if rc.restConfig != nil {
		dynClient, err := dynamic.NewForConfig(rc.restConfig)
		if err == nil {
			// Kyverno ClusterPolicies: kyverno.io/v1 or kyverno.io/v2beta1
			versions := []string{"v1", "v2beta1", "v2"}
			for _, ver := range versions {
				gvr := schema.GroupVersionResource{
					Group:    "kyverno.io",
					Version:  ver,
					Resource: "clusterpolicies",
				}
				list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
				if err == nil && list != nil {
					for _, item := range list.Items {
						policy := parseKyvernoPolicy(item)
						policies = append(policies, policy)

						if policy.Enforcement == "Enforce" {
							enforceCount++
						} else {
							auditCount++
						}
						validateRules += policy.ValidationRules
						mutateRules += policy.MutationRules
						generateRules += policy.GenerationRules

						if policy.Violations > 0 {
							result.Issues = append(result.Issues, KyvernoIssue{
								Severity: "warning",
								Type:     "policy-violation",
								Resource: policy.Name,
								Message:  fmt.Sprintf("Policy '%s' has %d violation(s)", policy.Name, policy.Violations),
							})
						}
					}
					break // Found policies with this version, no need to try others
				}
			}

			// Also try namespaced policies (Policy CRD)
			polGVR := schema.GroupVersionResource{
				Group:    "kyverno.io",
				Version:  "v1",
				Resource: "policies",
			}
			nsPolList, err := dynClient.Resource(polGVR).List(ctx, metav1.ListOptions{})
			if err == nil && nsPolList != nil {
				for _, item := range nsPolList.Items {
					policy := parseKyvernoPolicy(item)
					policy.Namespace = item.GetNamespace()
					policies = append(policies, policy)

					if policy.Enforcement == "Enforce" {
						enforceCount++
					} else {
						auditCount++
					}
					validateRules += policy.ValidationRules
					mutateRules += policy.MutationRules
					generateRules += policy.GenerationRules

					if _, ok := nsStats[policy.Namespace]; !ok {
						nsStats[policy.Namespace] = &KyvernoNSStat{Namespace: policy.Namespace}
					}
					nsStats[policy.Namespace].PolicyCount++
				}
			}
		}
	}

	// 4. Check for policy coverage gaps
	if len(kyvernoPods) > 0 && len(policies) == 0 {
		result.Issues = append(result.Issues, KyvernoIssue{
			Severity: "warning",
			Type:     "no-policies",
			Resource: "cluster",
			Message:  "Kyverno is installed but no ClusterPolicies found — define policies for governance enforcement",
		})
	}

	// 5. Check for audit-only policies that should be enforced
	for _, p := range policies {
		if p.Enforcement == "Audit" && p.Violations == 0 {
			result.Issues = append(result.Issues, KyvernoIssue{
				Severity: "info",
				Type:     "audit-only-policy",
				Resource: p.Name,
				Message:  fmt.Sprintf("Policy '%s' is in Audit mode with no violations — consider switching to Enforce", p.Name),
			})
		}
	}

	// Sort policies by risk level
	sort.Slice(policies, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[policies[i].RiskLevel] < riskOrder[policies[j].RiskLevel]
	})

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ViolationCount > result.ByNamespace[j].ViolationCount
	})

	// 6. Generate recommendations
	var recommendations []string
	if len(kyvernoPods) == 0 {
		recommendations = append(recommendations, "Kyverno is not installed — consider installing Kyverno for Kubernetes-native policy enforcement")
	}
	if len(policies) == 0 && len(kyvernoPods) > 0 {
		recommendations = append(recommendations, "No Kyverno policies defined — start with baseline policies (disallow latest tag, require resource limits, disallow privileged containers)")
	}
	if auditCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d policy/policies are in Audit mode — switch to Enforce after validating no false positives", auditCount))
	}
	if len(kyvernoPods) > 0 && readyPods < len(kyvernoPods) {
		recommendations = append(recommendations, fmt.Sprintf("%d/%d Kyverno pod(s) are not ready — check pod logs and configuration", len(kyvernoPods)-readyPods, len(kyvernoPods)))
	}
	if len(allViolations) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d policy violation(s) detected — review and remediate non-compliant resources", len(allViolations)))
	}
	if len(policies) > 0 && validateRules == 0 {
		recommendations = append(recommendations, "No validation rules found — add validate rules to enforce resource standards")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Kyverno is healthy — policies are properly configured and enforced")
	}

	kyvernoDetected := len(kyvernoPods) > 0
	violationCount := len(allViolations)

	result.Policies = policies
	result.Violations = allViolations
	result.Recommendations = recommendations
	result.Summary = KyvernoSummary{
		KyvernoDetected:    kyvernoDetected,
		KyvernoVersion:     extractKyvernoVersion(kyvernoImage),
		TotalPolicies:      len(policies),
		EnforcePolicies:    enforceCount,
		AuditPolicies:      auditCount,
		ValidateRules:      validateRules,
		MutateRules:        mutateRules,
		GenerateRules:      generateRules,
		ViolationCount:     violationCount,
		NamespacesWithVios: len(nsStats),
		PodCount:           len(kyvernoPods),
		ReadyPods:          readyPods,
	}
	result.HealthScore = computeKyvernoHealthScore(result.Summary, len(result.Issues))

	writeJSON(w, result)
}

// parseKyvernoPolicy extracts policy info from an unstructured Kyverno ClusterPolicy.
func parseKyvernoPolicy(item unstructured.Unstructured) KyvernoPolicyEntry {
	policy := KyvernoPolicyEntry{
		Name: item.GetName(),
	}

	// Get spec.failurePolicy (Enforce or Audit)
	if failurePolicy, ok, _ := unstructured.NestedString(item.Object, "spec", "failurePolicy"); ok {
		policy.Enforcement = failurePolicy
	}
	if policy.Enforcement == "" {
		policy.Enforcement = "Audit"
	}

	// Get background scan setting
	if bg, ok, _ := unstructured.NestedBool(item.Object, "spec", "background"); ok {
		if bg {
			policy.BackgroundScan = "enabled"
		} else {
			policy.BackgroundScan = "disabled"
		}
	} else {
		policy.BackgroundScan = "default"
	}

	// Count rules by type
	if rules, ok, _ := unstructured.NestedSlice(item.Object, "spec", "rules"); ok {
		for _, rule := range rules {
			if ruleMap, ok := rule.(map[string]interface{}); ok {
				if _, hasValidate := ruleMap["validate"]; hasValidate {
					policy.ValidationRules++
				}
				if _, hasMutate := ruleMap["mutate"]; hasMutate {
					policy.MutationRules++
				}
				if _, hasGenerate := ruleMap["generate"]; hasGenerate {
					policy.GenerationRules++
				}
			}
		}
	}

	// Count violations from status
	if violations, ok, _ := unstructured.NestedSlice(item.Object, "status", "violations"); ok {
		policy.Violations = len(violations)
	}

	policy.RiskLevel = assessKyvernoPolicyRisk(policy)

	return policy
}

// extractKyvernoVersion extracts version from image string.
func extractKyvernoVersion(image string) string {
	if image == "" {
		return ""
	}
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return ""
	}
	ver := parts[len(parts)-1]
	if idx := strings.Index(ver, "-"); idx != -1 {
		ver = ver[:idx]
	}
	return ver
}

// assessKyvernoPolicyRisk determines risk level of a policy.
func assessKyvernoPolicyRisk(policy KyvernoPolicyEntry) string {
	risk := 0
	if policy.Violations > 0 {
		risk += 2
	}
	if policy.Enforcement == "Audit" {
		risk += 1
	}
	if policy.BackgroundScan == "disabled" {
		risk += 1
	}
	switch {
	case risk >= 3:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computeKyvernoHealthScore computes a 0-100 health score.
func computeKyvernoHealthScore(summary KyvernoSummary, issueCount int) int {
	if !summary.KyvernoDetected {
		return 50 // neutral — not installed
	}
	score := 100
	score -= (summary.PodCount - summary.ReadyPods) * 15
	score -= summary.ViolationCount * 5
	score -= summary.AuditPolicies * 1
	score -= issueCount * 1
	if summary.TotalPolicies == 0 {
		score -= 10 // no policies is a risk
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
