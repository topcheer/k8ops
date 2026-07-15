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

// IncidentCorrelationResult is the output of the multi-signal incident
// correlation and root cause suggestion engine.
type IncidentCorrelationResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         IncidentSummary   `json:"summary"`
	Incidents       []IncidentCluster `json:"incidents"`
	OrphanSignals   []SignalEntry     `json:"orphanSignals,omitempty"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// IncidentSummary aggregates cluster-wide incident statistics.
type IncidentSummary struct {
	TotalSignals       int `json:"totalSignals"`
	TotalIncidents     int `json:"totalIncidents"`
	CriticalCount      int `json:"criticalCount"`
	HighCount          int `json:"highCount"`
	MediumCount        int `json:"mediumCount"`
	LowCount           int `json:"lowCount"`
	OrphanSignalCount  int `json:"orphanSignalCount"`
	AffectedNamespaces int `json:"affectedNamespaces"`
	AffectedNodes      int `json:"affectedNodes"`
}

// IncidentCluster represents a correlated group of signals forming an incident.
type IncidentCluster struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Severity       string          `json:"severity"` // critical, high, medium, low
	StartTime      time.Time       `json:"startTime"`
	EndTime        time.Time       `json:"endTime"`
	DurationMin    float64         `json:"durationMin"`
	Namespace      string          `json:"namespace"`
	Node           string          `json:"node,omitempty"`
	SignalCount    int             `json:"signalCount"`
	Signals        []SignalEntry   `json:"signals"`
	RootCause      *RootCauseGuess `json:"rootCause,omitempty"`
	BlastRadius    BlastRadius     `json:"blastRadius"`
	CorrelationKey string          `json:"correlationKey"` // namespace, node, workload
	Category       string          `json:"category"`       // resource-pressure, scheduling, networking, storage, config, security, unknown
}

// SignalEntry is a single correlated signal from any source.
type SignalEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // event, pod-status, node-condition
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"` // resource name
	Kind      string    `json:"kind"` // Pod, Node, etc.
	Node      string    `json:"node,omitempty"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`     // critical, warning, info
	Category  string    `json:"category"`     // resource-pressure, scheduling, etc.
	Causal    bool      `json:"causalWeight"` // likely to be a root cause
}

// RootCauseGuess is a probable root cause with confidence.
type RootCauseGuess struct {
	Description string      `json:"description"`
	Signal      SignalEntry `json:"signal"`
	Confidence  int         `json:"confidence"` // 0-100
	Category    string      `json:"category"`
}

// BlastRadius describes the impact scope of an incident.
type BlastRadius struct {
	AffectedPods       int      `json:"affectedPods"`
	AffectedNamespaces []string `json:"affectedNamespaces"`
	AffectedNodes      []string `json:"affectedNodes"`
	AffectedWorkloads  []string `json:"affectedWorkloads"`
}

