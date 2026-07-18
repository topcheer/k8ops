package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodRestartForensicsResult performs forensic analysis on pod restarts,
// identifying root causes from termination reasons, exit codes, and patterns.
type PodRestartForensicsResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         RestartForensicsSummary `json:"summary"`
	ByWorkload      []RestartForensicEntry  `json:"byWorkload"`
	TopSuspects     []RestartForensicEntry  `json:"topSuspects"`
	ForensicsScore  int                     `json:"forensicsScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type RestartForensicsSummary struct {
	TotalPods     int            `json:"totalPods"`
	RestartedPods int            `json:"restartedPods"`
	TotalRestarts int            `json:"totalRestarts"`
	OOMKills      int            `json:"oomKills"`
	ExitCode1     int            `json:"exitCode1"`
	SignalKills   int            `json:"signalKills"`
	UnknownExits  int            `json:"unknownExits"`
	RootCauses    map[string]int `json:"rootCauses"`
}

type RestartForensicEntry struct {
	Workload       string     `json:"workload"`
	Namespace      string     `json:"namespace"`
	PodName        string     `json:"podName"`
	RestartCount   int        `json:"restartCount"`
	LastExitCode   int        `json:"lastExitCode"`
	LastExitReason string     `json:"lastExitReason"`
	RootCause      string     `json:"rootCause"`
	Pattern        string     `json:"pattern"`
	FirstRestart   *time.Time `json:"firstRestart"`
	Severity       string     `json:"severity"`
}

// handlePodRestartForensics handles GET /api/operations/pod-restart-forensics
func (s *Server) handlePodRestartForensics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PodRestartForensicsResult{
		ScannedAt: time.Now(),
		Summary:   RestartForensicsSummary{RootCauses: make(map[string]int)},
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}

		totalRestarts := 0
		var entries []RestartForensicEntry

		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
			if cs.RestartCount == 0 {
				continue
			}

			entry := RestartForensicEntry{
				Workload: wlName, Namespace: pod.Namespace, PodName: pod.Name,
				RestartCount: int(cs.RestartCount),
			}

			// Analyze termination state
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				entry.LastExitCode = int(term.ExitCode)
				entry.LastExitReason = term.Reason
				entry.RootCause = classifyExit(term.Reason, term.ExitCode)
				entry.FirstRestart = &term.FinishedAt.Time

				switch entry.RootCause {
				case "oom":
					result.Summary.OOMKills++
				case "app-error":
					result.Summary.ExitCode1++
				case "signal":
					result.Summary.SignalKills++
				default:
					result.Summary.UnknownExits++
				}
				result.Summary.RootCauses[entry.RootCause]++
			} else {
				entry.RootCause = "unknown"
				result.Summary.UnknownExits++
				result.Summary.RootCauses["unknown"]++
			}

			// Pattern analysis
			switch {
			case cs.RestartCount > 10:
				entry.Pattern = "crashloop"
				entry.Severity = "critical"
			case cs.RestartCount > 3:
				entry.Pattern = "frequent-restart"
				entry.Severity = "high"
			default:
				entry.Pattern = "occasional"
				entry.Severity = "medium"
			}

			entries = append(entries, entry)
		}

		if totalRestarts > 0 {
			result.Summary.RestartedPods++
			result.Summary.TotalRestarts += totalRestarts
			result.ByWorkload = append(result.ByWorkload, entries...)
		}
	}

	// Sort by restart count descending
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].RestartCount > result.ByWorkload[j].RestartCount
	})

	// Top suspects (top 10)
	limit := 10
	if len(result.ByWorkload) < limit {
		limit = len(result.ByWorkload)
	}
	result.TopSuspects = result.ByWorkload[:limit]

	// Score: fewer restarts = higher score
	if result.Summary.TotalPods > 0 {
		restartRatio := float64(result.Summary.RestartedPods) / float64(result.Summary.TotalPods)
		result.ForensicsScore = int((1 - restartRatio) * 100)
		if result.ForensicsScore < 0 {
			result.ForensicsScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.ForensicsScore)

	result.Recommendations = buildRestartForensicsRecs(&result)
	writeJSON(w, result)
}

func classifyExit(reason string, exitCode int32) string {
	if reason == "OOMKilled" {
		return "oom"
	}
	if reason == "Error" || exitCode == 1 {
		return "app-error"
	}
	if exitCode == 137 || exitCode == 143 {
		return "signal"
	}
	if exitCode == 0 {
		return "completed"
	}
	return "unknown"
}

func buildRestartForensicsRecs(r *PodRestartForensicsResult) []string {
	recs := []string{
		fmt.Sprintf("Pod 重启取证: %d/%d Pod 有重启, 总计 %d 次", r.Summary.RestartedPods, r.Summary.TotalPods, r.Summary.TotalRestarts),
	}
	if r.Summary.OOMKills > 0 {
		recs = append(recs, fmt.Sprintf("OOM 终止: %d 次 - 建议增加 memory limits", r.Summary.OOMKills))
	}
	if r.Summary.ExitCode1 > 0 {
		recs = append(recs, fmt.Sprintf("应用错误 (exit=1): %d 次 - 检查应用日志", r.Summary.ExitCode1))
	}
	if len(r.TopSuspects) > 0 {
		top := r.TopSuspects[0]
		recs = append(recs, fmt.Sprintf("最大嫌疑: %s/%s (%d 次重启, 原因: %s)", top.Namespace, top.Workload, top.RestartCount, top.RootCause))
	}
	if r.ForensicsScore < 80 {
		recs = append(recs, "建议: 设置 resource limits, 添加 liveness probe, 检查日志中的 panic/error")
	}
	return recs
}
