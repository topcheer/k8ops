package dashboard

import (
	"context"
	"encoding/json"
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

// OPAComplianceResult is the OPA/Gatekeeper policy compliance audit.
type OPAComplianceResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         OPAComplianceSummary `json:"summary"`
	Constraints     []ConstraintEntry    `json:"constraints"`
	Violations      []OPAViolation       `json:"violations"`
	ByNamespace     []OPANSStat          `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

// OPAComplianceSummary aggregates OPA/Gatekeeper compliance statistics.
type OPAComplianceSummary struct {
	HasGatekeeper            bool `json:"hasGatekeeper"`
	HasKyverno               bool `json:"hasKyverno"`
	TotalConstraints         int  `json:"totalConstraints"`
	ViolationCount           int  `json:"violationCount"`
	EnforcedCount            int  `json:"enforcedCount"` // constraints in enforce mode
	AuditMode                int  `json:"auditMode"`     // constraints in audit mode
	NamespacesWithViolations int  `json:"namespacesWithViolations"`
	HealthScore              int  `json:"healthScore"`
}

// ConstraintEntry describes one OPA Gatekeeper constraint.
type ConstraintEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind"`        // K8sRequiredLabels, etc.
	Enforcement string `json:"enforcement"` // enforce, audit
	Violations  int    `json:"violations"`
}

// OPAViolation is a policy violation.
type OPAViolation struct {
	Kind      string `json:"kind"`     // constraint kind
	Resource  string `json:"resource"` // resource name
	Namespace string `json:"namespace"`
	Message   string `json:"message"`
	Severity  string `json:"severity"`
}

// OPANSStat shows violations per namespace.
type OPANSStat struct {
	Namespace      string `json:"namespace"`
	ViolationCount int    `json:"violationCount"`
}

// opaComplianceAuditCore performs the OPA/Gatekeeper compliance audit (testable).
func opaComplianceAuditCore(
	gatekeeperPods []corev1.Pod,
	kyvernoPods []corev1.Pod,
	constraints []unstructured.Unstructured,
	violationPods []corev1.Pod,
) OPAComplianceResult {
	result := OPAComplianceResult{
		ScannedAt: time.Now(),
	}

	result.Summary.HasGatekeeper = len(gatekeeperPods) > 0
	result.Summary.HasKyverno = len(kyvernoPods) > 0

	// Analyze constraints
	nsStats := make(map[string]*OPANSStat)

	for i := range constraints {
		c := &constraints[i]
		name := c.GetName()
		ns := c.GetNamespace()
		kind := c.GetKind()

		entry := ConstraintEntry{
			Name:      name,
			Namespace: ns,
			Kind:      kind,
		}

		// Check enforcement action (spec.enforcementAction)
		enforcement, found, _ := unstructured.NestedString(c.Object, "spec", "enforcementAction")
		if !found || enforcement == "" {
			enforcement = "audit"
		}
		entry.Enforcement = enforcement

		if enforcement == "deny" || enforcement == "enforce" {
			result.Summary.EnforcedCount++
		} else {
			result.Summary.AuditMode++
		}

		// Count violations from status
		violations, found, _ := unstructured.NestedSlice(c.Object, "status", "violations")
		violationCount := 0
		if found {
			violationCount = len(violations)
			for _, v := range violations {
				vMap, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				resName, _ := vMap["name"].(string)
				resNS, _ := vMap["namespace"].(string)
				msg, _ := vMap["message"].(string)
				if resNS == "" {
					resNS = ns
				}

				severity := "medium"
				if enforcement == "deny" || enforcement == "enforce" {
					severity = "high"
				}

				result.Violations = append(result.Violations, OPAViolation{
					Kind:      kind,
					Resource:  resName,
					Namespace: resNS,
					Message:   msg,
					Severity:  severity,
				})

				if _, ok := nsStats[resNS]; !ok {
					nsStats[resNS] = &OPANSStat{Namespace: resNS}
				}
				nsStats[resNS].ViolationCount++
			}
		}
		entry.Violations = violationCount
		result.Summary.TotalConstraints++
		result.Summary.ViolationCount += violationCount

		result.Constraints = append(result.Constraints, entry)
	}

	// Count violation pods (Gatekeeper audit pod labels: gatekeeper.sh/operation=audit)
	for i := range violationPods {
		pod := &violationPods[i]
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &OPANSStat{Namespace: ns}
		}
	}

	result.Summary.NamespacesWithViolations = len(nsStats)

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ViolationCount > result.ByNamespace[j].ViolationCount
	})

	sort.Slice(result.Constraints, func(i, j int) bool {
		return result.Constraints[i].Violations > result.Constraints[j].Violations
	})

	result.Summary.HealthScore = opaComplianceScore(result.Summary)
	result.Recommendations = opaComplianceRecommendations(result.Summary)

	return result
}