// handleIncidentCorrelation is the multi-signal incident correlation and
// root cause suggestion engine. It collects signals from cluster events,
// pod lifecycle data, and node conditions, then groups related signals into
// incidents with probable root cause analysis.
// GET /api/operations/incident-correlation?namespace=xxx&window=60
func (s *Server) handleIncidentCorrelation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	// Time window in minutes (default 60, max 360)
	windowMin := 60
	if w := r.URL.Query().Get("window"); w != "" {
		if v, err := parseIntSafe(w, 60); err == nil && v > 0 && v <= 360 {
			windowMin = v
		}
	}

	cutoff := time.Now().Add(-time.Duration(windowMin) * time.Minute)
	now := time.Now()

	var signals []SignalEntry

	// --- Source 1: Warning Events ---
	events, err := rc.clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         2000,
	})
	if err == nil {
		for i := range events.Items {
			ev := &events.Items[i]
			evTime := ev.LastTimestamp.Time
			if evTime.IsZero() {
				evTime = ev.CreationTimestamp.Time
			}
			if evTime.Before(cutoff) {
				continue
			}

			sig := SignalEntry{
				Timestamp: evTime,
				Source:    "event",
				Namespace: ev.InvolvedObject.Namespace,
				Name:      ev.InvolvedObject.Name,
				Kind:      ev.InvolvedObject.Kind,
				Node:      extractNodeFromEvent(ev),
				Reason:    ev.Reason,
				Message:   truncateMsg(ev.Message, 200),
				Severity:  classifyEventSeverity(ev.Reason),
				Category:  categorizeSignal(ev.Reason, ev.Message),
				Causal:    isCausalReason(ev.Reason),
			}
			signals = append(signals, sig)
		}
	}

	// --- Source 2: Pod Lifecycle Signals ---
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range pods.Items {
			pod := &pods.Items[i]

			// CrashLoopBackOff
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
					signals = append(signals, SignalEntry{
						Timestamp: now,
						Source:    "pod-status",
						Namespace: pod.Namespace,
						Name:      pod.Name,
						Kind:      "Pod",
						Node:      pod.Spec.NodeName,
						Reason:    "CrashLoopBackOff",
						Message:   fmt.Sprintf("Container %s in CrashLoopBackOff", cs.Name),
						Severity:  "critical",
						Category:  "application",
						Causal:    false,
					})
				}
				// OOMKilled
				if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					finishTime := cs.LastTerminationState.Terminated.FinishedAt.Time
					if finishTime.IsZero() || finishTime.After(cutoff) {
						signals = append(signals, SignalEntry{
							Timestamp: finishTime,
							Source:    "pod-status",
							Namespace: pod.Namespace,
							Name:      pod.Name,
							Kind:      "Pod",
							Node:      pod.Spec.NodeName,
							Reason:    "OOMKilled",
							Message:   fmt.Sprintf("Container %s killed by OOM (exit %d)", cs.Name, cs.LastTerminationState.Terminated.ExitCode),
							Severity:  "critical",
							Category:  "resource-pressure",
							Causal:    false,
						})
					}
				}
			}

			// High restart count
			totalRestarts := int32(0)
			for _, cs := range pod.Status.ContainerStatuses {
				totalRestarts += cs.RestartCount
			}
			if totalRestarts >= 5 {
				signals = append(signals, SignalEntry{
					Timestamp: now,
					Source:    "pod-status",
					Namespace: pod.Namespace,
					Name:      pod.Name,
					Kind:      "Pod",
					Node:      pod.Spec.NodeName,
					Reason:    "HighRestartCount",
					Message:   fmt.Sprintf("Pod has %d total container restarts", totalRestarts),
					Severity:  "warning",
					Category:  "application",
					Causal:    false,
				})
			}

			// Pod not in Running phase
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
				if pod.Status.StartTime != nil && pod.Status.StartTime.After(cutoff) {
					signals = append(signals, SignalEntry{
						Timestamp: pod.Status.StartTime.Time,
						Source:    "pod-status",
						Namespace: pod.Namespace,
						Name:      pod.Name,
						Kind:      "Pod",
						Node:      pod.Spec.NodeName,
						Reason:    fmt.Sprintf("Phase:%s", pod.Status.Phase),
						Message:   fmt.Sprintf("Pod is in %s phase", pod.Status.Phase),
						Severity:  classifyPhaseSeverity(pod.Status.Phase),
						Category:  "scheduling",
						Causal:    false,
					})
				}
			}
		}
	}

	// --- Source 3: Node Condition Signals ---
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range nodes.Items {
			node := &nodes.Items[i]
			for _, cond := range node.Status.Conditions {
				if cond.Status != corev1.ConditionTrue {
					continue
				}
				if cond.Type == corev1.NodeReady {
					continue // Ready=True is good
				}
				// Only process negative conditions
				if cond.LastTransitionTime.IsZero() {
					continue
				}
				// Check if transitioned within window
				if cond.LastTransitionTime.Time.Before(cutoff) {
					continue
				}
				signals = append(signals, SignalEntry{
					Timestamp: cond.LastTransitionTime.Time,
					Source:    "node-condition",
					Namespace: "",
					Name:      node.Name,
					Kind:      "Node",
					Node:      node.Name,
					Reason:    string(cond.Type),
					Message:   fmt.Sprintf("Node %s: %s is True (%s)", node.Name, cond.Type, truncateMsg(cond.Message, 150)),
					Severity:  classifyNodeCondition(cond.Type),
					Category:  "resource-pressure",
					Causal:    true, // Node pressure is often a root cause
				})
			}
		}
	}

	// Sort signals by timestamp
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Timestamp.Before(signals[j].Timestamp)
	})

	// --- Correlation Engine ---
	incidents := correlateSignals(signals, now)

	// --- Build result ---
	result := IncidentCorrelationResult{
		ScannedAt: now,
		Summary: IncidentSummary{
			TotalSignals: len(signals),
		},
	}

	// Collect orphan signals (not in any incident)
	incidentSignalIdx := map[int]bool{}
	for _, inc := range incidents {
		for _, sig := range inc.Signals {
			// Mark signals that are in incidents
			for i, s := range signals {
				if signalEqual(s, sig) {
					incidentSignalIdx[i] = true
					break
				}
			}
		}
	}
	for i, sig := range signals {
		if !incidentSignalIdx[i] {
			result.OrphanSignals = append(result.OrphanSignals, sig)
		}
	}
	if len(result.OrphanSignals) > 50 {
		result.OrphanSignals = result.OrphanSignals[:50]
	}

	// Sort incidents by severity then signal count
	sort.Slice(incidents, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		si, sj := sevOrder[incidents[i].Severity], sevOrder[incidents[j].Severity]
		if si != sj {
			return si < sj
		}
		return incidents[i].SignalCount > incidents[j].SignalCount
	})
	result.Incidents = incidents

	// Build summary
	result.Summary.TotalIncidents = len(incidents)
	for _, inc := range incidents {
		switch inc.Severity {
		case "critical":
			result.Summary.CriticalCount++
		case "high":
			result.Summary.HighCount++
		case "medium":
			result.Summary.MediumCount++
		case "low":
			result.Summary.LowCount++
		}
	}
	result.Summary.OrphanSignalCount = len(result.OrphanSignals)

	// Affected namespaces and nodes
	nsSet := map[string]bool{}
	nodeSet := map[string]bool{}
	for _, sig := range signals {
		if sig.Namespace != "" {
			nsSet[sig.Namespace] = true
		}
		if sig.Node != "" {
			nodeSet[sig.Node] = true
		}
	}
	result.Summary.AffectedNamespaces = len(nsSet)
	result.Summary.AffectedNodes = len(nodeSet)

	// Health score: start at 100, deduct for incidents
	score := 100
	for _, inc := range incidents {
		switch inc.Severity {
		case "critical":
			score -= 25
		case "high":
			score -= 15
		case "medium":
			score -= 8
		case "low":
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	result.Recommendations = generateIncidentRecommendations(result)

	writeJSON(w, result)
}

// correlateSignals groups related signals into incident clusters.
// Signals are correlated if they share the same namespace or node and
// occur within a time proximity window (default 5 minutes).
func correlateSignals(signals []SignalEntry, now time.Time) []IncidentCluster {
	if len(signals) == 0 {
		return nil
	}

	const proximityWindow = 5 * time.Minute

	// Build correlation groups using union-find
	parent := make([]int, len(signals))
	for i := range parent {
		parent[i] = i
	}

	var find func(x int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) {
		px, py := find(x), find(y)
		if px != py {
			parent[px] = py
		}
	}

	// Correlate signals that share namespace or node and are within proximity window
	for i := 0; i < len(signals); i++ {
		for j := i + 1; j < len(signals); j++ {
			si, sj := signals[i], signals[j]

			// Time proximity check
			timeDiff := sj.Timestamp.Sub(si.Timestamp)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			if timeDiff > proximityWindow {
				// Even if same resource, if they're far apart they might be separate incidents
				// But still correlate if exactly same resource
				if !(si.Namespace == sj.Namespace && si.Name == sj.Name && si.Kind == sj.Kind) {
					continue
				}
			}

			// Correlation criteria
			sameNamespace := si.Namespace != "" && si.Namespace == sj.Namespace
			sameNode := si.Node != "" && si.Node == sj.Node
			sameResource := si.Name == sj.Name && si.Kind == sj.Kind && si.Namespace == sj.Namespace
			causalLink := (si.Causal && (sj.Category == si.Category || sj.Namespace == si.Namespace)) ||
				(sj.Causal && (si.Category == sj.Category || si.Namespace == sj.Namespace))

			if sameResource || sameNode || (sameNamespace && timeDiff <= proximityWindow) || causalLink {
				union(i, j)
			}
		}
	}

	// Group signals by root
	groups := map[int][]int{}
	for i := range signals {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	// Build incident clusters from groups
	var incidents []IncidentCluster
	incCounter := 0
	for _, indices := range groups {
		if len(indices) < 2 {
			// Single-signal groups are not incidents
			continue
		}

		incCounter++
		cluster := buildIncidentCluster(indices, signals, fmt.Sprintf("INC-%03d", incCounter), now)
		incidents = append(incidents, cluster)
	}

	return incidents
}

// buildIncidentCluster constructs an IncidentCluster from a group of signal indices.
func buildIncidentCluster(indices []int, signals []SignalEntry, id string, now time.Time) IncidentCluster {
	var clusterSignals []SignalEntry
	var minTime, maxTime time.Time
	nsSet := map[string]bool{}
	nodeSet := map[string]bool{}
	podSet := map[string]bool{}
	workloadSet := map[string]bool{}

	for _, idx := range indices {
		sig := signals[idx]
		clusterSignals = append(clusterSignals, sig)

		if minTime.IsZero() || sig.Timestamp.Before(minTime) {
			minTime = sig.Timestamp
		}
		if maxTime.IsZero() || sig.Timestamp.After(maxTime) {
			maxTime = sig.Timestamp
		}
		if sig.Namespace != "" {
			nsSet[sig.Namespace] = true
		}
		if sig.Node != "" {
			nodeSet[sig.Node] = true
		}
		if sig.Kind == "Pod" {
			podSet[sig.Name] = true
			// Try to extract workload name (strip pod hash suffix)
			workload := extractWorkloadName(sig.Name)
			if workload != "" {
				workloadSet[workload] = true
			}
		}
	}

	// Sort cluster signals by timestamp
	sort.Slice(clusterSignals, func(i, j int) bool {
		return clusterSignals[i].Timestamp.Before(clusterSignals[j].Timestamp)
	})

	// Determine severity
	severity := "low"
	criticalCount := 0
	warningCount := 0
	for _, sig := range clusterSignals {
		switch sig.Severity {
		case "critical":
			criticalCount++
		case "warning":
			warningCount++
		}
	}
	switch {
	case criticalCount >= 2:
		severity = "critical"
	case criticalCount >= 1 || warningCount >= 5:
		severity = "high"
	case warningCount >= 2:
		severity = "medium"
	}

	// Determine primary namespace/node
	primaryNS := ""
	for ns := range nsSet {
		primaryNS = ns
		break
	}
	primaryNode := ""
	if len(nodeSet) == 1 {
		for n := range nodeSet {
			primaryNode = n
		}
	}

	// Determine correlation key
	corrKey := primaryNS
	if primaryNode != "" {
		corrKey = fmt.Sprintf("node:%s", primaryNode)
	}

	// Determine category from majority of signals
	catCount := map[string]int{}
	for _, sig := range clusterSignals {
		catCount[sig.Category]++
	}
	dominantCategory := "unknown"
	maxCat := 0
	for cat, count := range catCount {
		if count > maxCat {
			maxCat = count
			dominantCategory = cat
		}
	}

	// Find root cause: earliest causal signal
	rootCause := findRootCause(clusterSignals)

	// Build title
	title := buildIncidentTitle(dominantCategory, primaryNS, primaryNode, clusterSignals)

	durationMin := 0.0
	if !minTime.IsZero() && !maxTime.IsZero() {
		durationMin = maxTime.Sub(minTime).Minutes()
		if durationMin < 0 {
			durationMin = 0
		}
	}

	// Build namespaces and nodes slices
	var nsList []string
	for ns := range nsSet {
		nsList = append(nsList, ns)
	}
	sort.Strings(nsList)
	var nodeList []string
	for n := range nodeSet {
		nodeList = append(nodeList, n)
	}
	sort.Strings(nodeList)
	var workloadList []string
	for wl := range workloadSet {
		workloadList = append(workloadList, wl)
	}
	sort.Strings(workloadList)

	return IncidentCluster{
		ID:          id,
		Title:       title,
		Severity:    severity,
		StartTime:   minTime,
		EndTime:     maxTime,
		DurationMin: durationMin,
		Namespace:   primaryNS,
		Node:        primaryNode,
		SignalCount: len(clusterSignals),
		Signals:     clusterSignals,
		RootCause:   rootCause,
		BlastRadius: BlastRadius{
			AffectedPods:       len(podSet),
			AffectedNamespaces: nsList,
			AffectedNodes:      nodeList,
			AffectedWorkloads:  workloadList,
		},
		CorrelationKey: corrKey,
		Category:       dominantCategory,
	}
}

// findRootCause identifies the most probable root cause signal.
func findRootCause(signals []SignalEntry) *RootCauseGuess {
	if len(signals) == 0 {
		return nil
	}

	// Priority: causal signals first, then earliest
	type candidate struct {
		sig        SignalEntry
		confidence int
	}
	var candidates []candidate

	for _, sig := range signals {
		if !sig.Causal {
			continue
		}
		conf := 60 // Base confidence for causal signals
		// Boost confidence based on category
		switch sig.Category {
		case "resource-pressure":
			conf += 20 // Node pressure is strong root cause
		case "scheduling":
			conf += 15
		case "storage":
			conf += 10
		}
		// Boost if it's the earliest signal
		if len(signals) > 0 && sig.Timestamp.Equal(signals[0].Timestamp) {
			conf += 10
		}
		if conf > 100 {
			conf = 100
		}
		candidates = append(candidates, candidate{sig: sig, confidence: conf})
	}

	if len(candidates) == 0 {
		// No causal signal found — use earliest signal as best guess
		earliest := signals[0]
		return &RootCauseGuess{
			Description: fmt.Sprintf("Earliest detected signal: %s — %s", earliest.Reason, truncateMsg(earliest.Message, 100)),
			Signal:      earliest,
			Confidence:  30,
			Category:    earliest.Category,
		}
	}

	// Sort by confidence descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].confidence > candidates[j].confidence
	})

	best := candidates[0]
	return &RootCauseGuess{
		Description: fmt.Sprintf("%s: %s", best.sig.Reason, truncateMsg(best.sig.Message, 100)),
		Signal:      best.sig,
		Confidence:  best.confidence,
		Category:    best.sig.Category,
	}
}

