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

// StartupLatencyResult is the pod startup latency & readiness performance analysis.
type StartupLatencyResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         StartupLatencySummary `json:"summary"`
	ByNamespace     []StartupLatencyNS    `json:"byNamespace"`
	ByWorkload      []StartupLatencyWork  `json:"byWorkload"`
	SlowPods        []StartupLatencyEntry `json:"slowPods"`
	FastPods        []StartupLatencyEntry `json:"fastPods"`
	Issues          []StartupLatencyIssue `json:"issues"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// StartupLatencySummary aggregates startup latency statistics.
type StartupLatencySummary struct {
	TotalPods     int   `json:"totalPods"`
	AnalyzedPods  int   `json:"analyzedPods"`
	AvgStartupMs  int64 `json:"avgStartupMs"`
	P50StartupMs  int64 `json:"p50StartupMs"`
	P90StartupMs  int64 `json:"p90StartupMs"`
	P99StartupMs  int64 `json:"p99StartupMs"`
	SlowPods      int   `json:"slowPods"`
	FastPods      int   `json:"fastPods"`
	NoReadiness   int   `json:"noReadinessProbe"`
	NoLiveness    int   `json:"noLivenessProbe"`
	HasInitCtn    int   `json:"hasInitContainers"`
	CrashLoopBack int   `json:"crashLoopBackOff"`
}

// StartupLatencyNS per-namespace startup stats.
type StartupLatencyNS struct {
	Namespace    string `json:"namespace"`
	PodCount     int    `json:"podCount"`
	AvgStartupMs int64  `json:"avgStartupMs"`
	SlowPods     int    `json:"slowPods"`
}

// StartupLatencyWork per-workload-type startup stats.
type StartupLatencyWork struct {
	WorkloadType string `json:"workloadType"`
	PodCount     int    `json:"podCount"`
	AvgStartupMs int64  `json:"avgStartupMs"`
	SlowCount    int    `json:"slowCount"`
}

// StartupLatencyEntry describes one pod's startup performance.
type StartupLatencyEntry struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	WorkloadType string    `json:"workloadType"`
	StartupMs    int64     `json:"startupMs"`
	CreatedAt    time.Time `json:"createdAt"`
	ReadyAt      time.Time `json:"readyAt"`
	HasInitCtn   bool      `json:"hasInitContainers"`
	HasReadiness bool      `json:"hasReadinessProbe"`
	HasLiveness  bool      `json:"hasLivenessProbe"`
	RestartCount int       `json:"restartCount"`
	Phase        string    `json:"phase"`
	RiskLevel    string    `json:"riskLevel"`
}

// StartupLatencyIssue is a detected startup performance problem.
type StartupLatencyIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleStartupLatency audits pod startup latency and readiness probe performance.
// GET /api/deployment/startup-latency
func (s *Server) handleStartupLatency(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	result := &StartupLatencyResult{
		ScannedAt: time.Now(),
	}

	// Thresholds
	const slowThresholdMs int64 = 60_000 // 60 seconds = slow
	const verySlowMs int64 = 120_000     // 120 seconds = critical

	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var allEntries []StartupLatencyEntry
	var issues []StartupLatencyIssue
	var recommendations []string

	noReadiness := 0
	noLiveness := 0
	hasInitCtn := 0
	crashLoopBack := 0
	slowCount := 0
	fastCount := 0

	nsStats := make(map[string][]int64) // namespace -> []startupMs
	nsSlowPods := make(map[string]int)
	workStats := make(map[string][]int64) // workloadType -> []startupMs
	workSlow := make(map[string]int)

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}

		pods, err := rc.clientset.CoreV1().Pods(ns.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, pod := range pods.Items {
			// Skip pods that haven't been created or don't have ready condition
			if pod.Status.StartTime == nil {
				continue
			}

			// Find the Ready condition
			var readyTime *time.Time
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					t := cond.LastTransitionTime.Time
					readyTime = &t
					break
				}
			}

			// Skip pods that aren't ready yet
			if readyTime == nil {
				continue
			}

			startupMs := readyTime.Sub(pod.Status.StartTime.Time).Milliseconds()
			if startupMs < 0 {
				continue
			}

			// Determine workload type from owner references
			workloadType := "Pod"
			for _, ref := range pod.OwnerReferences {
				if ref.Kind != "" {
					workloadType = ref.Kind
					break
				}
			}

			// Check probes on primary container
			hasReadiness := false
			hasLiveness := false
			hasInit := len(pod.Spec.InitContainers) > 0
			for _, c := range pod.Spec.Containers {
				if c.ReadinessProbe != nil {
					hasReadiness = true
				}
				if c.LivenessProbe != nil {
					hasLiveness = true
				}
			}

			if !hasReadiness {
				noReadiness++
				issues = append(issues, StartupLatencyIssue{
					Severity: "warning",
					Type:     "no-readiness-probe",
					Resource: fmt.Sprintf("%s/%s", ns.Name, pod.Name),
					Message:  "Pod has no readiness probe — kubelet cannot detect when the pod is ready to serve traffic",
				})
			}
			if !hasLiveness {
				noLiveness++
				issues = append(issues, StartupLatencyIssue{
					Severity: "info",
					Type:     "no-liveness-probe",
					Resource: fmt.Sprintf("%s/%s", ns.Name, pod.Name),
					Message:  "Pod has no liveness probe — kubelet cannot detect and restart hung containers",
				})
			}
			if hasInit {
				hasInitCtn++
			}

			// Check for CrashLoopBackOff
			isCrashLoop := false
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
					isCrashLoop = true
					crashLoopBack++
					issues = append(issues, StartupLatencyIssue{
						Severity: "critical",
						Type:     "crash-loop-back-off",
						Resource: fmt.Sprintf("%s/%s/%s", ns.Name, pod.Name, cs.Name),
						Message:  fmt.Sprintf("Container %s is in CrashLoopBackOff — check container logs", cs.Name),
					})
					break
				}
			}

			// Determine risk level
			riskLevel := "healthy"
			if startupMs >= verySlowMs {
				riskLevel = "critical"
			} else if startupMs >= slowThresholdMs {
				riskLevel = "warning"
			}
			if isCrashLoop {
				riskLevel = "critical"
			}

			entry := StartupLatencyEntry{
				Name:         pod.Name,
				Namespace:    ns.Name,
				WorkloadType: workloadType,
				StartupMs:    startupMs,
				CreatedAt:    pod.Status.StartTime.Time,
				ReadyAt:      *readyTime,
				HasInitCtn:   hasInit,
				HasReadiness: hasReadiness,
				HasLiveness:  hasLiveness,
				RestartCount: getTotalRestarts(&pod),
				Phase:        string(pod.Status.Phase),
				RiskLevel:    riskLevel,
			}

			allEntries = append(allEntries, entry)
			nsStats[ns.Name] = append(nsStats[ns.Name], startupMs)
			workStats[workloadType] = append(workStats[workloadType], startupMs)

			if startupMs >= slowThresholdMs {
				slowCount++
				nsSlowPods[ns.Name]++
				workSlow[workloadType]++

				if startupMs >= verySlowMs {
					issues = append(issues, StartupLatencyIssue{
						Severity: "critical",
						Type:     "very-slow-startup",
						Resource: fmt.Sprintf("%s/%s", ns.Name, pod.Name),
						Message:  fmt.Sprintf("Pod took %dms (%.1fs) to become ready — exceeds 120s threshold", startupMs, float64(startupMs)/1000),
					})
				} else {
					issues = append(issues, StartupLatencyIssue{
						Severity: "warning",
						Type:     "slow-startup",
						Resource: fmt.Sprintf("%s/%s", ns.Name, pod.Name),
						Message:  fmt.Sprintf("Pod took %dms (%.1fs) to become ready — exceeds 60s threshold", startupMs, float64(startupMs)/1000),
					})
				}
			} else if startupMs <= 5_000 {
				fastCount++
			}
		}
	}

	// Compute percentiles
	var startupTimes []int64
	for _, e := range allEntries {
		startupTimes = append(startupTimes, e.StartupMs)
	}
	sort.Slice(startupTimes, func(i, j int) bool { return startupTimes[i] < startupTimes[j] })

	avgMs, p50, p90, p99 := computeStartupPercentiles(startupTimes)

	// Build namespace stats
	for ns, times := range nsStats {
		var sum int64
		for _, t := range times {
			sum += t
		}
		result.ByNamespace = append(result.ByNamespace, StartupLatencyNS{
			Namespace:    ns,
			PodCount:     len(times),
			AvgStartupMs: sum / int64(len(times)),
			SlowPods:     nsSlowPods[ns],
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].AvgStartupMs > result.ByNamespace[j].AvgStartupMs
	})

	// Build workload stats
	for wt, times := range workStats {
		var sum int64
		for _, t := range times {
			sum += t
		}
		result.ByWorkload = append(result.ByWorkload, StartupLatencyWork{
			WorkloadType: wt,
			PodCount:     len(times),
			AvgStartupMs: sum / int64(len(times)),
			SlowCount:    workSlow[wt],
		})
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].AvgStartupMs > result.ByWorkload[j].AvgStartupMs
	})

	// Sort entries and pick top slow/fast
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].StartupMs > allEntries[j].StartupMs
	})

	if len(allEntries) > 20 {
		result.SlowPods = allEntries[:20]
	} else {
		result.SlowPods = allEntries
	}

	// Fast pods (bottom of sorted list, reversed)
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].StartupMs < allEntries[j].StartupMs
	})
	if len(allEntries) > 10 {
		result.FastPods = allEntries[:10]
	} else {
		result.FastPods = allEntries
	}

	// Recommendations
	if slowCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d pod(s) took over 60s to start — review init containers, resource limits, and probe configurations", slowCount))
	}
	if noReadiness > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d pod(s) have no readiness probe — add readiness probes to enable proper traffic routing", noReadiness))
	}
	if noLiveness > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d pod(s) have no liveness probe — add liveness probes for automatic recovery from hung states", noLiveness))
	}
	if crashLoopBack > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d pod(s) in CrashLoopBackOff — check container logs and fix application startup errors", crashLoopBack))
	}
	if hasInitCtn > 0 && avgMs > 30_000 {
		recommendations = append(recommendations, fmt.Sprintf("%d pod(s) have init containers and average startup is %.1fs — review if init containers can be optimized or parallelized", hasInitCtn, float64(avgMs)/1000))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Pod startup performance is healthy — all pods start within acceptable thresholds")
	}

	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = StartupLatencySummary{
		TotalPods:     len(allEntries),
		AnalyzedPods:  len(allEntries),
		AvgStartupMs:  avgMs,
		P50StartupMs:  p50,
		P90StartupMs:  p90,
		P99StartupMs:  p99,
		SlowPods:      slowCount,
		FastPods:      fastCount,
		NoReadiness:   noReadiness,
		NoLiveness:    noLiveness,
		HasInitCtn:    hasInitCtn,
		CrashLoopBack: crashLoopBack,
	}
	result.HealthScore = computeStartupHealthScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// computeStartupPercentiles returns avg, p50, p90, p99 from a sorted int64 slice.
func computeStartupPercentiles(sorted []int64) (avg, p50, p90, p99 int64) {
	if len(sorted) == 0 {
		return 0, 0, 0, 0
	}
	var sum int64
	for _, v := range sorted {
		sum += v
	}
	avg = sum / int64(len(sorted))
	// Convert to float64 for existing percentile function
	floatSorted := make([]float64, len(sorted))
	for i, v := range sorted {
		floatSorted[i] = float64(v)
	}
	p50 = int64(percentile(floatSorted, 50))
	p90 = int64(percentile(floatSorted, 90))
	p99 = int64(percentile(floatSorted, 99))
	return
}

// startupPercentile returns the p-th percentile from a sorted int64 slice.
func startupPercentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// computeStartupHealthScore computes a 0-100 health score.
func computeStartupHealthScore(summary StartupLatencySummary, issueCount int) int {
	if summary.TotalPods == 0 {
		return 100
	}
	score := 100
	score -= summary.CrashLoopBack * 15
	score -= summary.SlowPods * 5
	score -= summary.NoReadiness * 3
	score -= summary.NoLiveness * 1
	// Penalize high p99
	if summary.P99StartupMs > 120_000 {
		score -= 10
	} else if summary.P99StartupMs > 60_000 {
		score -= 5
	}
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// startupWorkloadType extracts workload type from owner reference kind.
func startupWorkloadType(pod corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind != "" {
			return ref.Kind
		}
	}
	return "Pod"
}

// formatStartupMs converts milliseconds to human-readable string.
func formatStartupMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// suppress unused warning
var _ = strings.TrimSpace
