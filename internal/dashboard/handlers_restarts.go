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

// RestartPattern classifies the restart behavior of a pod.
type RestartPattern string

const (
	RestartPatternNone       RestartPattern = "none"        // no restarts
	RestartPatternCrashLoop  RestartPattern = "crash-loop"  // many restarts in short time
	RestartPatternOccasional RestartPattern = "occasional"  // few restarts over long time
	RestartPatternPostDeploy RestartPattern = "post-deploy" // restarts right after creation
)

// ContainerRestartInfo holds restart diagnosis for a single container.
type ContainerRestartInfo struct {
	Name            string         `json:"name"`
	Image           string         `json:"image"`
	RestartCount    int32          `json:"restartCount"`
	LastState       string         `json:"lastState"`       // running, waiting, terminated
	LastTermination *TerminationDetail `json:"lastTermination,omitempty"`
	CurrentWaiting  *WaitingDetail `json:"currentWaiting,omitempty"`
	Pattern         RestartPattern `json:"pattern"`
	RiskLevel       string         `json:"riskLevel"` // healthy, low, medium, high, critical
}

// TerminationDetail describes the last termination of a container.
type TerminationDetail struct {
	Reason     string `json:"reason"`     // OOMKilled, Error, Completed, etc.
	ExitCode   int32  `json:"exitCode"`
	Message    string `json:"message,omitempty"`
	Signal     int32  `json:"signal,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
}

// WaitingDetail describes why a container is currently waiting.
type WaitingDetail struct {
	Reason  string `json:"reason"`  // CrashLoopBackOff, ImagePullBackOff, ErrImagePull, etc.
	Message string `json:"message,omitempty"`
}

// PodRestartInfo is the restart diagnosis for a single pod.
type PodRestartInfo struct {
	Name            string                `json:"name"`
	Namespace       string                `json:"namespace"`
	Node            string                `json:"node"`
	Phase           string                `json:"phase"`
	CreatedAt       string                `json:"createdAt"`
	AgeHours        float64               `json:"ageHours"`
	TotalRestarts   int                   `json:"totalRestarts"`
	OverallPattern  RestartPattern        `json:"overallPattern"`
	OverallRisk     string                `json:"overallRisk"`
	Containers      []ContainerRestartInfo `json:"containers"`
}

// RestartDiagnosisResult is the full scan output.
type RestartDiagnosisResult struct {
	ScannedAt time.Time         `json:"scannedAt"`
	Summary   RestartSummary    `json:"summary"`
	Pods      []PodRestartInfo  `json:"pods"`
}

// RestartSummary aggregates restart statistics.
type RestartSummary struct {
	TotalPods        int `json:"totalPods"`
	PodsWithRestarts int `json:"podsWithRestarts"`
	TotalRestarts    int `json:"totalRestarts"`
	CrashLoops       int `json:"crashLoops"`
	HighRisk         int `json:"highRisk"`
	MediumRisk       int `json:"mediumRisk"`
	OOMKills         int `json:"oomKills"`
	WaitingPods      int `json:"waitingPods"` // pods with containers in CrashLoopBackOff etc.
}

// handleRestartDiagnosis diagnoses pod restart patterns across the cluster.
// GET /api/diagnostics/restarts?namespace=xxx
func (s *Server) handleRestartDiagnosis(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ns := r.URL.Query().Get("namespace")

	podList, err := rc.clientset.CoreV1().Pods(ns).List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := diagnoseRestarts(podList.Items)
	writeJSON(w, result)
}

// diagnoseRestarts analyzes all pods for restart patterns and root causes.
func diagnoseRestarts(pods []corev1.Pod) RestartDiagnosisResult {
	result := RestartDiagnosisResult{
		ScannedAt: time.Now(),
	}

	for _, pod := range pods {
		info := buildPodRestartInfo(&pod)
		if info.TotalRestarts == 0 && info.OverallRisk == "healthy" {
			result.Summary.TotalPods++
			continue
		}

		result.Summary.TotalPods++
		result.Summary.PodsWithRestarts++
		result.Summary.TotalRestarts += info.TotalRestarts

		switch info.OverallPattern {
		case RestartPatternCrashLoop:
			result.Summary.CrashLoops++
		}

		switch info.OverallRisk {
		case "high", "critical":
			result.Summary.HighRisk++
		case "medium":
			result.Summary.MediumRisk++
		}

		// Count OOM kills and waiting containers
		for _, c := range info.Containers {
			if c.LastTermination != nil && c.LastTermination.Reason == "OOMKilled" {
				result.Summary.OOMKills++
			}
			if c.CurrentWaiting != nil {
				result.Summary.WaitingPods++
			}
		}

		result.Pods = append(result.Pods, info)
	}

	// Sort by risk (critical first), then by restart count
	sort.Slice(result.Pods, func(i, j int) bool {
		ri := riskScore(result.Pods[i].OverallRisk)
		rj := riskScore(result.Pods[j].OverallRisk)
		if ri != rj {
			return ri < rj
		}
		return result.Pods[i].TotalRestarts > result.Pods[j].TotalRestarts
	})

	return result
}

// buildPodRestartInfo creates restart diagnosis for a single pod.
func buildPodRestartInfo(pod *corev1.Pod) PodRestartInfo {
	info := PodRestartInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Node:      pod.Spec.NodeName,
		Phase:     string(pod.Status.Phase),
		CreatedAt: pod.CreationTimestamp.Format(time.RFC3339),
	}

	age := time.Since(pod.CreationTimestamp.Time)
	info.AgeHours = roundTo2(age.Hours())

	var totalRestarts int
	var worstRisk string = "healthy"
	var worstPattern RestartPattern = RestartPatternNone

	for _, cs := range pod.Status.ContainerStatuses {
		cInfo := buildContainerRestartInfo(&cs, pod)
		info.Containers = append(info.Containers, cInfo)
		totalRestarts += int(cs.RestartCount)

		// Track worst risk and pattern
		if riskScore(cInfo.RiskLevel) < riskScore(worstRisk) {
			worstRisk = cInfo.RiskLevel
		}
		if patternPriority(cInfo.Pattern) < patternPriority(worstPattern) {
			worstPattern = cInfo.Pattern
		}
	}

	info.TotalRestarts = totalRestarts
	info.OverallRisk = worstRisk
	info.OverallPattern = worstPattern

	return info
}

// buildContainerRestartInfo analyzes a single container's restart behavior.
func buildContainerRestartInfo(cs *corev1.ContainerStatus, pod *corev1.Pod) ContainerRestartInfo {
	info := ContainerRestartInfo{
		Name:         cs.Name,
		Image:        cs.Image,
		RestartCount: cs.RestartCount,
		Pattern:      RestartPatternNone,
		RiskLevel:    "healthy",
	}

	// Extract last termination details (independent of current state — 
	// a container can have both a last termination AND be currently waiting)
	if cs.LastTerminationState.Terminated != nil {
		term := cs.LastTerminationState.Terminated
		info.LastState = "terminated"
		info.LastTermination = &TerminationDetail{
			Reason:     term.Reason,
			ExitCode:   term.ExitCode,
			Message:    term.Message,
			Signal:     term.Signal,
			FinishedAt: formatTimeVal(term.FinishedAt),
			StartedAt:  formatTimeVal(term.StartedAt),
		}
	}

	// Extract current state (can coexist with last termination)
	if cs.State.Running != nil {
		if info.LastState == "" {
			info.LastState = "running"
		}
	} else if cs.State.Waiting != nil {
		if info.LastState == "" {
			info.LastState = "waiting"
		}
		info.CurrentWaiting = &WaitingDetail{
			Reason:  cs.State.Waiting.Reason,
			Message: cs.State.Waiting.Message,
		}
	}

	// Classify restart pattern
	info.Pattern = classifyRestartPattern(cs.RestartCount, pod.CreationTimestamp.Time, info.LastTermination, info.CurrentWaiting)
	info.RiskLevel = classifyRestartRisk(info.Pattern, cs.RestartCount, info.LastTermination, info.CurrentWaiting)

	return info
}

// classifyRestartPattern determines the restart behavior pattern.
func classifyRestartPattern(restartCount int32, createdAt time.Time, term *TerminationDetail, waiting *WaitingDetail) RestartPattern {
	if restartCount == 0 && waiting == nil {
		return RestartPatternNone
	}

	podAge := time.Since(createdAt)

	// If currently in CrashLoopBackOff, it's a crash-loop
	if waiting != nil && isCrashLoopReason(waiting.Reason) {
		return RestartPatternCrashLoop
	}

	if restartCount == 0 {
		return RestartPatternNone
	}

	// Many restarts in a short period = crash-loop
	if restartCount >= 5 && podAge < 1*time.Hour {
		return RestartPatternCrashLoop
	}

	// Restarts shortly after creation = post-deploy
	if restartCount <= 3 && podAge < 30*time.Minute {
		return RestartPatternPostDeploy
	}

	// Few restarts over a longer period = occasional
	if restartCount < 5 || podAge > 24*time.Hour {
		return RestartPatternOccasional
	}

	// Default to crash-loop for high restart counts
	return RestartPatternCrashLoop
}

// classifyRestartRisk assigns a risk level based on pattern and conditions.
func classifyRestartRisk(pattern RestartPattern, restartCount int32, term *TerminationDetail, waiting *WaitingDetail) string {
	// Waiting in CrashLoopBackOff = critical
	if waiting != nil && isCrashLoopReason(waiting.Reason) {
		return "critical"
	}

	// Other waiting states (ImagePullBackOff etc) = high
	if waiting != nil {
		return "high"
	}

	switch pattern {
	case RestartPatternCrashLoop:
		if restartCount >= 10 {
			return "critical"
		}
		return "high"

	case RestartPatternOccasional:
		if restartCount >= 3 {
			return "medium"
		}
		return "low"

	case RestartPatternPostDeploy:
		return "low"

	case RestartPatternNone:
		return "healthy"
	}

	return "healthy"
}

// isCrashLoopReason returns true for reasons that indicate a crash loop.
func isCrashLoopReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff":
		return true
	}
	return false
}

// formatTimePtr formats a *metav1.Time safely.
func formatTimePtr(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatTimeVal formats a metav1.Time value.
func formatTimeVal(t metav1.Time) string {
	return t.Format(time.RFC3339)
}

// riskScore returns numeric priority for sorting (lower = more urgent).
func riskScore(risk string) int {
	switch risk {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	case "healthy":
		return 4
	default:
		return 5
	}
}

// patternPriority returns numeric priority for pattern classification.
func patternPriority(p RestartPattern) int {
	switch p {
	case RestartPatternCrashLoop:
		return 0
	case RestartPatternOccasional:
		return 1
	case RestartPatternPostDeploy:
		return 2
	case RestartPatternNone:
		return 3
	default:
		return 4
	}
}

// RestartDiagnosisHint generates a human-readable hint for a restart finding.
func RestartDiagnosisHint(info *PodRestartInfo) string {
	var hints []string

	for _, c := range info.Containers {
		if c.RestartCount == 0 && c.CurrentWaiting == nil {
			continue
		}

		if c.LastTermination != nil {
			switch c.LastTermination.Reason {
			case "OOMKilled":
				hints = append(hints, fmt.Sprintf("Container %s was OOMKilled (exit code %d) — increase memory limits or check for memory leaks",
					c.Name, c.LastTermination.ExitCode))
			case "Error":
				hints = append(hints, fmt.Sprintf("Container %s exited with error (exit code %d) — check application logs",
					c.Name, c.LastTermination.ExitCode))
			case "Completed":
				hints = append(hints, fmt.Sprintf("Container %s completed and was restarted — expected for Job/CronJob workloads",
					c.Name))
			}
		}

		if c.CurrentWaiting != nil {
			switch c.CurrentWaiting.Reason {
			case "CrashLoopBackOff":
				hints = append(hints, fmt.Sprintf("Container %s is in CrashLoopBackOff — application is failing to start, check previous container logs",
					c.Name))
			case "ImagePullBackOff", "ErrImagePull":
				hints = append(hints, fmt.Sprintf("Container %s cannot pull image %s — verify image name, tag, and registry credentials",
					c.Name, c.Image))
			case "CreateContainerConfigError":
				hints = append(hints, fmt.Sprintf("Container %s has config error — check env vars, secrets, and configmap references",
					c.Name))
			case "CreateContainerError":
				hints = append(hints, fmt.Sprintf("Container %s failed to create — check runtime and volume mounts",
					c.Name))
			}
		}
	}

	if len(hints) == 0 {
		return ""
	}
	return strings.Join(hints, "; ")
}
