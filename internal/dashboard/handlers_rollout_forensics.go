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

// RolloutForensicsResult is the rollout failure forensics & deployment pattern detector.
// It correlates deployment state, pod conditions, and restart patterns to identify
// systematic rollout risks, deployment anti-patterns, and per-workload rollout reliability.
type RolloutForensicsResult struct {
	ScannedAt                   time.Time                     `json:"scannedAt"`
	Summary                     RolloutForensicsSummary       `json:"summary"`
	FailedRollouts              []FailedRollout               `json:"failedRollouts"`
	AntiPatterns                []RolloutForensicsAntiPattern `json:"antiPatterns"`
	ReliabilityScore            []RolloutForensicsReliability `json:"reliabilityScore"`
	RolloutForensicsRiskFactors []RolloutForensicsRiskFactor  `json:"riskFactors"`
	Recommendations             []string                      `json:"recommendations"`
}

// RolloutForensicsSummary aggregates deployment rollout statistics.
type RolloutForensicsSummary struct {
	TotalDeployments int     `json:"totalDeployments"`
	InProgress       int     `json:"inProgress"`     // deployments with ongoing rollout
	Completed        int     `json:"completed"`      // successfully rolled out
	Failed           int     `json:"failed"`         // rollout not progressing
	Stale            int     `json:"stale"`          // no rollout in a long time
	AvgReliability   float64 `json:"avgReliability"` // average rollout reliability score
	HighRiskCount    int     `json:"highRiskCount"`  // deployments with reliability < 50
}

// FailedRollout describes a deployment with rollout issues.
type FailedRollout struct {
	Name              string    `json:"name"`
	Namespace         string    `json:"namespace"`
	Issue             string    `json:"issue"`
	Severity          string    `json:"severity"`
	Progress          string    `json:"progress"` // human-readable progress description
	UpdatedReplicas   int32     `json:"updatedReplicas"`
	ReadyReplicas     int32     `json:"readyReplicas"`
	DesiredReplicas   int32     `json:"desiredReplicas"`
	AvailableReplicas int32     `json:"availableReplicas"`
	LastUpdate        time.Time `json:"lastUpdateTime"`
	RestartCount      int       `json:"podRestarts"`
	FailingPods       int       `json:"failingPods"`
}

// RolloutForensicsAntiPattern identifies a deployment anti-pattern across the cluster.
type RolloutForensicsAntiPattern struct {
	Type     string   `json:"type"` // no-readiness-probe, no-liveness-probe, recreate-strategy, etc.
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Affected int      `json:"affectedCount"`
	Examples []string `json:"examples"` // sample workload names
}

// RolloutForensicsReliability scores each deployment's rollout reliability.
type RolloutForensicsReliability struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Score     int      `json:"score"`     // 0-100
	Grade     string   `json:"grade"`     // A-F
	RiskLevel string   `json:"riskLevel"` // low, moderate, high, critical
	Signals   []string `json:"signals"`   // contributing factors
	HasHPA    bool     `json:"hasHPA"`
	HasPDB    bool     `json:"hasPDB"`
	HasProbes bool     `json:"hasProbes"`
	Strategy  string   `json:"strategy"`
}

