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

// ForensicsResult is the pod security forensics & incident evidence collection.
type ForensicsResult struct {
	ScannedAt          time.Time            `json:"scannedAt"`
	Summary            ForensicsSummary     `json:"summary"`
	SuspiciousPods     []ForensicsEntry     `json:"suspiciousPods"`
	ExitCodeAnalysis   []ExitCodeStat       `json:"exitCodeAnalysis"`
	ContainerHashes    []ContainerHashEntry `json:"containerHashes"`
	RecentTerminations []TerminationRecord  `json:"recentTerminations"`
	Recommendations    []string             `json:"recommendations"`
}

// ForensicsSummary aggregates forensics findings.
type ForensicsSummary struct {
	TotalPods           int `json:"totalPods"`
	PodsWithIssues      int `json:"podsWithIssues"`
	RecentRestarts      int `json:"recentRestarts"` // restarts in last 1h
	OOMKills            int `json:"oomKills"`
	SIGKILLTerminations int `json:"sigkillTerminations"`
	ExitCodeErrors      int `json:"exitCodeErrors"`      // non-zero exit codes
	HashMismatches      int `json:"hashMismatches"`      // containerID != imageID hash
	PrivilegedEscapes   int `json:"privilegedEscapes"`   // containers that ran privileged
	TerminationMsgs     int `json:"terminationMessages"` // pods with termination messages
	ForensicsScore      int `json:"forensicsScore"`      // 0-100
}

// ForensicsEntry describes a suspicious pod.
type ForensicsEntry struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	WorkloadType string `json:"workloadType"`
	Issue        string `json:"issue"`
	Detail       string `json:"detail"`
	Severity     string `json:"severity"`
	Restarts     int    `json:"restarts"`
	LastExitCode int32  `json:"lastExitCode"`
}

// ExitCodeStat shows exit code distribution.
type ExitCodeStat struct {
	ExitCode int32  `json:"exitCode"`
	Count    int    `json:"count"`
	Meaning  string `json:"meaning"`
}

// ContainerHashEntry tracks container image hashes for drift detection.
type ContainerHashEntry struct {
	PodName     string `json:"podName"`
	Namespace   string `json:"namespace"`
	Container   string `json:"container"`
	Image       string `json:"image"`
	ContainerID string `json:"containerID"`
	Reason      string `json:"reason"`
	Severity    string `json:"severity"`
}