// buildIncidentTitle generates a human-readable title for the incident.
func buildIncidentTitle(category, namespace, node string, signals []SignalEntry) string {
	scope := namespace
	if node != "" {
		scope = fmt.Sprintf("node %s", node)
	} else if namespace != "" {
		scope = fmt.Sprintf("namespace %s", namespace)
	}

	// Find the most common reason
	reasonCount := map[string]int{}
	for _, sig := range signals {
		reasonCount[sig.Reason]++
	}
	topReason := ""
	maxCount := 0
	for reason, count := range reasonCount {
		if count > maxCount {
			maxCount = count
			topReason = reason
		}
	}

	switch category {
	case "resource-pressure":
		return fmt.Sprintf("Resource pressure incident in %s (%s)", scope, topReason)
	case "scheduling":
		return fmt.Sprintf("Scheduling failure cascade in %s (%s)", scope, topReason)
	case "networking":
		return fmt.Sprintf("Network connectivity incident in %s (%s)", scope, topReason)
	case "storage":
		return fmt.Sprintf("Storage/volume incident in %s (%s)", scope, topReason)
	case "config":
		return fmt.Sprintf("Configuration/misconfiguration incident in %s (%s)", scope, topReason)
	case "application":
		return fmt.Sprintf("Application failure cascade in %s (%s)", scope, topReason)
	case "security":
		return fmt.Sprintf("Security-related incident in %s (%s)", scope, topReason)
	default:
		if topReason != "" {
			return fmt.Sprintf("Correlated incident in %s (%s)", scope, topReason)
		}
		return fmt.Sprintf("Correlated incident in %s (%d signals)", scope, len(signals))
	}
}

