package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProgressiveDeliveryResult is the progressive delivery & canary rollout health analysis.
type ProgressiveDeliveryResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PDSummary           `json:"summary"`
	Deployments     []PDDeploymentEntry `json:"deployments"`
	ByNamespace     []PDNSStat          `json:"byNamespace"`
	Issues          []PDIssue           `json:"issues"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// PDSummary aggregates progressive delivery statistics.
type PDSummary struct {
	TotalDeployments   int  `json:"totalDeployments"`
	WithCanary         int  `json:"withCanaryAnnotations"`
	WithRollout        int  `json:"withRolloutStrategy"`
	RecreateStrategy   int  `json:"recreateStrategy"`
	RollingStrategy    int  `json:"rollingStrategy"`
	StalledRollouts    int  `json:"stalledRollouts"`
	HighReplicaCount   int  `json:"highReplicaCount"`
	NoProgressDeadline int  `json:"noProgressDeadline"`
	ArgoRollouts       int  `json:"argoRolloutsDetected"`
	FlaggerDetected    bool `json:"flaggerDetected"`
}

// PDDeploymentEntry describes one deployment's progressive delivery posture.
type PDDeploymentEntry struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Strategy            string `json:"strategy"`
	Replicas            int    `json:"replicas"`
	UpdatedReplicas     int    `json:"updatedReplicas"`
	ReadyReplicas       int    `json:"readyReplicas"`
	HasProgressDeadline bool   `json:"hasProgressDeadline"`
	HasCanaryAnnotation bool   `json:"hasCanaryAnnotation"`
	CanaryWeight        string `json:"canaryWeight,omitempty"`
	Stalled             bool   `json:"stalled"`
	RiskLevel           string `json:"riskLevel"`
}

// PDNSStat per-namespace progressive delivery stats.
type PDNSStat struct {
	Namespace        string `json:"namespace"`
	DeploymentCount  int    `json:"deploymentCount"`
	StalledRollouts  int    `json:"stalledRollouts"`
	RecreateStrategy int    `json:"recreateStrategy"`
}

// PDIssue is a detected progressive delivery problem.
type PDIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleProgressiveDelivery audits progressive delivery and canary rollout health.
// GET /api/deployment/progressive-delivery
func (s *Server) handleProgressiveDelivery(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &ProgressiveDeliveryResult{
		ScannedAt: time.Now(),
	}

	// 1. Detect Argo Rollouts and Flagger pods
	allPods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	argoRollouts := 0
	var flaggerDetected bool

	for i := range allPods.Items {
		pod := &allPods.Items[i]
		podName := strings.ToLower(pod.Name)
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			if strings.Contains(podName, "argo-rollouts") || strings.Contains(img, "argo-rollouts") {
				argoRollouts++
			}
			if strings.Contains(podName, "flagger") || strings.Contains(img, "flagger") {
				flaggerDetected = true
			}
		}
	}

	// 2. Scan all Deployments
	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var entries []PDDeploymentEntry
	var issues []PDIssue
	nsStats := make(map[string]*PDNSStat)

	recreateCount := 0
	rollingCount := 0
	stalledCount := 0
	highReplica := 0
	noProgressDeadline := 0
	canaryAnno := 0

	for i := range deployments.Items {
		dep := &deployments.Items[i]
		if isSystemNamespace(dep.Namespace) {
			continue
		}

		entry := PDDeploymentEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
		}

		// Strategy
		strategy := "Rolling"
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			strategy = "Recreate"
			recreateCount++
		} else {
			rollingCount++
		}
		entry.Strategy = strategy

		// Replica counts
		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}
		entry.Replicas = replicas
		entry.UpdatedReplicas = int(dep.Status.UpdatedReplicas)
		entry.ReadyReplicas = int(dep.Status.ReadyReplicas)

		// Progress deadline
		if dep.Spec.ProgressDeadlineSeconds != nil {
			entry.HasProgressDeadline = true
		} else {
			noProgressDeadline++
			issues = append(issues, PDIssue{
				Severity: "info",
				Type:     "no-progress-deadline",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  "No progressDeadlineSeconds set — stalled rollouts may not be detected automatically",
			})
		}

		// Canary annotations (Flagger or custom)
		if dep.Annotations != nil {
			if w, ok := dep.Annotations["canary.flagger.app/weight"]; ok {
				entry.HasCanaryAnnotation = true
				entry.CanaryWeight = w
				canaryAnno++
			}
			if _, ok := dep.Annotations["fluentd.flagger.app/ready"]; ok {
				entry.HasCanaryAnnotation = true
				canaryAnno++
			}
			for k := range dep.Annotations {
				if strings.Contains(k, "canary") || strings.Contains(k, "flagger") {
					entry.HasCanaryAnnotation = true
					canaryAnno++
					break
				}
			}
		}

		// Check for stalled rollout (updatedReplicas < replicas for significant time)
		if dep.Status.UpdatedReplicas < int32(replicas) && replicas > 0 {
			entry.Stalled = true
			stalledCount++
			issues = append(issues, PDIssue{
				Severity: "warning",
				Type:     "stalled-rollout",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Rollout stalled: %d/%d replicas updated — check pod events and readiness", dep.Status.UpdatedReplicas, replicas),
			})
		}

		// High replica count without progressive delivery
		if replicas >= 10 && !entry.HasCanaryAnnotation && strategy == "Rolling" {
			highReplica++
			issues = append(issues, PDIssue{
				Severity: "info",
				Type:     "high-replica-no-canary",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment has %d replicas but no canary/progressive delivery — consider Argo Rollouts or Flagger for safer rollouts", replicas),
			})
		}

		// Recreate strategy with multiple replicas is risky
		if strategy == "Recreate" && replicas > 1 {
			issues = append(issues, PDIssue{
				Severity: "warning",
				Type:     "recreate-with-replicas",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Recreate strategy with %d replicas causes downtime during updates — use RollingUpdate instead", replicas),
			})
		}

		// Check for RollbackInProgress condition
		for _, cond := range dep.Status.Conditions {
			if cond.Type == "Progressing" && cond.Reason == "ProgressDeadlineExceeded" {
				entry.Stalled = true
				issues = append(issues, PDIssue{
					Severity: "critical",
					Type:     "progress-deadline-exceeded",
					Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
					Message:  "ProgressDeadlineExceeded — rollout has failed, check pod events and consider rollback",
				})
			}
			if cond.Type == "ReplicaFailure" && cond.Status == corev1.ConditionTrue {
				issues = append(issues, PDIssue{
					Severity: "warning",
					Type:     "replica-failure",
					Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
					Message:  fmt.Sprintf("Replica failure condition: %s", cond.Message),
				})
			}
		}

		entry.RiskLevel = assessPDRisk(entry)
		entries = append(entries, entry)

		// Update namespace stats
		if _, ok := nsStats[dep.Namespace]; !ok {
			nsStats[dep.Namespace] = &PDNSStat{Namespace: dep.Namespace}
		}
		ns := nsStats[dep.Namespace]
		ns.DeploymentCount++
		if entry.Stalled {
			ns.StalledRollouts++
		}
		if strategy == "Recreate" {
			ns.RecreateStrategy++
		}
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].StalledRollouts > result.ByNamespace[j].StalledRollouts
	})

	// Sort entries by risk
	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if recreateCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) use Recreate strategy — switch to RollingUpdate for zero-downtime updates", recreateCount))
	}
	if stalledCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) have stalled rollouts — check 'kubectl rollout status' and pod events", stalledCount))
	}
	if noProgressDeadline > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d deployment(s) have no progressDeadlineSeconds — set it to detect stalled rollouts automatically", noProgressDeadline))
	}
	if highReplica > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d high-replica deployment(s) lack progressive delivery — consider Argo Rollouts or Flagger for canary deployments", highReplica))
	}
	if argoRollouts == 0 && !flaggerDetected && highReplica > 0 {
		recommendations = append(recommendations, "No Argo Rollouts or Flagger detected — install for progressive delivery capabilities (canary, blue-green, A/B testing)")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Progressive delivery is healthy — all deployments use proper rollout strategies")
	}

	result.Deployments = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = PDSummary{
		TotalDeployments:   len(entries),
		WithCanary:         canaryAnno,
		RecreateStrategy:   recreateCount,
		RollingStrategy:    rollingCount,
		StalledRollouts:    stalledCount,
		HighReplicaCount:   highReplica,
		NoProgressDeadline: noProgressDeadline,
		ArgoRollouts:       argoRollouts,
		FlaggerDetected:    flaggerDetected,
	}
	result.HealthScore = computePDHealthScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessPDRisk determines risk level of a deployment's progressive delivery posture.
func assessPDRisk(entry PDDeploymentEntry) string {
	risk := 0
	if entry.Stalled {
		risk += 3
	}
	if entry.Strategy == "Recreate" && entry.Replicas > 1 {
		risk += 2
	}
	if !entry.HasProgressDeadline {
		risk += 1
	}
	switch {
	case risk >= 4:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computePDHealthScore computes a 0-100 health score.
func computePDHealthScore(s PDSummary, issueCount int) int {
	if s.TotalDeployments == 0 {
		return 100
	}
	score := 100
	score -= s.StalledRollouts * 10
	score -= s.RecreateStrategy * 3
	score -= s.NoProgressDeadline * 1
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
