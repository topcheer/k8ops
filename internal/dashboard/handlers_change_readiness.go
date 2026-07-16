package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChangeReadinessResult is the deployment change readiness pre-flight gate.
// It evaluates whether the cluster is in a safe state to accept new deployments.
type ChangeReadinessResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ReadinessSummary    `json:"summary"`
	GateDecision    string              `json:"gateDecision"` // proceed, proceed-with-caution, blocked
	ReadinessScore  int                 `json:"readinessScore"` // 0-100, higher = more ready
	Checks          []ReadinessCheck    `json:"checks"`
	Blockers        []ReadinessBlocker  `json:"blockers,omitempty"`
	Warnings        []ReadinessWarning  `json:"warnings,omitempty"`
	AffectedWorkloads int               `json:"affectedWorkloads"`
	RecentFailures  []RecentFailure     `json:"recentFailures,omitempty"`
	CapacityHeadroom CapacityHeadroom   `json:"capacityHeadroom"`
	RollbackPaths   int                 `json:"rollbackPaths"`
	Recommendations []string            `json:"recommendations"`
}

// ReadinessSummary aggregates gate check results.
type ReadinessSummary struct {
	TotalChecks     int `json:"totalChecks"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Warnings        int `json:"warnings"`
	Skipped         int `json:"skipped"`
	ActiveRollouts  int `json:"activeRollouts"`  // workloads currently rolling out
	FailedPods      int `json:"failedPods"`      // pods in CrashLoopBackOff or ImagePullBackOff
	NodePressure    int `json:"nodePressure"`    // nodes with pressure conditions
}

// ReadinessCheck describes one pre-flight gate check.
type ReadinessCheck struct {
	Name        string `json:"name"`
	Category    string `json:"category"` // pdb, probe, resource, image, rollback, capacity, stability
	Status      string `json:"status"`   // pass, fail, warn, skip
	Description string `json:"description"`
	Detail      string `json:"detail,omitempty"`
}

// ReadinessBlocker is a hard blocker that prevents deployment.
type ReadinessBlocker struct {
	Check     string `json:"check"`
	Severity  string `json:"severity"` // critical, high
	Message   string `json:"message"`
	Workload  string `json:"workload,omitempty"`
}

// ReadinessWarning is a soft warning that doesn't block but should be reviewed.
type ReadinessWarning struct {
	Check   string `json:"check"`
	Message string `json:"message"`
}

// RecentFailure tracks a recent deployment failure signal.
type RecentFailure struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"` // CrashLoopBackOff, ImagePullBackOff, ErrImagePull
	Age       string `json:"age"`
}

// CapacityHeadroom describes available scheduling capacity.
type CapacityHeadroom struct {
	TotalPodSlots int `json:"totalPodSlots"`
	UsedPodSlots  int `json:"usedPodSlots"`
	Available     int `json:"available"`
	Utilization   float64 `json:"utilization"` // percentage
	NodesReady    int `json:"nodesReady"`
	NodesNotReady int `json:"nodesNotReady"`
}