// --- Signal classification helpers ---

// classifyEventSeverity assigns a severity level to an event reason.
func classifyEventSeverity(reason string) string {
	criticalReasons := map[string]bool{
		"OOMKilling":         true,
		"OOMKilled":          true,
		"CrashLoopBackOff":   true,
		"BackOff":            true,
		"NodeNotReady":       true,
		"EvictionThreshold":  true,
		"Evicted":            true,
		"FailedScheduling":   true,
		"FailedMount":        true,
		"FailedAttachVolume": true,
		"Unhealthy":          true,
	}
	if criticalReasons[reason] {
		return "critical"
	}
	return "warning"
}

// categorizeSignal maps an event reason/message to a category.
func categorizeSignal(reason, message string) string {
	reasonLower := strings.ToLower(reason)
	msgLower := strings.ToLower(message)
	combined := reasonLower + " " + msgLower

	// Check scheduling first (more specific) — FailedScheduling often mentions memory/CPU
	switch {
	case strings.Contains(combined, "schedul") || strings.Contains(combined, "nodeaffinity") ||
		strings.Contains(combined, "taint") || strings.Contains(combined, "toleration") ||
		strings.Contains(combined, "not fit"):
		return "scheduling"

	case strings.Contains(combined, "memory") || strings.Contains(combined, "oom") ||
		strings.Contains(combined, "diskpressure") || strings.Contains(combined, "disk") ||
		strings.Contains(combined, "cpupress") || strings.Contains(combined, "pressure") ||
		strings.Contains(combined, "pidpressure") || strings.Contains(combined, "evict"):
		return "resource-pressure"

	case strings.Contains(combined, "network") || strings.Contains(combined, "dns") ||
		strings.Contains(combined, "connect") || strings.Contains(combined, "timeout") ||
		strings.Contains(combined, "unreachable") || strings.Contains(combined, "refused"):
		return "networking"

	case strings.Contains(combined, "volume") || strings.Contains(combined, "mount") ||
		strings.Contains(combined, "pvc") || strings.Contains(combined, "attach") ||
		strings.Contains(combined, "storage"):
		return "storage"

	case strings.Contains(combined, "configmap") || strings.Contains(combined, "secret") ||
		strings.Contains(combined, "config") || strings.Contains(combined, "env"):
		return "config"

	case strings.Contains(combined, "image") || strings.Contains(combined, "pull") ||
		strings.Contains(combined, "crash") || strings.Contains(combined, "backoff") ||
		strings.Contains(combined, "exit"):
		return "application"

	case strings.Contains(combined, "auth") || strings.Contains(combined, "rbac") ||
		strings.Contains(combined, "certificate") || strings.Contains(combined, "denied") ||
		strings.Contains(combined, "forbidden"):
		return "security"

	default:
		return "unknown"
	}
}

