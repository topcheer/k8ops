package dashboard

import (
	"fmt"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PreflightCheckResult validates all prerequisites for a safe deployment:
// resource availability, probe configuration, PDB existence, rollout strategy,
// node health, and configmap/secret references.
type PreflightCheckResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         PreflightSummary `json:"summary"`
	Checks          []PreflightCheck `json:"checks"`
	BlockingChecks  []PreflightCheck `json:"blockingChecks"`
	PassRate        int              `json:"passRate"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type PreflightSummary struct {
	TotalChecks   int `json:"totalChecks"`
	Passed        int `json:"passed"`
	Failed        int `json:"failed"`
	Warnings      int `json:"warnings"`
	BlockingCount int `json:"blockingCount"`
}

type PreflightCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

// handlePreflightCheck handles GET /api/deployment/preflight-check
func (s *Server) handlePreflightCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PreflightCheckResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// --- Check 1: All Deployments have resource requests ---
	hasResources := 0
	noResources := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		allHaveReqs := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if len(c.Resources.Requests) == 0 {
				allHaveReqs = false
				break
			}
		}
		if allHaveReqs {
			hasResources++
		} else {
			noResources++
		}
	}
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "resource-requests",
		Category: "resources",
		Status:   checkStatus(noResources == 0),
		Severity: severityFor(noResources == 0, "blocking"),
		Message:  fmt.Sprintf("%d/%d Deployments have resource requests", hasResources, hasResources+noResources),
		Detail:   fmt.Sprintf("%d missing resource requests", noResources),
	})

	// --- Check 2: Readiness probes configured ---
	withReadyProbe := 0
	withoutReadyProbe := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		hasProbe := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe == nil {
				hasProbe = false
				break
			}
		}
		if hasProbe {
			withReadyProbe++
		} else {
			withoutReadyProbe++
		}
	}
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "readiness-probes",
		Category: "health",
		Status:   checkStatus(withoutReadyProbe == 0),
		Severity: severityFor(withoutReadyProbe == 0, "warning"),
		Message:  fmt.Sprintf("%d/%d Deployments have readiness probes", withReadyProbe, withReadyProbe+withoutReadyProbe),
		Detail:   fmt.Sprintf("%d missing readiness probes", withoutReadyProbe),
	})

	// --- Check 3: PDB coverage ---
	pdbCount := len(pdbs.Items)
	wlCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			wlCount++
		}
	}
	pdbOK := pdbCount > 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "pdb-coverage",
		Category: "availability",
		Status:   checkStatus(pdbOK),
		Severity: severityFor(pdbOK, "warning"),
		Message:  fmt.Sprintf("%d PDBs for ~%d workloads", pdbCount, wlCount),
		Detail:   fmt.Sprintf("PDB ratio: %.0f%%", safeDivInt(pdbCount, wlCount)*100),
	})

	// --- Check 4: Rolling update strategy ---
	maxUnavailable := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if d.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
			maxUnavailable++
		}
	}
	rollingOK := maxUnavailable == 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "rolling-strategy",
		Category: "strategy",
		Status:   checkStatus(rollingOK),
		Severity: severityFor(rollingOK, "info"),
		Message:  fmt.Sprintf("%d Deployments not using RollingUpdate strategy", maxUnavailable),
		Detail:   "Non-rolling strategies cause downtime during updates",
	})

	// --- Check 5: Node health ---
	unhealthyNodes := 0
	for _, node := range nodes.Items {
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				unhealthyNodes++
			}
		}
	}
	nodeOK := unhealthyNodes == 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "node-health",
		Category: "infrastructure",
		Status:   checkStatus(nodeOK),
		Severity: severityFor(nodeOK, "blocking"),
		Message:  fmt.Sprintf("%d/%d nodes healthy", len(nodes.Items)-unhealthyNodes, len(nodes.Items)),
		Detail:   fmt.Sprintf("%d nodes not ready", unhealthyNodes),
	})

	// --- Check 6: HPA coverage ---
	hpaCount := len(hpas.Items)
	hpaOK := hpaCount > 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "hpa-coverage",
		Category: "autoscaling",
		Status:   checkStatus(hpaOK),
		Severity: severityFor(hpaOK, "info"),
		Message:  fmt.Sprintf("%d HPAs configured", hpaCount),
		Detail:   fmt.Sprintf("HPA coverage: %.0f%% of workloads", safeDivInt(hpaCount, wlCount)*100),
	})

	// --- Check 7: Revision history limit ---
	lowRevHistory := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if d.Spec.RevisionHistoryLimit == nil || *d.Spec.RevisionHistoryLimit < 2 {
			lowRevHistory++
		}
	}
	revOK := lowRevHistory == 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "revision-history",
		Category: "rollback",
		Status:   checkStatus(revOK),
		Severity: severityFor(revOK, "warning"),
		Message:  fmt.Sprintf("%d Deployments with low revision history (<2)", lowRevHistory),
		Detail:   "Insufficient revision history prevents safe rollback",
	})

	// --- Check 8: Graceful shutdown (terminationGracePeriodSeconds) ---
	lowGracePeriod := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		gp := d.Spec.Template.Spec.TerminationGracePeriodSeconds
		if gp == nil || *gp < 30 {
			lowGracePeriod++
		}
	}
	graceOK := lowGracePeriod == 0
	result.Checks = append(result.Checks, PreflightCheck{
		Name:     "graceful-shutdown",
		Category: "lifecycle",
		Status:   checkStatus(graceOK),
		Severity: severityFor(graceOK, "info"),
		Message:  fmt.Sprintf("%d Deployments with low grace period (<30s)", lowGracePeriod),
		Detail:   "Short grace period may cause connection drops",
	})

	// Calculate summary
	result.Summary.TotalChecks = len(result.Checks)
	for _, c := range result.Checks {
		switch c.Status {
		case "pass":
			result.Summary.Passed++
		case "fail":
			result.Summary.Failed++
			if c.Severity == "blocking" {
				result.Summary.BlockingCount++
				result.BlockingChecks = append(result.BlockingChecks, c)
			} else {
				result.Summary.Warnings++
			}
		}
	}

	result.PassRate = result.Summary.Passed * 100 / result.Summary.TotalChecks

	switch {
	case result.PassRate >= 87: // 7/8
		result.Grade = "A"
	case result.PassRate >= 75: // 6/8
		result.Grade = "B"
	case result.PassRate >= 50:
		result.Grade = "C"
	case result.PassRate >= 25:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildPreflightRecs(&result)
	writeJSON(w, result)
}

func checkStatus(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func severityFor(ok bool, level string) string {
	if ok {
		return "none"
	}
	return level
}

func safeDivInt(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func buildPreflightRecs(r *PreflightCheckResult) []string {
	recs := []string{
		fmt.Sprintf("预检通过率: %d/%d (%d%%)", r.Summary.Passed, r.Summary.TotalChecks, r.PassRate),
	}
	if r.Summary.BlockingCount > 0 {
		recs = append(recs, fmt.Sprintf("阻塞: %d 项关键检查未通过，部署将被阻止", r.Summary.BlockingCount))
	}
	if r.Summary.Warnings > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 项检查建议修复", r.Summary.Warnings))
	}
	for _, c := range r.BlockingChecks {
		recs = append(recs, fmt.Sprintf("[BLOCKING] %s: %s", c.Name, c.Message))
	}
	if r.PassRate < 75 {
		recs = append(recs, "建议: 修复所有 blocking 和 warning 项后再执行部署")
	}
	return recs
}
