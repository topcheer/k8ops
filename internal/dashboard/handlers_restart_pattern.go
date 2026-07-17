package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestartPatternResult analyzes pod restart patterns to detect chronic issues.
// Unlike crash-loop detection (which looks at current state), this engine
// looks at restart history to find: cyclical restart patterns, periodic
// OOM kills, configuration-triggered restarts, and time-correlated failures.
type RestartPatternResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         RestartPatSummary `json:"summary"`
	Workloads       []RestartPatEntry `json:"workloads"`
	ByPattern       []PatternType     `json:"byPattern"`
	TimeCorrelation []TimeCorrEntry   `json:"timeCorrelation"`
	ChronicIssues   []ChronicIssue    `json:"chronicIssues"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type RestartPatSummary struct {
	TotalPods         int     `json:"totalPods"`
	PodsWithRestarts  int     `json:"podsWithRestarts"`
	TotalRestarts     int     `json:"totalRestarts"`
	AvgRestartsPerPod float64 `json:"avgRestartsPerPod"`
	OOMKills          int     `json:"oomKills"`
	CrashLoops        int     `json:"crashLoops"`
	ChronicCount      int     `json:"chronicCount"` // pods with >10 restarts
	MaxRestarts       int     `json:"maxRestarts"`
}

type RestartPatEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Restarts    int    `json:"restarts"`
	Pattern     string `json:"pattern"` // periodic, burst, chronic, none
	OOMKills    int    `json:"oomKills"`
	LastRestart string `json:"lastRestart"`
	Severity    string `json:"severity"`
	RootCause   string `json:"rootCauseGuess"`
}

type PatternType struct {
	Pattern string  `json:"pattern"`
	Count   int     `json:"count"`
	Pct     float64 `json:"pct"`
}

type TimeCorrEntry struct {
	Hour       int  `json:"hour"`
	Restarts   int  `json:"restarts"`
	OOMKills   int  `json:"oomKills"`
	Correlated bool `json:"correlated"`
}

type ChronicIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Restarts  int    `json:"restarts"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
	Fix       string `json:"suggestedFix"`
}