// isCausalReason returns true if the reason is likely a root cause rather than a symptom.
func isCausalReason(reason string) bool {
	causalReasons := map[string]bool{
		"NodeNotReady":       true,
		"MemoryPressure":     true,
		"DiskPressure":       true,
		"PIDPressure":        true,
		"NetworkUnavailable": true,
		"FailedScheduling":   true,
		"FailedMount":        true,
		"FailedAttachVolume": true,
		"Evicted":            true,
		"EvictionThreshold":  true,
		"NodeSysctlChange":   true,
	}
	return causalReasons[reason]
}

// classifyPhaseSeverity maps a pod phase to severity.
func classifyPhaseSeverity(phase corev1.PodPhase) string {
	switch phase {
	case corev1.PodFailed:
		return "critical"
	case corev1.PodPending:
		return "warning"
	default:
		return "info"
	}
}

// classifyNodeCondition maps a node condition type to severity.
func classifyNodeCondition(condType corev1.NodeConditionType) string {
	switch condType {
	case corev1.NodeMemoryPressure, corev1.NodeDiskPressure:
		return "critical"
	case corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
		return "warning"
	default:
		return "warning"
	}
}

// extractNodeFromEvent tries to find the node name from an event.
func extractNodeFromEvent(ev *corev1.Event) string {
	if ev.InvolvedObject.Kind == "Node" {
		return ev.InvolvedObject.Name
	}
	// Some events include the node name in the message
	msg := ev.Message
	if idx := strings.Index(msg, "node "); idx >= 0 {
		rest := msg[idx+5:]
		if space := strings.IndexByte(rest, ' '); space > 0 {
			return rest[:space]
		}
	}
	return ""
}

