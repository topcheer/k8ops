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

// PodStartupResult is the full pod startup lifecycle analysis.
type PodStartupResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         PodStartupSummary     `json:"summary"`
	SlowPods        []SlowStartupEntry    `json:"slowPods"`
	StuckPods       []StuckStartupEntry   `json:"stuckPods"`
	Bottlenecks     []StartupBottleneck   `json:"bottlenecks"`
	ByWorkload      []StartupWorkloadStat `json:"byWorkload"`
	Recommendations []string              `json:"recommendations"`
}

// PodStartupSummary aggregates cluster-wide startup statistics.
type PodStartupSummary struct {
	TotalPods         int     `json:"totalPods"`
	RunningPods       int     `json:"runningPods"`
	PendingPods       int     `json:"pendingPods"`
	FailedPods        int     `json:"failedPods"`
	AvgStartupSeconds float64 `json:"avgStartupSeconds"`
	MaxStartupSeconds float64 `json:"maxStartupSeconds"`
	SlowStartupCount  int     `json:"slowStartupCount"` // > 120s total startup
	StuckCount        int     `json:"stuckCount"`       // pending > 5min
	HealthScore       int     `json:"healthScore"`
}

// SlowStartupEntry describes a pod that took an abnormally long time to start.
type SlowStartupEntry struct {
	PodName         string  `json:"podName"`
	Namespace       string  `json:"namespace"`
	WorkloadType    string  `json:"workloadType"`
	TotalStartupSec float64 `json:"totalStartupSeconds"`
	SchedulingSec   float64 `json:"schedulingSeconds"`
	InitSec         float64 `json:"initSeconds"`
	ContainerSec    float64 `json:"containerSeconds"`
	ReadinessSec    float64 `json:"readinessSeconds"`
	HasInit         bool    `json:"hasInitContainers"`
	RiskLevel       string  `json:"riskLevel"`
}

// StuckStartupEntry describes a pod currently stuck in the startup pipeline.
type StuckStartupEntry struct {
	PodName       string  `json:"podName"`
	Namespace     string  `json:"namespace"`
	Phase         string  `json:"phase"`
	WaitingReason string  `json:"waitingReason"`
	Message       string  `json:"message"`
	PendingMin    float64 `json:"pendingMinutes"`
	RiskLevel     string  `json:"riskLevel"`
}

