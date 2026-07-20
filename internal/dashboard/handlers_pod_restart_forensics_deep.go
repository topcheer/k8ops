package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodRestartForensicsDeepResult performs deep forensic analysis of pod restart patterns.
type PodRestartForensicsDeepResult struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	Summary         RestartForensicDeepSummary `json:"summary"`
	TopOffenders    []RestartForensicDeepEntry `json:"topOffenders"`
	ByPattern       []RestartPatternDeepEntry  `json:"byPattern"`
	Timeline        []RestartTimelineDeepEntry `json:"timeline"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Recommendations []string                   `json:"recommendations"`
}

type RestartForensicDeepSummary struct {
	TotalPods         int     `json:"totalPods"`
	PodsWithRestarts  int     `json:"podsWithRestarts"`
	TotalRestarts     int     `json:"totalRestarts"`
	AvgRestartsPerPod float64 `json:"avgRestartsPerPod"`
	CrashLoopPods     int     `json:"crashLoopPods"`
	OOMKilledPods     int     `json:"oomKilledPods"`
	HighRestartPods   int     `json:"highRestartPods"` // >10 restarts
}

type RestartForensicDeepEntry struct {
	PodName        string `json:"podName"`
	Namespace      string `json:"namespace"`
	ContainerName  string `json:"containerName"`
	RestartCount   int    `json:"restartCount"`
	LastState      string `json:"lastTerminationState"`
	LastReason     string `json:"lastTerminationReason"`
	ExitCode       int32  `json:"exitCode"`
	RiskLevel      string `json:"riskLevel"`
	RootCauseGuess string `json:"rootCauseGuess"`
}

type RestartPatternDeepEntry struct {
	Pattern  string `json:"pattern"`
	Count    int    `json:"podCount"`
	Severity string `json:"severity"`
}

type RestartTimelineDeepEntry struct {
	Namespace string `json:"namespace"`
	PodName   string `json:"podName"`
	Restarts  int    `json:"restarts"`
	Age       string `json:"podAge"`
}

// handlePodRestartForensicsDeep handles GET /api/operations/pod-restart-forensics-deep
func (s *Server) handlePodRestartForensicsDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PodRestartForensicsDeepResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	patternMap := make(map[string]int)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		podHasRestart := false
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount == 0 {
				continue
			}
			podHasRestart = true
			result.Summary.TotalRestarts += int(cs.RestartCount)

			entry := RestartForensicDeepEntry{
				PodName:       pod.Name,
				Namespace:     pod.Namespace,
				ContainerName: cs.Name,
				RestartCount:  int(cs.RestartCount),
			}

			// Analyze last termination
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				entry.LastState = "terminated"
				entry.LastReason = term.Reason
				entry.ExitCode = term.ExitCode

				// Root cause guess based on exit code and reason
				switch {
				case term.Reason == "OOMKilled":
					entry.RootCauseGuess = "Out of memory - increase memory limits or fix memory leak"
					result.Summary.OOMKilledPods++
					patternMap["oom-killed"]++
				case term.ExitCode == 1:
					entry.RootCauseGuess = "Application error - check application logs"
					patternMap["app-error"]++
				case term.ExitCode == 137:
					entry.RootCauseGuess = "SIGKILL - possible OOM or manual kill"
					result.Summary.OOMKilledPods++
					patternMap["sigkill"]++
				case term.ExitCode == 143:
					entry.RootCauseGuess = "SIGTERM - graceful shutdown timeout"
					patternMap["sigterm"]++
				case term.ExitCode == 2:
					entry.RootCauseGuess = "Misconfiguration - check command/args"
					patternMap["config-error"]++
				default:
					entry.RootCauseGuess = fmt.Sprintf("Exit code %d - check container logs", term.ExitCode)
					patternMap["other"]++
				}
			}

			// Risk level
			switch {
			case cs.RestartCount > 50:
				entry.RiskLevel = "critical"
				result.Summary.CrashLoopPods++
			case cs.RestartCount > 10:
				entry.RiskLevel = "high"
				result.Summary.HighRestartPods++
			case cs.RestartCount > 3:
				entry.RiskLevel = "medium"
			default:
				entry.RiskLevel = "low"
			}

			result.TopOffenders = append(result.TopOffenders, entry)
		}

		if podHasRestart {
			result.Summary.PodsWithRestarts++
		}
	}

	// Build pattern summary
	for pattern, count := range patternMap {
		sev := "medium"
		if pattern == "oom-killed" || pattern == "sigkill" {
			sev = "high"
		}
		result.ByPattern = append(result.ByPattern, RestartPatternDeepEntry{
			Pattern: pattern, Count: count, Severity: sev,
		})
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestartsPerPod = float64(result.Summary.TotalRestarts) / float64(result.Summary.TotalPods)
	}

	// Score: penalize for high restart counts
	result.HealthScore = 100
	if result.Summary.CrashLoopPods > 0 {
		result.HealthScore -= result.Summary.CrashLoopPods * 15
	}
	if result.Summary.HighRestartPods > 0 {
		result.HealthScore -= result.Summary.HighRestartPods * 5
	}
	if result.Summary.OOMKilledPods > 0 {
		result.HealthScore -= result.Summary.OOMKilledPods * 3
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Pod 重启取证: %d Pod, %d 有重启, 总计 %d 次, %d CrashLoop, %d OOM, %d 高重启",
			result.Summary.TotalPods, result.Summary.PodsWithRestarts,
			result.Summary.TotalRestarts, result.Summary.CrashLoopPods,
			result.Summary.OOMKilledPods, result.Summary.HighRestartPods),
	}
	if result.Summary.OOMKilledPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 OOM Killed 容器, 建议增加内存限制或排查内存泄漏", result.Summary.OOMKilledPods))
	}
	if result.Summary.CrashLoopPods > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 CrashLoop (>50次重启), 紧急排查", result.Summary.CrashLoopPods))
	}
	writeJSON(w, result)
}