// extractWorkloadName strips pod hash suffixes to get the workload name.
func extractWorkloadName(podName string) string {
	// Pod names typically follow: <deployment>-<hash>-<hash>
	// or <statefulset>-<ordinal>
	parts := strings.Split(podName, "-")
	if len(parts) >= 3 {
		// Check if last part looks like a hash (5 chars, alphanumeric)
		last := parts[len(parts)-1]
		if len(last) == 5 && isAlnum(last) {
			secondLast := parts[len(parts)-2]
			if len(secondLast) == 5 && isAlnum(secondLast) {
				return strings.Join(parts[:len(parts)-2], "-")
			}
		}
	}
	// For StatefulSets: name-0, name-1
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if isNumeric(last) {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}
	return podName
}

func isAlnum(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return len(s) > 0
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// signalEqual checks if two signals are equivalent.
func signalEqual(a, b SignalEntry) bool {
	return a.Timestamp.Equal(b.Timestamp) &&
		a.Source == b.Source &&
		a.Namespace == b.Namespace &&
		a.Name == b.Name &&
		a.Reason == b.Reason
}

// parseIntSafe parses an integer with a fallback default.
func parseIntSafe(s string, def int) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return def, err
	}
	return v, nil
}

// generateIncidentRecommendations produces actionable recommendations.
func generateIncidentRecommendations(result IncidentCorrelationResult) []string {
	var recs []string

	if result.Summary.CriticalCount > 0 {
		recs = append(recs, fmt.Sprintf("%d critical incident(s) detected — immediate investigation required", result.Summary.CriticalCount))
	}

	// Top incident recommendation
	if len(result.Incidents) > 0 {
		top := result.Incidents[0]
		if top.RootCause != nil {
			recs = append(recs, fmt.Sprintf("Incident %q likely caused by: %s (confidence: %d%%)",
				top.Title, top.RootCause.Description, top.RootCause.Confidence))
		}
		if top.BlastRadius.AffectedPods > 0 {
			recs = append(recs, fmt.Sprintf("Incident %q affects %d pod(s) across %d namespace(s)",
				top.Title, top.BlastRadius.AffectedPods, len(top.BlastRadius.AffectedNamespaces)))
		}
	}

	// Category-specific recommendations
	catSeen := map[string]bool{}
	for _, inc := range result.Incidents {
		if catSeen[inc.Category] {
			continue
		}
		catSeen[inc.Category] = true
		switch inc.Category {
		case "resource-pressure":
			recs = append(recs, "Resource pressure detected — review node capacity, resource requests/limits, and consider autoscaling")
		case "scheduling":
			recs = append(recs, "Scheduling failures detected — check node resources, taints/tolerations, and affinity rules")
		case "storage":
			recs = append(recs, "Storage issues detected — verify PVC binding, storage class availability, and CSI driver health")
		case "networking":
			recs = append(recs, "Network issues detected — check CNI health, DNS resolution, and service connectivity")
		case "config":
			recs = append(recs, "Configuration issues detected — verify ConfigMap/Secret references and mount paths")
		}
	}

	if result.Summary.OrphanSignalCount > 10 {
		recs = append(recs, fmt.Sprintf("%d uncorrelated warning signals — review for potential isolated issues", result.Summary.OrphanSignalCount))
	}

	if result.HealthScore < 50 {
		recs = append(recs, fmt.Sprintf("Cluster incident health score is %d/100 — multiple active incidents require attention", result.HealthScore))
	}

	if len(recs) == 0 && result.Summary.TotalSignals == 0 {
		recs = append(recs, "No warning signals detected in the analysis window — cluster appears healthy")
	}

	return recs
}
