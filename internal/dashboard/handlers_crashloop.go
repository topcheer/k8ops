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

// CrashLoopResult is the CrashLoopBackOff analysis.
type CrashLoopResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         CrashLoopSummary `json:"summary"`
	AffectedPods    []CrashPodEntry  `json:"affectedPods"`
	ByNamespace     []CrashNSStat    `json:"byNamespace"`
	ByPattern       []CrashPattern   `json:"patterns"`
	TopCrashers     []CrashPodEntry  `json:"topCrashers"`
	Issues          []CrashIssue     `json:"issues"`
	Recommendations []string         `json:"recommendations"`
}

// CrashLoopSummary aggregates crash loop statistics.
type CrashLoopSummary struct {
	TotalPods         int `json:"totalPods"`
	CrashLoopPods     int `json:"crashLoopPods"`     // pods in CrashLoopBackOff
	HighRestartPods   int `json:"highRestartPods"`   // >=5 restarts
	FailedContainers  int `json:"failedContainers"`  // containers with non-zero restart count
	RapidRestarts     int `json:"rapidRestarts"`     // restarts within last hour
	PatternRolling    int `json:"patternRolling"`    // new pods crash (deployment issue)
	PatternConfig     int `json:"patternConfig"`     // exit code 1 (config/app error)
	PatternOOM        int `json:"patternOOM"`        // OOMKilled
	PatternPermission int `json:"patternPermission"` // permission denied
	PatternImage      int `json:"patternImage"`      // image pull issues
	HealthScore       int `json:"healthScore"`       // 0-100
}

// CrashPodEntry describes one crashing pod.
type CrashPodEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Node           string `json:"node"`
	Deployment     string `json:"deployment,omitempty"` // owner deployment
	RestartCount   int32  `json:"restartCount"`
	ContainerName  string `json:"containerName"`
	LastState      string `json:"lastState"` // last termination reason
	ExitCode       int32  `json:"exitCode"`
	StartedAt      string `json:"startedAt"`
	Age            string `json:"age"`
	CrashInterval  int    `json:"crashIntervalSec"` // estimated seconds between restarts
	Pattern        string `json:"pattern"`          // classification
	RiskLevel      string `json:"riskLevel"`
	SuggestedCause string `json:"suggestedCause"`
}

// CrashNSStat per-namespace crash stats.
type CrashNSStat struct {
	Namespace     string `json:"namespace"`
	CrashCount    int    `json:"crashCount"`
	TotalRestarts int    `json:"totalRestarts"`
}

// CrashPattern groups crashing pods by root cause pattern.
type CrashPattern struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	PodCount    int      `json:"podCount"`
	Pods        []string `json:"pods"`
}

