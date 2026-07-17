package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestartAnalyzerResult performs deep analysis of pod restart patterns
// to identify root causes: OOM kills, liveness probe failures, crash loops,
// and deployment-triggered restarts. Helps distinguish systematic issues
// from one-off incidents.
type RestartAnalyzerResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         RestartAnalysisSummary  `json:"summary"`
	Patterns        []RestartAnalyzePattern `json:"patterns"`
	ByNamespace     []RestartNSAnalysis     `json:"byNamespace"`
	RootCauses      []RootCauseEntry        `json:"rootCauses"`
	StabilityScore  int                     `json:"stabilityScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type RestartAnalysisSummary struct {
	TotalPods      int     `json:"totalPods"`
	RestartingPods int     `json:"restartingPods"`
	TotalRestarts  int     `json:"totalRestarts"`
	MaxRestarts    int     `json:"maxRestarts"`
	AvgRestarts    float64 `json:"avgRestarts"`
	OOMKills       int     `json:"oomKills"`
	ProbeFailures  int     `json:"probeFailures"`
	CrashLoops     int     `json:"crashLoops"`
	NormalRestarts int     `json:"normalRestarts"`
}

type RestartAnalyzePattern struct {
	Pod        string `json:"pod"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	Restarts   int    `json:"restarts"`
	Pattern    string `json:"pattern"` // crash-loop, oom-kill, probe-fail, periodic, deployment
	Severity   string `json:"severity"`
	LastState  string `json:"lastState"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
}

type RestartNSAnalysis struct {
	Namespace   string  `json:"namespace"`
	Pods        int     `json:"pods"`
	Restarts    int     `json:"restarts"`
	AvgRestarts float64 `json:"avgRestarts"`
	WorstPod    string  `json:"worstPod"`
}

type RootCauseEntry struct {
	Category    string `json:"category"`
	Count       int    `json:"count"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
}