// StartupBottleneck identifies a startup performance bottleneck category.
type StartupBottleneck struct {
	Category    string `json:"category"` // scheduling, image_pull, init_container, probe, volume, unknown
	Count       int    `json:"count"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// StartupWorkloadStat shows average startup time by workload controller type.
type StartupWorkloadStat struct {
	WorkloadType       string  `json:"workloadType"`
	Count              int     `json:"count"`
	AvgStartupSec      float64 `json:"avgStartupSeconds"`
	MaxStartupSec      float64 `json:"maxStartupSeconds"`
	WithInitContainers int     `json:"withInitContainers"`
}

// handlePodStartup analyzes the full pod startup lifecycle and identifies bottlenecks.
// GET /api/operations/pod-startup
func (s *Server) handlePodStartup(w http.ResponseWriter, r *http.Request) {
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
	result := PodStartupResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)

	var startupTimes []float64
	wlStats := map[string]*StartupWorkloadStat{}
	bnCount := map[string]int{}

	for _, pod := range pods.Items {
		phase := pod.Status.Phase

		switch phase {
		case corev1.PodRunning:
			result.Summary.RunningPods++
			startupSec := analyzeRunningPodStartup(&pod, wlStats, &result, bnCount)
			if startupSec > 0 {
				startupTimes = append(startupTimes, startupSec)
				if startupSec > result.Summary.MaxStartupSeconds {
					result.Summary.MaxStartupSeconds = startupSec
				}
				if startupSec > 120 {
					result.Summary.SlowStartupCount++
				}
			}

		case corev1.PodPending:
			result.Summary.PendingPods++
			analyzeStuckPod(&pod, now, &result, bnCount)

		case corev1.PodFailed:
			result.Summary.FailedPods++
		}
	}

	// Compute average startup time
	if len(startupTimes) > 0 {
		total := 0.0
		for _, t := range startupTimes {
			total += t
		}
		result.Summary.AvgStartupSeconds = total / float64(len(startupTimes))
	}

	// Build bottlenecks
	result.Bottlenecks = buildStartupBottlenecks(bnCount)

	// Build workload stats
	for _, stat := range wlStats {
		if stat.Count > 0 {
			stat.AvgStartupSec = stat.AvgStartupSec / float64(stat.Count)
		}
		result.ByWorkload = append(result.ByWorkload, *stat)
	}
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return result.ByWorkload[i].AvgStartupSec > result.ByWorkload[j].AvgStartupSec
	})

	// Sort slow pods
	sort.Slice(result.SlowPods, func(i, j int) bool {
		return result.SlowPods[i].TotalStartupSec > result.SlowPods[j].TotalStartupSec
	})
	if len(result.SlowPods) > 20 {
		result.SlowPods = result.SlowPods[:20]
	}

	// Sort stuck pods
	sort.Slice(result.StuckPods, func(i, j int) bool {
		return result.StuckPods[i].PendingMin > result.StuckPods[j].PendingMin
	})
	if len(result.StuckPods) > 20 {
		result.StuckPods = result.StuckPods[:20]
	}

	// Compute health score
	result.Summary.HealthScore = podStartupScore(result.Summary)

	// Recommendations
	result.Recommendations = podStartupRecommendations(&result)

	writeJSON(w, result)
}

// analyzeRunningPodStartup extracts startup timing phases for a running pod.
// Returns total startup seconds (0 if not measurable).
func analyzeRunningPodStartup(pod *corev1.Pod, wlStats map[string]*StartupWorkloadStat, result *PodStartupResult, bnCount map[string]int) float64 {
	created := pod.CreationTimestamp.Time
	startTime := pod.Status.StartTime
	if startTime == nil {
		return 0
	}
	startTimeT := startTime.Time

	// Scheduling delay
	schedulingSec := startTimeT.Sub(created).Seconds()
	if schedulingSec < 0 {
		schedulingSec = 0
	}

	// Init container time
	initSec := 0.0
	hasInit := len(pod.Status.InitContainerStatuses) > 0
	for _, ics := range pod.Status.InitContainerStatuses {
		if ics.State.Terminated != nil {
			d := ics.State.Terminated.FinishedAt.Time.Sub(ics.State.Terminated.StartedAt.Time)
			if d > 0 {
				initSec += d.Seconds()
			}
		}
	}

	// Container start delay (from scheduled to first container running)
	containerSec := 0.0
	var firstContainerStart *time.Time
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Running != nil {
			st := cs.State.Running.StartedAt.Time
			if firstContainerStart == nil || st.Before(*firstContainerStart) {
				firstContainerStart = &st
			}
		}
	}
	if firstContainerStart != nil {
		containerSec = firstContainerStart.Sub(startTimeT).Seconds() - initSec
		if containerSec < 0 {
			containerSec = 0
		}
	}

	// Readiness delay (from first container start to PodReady)
	readinessSec := 0.0
	var readyTime *time.Time
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			rt := cond.LastTransitionTime.Time
			readyTime = &rt
			break
		}
	}
	if readyTime != nil && firstContainerStart != nil {
		readinessSec = readyTime.Sub(*firstContainerStart).Seconds()
		if readinessSec < 0 {
			readinessSec = 0
		}
	}

	// Total startup
	totalSec := 0.0
	if readyTime != nil {
		totalSec = readyTime.Sub(created).Seconds()
		if totalSec < 0 {
			totalSec = 0
		}
	} else if firstContainerStart != nil {
		totalSec = firstContainerStart.Sub(created).Seconds()
		if totalSec < 0 {
			totalSec = 0
		}
	}

	// Categorize bottlenecks
	if schedulingSec > 30 {
		bnCount["scheduling"]++
	}
	if containerSec > 60 {
		bnCount["image_pull"]++
	}
	if initSec > 30 {
		bnCount["init_container"]++
	}
	if readinessSec > 60 {
		bnCount["probe"]++
	}

	// Workload stats
	wlType := inferWorkloadTypeFromPod(pod)
	stat, ok := wlStats[wlType]
	if !ok {
		stat = &StartupWorkloadStat{WorkloadType: wlType}
		wlStats[wlType] = stat
	}
	stat.Count++
	stat.AvgStartupSec += totalSec
	if totalSec > stat.MaxStartupSec {
		stat.MaxStartupSec = totalSec
	}
	if hasInit {
		stat.WithInitContainers++
	}

	// Flag slow pods
	if totalSec > 120 {
		risk := "medium"
		if totalSec > 300 {
			risk = "high"
		}
		result.SlowPods = append(result.SlowPods, SlowStartupEntry{
			PodName:         pod.Name,
			Namespace:       pod.Namespace,
			WorkloadType:    wlType,
			TotalStartupSec: roundTo2(totalSec),
			SchedulingSec:   roundTo2(schedulingSec),
			InitSec:         roundTo2(initSec),
			ContainerSec:    roundTo2(containerSec),
			ReadinessSec:    roundTo2(readinessSec),
			HasInit:         hasInit,
			RiskLevel:       risk,
		})
	}

	return totalSec
}

// analyzeStuckPod identifies pods stuck in Pending/ContainerCreating.
func analyzeStuckPod(pod *corev1.Pod, now time.Time, result *PodStartupResult, bnCount map[string]int) {
	pendingMin := now.Sub(pod.CreationTimestamp.Time).Minutes()
	if pendingMin < 0 {
		pendingMin = 0
	}

	waitingReason := ""
	message := ""
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			waitingReason = cs.State.Waiting.Reason
			message = cs.State.Waiting.Message
			break
		}
	}
	// Also check init container waiting state
	if waitingReason == "" {
		for _, ics := range pod.Status.InitContainerStatuses {
			if ics.State.Waiting != nil {
				waitingReason = ics.State.Waiting.Reason
				message = ics.State.Waiting.Message
				break
			}
		}
	}

	// Categorize bottleneck
	cat := categorizeWaitingReason(waitingReason)
	if pendingMin > 5 {
		bnCount[cat]++
	}

	// Only report pods stuck > 2 minutes
	if pendingMin < 2 {
		return
	}

	risk := "medium"
	if pendingMin > 10 {
		risk = "high"
	} else if pendingMin > 3 {
		risk = "medium"
	} else {
		risk = "low"
	}

	result.StuckPods = append(result.StuckPods, StuckStartupEntry{
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		Phase:         string(pod.Status.Phase),
		WaitingReason: waitingReason,
		Message:       truncateMsg(message, 200),
		PendingMin:    roundTo2(pendingMin),
		RiskLevel:     risk,
	})

	result.Summary.StuckCount++
}

// categorizeWaitingReason maps a container waiting reason to a bottleneck category.
func categorizeWaitingReason(reason string) string {
	switch {
	case strings.Contains(reason, "ImagePull") || strings.Contains(reason, "ErrImage") || strings.Contains(reason, "ImageApply"):
		return "image_pull"
	case strings.Contains(reason, "ContainerCreating") || strings.Contains(reason, "CreateContainer"):
		return "volume"
	case strings.HasPrefix(reason, "Init"):
		return "init_container"
	case reason == "" || reason == "PodInitializing":
		return "unknown"
	default:
		return "unknown"
	}
}

// buildStartupBottlenecks converts the bottleneck counts into structured entries.
func buildStartupBottlenecks(bnCount map[string]int) []StartupBottleneck {
	descriptions := map[string]string{
		"scheduling":     "Pods experienced significant scheduling delays (>30s)",
		"image_pull":     "Pods experienced slow image pulls or container creation (>60s)",
		"init_container": "Init containers took abnormally long to complete (>30s)",
		"probe":          "Readiness probes delayed pod readiness (>60s after container start)",
		"volume":         "Pods stuck on volume mount/attach during container creation",
		"unknown":        "Pods stuck for unclassified reasons",
	}
	severity := map[string]string{
		"scheduling":     "medium",
		"image_pull":     "high",
		"init_container": "medium",
		"probe":          "medium",
		"volume":         "high",
		"unknown":        "low",
	}

	var bottlenecks []StartupBottleneck
	for cat, count := range bnCount {
		if count == 0 {
			continue
		}
		sev := severity[cat]
		if count >= 5 {
			sev = "high"
		}
		bottlenecks = append(bottlenecks, StartupBottleneck{
			Category:    cat,
			Count:       count,
			Severity:    sev,
			Description: descriptions[cat],
		})
	}
	sort.Slice(bottlenecks, func(i, j int) bool {
		return bottlenecks[i].Count > bottlenecks[j].Count
	})
	return bottlenecks
}

// inferWorkloadTypeFromPod derives the controller type from pod owner references and labels.
func inferWorkloadTypeFromPod(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		switch ref.Kind {
		case "Deployment":
			return "Deployment"
		case "StatefulSet":
			return "StatefulSet"
		case "DaemonSet":
			return "DaemonSet"
		case "Job":
			return "Job"
		case "CronJob":
			return "CronJob"
		case "ReplicaSet":
			// Check if owned by Deployment
			for _, parent := range pod.OwnerReferences {
				if parent.Kind == "Deployment" {
					return "Deployment"
				}
			}
			return "ReplicaSet"
		}
	}
	// Check if it's a static pod (mirror pod)
	if strings.HasPrefix(pod.Name, "kube-") && strings.HasSuffix(pod.Name, "-master") {
		return "StaticPod"
	}
	if _, ok := pod.Annotations["kubernetes.io/config.hash"]; ok && pod.OwnerReferences == nil {
		return "StaticPod"
	}
	return "Pod"
}

// podStartupScore computes a 0-100 health score for pod startup performance.
func podStartupScore(s PodStartupSummary) int {
	score := 100

	// Penalize stuck pods heavily
	if s.StuckCount > 0 {
		score -= min(30, s.StuckCount*5)
	}

	// Penalize slow startups
	if s.SlowStartupCount > 0 {
		score -= min(25, s.SlowStartupCount*3)
	}

	// Penalize high average startup time
	if s.AvgStartupSeconds > 60 {
		score -= min(20, int(s.AvgStartupSeconds/10)-5)
	}

	// Penalize failed pods
	if s.FailedPods > 0 {
		score -= min(15, s.FailedPods*3)
	}

	// Penalize very high max startup
	if s.MaxStartupSeconds > 300 {
		score -= min(10, int(s.MaxStartupSeconds/60)-4)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// podStartupRecommendations generates actionable recommendations.
func podStartupRecommendations(r *PodStartupResult) []string {
	var recs []string

	for _, bn := range r.Bottlenecks {
		switch bn.Category {
		case "scheduling":
			recs = append(recs, "Scheduling delays detected — check node capacity, resource requests, and affinity/anti-affinity rules that may constrain scheduling")
		case "image_pull":
			recs = append(recs, "Slow image pulls detected — consider pre-pulling images, using a local registry mirror, or reducing image size")
		case "init_container":
			recs = append(recs, "Init containers are slow — review init container logic, consider lazy initialization, or merge init containers into the main container")
		case "probe":
			recs = append(recs, "Readiness probes delay startup — tune initialDelaySeconds, failureThreshold, and periodSeconds for faster readiness detection")
		case "volume":
			recs = append(recs, "Volume mount/attach delays — check CSI driver health, storage class provisioning speed, and consider volume caching")
		}
	}

	if r.Summary.StuckCount > 0 {
		recs = append(recs, fmt.Sprintf("%d pods are stuck in startup — investigate waiting reasons and container events", r.Summary.StuckCount))
	}

	if r.Summary.AvgStartupSeconds > 60 {
		recs = append(recs, fmt.Sprintf("Average startup time is %.0fs — target <30s for optimal user experience", r.Summary.AvgStartupSeconds))
	}

	if r.Summary.SlowStartupCount > 5 {
		recs = append(recs, "Multiple pods have slow startup — consider a startup analysis dashboard and alerting on startup time SLIs")
	}

	if len(recs) == 0 {
		recs = append(recs, "Pod startup performance is healthy — no significant bottlenecks detected")
	}

	return recs
}