// handleRestartPattern handles GET /api/operations/restart-pattern
func (s *Server) handleRestartPattern(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RestartPatternResult{ScannedAt: time.Now()}
	now := time.Now()

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})

	// Hourly restart correlation
	hourlyRestarts := [24]int{}
	hourlyOOM := [24]int{}

	for _, evt := range events.Items {
		age := now.Sub(evt.CreationTimestamp.Time)
		if age > 7*24*time.Hour {
			continue
		}
		reason := strings.ToLower(evt.Reason)
		if reason == "pulled" || reason == "created" || reason == "started" {
			continue
		}
		hour := evt.CreationTimestamp.Time.Hour()
		if strings.Contains(reason, "kill") || strings.Contains(reason, "oom") {
			hourlyOOM[hour]++
		}
		if strings.Contains(reason, "backoff") || strings.Contains(reason, "unhealthy") || strings.Contains(reason, "failed") {
			hourlyRestarts[hour]++
		}
	}

	// Build time correlation
	avgRestarts := 0
	for _, r := range hourlyRestarts {
		avgRestarts += r
	}
	if len(hourlyRestarts) > 0 {
		avgRestarts /= 24
	}
	for h := 0; h < 24; h++ {
		corr := hourlyRestarts[h] > avgRestarts*2 && avgRestarts > 0
		result.TimeCorrelation = append(result.TimeCorrelation, TimeCorrEntry{
			Hour: h, Restarts: hourlyRestarts[h], OOMKills: hourlyOOM[h], Correlated: corr,
		})
	}

	// Analyze pods
	patternCounts := map[string]int{}
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := 0
		oomKills := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				oomKills++
			}
		}

		if totalRestarts == 0 {
			continue
		}
		result.Summary.PodsWithRestarts++
		result.Summary.TotalRestarts += totalRestarts
		if totalRestarts > result.Summary.MaxRestarts {
			result.Summary.MaxRestarts = totalRestarts
		}

		// Classify pattern
		pattern := "none"
		severity := "low"
		rootCause := ""
		switch {
		case totalRestarts > 20:
			pattern = "chronic"
			severity = "critical"
			rootCause = "Chronic instability — check logs, resource limits, and dependencies"
			result.Summary.ChronicCount++
		case oomKills > 0:
			pattern = "oom-cycle"
			severity = "high"
			rootCause = fmt.Sprintf("OOM killed %d time(s) — increase memory limit or fix leak", oomKills)
			result.Summary.OOMKills += oomKills
		case totalRestarts > 5:
			pattern = "periodic"
			severity = "medium"
			rootCause = "Periodic restarts — check liveness probe and health check config"
		default:
			pattern = "sporadic"
			severity = "low"
			rootCause = "Sporadic restarts — likely transient"
		}

		kind := "Pod"
		if len(pod.OwnerReferences) > 0 {
			kind = pod.OwnerReferences[0].Kind
		}

		lastRestart := "unknown"
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				lastRestart = cs.LastTerminationState.Terminated.FinishedAt.Time.Format("2006-01-02 15:04")
				break
			}
		}

		result.Workloads = append(result.Workloads, RestartPatEntry{
			Name: pod.Name, Namespace: pod.Namespace, Kind: kind,
			Restarts: totalRestarts, Pattern: pattern,
			OOMKills: oomKills, LastRestart: lastRestart,
			Severity: severity, RootCause: rootCause,
		})

		patternCounts[pattern]++

		// Chronic issues
		if totalRestarts > 10 {
			fix := "investigate logs and resource limits"
			if oomKills > 0 {
				fix = fmt.Sprintf("increase memory limit (OOM killed %d times)", oomKills)
			}
			result.ChronicIssues = append(result.ChronicIssues, ChronicIssue{
				Name: pod.Name, Namespace: pod.Namespace,
				Restarts: totalRestarts, Issue: rootCause,
				Severity: severity, Fix: fix,
			})
		}
	}

	// CrashLoop count
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.CrashLoops++
			}
		}
	}

	// Finalize summary
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestartsPerPod = float64(result.Summary.TotalRestarts) / float64(result.Summary.TotalPods)
	}

	// Pattern stats
	for _, p := range []string{"chronic", "oom-cycle", "periodic", "sporadic", "none"} {
		count := patternCounts[p]
		if count == 0 && p != "none" {
			continue
		}
		pct := 0.0
		if result.Summary.PodsWithRestarts > 0 {
			pct = float64(count) / float64(result.Summary.PodsWithRestarts) * 100
		}
		result.ByPattern = append(result.ByPattern, PatternType{Pattern: p, Count: count, Pct: pct})
	}

	// Sort workloads by restarts
	sort.Slice(result.Workloads, func(i, j int) bool { return result.Workloads[i].Restarts > result.Workloads[j].Restarts })
	if len(result.Workloads) > 30 {
		result.Workloads = result.Workloads[:30]
	}

	result.HealthScore = computeRestartScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)
	result.Recommendations = generateRestartRecs(result)

	writeJSON(w, result)
}

func computeRestartScore(s RestartPatSummary) int {
	score := 100
	score -= minInt(s.ChronicCount*10, 30)
	score -= minInt(s.CrashLoops*5, 20)
	score -= minInt(s.OOMKills*3, 15)
	if score < 0 {
		score = 0
	}
	return score
}

func generateRestartRecs(r RestartPatternResult) []string {
	var recs []string
	recs = append(recs, fmt.Sprintf("Restart patterns: %d/%d pods with restarts (%d total), %d chronic, %d OOM kills — score %d/100",
		r.Summary.PodsWithRestarts, r.Summary.TotalPods, r.Summary.TotalRestarts,
		r.Summary.ChronicCount, r.Summary.OOMKills, r.HealthScore))
	for _, ci := range r.ChronicIssues {
		recs = append(recs, fmt.Sprintf("CHRONIC: %s/%s (%d restarts) — %s → %s", ci.Namespace, ci.Name, ci.Restarts, ci.Issue, ci.Fix))
	}
	// Time-correlated restarts
	for _, tc := range r.TimeCorrelation {
		if tc.Correlated {
			recs = append(recs, fmt.Sprintf("Time correlation: hour %02d:00 has %d restarts (correlated spike)", tc.Hour, tc.Restarts))
			break
		}
	}
	if r.Summary.CrashLoops > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) currently in CrashLoopBackOff", r.Summary.CrashLoops))
	}
	return recs
}