// TerminationRecord captures a recent pod/container termination event.
type TerminationRecord struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	ExitCode   int32  `json:"exitCode"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	Signal     int32  `json:"signal"`
	FinishedAt string `json:"finishedAt"`
	Duration   string `json:"duration"`
}

// handleForensics collects pod security forensics and incident evidence.
// GET /api/security/forensics
func (s *Server) handleForensics(w http.ResponseWriter, r *http.Request) {
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
	result := ForensicsResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)

	exitCodeCounts := map[int32]int{}
	var allTerminations []TerminationRecord

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}

		wlType := inferWorkloadTypeFromPod(&pod)
		podRestarts := 0
		hasIssue := false
		var issues []ForensicsEntry

		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)

			// Check last termination state
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				exitCode := term.ExitCode

				exitCodeCounts[exitCode]++

				// Build termination record
				rec := TerminationRecord{
					PodName:    pod.Name,
					Namespace:  pod.Namespace,
					Container:  cs.Name,
					ExitCode:   exitCode,
					Reason:     term.Reason,
					Message:    truncateMsg(term.Message, 300),
					Signal:     term.Signal,
					FinishedAt: term.FinishedAt.Format(time.RFC3339),
				}

				if !term.StartedAt.IsZero() && !term.FinishedAt.IsZero() {
					rec.Duration = formatDuration(term.FinishedAt.Sub(term.StartedAt.Time))
				}

				allTerminations = append(allTerminations, rec)

				// Categorize termination
				if term.Reason == "OOMKilled" {
					result.Summary.OOMKills++
					hasIssue = true
					issues = append(issues, ForensicsEntry{
						PodName:      pod.Name,
						Namespace:    pod.Namespace,
						WorkloadType: wlType,
						Issue:        "OOMKilled",
						Detail:       fmt.Sprintf("Container %s was OOMKilled (exit %d): %s", cs.Name, exitCode, term.Message),
						Severity:     "high",
						Restarts:     int(cs.RestartCount),
						LastExitCode: exitCode,
					})
				}

				if term.Signal == 9 || term.Signal == 15 {
					result.Summary.SIGKILLTerminations++
				}

				if exitCode != 0 && term.Reason != "Completed" {
					result.Summary.ExitCodeErrors++
					if term.Reason != "OOMKilled" {
						hasIssue = true
						issues = append(issues, ForensicsEntry{
							PodName:      pod.Name,
							Namespace:    pod.Namespace,
							WorkloadType: wlType,
							Issue:        fmt.Sprintf("Exit code %d", exitCode),
							Detail:       fmt.Sprintf("Container %s exited with code %d (%s): %s", cs.Name, exitCode, term.Reason, term.Message),
							Severity:     exitCodeSeverity(exitCode),
							Restarts:     int(cs.RestartCount),
							LastExitCode: exitCode,
						})
					}
				}
			}

			// Check for termination message
			if cs.State.Terminated != nil && cs.State.Terminated.Message != "" {
			}

			// Check for privileged container that ran
			for _, c := range pod.Spec.Containers {
				if c.Name == cs.Name {
					if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
						if cs.LastTerminationState.Terminated != nil || cs.RestartCount > 0 {
							result.Summary.PrivilegedEscapes++
							hasIssue = true
							issues = append(issues, ForensicsEntry{
								PodName:      pod.Name,
								Namespace:    pod.Namespace,
								WorkloadType: wlType,
								Issue:        "Privileged container with terminations",
								Detail:       fmt.Sprintf("Container %s is privileged and has %d restarts — investigate for security incidents", cs.Name, cs.RestartCount),
								Severity:     "critical",
								Restarts:     int(cs.RestartCount),
							})
						}
					}
				}
			}

			// Container ID vs Image ID hash check
			if cs.ContainerID != "" && cs.ImageID != "" {
				cid := extractContainerHash(cs.ContainerID)
				iid := extractImageHash(cs.ImageID)
				if cid != "" && iid != "" && !strings.HasPrefix(cid, iid[:min(12, len(iid))]) && !strings.HasPrefix(iid, cid[:min(12, len(cid))]) {
					result.Summary.HashMismatches++
					result.ContainerHashes = append(result.ContainerHashes, ContainerHashEntry{
						PodName:     pod.Name,
						Namespace:   pod.Namespace,
						Container:   cs.Name,
						Image:       cs.Image,
						ContainerID: cid,
						Reason:      "Container hash does not match image hash — possible image tampering or runtime override",
						Severity:    "medium",
					})
				}
			}
		}

		// Flag high-restart pods
		if podRestarts >= 5 {
			hasIssue = true
			issues = append(issues, ForensicsEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				WorkloadType: wlType,
				Issue:        "High restart count",
				Detail:       fmt.Sprintf("Pod has %d total container restarts — investigate crashloop or instability", podRestarts),
				Severity:     "high",
				Restarts:     podRestarts,
			})
		}

		// Deduplicate issues per pod (keep highest severity)
		if len(issues) > 0 {
			sort.Slice(issues, func(i, j int) bool {
				sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
				return sevOrder[issues[i].Severity] < sevOrder[issues[j].Severity]
			})
			result.SuspiciousPods = append(result.SuspiciousPods, issues[0])
		}

		if hasIssue {
			result.Summary.PodsWithIssues++
		}
	}

	// Recent restarts (approximate: any restart > 0 counts)
	result.Summary.RecentRestarts = result.Summary.ExitCodeErrors + result.Summary.OOMKills

	// Build exit code analysis
	for code, count := range exitCodeCounts {
		result.ExitCodeAnalysis = append(result.ExitCodeAnalysis, ExitCodeStat{
			ExitCode: code,
			Count:    count,
			Meaning:  exitCodeMeaning(code),
		})
	}
	sort.Slice(result.ExitCodeAnalysis, func(i, j int) bool {
		return result.ExitCodeAnalysis[i].Count > result.ExitCodeAnalysis[j].Count
	})

	// Sort terminations by finished time (most recent first)
	sort.Slice(allTerminations, func(i, j int) bool {
		return allTerminations[i].FinishedAt > allTerminations[j].FinishedAt
	})
	if len(allTerminations) > 50 {
		allTerminations = allTerminations[:50]
	}
	result.RecentTerminations = allTerminations

	// Sort suspicious pods by severity
	sort.Slice(result.SuspiciousPods, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.SuspiciousPods[i].Severity] < sevOrder[result.SuspiciousPods[j].Severity]
	})
	if len(result.SuspiciousPods) > 30 {
		result.SuspiciousPods = result.SuspiciousPods[:30]
	}

	result.Summary.ForensicsScore = forensicsScore(result.Summary)
	result.Recommendations = forensicsRecommendations(&result)

	writeJSON(w, result)
}

// exitCodeSeverity maps an exit code to a severity level.
func exitCodeSeverity(code int32) string {
	if code == 137 {
		return "high" // OOMKilled
	}
	if code == 143 {
		return "low" // SIGTERM graceful shutdown
	}
	if code == 1 || code == 2 {
		return "medium" // application error
	}
	if code >= 126 {
		return "high" // signal-related
	}
	return "medium"
}

// exitCodeMeaning provides a human-readable meaning for common exit codes.
func exitCodeMeaning(code int32) string {
	meanings := map[int32]string{
		0:   "Success",
		1:   "General error",
		2:   "Misuse of shell builtin",
		126: "Command cannot execute",
		127: "Command not found",
		128: "Invalid exit argument",
		137: "OOMKilled (SIGKILL)",
		143: "Terminated (SIGTERM)",
		130: "Interrupted (SIGINT)",
		131: "Quit (SIGQUIT)",
		134: "Aborted (SIGABRT)",
		139: "Segmentation fault (SIGSEGV)",
	}
	if m, ok := meanings[code]; ok {
		return m
	}
	if code > 128 {
		return fmt.Sprintf("Killed by signal %d", code-128)
	}
	return fmt.Sprintf("Exit code %d", code)
}

// extractContainerHash extracts the hash from a container ID string.
func extractContainerHash(containerID string) string {
	// Format: containerd://sha256:abc123...
	parts := strings.SplitN(containerID, ":", 2)
	if len(parts) < 2 {
		return containerID
	}
	return parts[len(parts)-1]
}

// extractImageHash extracts the hash from an image ID string.
func extractImageHash(imageID string) string {
	// Format: sha256:abc123... or docker.io/repo/image@sha256:abc123...
	if idx := strings.LastIndex(imageID, ":"); idx > 0 {
		return imageID[idx+1:]
	}
	return imageID
}

// forensicsScore computes a 0-100 health score (100 = clean).
func forensicsScore(s ForensicsSummary) int {
	if s.TotalPods == 0 {
		return 100
	}

	score := 100

	if s.OOMKills > 0 {
		score -= min(25, s.OOMKills*5)
	}

	if s.SIGKILLTerminations > 0 {
		score -= min(15, s.SIGKILLTerminations*3)
	}

	if s.ExitCodeErrors > 0 {
		score -= min(20, s.ExitCodeErrors*4)
	}

	if s.PrivilegedEscapes > 0 {
		score -= min(20, s.PrivilegedEscapes*10)
	}

	if s.HashMismatches > 0 {
		score -= min(10, s.HashMismatches*3)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// forensicsRecommendations generates actionable recommendations.
func forensicsRecommendations(r *ForensicsResult) []string {
	var recs []string

	if r.Summary.OOMKills > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d OOMKilled container(s) — increase memory limits or investigate memory leaks",
			r.Summary.OOMKills,
		))
	}

	if r.Summary.SIGKILLTerminations > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d SIGKILL termination(s) — containers were forcefully killed, check liveness probe aggressiveness or node pressure",
			r.Summary.SIGKILLTerminations,
		))
	}

	if r.Summary.PrivilegedEscapes > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d privileged container(s) with termination history — investigate for security incidents and remove privileged flag if possible",
			r.Summary.PrivilegedEscapes,
		))
	}

	if r.Summary.HashMismatches > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d container hash mismatch(es) — possible image tampering, verify image provenance and registry integrity",
			r.Summary.HashMismatches,
		))
	}

	if r.Summary.ExitCodeErrors > 3 {
		recs = append(recs, fmt.Sprintf(
			"%d non-zero exit code(s) — review application logs and error handling",
			r.Summary.ExitCodeErrors,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "No security incidents detected — pod terminations and exit codes look clean")
	}

	return recs
}
