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

// GoldenSignalsResult is the SRE Four Golden Signals unified health engine.
// It synthesizes Latency, Traffic, Errors, and Saturation into a single actionable health view.
type GoldenSignalsResult struct {
	ScannedAt      time.Time          `json:"scannedAt"`
	OverallScore   int                `json:"overallScore"`   // 0-100, weakest-link principle
	OverallGrade   string             `json:"overallGrade"`   // A-F
	Signals        []GoldenSignal     `json:"signals"`        // 4 signals
	TopIssues      []GoldenIssue      `json:"topIssues"`      // cross-signal issues
	ByNamespace    []GoldenNS         `json:"byNamespace"`    // per-namespace signal scores
	Recommendations []string          `json:"recommendations"`
}

// GoldenSignal represents one of the four SRE golden signals.
type GoldenSignal struct {
	Name        string        `json:"name"`        // latency, traffic, errors, saturation
	DisplayName string        `json:"displayName"` // human-readable
	Score       int           `json:"score"`       // 0-100
	Status      string        `json:"status"`      // healthy, warning, critical
	Summary     string        `json:"summary"`     // one-line description
	Metrics     []SignalMetric `json:"metrics"`   // supporting metrics
}

// SignalMetric is a single data point supporting a signal score.
type SignalMetric struct {
	Name     string `json:"name"`
	Value    string `json:"value"`    // human-readable value
	Status   string `json:"status"`   // healthy, warning, critical
	Detail   string `json:"detail,omitempty"`
}

