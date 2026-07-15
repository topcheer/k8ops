package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminationResult is the pod termination message & exit code pattern audit.
type TerminationResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         TerminationSummary   `json:"summary"`
	ExitCodes       []ExitCodeStat       `json:"exitCodes"`
	SignaledPods    []TerminatedPod      `json:"signaledPods"`
	OOMKilledPods   []TerminatedPod      `json:"oomKilledPods"`
	Patterns        []TerminationPattern `json:"patterns"`
	Risks           []TerminationRisk    `json:"risks"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// TerminationSummary aggregates termination metrics.
type TerminationSummary struct {
	TotalPods        int `json:"totalPods"`
	TerminatedPods   int `json:"terminatedPods"` // pods with terminated containers
	OOMKilledCount   int `json:"oomKilledCount"`
	SignalKilled     int `json:"signalKilled"` // SIGKILL/SIGTERM
	ExitCode0        int `json:"exitCode0"`    // clean exits
	NonZeroExitCount int `json:"nonZeroExitCount"`
	WithTermMsg      int `json:"withTermMsg"`      // has termination message
	NoTermMsg        int `json:"noTermMsg"`        // no termination message
	HighRestartCount int `json:"highRestartCount"` // pods with >5 restarts
}

// ExitCodeStat is defined in handlers_forensics.go and reused here.

// TerminatedPod describes a pod with terminated containers.
type TerminatedPod struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	ExitCode  int32  `json:"exitCode"`
	Reason    string `json:"reason"`
	Signal    int    `json:"signal,omitempty"`
	OOMKilled bool   `json:"oomKilled"`
	Restarts  int    `json:"restarts"`
	Message   string `json:"message,omitempty"`
}

// TerminationPattern describes a recurring termination pattern.
type TerminationPattern struct {
	Pattern  string `json:"pattern"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
}

// TerminationRisk describes a termination-related risk.
type TerminationRisk struct {
	Pod      string `json:"pod,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleTerminationAudit audits pod termination messages & exit code patterns.
// GET /api/deployment/termination-audit
func (s *Server) handleTerminationAudit(w http.ResponseWriter, r *http.Request) {
	result := TerminationResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	exitCodeMap := map[int32]*ExitCodeStat{}
	patternMap := map[string]*TerminationPattern{}

	for i := range pods.Items {
		pod := &pods.Items[i]
		result.Summary.TotalPods++

		for _, cs := range pod.Status.ContainerStatuses {
			// Count restarts
			if cs.RestartCount > 5 {
				result.Summary.HighRestartCount++
			}

			// Check terminated state
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				result.Summary.TerminatedPods++

				entry := TerminatedPod{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Container: cs.Name,
					ExitCode:  int32(term.ExitCode),
					Reason:    term.Reason,
					Restarts:  int(cs.RestartCount),
					Message:   term.Message,
				}

				// OOMKilled detection
				if term.Reason == "OOMKilled" {
					entry.OOMKilled = true
					result.Summary.OOMKilledCount++
					result.OOMKilledPods = append(result.OOMKilledPods, entry)
					p := "OOMKilled"
					if patternMap[p] == nil {
						patternMap[p] = &TerminationPattern{Pattern: p, Severity: "critical"}
					}
					patternMap[p].Count++
					result.Risks = append(result.Risks, TerminationRisk{
						Pod: pod.Name,
						Issue: fmt.Sprintf("Container %s in %s/%s was OOMKilled — increase memory limit or check for memory leak",
							cs.Name, pod.Namespace, pod.Name),
						Severity: "critical",
					})
				} else if term.Signal > 0 {
					entry.Signal = int(term.Signal)
					result.Summary.SignalKilled++
					result.SignaledPods = append(result.SignaledPods, entry)
					sigStr := fmt.Sprintf("Signal %d", term.Signal)
					if patternMap[sigStr] == nil {
						patternMap[sigStr] = &TerminationPattern{Pattern: sigStr, Severity: "warning"}
					}
					patternMap[sigStr].Count++
				} else {
					// Normal exit
					if int(term.ExitCode) == 0 {
						result.Summary.ExitCode0++
					} else {
						result.Summary.NonZeroExitCount++
						result.SignaledPods = append(result.SignaledPods, entry)
					}
				}

				// Exit code stats
				if exitCodeMap[int32(term.ExitCode)] == nil {
					exitCodeMap[int32(term.ExitCode)] = &ExitCodeStat{
						ExitCode: int32(term.ExitCode),
						Meaning:  term.Reason,
					}
				}
				exitCodeMap[int32(term.ExitCode)].Count++

				// Termination message
				if term.Message != "" {
					result.Summary.WithTermMsg++
				} else {
					result.Summary.NoTermMsg++
					// Only flag if it's a non-clean exit
					if int(term.ExitCode) != 0 {
						result.Risks = append(result.Risks, TerminationRisk{
							Pod: pod.Name,
							Issue: fmt.Sprintf("Container %s in %s/%s exited with code %d but has no termination message",
								cs.Name, pod.Namespace, pod.Name, term.ExitCode),
							Severity: "low",
						})
					}
				}
			}
		}
	}

	// Build exit code stats
	for _, stat := range exitCodeMap {
		result.ExitCodes = append(result.ExitCodes, *stat)
	}
	sort.Slice(result.ExitCodes, func(i, j int) bool {
		return result.ExitCodes[i].Count > result.ExitCodes[j].Count
	})

	// Build patterns
	for _, p := range patternMap {
		result.Patterns = append(result.Patterns, *p)
	}
	sort.Slice(result.Patterns, func(i, j int) bool {
		return result.Patterns[i].Count > result.Patterns[j].Count
	})

	// Health score
	score := 100
	if result.Summary.OOMKilledCount > 0 {
		score -= min(30, result.Summary.OOMKilledCount*10)
	}
	if result.Summary.NonZeroExitCount > 0 {
		score -= min(20, result.Summary.NonZeroExitCount*5)
	}
	if result.Summary.HighRestartCount > 0 {
		score -= min(15, result.Summary.HighRestartCount*3)
	}
	if result.Summary.NoTermMsg > 0 {
		score -= min(10, result.Summary.NoTermMsg)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.OOMKilledCount > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d OOMKilled container(s) — increase memory limits or investigate memory leaks", result.Summary.OOMKilledCount))
	}
	if result.Summary.HighRestartCount > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) with high restart count (>5) — check for crash loops or resource pressure", result.Summary.HighRestartCount))
	}
	if result.Summary.NoTermMsg > 0 && result.Summary.NonZeroExitCount > 0 {
		result.Recommendations = append(result.Recommendations,
			"Enable termination messages for non-zero exit codes — set terminationMessagePolicy and terminationMessagePath")
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"No problematic terminations detected — all pods are running cleanly")
	}

	writeJSON(w, result)
}