// RolloutForensicsRiskFactor describes a cluster-level rollout risk.
type RolloutForensicsRiskFactor struct {
	Factor      string `json:"factor"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
}

// handleRolloutForensics provides rollout failure forensics & deployment pattern detection.
// GET /api/deployment/rollout-forensics
func (s *Server) handleRolloutForensics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RolloutForensicsResult{ScannedAt: time.Now()}
	now := time.Now()
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Collect deployments
	deploys, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list deployments")
		return
	}

	// Collect pods for restart/crash analysis
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod map by owner for restart counting
	type podInfo struct {
		restarts  int
		ready     bool
		phase     corev1.PodPhase
		crashLoop bool
	}
	podMap := make(map[string][]podInfo) // key: namespace/name
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" {
				// Map RS to Deployment by trimming hash suffix
				depName := strings.TrimSuffix(ref.Name, "-"+getRSDeployHash(ref.Name))
				key := pod.Namespace + "/" + depName
				pi := podInfo{phase: pod.Status.Phase}
				ready := true
				for _, cs := range pod.Status.ContainerStatuses {
					pi.restarts += int(cs.RestartCount)
					if !cs.Ready {
						ready = false
					}
					if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
						pi.crashLoop = true
					}
				}
				pi.ready = ready
				podMap[key] = append(podMap[key], pi)
			}
		}
	}

	// Anti-pattern tracking
	noReadinessProbe := []string{}
	noLivenessProbe := []string{}
	recreateStrategy := []string{}
	noResources := []string{}
	singleReplica := []string{}
	noRevisionHistory := []string{}

	totalScore := 0
	scoreCount := 0

	for _, dep := range deploys.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		result.Summary.TotalDeployments++

		key := dep.Namespace + "/" + dep.Name
		podInfos := podMap[key]

		// Calculate pod stats
		totalRestarts := 0
		failingPods := 0
		hasCrashLoop := false
		for _, pi := range podInfos {
			totalRestarts += pi.restarts
			if !pi.ready || pi.phase != corev1.PodRunning {
				failingPods++
			}
			if pi.crashLoop {
				hasCrashLoop = true
			}
		}

		// Check rollout status
		desired := dep.Status.Replicas
		updated := dep.Status.UpdatedReplicas
		ready := dep.Status.ReadyReplicas
		available := dep.Status.AvailableReplicas
		strategy := string(dep.Spec.Strategy.Type)

		// Determine rollout state
		isInProgress := false
		isFailed := false
		issue := ""

		if desired > 0 && updated < desired {
			isInProgress = true
			issue = fmt.Sprintf("Rollout in progress: %d/%d replicas updated", updated, desired)
		}
		if ready < desired && desired > 0 {
			if ready == 0 {
				isFailed = true
				issue = fmt.Sprintf("No ready replicas: 0/%d ready", desired)
			} else if now.Sub(dep.Status.Conditions[len(dep.Status.Conditions)-1].LastUpdateTime.Time) > 10*time.Minute {
				isFailed = true
				issue = fmt.Sprintf("Rollout stalled: %d/%d ready for >10min", ready, desired)
			}
		}
		if hasCrashLoop {
			isFailed = true
			if issue == "" {
				issue = "Pods in CrashLoopBackOff"
			}
		}

		if isInProgress {
			result.Summary.InProgress++
		}
		if isFailed {
			result.Summary.Failed++
			result.FailedRollouts = append(result.FailedRollouts, FailedRollout{
				Name:              dep.Name,
				Namespace:         dep.Namespace,
				Issue:             issue,
				Severity:          "critical",
				Progress:          fmt.Sprintf("updated=%d ready=%d desired=%d available=%d", updated, ready, desired, available),
				UpdatedReplicas:   updated,
				ReadyReplicas:     ready,
				DesiredReplicas:   desired,
				AvailableReplicas: available,
				LastUpdate:        dep.Status.Conditions[len(dep.Status.Conditions)-1].LastUpdateTime.Time,
				RestartCount:      totalRestarts,
				FailingPods:       failingPods,
			})
		}

		// Check staleness (no update in 30 days)
		if dep.Status.ObservedGeneration > 0 {
			if len(dep.Status.Conditions) > 0 {
				lastCond := dep.Status.Conditions[len(dep.Status.Conditions)-1]
				if now.Sub(lastCond.LastUpdateTime.Time) > 30*24*time.Hour {
					result.Summary.Stale++
				}
			}
		}

		// === Anti-pattern detection ===
		// No readiness probe
		hasReadinessProbe := false
		hasLivenessProbe := false
		hasResources := false
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.ReadinessProbe != nil {
				hasReadinessProbe = true
			}
			if c.LivenessProbe != nil {
				hasLivenessProbe = true
			}
			if c.Resources.Requests != nil && len(c.Resources.Requests) > 0 {
				hasResources = true
			}
		}

		if !hasReadinessProbe {
			noReadinessProbe = append(noReadinessProbe, key)
		}
		if !hasLivenessProbe {
			noLivenessProbe = append(noLivenessProbe, key)
		}
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			recreateStrategy = append(recreateStrategy, key)
		}
		if !hasResources {
			noResources = append(noResources, key)
		}
		if dep.Spec.RevisionHistoryLimit == nil || (dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0) {
			noRevisionHistory = append(noRevisionHistory, key)
		}
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas == 1 {
			singleReplica = append(singleReplica, key)
		}

		// === Reliability score ===
		score := 100
		signals := []string{}

		if !hasReadinessProbe {
			score -= 20
			signals = append(signals, "Missing readiness probe — rollout progress cannot be validated")
		}
		if !hasLivenessProbe {
			score -= 10
			signals = append(signals, "Missing liveness probe — dead containers won't be restarted")
		}
		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			score -= 15
			signals = append(signals, "Recreate strategy causes downtime during rollouts")
		}
		if !hasResources {
			score -= 10
			signals = append(signals, "No resource requests — scheduling and HPA unreliable")
		}
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas == 1 {
			score -= 15
			signals = append(signals, "Single replica — no high availability during rollouts")
		}
		if totalRestarts > 10 {
			score -= 15
			signals = append(signals, fmt.Sprintf("High restart count (%d) — unstable workload", totalRestarts))
		}
		if hasCrashLoop {
			score -= 30
			signals = append(signals, "Pods in CrashLoopBackOff — active failure")
		}
		if dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0 {
			score -= 10
			signals = append(signals, "No revision history — rollback impossible")
		}
		if ready < desired && desired > 0 {
			score -= 20
			signals = append(signals, fmt.Sprintf("Not all replicas ready (%d/%d)", ready, desired))
		}

		if score < 0 {
			score = 0
		}

		riskLevel := "low"
		switch {
		case score < 40:
			riskLevel = "critical"
			result.Summary.HighRiskCount++
		case score < 60:
			riskLevel = "high"
			result.Summary.HighRiskCount++
		case score < 80:
			riskLevel = "moderate"
		}

		totalScore += score
		scoreCount++

		result.ReliabilityScore = append(result.ReliabilityScore, RolloutForensicsReliability{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Score:     score,
			Grade:     goldenScoreToGrade(score),
			RiskLevel: riskLevel,
			Signals:   signals,
			HasProbes: hasReadinessProbe && hasLivenessProbe,
			Strategy:  strategy,
		})
	}

	// Sort reliability scores (worst first)
	sort.Slice(result.ReliabilityScore, func(i, j int) bool {
		return result.ReliabilityScore[i].Score < result.ReliabilityScore[j].Score
	})

	// Sort failed rollouts by severity
	sort.Slice(result.FailedRollouts, func(i, j int) bool {
		return result.FailedRollouts[i].RestartCount > result.FailedRollouts[j].RestartCount
	})

	// Average reliability
	if scoreCount > 0 {
		result.Summary.AvgReliability = float64(totalScore) / float64(scoreCount)
	}
	result.Summary.Completed = result.Summary.TotalDeployments - result.Summary.InProgress - result.Summary.Failed

	// === Build anti-patterns ===
	if len(noReadinessProbe) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "no-readiness-probe",
			Severity: "high",
			Title:    "Deployments without readiness probes",
			Detail:   fmt.Sprintf("%d deployments lack readiness probes — Kubernetes cannot determine if new pods are ready to receive traffic, risking rollout failures going undetected", len(noReadinessProbe)),
			Affected: len(noReadinessProbe),
			Examples: takeFirst(noReadinessProbe, 5),
		})
	}
	if len(noLivenessProbe) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "no-liveness-probe",
			Severity: "medium",
			Title:    "Deployments without liveness probes",
			Detail:   fmt.Sprintf("%d deployments lack liveness probes — deadlocked or hung containers won't be automatically restarted", len(noLivenessProbe)),
			Affected: len(noLivenessProbe),
			Examples: takeFirst(noLivenessProbe, 5),
		})
	}
	if len(recreateStrategy) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "recreate-strategy",
			Severity: "high",
			Title:    "Deployments using Recreate strategy",
			Detail:   fmt.Sprintf("%d deployments use Recreate strategy — causes downtime during every rollout as old pods are killed before new ones start", len(recreateStrategy)),
			Affected: len(recreateStrategy),
			Examples: takeFirst(recreateStrategy, 5),
		})
	}
	if len(noResources) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "no-resources",
			Severity: "medium",
			Title:    "Deployments without resource requests",
			Detail:   fmt.Sprintf("%d deployments lack resource requests — HPA cannot function, scheduling is unreliable, and cluster capacity planning is impossible", len(noResources)),
			Affected: len(noResources),
			Examples: takeFirst(noResources, 5),
		})
	}
	if len(singleReplica) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "single-replica",
			Severity: "high",
			Title:    "Single-replica deployments",
			Detail:   fmt.Sprintf("%d deployments run with replicas=1 — no HA, any rollout causes service disruption", len(singleReplica)),
			Affected: len(singleReplica),
			Examples: takeFirst(singleReplica, 5),
		})
	}
	if len(noRevisionHistory) > 0 {
		result.AntiPatterns = append(result.AntiPatterns, RolloutForensicsAntiPattern{
			Type:     "no-revision-history",
			Severity: "medium",
			Title:    "Deployments without revision history",
			Detail:   fmt.Sprintf("%d deployments have revisionHistoryLimit=0 — rollbacks are impossible, deployment failures cannot be quickly reverted", len(noRevisionHistory)),
			Affected: len(noRevisionHistory),
			Examples: takeFirst(noRevisionHistory, 5),
		})
	}

	// === Cluster-level risk factors ===
	if result.Summary.Failed > 0 {
		result.RolloutForensicsRiskFactors = append(result.RolloutForensicsRiskFactors, RolloutForensicsRiskFactor{
			Factor:      "active-rollout-failures",
			Severity:    "critical",
			Description: fmt.Sprintf("%d deployments have active rollout failures", result.Summary.Failed),
			Impact:      "Failed rollouts indicate application or configuration issues that need immediate attention",
		})
	}
	if result.Summary.HighRiskCount > result.Summary.TotalDeployments/3 && result.Summary.TotalDeployments > 0 {
		result.RolloutForensicsRiskFactors = append(result.RolloutForensicsRiskFactors, RolloutForensicsRiskFactor{
			Factor:      "widespread-reliability-issues",
			Severity:    "high",
			Description: fmt.Sprintf("%d/%d deployments have high rollout risk", result.Summary.HighRiskCount, result.Summary.TotalDeployments),
			Impact:      "Large portion of workloads are at risk of failed deployments — systematic improvement needed",
		})
	}
	if len(noReadinessProbe) > result.Summary.TotalDeployments/2 && result.Summary.TotalDeployments > 0 {
		result.RolloutForensicsRiskFactors = append(result.RolloutForensicsRiskFactors, RolloutForensicsRiskFactor{
			Factor:      "missing-observability",
			Severity:    "high",
			Description: fmt.Sprintf("%d deployments lack readiness probes", len(noReadinessProbe)),
			Impact:      "Without readiness probes, Kubernetes cannot properly manage rollout traffic switching",
		})
	}
	if result.Summary.Stale > 0 {
		result.RolloutForensicsRiskFactors = append(result.RolloutForensicsRiskFactors, RolloutForensicsRiskFactor{
			Factor:      "stale-deployments",
			Severity:    "info",
			Description: fmt.Sprintf("%d deployments haven't been updated in 30+ days", result.Summary.Stale),
			Impact:      "Stale deployments may contain security vulnerabilities or miss critical patches",
		})
	}

	// === Recommendations ===
	result.Recommendations = generateRolloutForensicsRecs(result)

	writeJSON(w, result)
}

// generateRolloutForensicsRecs produces actionable recommendations.
func generateRolloutForensicsRecs(result RolloutForensicsResult) []string {
	var recs []string

	if result.Summary.Failed > 0 {
		recs = append(recs, fmt.Sprintf("%d deployments have active rollout failures — investigate immediately", result.Summary.Failed))
	}

	if result.Summary.HighRiskCount > 0 {
		recs = append(recs, fmt.Sprintf("%d deployments have high rollout risk (score <60) — prioritize adding probes, resources, and HA replicas", result.Summary.HighRiskCount))
	}

	for _, ap := range result.AntiPatterns {
		if ap.Severity == "high" || ap.Severity == "critical" {
			recs = append(recs, fmt.Sprintf("[%s] %s (%d affected)", ap.Severity, ap.Title, ap.Affected))
		}
	}

	if result.Summary.AvgReliability > 0 && result.Summary.AvgReliability < 60 {
		recs = append(recs, fmt.Sprintf("Average rollout reliability is %.0f/100 — implement deployment best practices across the cluster", result.Summary.AvgReliability))
	}

	if len(result.ReliabilityScore) > 0 {
		worst := result.ReliabilityScore[0]
		recs = append(recs, fmt.Sprintf("Worst deployment: '%s' in '%s' (score %d, grade %s) — %s", worst.Name, worst.Namespace, worst.Score, worst.Grade, strings.Join(worst.Signals[:min(2, len(worst.Signals))], "; ")))
	}

	if len(recs) == 0 {
		recs = append(recs, "All deployments have healthy rollouts — maintain current deployment practices")
	}

	return recs
}

// getRSDeployHash extracts the hash suffix from a ReplicaSet name for Deployment mapping.
func getRSDeployHash(rsName string) string {
	return getRSSuffix(rsName)
}

// takeFirst returns the first n elements of a slice.
func takeFirst(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