// handleRestartAnalyzer handles GET /api/operations/restart-analyzer
func (s *Server) handleRestartAnalyzer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RestartAnalyzerResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*RestartNSAnalysis)
	var patterns []RestartAnalyzePattern
	oomKills := 0
	probeFails := 0
	crashLoops := 0
	normalRestarts := 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++

		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &RestartNSAnalysis{Namespace: pod.Namespace}
		}
		ns := nsMap[pod.Namespace]
		ns.Pods++

		for _, cs := range pod.Status.ContainerStatuses {
			restarts := int(cs.RestartCount)
			result.Summary.TotalRestarts += restarts
			ns.Restarts += restarts

			if restarts == 0 {
				continue
			}

			if restarts > result.Summary.MaxRestarts {
				result.Summary.MaxRestarts = restarts
			}

			result.Summary.RestartingPods++

			// Analyze pattern from last termination state
			pattern := RestartAnalyzePattern{
				Pod: pod.Name, Namespace: pod.Namespace,
				Container: cs.Name, Restarts: restarts,
			}

			// Determine root cause from last terminated state
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				pattern.LastState = term.Reason

				switch term.Reason {
				case "OOMKilled":
					pattern.Pattern = "oom-kill"
					pattern.Severity = "critical"
					pattern.Detail = fmt.Sprintf("容器因内存不足被杀死 (exit=%d)", term.ExitCode)
					pattern.Suggestion = "增加 memory limit 或排查内存泄漏"
					oomKills++
				case "Completed":
					pattern.Pattern = "normal-exit"
					pattern.Severity = "low"
					pattern.Detail = "容器正常退出后重启"
					pattern.Suggestion = "检查是否为 Job 或 init container"
					normalRestarts++
				default:
					if term.ExitCode != 0 {
						pattern.Pattern = "crash"
						pattern.Severity = "high"
						pattern.Detail = fmt.Sprintf("异常退出 (exit=%d, reason=%s)", term.ExitCode, term.Reason)
						pattern.Suggestion = "查看容器日志定位崩溃原因"
						crashLoops++
					} else {
						pattern.Pattern = "unknown"
						pattern.Severity = "low"
						normalRestarts++
					}
				}
			} else if cs.State.Waiting != nil {
				pattern.LastState = cs.State.Waiting.Reason
				if cs.State.Waiting.Reason == "CrashLoopBackOff" {
					pattern.Pattern = "crash-loop"
					pattern.Severity = "critical"
					pattern.Detail = "容器反复崩溃，指数退避等待中"
					pattern.Suggestion = "检查应用日志和启动依赖"
					crashLoops++
				}
			} else {
				// Probe failure detection heuristic
				if restarts > 2 && cs.Ready {
					pattern.Pattern = "periodic"
					pattern.Severity = "medium"
					pattern.Detail = "周期性重启，可能是探针失败或资源压力"
					pattern.Suggestion = "检查 livenessProbe 配置和资源限制"
					probeFails++
				} else {
					pattern.Pattern = "infrequent"
					pattern.Severity = "low"
					pattern.Detail = fmt.Sprintf("重启 %d 次，频率较低", restarts)
					pattern.Suggestion = "监控趋势"
					normalRestarts++
				}
			}

			if ns.WorstPod == "" || restarts > 0 {
				ns.WorstPod = fmt.Sprintf("%s/%s (%d)", pod.Name, cs.Name, restarts)
			}

			patterns = append(patterns, pattern)
		}
	}

	// Summary
	result.Summary.OOMKills = oomKills
	result.Summary.ProbeFailures = probeFails
	result.Summary.CrashLoops = crashLoops
	result.Summary.NormalRestarts = normalRestarts
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestarts = float64(result.Summary.TotalRestarts) / float64(result.Summary.TotalPods)
	}

	// Root causes
	if oomKills > 0 {
		result.RootCauses = append(result.RootCauses, RootCauseEntry{
			Category: "OOM", Count: oomKills, Severity: "critical",
			Description: "内存不足导致容器被杀死",
			Fix:         "增加 memory limit 或排查内存泄漏",
		})
	}
	if crashLoops > 0 {
		result.RootCauses = append(result.RootCauses, RootCauseEntry{
			Category: "CrashLoop", Count: crashLoops, Severity: "critical",
			Description: "容器反复崩溃进入 CrashLoopBackOff",
			Fix:         "检查应用启动日志和依赖服务",
		})
	}
	if probeFails > 0 {
		result.RootCauses = append(result.RootCauses, RootCauseEntry{
			Category: "ProbeFailure", Count: probeFails, Severity: "medium",
			Description: "探针配置不当导致周期性重启",
			Fix:         "调整 livenessProbe 超时和阈值",
		})
	}

	// NS breakdown
	for _, ns := range nsMap {
		if ns.Pods > 0 {
			ns.AvgRestarts = float64(ns.Restarts) / float64(ns.Pods)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Restarts > result.ByNamespace[j].Restarts
	})

	// Sort patterns by restart count
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Restarts > patterns[j].Restarts
	})
	result.Patterns = patterns

	// Score
	result.StabilityScore = 100
	if result.Summary.TotalPods > 0 {
		result.StabilityScore -= result.Summary.OOMKills * 10
		result.StabilityScore -= result.Summary.CrashLoops * 8
		result.StabilityScore -= result.Summary.ProbeFailures * 3
		result.StabilityScore -= int(result.Summary.AvgRestarts * 5)
	}
	if result.StabilityScore < 0 {
		result.StabilityScore = 0
	}

	switch {
	case result.StabilityScore >= 80:
		result.Grade = "A"
	case result.StabilityScore >= 60:
		result.Grade = "B"
	case result.StabilityScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildRestartAnalysisRecs(&result)
	writeJSON(w, result)
}

func buildRestartAnalysisRecs(r *RestartAnalyzerResult) []string {
	recs := []string{
		fmt.Sprintf("集群稳定性: %d/100 (%s)，平均重启 %.1f 次/Pod", r.StabilityScore, r.Grade, r.Summary.AvgRestarts),
	}
	if r.Summary.OOMKills > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个 OOM Kill，建议增加内存限制或排查泄漏", r.Summary.OOMKills))
	}
	if r.Summary.CrashLoops > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个 CrashLoopBackOff，需立即排查启动失败原因", r.Summary.CrashLoops))
	}
	if r.Summary.ProbeFailures > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个疑似探针失败导致的周期性重启", r.Summary.ProbeFailures))
	}
	if len(recs) == 1 && r.Summary.RestartingPods == 0 {
		recs = append(recs, "无重启问题，集群稳定性良好")
	}
	return recs
}

var _ corev1.ContainerStatus