// GoldenIssue is a cross-cutting issue identified by correlating signals.
type GoldenIssue struct {
	Signals    []string `json:"signals"`    // which signals contributed
	Namespace  string   `json:"namespace,omitempty"`
	Severity   string   `json:"severity"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Resolution string   `json:"resolution"`
}

// GoldenNS shows per-namespace golden signal scores.
type GoldenNS struct {
	Namespace  string `json:"namespace"`
	Latency    int    `json:"latencyScore"`
	Traffic    int    `json:"trafficScore"`
	Errors     int    `json:"errorScore"`
	Saturation int    `json:"saturationScore"`
	Overall    int    `json:"overallScore"`
	Status     string `json:"status"`
	PodCount   int    `json:"podCount"`
}

// handleGoldenSignals provides the SRE Four Golden Signals unified health view.
// GET /api/product/golden-signals
func (s *Server) handleGoldenSignals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := GoldenSignalsResult{ScannedAt: time.Now()}

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Gather all pods
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pods")
		return
	}

	// Gather nodes for saturation analysis
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		nodes = &corev1.NodeList{}
	}

	// Gather events for error analysis
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type!=Normal",
		Limit:         200,
	})

	now := time.Now()

	// --- Signal 1: LATENCY ---
	latencySignal := analyzeLatencySignal(pods.Items, now)

	// --- Signal 2: TRAFFIC ---
	trafficSignal := analyzeTrafficSignal(pods.Items, nodes.Items)

	// --- Signal 3: ERRORS ---
	errorSignal := analyzeErrorSignal(pods.Items, events.Items, now)

	// --- Signal 4: SATURATION ---
	saturationSignal := analyzeSaturationSignal(pods.Items, nodes.Items)

	result.Signals = []GoldenSignal{latencySignal, trafficSignal, errorSignal, saturationSignal}

	// Overall score = weakest link (minimum of all signals)
	minScore := 100
	for _, sig := range result.Signals {
		if sig.Score < minScore {
			minScore = sig.Score
		}
	}
	result.OverallScore = minScore
	result.OverallGrade = goldenScoreToGrade(minScore)

	// Per-namespace analysis
	result.ByNamespace = analyzeGoldenByNamespace(pods.Items, events.Items, systemNS, now)

	// Cross-signal issues
	result.TopIssues = findCrossSignalIssues(result.Signals, result.ByNamespace)

	// Recommendations
	result.Recommendations = generateGoldenRecs(result)

	writeJSON(w, result)
}

// analyzeLatencySignal evaluates the Latency golden signal.
func analyzeLatencySignal(pods []corev1.Pod, now time.Time) GoldenSignal {
	signal := GoldenSignal{
		Name:        "latency",
		DisplayName: "Latency",
		Score:       100,
		Status:      "healthy",
	}

	longStartingPods := 0
	notReadyContainers := 0
	containerWaitTotal := 0
	totalContainers := 0
	recentPods := 0

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			age := now.Sub(pod.CreationTimestamp.Time)
			if age > 2*time.Minute {
				longStartingPods++
			}
		}

		for _, cs := range pod.Status.ContainerStatuses {
			totalContainers++
			if !cs.Ready {
				notReadyContainers++
			}
			if cs.State.Terminated != nil && cs.State.Terminated.Reason == "Completed" {
				continue // Don't count completed containers as not ready
			}
			if cs.LastTerminationState.Terminated != nil {
				// Check restart timing
				if cs.LastTerminationState.Terminated.FinishedAt.After(now.Add(-5 * time.Minute)) {
					containerWaitTotal++
				}
			}
		}

		// Count pods started in the last hour for startup latency analysis
		if pod.CreationTimestamp.Time.After(now.Add(-time.Hour)) {
			recentPods++
		}
	}

	// Score calculation
	score := 100
	if totalContainers > 0 {
		notReadyPct := float64(notReadyContainers) / float64(totalContainers) * 100
		score -= int(notReadyPct * 0.5) // up to -50
	}
	score -= longStartingPods * 5
	if score < 0 {
		score = 0
	}
	signal.Score = score
	signal.Status = goldenScoreToStatus(score)

	signal.Metrics = []SignalMetric{
		{
			Name:   "not_ready_containers",
			Value:  fmt.Sprintf("%d / %d", notReadyContainers, totalContainers),
			Status: severityFromRatio(notReadyContainers, totalContainers, 0.1, 0.3),
			Detail: "Containers that are not in Ready state",
		},
		{
			Name:   "long_starting_pods",
			Value:  fmt.Sprintf("%d", longStartingPods),
			Status: severityFromCount(longStartingPods, 3, 10),
			Detail: "Pods pending for more than 2 minutes",
		},
		{
			Name:   "recent_pod_starts",
			Value:  fmt.Sprintf("%d", recentPods),
			Status: "healthy",
			Detail: "Pods started in the last hour",
		},
	}

	signal.Summary = fmt.Sprintf("%d/%d containers ready, %d pods slow to start", totalContainers-notReadyContainers, totalContainers, longStartingPods)

	return signal
}

// analyzeTrafficSignal evaluates the Traffic golden signal.
func analyzeTrafficSignal(pods []corev1.Pod, nodes []corev1.Node) GoldenSignal {
	signal := GoldenSignal{
		Name:        "traffic",
		DisplayName: "Traffic",
		Score:       100,
		Status:      "healthy",
	}

	totalRunningPods := 0
	totalEndpoints := 0
	runningPodsByNS := make(map[string]int)

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning {
			totalRunningPods++
			runningPodsByNS[pod.Namespace]++
			// Each running pod with ready containers contributes to endpoints
			ready := true
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
					break
				}
			}
			if ready {
				totalEndpoints++
			}
		}
	}

	// Traffic health is about serving capacity
	// Low endpoint count relative to pods = degraded traffic handling
	endpointRatio := 1.0
	if totalRunningPods > 0 {
		endpointRatio = float64(totalEndpoints) / float64(totalRunningPods)
	}

	score := 100
	if endpointRatio < 0.95 {
		score -= int((1.0 - endpointRatio) * 50) // up to -50
	}
	if totalRunningPods == 0 {
		score = 0 // no traffic capacity at all
	}
	if score < 0 {
		score = 0
	}
	signal.Score = score
	signal.Status = goldenScoreToStatus(score)

	notReadyRatio := 0.0
	if totalRunningPods > 0 {
		notReadyRatio = 1.0 - endpointRatio
	}

	signal.Metrics = []SignalMetric{
		{
			Name:   "running_pods",
			Value:  fmt.Sprintf("%d", totalRunningPods),
			Status: "healthy",
			Detail: "Pods currently in Running phase",
		},
		{
			Name:   "ready_endpoints",
			Value:  fmt.Sprintf("%d (%.1f%%)", totalEndpoints, endpointRatio*100),
			Status: severityFromRatioFloat(notReadyRatio, 0.05, 0.15),
			Detail: "Running pods with all containers ready (serving traffic)",
		},
		{
			Name:   "node_capacity",
			Value:  fmt.Sprintf("%d nodes", len(nodes)),
			Status: "healthy",
			Detail: "Nodes available for traffic distribution",
		},
	}

	signal.Summary = fmt.Sprintf("%d/%d pods serving traffic (%.1f%% endpoint readiness)", totalEndpoints, totalRunningPods, endpointRatio*100)

	return signal
}

// analyzeErrorSignal evaluates the Errors golden signal.
func analyzeErrorSignal(pods []corev1.Pod, events []corev1.Event, now time.Time) GoldenSignal {
	signal := GoldenSignal{
		Name:        "errors",
		DisplayName: "Errors",
		Score:       100,
		Status:      "healthy",
	}

	crashLoopPods := 0
	highRestartPods := 0
	totalRestarts := 0
	recentWarnings := 0
	oomKills := 0

	for _, pod := range pods {
		// Check for CrashLoopBackOff
		if pod.Status.Phase == corev1.PodRunning {
			for _, cs := range pod.Status.ContainerStatuses {
				totalRestarts += int(cs.RestartCount)
				if cs.RestartCount > 5 {
					highRestartPods++
				}
				if cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled" {
					oomKills++
				}
				if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					oomKills++
				}
			}
		}
		// Check container waiting states for CrashLoopBackOff
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoopPods++
			}
		}
	}

	// Count recent warning events
	for _, ev := range events {
		if ev.LastTimestamp.Time.After(now.Add(-time.Hour)) {
			recentWarnings++
		}
	}

		// Score: penalize for each error indicator
	score := 100
	score -= crashLoopPods * 20
	score -= highRestartPods * 8
	score -= oomKills * 5
	if recentWarnings > 10 {
		score -= (recentWarnings - 10) // each warning above 10 reduces by 1
	}
	if score < 0 {
		score = 0
	}
	signal.Score = score
	signal.Status = goldenScoreToStatus(score)

	signal.Metrics = []SignalMetric{
		{
			Name:   "crashloop_pods",
			Value:  fmt.Sprintf("%d", crashLoopPods),
			Status: severityFromCount(crashLoopPods, 1, 5),
			Detail: "Pods in CrashLoopBackOff state",
		},
		{
			Name:   "high_restart_pods",
			Value:  fmt.Sprintf("%d", highRestartPods),
			Status: severityFromCount(highRestartPods, 3, 10),
			Detail: "Pods with >5 restarts",
		},
		{
			Name:   "total_restarts",
			Value:  fmt.Sprintf("%d", totalRestarts),
			Status: severityFromCount(totalRestarts, 20, 100),
			Detail: "Cumulative container restart count",
		},
		{
			Name:   "oom_kills",
			Value:  fmt.Sprintf("%d", oomKills),
			Status: severityFromCount(oomKills, 1, 5),
			Detail: "Containers killed by OOMKiller",
		},
		{
			Name:   "recent_warnings",
			Value:  fmt.Sprintf("%d", recentWarnings),
			Status: severityFromCount(recentWarnings, 10, 50),
			Detail: "Warning events in the last hour",
		},
	}

	signal.Summary = fmt.Sprintf("%d crashloops, %d high-restart pods, %d OOM kills, %d warnings/hr",
		crashLoopPods, highRestartPods, oomKills, recentWarnings)

	return signal
}

// analyzeSaturationSignal evaluates the Saturation golden signal.
func analyzeSaturationSignal(pods []corev1.Pod, nodes []corev1.Node) GoldenSignal {
	signal := GoldenSignal{
		Name:        "saturation",
		DisplayName: "Saturation",
		Score:       100,
		Status:      "healthy",
	}

	// Check node conditions for pressure
	nodePressure := 0
	readyNodes := 0
	for _, node := range nodes {
		isReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				nodePressure++ // DiskPressure, MemoryPressure, PIDPressure, NetworkUnavailable
			}
		}
		if isReady {
			readyNodes++
		}
	}

	// Check for pods without resource limits (unbounded consumption risk)
	podsWithoutLimits := 0
	totalPods := 0
	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		totalPods++
		hasLimits := false
		for _, c := range pod.Spec.Containers {
			if c.Resources.Limits != nil {
				if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
					hasLimits = true
				}
				if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
					hasLimits = true
				}
			}
		}
		if !hasLimits {
			podsWithoutLimits++
		}
	}

	// Check for pending pods (scheduling saturation)
	pendingPods := 0
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodPending {
			pendingPods++
		}
	}

	// Score calculation
	score := 100
	score -= nodePressure * 10
	score -= pendingPods * 5
	if totalPods > 0 {
		noLimitPct := float64(podsWithoutLimits) / float64(totalPods) * 100
		score -= int(noLimitPct * 0.2) // up to -20
	}
	if readyNodes == 0 && len(nodes) > 0 {
		score = 0 // no ready nodes = fully saturated
	}
	if score < 0 {
		score = 0
	}
	signal.Score = score
	signal.Status = goldenScoreToStatus(score)

	signal.Metrics = []SignalMetric{
		{
			Name:   "node_pressure",
			Value:  fmt.Sprintf("%d conditions", nodePressure),
			Status: severityFromCount(nodePressure, 1, 5),
			Detail: "Nodes with DiskPressure/MemoryPressure/PIDPressure",
		},
		{
			Name:   "ready_nodes",
			Value:  fmt.Sprintf("%d / %d", readyNodes, len(nodes)),
			Status: severityFromRatio(readyNodes, len(nodes), 0.7, 0.5),
			Detail: "Nodes in Ready condition",
		},
		{
			Name:   "pending_pods",
			Value:  fmt.Sprintf("%d", pendingPods),
			Status: severityFromCount(pendingPods, 5, 20),
			Detail: "Pods stuck in Pending (scheduling saturation)",
		},
		{
			Name:   "pods_without_limits",
			Value:  fmt.Sprintf("%d / %d", podsWithoutLimits, totalPods),
			Status: severityFromRatio(podsWithoutLimits, totalPods, 0.3, 0.5),
			Detail: "Running pods without resource limits (unbounded consumption risk)",
		},
	}

	signal.Summary = fmt.Sprintf("%d/%d nodes ready, %d node pressures, %d pending pods, %d pods without limits",
		readyNodes, len(nodes), nodePressure, pendingPods, podsWithoutLimits)

	return signal
}

// analyzeGoldenByNamespace computes per-namespace golden signal scores.
func analyzeGoldenByNamespace(pods []corev1.Pod, events []corev1.Event, systemNS map[string]bool, now time.Time) []GoldenNS {
	type nsSignals struct {
		notReady      int
		totalCont     int
		restarts      int
		pendingPods   int
		crashLoops    int
		runningPods   int
		warnings      int
	}

	nsMap := make(map[string]*nsSignals)

	for _, pod := range pods {
		ns := pod.Namespace
		if systemNS[ns] {
			continue
		}
		s, ok := nsMap[ns]
		if !ok {
			s = &nsSignals{}
			nsMap[ns] = s
		}
		s.totalCont += len(pod.Status.ContainerStatuses)
		if pod.Status.Phase == corev1.PodRunning {
			s.runningPods++
		}
		if pod.Status.Phase == corev1.PodPending {
			s.pendingPods++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				s.notReady++
			}
			s.restarts += int(cs.RestartCount)
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				s.crashLoops++
			}
		}
	}

	// Count events by namespace
	for _, ev := range events {
		ns := ev.InvolvedObject.Namespace
		if systemNS[ns] {
			continue
		}
		if ev.LastTimestamp.Time.After(now.Add(-time.Hour)) {
			s, ok := nsMap[ns]
			if !ok {
				s = &nsSignals{}
				nsMap[ns] = s
			}
			s.warnings++
		}
	}

	var result []GoldenNS
	for nsName, s := range nsMap {
		latency := 100
		if s.totalCont > 0 {
			latency -= int(float64(s.notReady) / float64(s.totalCont) * 50)
		}
		latency -= s.pendingPods * 5

		traffic := 100
		if s.runningPods > 0 {
			readyRatio := float64(s.runningPods-s.notReady) / float64(s.runningPods)
			if readyRatio < 1.0 {
				traffic -= int((1.0 - readyRatio) * 50)
			}
		} else {
			traffic = 0
		}

		errors := 100
		errors -= s.crashLoops * 20
		errors -= s.warnings
		if s.restarts > 5 {
			errors -= (s.restarts - 5)
		}

		saturation := 100
		saturation -= s.pendingPods * 5
		if s.runningPods == 0 && s.totalCont > 0 {
			saturation -= 30
		}

		// Clamp
		for _, v := range []*int{&latency, &traffic, &errors, &saturation} {
			if *v < 0 {
				*v = 0
			}
			if *v > 100 {
				*v = 100
			}
		}

		overall := min4(latency, traffic, errors, saturation)

		result = append(result, GoldenNS{
			Namespace:  nsName,
			Latency:    latency,
			Traffic:    traffic,
			Errors:     errors,
			Saturation: saturation,
			Overall:    overall,
			Status:     goldenScoreToStatus(overall),
			PodCount:   s.runningPods + s.pendingPods,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Overall < result[j].Overall // worst first
	})

	return result
}

// findCrossSignalIssues correlates multiple signals to identify compound problems.
func findCrossSignalIssues(signals []GoldenSignal, byNS []GoldenNS) []GoldenIssue {
	var issues []GoldenIssue

	// Signal lookup
	sigMap := make(map[string]GoldenSignal)
	for _, s := range signals {
		sigMap[s.Name] = s
	}

	// Pattern 1: High errors + low latency = silent failures (pods start fast but crash)
	if sigMap["errors"].Score < 60 && sigMap["latency"].Score > 70 {
		issues = append(issues, GoldenIssue{
			Signals:    []string{"errors", "latency"},
			Severity:   "critical",
			Title:      "Silent failure pattern detected",
			Detail:      "Pods start quickly but have high error rates — investigate CrashLoopBackOff and OOMKill patterns",
			Resolution: "Check container logs, resource limits, and application health endpoints",
		})
	}

	// Pattern 2: High saturation + elevated errors = cascading failure risk
	if sigMap["saturation"].Score < 50 && sigMap["errors"].Score < 75 {
		issues = append(issues, GoldenIssue{
			Signals:    []string{"saturation", "errors"},
			Severity:   "critical",
			Title:      "Potential cascading failure risk",
			Detail:      "Resource saturation coinciding with high error rate — cluster may be in or approaching cascading failure",
			Resolution: "Reduce load, scale out nodes, or investigate resource contention immediately",
		})
	}

	// Pattern 3: Low traffic + healthy pods = unused capacity or networking issue
	if sigMap["traffic"].Score < 60 && sigMap["latency"].Score > 70 {
		issues = append(issues, GoldenIssue{
			Signals:    []string{"traffic", "latency"},
			Severity:   "warning",
			Title:      "Low serving capacity despite healthy pods",
			Detail:      "Many pods are running but not ready to serve — possible readiness probe misconfiguration or networking issue",
			Resolution: "Check readiness probes, service selectors, and endpoint health",
		})
	}

	// Pattern 4: Namespace-specific compound issues
	for _, ns := range byNS {
		if ns.Overall < 40 {
			var signals []string
			if ns.Errors < 40 {
				signals = append(signals, "errors")
			}
			if ns.Saturation < 40 {
				signals = append(signals, "saturation")
			}
			if ns.Latency < 40 {
				signals = append(signals, "latency")
			}
			if len(signals) >= 2 {
				issues = append(issues, GoldenIssue{
					Signals:    signals,
					Namespace:  ns.Namespace,
					Severity:   "critical",
					Title:      fmt.Sprintf("Namespace %s has compound signal degradation", ns.Namespace),
					Detail:      fmt.Sprintf("Multiple signals failing: latency=%d, traffic=%d, errors=%d, saturation=%d", ns.Latency, ns.Traffic, ns.Errors, ns.Saturation),
					Resolution: fmt.Sprintf("Investigate namespace %s — check events, resource pressure, and pod health", ns.Namespace),
				})
			}
		}
	}

	// Limit issues
	if len(issues) > 10 {
		issues = issues[:10]
	}

	return issues
}

// generateGoldenRecs produces actionable recommendations from signal analysis.
func generateGoldenRecs(result GoldenSignalsResult) []string {
	var recs []string

	// Weakest signal recommendation
	sigMap := make(map[string]GoldenSignal)
	for _, s := range result.Signals {
		sigMap[s.Name] = s
	}

	weakest := result.Signals[0]
	for _, s := range result.Signals {
		if s.Score < weakest.Score {
			weakest = s
		}
	}

	recs = append(recs, fmt.Sprintf("Weakest signal: %s (score %d) — focus optimization here first", weakest.DisplayName, weakest.Score))

	// Signal-specific recommendations
	if sigMap["errors"].Score < 70 {
		recs = append(recs, "Error rate is elevated — review CrashLoopBackOff pods, OOMKill events, and recent warning events")
	}
	if sigMap["saturation"].Score < 70 {
		recs = append(recs, "Resource saturation detected — add nodes, right-size workloads, or add resource limits to unbounded pods")
	}
	if sigMap["latency"].Score < 70 {
		recs = append(recs, "Latency issues detected — investigate pods stuck in Pending, slow container starts, and not-ready containers")
	}
	if sigMap["traffic"].Score < 70 {
		recs = append(recs, "Traffic capacity is reduced — check for pods that are Running but not Ready (readiness probe failures)")
	}

	// Cross-signal issues
	critCount := 0
	for _, iss := range result.TopIssues {
		if iss.Severity == "critical" {
			critCount++
		}
	}
	if critCount > 0 {
		recs = append(recs, fmt.Sprintf("%d critical cross-signal issues detected — compound failure patterns require immediate attention", critCount))
	}

	// Namespace hotspot
	if len(result.ByNamespace) > 0 && result.ByNamespace[0].Overall < 50 {
		ns := result.ByNamespace[0]
		recs = append(recs, fmt.Sprintf("Namespace hotspot: '%s' has the lowest overall score (%d) — prioritize investigation", ns.Namespace, ns.Overall))
	}

	return recs
}

// Helper functions

func goldenScoreToGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

func goldenScoreToStatus(score int) string {
	switch {
	case score >= 80:
		return "healthy"
	case score >= 60:
		return "warning"
	default:
		return "critical"
	}
}

func severityFromCount(value, warnThreshold, critThreshold int) string {
	if value >= critThreshold {
		return "critical"
	}
	if value >= warnThreshold {
		return "warning"
	}
	return "healthy"
}

func severityFromRatio(part, total int, warnPct, critPct float64) string {
	if total == 0 {
		return "healthy"
	}
	ratio := float64(part) / float64(total)
	if ratio >= critPct {
		return "critical"
	}
	if ratio >= warnPct {
		return "warning"
	}
	return "healthy"
}

func severityFromRatioFloat(ratio, warnPct, critPct float64) string {
	if ratio >= critPct {
		return "critical"
	}
	if ratio >= warnPct {
		return "warning"
	}
	return "healthy"
}

func min4(a, b, c, d int) int {
	min := a
	if b < min {
		min = b
	}
	if c < min {
		min = c
	}
	if d < min {
		min = d
	}
	return min
}

// Suppress unused import warning for strings (used in findCrossSignalIssues pattern strings)
var _ = strings.Contains
