package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestartPolicyResult is the restart policy & lifecycle audit result.
type RestartPolicyResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RestartPolicySummary `json:"summary"`
	ByWorkload      []RPEntry            `json:"byWorkload"`
	Issues          []RPIssue            `json:"issues"`
	ByNamespace     []RPNSStat           `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

// RestartPolicySummary aggregates restart policy statistics.
type RestartPolicySummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	AlwaysPolicy     int `json:"alwaysPolicy"`
	OnFailurePolicy  int `json:"onFailurePolicy"`
	NeverPolicy      int `json:"neverPolicy"`
	PolicyMismatches int `json:"policyMismatches"` // wrong policy for workload type
	WithPostStart    int `json:"withPostStartHook"`
	WithPreStop      int `json:"withPreStopHook"`
	NoLifecycleHook  int `json:"noLifecycleHook"`
	HealthScore      int `json:"healthScore"`
}

// RPEntry describes one workload's restart policy and lifecycle config.
type RPEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	WorkloadType  string `json:"workloadType"`
	RestartPolicy string `json:"restartPolicy"`
	HasPostStart  bool   `json:"hasPostStartHook"`
	HasPreStop    bool   `json:"hasPreStopHook"`
	IsCorrect     bool   `json:"isCorrectPolicy"`
	RiskLevel     string `json:"riskLevel"`
}

// RPIssue is a detected restart policy or lifecycle issue.
type RPIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// RPNSStat shows restart policy stats per namespace.
type RPNSStat struct {
	Namespace   string `json:"namespace"`
	TotalPods   int    `json:"totalPods"`
	Mismatches  int    `json:"mismatches"`
	NoLifecycle int    `json:"noLifecycle"`
	IsSystem    bool   `json:"isSystem"`
}

// handleRestartPolicy audits container restart policies and lifecycle hooks.
// GET /api/deployment/restart-policy
func (s *Server) handleRestartPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	now := time.Now()
	result := RestartPolicyResult{ScannedAt: now}
	result.Summary.TotalWorkloads = len(pods.Items)
	nsStats := map[string]*RPNSStat{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		nsStat, ok := nsStats[pod.Namespace]
		if !ok {
			nsStat = &RPNSStat{Namespace: pod.Namespace, IsSystem: isSystemNamespace(pod.Namespace)}
			nsStats[pod.Namespace] = nsStat
		}
		nsStat.TotalPods++

		rp := string(pod.Spec.RestartPolicy)
		wlType := inferWorkloadTypeFromPod(&pod)

		// Check postStart/preStop hooks
		hasPostStart := false
		hasPreStop := false
		for _, c := range pod.Spec.Containers {
			if c.Lifecycle != nil {
				if c.Lifecycle.PostStart != nil {
					hasPostStart = true
				}
				if c.Lifecycle.PreStop != nil {
					hasPreStop = true
				}
			}
		}

		// Determine correct policy for workload type
		isCorrect := true
		risk := "low"
		var issue *RPIssue

		// Deployments/StatefulSets/DaemonSets should always use Always
		// Jobs should use OnFailure or Never
		// Standalone pods can use any
		switch wlType {
		case "Deployment", "StatefulSet", "DaemonSet":
			if rp != "Always" && rp != "" {
				isCorrect = false
				risk = "high"
				issue = &RPIssue{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Issue:     fmt.Sprintf("%s should use RestartPolicy=Always but has %s", wlType, rp),
					Severity:  "high",
				}
			}
		case "Job", "CronJob":
			if rp == "Always" {
				isCorrect = false
				risk = "high"
				issue = &RPIssue{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Issue:     fmt.Sprintf("%s should not use RestartPolicy=Always (use OnFailure or Never)", wlType),
					Severity:  "high",
				}
			}
		}

		// Update summary
		switch rp {
		case "Always":
			result.Summary.AlwaysPolicy++
		case "OnFailure":
			result.Summary.OnFailurePolicy++
		case "Never":
			result.Summary.NeverPolicy++
		}

		if hasPostStart {
			result.Summary.WithPostStart++
		}
		if hasPreStop {
			result.Summary.WithPreStop++
		}
		if !hasPostStart && !hasPreStop {
			result.Summary.NoLifecycleHook++
		}

		if !isCorrect {
			result.Summary.PolicyMismatches++
			nsStat.Mismatches++
			if issue != nil {
				result.Issues = append(result.Issues, *issue)
			}
		}

		if !hasPreStop && (wlType == "Deployment" || wlType == "StatefulSet") {
			result.Issues = append(result.Issues, RPIssue{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Issue:     fmt.Sprintf("%s has no preStop lifecycle hook — may not drain gracefully during rolling updates", wlType),
				Severity:  "low",
			})
			nsStat.NoLifecycle++
		}

		entry := RPEntry{
			Name:          pod.Name,
			Namespace:     pod.Namespace,
			WorkloadType:  wlType,
			RestartPolicy: rp,
			HasPostStart:  hasPostStart,
			HasPreStop:    hasPreStop,
			IsCorrect:     isCorrect,
			RiskLevel:     risk,
		}
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Mismatches > result.ByNamespace[j].Mismatches
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Issues[i].Severity] < sevOrder[result.Issues[j].Severity]
	})
	if len(result.Issues) > 30 {
		result.Issues = result.Issues[:30]
	}

	result.Summary.HealthScore = restartPolicyScore(result.Summary)
	result.Recommendations = restartPolicyRecommendations(&result)

	writeJSON(w, result)
}

// restartPolicyScore computes a 0-100 health score.
func restartPolicyScore(s RestartPolicySummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}

	score := 100

	if s.PolicyMismatches > 0 {
		score -= min(40, s.PolicyMismatches*10)
	}

	if s.NoLifecycleHook > 0 {
		ratio := float64(s.NoLifecycleHook) / float64(s.TotalWorkloads)
		score -= int(ratio * 20)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// restartPolicyRecommendations generates actionable recommendations.
func restartPolicyRecommendations(r *RestartPolicyResult) []string {
	var recs []string

	if r.Summary.PolicyMismatches > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d workload(s) have incorrect restart policy for their type — fix to ensure proper pod lifecycle management",
			r.Summary.PolicyMismatches,
		))
	}

	if r.Summary.NoLifecycleHook > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d workload(s) have no lifecycle hooks — add preStop hooks for graceful shutdown during rolling updates",
			r.Summary.NoLifecycleHook,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "Restart policies and lifecycle hooks are properly configured")
	}

	return recs
}