// handleChangeReadiness evaluates cluster readiness for accepting new deployments.
// GET /api/deployment/change-readiness
func (s *Server) handleChangeReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChangeReadinessResult{ScannedAt: time.Now()}

	// 1. Collect cluster state
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	var checks []ReadinessCheck
	var blockers []ReadinessBlocker
	var warnings []ReadinessWarning

	// ========================================
	// CHECK 1: Node Stability — no nodes under pressure
	// ========================================
	nodesNotReady := 0
	nodesReady := 0
	nodePressureCount := 0
	for _, node := range nodes.Items {
		isReady := false
		hasPressure := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
			if cond.Type != corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				hasPressure = true
			}
		}
		if isReady && !hasPressure {
			nodesReady++
		} else {
			nodesNotReady++
		}
		if hasPressure {
			nodePressureCount++
		}
	}

	if nodePressureCount > 0 {
		checks = append(checks, ReadinessCheck{
			Name:        "node-stability",
			Category:    "stability",
			Status:      "fail",
			Description: "Nodes with pressure conditions detected",
			Detail:      fmt.Sprintf("%d node(s) have pressure conditions — deploying now risks cascading failures", nodePressureCount),
		})
		blockers = append(blockers, ReadinessBlocker{
			Check:    "node-stability",
			Severity: "high",
			Message:  fmt.Sprintf("%d node(s) under pressure — resolve before deploying", nodePressureCount),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "node-stability",
			Category:    "stability",
			Status:      "pass",
			Description: "All nodes stable, no pressure conditions",
		})
	}
	result.Summary.NodePressure = nodePressureCount

	// ========================================
	// CHECK 2: No Active Rollouts — cluster not mid-deployment
	// ========================================
	activeRollouts := 0
	if deployments != nil {
		for _, d := range deployments.Items {
			if d.Status.UpdatedReplicas < *d.Spec.Replicas ||
				d.Status.Replicas != d.Status.UpdatedReplicas ||
				d.Status.AvailableReplicas < d.Status.UpdatedReplicas {
				activeRollouts++
			}
		}
	}

	if activeRollouts > 3 {
		checks = append(checks, ReadinessCheck{
			Name:        "active-rollouts",
			Category:    "stability",
			Status:      "fail",
			Description: "Too many concurrent active rollouts",
			Detail:      fmt.Sprintf("%d deployment(s) are mid-rollout — wait for them to complete", activeRollouts),
		})
		blockers = append(blockers, ReadinessBlocker{
			Check:    "active-rollouts",
			Severity: "high",
			Message:  fmt.Sprintf("%d active rollouts in progress — wait for stabilization", activeRollouts),
		})
	} else if activeRollouts > 0 {
		checks = append(checks, ReadinessCheck{
			Name:        "active-rollouts",
			Category:    "stability",
			Status:      "warn",
			Description: fmt.Sprintf("%d active rollout(s) in progress", activeRollouts),
			Detail:      "Minor rollout activity — generally safe but monitor closely",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "active-rollouts",
			Message: fmt.Sprintf("%d active rollout(s) — consider waiting for completion", activeRollouts),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "active-rollouts",
			Category:    "stability",
			Status:      "pass",
			Description: "No active rollouts — cluster is stable",
		})
	}
	result.Summary.ActiveRollouts = activeRollouts

	// ========================================
	// CHECK 3: Failed Pods — no CrashLoopBackOff or ImagePullBackOff
	// ========================================
	failedPods := 0
	var recentFailures []RecentFailure
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodPending {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.State.Waiting != nil {
						reason := cs.State.Waiting.Reason
						if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CrashLoopBackOff" {
							failedPods++
							recentFailures = append(recentFailures, RecentFailure{
								PodName:   pod.Name,
								Namespace: pod.Namespace,
								Reason:    reason,
								Age:       time.Since(pod.CreationTimestamp.Time).Round(time.Minute).String(),
							})
						}
					}
				}
			}
			// Check running pods for crash loops
			if pod.Status.Phase == corev1.PodRunning {
				for _, cs := range pod.Status.ContainerStatuses {
					totalRestarts := int(cs.RestartCount)
					if totalRestarts > 10 {
						failedPods++
						recentFailures = append(recentFailures, RecentFailure{
							PodName:   pod.Name,
							Namespace: pod.Namespace,
							Reason:    fmt.Sprintf("HighRestarts(%d)", totalRestarts),
							Age:       time.Since(pod.CreationTimestamp.Time).Round(time.Minute).String(),
						})
					}
				}
			}
		}
	}

	if failedPods > 10 {
		checks = append(checks, ReadinessCheck{
			Name:        "failed-pods",
			Category:    "stability",
			Status:      "fail",
			Description: "Excessive failed or crash-looping pods",
			Detail:      fmt.Sprintf("%d pod(s) in CrashLoopBackOff, ImagePullBackOff, or high-restart state — cluster is unhealthy", failedPods),
		})
		blockers = append(blockers, ReadinessBlocker{
			Check:    "failed-pods",
			Severity: "critical",
			Message:  fmt.Sprintf("%d failed/crashing pods — fix existing issues before adding new deployments", failedPods),
		})
	} else if failedPods > 0 {
		checks = append(checks, ReadinessCheck{
			Name:        "failed-pods",
			Category:    "stability",
			Status:      "warn",
			Description: fmt.Sprintf("%d pod(s) in degraded state", failedPods),
			Detail:      "Some pods are failing — review before deploying",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "failed-pods",
			Message: fmt.Sprintf("%d pod(s) in degraded state", failedPods),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "failed-pods",
			Category:    "stability",
			Status:      "pass",
			Description: "No crashing or failed pods detected",
		})
	}
	result.Summary.FailedPods = failedPods
	result.RecentFailures = recentFailures

	// ========================================
	// CHECK 4: PDB Coverage — workloads have disruption protection
	// ========================================
	workloadsWithPDB := 0
	workloadsWithoutPDB := 0
	pdbSelectors := []map[string]string{}
	if pdbs != nil {
		for _, pdb := range pdbs.Items {
			if pdb.Spec.Selector != nil {
				pdbSelectors = append(pdbSelectors, pdb.Spec.Selector.MatchLabels)
			}
		}
	}
	if deployments != nil {
		for _, d := range deployments.Items {
			if d.Spec.Replicas != nil && *d.Spec.Replicas > 0 {
				hasPDB := false
				for _, sel := range pdbSelectors {
					matchCount := 0
					for k, v := range sel {
						if d.Spec.Selector.MatchLabels[k] == v {
							matchCount++
						}
					}
					if matchCount == len(sel) && len(sel) > 0 {
						hasPDB = true
						break
					}
				}
				if hasPDB {
					workloadsWithPDB++
				} else {
					workloadsWithoutPDB++
				}
			}
		}
	}

	pdbCoverage := 0
	totalWorkloads := workloadsWithPDB + workloadsWithoutPDB
	if totalWorkloads > 0 {
		pdbCoverage = workloadsWithPDB * 100 / totalWorkloads
	}
	if pdbCoverage < 50 && totalWorkloads > 5 {
		checks = append(checks, ReadinessCheck{
			Name:        "pdb-coverage",
			Category:    "pdb",
			Status:      "warn",
			Description: fmt.Sprintf("PDB coverage at %d%% (%d/%d workloads)", pdbCoverage, workloadsWithPDB, totalWorkloads),
			Detail:      "Low PDB coverage means rollout disruptions may cause unexpected downtime",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "pdb-coverage",
			Message: fmt.Sprintf("Only %d%% of workloads have PDB — add PDBs before deploying critical workloads", pdbCoverage),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "pdb-coverage",
			Category:    "pdb",
			Status:      "pass",
			Description: fmt.Sprintf("PDB coverage %d%% (%d/%d)", pdbCoverage, workloadsWithPDB, totalWorkloads),
		})
	}

	// ========================================
	// CHECK 5: Capacity Headroom — enough room for surge pods
	// ========================================
	totalPodSlots := 0
	usedPodSlots := 0
	if nodes != nil {
		for _, node := range nodes.Items {
			if !nodeIsReady(&node) {
				continue
			}
			if slots, ok := node.Status.Allocatable.Pods().AsInt64(); ok {
				totalPodSlots += int(slots)
			}
		}
	}
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
				usedPodSlots++
			}
		}
	}

	available := totalPodSlots - usedPodSlots
	utilization := 0.0
	if totalPodSlots > 0 {
		utilization = float64(usedPodSlots) * 100 / float64(totalPodSlots)
	}

	if utilization > 85 {
		checks = append(checks, ReadinessCheck{
			Name:        "capacity-headroom",
			Category:    "capacity",
			Status:      "fail",
			Description: fmt.Sprintf("Pod capacity at %.1f%% — insufficient surge room", utilization),
			Detail:      fmt.Sprintf("Only %d pod slots available — deployment surge may fail scheduling", available),
		})
		blockers = append(blockers, ReadinessBlocker{
			Check:    "capacity-headroom",
			Severity: "high",
			Message:  fmt.Sprintf("Pod capacity at %.1f%% — add nodes or clean up stale pods", utilization),
		})
	} else if utilization > 70 {
		checks = append(checks, ReadinessCheck{
			Name:        "capacity-headroom",
			Category:    "capacity",
			Status:      "warn",
			Description: fmt.Sprintf("Pod capacity at %.1f%% — limited surge room", utilization),
			Detail:      fmt.Sprintf("%d slots available — large deployments may be tight", available),
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "capacity-headroom",
			Message: fmt.Sprintf("Capacity at %.1f%% — monitor scheduling during rollout", utilization),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "capacity-headroom",
			Category:    "capacity",
			Status:      "pass",
			Description: fmt.Sprintf("Pod capacity at %.1f%% (%d slots available)", utilization, available),
		})
	}
	result.CapacityHeadroom = CapacityHeadroom{
		TotalPodSlots: totalPodSlots,
		UsedPodSlots:  usedPodSlots,
		Available:     available,
		Utilization:   utilRound(utilization),
		NodesReady:    nodesReady,
		NodesNotReady: nodesNotReady,
	}

	// ========================================
	// CHECK 6: Rollback Path — revision history available
	// ========================================
	rollbackPaths := 0
	if deployments != nil {
		for _, d := range deployments.Items {
			limit := int32(10) // default
			if d.Spec.RevisionHistoryLimit != nil {
				limit = *d.Spec.RevisionHistoryLimit
			}
			if limit > 0 {
				rollbackPaths++
			}
		}
	}
	rollbackCoverage := 0
	if deployments != nil && len(deployments.Items) > 0 {
		rollbackCoverage = rollbackPaths * 100 / len(deployments.Items)
	}
	if rollbackCoverage < 80 {
		checks = append(checks, ReadinessCheck{
			Name:        "rollback-path",
			Category:    "rollback",
			Status:      "warn",
			Description: fmt.Sprintf("Rollback path available for %d%% of deployments", rollbackCoverage),
			Detail:      "Some deployments have RevisionHistoryLimit=0 — cannot roll back",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "rollback-path",
			Message: fmt.Sprintf("%d%% rollback coverage — set RevisionHistoryLimit > 0 for all deployments", rollbackCoverage),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "rollback-path",
			Category:    "rollback",
			Status:      "pass",
			Description: fmt.Sprintf("Rollback path available for %d%% of deployments", rollbackCoverage),
		})
	}
	result.RollbackPaths = rollbackPaths

	// ========================================
	// CHECK 7: Resource Limits — workloads have resource boundaries
	// ========================================
	noLimitCount := 0
	checkedWorkloads := 0
	if deployments != nil {
		for _, d := range deployments.Items {
			checkedWorkloads++
			for _, c := range d.Spec.Template.Spec.Containers {
				if c.Resources.Limits.Cpu() == nil || c.Resources.Limits.Memory() == nil {
					noLimitCount++
				}
			}
		}
	}
	if checkedWorkloads > 0 && noLimitCount > checkedWorkloads {
		checks = append(checks, ReadinessCheck{
			Name:        "resource-limits",
			Category:    "resource",
			Status:      "warn",
			Description: fmt.Sprintf("%d container(s) without resource limits", noLimitCount),
			Detail:      "Containers without limits can cause node pressure during rollout surges",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "resource-limits",
			Message: fmt.Sprintf("%d containers without limits — set CPU/memory limits", noLimitCount),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "resource-limits",
			Category:    "resource",
			Status:      "pass",
			Description: "Resource limits generally set across workloads",
		})
	}

	// ========================================
	// CHECK 8: Health Probes — workloads have readiness checks
	// ========================================
	missingProbes := 0
	if deployments != nil {
		for _, d := range deployments.Items {
			for _, c := range d.Spec.Template.Spec.Containers {
				if c.ReadinessProbe == nil {
					missingProbes++
				}
			}
		}
	}
	if missingProbes > 5 {
		checks = append(checks, ReadinessCheck{
			Name:        "health-probes",
			Category:    "probe",
			Status:      "warn",
			Description: fmt.Sprintf("%d container(s) without readiness probes", missingProbes),
			Detail:      "Without readiness probes, rollout cannot detect bad deployments — traffic may route to broken pods",
		})
		warnings = append(warnings, ReadinessWarning{
			Check:   "health-probes",
			Message: fmt.Sprintf("%d containers without readiness probes — add probes for safe rollouts", missingProbes),
		})
	} else {
		checks = append(checks, ReadinessCheck{
			Name:        "health-probes",
			Category:    "probe",
			Status:      "pass",
			Description: "Health probes generally configured",
		})
	}

	// ========================================
	// Compute Gate Decision & Score
	// ========================================
	passed := 0
	failed := 0
	warnCount := 0
	for _, c := range checks {
		switch c.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "warn":
			warnCount++
		}
	}

	result.Summary.TotalChecks = len(checks)
	result.Summary.Passed = passed
	result.Summary.Failed = failed
	result.Summary.Warnings = warnCount

	// Gate decision
	criticalBlockers := 0
	highBlockers := 0
	for _, b := range blockers {
		if b.Severity == "critical" {
			criticalBlockers++
		} else {
			highBlockers++
		}
	}

	if criticalBlockers > 0 || highBlockers > 1 {
		result.GateDecision = "blocked"
	} else if highBlockers > 0 || warnCount > 3 {
		result.GateDecision = "proceed-with-caution"
	} else {
		result.GateDecision = "proceed"
	}

	// Readiness score
	score := 100
	score -= criticalBlockers * 30
	score -= highBlockers * 15
	score -= warnCount * 5
	if score < 0 {
		score = 0
	}
	result.ReadinessScore = score

	result.AffectedWorkloads = totalWorkloads
	result.Blockers = blockers
	result.Warnings = warnings
	result.Checks = checks

	// Sort checks by status (fail first, then warn, then pass)
	sort.Slice(result.Checks, func(i, j int) bool {
		order := map[string]int{"fail": 0, "warn": 1, "pass": 2, "skip": 3}
		return order[result.Checks[i].Status] < order[result.Checks[j].Status]
	})

	// Recommendations
	result.Recommendations = generateChangeReadinessRecs(result)

	writeJSON(w, result)
}

