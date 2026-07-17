package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OncallReadinessResult evaluates whether the cluster can safely operate
// unattended by checking: alerting coverage, PDB existence, monitoring gaps,
// auto-remediation readiness, and runbook availability.
type OncallReadinessResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         OncallSummary `json:"summary"`
	Checks          []OncallCheck `json:"checks"`
	CoverageGaps    []OncallCheck `json:"coverageGaps"`
	ReadinessScore  int           `json:"readinessScore"`
	Grade           string        `json:"grade"`
	Recommendations []string      `json:"recommendations"`
}

type OncallSummary struct {
	TotalChecks       int  `json:"totalChecks"`
	Passed            int  `json:"passed"`
	Failed            int  `json:"failed"`
	Warnings          int  `json:"warnings"`
	CriticalGaps      int  `json:"criticalGaps"`
	SafeForUnattended bool `json:"safeForUnattended"`
}

type OncallCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

// handleOncallReadiness handles GET /api/docs/oncall-readiness
func (s *Server) handleOncallReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := OncallReadinessResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	workerNodes := 0
	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		workerNodes++
	}

	// --- Check 1: Multi-node HA ---
	haOK := workerNodes >= 3
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "multi-node-ha",
		Category: "infrastructure",
		Status:   checkStatusOKForOncall(haOK),
		Severity: oncallSeverity(haOK, "critical"),
		Message:  fmt.Sprintf("%d worker nodes (need >=3 for HA)", workerNodes),
		Detail:   "Single node = SPOF, no fault tolerance during oncall",
	})

	// --- Check 2: PDB coverage ---
	wlCount := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			wlCount++
		}
	}
	pdbCount := len(pdbs.Items)
	pdbRatio := 0.0
	if wlCount > 0 {
		pdbRatio = float64(pdbCount) / float64(wlCount)
	}
	pdbOK := pdbRatio > 0.3
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "pdb-coverage",
		Category: "availability",
		Status:   checkStatusOKForOncall(pdbOK),
		Severity: oncallSeverity(pdbOK, "high"),
		Message:  fmt.Sprintf("%d PDBs / %d workloads (%.0f%%)", pdbCount, wlCount, pdbRatio*100),
		Detail:   "PDBs prevent voluntary disruption during maintenance",
	})

	// --- Check 3: HPA for auto-scaling ---
	hpaCount := len(hpas.Items)
	hpaOK := hpaCount > 0
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "hpa-coverage",
		Category: "autoscaling",
		Status:   checkStatusOKForOncall(hpaOK),
		Severity: oncallSeverity(hpaOK, "medium"),
		Message:  fmt.Sprintf("%d HPAs configured", hpaCount),
		Detail:   "HPA ensures workloads scale automatically under load",
	})

	// --- Check 4: Health probes ---
	withProbes := 0
	withoutProbes := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		hasProbe := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.ReadinessProbe == nil || c.LivenessProbe == nil {
				hasProbe = false
				break
			}
		}
		if hasProbe {
			withProbes++
		} else {
			withoutProbes++
		}
	}
	probesOK := withoutProbes == 0
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "health-probes",
		Category: "monitoring",
		Status:   checkStatusOKForOncall(probesOK),
		Severity: oncallSeverity(probesOK, "high"),
		Message:  fmt.Sprintf("%d/%d have readiness+liveness probes", withProbes, withProbes+withoutProbes),
		Detail:   "Probes enable automatic restart of unhealthy pods",
	})

	// --- Check 5: Resource limits ---
	withLimits := 0
	withoutLimits := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		hasLimits := true
		for _, c := range d.Spec.Template.Spec.Containers {
			if len(c.Resources.Limits) == 0 {
				hasLimits = false
				break
			}
		}
		if hasLimits {
			withLimits++
		} else {
			withoutLimits++
		}
	}
	limitsOK := withoutLimits == 0
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "resource-limits",
		Category: "resources",
		Status:   checkStatusOKForOncall(limitsOK),
		Severity: oncallSeverity(limitsOK, "high"),
		Message:  fmt.Sprintf("%d/%d have resource limits", withLimits, withLimits+withoutLimits),
		Detail:   "Without limits, a single pod can consume all node resources",
	})

	// --- Check 6: Crash loop detection ---
	crashLoopPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 {
				crashLoopPods++
				break
			}
		}
	}
	noCrashLoops := crashLoopPods == 0
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "crash-loop-check",
		Category: "stability",
		Status:   checkStatusOKForOncall(noCrashLoops),
		Severity: oncallSeverity(noCrashLoops, "critical"),
		Message:  fmt.Sprintf("%d pods with >5 restarts", crashLoopPods),
		Detail:   "Crash looping pods indicate ongoing issues requiring attention",
	})

	// --- Check 7: Runbook annotations ---
	withRunbook := 0
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		for _, ann := range []string{"runbook", "runbook-url", "docs", "wiki"} {
			if _, ok := d.Annotations[ann]; ok {
				withRunbook++
				break
			}
		}
	}
	runbookOK := withRunbook > 0
	result.Checks = append(result.Checks, OncallCheck{
		Name:     "runbook-annotations",
		Category: "documentation",
		Status:   checkStatusOKForOncall(runbookOK),
		Severity: oncallSeverity(runbookOK, "medium"),
		Message:  fmt.Sprintf("%d/%d workloads have runbook annotations", withRunbook, wlCount),
		Detail:   "Runbooks help on-call engineers resolve incidents quickly",
	})

	// Calculate summary
	result.Summary.TotalChecks = len(result.Checks)
	for _, c := range result.Checks {
		switch c.Status {
		case "pass":
			result.Summary.Passed++
		case "fail":
			result.Summary.Failed++
			if c.Severity == "critical" {
				result.Summary.CriticalGaps++
			}
			result.Summary.Warnings++
		}
	}

	// Safe for unattended if no critical gaps
	result.Summary.SafeForUnattended = result.Summary.CriticalGaps == 0

	// Collect coverage gaps
	for _, c := range result.Checks {
		if c.Status == "fail" {
			result.CoverageGaps = append(result.CoverageGaps, c)
		}
	}
	sort.Slice(result.CoverageGaps, func(i, j int) bool {
		sevRank := map[string]int{"critical": 0, "high": 1, "medium": 2}
		return sevRank[result.CoverageGaps[i].Severity] < sevRank[result.CoverageGaps[j].Severity]
	})

	// Readiness score
	result.ReadinessScore = result.Summary.Passed * 100 / result.Summary.TotalChecks

	switch {
	case result.ReadinessScore >= 85:
		result.Grade = "A"
	case result.ReadinessScore >= 70:
		result.Grade = "B"
	case result.ReadinessScore >= 50:
		result.Grade = "C"
	case result.ReadinessScore >= 30:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildOncallReadinessRecs(&result)
	writeJSON(w, result)
}

func checkStatusOKForOncall(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func oncallSeverity(ok bool, level string) string {
	if ok {
		return "none"
	}
	return level
}

func buildOncallReadinessRecs(r *OncallReadinessResult) []string {
	recs := []string{
		fmt.Sprintf("值班就绪: %d/%d 检查通过 (%d%%), %s", r.Summary.Passed, r.Summary.TotalChecks, r.ReadinessScore, safeUnattendedText(r.Summary.SafeForUnattended)),
	}
	if r.Summary.CriticalGaps > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个关键检查未通过, 集群不适合无人值守", r.Summary.CriticalGaps))
	}
	for _, gap := range r.CoverageGaps {
		if gap.Severity == "critical" || gap.Severity == "high" {
			recs = append(recs, fmt.Sprintf("[%s] %s: %s", gap.Severity, gap.Name, gap.Message))
		}
	}
	if !r.Summary.SafeForUnattended {
		recs = append(recs, "建议: 修复所有 critical 检查项后, 集群才能安全无人值守运行")
	}
	return recs
}

func safeUnattendedText(safe bool) string {
	if safe {
		return "适合无人值守"
	}
	return "不适合无人值守"
}