// CrashIssue is a detected problem.
type CrashIssue struct {
	Pod      string `json:"pod"`
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

// handleCrashLoop analyzes CrashLoopBackOff and crash patterns.
// GET /api/operations/crashloop
func (s *Server) handleCrashLoop(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := CrashLoopResult{ScannedAt: time.Now()}
	result.Summary.TotalPods = len(pods.Items)
	nsMap := make(map[string]*CrashNSStat)
	patternMap := make(map[string][]string)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		var hasCrash bool
		var podMaxRestarts int32
		var crashEntry *CrashPodEntry

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount == 0 {
				continue
			}

			hasCrash = true
			result.Summary.FailedContainers++

			entry := CrashPodEntry{
				Name:          pod.Name,
				Namespace:     pod.Namespace,
				Node:          pod.Spec.NodeName,
				ContainerName: cs.Name,
				RestartCount:  cs.RestartCount,
			}

			// Owner reference (deployment)
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == "ReplicaSet" {
					// Strip hash suffix to get deployment name
					parts := strings.Split(ref.Name, "-")
					if len(parts) > 2 {
						entry.Deployment = strings.Join(parts[:len(parts)-2], "-")
					} else {
						entry.Deployment = ref.Name
					}
				}
			}

			// Last termination state
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				entry.LastState = term.Reason
				entry.ExitCode = term.ExitCode
				if !term.StartedAt.IsZero() {
					entry.StartedAt = term.StartedAt.Format(time.RFC3339)
				}
			}

			// Age
			entry.Age = time.Since(pod.CreationTimestamp.Time).Round(time.Minute).String()

			// Crash interval estimate: pod age / restart count
			ageSec := int(time.Since(pod.CreationTimestamp.Time).Seconds())
			if cs.RestartCount > 0 && ageSec > 0 {
				entry.CrashInterval = ageSec / int(cs.RestartCount)
			}

			// Check for CrashLoopBackOff state
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.CrashLoopPods++
			}

			// High restart count
			if cs.RestartCount >= 5 {
				result.Summary.HighRestartPods++
			}

			// Rapid restart (crash within last hour)
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				if time.Since(term.FinishedAt.Time) < time.Hour {
					result.Summary.RapidRestarts++
				}
			}

			// Classify crash pattern
			entry.Pattern, entry.SuggestedCause = classifyCrash(entry)
			patternMap[entry.Pattern] = append(patternMap[entry.Pattern], fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))

			// Aggregate by pattern
			switch entry.Pattern {
			case "oom":
				result.Summary.PatternOOM++
			case "config-error":
				result.Summary.PatternConfig++
			case "permission-denied":
				result.Summary.PatternPermission++
			case "image-issue":
				result.Summary.PatternImage++
			case "rolling-crash":
				result.Summary.PatternRolling++
			}

			entry.RiskLevel = assessCrashRisk(entry)

			if cs.RestartCount > podMaxRestarts {
				podMaxRestarts = cs.RestartCount
				crashEntry = &entry
			}
		}

		if hasCrash && crashEntry != nil {
			// Namespace stats
			nsStat := getOrCreateCrashNS(nsMap, pod.Namespace)
			nsStat.CrashCount++
			nsStat.TotalRestarts += int(podMaxRestarts)

			result.AffectedPods = append(result.AffectedPods, *crashEntry)

			// Generate issues for high-risk
			if crashEntry.RiskLevel == "critical" || crashEntry.RiskLevel == "high" {
				result.Issues = append(result.Issues, CrashIssue{
					Pod:      fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
					Severity: crashEntry.RiskLevel,
					Type:     crashEntry.Pattern,
					Message:  fmt.Sprintf("%s: %d restarts, last: %s (exit %d)", pod.Name, crashEntry.RestartCount, crashEntry.LastState, crashEntry.ExitCode),
				})
			}
		}
	}

	// Sort affected pods by risk
	sort.Slice(result.AffectedPods, func(i, j int) bool {
		return crashRiskRank(result.AffectedPods[i].RiskLevel) < crashRiskRank(result.AffectedPods[j].RiskLevel)
	})

	// Top crashers (max 10)
	if len(result.AffectedPods) > 10 {
		result.TopCrashers = result.AffectedPods[:10]
	} else {
		result.TopCrashers = result.AffectedPods
	}

	// Namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalRestarts > result.ByNamespace[j].TotalRestarts
	})

	// Pattern summary
	patternDescs := map[string]string{
		"oom":               "Containers killed by OOM — increase memory limits or fix memory leaks",
		"config-error":      "Exit code 1 — application crash, check config, dependencies, or startup logic",
		"permission-denied": "Permission denied — check securityContext, RBAC, or volume mount permissions",
		"image-issue":       "Image pull failures — verify image name, registry credentials, or network access",
		"rolling-crash":     "Newly created pods crashing — likely deployment/config issue introduced in latest rollout",
		"unknown":           "Unclassified crash pattern — inspect pod logs for details",
	}
	for patternType, podNames := range patternMap {
		result.ByPattern = append(result.ByPattern, CrashPattern{
			Type:        patternType,
			Description: patternDescs[patternType],
			PodCount:    len(podNames),
			Pods:        podNames,
		})
	}
	sort.Slice(result.ByPattern, func(i, j int) bool {
		return result.ByPattern[i].PodCount > result.ByPattern[j].PodCount
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return crashRiskRank(result.Issues[i].Severity) < crashRiskRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = calculateCrashScore(result.Summary)
	result.Recommendations = generateCrashRecs(result.Summary, result.ByPattern, result.TopCrashers)

	writeJSON(w, result)
}

// classifyCrash determines the crash pattern and likely cause.
func classifyCrash(entry CrashPodEntry) (pattern, cause string) {
	// OOMKilled
	if entry.LastState == "OOMKilled" {
		return "oom", "Container exceeded memory limit — increase memory limit or investigate memory leak"
	}

	// Permission denied
	if entry.ExitCode == 137 || strings.Contains(strings.ToLower(entry.LastState), "permission") {
		return "permission-denied", "Permission denied — check securityContext runAsUser or volume permissions"
	}

	// Image pull issues
	if strings.Contains(strings.ToLower(entry.LastState), "imagepullbackoff") ||
		strings.Contains(strings.ToLower(entry.LastState), "errimagepull") {
		return "image-issue", "Image pull failure — verify image name, tag, and registry credentials"
	}

	// Exit code 1 — typical app/config error
	if entry.ExitCode == 1 {
		return "config-error", "Application error (exit 1) — check config files, env vars, or missing dependencies"
	}

	// Exit code 127 — command not found
	if entry.ExitCode == 127 {
		return "config-error", "Command not found (exit 127) — verify container command/entrypoint"
	}

	// Exit code 126 — permission denied on executable
	if entry.ExitCode == 126 {
		return "permission-denied", "Command not executable (exit 126) — check file permissions in image"
	}

	// Rapid crash loop on new pod (rolling)
	if entry.CrashInterval > 0 && entry.CrashInterval < 30 {
		return "rolling-crash", "Rapid crash loop (<30s interval) — likely startup failure in latest deployment"
	}

	return "unknown", "Unclassified — inspect container logs with kubectl logs for details"
}

// assessCrashRisk determines risk level.
func assessCrashRisk(entry CrashPodEntry) string {
	risk := 0
	if entry.RestartCount >= 10 {
		risk += 30
	} else if entry.RestartCount >= 5 {
		risk += 20
	} else if entry.RestartCount >= 3 {
		risk += 10
	}

	if entry.CrashInterval > 0 && entry.CrashInterval < 30 {
		risk += 15
	}

	if entry.Pattern == "oom" || entry.Pattern == "rolling-crash" {
		risk += 5
	}

	switch {
	case risk >= 30:
		return "critical"
	case risk >= 15:
		return "high"
	case risk >= 5:
		return "medium"
	default:
		return "low"
	}
}

// calculateCrashScore computes 0-100.
func calculateCrashScore(s CrashLoopSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= s.CrashLoopPods * 10
	score -= s.HighRestartPods * 5
	score -= s.RapidRestarts * 8
	score -= s.PatternRolling * 6
	if score < 0 {
		score = 0
	}
	return score
}

// generateCrashRecs produces actionable advice.
func generateCrashRecs(s CrashLoopSummary, patterns []CrashPattern, topCrashers []CrashPodEntry) []string {
	var recs []string

	if s.CrashLoopPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) in CrashLoopBackOff — check logs: kubectl logs <pod> --previous", s.CrashLoopPods))
	}
	if s.RapidRestarts > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) crashed within the last hour — likely a recent deployment or config change", s.RapidRestarts))
	}
	if s.PatternOOM > 0 {
		recs = append(recs, fmt.Sprintf("%d OOM crash pattern(s) — increase memory limits or profile for memory leaks", s.PatternOOM))
	}
	if s.PatternRolling > 0 {
		recs = append(recs, fmt.Sprintf("%d rolling crash pattern(s) — consider rollback: kubectl rollout undo deployment/<name>", s.PatternRolling))
	}
	if s.PatternConfig > 0 {
		recs = append(recs, fmt.Sprintf("%d config error pattern(s) — check env vars, ConfigMaps, and Secrets are mounted correctly", s.PatternConfig))
	}
	if s.PatternPermission > 0 {
		recs = append(recs, fmt.Sprintf("%d permission denied pattern(s) — check securityContext and runAsUser settings", s.PatternPermission))
	}

	if len(topCrashers) > 0 {
		top := topCrashers[0]
		recs = append(recs, fmt.Sprintf("Top crasher: %s/%s (%s) — %d restarts, pattern: %s", top.Namespace, top.Name, top.ContainerName, top.RestartCount, top.Pattern))
	}

	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Crash health score is %d/100 — investigate crash patterns across workloads", s.HealthScore))
	}

	return recs
}

func getOrCreateCrashNS(m map[string]*CrashNSStat, ns string) *CrashNSStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &CrashNSStat{Namespace: ns}
	m[ns] = e
	return e
}

func crashRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}