// opaComplianceScore calculates health score.
func opaComplianceScore(s OPAComplianceSummary) int {
	if !s.HasGatekeeper && !s.HasKyverno {
		return 50 // neutral — no policy engine installed
	}
	base := 100
	base -= s.ViolationCount * 5
	base -= s.NamespacesWithViolations * 3
	// Bonus for having enforced constraints
	if s.EnforcedCount > 0 && s.ViolationCount == 0 {
		base += 5
	}
	if base < 0 {
		base = 0
	}
	if base > 100 {
		base = 100
	}
	return base
}

// opaComplianceRecommendations generates recommendations.
func opaComplianceRecommendations(s OPAComplianceSummary) []string {
	var recs []string
	if !s.HasGatekeeper && !s.HasKyverno {
		recs = append(recs, "no policy engine (Gatekeeper/Kyverno) detected — install OPA Gatekeeper or Kyverno for policy-as-code compliance")
		return recs
	}
	if s.ViolationCount > 0 {
		recs = append(recs, fmt.Sprintf("%d policy violations detected across %d namespaces — fix violating resources or adjust constraints", s.ViolationCount, s.NamespacesWithViolations))
	}
	if s.AuditMode > 0 && s.EnforcedCount == 0 {
		recs = append(recs, "all constraints are in audit mode — consider switching to enforce mode for active compliance")
	}
	if s.EnforcedCount > 0 && s.ViolationCount == 0 {
		recs = append(recs, "policy compliance is well enforced — no violations detected")
	}
	return recs
}

// handleOPACompliance audits OPA/Gatekeeper policy compliance.
// GET /api/security/opa-compliance
func (s *Server) handleOPACompliance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// Check for Gatekeeper and Kyverno pods
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	var gatekeeperPods, kyvernoPods []corev1.Pod
	for i := range allPods.Items {
		pod := &allPods.Items[i]
		podName := strings.ToLower(pod.Name)
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			if strings.Contains(podName, "gatekeeper") || strings.Contains(img, "gatekeeper") {
				gatekeeperPods = append(gatekeeperPods, *pod)
			}
			if strings.Contains(podName, "kyverno") || strings.Contains(img, "kyverno") {
				kyvernoPods = append(kyvernoPods, *pod)
			}
		}
	}

	// Try to list Constraint CRDs using dynamic client
	var constraints []unstructured.Unstructured
	if rc.restConfig != nil {
		dynClient, err := dynamic.NewForConfig(rc.restConfig)
		if err == nil {
			// Gatekeeper constraints are CRDs under constraints.gatekeeper.sh/v1beta1
			// Try listing all resources in that group
			gvr := schema.GroupVersionResource{
				Group:    "constraints.gatekeeper.sh",
				Version:  "v1beta1",
				Resource: "", // will try each constraint kind
			}
			// List all constraint kinds by listing each known kind
			knownKinds := []string{"k8srequiredlabels", "k8srequiredannotations", "k8srequiredresources", "k8sdisallowedhostpath", "k8scontainerlimits"}
			for _, kind := range knownKinds {
				gvr.Resource = kind
				list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
				if err == nil && list != nil {
					constraints = append(constraints, list.Items...)
				}
			}
		}
	}

	result := opaComplianceAuditCore(gatekeeperPods, kyvernoPods, constraints, nil)
	writeJSON(w, result)
}

// Suppress unused import
var _ = json.Marshal
var _ = context.Background