// nodeIsReady checks if a node is in Ready state.
func nodeIsReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// utilRound rounds to 1 decimal place.
func utilRound(v float64) float64 {
	return float64(int(v*10)) / 10
}

// generateChangeReadinessRecs produces gate-based recommendations.
func generateChangeReadinessRecs(result ChangeReadinessResult) []string {
	var recs []string

	switch result.GateDecision {
	case "blocked":
		recs = append(recs, fmt.Sprintf("GATE: BLOCKED — %d blocker(s) must be resolved before deploying", len(result.Blockers)))
		for _, b := range result.Blockers {
			recs = append(recs, fmt.Sprintf("  → [%s] %s", b.Severity, b.Message))
		}
	case "proceed-with-caution":
		recs = append(recs, fmt.Sprintf("GATE: PROCEED WITH CAUTION — %d warning(s) to review", len(result.Warnings)))
		recs = append(recs, "Deploy in small batches and monitor rollout closely")
		for _, wr := range result.Warnings {
			recs = append(recs, fmt.Sprintf("  → [%s] %s", wr.Check, wr.Message))
		}
	default:
		recs = append(recs, "GATE: PROCEED — cluster is ready for deployment")
	}

	// Capacity tip
	if result.CapacityHeadroom.Available < 20 && result.CapacityHeadroom.TotalPodSlots > 0 {
		recs = append(recs, fmt.Sprintf("Low pod capacity (%d slots free) — clean up stale pods or add nodes before large deployments", result.CapacityHeadroom.Available))
	}

	// Rollback tip
	if result.RollbackPaths < result.AffectedWorkloads/2 && result.AffectedWorkloads > 0 {
		recs = append(recs, "Ensure RevisionHistoryLimit is set on all deployments for rollback capability")
	}

	if len(recs) == 1 && result.GateDecision == "proceed" {
		recs = append(recs, fmt.Sprintf("All %d checks passed — safe to deploy (score: %d/100)", result.Summary.Passed, result.ReadinessScore))
	}

	return recs
}

// Ensure appsv1 and policyv1 imports are used
var _ appsv1.Deployment
var _ policyv1.PodDisruptionBudget
var _ = strings.Contains
