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

// PRResult is the pod restart reason analysis.
type PRResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         PRSummary      `json:"summary"`
	ByReason        map[string]int `json:"byReason"`
	TopRestarters   []PREntry      `json:"topRestarters"` // pods with most restarts
	ByWorkload      []PREntry      `json:"byWorkload"`
	ByNamespace     []PRNSEntry    `json:"byNamespace"`
	OOMKills        []PREntry      `json:"oomKills"`
	AppErrors       []PREntry      `json:"appErrors"`    // ExitCode != 0, not OOM
	ConfigErrors    []PREntry      `json:"configErrors"` // CreateContainerConfigError etc
	Issues          []PRIssue      `json:"issues"`
	Recommendations []string       `json:"recommendations"`
}

// PRSummary aggregates restart statistics.
type PRSummary struct {
	TotalPods        int   `json:"totalPods"`
	RestartedPods    int   `json:"restartedPods"` // at least 1 restart
	HealthyPods      int   `json:"healthyPods"`
	TotalRestarts    int   `json:"totalRestarts"`
	OOMKills         int   `json:"oomKills"`
	AppErrors        int   `json:"appErrors"`        // exit code != 0, not OOM
	ConfigErrors     int   `json:"configErrors"`     // CreateContainerError etc
	DeadlineExceeded int   `json:"deadlineExceeded"` // Jobs only
	Completed        int   `json:"completed"`        // normal exit (exit 0)
	UnknownReason    int   `json:"unknownReason"`
	MaxRestarts      int32 `json:"maxRestarts"`    // single container max
	StabilityScore   int   `json:"stabilityScore"` // 0-100
}

// PREntry describes one container's restart info.
type PREntry struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	ContainerName string `json:"containerName"`
	Image         string `json:"image"`
	RestartCount  int32  `json:"restartCount"`
	LastReason    string `json:"lastReason"` // last termination reason
	LastExitCode  int32  `json:"lastExitCode"`
	Age           string `json:"age"`
	RiskLevel     string `json:"riskLevel"`
}

// PRNSEntry per-namespace restart stats.
type PRNSEntry struct {
	Namespace      string         `json:"namespace"`
	TotalPods      int            `json:"totalPods"`
	RestartedPods  int            `json:"restartedPods"`
	TotalRestarts  int            `json:"totalRestarts"`
	ByReason       map[string]int `json:"byReason"`
	StabilityScore int            `json:"stabilityScore"`
}

// PRIssue is a detected restart problem.
type PRIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleRestartReasons analyzes pod restart reasons across the cluster.
// GET /api/operations/restart-reasons
func (s *Server) handleRestartReasons(w http.ResponseWriter, r *http.Request) {
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

	result := PRResult{ScannedAt: time.Now(), ByReason: make(map[string]int)}
	nsMap := make(map[string]*PRNSEntry)
	now := time.Now()

	for _, pod := range pods.Items {
		result.Summary.TotalPods++
		nsStat := prGetOrCreateNS(nsMap, pod.Namespace)
		nsStat.TotalPods++

		podHasRestarts := false

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount == 0 {
				continue
			}

			podHasRestarts = true
			result.Summary.TotalRestarts += int(cs.RestartCount)

			entry := PREntry{
				PodName:       pod.Name,
				Namespace:     pod.Namespace,
				ContainerName: cs.Name,
				Image:         cs.Image,
				RestartCount:  cs.RestartCount,
				Age:           now.Sub(pod.CreationTimestamp.Time).Round(time.Minute).String(),
			}

			// Extract last termination reason and exit code
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				entry.LastReason = term.Reason
				entry.LastExitCode = term.ExitCode
			} else if cs.LastTerminationState.Waiting != nil {
				entry.LastReason = cs.LastTerminationState.Waiting.Reason
			}

			if entry.LastReason == "" {
				entry.LastReason = "Unknown"
			}

			result.ByReason[entry.LastReason]++
			entry.RiskLevel = prAssessRisk(entry)

			// Categorize
			switch {
			case entry.LastReason == "OOMKilled" || entry.LastExitCode == 137:
				result.Summary.OOMKills++
				result.OOMKills = append(result.OOMKills, entry)
				result.Issues = append(result.Issues, PRIssue{
					Severity: "critical", Type: "oom-kill",
					Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
					Message:  fmt.Sprintf("Container %s in %s/%s OOMKilled (%d restarts) — increase memory limit or fix memory leak", cs.Name, pod.Namespace, pod.Name, cs.RestartCount),
				})
			case prIsConfigError(entry.LastReason):
				result.Summary.ConfigErrors++
				result.ConfigErrors = append(result.ConfigErrors, entry)
			case entry.LastReason == "DeadlineExceeded":
				result.Summary.DeadlineExceeded++
			case entry.LastExitCode == 0:
				result.Summary.Completed++
			case entry.LastExitCode != 0:
				result.Summary.AppErrors++
				result.AppErrors = append(result.AppErrors, entry)
				if cs.RestartCount > 10 {
					result.Issues = append(result.Issues, PRIssue{
						Severity: "warning", Type: "app-error",
						Resource: fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, cs.Name),
						Message:  fmt.Sprintf("Container %s in %s/%s exited %d times (exitCode=%d) — check application logs", cs.Name, pod.Namespace, pod.Name, cs.RestartCount, entry.LastExitCode),
					})
				}
			default:
				result.Summary.UnknownReason++
			}

			// Track max
			if cs.RestartCount > result.Summary.MaxRestarts {
				result.Summary.MaxRestarts = cs.RestartCount
			}

			// Namespace tracking
			nsStat.TotalRestarts += int(cs.RestartCount)
			nsStat.ByReason[entry.LastReason]++

			result.ByWorkload = append(result.ByWorkload, entry)
			result.TopRestarters = append(result.TopRestarters, entry)
		}

		if podHasRestarts {
			result.Summary.RestartedPods++
			nsStat.RestartedPods++
		} else {
			result.Summary.HealthyPods++
		}
	}

	// Finalize namespace scores
	for _, nsStat := range nsMap {
		nsStat.StabilityScore = prNSScore(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.TopRestarters, func(i, j int) bool {
		return result.TopRestarters[i].RestartCount > result.TopRestarters[j].RestartCount
	})
	if len(result.TopRestarters) > 20 {
		result.TopRestarters = result.TopRestarters[:20]
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalRestarts > result.ByNamespace[j].TotalRestarts
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return prIssueRank(result.Issues[i].Severity) < prIssueRank(result.Issues[j].Severity)
	})

	result.Summary.StabilityScore = prScore(result.Summary)
	result.Recommendations = prGenRecs(result.Summary, result.OOMKills, result.AppErrors)

	writeJSON(w, result)
}

// prIsConfigError checks if reason is a config-related error.
func prIsConfigError(reason string) bool {
	switch reason {
	case "CreateContainerConfigError", "CreateContainerError",
		"InvalidImageName", "ErrImagePull", "ImagePullBackOff":
		return true
	}
	return false
}

// prAssessRisk determines risk level based on restart count and reason.
func prAssessRisk(entry PREntry) string {
	if entry.LastReason == "OOMKilled" || entry.LastExitCode == 137 {
		if entry.RestartCount > 10 {
			return "critical"
		}
		return "high"
	}
	if prIsConfigError(entry.LastReason) {
		return "high"
	}
	if entry.RestartCount > 20 {
		return "critical"
	}
	if entry.RestartCount > 5 {
		return "high"
	}
	return "medium"
}

// prScore computes cluster stability 0-100.
func prScore(s PRSummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	restartRate := float64(s.RestartedPods) / float64(s.TotalPods) * 100
	score := 100 - int(restartRate*1.5)
	if score < 0 {
		score = 0
	}
	return score
}

// prNSScore computes namespace stability 0-100.
func prNSScore(ns PRNSEntry) int {
	if ns.TotalPods == 0 {
		return 100
	}
	restartRate := float64(ns.RestartedPods) / float64(ns.TotalPods) * 100
	score := 100 - int(restartRate*1.5)
	if score < 0 {
		score = 0
	}
	return score
}

// prGenRecs produces actionable advice.
func prGenRecs(s PRSummary, oomKills []PREntry, appErrors []PREntry) []string {
	var recs []string

	if s.OOMKills > 0 {
		recs = append(recs, fmt.Sprintf("%d OOMKilled container(s) — increase memory limits, fix memory leaks, or add VPA for auto-tuning", s.OOMKills))
	}
	if s.AppErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) exited with non-zero code — check application logs for crash causes", s.AppErrors))
	}
	if s.ConfigErrors > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) failed to start (config/image errors) — verify image name, entrypoint, and volume mounts", s.ConfigErrors))
	}
	if s.DeadlineExceeded > 0 {
		recs = append(recs, fmt.Sprintf("%d Job(s) exceeded activeDeadlineSeconds — increase timeout or optimize workload", s.DeadlineExceeded))
	}
	if s.MaxRestarts > 50 {
		recs = append(recs, fmt.Sprintf("Max restart count is %d — consider backoff limit or investigate persistent crash loop", s.MaxRestarts))
	}
	if s.TotalRestarts > 100 {
		recs = append(recs, fmt.Sprintf("Total %d restarts across cluster — high churn detected, prioritize fixing top restarters", s.TotalRestarts))
	}
	if s.StabilityScore < 70 {
		recs = append(recs, fmt.Sprintf("Cluster stability score is %d/100 — multiple pods are restarting", s.StabilityScore))
	}
	if s.RestartedPods == 0 {
		recs = append(recs, "No pod restarts detected — cluster is stable")
	}

	return recs
}

func prGetOrCreateNS(m map[string]*PRNSEntry, ns string) *PRNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &PRNSEntry{Namespace: ns, ByReason: make(map[string]int)}
	m[ns] = e
	return e
}

func prIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// Ensure imports used
var _ = strings.Contains
var _ = corev1.PodSpec{}
